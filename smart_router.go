package main

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	smartRouterModel = "yunqiao-auto"
	smartRouteGrok   = "grok-4.6"
	smartRouteTerra  = "gpt-5.6-terra"
	smartRouteLuna   = "gpt-5.6-luna"
	smartRouteSol    = "gpt-5.6-sol"
)

var smartRouterTargets = []string{smartRouteGrok, smartRouteTerra, smartRouteLuna, smartRouteSol}

type smartRouteDecision struct {
	Model     string
	Reason    string
	TextChars int
	FileCount int
}

type smartRouteMemory struct {
	mu     sync.Mutex
	routes map[string]string
	order  []string
	limit  int
}

func newSmartRouteMemory(limit int) *smartRouteMemory {
	if limit < 1 {
		limit = 1
	}
	return &smartRouteMemory{routes: make(map[string]string), limit: limit}
}

func (memory *smartRouteMemory) resolve(cacheKey string, decision smartRouteDecision) smartRouteDecision {
	if memory == nil || strings.TrimSpace(cacheKey) == "" {
		return decision
	}
	cacheKey = strings.TrimSpace(cacheKey)
	memory.mu.Lock()
	defer memory.mu.Unlock()

	// Tool-call continuations can contain only function outputs. Reuse the
	// session's prior decision so an agent does not switch models mid-turn.
	if decision.TextChars == 0 && decision.FileCount == 0 {
		if model := memory.routes[cacheKey]; model != "" {
			decision.Model, decision.Reason = model, "session_continuation"
		}
		return decision
	}
	if _, exists := memory.routes[cacheKey]; !exists {
		memory.order = append(memory.order, cacheKey)
	}
	memory.routes[cacheKey] = decision.Model
	for len(memory.order) > memory.limit {
		oldest := memory.order[0]
		memory.order = memory.order[1:]
		delete(memory.routes, oldest)
	}
	return decision
}

// withSmartRouterModel exposes Auto only when every target needed by the
// policy exists. This prevents a saved or partially configured provider from
// advertising a virtual model that it cannot fulfill.
func withSmartRouterModel(models []string) []string {
	seen := make(map[string]bool, len(models)+1)
	result := make([]string, 0, len(models)+1)
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == smartRouterModel {
			continue
		}
		if model != "" && !seen[model] {
			seen[model] = true
			result = append(result, model)
		}
	}
	for _, model := range smartRouterTargets {
		if !seen[model] {
			sort.Strings(result)
			return result
		}
	}
	if !seen[smartRouterModel] {
		result = append(result, smartRouterModel)
	}
	sort.Strings(result)
	return result
}

func chooseSmartRoute(input map[string]any) smartRouteDecision {
	text, fileCount := smartRoutingInput(input["input"])
	lower := strings.ToLower(text)
	decision := smartRouteDecision{
		Model: smartRouteGrok, Reason: "general", TextChars: utf8.RuneCountInString(text), FileCount: fileCount,
	}

	if containsRoutingTerm(lower, []string{
		"contract", "agreement", "legal", "law", "regulatory", "compliance", "financial statement",
		"audit", "due diligence", "patent", "conflict clause", "合同", "协议", "法律", "法规", "合规",
		"财务报表", "审计", "尽调", "专利", "风险条款", "矛盾条款",
	}) {
		decision.Model, decision.Reason = smartRouteSol, "high_risk_document"
		return decision
	}
	if containsRoutingTerm(lower, []string{
		"extract", "classify", "categorize", "csv", "json", "table", "schema", "field mapping", "batch",
		"批量", "提取", "分类", "字段", "表格", "格式转换", "结构化", "清单", "去重",
	}) {
		decision.Model, decision.Reason = smartRouteLuna, "structured_batch"
		return decision
	}
	if decision.TextChars >= 700000 || fileCount >= 6 || containsRoutingTerm(lower, []string{
		"all files", "entire folder", "directory", "multiple documents", "cross-document", "long document",
		"全部文件", "整个文件夹", "文件夹", "多份文档", "跨文档", "交叉分析", "长文档", "综合多份",
	}) {
		decision.Model, decision.Reason = smartRouteTerra, "large_multi_document"
		return decision
	}
	return decision
}

func smartRoutingInput(value any) (string, int) {
	parts := make([]string, 0, 8)
	files := 0
	collectSmartRoutingInput(value, "user", &parts, &files)
	return strings.Join(parts, "\n"), files
}

func collectSmartRoutingInput(value any, inheritedRole string, parts *[]string, files *int) {
	switch item := value.(type) {
	case string:
		if inheritedRole == "" || inheritedRole == "user" {
			*parts = append(*parts, item)
		}
	case []any:
		for _, child := range item {
			collectSmartRoutingInput(child, inheritedRole, parts, files)
		}
	case map[string]any:
		role := strings.ToLower(strings.TrimSpace(stringValue(item["role"])))
		if role == "" {
			role = inheritedRole
		}
		typeName := strings.ToLower(strings.TrimSpace(stringValue(item["type"])))
		switch typeName {
		case "input_file", "file", "file_attachment", "attachment":
			(*files)++
		}
		if role != "" && role != "user" {
			return
		}
		if text := strings.TrimSpace(stringValue(item["text"])); text != "" {
			*parts = append(*parts, text)
		}
		if content, ok := item["content"]; ok {
			collectSmartRoutingInput(content, role, parts, files)
		}
	}
}

func containsRoutingTerm(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}
