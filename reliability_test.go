package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type reliabilityTransportFunc func(*http.Request) (*http.Response, error)

func (f reliabilityTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reliabilityRequest() *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "http://upstream/v1/responses", strings.NewReader(`{"model":"test","input":"hello"}`))
	return r
}
func TestOverloadRetryIsBounded(t *testing.T) {
	calls := 0
	tr := &compatibilityTransport{retryDelay: 0, base: reliabilityTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 502, Body: io.NopCloser(strings.NewReader("Concurrency limit exceeded for user")), Header: make(http.Header)}, nil
	})}
	resp, err := tr.RoundTrip(reliabilityRequest())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if calls != 3 {
		t.Fatalf("wanted three attempts, got %d", calls)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), "Concurrency limit") {
		t.Fatalf("final error body lost: %s", b)
	}
}
func TestSuccessfulStreamIsNeverReplayed(t *testing.T) {
	calls := 0
	tr := &compatibilityTransport{base: reliabilityTransportFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: output\n\ndata: overloaded\n\n")), Header: make(http.Header)}, nil
	})}
	resp, err := tr.RoundTrip(reliabilityRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if calls != 1 {
		t.Fatalf("stream replayed %d times", calls)
	}
}
func TestRetryCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := reliabilityRequest().WithContext(ctx)
	cancel()
	if err := waitForCompatibilityRetry(r, time.Hour); err != context.Canceled {
		t.Fatalf("got %v", err)
	}
}
func TestDiagnosticRedaction(t *testing.T) {
	text := redactDiagnostics("Authorization: Bearer bearer-secret\napi_key=custom-secret\naccess_token=token-secret\npassword=pass-secret\n sk-abcd1234\n saved-secret", "saved-secret")
	for _, s := range []string{"bearer-secret", "custom-secret", "token-secret", "pass-secret", "sk-abcd1234", "saved-secret"} {
		if strings.Contains(text, s) {
			t.Fatalf("secret leaked: %s", s)
		}
	}
}
func TestOverloadInspectionPreservesLongBody(t *testing.T) {
	original := strings.Repeat("x", 100000)
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(original))}
	if isOverloadResponse(resp) {
		t.Fatal("ordinary error marked overloaded")
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != original {
		t.Fatal("inspection truncated the error body")
	}
}
