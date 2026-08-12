package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseModelsJSON(t *testing.T) {
	body := []byte(`{"data":[{"id":"gpt-5.6-sol"},{"id":"gpt-5.6-terra"},{"id":"gpt-5.6-sol"}]}`)
	models, err := parseModelsJSON(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0] != "gpt-5.6-sol" || models[1] != "gpt-5.6-terra" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestNormalizeBaseURLAcceptsModelsEndpoint(t *testing.T) {
	value, err := normalizeBaseURL("https://api.example.com/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	if value != "https://api.example.com/v1" {
		t.Fatalf("unexpected base URL: %s", value)
	}
}

func TestFetchModelsUsesEnteredKeyAndParsesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/models" {
			t.Fatalf("unexpected models path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer sk-entered" {
			t.Fatalf("entered key was not forwarded: %q", request.Header.Get("Authorization"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.6-sol"}]}`))
	}))
	defer server.Close()

	models, err := fetchModels(server.URL+"/v1", "sk-entered")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0] != "gpt-5.6-sol" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestUpdateCodexConfigPreservesOtherSections(t *testing.T) {
	existing := "model=\"old\"\nmodel_provider = \"openai\"\n\n[features]\nweb_search = true\n"
	updated := updateCodexConfig(existing, "https://api.example.com/v1", "gpt-5.6-sol")

	for _, expected := range []string{
		`model = "gpt-5.6-sol"`,
		`model_provider = "yunqiao_bridge"`,
		`[model_providers.yunqiao_bridge]`,
		`base_url = "https://api.example.com/v1"`,
		`requires_openai_auth = false`,
		`[features]`,
		`web_search = true`,
	} {
		if !strings.Contains(updated, expected) {
			t.Fatalf("missing %q in:\n%s", expected, updated)
		}
	}
	if strings.Contains(updated, `model = "old"`) {
		t.Fatal("old top-level model was not removed")
	}
}

func TestUpdateCodexConfigIsIdempotent(t *testing.T) {
	first := updateCodexConfig("", defaultBaseURL, "gpt-5.6-sol")
	second := updateCodexConfig(first, defaultBaseURL, "gpt-5.6-terra")
	if strings.Count(second, "[model_providers."+providerID+"]") != 1 {
		t.Fatalf("provider duplicated:\n%s", second)
	}
	if strings.Contains(second, "experimental_bearer_token") {
		t.Fatal("experimental token field remains")
	}
}

func TestCompareVersions(t *testing.T) {
	for _, test := range []struct {
		left, right string
		want        int
	}{
		{"1.3.3", "1.3.2", 1},
		{"1.3.2", "1.3.1", 1},
		{"1.3.1", "1.3.0", 1},
		{"1.3.0", "1.2.4", 1},
		{"v1.2.4", "1.2.4", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.2.4-beta", "1.2.4", 0},
	} {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
