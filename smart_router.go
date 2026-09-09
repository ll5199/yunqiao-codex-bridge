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
	Effort    string
	Reason    string
	TextChars int
	FileCount int
}

type smartRouteMemory struct {
	mu     sync.Mutex
	routes map[string]smartRouteDecision
	order  []string
	limit  int
}

func newSmartRouteMemory(limit int) *smartRouteMemory {
	if limit < 1 {
		limit = 1
	}
	return &smartRouteMemory{routes: make(map[string]smartRouteDecision), limit: limit}
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
		if previous, exists := memory.routes[cacheKey]; exists && previous.Model != "" {
			decision.Model, decision.Effort, decision.Reason = previous.Model, previous.Effort, "session_continuation"
		}
		return decision
	}
	if _, exists := memory.routes[cacheKey]; !exists {
		memory.order = append(memory.order, cacheKey)
	}
	memory.routes[cacheKey] = decision
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
	decision, _ := chooseSmartRouteWithPolicy(input, defaultRoutingPolicy())
	return decision
}

func chooseSmartRouteWithPolicy(input map[string]any, policy routingPolicy) (smartRouteDecision, []routingRule) {
	text, fileCount := smartRoutingInput(input["input"])
	lower := strings.ToLower(text)
	decision := smartRouteDecision{
		Model: policy.Default.Model, Effort: policy.Default.Effort, Reason: "policy_default",
		TextChars: utf8.RuneCountInString(text), FileCount: fileCount,
	}
	matches := matchingRoutingRules(policy.Rules, lower, decision.TextChars, fileCount)
	if len(matches) > 0 {
		decision.Model = matches[0].Model
		decision.Effort = matches[0].Effort
		decision.Reason = matches[0].Reason
	}
	return decision, matches
}

func matchingRoutingRules(rules []routingRule, text string, textChars, fileCount int) []routingRule {
	matches := make([]routingRule, 0, 4)
	for _, rule := range rules {
		if routingRuleMatches(rule, text, textChars, fileCount) {
			matches = append(matches, rule)
		}
	}
	sort.SliceStable(matches, func(left, right int) bool { return matches[left].Priority > matches[right].Priority })
	return matches
}

func routingRuleMatches(rule routingRule, text string, textChars, fileCount int) bool {
	if rule.MinTextChars > 0 && textChars >= rule.MinTextChars {
		return true
	}
	if rule.MinFileCount > 0 && fileCount >= rule.MinFileCount {
		return true
	}
	if len(rule.MatchAny) > 0 && containsRoutingTerm(text, rule.MatchAny) {
		return true
	}
	if len(rule.MatchAllGroups) > 0 {
		for _, group := range rule.MatchAllGroups {
			if len(group) == 0 || !containsRoutingTerm(text, group) {
				return false
			}
		}
		return true
	}
	return false
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
