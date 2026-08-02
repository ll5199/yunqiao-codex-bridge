package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type compatibilityTransport struct {
	base   http.RoundTripper
	gemini sync.Mutex
	logger func(string, string)
}

func newCompatibilityTransport(base http.RoundTripper, logger func(string, string)) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &compatibilityTransport{base: base, logger: logger}
}

func (transport *compatibilityTransport) RoundTrip(request *http.Request) (*http.Response, error) {
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
