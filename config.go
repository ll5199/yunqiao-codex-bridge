package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.velyn65.com/v1"
	providerID     = "yunqiao_bridge"
	appVersion     = "1.4.4"
)

type appConfig struct {
	BaseURL      string   `json:"base_url"`
	EncryptedKey string   `json:"encrypted_key"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
}

func normalizeBaseURL(raw string) (string, error) {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", errors.New("API 地址格式不正确")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("API 地址必须以 https:// 或 http:// 开头")
	}
	if strings.HasSuffix(strings.ToLower(parsed.Path), "/models") {
		parsed.Path = strings.TrimSuffix(parsed.Path, parsed.Path[len(parsed.Path)-len("/models"):])
		parsed.RawPath = ""
		value = strings.TrimRight(parsed.String(), "/")
	}
	return value, nil
}

func fetchModels(baseURL, apiKey string) ([]string, error) {
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("请输入 API Key")
	}

	request, err := http.NewRequest(http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)

	client := &http.Client{Timeout: 25 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接模型接口失败：%w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("读取模型接口失败：%w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if len(message) > 240 {
			message = message[:240] + "…"
		}
		return nil, fmt.Errorf("模型接口返回 HTTP %d：%s", response.StatusCode, message)
	}

	models, err := parseModelsJSON(body)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, errors.New("接口连接成功，但没有返回可用模型")
	}
	return withSmartRouterModel(models), nil
}

func parseModelsJSON(body []byte) ([]string, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root any
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("模型接口返回的不是有效 JSON：%w", err)
	}

	var items []any
	switch value := root.(type) {
	case []any:
		items = value
	case map[string]any:
		for _, key := range []string{"data", "models", "items"} {
			if array, ok := value[key].([]any); ok {
				items = array
				break
			}
		}
	}

	seen := make(map[string]bool)
	models := make([]string, 0, len(items))
	for _, item := range items {
		var name string
		switch value := item.(type) {
		case string:
			name = value
		case map[string]any:
			for _, key := range []string{"id", "slug", "name", "model"} {
				if text, ok := value[key].(string); ok && strings.TrimSpace(text) != "" {
					name = text
					break
				}
			}
		}
		name = strings.TrimSpace(name)
		if name != "" && !seen[name] {
			seen[name] = true
			models = append(models, name)
		}
	}
	sort.Strings(models)
	return models, nil
}

func chooseDefaultModel(models []string, previous string) string {
	if containsString(models, smartRouterModel) {
		return smartRouterModel
	}
	if containsString(models, previous) {
		return previous
	}
	for _, preferred := range []string{"gpt-5.6-sol", "gpt-5.6", "gpt-5.5", "gpt-5.4"} {
		if containsString(models, preferred) {
			return preferred
		}
	}
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

func compareVersions(left, right string) int {
	leftParts := strings.Split(strings.TrimPrefix(strings.TrimSpace(left), "v"), ".")
	rightParts := strings.Split(strings.TrimPrefix(strings.TrimSpace(right), "v"), ".")
	length := len(leftParts)
	if len(rightParts) > length {
		length = len(rightParts)
	}
	for index := 0; index < length; index++ {
		leftValue, rightValue := versionPart(leftParts, index), versionPart(rightParts, index)
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

func versionPart(parts []string, index int) int {
	if index >= len(parts) {
		return 0
	}
	digits := strings.Builder{}
	for _, character := range parts[index] {
		if character < '0' || character > '9' {
			break
		}
		digits.WriteRune(character)
	}
	value, _ := strconv.Atoi(digits.String())
	return value
}

func updateCodexConfig(existing, baseURL, model string) string {
	const beginMarker = "# BEGIN YUNQIAO CODEX BRIDGE"
	const endMarker = "# END YUNQIAO CODEX BRIDGE"

	lines := strings.Split(strings.ReplaceAll(existing, "\r\n", "\n"), "\n")
	clean := make([]string, 0, len(lines))
	inMarker := false
	inProvider := false
	inTopLevel := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == beginMarker {
			inMarker = true
			continue
		}
		if inMarker {
			if trimmed == endMarker {
				inMarker = false
			}
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTopLevel = false
			inProvider = trimmed == "[model_providers."+providerID+"]" ||
				strings.HasPrefix(trimmed, "[model_providers."+providerID+".")
			if inProvider {
				continue
			}
		} else if inProvider {
			continue
		}

		if inTopLevel {
			if key, _, ok := strings.Cut(trimmed, "="); ok {
				key = strings.TrimSpace(key)
				if key == "model_provider" || key == "model" {
					continue
				}
			}
		}
		clean = append(clean, line)
	}

	body := strings.TrimSpace(strings.Join(clean, "\n"))
	top := []string{
		beginMarker,
		"model = " + strconv.Quote(model),
		"model_provider = " + strconv.Quote(providerID),
		endMarker,
	}
	provider := []string{
		beginMarker,
		"[model_providers." + providerID + "]",
		`name = "Yunqiao API"`,
		"base_url = " + strconv.Quote(strings.TrimRight(baseURL, "/")),
		`wire_api = "responses"`,
		`requires_openai_auth = false`,
		endMarker,
	}

	parts := []string{strings.Join(top, "\n")}
	if body != "" {
		parts = append(parts, body)
	}
	parts = append(parts, strings.Join(provider, "\n"))
	return strings.Join(parts, "\n\n") + "\n"
}

func removeYunqiaoCodexConfig(existing string) string {
	const beginMarker = "# BEGIN YUNQIAO CODEX BRIDGE"
	const endMarker = "# END YUNQIAO CODEX BRIDGE"

	normalized := strings.ReplaceAll(existing, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	legacyProvider := false
	inTopLevel := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTopLevel = false
			continue
		}
		if !inTopLevel {
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if ok && strings.TrimSpace(key) == "model_provider" &&
			strings.Trim(strings.TrimSpace(value), "\"'") == providerID {
			legacyProvider = true
		}
	}

	clean := make([]string, 0, len(lines))
	inMarker := false
	inProvider := false
	inTopLevel = true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == beginMarker {
			inMarker = true
			continue
		}
		if inMarker {
			if trimmed == endMarker {
				inMarker = false
			}
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inTopLevel = false
			inProvider = trimmed == "[model_providers."+providerID+"]" ||
				strings.HasPrefix(trimmed, "[model_providers."+providerID+".")
			if inProvider {
				continue
			}
		} else if inProvider {
			continue
		}
		if legacyProvider && inTopLevel {
			if key, _, ok := strings.Cut(trimmed, "="); ok {
				key = strings.TrimSpace(key)
				if key == "model" || key == "model_provider" {
					continue
				}
			}
		}
		clean = append(clean, line)
	}

	body := strings.TrimSpace(strings.Join(clean, "\n"))
	if body == "" {
		return ""
	}
	return body + "\n"
}
