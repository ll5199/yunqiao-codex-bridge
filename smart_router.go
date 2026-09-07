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
		Model: smartRouteGrok, Reason: "routine_bid_work", TextChars: utf8.RuneCountInString(text), FileCount: fileCount,
	}

	// Tender work is dominated by inexpensive tool operations. Keep explicit
	// find/copy/paste/replace/format requests on Grok even when the target file
	// happens to be a technical proposal or the operation is described as batch.
	if isRoutineBidFileOperation(lower) {
		decision.Reason = "routine_file_operation"
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
	// Final compliance and disqualification review is performed by a person.
	// Sol is therefore reserved for difficult drafting and substantive rewrites,
	// not merely because a prompt mentions a tender, contract, law or risk.
	if isComplexBidDrafting(lower) {
		decision.Model, decision.Reason = smartRouteSol, "complex_bid_drafting"
		return decision
	}
	return decision
}

func isRoutineBidFileOperation(text string) bool {
	return containsRoutingTerm(text, []string{
		"find and replace", "copy and paste", "copy/paste", "replace company name", "replace date",
		"rename file", "find file", "search files", "locate file", "open file", "format document",
		"复制粘贴", "复制并粘贴", "查找替换", "查找并替换", "批量替换", "全局替换",
		"替换公司名称", "修改公司名称", "替换日期", "修改日期", "修改页码", "调整格式",
		"统一格式", "套用格式", "整理目录", "更新目录", "重命名文件", "查找文件",
		"搜索文件", "定位文件", "打开文件",
	})
}

func isComplexBidDrafting(text string) bool {
	if containsRoutingTerm(text, []string{
		"complex rewrite", "substantive rewrite", "scoring point response", "point-by-point response",
		"复杂改写", "深度重写", "评分点响应", "逐条响应",
	}) {
		return true
	}
	draftingAction := containsRoutingTerm(text, []string{
		"draft", "write", "rewrite", "expand", "polish", "optimize", "create",
		"撰写", "编写", "起草", "重写", "改写", "扩写", "润色", "优化", "生成",
		"制定", "完善", "制作", "做一份",
	})
	draftingSubject := containsRoutingTerm(text, []string{
		"construction organization design", "technical proposal", "method statement", "implementation plan",
		"technical response", "project execution plan", "quality assurance plan", "safety plan",
		"emergency response plan", "scoring criteria", "evaluation criteria", "technical section", "response text",
		"施工组织设计", "技术方案", "施工方案", "实施方案", "项目实施方案", "技术标",
		"技术章节", "质量保证措施", "质量保障措施", "安全保证措施", "安全保障措施",
		"安全文明施工", "环境保护措施", "环保措施", "应急预案", "应急保障措施",
		"项目重点", "项目难点", "技术难点", "评分标准", "评分办法", "评分细则",
		"响应内容", "技术内容", "核心章节", "核心段落",
	})
	return draftingAction && draftingSubject
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
