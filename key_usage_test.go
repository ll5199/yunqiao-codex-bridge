package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const sampleKeyUsage = `{"balance":59.9073368,"daily_usage":[{"date":"2026-10-25","requests":16,"cost":1.61325244,"actual_cost":1.61325244},{"date":"2026-09-28","requests":90,"cost":2.06421572,"actual_cost":2.06421572}],"isValid":true,"mode":"unrestricted","model_stats":[{"model":"gpt-6-luna","requests":132,"cost":2.4007027,"actual_cost":2.4007027},{"model":"gpt-6-sol","requests":178,"cost":7.4190356,"actual_cost":7.4190356}],"planName":"钱包余额","remaining":59.9073368,"unit":"USD","usage":{"today":{"actual_cost":0,"cost":0,"requests":0,"total_tokens":0},"total":{"actual_cost":13.9079479,"cost":13.9079479,"requests":482,"total_tokens":36574257}}}`

func TestParseKeyUsage(t *testing.T) {
	usage, err := parseKeyUsage([]byte(sampleKeyUsage))
	if err != nil {
		t.Fatal(err)
	}
	if !usage.HasBalance || usage.Balance < 59.9 || usage.Plan != "钱包余额" || usage.Unit != "USD" {
		t.Fatalf("balance: %+v", usage)
	}
	if usage.TotalRequests != 482 || usage.TotalTokens != 36574257 || usage.TotalCost < 13.9 || usage.TodayRequests != 0 {
		t.Fatalf("totals: %+v", usage)
	}
	if usage.Daily[0].Date != "2026-09-28" || usage.Models[0].Model != "gpt-6-sol" {
		t.Fatalf("ordering: %+v %+v", usage.Daily, usage.Models)
	}
}

func TestParseKeyUsageWithoutBalance(t *testing.T) {
	usage, err := parseKeyUsage([]byte(`{"isValid":true,"usage":{"total":{"cost":1.5,"requests":3}}}`))
	if err != nil || usage.HasBalance || usage.TotalCost != 1.5 || usage.Unit != "USD" {
		t.Fatalf("got %+v %v", usage, err)
	}
	if _, err := parseKeyUsage([]byte(`{"isValid":false}`)); err == nil {
		t.Fatal("invalid key should fail")
	}
}

func TestFetchKeyUsageUsesKeyAndUsagePath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/usage" || request.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(writer, "bad", http.StatusUnauthorized)
			return
		}
		_, _ = writer.Write([]byte(sampleKeyUsage))
	}))
	defer server.Close()
	usage, err := fetchKeyUsage(server.URL+"/v1", "sk-test")
	if err != nil || usage.TotalRequests != 482 {
		t.Fatalf("got %+v %v", usage, err)
	}
	if _, err := fetchKeyUsage(server.URL+"/v1", "sk-wrong"); err == nil {
		t.Fatal("expected HTTP error")
	}
}
