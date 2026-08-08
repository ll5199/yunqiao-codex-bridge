package main

import "testing"

func TestStatusSummaryIsSingleLineAndBounded(t *testing.T) {
	got := statusSummary("第一行\r\n第二行   详情", 8)
	if got != "第一行 第二行…" {
		t.Fatalf("unexpected status summary: %q", got)
	}
}

func TestStatusSummaryKeepsShortText(t *testing.T) {
	const want = "已是最新版。"
	if got := statusSummary(want, 20); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
