package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChatGPTAuthSurvivesRepeatedConfigGeneration(t *testing.T) {
	home := t.TempDir()
	credentials := []byte(`{"tokens":{"access_token":"private-chatgpt-token"}}`)
	path := filepath.Join(home, "auth.json")
	if err := os.WriteFile(path, credentials, 0600); err != nil {
		t.Fatal(err)
	}
	config := "cli_auth_credentials_store = \"auto\"\n"
	for i := 0; i < 3; i++ {
		enabled, err := preserveChatGPTAuth(home, config)
		if err != nil || !enabled {
			t.Fatalf("auth detection: %v %v", enabled, err)
		}
		config = updateCodexConfigWithAuth(config, codexProxyBase, "smart-auto", enabled)
		if !bridgeRequiresAuth(config) || strings.Count(config, "[model_providers.yunqiao_bridge]") != 1 {
			t.Fatal(config)
		}
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(home, "config.toml"))
		if err != nil {
			t.Fatal(err)
		}
		config = string(data)
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(credentials) {
		t.Fatal("credentials changed")
	}
	if strings.Contains(config, "private-chatgpt-token") {
		t.Fatal("credential leaked into config")
	}
	// A keyring user can opt in without an auth.json file.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	enabled, err := preserveChatGPTAuth(home, config)
	if err != nil || !enabled {
		t.Fatal("explicit auth lost")
	}
}

func TestRelayAuthIsolation(t *testing.T) {
	request := httptest.NewRequest("POST", codexProxyBase+"/responses", nil)
	for _, name := range []string{"Authorization", "Cookie", "ChatGPT-Account-ID", "OpenAI-Organization", "OpenAI-Project", "Proxy-Authorization", "X-API-Key", "Sec-WebSocket-Protocol"} {
		request.Header.Set(name, "chatgpt-secret")
	}
	request.Header.Set("Content-Type", "application/json")
	setRelayAuthorization(request, "relay-secret")
	if request.Header.Get("Authorization") != "Bearer relay-secret" {
		t.Fatal("relay key missing")
	}
	for _, values := range request.Header {
		for _, value := range values {
			if strings.Contains(value, "chatgpt-secret") {
				t.Fatal("ChatGPT credential forwarded")
			}
		}
	}
	if request.Header.Get("Content-Type") != "application/json" {
		t.Fatal("request content header lost")
	}
}

func TestAuthCannotTargetExternalProvider(t *testing.T) {
	config := updateCodexConfigWithAuth("", "https://example.com/v1", "test", true)
	if bridgeRequiresAuth(config) {
		t.Fatal("OpenAI auth enabled for external endpoint")
	}
}

func TestEffectiveCodexHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	actual, err := effectiveCodexHome()
	if err != nil || actual != home {
		t.Fatalf("%q %v", actual, err)
	}
	t.Setenv("CODEX_HOME", "relative")
	if _, err := effectiveCodexHome(); err == nil {
		t.Fatal("relative home accepted")
	}
}
