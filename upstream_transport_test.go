package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestUpstreamTransportBoundsWaitForResponseHeaders(t *testing.T) {
	transport := newUpstreamTransport()
	defer transport.CloseIdleConnections()
	if transport.ResponseHeaderTimeout != upstreamHeaderTimeout || upstreamHeaderTimeout <= 0 {
		t.Fatalf("upstream header wait has no bound: %s", transport.ResponseHeaderTimeout)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(100 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	transport.ResponseHeaderTimeout = 20 * time.Millisecond
	_, err := (&http.Client{Transport: transport}).Get(server.URL)
	if err == nil || !strings.Contains(err.Error(), "timeout awaiting response headers") {
		t.Fatalf("expected bounded response header wait, got %v", err)
	}
}

func TestUpstreamTransportAvoidsHTTP2ProtocolErrors(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, request.Proto)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	transport := newUpstreamTransport()
	if len(transport.TLSClientConfig.NextProtos) != 1 || transport.TLSClientConfig.NextProtos[0] != "http/1.1" {
		t.Fatalf("outbound ALPN must advertise HTTP/1.1 only, got %v", transport.TLSClientConfig.NextProtos)
	}
	certificateConfig := server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	certificateConfig.NextProtos = append([]string(nil), transport.TLSClientConfig.NextProtos...)
	transport.TLSClientConfig = certificateConfig
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "HTTP/1.1" {
		t.Fatalf("expected HTTP/1.1 upstream, got %q", body)
	}
}

func TestLiveUpstreamHTTP11Handshake(t *testing.T) {
	if os.Getenv("YUNQIAO_LIVE_UPSTREAM_TEST") != "1" {
		t.Skip("enable explicitly to verify the deployed endpoint")
	}
	client := &http.Client{Transport: newUpstreamTransport()}
	response, err := client.Get("https://api.velyn65.com/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Proto != "HTTP/1.1" || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected upstream protocol/status: %s %d", response.Proto, response.StatusCode)
	}
}

func TestNativeResponsesRequestIsReplayable(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://localhost/v1/responses", strings.NewReader(`{"model":"gpt-6-luna","input":"hello"}`))
	adaptResponsesRequest(request, "/v1/responses", nil)
	if request.GetBody == nil {
		t.Fatal("Responses request body cannot be replayed")
	}
	copy, err := request.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	body, _ := io.ReadAll(copy)
	if !strings.Contains(string(body), `"gpt-6-luna"`) {
		t.Fatalf("unexpected replay body: %s", body)
	}
}
