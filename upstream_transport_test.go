package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpstreamTransportAvoidsHTTP2ProtocolErrors(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.WriteString(writer, request.Proto)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	transport := newUpstreamTransport()
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
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
