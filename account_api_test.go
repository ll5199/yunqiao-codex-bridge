package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageWindowAcceptsCurrentSub2APIUSDFields(t *testing.T) {
	var items []progressItem
	data := []byte(`[{"subscription":{"group_id":42},"progress":{"daily":{"used_usd":1.25,"limit_usd":10,"percentage":12.5},"weekly":{"used_usd":3.5,"limit_usd":30,"percentage":11.67},"monthly":{"used_usd":12,"limit_usd":120,"percentage":10}}}]`)
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("parse current Sub2API progress response: %v", err)
	}
	lines := usageLines(items)
	if lines[0] != "每日用量    $1.25 / $10.00    12%" {
		t.Fatalf("daily usage was not parsed from USD fields: %q", lines[0])
	}
	if lines[1] != "每周用量    $3.50 / $30.00    12%" {
		t.Fatalf("weekly usage was not parsed from USD fields: %q", lines[1])
	}
	if lines[2] != "每月用量    $12.00 / $120.00    10%" {
		t.Fatalf("monthly usage was not parsed from USD fields: %q", lines[2])
	}
}

func TestUsageWindowStillAcceptsLegacyFields(t *testing.T) {
	var items []progressItem
	data := []byte(`[{"progress":{"daily":{"used":2,"limit":20,"percentage":10}}}]`)
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("parse legacy Sub2API progress response: %v", err)
	}
	if got := usageLines(items)[0]; got != "每日用量    $2.00 / $20.00    10%" {
		t.Fatalf("legacy usage response regressed: %q", got)
	}
}

func TestUsageLinesAlwaysProduceThreeSingleLineRows(t *testing.T) {
	dailyLimit, weeklyLimit, monthlyLimit := 10.0, 30.0, 120.0
	lines := usageLines([]progressItem{{Progress: subscriptionProgress{
		Daily:   &usageWindow{Used: 1, Limit: &dailyLimit, Percentage: 10},
		Weekly:  &usageWindow{Used: 3, Limit: &weeklyLimit, Percentage: 10},
		Monthly: &usageWindow{Used: 12, Limit: &monthlyLimit, Percentage: 10},
	}}})
	for index, line := range lines {
		if strings.ContainsAny(line, "\r\n") {
			t.Fatalf("usage row %d wraps: %q", index, line)
		}
	}
	if !strings.HasPrefix(lines[0], "每日用量") || !strings.HasPrefix(lines[1], "每周用量") || !strings.HasPrefix(lines[2], "每月用量") {
		t.Fatalf("unexpected usage rows: %#v", lines)
	}
}

func TestNormalizeProviderIncludesRequestedUpstreams(t *testing.T) {
	tests := map[string]string{"openai": "ChatGPT", "gemini": "Gemini", "antigravity": "Gemini", "grok": "Grok"}
	for input, expected := range tests {
		if actual := normalizeProvider(input); actual != expected {
			t.Fatalf("normalizeProvider(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestProviderKeysJoinKeyGroupIDToActiveSubscriptionGroup(t *testing.T) {
	groupID := int64(42)
	group := &accountGroup{ID: groupID, Name: "Codex 月用户", Platform: "openai"}
	keys := []accountKey{{Key: "sk-user-key", Status: "active", GroupID: &groupID}}

	providers := providerKeysForEntitlements(
		keys,
		map[int64]bool{groupID: true},
		map[int64]*accountGroup{groupID: group},
	)

	if providers["ChatGPT"] != "sk-user-key" {
		t.Fatalf("key was not joined to its subscription group: %#v", providers)
	}
}

func TestProviderKeysRejectInactiveSubscriptionGroup(t *testing.T) {
	groupID := int64(42)
	group := &accountGroup{ID: groupID, Platform: "openai"}
	keys := []accountKey{{Key: "sk-user-key", Status: "active", GroupID: &groupID}}

	providers := providerKeysForEntitlements(keys, map[int64]bool{99: true}, map[int64]*accountGroup{groupID: group})
	if len(providers) != 0 {
		t.Fatalf("inactive subscription group was accepted: %#v", providers)
	}
}

func TestConsultationButtonCopyIsFixed(t *testing.T) {
	if consultationButtonText != "点我咨询购买" {
		t.Fatalf("unexpected consultation button: %q", consultationButtonText)
	}
}
