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
	base       http.RoundTripper
	gemini     sync.Mutex
	logger     func(string, string)
	retryDelay time.Duration
}

func newCompatibilityTransport(base http.RoundTripper, logger func(string, string)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &compatibilityTransport{base: base, logger: logger, retryDelay: 3 * time.Second}
}

func (transport *compatibilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.roundTripOnce(request)
	if err == nil && response != nil && response.StatusCode == http.StatusBadGateway &&
		request.GetBody != nil && request.Header.Get("X-Yunqiao-Model") == smartRouteGrok {
		if isGrokConcurrencyLimit(response) {
			if transport.logger != nil {
				transport.logger("protocol.retry", "family=grok reason=concurrency_limit delay="+transport.retryDelay.String())
			}
			if waitErr := waitForCompatibilityRetry(request, transport.retryDelay); waitErr != nil {
				return nil, waitErr
			}
			retry, retryErr := cloneRequestWithBody(request)
			if retryErr != nil {
				return nil, retryErr
			}
			response, err = transport.roundTripOnce(retry)
		}
	}
	if err != nil || response == nil || response.StatusCode != http.StatusServiceUnavailable ||
		request.Header.Get("X-Yunqiao-Smart-Route") != "1" || request.GetBody == nil ||
		request.Header.Get("X-Yunqiao-Model") == smartRouteGrok {
		return response, err
	}

	retry, retryErr := smartRouteFallbackRequest(request, smartRouteGrok)
	if retryErr != nil {
		return response, nil
	}
	_ = response.Body.Close()
	if transport.logger != nil {
		transport.logger("router.fallback", fmt.Sprintf(
			"from=%s to=%s reason=upstream_503",
			safeLogID(request.Header.Get("X-Yunqiao-Model")), smartRouteGrok,
		))
	}
	return transport.roundTripOnce(retry)
}

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
	if response.StatusCode == http.StatusTooManyRequests && request.GetBody != nil {
		body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		_ = response.Body.Close()
		lower := strings.ToLower(string(body))
		if readErr == nil && (strings.Contains(lower, "concurrency slot") || strings.Contains(lower, "resource_exhausted")) {
			if transport.logger != nil {
				transport.logger("protocol.retry", "family=gemini reason=concurrency_slot delay=3s")
			}
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-request.Context().Done():
				timer.Stop()
				transport.gemini.Unlock()
				return nil, request.Context().Err()
			case <-timer.C:
			}
			retry := request.Clone(request.Context())
			retry.Body, err = request.GetBody()
			if err != nil {
				transport.gemini.Unlock()
				return nil, err
			}
			retry.GetBody = request.GetBody
			response, err = transport.base.RoundTrip(retry)
			if err != nil {
				transport.gemini.Unlock()
				return nil, err
			}
		} else {
			response.Body = io.NopCloser(bytes.NewReader(body))
			response.ContentLength = int64(len(body))
		}
	}
	response.Body = &unlockingReadCloser{ReadCloser: response.Body, unlock: transport.gemini.Unlock}
	return response, nil
}

func smartRouteFallbackRequest(request *http.Request, model string) (*http.Request, error) {
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
	retry.Header.Set("X-Yunqiao-Family", "grok")
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
