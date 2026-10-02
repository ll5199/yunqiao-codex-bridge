package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChatGPTAuthenticatedRelayReturnsModelAnswer(t *testing.T) {
	seen := make(chan http.Header, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"resp_auth_test","object":"response","status":"completed","model":"test-model","output":[{"id":"msg_auth_test","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"relay answer OK","annotations":[]}]}]}`)
	}))
	defer upstream.Close()
	proxy, err := startAPIProxy(upstream.URL+"/v1", "relay-only-key", nil, []string{"test-model"})
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.close()
	request, err := http.NewRequest("POST", codexProxyBase+"/responses", strings.NewReader(`{"model":"test-model","input":"hello","stream":false}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer private-chatgpt-token")
	request.Header.Set("ChatGPT-Account-ID", "private-account")
	request.Header.Set("Cookie", "private-session")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 || !strings.Contains(string(body), "relay answer OK") {
		t.Fatalf("%d %s", response.StatusCode, body)
	}
	headers := <-seen
	if headers.Get("Authorization") != "Bearer relay-only-key" || headers.Get("ChatGPT-Account-ID") != "" || headers.Get("Cookie") != "" {
		t.Fatalf("authentication isolation failed")
	}
}
