package main

import (
	"errors"
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

// Older launcher configurations did not record this preference. Enable browser
// compatibility on upgrade; users without a native login can opt out in the UI.
func chatGPTAuthPreference(selected *bool) bool {
	return selected == nil || *selected
}

func setRelayAuthorization(request *http.Request, relayKey string) {
	for _, header := range []string{"Authorization", "Proxy-Authorization", "Cookie", "ChatGPT-Account-ID", "OpenAI-Organization", "OpenAI-Project", "X-API-Key", "Sec-WebSocket-Protocol"} {
		request.Header.Del(header)
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(relayKey))
}
