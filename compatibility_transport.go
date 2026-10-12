package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type compatibilityTransport struct {
	base          http.RoundTripper
	gemini        sync.Mutex
	logger        func(string, string)
	retryDelay    time.Duration
	fallbackModel func(string) (string, string)
}

func newCompatibilityTransport(base http.RoundTripper, logger func(string, string)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &compatibilityTransport{base: base, logger: logger, retryDelay: 3 * time.Second}
}

func (transport *compatibilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
 // Never replay a successful response: its stream may already contain output or tool calls.
 current:=request
 for attempt:=0; ; attempt++ {
  response,err:=transport.roundTripOnce(current)
  if err!=nil || response==nil || current.GetBody==nil { return response,err }
  retryable:=response.StatusCode==http.StatusTooManyRequests || response.StatusCode==http.StatusServiceUnavailable
  if response.StatusCode==http.StatusBadGateway { retryable=isOverloadResponse(response) }
  if !retryable || attempt>=2 { return response,nil }
  next,cloneErr:=cloneRequestWithBody(current)
  if cloneErr!=nil { return response,nil }
  if attempt==0 && current.Header.Get("X-Yunqiao-Smart-Route")=="1" && transport.fallbackModel!=nil {
   model,effort:=transport.fallbackModel(current.Header.Get("X-Yunqiao-Model"))
   if model!="" && model!=current.Header.Get("X-Yunqiao-Model") {
    if fallback,e:=smartRouteFallbackRequest(current,model,effort); e==nil { next=fallback }
   }
  }
  delay:=transport.retryDelay*time.Duration(1<<attempt)
  if transport.logger!=nil { transport.logger("protocol.retry",fmt.Sprintf("attempt=%d status=%d delay=%s",attempt+1,response.StatusCode,delay)) }
  _=response.Body.Close()
  if err:=waitForCompatibilityRetry(current,delay); err!=nil { _=next.Body.Close(); return nil,err }
  current=next
 }
}

func isOverloadResponse(response *http.Response) bool {
 prefix,err:=io.ReadAll(io.LimitReader(response.Body,64<<10))
 // Preserve the entire response, including any bytes beyond the inspected prefix.
 response.Body=&prefixedResponseBody{Reader:io.MultiReader(bytes.NewReader(prefix),response.Body),Closer:response.Body}
 if err!=nil { return false }
 lower:=strings.ToLower(string(prefix))
 return strings.Contains(lower,"concurrency limit") || strings.Contains(lower,"overloaded") || strings.Contains(lower,"concurrency slot")
}
type prefixedResponseBody struct { io.Reader; io.Closer }

func isGrokConcurrencyLimit(response *http.Response) bool {
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	_ = response.Body.Close()
	if err != nil {
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return false
	}
	lower := strings.ToLower(string(body))
	limited := strings.Contains(lower, "concurrency limit exceeded for user")
	if !limited {
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
	}
	return limited
}

func waitForCompatibilityRetry(request *http.Request, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-request.Context().Done():
		return request.Context().Err()
	case <-timer.C:
		return nil
	}
}

func cloneRequestWithBody(request *http.Request) (*http.Request, error) {
	body, err := request.GetBody()
	if err != nil {
		return nil, err
	}
	retry := request.Clone(request.Context())
	retry.Body = body
	retry.GetBody = request.GetBody
	return retry, nil
}

func (transport *compatibilityTransport) roundTripOnce(request *http.Request) (*http.Response, error) {
	if request.Header.Get("X-Yunqiao-Family") != "gemini" {
		return transport.base.RoundTrip(request)
	}
	started := time.Now()
	transport.gemini.Lock()
	waited := time.Since(started)
	if waited >= 100*time.Millisecond && transport.logger != nil {
		transport.logger("protocol.queue_wait", "family=gemini waited="+waited.Round(time.Millisecond).String())
	}
	response, err := transport.base.RoundTrip(request)
	if err != nil {
		transport.gemini.Unlock()
		return nil, err
	}

	response.Body = &unlockingReadCloser{ReadCloser: response.Body, unlock: transport.gemini.Unlock}
	return response, nil
}

func smartRouteFallbackRequest(request *http.Request, model string, effort string) (*http.Request, error) {
	body, err := request.GetBody()
	if err != nil {
		return nil, err
	}
	payload, err := io.ReadAll(io.LimitReader(body, compatibilityBodyLimit))
	_ = body.Close()
	if err != nil {
		return nil, err
	}
	var input map[string]any
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, err
	}
	input["model"] = model
	if effort != "" {
		input["reasoning"] = map[string]any{"effort": effort}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	retry := request.Clone(request.Context())
	retry.Body = io.NopCloser(bytes.NewReader(encoded))
	retry.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(encoded)), nil
	}
	retry.ContentLength = int64(len(encoded))
	retry.Header.Set("Content-Type", "application/json")
	retry.Header.Set("X-Yunqiao-Model", model)
	retry.Header.Del("X-Yunqiao-Family")
	if strings.Contains(strings.ToLower(model), "grok") {
		retry.Header.Set("X-Yunqiao-Family", "grok")
	}
	retry.Header.Set("X-Yunqiao-Reasoning-Effort", effort)
	retry.Header.Del("Content-Length")
	return retry, nil
}

type unlockingReadCloser struct {
	io.ReadCloser
	once   sync.Once
	unlock func()
}

func (body *unlockingReadCloser) Close() error {
	err := body.ReadCloser.Close()
	body.once.Do(body.unlock)
	return err
}
