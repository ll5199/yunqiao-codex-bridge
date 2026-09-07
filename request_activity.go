package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type requestActivityContextKey struct{}

type requestActivity struct {
	ID        string `json:"id"`
	Model     string `json:"model"`
	Stage     string `json:"stage"`
	StartedAt int64  `json:"started_at"`
	ElapsedMS int64  `json:"elapsed_ms"`
	started   time.Time
}

type requestActivitySnapshot struct {
	Active bool `json:"active"`
	requestActivity
}

type requestActivityTracker struct {
	mu      sync.Mutex
	active  map[string]*requestActivity
	counter atomic.Uint64
}

func newRequestActivityTracker() *requestActivityTracker {
	return &requestActivityTracker{active: make(map[string]*requestActivity)}
}

func (tracker *requestActivityTracker) begin(request *http.Request, model string) {
	if tracker == nil || request == nil || request.Method != http.MethodPost ||
		!strings.HasSuffix(strings.ToLower(request.URL.Path), "/responses") || strings.TrimSpace(model) == "" {
		return
	}
	id := time.Now().Format("150405.000") + "-" + stringID(tracker.counter.Add(1))
	started := time.Now()
	tracker.mu.Lock()
	tracker.pruneLocked(started)
	tracker.active[id] = &requestActivity{
		ID: id, Model: strings.TrimSpace(model), Stage: "waiting",
		StartedAt: started.UnixMilli(), started: started,
	}
	tracker.mu.Unlock()
	*request = *request.WithContext(context.WithValue(request.Context(), requestActivityContextKey{}, id))
}

func stringID(value uint64) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if value == 0 {
		return "0"
	}
	var result [20]byte
	index := len(result)
	for value > 0 {
		index--
		result[index] = alphabet[value%uint64(len(alphabet))]
		value /= uint64(len(alphabet))
	}
	return string(result[index:])
}

func requestActivityID(request *http.Request) string {
	if request == nil {
		return ""
	}
	id, _ := request.Context().Value(requestActivityContextKey{}).(string)
	return id
}

func (tracker *requestActivityTracker) setModel(id, model string) {
	if tracker == nil || id == "" || strings.TrimSpace(model) == "" {
		return
	}
	tracker.mu.Lock()
	if activity := tracker.active[id]; activity != nil {
		activity.Model = strings.TrimSpace(model)
	}
	tracker.mu.Unlock()
}

func (tracker *requestActivityTracker) setStage(id, stage string) {
	if tracker == nil || id == "" || stage == "" {
		return
	}
	tracker.mu.Lock()
	if activity := tracker.active[id]; activity != nil {
		activity.Stage = stage
	}
	tracker.mu.Unlock()
}

func (tracker *requestActivityTracker) markStreaming(id string) {
	if tracker == nil || id == "" {
		return
	}
	tracker.mu.Lock()
	if activity := tracker.active[id]; activity != nil && activity.Stage == "waiting" {
		activity.Stage = "streaming"
	}
	tracker.mu.Unlock()
}

func (tracker *requestActivityTracker) finish(id string) {
	if tracker == nil || id == "" {
		return
	}
	tracker.mu.Lock()
	delete(tracker.active, id)
	tracker.mu.Unlock()
}

func (tracker *requestActivityTracker) snapshot() requestActivitySnapshot {
	result := requestActivitySnapshot{}
	if tracker == nil {
		return result
	}
	now := time.Now()
	tracker.mu.Lock()
	tracker.pruneLocked(now)
	var latest *requestActivity
	for _, activity := range tracker.active {
		if latest == nil || activity.started.After(latest.started) {
			latest = activity
		}
	}
	if latest != nil {
		result.Active = true
		result.requestActivity = *latest
		result.ElapsedMS = now.Sub(latest.started).Milliseconds()
	}
	tracker.mu.Unlock()
	return result
}

func (tracker *requestActivityTracker) pruneLocked(now time.Time) {
	for id, activity := range tracker.active {
		if now.Sub(activity.started) > 6*time.Hour {
			delete(tracker.active, id)
		}
	}
	if len(tracker.active) <= 32 {
		return
	}
	for len(tracker.active) > 32 {
		var oldestID string
		var oldest time.Time
		for id, activity := range tracker.active {
			if oldestID == "" || activity.started.Before(oldest) {
				oldestID, oldest = id, activity.started
			}
		}
		delete(tracker.active, oldestID)
	}
}

func (tracker *requestActivityTracker) wrap(response *http.Response) {
	if tracker == nil || response == nil || response.Request == nil || response.Body == nil {
		return
	}
	id := requestActivityID(response.Request)
	if id == "" {
		return
	}
	tracker.setModel(id, response.Request.Header.Get("X-Yunqiao-Model"))
	response.Body = &activityReadCloser{source: response.Body, tracker: tracker, id: id}
}

func (tracker *requestActivityTracker) serveHTTP(writer http.ResponseWriter, request *http.Request) {
	setLocalCORS(writer)
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("Content-Type", "application/json")
	if request.Method == http.MethodOptions {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(writer).Encode(tracker.snapshot())
}

type activityReadCloser struct {
	source  io.ReadCloser
	tracker *requestActivityTracker
	id      string
	once    sync.Once
	buffer  []byte
}

func (body *activityReadCloser) Read(data []byte) (int, error) {
	count, err := body.source.Read(data)
	if count > 0 {
		body.tracker.markStreaming(body.id)
		body.buffer = append(body.buffer, data[:count]...)
		if len(body.buffer) > 2048 {
			body.buffer = append([]byte(nil), body.buffer[len(body.buffer)-2048:]...)
		}
		lower := bytes.ToLower(body.buffer)
		if bytes.Contains(lower, []byte(`"custom_tool_call"`)) ||
			bytes.Contains(lower, []byte(`"function_call"`)) ||
			bytes.Contains(lower, []byte(`"tool_search_call"`)) {
			body.tracker.setStage(body.id, "tool")
		}
	}
	if errors.Is(err, io.EOF) {
		body.finalize()
	}
	return count, err
}

func (body *activityReadCloser) Close() error {
	body.finalize()
	return body.source.Close()
}

func (body *activityReadCloser) finalize() {
	body.once.Do(func() { body.tracker.finish(body.id) })
}
