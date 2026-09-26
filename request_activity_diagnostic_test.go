package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestStreamDiagnosticsReportFirstByteCompletionAndEnd(t *testing.T) {
	var events []string
	tracker := newRequestActivityTracker(func(event, _ string) { events = append(events, event) })
	request, _ := http.NewRequest(http.MethodPost, "http://localhost/v1/responses", nil)
	tracker.begin(request, "智能路由判定")
	response := &http.Response{Request: request, Body: io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\"}\n\n"))}
	tracker.wrap(response)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	for _, expected := range []string{"proxy.stream_first_byte", "proxy.stream_completed_event", "proxy.stream_end"} {
		found := false
		for _, event := range events {
			if event == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing %s: %v", expected, events)
		}
	}
	if tracker.snapshot().Active {
		t.Fatal("completed stream is still active")
	}
}
