package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const compatibilityBodyLimit = 64 << 20

func adaptResponsesRequest(request *http.Request, path string, logger func(string, string)) (string, string) {
	return adaptResponsesRequestWithMemory(request, path, logger, nil)
}

func adaptResponsesRequestWithMemory(request *http.Request, path string, logger func(string, string), memory *smartRouteMemory) (string, string) {
	if request.Method != http.MethodPost || !strings.HasSuffix(strings.ToLower(path), "/responses") || request.Body == nil {
		return path, ""
	}
	body, err := io.ReadAll(io.LimitReader(request.Body, compatibilityBodyLimit))
	if err != nil {
		request.Body = io.NopCloser(bytes.NewReader(body))
		return path, ""
	}
	request.Body = io.NopCloser(bytes.NewReader(body))
	var input map[string]any
	if json.Unmarshal(body, &input) != nil {
		return path, ""
	}
	model := strings.TrimSpace(stringValue(input["model"]))
	if model == smartRouterModel {
		request.Header.Set("X-Yunqiao-Smart-Route", "1")
		decision := chooseSmartRoute(input)
		decision = memory.resolve(stringValue(input["prompt_cache_key"]), decision)
		input["model"] = decision.Model
		if smartRouteUsesLowReasoning(decision) {
			input["reasoning"] = map[string]any{"effort": "low"}
			request.Header.Set("X-Yunqiao-Reasoning-Effort", "low")
		}
		encoded, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			return path, ""
		}
		replaceRequestBody(request, encoded)
		model = decision.Model
		if logger != nil {
			logger("router.selected", fmt.Sprintf("model=%s reason=%s text_chars=%d files=%d",
				safeLogID(model), decision.Reason, decision.TextChars, decision.FileCount))
		}
	}
	request.Header.Set("X-Yunqiao-Model", model)
	lowerModel := strings.ToLower(model)
	if !strings.Contains(lowerModel, "gemini") && !strings.Contains(lowerModel, "grok") {
		return path, ""
	}
	if strings.Contains(lowerModel, "gemini") {
		request.Header.Set("X-Yunqiao-Family", "gemini")
	} else {
		request.Header.Set("X-Yunqiao-Family", "grok")
	}

	var adapted any
	protocol := "chat"
	adaptedPath := strings.TrimSuffix(path, "responses") + "chat/completions"
	switch {
	case strings.HasPrefix(lowerModel, "grok-imagine-video"):
		protocol = "video"
		adaptedPath = strings.TrimSuffix(path, "responses") + "videos/generations"
		adapted = map[string]any{
			"model": model, "prompt": responsesPrompt(input["input"]),
			"duration": 6, "resolution": "720p",
		}
	case strings.HasPrefix(lowerModel, "grok-imagine"):
		protocol = "image"
		adaptedPath = strings.TrimSuffix(path, "responses") + "images/generations"
		adapted = map[string]any{
			"model": model, "prompt": responsesPrompt(input["input"]),
		}
	case isGeminiImageModelName(lowerModel):
		protocol = "gemini-image"
		adaptedPath = "/v1beta/models/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":generateContent"
		adapted = map[string]any{
			"contents": []any{map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": responsesPrompt(input["input"])}},
			}},
			"generationConfig": map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}},
		}
	case strings.Contains(lowerModel, "grok"):
		// Sub2API's native Responses adapter preserves Codex-specific custom,
		// namespace and tool_search tools for Grok. Converting these requests to
		// Chat Completions here drops those tool definitions and leaves Grok able
		// to describe an action, but unable to actually invoke it.
		if logger != nil {
			logger("protocol.native", fmt.Sprintf("model=%s responses=%s protocol=responses", safeLogID(model), path))
		}
		return path, ""
	default:
		adapted = responsesToChatRequest(input)
	}
	encoded, err := json.Marshal(adapted)
	if err != nil {
		return path, ""
	}
	replaceRequestBody(request, encoded)
	if logger != nil {
		logger("protocol.adapted", fmt.Sprintf("model=%s responses=%s protocol=%s", safeLogID(model), adaptedPath, protocol))
	}
	return adaptedPath, protocol
}

func smartRouteUsesLowReasoning(decision smartRouteDecision) bool {
	return decision.Model == smartRouteGrok && (decision.Reason == "routine_bid_work" ||
		decision.Reason == "routine_file_operation" || decision.Reason == "session_continuation")
}

func replaceRequestBody(request *http.Request, body []byte) {
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	request.ContentLength = int64(len(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Del("Content-Length")
}

func responsesToChatRequest(input map[string]any) map[string]any {
	messages := make([]any, 0, 8)
	if instructions := strings.TrimSpace(stringValue(input["instructions"])); instructions != "" {
		messages = append(messages, map[string]any{"role": "system", "content": instructions})
	}
	switch value := input["input"].(type) {
	case string:
		messages = append(messages, map[string]any{"role": "user", "content": value})
	case []any:
		for _, raw := range value {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			role := strings.TrimSpace(stringValue(item["role"]))
			if role == "" {
				role = "user"
			}
			switch stringValue(item["type"]) {
			case "function_call_output":
				messages = append(messages, map[string]any{
					"role": "tool", "tool_call_id": stringValue(item["call_id"]),
					"content": stringValue(item["output"]),
				})
			case "function_call":
				messages = append(messages, map[string]any{
					"role": "assistant",
					"tool_calls": []any{map[string]any{
						"id": stringValue(item["call_id"]), "type": "function",
						"function": map[string]any{"name": stringValue(item["name"]), "arguments": stringValue(item["arguments"])},
					}},
				})
			default:
				content := chatContent(item["content"])
				if content != nil {
					messages = append(messages, map[string]any{"role": role, "content": content})
				}
			}
		}
	}
	if len(messages) == 0 {
		messages = append(messages, map[string]any{"role": "user", "content": responsesPrompt(input["input"])})
	}
	result := map[string]any{"model": input["model"], "messages": messages, "stream": false}
	if temperature, ok := input["temperature"]; ok {
		result["temperature"] = temperature
	}
	if maxTokens, ok := input["max_output_tokens"]; ok {
		result["max_tokens"] = maxTokens
	}
	if tools, ok := input["tools"].([]any); ok {
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || stringValue(tool["type"]) != "function" || stringValue(tool["name"]) == "" {
				continue
			}
			converted = append(converted, map[string]any{"type": "function", "function": map[string]any{
				"name": tool["name"], "description": tool["description"], "parameters": tool["parameters"],
			}})
		}
		if len(converted) > 0 {
			result["tools"] = converted
		}
	}
	return result
}

func chatContent(value any) any {
	if text, ok := value.(string); ok {
		return text
	}
	parts, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]any, 0, len(parts))
	for _, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch stringValue(part["type"]) {
		case "input_text", "output_text", "text":
			result = append(result, map[string]any{"type": "text", "text": stringValue(part["text"])})
		case "input_image", "image_url":
			imageURL := part["image_url"]
			if imageURL == nil {
				imageURL = part["url"]
			}
			result = append(result, map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}})
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func responsesPrompt(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	var parts []string
	var walk func(any)
	walk = func(current any) {
		switch item := current.(type) {
		case []any:
			for _, child := range item {
				walk(child)
			}
		case map[string]any:
			if text := strings.TrimSpace(stringValue(item["text"])); text != "" {
				parts = append(parts, text)
			}
			for _, key := range []string{"content", "input"} {
				walk(item[key])
			}
		}
	}
	walk(value)
	return strings.Join(parts, "\n")
}

func adaptCompatibilityResponse(response *http.Response, protocol string, store *imageStore, logger func(string, string)) error {
	body, err := io.ReadAll(io.LimitReader(response.Body, compatibilityBodyLimit))
	_ = response.Body.Close()
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if logger != nil {
			logger("protocol.upstream_error", fmt.Sprintf(
				"protocol=%s status=%d body=%s",
				protocol, response.StatusCode, safeLogBody(body),
			))
		}
		message := compatibilityErrorMessage(
			response.Request.Header.Get("X-Yunqiao-Family"),
			response.Request.Header.Get("X-Yunqiao-Model"),
			response.StatusCode,
			body,
		)
		converted := syntheticResponsesSSE(response.Request.Header.Get("X-Yunqiao-Model"), message)
		response.Body = io.NopCloser(bytes.NewReader(converted))
		response.StatusCode = http.StatusOK
		response.Status = "200 OK"
		response.ContentLength = int64(len(converted))
		response.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
		response.Header.Set("Cache-Control", "no-cache")
		response.Header.Del("Content-Encoding")
		response.Header.Del("Content-Length")
		return nil
	}
	captureResponseImages(body, response.Header.Get("Content-Type"), store)
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return nil
	}
	text := ""
	model := stringValue(root["model"])
	if protocol == "image" || protocol == "gemini-image" || len(extractImageSources(root)) > 0 {
		text = "图片已生成，并显示在当前对话中。"
	} else if protocol == "video" {
		text = videoResponseText(root)
	} else {
		text = chatResponseText(root)
	}
	if text == "" {
		text = "模型已返回响应，但没有可显示的文本。"
	}
	converted := syntheticResponsesSSE(model, text)
	response.Body = io.NopCloser(bytes.NewReader(converted))
	response.StatusCode = http.StatusOK
	response.Status = "200 OK"
	response.ContentLength = int64(len(converted))
	response.Header.Set("Content-Type", "text/event-stream; charset=utf-8")
	response.Header.Set("Cache-Control", "no-cache")
	response.Header.Del("Content-Encoding")
	response.Header.Del("Content-Length")
	if logger != nil {
		logger("protocol.response_converted", fmt.Sprintf("protocol=%s bytes=%d", protocol, len(body)))
	}
	return nil
}

func isGeminiImageModelName(model string) bool {
	return strings.Contains(model, "imagen") ||
		(strings.Contains(model, "gemini") && strings.Contains(model, "image"))
}

func videoResponseText(root map[string]any) string {
	id := stringValue(root["id"])
	if id == "" {
		id = stringValue(root["request_id"])
	}
	status := stringValue(root["status"])
	if id == "" {
		return "视频任务已提交。"
	}
	if status == "" {
		status = "queued"
	}
	return fmt.Sprintf("视频任务已提交。任务 ID：%s，状态：%s。", id, status)
}

func safeLogBody(body []byte) string {
	const limit = 1200
	text := strings.TrimSpace(string(body))
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " ")
	if len(text) > limit {
		text = text[:limit] + "…"
	}
	return text
}

func compatibilityErrorMessage(family, model string, status int, body []byte) string {
	detail := safeLogBody(body)
	lower := strings.ToLower(detail)
	switch {
	case status == http.StatusServiceUnavailable && strings.Contains(lower, "all available accounts exhausted"):
		return fmt.Sprintf("%s 暂时没有可用的上游账号（HTTP 503）。这是 Sub2API 账号池已耗尽或全部处于限流/不可用状态，请检查账号状态、并发、额度和调度冷却时间。", model)
	case status == http.StatusServiceUnavailable &&
		(strings.Contains(lower, "service temporarily unavailable") || strings.Contains(lower, "no available accounts")):
		return fmt.Sprintf("%s 暂时没有可用的上游账号（HTTP 503）。账号可能已达到额度、处于自动冷却或不支持当前模型；智能路由已尝试备用模型，请在 Sub2API 检查该分组的账号状态和模型映射。", model)
	case family == "gemini" && status == http.StatusTooManyRequests &&
		(strings.Contains(lower, "concurrency slot") || strings.Contains(lower, "resource_exhausted")):
		return "Gemini 请求未完成：当前 Sub2API 账号的并发槽仍被其他请求占用。Bridge 已启用 Gemini 请求排队；如果持续出现，请在 Sub2API 检查该账号的并发数、限流状态或更换可用账号。"
	case family == "grok" && status == http.StatusBadGateway &&
		strings.Contains(lower, "concurrency limit exceeded for user"):
		return "Grok 请求未完成：当前 Sub2API 上游账号的并发已满。Bridge 已等待 3 秒并自动重试一次；如果仍然出现，请稍后再试或在 Sub2API 增加可用 Grok 账号。"
	case family == "grok" && status == http.StatusBadRequest &&
		strings.Contains(lower, "xai upstream"):
		return fmt.Sprintf("Grok 请求被 xAI 上游拒绝（HTTP 400）。Bridge 已使用正确接口和请求格式；请在 Sub2API 检查模型 %s 所属的 xAI 渠道账号、额度、模型权限及渠道状态。", model)
	default:
		if detail == "" {
			detail = "上游没有返回详细信息"
		}
		return fmt.Sprintf("%s 请求失败（HTTP %d）：%s", model, status, detail)
	}
}

func chatResponseText(root map[string]any) string {
	choices, _ := root["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	choice, _ := choices[0].(map[string]any)
	message, _ := choice["message"].(map[string]any)
	switch content := message["content"].(type) {
	case string:
		return content
	case []any:
		var text []string
		for _, raw := range content {
			if part, ok := raw.(map[string]any); ok {
				if value := strings.TrimSpace(stringValue(part["text"])); value != "" {
					text = append(text, value)
				}
			}
		}
		return strings.Join(text, "\n")
	}
	return ""
}

func syntheticResponsesSSE(model, text string) []byte {
	if model == "" {
		model = "yunqiao-compatible"
	}
	responseID := fmt.Sprintf("resp_yunqiao_%d", time.Now().UnixNano())
	messageID := "msg_" + strings.TrimPrefix(responseID, "resp_")
	response := map[string]any{
		"id": responseID, "object": "response", "created_at": time.Now().Unix(),
		"status": "completed", "model": model,
		"output": []any{map[string]any{
			"id": messageID, "type": "message", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}},
		}},
		"usage": map[string]any{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0},
	}
	var output bytes.Buffer
	_ = writeSSEJSON(&output, "response.created", map[string]any{
		"type": "response.created", "response": cloneResponseForStatus(response, "in_progress", []any{}),
	})
	events := []struct {
		name string
		data map[string]any
	}{
		{"response.output_item.added", map[string]any{
			"type": "response.output_item.added", "output_index": 0,
			"item": map[string]any{"id": messageID, "type": "message", "status": "in_progress", "role": "assistant", "content": []any{}},
		}},
		{"response.content_part.added", map[string]any{
			"type": "response.content_part.added", "item_id": messageID, "output_index": 0,
			"content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
		}},
		{"response.output_text.delta", map[string]any{
			"type": "response.output_text.delta", "item_id": messageID, "output_index": 0, "content_index": 0, "delta": text,
		}},
		{"response.output_text.done", map[string]any{
			"type": "response.output_text.done", "item_id": messageID, "output_index": 0, "content_index": 0, "text": text,
		}},
		{"response.content_part.done", map[string]any{
			"type": "response.content_part.done", "item_id": messageID, "output_index": 0,
			"content_index": 0, "part": map[string]any{"type": "output_text", "text": text, "annotations": []any{}},
		}},
		{"response.output_item.done", map[string]any{
			"type": "response.output_item.done", "output_index": 0, "item": response["output"].([]any)[0],
		}},
	}
	for _, event := range events {
		_ = writeSSEJSON(&output, event.name, event.data)
	}
	_ = writeSSEJSON(&output, "response.completed", map[string]any{"type": "response.completed", "response": response})
	output.WriteString("data: [DONE]\n\n")
	return output.Bytes()
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
