package main

import (
	"strings"
	"testing"
)

func TestWithSmartRouterModelRequiresAllTargets(t *testing.T) {
	partial := withSmartRouterModel([]string{smartRouteGrok, smartRouteTerra, smartRouteSol, smartRouterModel})
	if containsString(partial, smartRouterModel) {
		t.Fatalf("Auto was exposed without every route target: %#v", partial)
	}

	complete := withSmartRouterModel([]string{smartRouteSol, smartRouteLuna, smartRouteGrok, smartRouteTerra})
	if !containsString(complete, smartRouterModel) {
		t.Fatalf("Auto was not exposed for a complete route set: %#v", complete)
	}
}

func TestChooseDefaultModelPrefersSmartRouter(t *testing.T) {
	models := []string{smartRouteSol, smartRouterModel, smartRouteGrok}
	if got := chooseDefaultModel(models, smartRouteSol); got != smartRouterModel {
		t.Fatalf("default model = %q, want %q", got, smartRouterModel)
	}
}

func TestChooseSmartRoute(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{name: "general defaults to Grok", input: "帮我写一封客户回访邮件", want: smartRouteGrok},
		{name: "structured batch uses Luna", input: "批量提取这些发票的日期、金额和税号，输出表格", want: smartRouteLuna},
		{name: "multiple documents use Terra", input: "请读取整个文件夹，综合多份文档给出项目总结", want: smartRouteTerra},
		{name: "high risk document uses Sol", input: "审查这份合同的合规风险条款", want: smartRouteSol},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision := chooseSmartRoute(map[string]any{"input": test.input})
			if decision.Model != test.want {
				t.Fatalf("route = %q (%s), want %q", decision.Model, decision.Reason, test.want)
			}
			if decision.Model == "gpt-6-astra" {
				t.Fatal("Astra must never be selected automatically")
			}
		})
	}
}

func TestChooseSmartRouteCountsUserFilesAndIgnoresToolOutput(t *testing.T) {
	items := make([]any, 0, 8)
	items = append(items, map[string]any{
		"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "分析附件"}},
	})
	for index := 0; index < 6; index++ {
		items = append(items, map[string]any{"type": "input_file", "role": "user", "file_id": "file"})
	}
	items = append(items, map[string]any{"type": "function_call_output", "role": "tool", "output": "合同 合规"})

	decision := chooseSmartRoute(map[string]any{"input": items})
	if decision.Model != smartRouteTerra || decision.FileCount != 6 {
		t.Fatalf("unexpected route: %#v", decision)
	}
	if strings.Contains(decision.Reason, "high_risk") {
		t.Fatal("tool output affected semantic routing")
	}
}

func TestSmartRouteMemoryKeepsToolContinuationOnSessionModel(t *testing.T) {
	memory := newSmartRouteMemory(4)
	first := memory.resolve("thread-1", chooseSmartRoute(map[string]any{"input": "审查合同风险"}))
	if first.Model != smartRouteSol {
		t.Fatalf("first route = %q, want Sol", first.Model)
	}
	continuation := memory.resolve("thread-1", chooseSmartRoute(map[string]any{
		"input": []any{map[string]any{"type": "function_call_output", "role": "tool", "output": "done"}},
	}))
	if continuation.Model != smartRouteSol || continuation.Reason != "session_continuation" {
		t.Fatalf("continuation route was not sticky: %#v", continuation)
	}
}
