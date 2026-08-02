package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	proxyPort       = 9230
	codexProxyBase  = "http://127.0.0.1:9230/v1"
	maxCaptureBytes = 64 << 20
)

type apiProxy struct {
	server    *http.Server
	store     *imageStore
	done      chan struct{}
	once      sync.Once
	startedAt int64
	logger    func(string, string)
}

func startAPIProxy(rawTarget, apiKey string, logger func(string, string)) (*apiProxy, error) {
	target, err := url.Parse(strings.TrimRight(strings.TrimSpace(rawTarget), "/"))
	if err != nil || target.Host == "" || (target.Scheme != "https" && target.Scheme != "http") {
		return nil, errors.New("中转 API 地址无效")
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort))
	if err != nil {
		return nil, fmt.Errorf("本机代理端口 %d 无法使用：%w", proxyPort, err)
	}

	store := newPersistentImageStore(imageStorageDirectory(), logger)
	transport := newCompatibilityTransport(http.DefaultTransport, logger)
	reverse := &httputil.ReverseProxy{
		FlushInterval: 30 * time.Millisecond,
		Transport:     transport,
		Director: func(request *http.Request) {
			incomingPath := request.URL.Path
			adaptedPath, protocol := adaptResponsesRequest(request, incomingPath, logger)
			if adaptedPath != "" {
				incomingPath = adaptedPath
			}
			if protocol != "" {
				request.Header.Set("X-Yunqiao-Protocol", protocol)
			}
			targetPath := strings.TrimRight(target.Path, "/")
			if protocol == "gemini-image" {
				targetPath = ""
			}
			if strings.HasSuffix(strings.ToLower(targetPath), "/v1") &&
				(strings.EqualFold(incomingPath, "/v1") || strings.HasPrefix(strings.ToLower(incomingPath), "/v1/")) {
				incomingPath = incomingPath[len("/v1"):]
			}
			request.URL.Scheme = target.Scheme
			request.URL.Host = target.Host
			request.URL.Path = targetPath + "/" + strings.TrimLeft(incomingPath, "/")
			request.Host = target.Host
			request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
			request.Header.Set("Accept-Encoding", "identity")
			request.Header.Set("X-Yunqiao-Bridge", appVersion)
		},
		ModifyResponse: func(response *http.Response) error {
			if logger != nil {
				logger("proxy.response", fmt.Sprintf(
					"status=%d method=%s path=%s content_type=%s",
					response.StatusCode,
					response.Request.Method,
					response.Request.URL.Path,
					response.Header.Get("Content-Type"),
				))
			}
			if protocol := response.Request.Header.Get("X-Yunqiao-Protocol"); protocol != "" {
				return adaptCompatibilityResponse(response, protocol, store, logger)
			}
			if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
				response.Header.Del("Content-Length")
				response.ContentLength = -1
				response.Body = newImageCompatibleSSEBody(response.Body, store, logger)
			} else {
				response.Body = &captureReadCloser{
					source:      response.Body,
					contentType: response.Header.Get("Content-Type"),
					store:       store,
				}
			}
			return nil
		},
		ErrorHandler: func(writer http.ResponseWriter, request *http.Request, proxyErr error) {
			if logger != nil {
				logger("proxy.error", proxyErr.Error())
			}
			http.Error(writer, "Yunqiao Bridge proxy error", http.StatusBadGateway)
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/yunqiao/health", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", "*")
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"status": "ok", "version": appVersion})
	})
	mux.HandleFunc("/yunqiao/images", func(writer http.ResponseWriter, request *http.Request) {
		setLocalCORS(writer)
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Type", "application/json")
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		conversationKey := strings.TrimSpace(request.URL.Query().Get("conversation_key"))
		_ = json.NewEncoder(writer).Encode(map[string]any{"images": store.listForConversation(conversationKey)})
	})
	mux.HandleFunc("/yunqiao/associate", func(writer http.ResponseWriter, request *http.Request) {
		setLocalCORS(writer)
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodPost {
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			ImageIDs        []string `json:"image_ids"`
			ConversationKey string   `json:"conversation_key"`
		}
		if err := json.NewDecoder(io.LimitReader(request.Body, 1<<20)).Decode(&payload); err != nil {
			http.Error(writer, "invalid json", http.StatusBadRequest)
			return
		}
		store.associate(payload.ImageIDs, payload.ConversationKey)
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"ok": true})
	})
	mux.HandleFunc("/yunqiao/image/", func(writer http.ResponseWriter, request *http.Request) {
		setLocalCORS(writer)
		store.serveImage(writer, request, strings.TrimPrefix(request.URL.Path, "/yunqiao/image/"), false)
	})
	mux.HandleFunc("/yunqiao/download/", func(writer http.ResponseWriter, request *http.Request) {
		setLocalCORS(writer)
		store.serveImage(writer, request, strings.TrimPrefix(request.URL.Path, "/yunqiao/download/"), true)
	})
	mux.Handle("/", reverse)

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 15 * time.Second,
	}
	proxy := &apiProxy{
		server: server, store: store, done: make(chan struct{}),
		startedAt: time.Now().UnixMilli(), logger: logger,
	}
	go func() {
		if logger != nil {
			logger("proxy.started", fmt.Sprintf("version=%s target=%s://%s%s", appVersion, target.Scheme, target.Host, target.Path))
		}
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && logger != nil {
			logger("proxy.stopped", serveErr.Error())
		}
	}()
	return proxy, nil
}

func (proxy *apiProxy) close() {
	if proxy == nil {
		return
	}
	proxy.once.Do(func() {
		close(proxy.done)
		if proxy.server != nil {
			_ = proxy.server.Close()
		}
	})
}

type captureReadCloser struct {
	source      io.ReadCloser
	contentType string
	store       *imageStore
	buffer      bytes.Buffer
	overflow    bool
	once        sync.Once
}

func (reader *captureReadCloser) Read(data []byte) (int, error) {
	count, err := reader.source.Read(data)
	if count > 0 && !reader.overflow {
		if reader.buffer.Len()+count <= maxCaptureBytes {
			_, _ = reader.buffer.Write(data[:count])
		} else {
			reader.overflow = true
		}
	}
	if errors.Is(err, io.EOF) {
		reader.finalize()
	}
	return count, err
}

func (reader *captureReadCloser) Close() error {
	reader.finalize()
	return reader.source.Close()
}

func (reader *captureReadCloser) finalize() {
	reader.once.Do(func() {
		if reader.overflow || reader.store == nil || reader.buffer.Len() == 0 {
			return
		}
		captureResponseImages(reader.buffer.Bytes(), reader.contentType, reader.store)
	})
}

type imageCompatibleSSEBody struct {
	reader *io.PipeReader
	source io.ReadCloser
	once   sync.Once
}

func newImageCompatibleSSEBody(source io.ReadCloser, store *imageStore, logger func(string, string)) io.ReadCloser {
	reader, writer := io.Pipe()
	body := &imageCompatibleSSEBody{reader: reader, source: source}
	go func() {
		err := transformImageSSE(source, writer, store, logger)
		_ = source.Close()
		_ = writer.CloseWithError(err)
	}()
	return body
}

func (body *imageCompatibleSSEBody) Read(data []byte) (int, error) {
	return body.reader.Read(data)
}

func (body *imageCompatibleSSEBody) Close() error {
	var closeErr error
	body.once.Do(func() {
		closeErr = body.source.Close()
		_ = body.reader.Close()
	})
	return closeErr
}

type imageSSEState struct {
	imageSeen        bool
	completedEmitted bool
	responseStarted  bool
	responseID       string
	model            string
	createdAt        int64
	imageEvents      int
}

func transformImageSSE(source io.Reader, destination io.Writer, store *imageStore, logger func(string, string)) error {
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 64<<10), maxCaptureBytes)
	state := &imageSSEState{createdAt: time.Now().Unix()}
	block := make([]string, 0, 4)

	flushBlock := func() error {
		if len(block) == 0 {
			return nil
		}
		lines := append([]string(nil), block...)
		block = block[:0]
		data := sseData(lines)
		if strings.TrimSpace(data) == "[DONE]" {
			if state.imageSeen && !state.completedEmitted {
				if err := writeImageCompletion(destination, state, nil); err != nil {
					return err
				}
				state.completedEmitted = true
			}
			_, err := io.WriteString(destination, "data: [DONE]\n\n")
			return err
		}

		var event map[string]any
		if json.Unmarshal([]byte(data), &event) != nil {
			return writeSSEBlock(destination, lines)
		}
		state.rememberResponse(event)
		eventType, _ := event["type"].(string)
		imageEvent := containsImageGeneration(event)
		if imageEvent {
			state.imageSeen = true
			state.imageEvents++
			for _, source := range extractImageSources(event) {
				store.add(source)
			}
		}

		if eventType == "response.completed" && state.imageSeen {
			if err := writeImageCompletion(destination, state, event); err != nil {
				return err
			}
			state.completedEmitted = true
			return nil
		}
		if imageEvent {
			return nil
		}
		return writeSSEBlock(destination, lines)
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flushBlock(); err != nil {
				return err
			}
			continue
		}
		block = append(block, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := flushBlock(); err != nil {
		return err
	}
	if state.imageSeen && !state.completedEmitted {
		if err := writeImageCompletion(destination, state, nil); err != nil {
			return err
		}
		state.completedEmitted = true
		if _, err := io.WriteString(destination, "data: [DONE]\n\n"); err != nil {
			return err
		}
	}
	if state.imageSeen && logger != nil {
		logger("image.stream_transformed", fmt.Sprintf("events=%d response_id=%s", state.imageEvents, safeLogID(state.responseID)))
	}
	return nil
}

func sseData(lines []string) string {
	var values []string
	for _, line := range lines {
		if strings.HasPrefix(line, "data:") {
			values = append(values, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	return strings.Join(values, "\n")
}

func writeSSEBlock(destination io.Writer, lines []string) error {
	for _, line := range lines {
		if _, err := io.WriteString(destination, line+"\n"); err != nil {
			return err
		}
	}
	_, err := io.WriteString(destination, "\n")
	return err
}

func containsImageGeneration(value any) bool {
	var walk func(any, int) bool
	walk = func(current any, depth int) bool {
		if depth > 8 || current == nil {
			return false
		}
		switch item := current.(type) {
		case []any:
			for _, child := range item {
				if walk(child, depth+1) {
					return true
				}
			}
		case map[string]any:
			for key, child := range item {
				if key == "type" || key == "kind" {
					if text, ok := child.(string); ok && strings.Contains(strings.ToLower(text), "image_generation") {
						return true
					}
				}
				if walk(child, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return walk(value, 0)
}

func (state *imageSSEState) rememberResponse(event map[string]any) {
	eventType, _ := event["type"].(string)
	if eventType == "response.created" || eventType == "response.in_progress" {
		state.responseStarted = true
	}
	response, _ := event["response"].(map[string]any)
	if response == nil {
		return
	}
	if value, ok := response["id"].(string); ok && value != "" {
		state.responseID = value
	}
	if value, ok := response["model"].(string); ok && value != "" {
		state.model = value
	}
	switch value := response["created_at"].(type) {
	case float64:
		state.createdAt = int64(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			state.createdAt = parsed
		}
	}
}

func writeImageCompletion(destination io.Writer, state *imageSSEState, completedEvent map[string]any) error {
	if state.responseID == "" {
		state.responseID = fmt.Sprintf("resp_yunqiao_%d", time.Now().UnixNano())
	}
	if state.model == "" {
		state.model = "yunqiao-image"
	}
	messageID := "msg_" + safeLogID(state.responseID)
	if messageID == "msg_" {
		messageID = fmt.Sprintf("msg_yunqiao_%d", time.Now().UnixNano())
	}
	text := "图片已生成，并显示在当前对话中。"

	response := map[string]any{
		"id":         state.responseID,
		"object":     "response",
		"created_at": state.createdAt,
		"status":     "completed",
		"model":      state.model,
		"output":     []any{},
		"usage": map[string]any{
			"input_tokens":  0,
			"output_tokens": 0,
			"total_tokens":  0,
		},
	}
	if existing, ok := completedEvent["response"].(map[string]any); ok {
		response = existing
		response["status"] = "completed"
	}

	output, _ := response["output"].([]any)
	cleaned := make([]any, 0, len(output)+1)
	hasMessage := false
	for _, item := range output {
		if containsImageGeneration(item) {
			continue
		}
		cleaned = append(cleaned, item)
		if object, ok := item.(map[string]any); ok && object["type"] == "message" {
			hasMessage = true
		}
	}
	if hasMessage {
		response["output"] = cleaned
		return writeSSEJSON(destination, "response.completed", map[string]any{
			"type":     "response.completed",
			"response": response,
		})
	}

	outputIndex := len(cleaned)
	message := map[string]any{
		"id":     messageID,
		"type":   "message",
		"status": "completed",
		"role":   "assistant",
		"content": []any{
			map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
		},
	}
	if !state.responseStarted {
		started := cloneResponseForStatus(response, "in_progress", cleaned)
		if err := writeSSEJSON(destination, "response.created", map[string]any{"type": "response.created", "response": started}); err != nil {
			return err
		}
		if err := writeSSEJSON(destination, "response.in_progress", map[string]any{"type": "response.in_progress", "response": started}); err != nil {
			return err
		}
	}
	events := []struct {
		name string
		data map[string]any
	}{
		{"response.output_item.added", map[string]any{
			"type": "response.output_item.added", "output_index": outputIndex,
			"item": map[string]any{"id": messageID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}},
		}},
		{"response.content_part.added", map[string]any{
			"type": "response.content_part.added", "item_id": messageID, "output_index": outputIndex,
			"content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		}},
		{"response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "item_id": messageID, "output_index": outputIndex,
			"content_index": 0, "delta": text,
		}},
		{"response.output_text.done", map[string]any{
			"type": "response.output_text.done", "item_id": messageID, "output_index": outputIndex,
			"content_index": 0, "text": text,
		}},
		{"response.content_part.done", map[string]any{
			"type": "response.content_part.done", "item_id": messageID, "output_index": outputIndex,
			"content_index": 0, "part": map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
		}},
		{"response.output_item.done", map[string]any{
			"type": "response.output_item.done", "output_index": outputIndex, "item": message,
		}},
	}
	for _, event := range events {
		if err := writeSSEJSON(destination, event.name, event.data); err != nil {
			return err
		}
	}
	response["output"] = append(cleaned, message)
	return writeSSEJSON(destination, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": response,
	})
}

func cloneResponseForStatus(response map[string]any, status string, output []any) map[string]any {
	cloned := make(map[string]any, len(response))
	for key, value := range response {
		cloned[key] = value
	}
	cloned["status"] = status
	cloned["output"] = append([]any(nil), output...)
	return cloned
}

func writeSSEJSON(destination io.Writer, eventName string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(destination, "event: %s\ndata: %s\n\n", eventName, encoded)
	return err
}

func safeLogID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 48 {
		return value[:48]
	}
	return value
}

func captureResponseImages(body []byte, contentType string, store *imageStore) {
	lowerType := strings.ToLower(contentType)
	if strings.HasPrefix(lowerType, "image/") {
		mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
		store.add("data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(body))
		return
	}

	parseJSON := func(payload []byte) {
		var root any
		if json.Unmarshal(bytes.TrimSpace(payload), &root) == nil {
			for _, source := range extractImageSources(root) {
				store.add(source)
			}
		}
	}
	if json.Valid(bytes.TrimSpace(body)) {
		parseJSON(body)
		return
	}
	for _, line := range bytes.Split(body, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if bytes.HasPrefix(line, []byte("data:")) {
			line = bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
		}
		if len(line) > 0 && !bytes.Equal(line, []byte("[DONE]")) {
			parseJSON(line)
		}
	}
}

func extractImageSources(root any) []string {
	var sources []string
	seen := make(map[string]bool)
	var walk func(any, string, int)
	walk = func(value any, parentType string, depth int) {
		if depth > 10 || value == nil {
			return
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				walk(child, parentType, depth+1)
			}
		case map[string]any:
			currentType := parentType
			for _, key := range []string{"type", "kind"} {
				if text, ok := item[key].(string); ok && text != "" {
					currentType = strings.ToLower(text)
					break
				}
			}
			mimeType := ""
			for _, key := range []string{"mime_type", "mimeType"} {
				if text, ok := item[key].(string); ok {
					mimeType = strings.ToLower(strings.TrimSpace(text))
					break
				}
			}
			imageType := strings.Contains(currentType, "image_generation") ||
				currentType == "output_image" || currentType == "image" ||
				currentType == "images" || strings.HasPrefix(mimeType, "image/")
			for key, child := range item {
				lowerKey := strings.ToLower(key)
				if text, ok := child.(string); ok {
					text = strings.TrimSpace(text)
					compact := strings.ReplaceAll(strings.ReplaceAll(text, "\r", ""), "\n", "")
					source := ""
					switch {
					case strings.HasPrefix(text, "data:image/"):
						source = text
					case (lowerKey == "b64_json" || lowerKey == "image_base64" ||
						(lowerKey == "result" && imageType) ||
						(lowerKey == "data" && imageType)) && looksLikeImageBase64(compact):
						if mimeType == "" {
							mimeType = "image/png"
						}
						source = "data:" + mimeType + ";base64," + compact
					case (lowerKey == "image_url" || lowerKey == "imageurl" || imageType) &&
						(strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://")):
						source = text
					case (lowerKey == "url" || lowerKey == "src") && looksLikeImageURL(text):
						source = text
					}
					if source != "" && !seen[source] {
						seen[source] = true
						sources = append(sources, source)
					}
				} else {
					childType := currentType
					if strings.Contains(lowerKey, "image") || lowerKey == "inline_data" || lowerKey == "inlinedata" {
						childType = "image"
					}
					walk(child, childType, depth+1)
				}
			}
		}
	}
	walk(root, "", 0)
	return sources
}

func looksLikeImageBase64(value string) bool {
	value = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "\r", ""), "\n", "")
	if len(value) < 256 {
		return false
	}
	if _, err := base64.StdEncoding.DecodeString(value); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(value, "="))
	return err == nil
}

func looksLikeImageURL(value string) bool {
	lower := strings.ToLower(value)
	for _, suffix := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif"} {
		if strings.Contains(lower, suffix) {
			return strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://")
		}
	}
	return false
}
