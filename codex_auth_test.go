package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRelayConfigIgnoresLegacyLoginPreference(t *testing.T) {
	home := t.TempDir()
	credentials := []byte(`{"tokens":{"access_token":"private-chatgpt-token"}}`)
	path := filepath.Join(home, "auth.json")
	if err := os.WriteFile(path, credentials, 0600); err != nil {
		t.Fatal(err)
	}
	// A prior version left false while native login was held in the system keyring.
	config := "cli_auth_credentials_store = \"keyring\"\n"
	enabled := true
	saved := appConfig{PreserveChatGPTAuth: &enabled}
	for i := 0; i < 3; i++ {
		config = updateCodexConfigWithAuth(config, codexProxyBase, "smart-auto", *saved.PreserveChatGPTAuth)
		if bridgeRequiresAuth(config) || strings.Count(config, "[model_providers.yunqiao_bridge]") != 1 {
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
		appData, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(appData, &saved); err != nil {
			t.Fatal(err)
		}
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != string(credentials) {
		t.Fatal("credentials changed")
	}
	if strings.Contains(config, "private-chatgpt-token") {
		t.Fatal("credential leaked into config")
	}
	disabled := false
	config = updateCodexConfigWithAuth(config, codexProxyBase, "smart-auto", disabled)
	if bridgeRequiresAuth(config) {
		t.Fatal("explicit disable ignored")
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

func TestLegacyAuthPreferenceDisabled(t *testing.T) {
	enabled, disabled := true, false
	for _, selected := range []*bool{nil, &enabled, &disabled} {
		if chatGPTAuthPreference(selected) {
			t.Fatal("legacy login preference must be ignored")
		}
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
