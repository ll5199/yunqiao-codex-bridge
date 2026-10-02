package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Resolve the same home as the launched client. Never copy or replace credentials.
func effectiveCodexHome() (string, error) {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		if !filepath.IsAbs(value) {
			return "", errors.New("CODEX_HOME 必须为绝对路径，以确保启动器和 Codex 使用同一认证目录")
		}
		return filepath.Clean(value), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codex"), nil
}

func bridgeRequiresAuth(config string) bool {
	inProvider := false
	for _, line := range strings.Split(config, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "#", 2)[0])
		if strings.HasPrefix(line, "[") {
			inProvider = line == "[model_providers."+providerID+"]"
			continue
		}
		if inProvider {
			key, value, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(key) == "requires_openai_auth" {
				return strings.TrimSpace(value) == "true"
			}
		}
	}
	return false
}

func preserveChatGPTAuth(home, config string) (bool, error) {
	// Explicit opt-in also supports credentials held only in the OS keyring.
	if bridgeRequiresAuth(config) {
		return true, nil
	}
	data, err := os.ReadFile(filepath.Join(home, "auth.json"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("无法检查 Codex 登录状态：%w", err)
	}
	var auth struct {
		Tokens *struct {
			AccessToken string `json:"access_token"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(data, &auth); err != nil {
		return false, errors.New("Codex auth.json 格式无效；未修改认证文件")
	}
	return auth.Tokens != nil && strings.TrimSpace(auth.Tokens.AccessToken) != "", nil
}

func setRelayAuthorization(request *http.Request, relayKey string) {
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "ChatGPT-Account-ID", "OpenAI-Organization", "OpenAI-Project", "X-API-Key", "Sec-WebSocket-Protocol"} {
		request.Header.Del(header)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(relayKey))
}
