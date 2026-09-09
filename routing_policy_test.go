package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutingPolicyRoundTripAndRejectsUnknownFields(t *testing.T) {
	body, err := json.Marshal(defaultRoutingPolicy())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := parseRoutingPolicy(body)
	if err != nil || policy.Default.Model != smartRouteGrok {
		t.Fatalf("valid policy was rejected: policy=%#v err=%v", policy, err)
	}
	invalid := strings.Replace(string(body), `"schema_version":1`, `"schema_version":1,"unexpected":true`, 1)
	if _, err := parseRoutingPolicy([]byte(invalid)); err == nil {
		t.Fatal("policy with an unknown field was accepted")
	}
}

func TestPublishedRoutingPolicyIsValid(t *testing.T) {
	body, err := os.ReadFile("routing-policy.json")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := parseRoutingPolicy(body)
	if err != nil {
		t.Fatal(err)
	}
	if policy.PolicyVersion != "2026-09-09.1" || !policy.Classifier.Enabled {
		t.Fatalf("unexpected published policy: %#v", policy)
	}
}

func TestRoutingPolicyCacheKeepsValidatedPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "routing-policy.json")
	body, _ := json.Marshal(defaultRoutingPolicy())
	if err := saveRoutingPolicyCache(path, body); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadRoutingPolicyCache(path)
	if err != nil || loaded.PolicyVersion != "builtin-1.5.0" {
		t.Fatalf("cache did not round trip: policy=%#v err=%v", loaded, err)
	}
	before, _ := os.ReadFile(path)
	if err := saveRoutingPolicyCache(path, []byte(`{"schema_version":1}`)); err == nil {
		t.Fatal("invalid policy replaced the cache")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid policy changed the last known-good cache")
	}
}

func TestDynamicRouterUsesClassifierAndConfiguredEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/responses" || request.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("unexpected classifier request: %s %v", request.URL.Path, request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"output_text":"{\"task_type\":\"document_cleanup\",\"model\":\"gpt-5.6-terra\",\"effort\":\"medium\",\"confidence\":0.91,\"reason\":\"多文档整理\"}"}`))
	}))
	defer server.Close()

	manager := newRoutingPolicyManager("", filepath.Join(t.TempDir(), "missing.json"), nil)
	router := newDynamicSmartRouter(manager, server.URL+"/v1", "secret", nil)
	decision := router.choose(context.Background(), map[string]any{
		"input": "整理项目资料并按网站栏目上传，先检查缺失内容",
	})
	if decision.Model != smartRouteTerra || decision.Effort != "medium" || decision.Reason != "classifier" {
		t.Fatalf("classifier decision was not used: %#v", decision)
	}
}

func TestDynamicRouterLowConfidenceUsesConfiguredFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"task_type\":\"ambiguous\",\"model\":\"grok-4.6\",\"effort\":\"low\",\"confidence\":0.55,\"reason\":\"不确定\"}"}}]}`))
	}))
	defer server.Close()
	manager := newRoutingPolicyManager("", filepath.Join(t.TempDir(), "missing.json"), nil)
	router := newDynamicSmartRouter(manager, server.URL, "secret", nil)
	decision := router.choose(context.Background(), map[string]any{"input": "帮我处理这些项目资料"})
	if decision.Model != smartRouteTerra || decision.Effort != "medium" || decision.Reason != "classifier_low_confidence" {
		t.Fatalf("low-confidence fallback was not used: %#v", decision)
	}
}

func TestWeightedRoutingIsSticky(t *testing.T) {
	policy := defaultRoutingPolicy()
	policy.Distribution.Enabled = true
	first := weightedRoutingDecision(map[string]any{"input": "任务", "prompt_cache_key": "thread-42"}, policy, smartRouteDecision{})
	second := weightedRoutingDecision(map[string]any{"input": "不同文字", "prompt_cache_key": "thread-42"}, policy, smartRouteDecision{})
	if first.Model == "" || first.Model != second.Model || first.Effort != second.Effort {
		t.Fatalf("weighted route was not sticky: first=%#v second=%#v", first, second)
	}
}
