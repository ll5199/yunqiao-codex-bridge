package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestExtractImageGenerationResult(t *testing.T) {
	payload := strings.Repeat("image-bytes", 40)
	encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	root := map[string]any{
		"type":   "image_generation_call",
		"result": encoded,
	}
	sources := extractImageSources(root)
	if len(sources) != 1 || sources[0] != "data:image/png;base64,"+encoded {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestCaptureSSEImage(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("png", 100)))
	event, _ := json.Marshal(map[string]any{
		"type":   "response.image_generation_call.completed",
		"result": encoded,
	})
	store := newImageStore(nil)
	captureResponseImages([]byte("event: completed\ndata: "+string(event)+"\n\n"), "text/event-stream", store)
	items := store.list()
	if len(items) != 1 || !strings.HasPrefix(items[0].Source, "data:image/png;base64,") {
		t.Fatalf("unexpected captured images: %#v", items)
	}
}

func TestCaptureDirectImageBody(t *testing.T) {
	store := newImageStore(nil)
	captureResponseImages([]byte("fake-png"), "image/png", store)
	if items := store.list(); len(items) != 1 || !strings.HasPrefix(items[0].Source, "data:image/png;base64,") {
		t.Fatalf("unexpected direct image capture: %#v", items)
	}
}

func TestCaptureGeminiInlineData(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("gemini-image", 40)))
	root := map[string]any{
		"candidates": []any{
			map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{
							"inlineData": map[string]any{
								"mimeType": "image/webp",
								"data":     encoded,
							},
						},
					},
				},
			},
		},
	}
	sources := extractImageSources(root)
	if len(sources) != 1 || sources[0] != "data:image/webp;base64,"+encoded {
		t.Fatalf("unexpected Gemini sources: %#v", sources)
	}
}

func TestCaptureGrokImageBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("grok-image", 40)))
	sources := extractImageSources(map[string]any{"image_base64": encoded})
	if len(sources) != 1 || sources[0] != "data:image/png;base64,"+encoded {
		t.Fatalf("unexpected Grok sources: %#v", sources)
	}
}

func TestAPIProxyForwardsAuthPathAndCapturesImage(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("pixel", 80)))
	requestSeen := make(chan *http.Request, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestSeen <- request.Clone(request.Context())
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"type":   "image_generation_call",
			"result": encoded,
		})
	}))
	defer upstream.Close()

	proxy, err := startAPIProxy(upstream.URL+"/v1", "test-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()

	response, err := http.Get(codexProxyBase + "/responses")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()

	request := <-requestSeen
	if request.URL.Path != "/v1/responses" {
		t.Fatalf("unexpected upstream path: %s", request.URL.Path)
	}
	if authorization := request.Header.Get("Authorization"); authorization != "Bearer test-secret" {
		t.Fatalf("unexpected authorization: %q", authorization)
	}
	if encoding := request.Header.Get("Accept-Encoding"); encoding != "identity" {
		t.Fatalf("unexpected accept-encoding: %q", encoding)
	}
	if images := proxy.store.list(); len(images) != 1 {
		t.Fatalf("expected one captured image, got %#v", images)
	}
}

func TestTransformImageSSEForCodexCompatibility(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("generated-image", 40)))
	response := map[string]any{
		"id":         "resp_test_image",
		"object":     "response",
		"created_at": 123,
		"status":     "completed",
		"model":      "gpt-image-1.5",
		"output": []any{
			map[string]any{
				"id":     "ig_test",
				"type":   "image_generation_call",
				"status": "completed",
				"result": encoded,
			},
		},
		"usage": map[string]any{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
	}
	events := []map[string]any{
		{"type": "response.created", "response": map[string]any{
			"id": "resp_test_image", "object": "response", "created_at": 123,
			"status": "in_progress", "model": "gpt-image-1.5", "output": []any{},
		}},
		{"type": "response.output_item.added", "output_index": 0, "item": map[string]any{
			"id": "ig_test", "type": "image_generation_call", "status": "in_progress",
		}},
		{"type": "response.image_generation_call.completed", "output_index": 0, "result": encoded},
		{"type": "response.output_item.done", "output_index": 0, "item": response["output"].([]any)[0]},
		{"type": "response.completed", "response": response},
	}
	var upstream strings.Builder
	for _, event := range events {
		encodedEvent, _ := json.Marshal(event)
		eventType, _ := event["type"].(string)
		upstream.WriteString("event: " + eventType + "\n")
		upstream.WriteString("data: " + string(encodedEvent) + "\n\n")
	}
	upstream.WriteString("data: [DONE]\n\n")

	store := newImageStore(nil)
	var transformed bytes.Buffer
	if err := transformImageSSE(strings.NewReader(upstream.String()), &transformed, store, nil); err != nil {
		t.Fatal(err)
	}
	output := transformed.String()
	if strings.Contains(output, "image_generation_call") || strings.Contains(output, encoded) {
		t.Fatalf("image payload leaked to Codex stream:\n%s", output)
	}
	for _, expected := range []string{
		"图片已生成，并显示在当前对话中。",
		"response.output_item.added",
		"response.output_text.delta",
		"response.completed",
		"data: [DONE]",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("transformed stream is missing %q:\n%s", expected, output)
		}
	}
	if count := strings.Count(output, `"type":"response.completed"`); count != 1 {
		t.Fatalf("expected one completion event, got %d:\n%s", count, output)
	}
	if images := store.list(); len(images) != 1 || images[0].Source != "data:image/png;base64,"+encoded {
		t.Fatalf("unexpected captured images: %#v", images)
	}
}

func TestTransformTextSSEUnchanged(t *testing.T) {
	input := "event: response.output_text.delta\n" +
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"data: [DONE]\n\n"
	var output bytes.Buffer
	if err := transformImageSSE(strings.NewReader(input), &output, newImageStore(nil), nil); err != nil {
		t.Fatal(err)
	}
	if output.String() != input {
		t.Fatalf("text stream changed:\nwant %q\ngot  %q", input, output.String())
	}
}

func TestPersistentImageDownloadAndConversation(t *testing.T) {
	root := t.TempDir()
	store := newPersistentImageStore(root, nil)
	encoded := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("persisted-png", 40)))
	store.add("data:image/png;base64," + encoded)
	items := store.list()
	if len(items) != 1 || items[0].DownloadURL == "" {
		t.Fatalf("unexpected persisted image: %#v", items)
	}
	if _, err := os.Stat(filepath.Join(root, items[0].FileName)); err != nil {
		t.Fatal(err)
	}
	store.associate([]string{items[0].ID}, "thread:test")
	reloaded := newPersistentImageStore(root, nil)
	associated := reloaded.listForConversation("thread:test")
	if len(associated) != 1 || associated[0].ID != items[0].ID {
		t.Fatalf("image association was not persisted: %#v", associated)
	}
	recorder := httptest.NewRecorder()
	reloaded.serveImage(recorder, httptest.NewRequest(http.MethodGet, associated[0].DownloadURL, nil), associated[0].ID, true)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("unexpected download response: code=%d headers=%v", recorder.Code, recorder.Header())
	}
}

func TestAdaptGeminiResponsesToChatCompletions(t *testing.T) {
	body := `{"model":"gemini-2.5-pro","instructions":"be concise","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"stream":true}`
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", strings.NewReader(body))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1/chat/completions" || protocol != "chat" {
		t.Fatalf("unexpected adapter route: %s %s", path, protocol)
	}
	adapted, _ := io.ReadAll(request.Body)
	if !bytes.Contains(adapted, []byte(`"messages"`)) || !bytes.Contains(adapted, []byte(`"stream":false`)) {
		t.Fatalf("unexpected adapted request: %s", adapted)
	}
}

func TestGrokResponsesPreservesCodexTools(t *testing.T) {
	body := `{"model":"grok-4.6","input":"inspect the workspace","stream":true,"tools":[{"type":"custom","name":"apply_patch","description":"Edit files"},{"type":"namespace","name":"image_gen"},{"type":"tool_search"}]}`
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", strings.NewReader(body))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1/responses" || protocol != "" {
		t.Fatalf("Grok agent request must use native Responses, got: %s %s", path, protocol)
	}
	forwarded, _ := io.ReadAll(request.Body)
	if string(forwarded) != body {
		t.Fatalf("Grok Responses body changed:\nwant %s\ngot  %s", body, forwarded)
	}
}

func TestSmartRouterRewritesVirtualModelAndPreservesResponsesTools(t *testing.T) {
	body := `{"model":"yunqiao-auto","input":"inspect the workspace","stream":true,"tools":[{"type":"custom","name":"apply_patch"},{"type":"namespace","name":"image_gen"},{"type":"tool_search"}]}`
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", strings.NewReader(body))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1/responses" || protocol != "" {
		t.Fatalf("Auto Grok request must use native Responses, got: %s %s", path, protocol)
	}
	forwarded, _ := io.ReadAll(request.Body)
	if bytes.Contains(forwarded, []byte(smartRouterModel)) || !bytes.Contains(forwarded, []byte(`"model":"grok-4.6"`)) {
		t.Fatalf("virtual model was not rewritten: %s", forwarded)
	}
	for _, toolType := range []string{`"type":"custom"`, `"type":"namespace"`, `"type":"tool_search"`} {
		if !bytes.Contains(forwarded, []byte(toolType)) {
			t.Fatalf("Codex tool %s was not preserved: %s", toolType, forwarded)
		}
	}
}

func TestAPIProxyPassesGrokResponsesToolsAndEventsThrough(t *testing.T) {
	type capturedRequest struct {
		path string
		body string
	}
	seen := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		seen <- capturedRequest{path: request.URL.Path, body: string(body)}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(writer, "event: response.output_item.done\n"+
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"custom_tool_call","call_id":"call_test","name":"apply_patch","input":"*** Begin Patch"}}`+"\n\n"+
			"data: [DONE]\n\n")
	}))
	defer upstream.Close()

	proxy, err := startAPIProxy(upstream.URL+"/v1", "test-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()

	body := `{"model":"grok-4.6","input":"edit a file","stream":true,"tools":[{"type":"custom","name":"apply_patch","description":"Edit files"},{"type":"namespace","name":"image_gen"},{"type":"tool_search"}]}`
	response, err := http.Post(codexProxyBase+"/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	responseBody, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()

	forwarded := <-seen
	if forwarded.path != "/v1/responses" || forwarded.body != body {
		t.Fatalf("unexpected Grok upstream request: path=%s body=%s", forwarded.path, forwarded.body)
	}
	if !bytes.Contains(responseBody, []byte(`"type":"custom_tool_call"`)) ||
		!bytes.Contains(responseBody, []byte(`"call_id":"call_test"`)) {
		t.Fatalf("Grok tool event was not preserved: %s", responseBody)
	}
}

func TestAdaptGrokImageToGenerations(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses",
		strings.NewReader(`{"model":"grok-imagine-1.0","input":"draw a dog"}`))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1/images/generations" || protocol != "image" {
		t.Fatalf("unexpected image adapter route: %s %s", path, protocol)
	}
}

func TestAdaptGrokVideoToVideoGenerations(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses",
		strings.NewReader(`{"model":"grok-imagine-video","input":"make a short video"}`))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1/videos/generations" || protocol != "video" {
		t.Fatalf("unexpected video adapter route: %s %s", path, protocol)
	}
}

func TestAdaptGeminiImageToNativeGenerateContent(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses",
		strings.NewReader(`{"model":"gemini-3.1-flash-lite-image","input":"draw a dog"}`))
	path, protocol := adaptResponsesRequest(request, request.URL.Path, nil)
	if path != "/v1beta/models/gemini-3.1-flash-lite-image:generateContent" || protocol != "gemini-image" {
		t.Fatalf("unexpected Gemini image adapter route: %s %s", path, protocol)
	}
	adapted, _ := io.ReadAll(request.Body)
	if !bytes.Contains(adapted, []byte(`"responseModalities":["TEXT","IMAGE"]`)) {
		t.Fatalf("unexpected Gemini image request: %s", adapted)
	}
}

func TestConvertChatResponseToResponsesSSE(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"model":"gemini-2.5-pro","choices":[{"message":{"content":"你好"}}]}`)),
		Request:    httptest.NewRequest(http.MethodPost, "http://upstream/v1/chat/completions", nil),
	}
	if err := adaptCompatibilityResponse(response, "chat", newImageStore(nil), nil); err != nil {
		t.Fatal(err)
	}
	converted, _ := io.ReadAll(response.Body)
	if !bytes.Contains(converted, []byte("你好")) || !bytes.Contains(converted, []byte("response.completed")) {
		t.Fatalf("unexpected converted response: %s", converted)
	}
}

func TestAPIProxyRoutesGeminiImageToNativeEndpoint(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("gemini-pixel", 40)))
	pathSeen := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		pathSeen <- request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"candidates": []any{map[string]any{
				"content": map[string]any{"parts": []any{map[string]any{
					"inlineData": map[string]any{"mimeType": "image/png", "data": image},
				}}},
			}},
		})
	}))
	defer upstream.Close()

	proxy, err := startAPIProxy(upstream.URL+"/v1", "test-secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()
	body := `{"model":"gemini-3.1-flash-lite-image","input":"draw a dog","stream":true}`
	response, err := http.Post(codexProxyBase+"/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	converted, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if path := <-pathSeen; path != "/v1beta/models/gemini-3.1-flash-lite-image:generateContent" {
		t.Fatalf("unexpected upstream Gemini path: %s", path)
	}
	if !bytes.Contains(converted, []byte("图片已生成")) {
		t.Fatalf("unexpected converted Gemini response: %s", converted)
	}
	expectedSource := "data:image/png;base64," + image
	expectedID := imageID(expectedSource)
	found := false
	for _, stored := range proxy.store.list() {
		if stored.ID == expectedID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected captured Gemini image %s, got %#v", expectedID, proxy.store.list())
	}
}

func imageID(source string) string {
	sum := sha256.Sum256([]byte(source))
	return hex.EncodeToString(sum[:12])
}

func TestCompatibilityErrorIsReadableResponsesStream(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"error":{"message":"timeout waiting for user concurrency slot","status":"RESOURCE_EXHAUSTED"}}`,
		)),
		Request: httptest.NewRequest(http.MethodPost, "http://upstream/v1beta/models/test:generateContent", nil),
	}
	response.Request.Header.Set("X-Yunqiao-Family", "gemini")
	response.Request.Header.Set("X-Yunqiao-Model", "gemini-image")
	if err := adaptCompatibilityResponse(response, "gemini-image", newImageStore(nil), nil); err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("并发槽")) ||
		!bytes.Contains(body, []byte("response.completed")) {
		t.Fatalf("unexpected readable error response: status=%d body=%s", response.StatusCode, body)
	}
}

func TestGeminiConcurrencyResponseRetriesOnce(t *testing.T) {
	calls := 0
	transport := newCompatibilityTransport(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		status := http.StatusOK
		body := `{"ok":true}`
		if calls == 1 {
			status = http.StatusTooManyRequests
			body = `{"error":{"message":"timeout waiting for user concurrency slot","status":"RESOURCE_EXHAUSTED"}}`
		}
		return &http.Response{
			StatusCode: status,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	}), nil)
	request := httptest.NewRequest(http.MethodPost, "http://upstream/v1beta/models/test:generateContent", strings.NewReader(`{}`))
	payload := []byte(`{}`)
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload)), nil
	}
	request.Header.Set("X-Yunqiao-Family", "gemini")
	response, err := transport.RoundTrip(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if calls != 2 || response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected retry result: calls=%d status=%d", calls, response.StatusCode)
	}
}

func TestExhaustedAccountErrorIsExplained(t *testing.T) {
	message := compatibilityErrorMessage(
		"gemini", "gemini-3.5-flash", http.StatusServiceUnavailable,
		[]byte(`{"error":{"message":"All available accounts exhausted","type":"server_error"}}`),
	)
	if !strings.Contains(message, "Sub2API") || !strings.Contains(message, "账号池") {
		t.Fatalf("unexpected exhausted account message: %s", message)
	}
}
