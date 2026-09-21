package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type dynamicSmartRouter struct {
	manager   *routingPolicyManager
	client    *http.Client
	baseURL   string
	apiKey    string
	available map[string]bool
	logger    func(string, string)
}

type classifierResult struct {
	TaskType   string  `json:"task_type"`
	Model      string  `json:"model"`
	Effort     string  `json:"effort"`
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

func newDynamicSmartRouter(manager *routingPolicyManager, rawTarget, apiKey string, logger func(string, string), availableModels ...[]string) *dynamicSmartRouter {
	router := &dynamicSmartRouter{
		manager: manager,
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: strings.TrimRight(strings.TrimSpace(rawTarget), "/"),
		apiKey:  strings.TrimSpace(apiKey),
		logger:  logger,
	}
	if len(availableModels) > 0 {
		router.available = make(map[string]bool)
		for _, model := range availableModels[0] {
			model = strings.TrimSpace(model)
			if isSmartRouterTarget(model) {
				router.available[model] = true
			}
		}
	}
	return router
}

func (router *dynamicSmartRouter) choose(ctx context.Context, input map[string]any) smartRouteDecision {
	policy := defaultRoutingPolicy()
	if router != nil && router.manager != nil {
		policy = router.manager.snapshot()
	}
	decision, matches := chooseSmartRouteWithPolicy(input, policy)
	decision, matches = router.filterUnavailableDecision(decision, matches, policy)
	text, fileCount := smartRoutingInput(input["input"])
	if strings.TrimSpace(text) == "" && fileCount == 0 {
		return decision
	}
	if len(matches) == 1 || router == nil || !policy.Classifier.Enabled {
		return decision
	}

	result, err := router.classify(ctx, input, policy, matches)
	if err != nil {
		router.log("router.classifier_failed", "policy="+safeLogID(policy.PolicyVersion)+" error="+err.Error())
		if len(matches) == 0 && policy.Distribution.Enabled {
			return weightedRoutingDecision(input, policy, decision)
		}
		return decision
	}
	if result.Confidence < policy.Classifier.ConfidenceThreshold {
		if policy.Distribution.Enabled {
			fallback := weightedRoutingDecision(input, policy, decision)
			fallback.Reason = "ambiguous_distribution"
			return fallback
		}
		decision.Model = policy.Classifier.LowConfidence.Model
		decision.Effort = policy.Classifier.LowConfidence.Effort
		decision.Reason = "classifier_low_confidence"
		decision, _ = router.filterUnavailableDecision(decision, nil, policy)
		return decision
	}
	if router.hasAvailabilityList() && (!router.available[result.Model] || !policy.ModelSettings[result.Model].Enabled && policy.ModelSettings[result.Model].ModelIsConfigured()) {
		router.log("router.classifier_unavailable", "model="+safeLogID(result.Model))
		return decision
	}
	decision.Model = result.Model
	decision.Effort = result.Effort
	decision.Reason = "classifier"
	router.log("router.classifier_selected", fmt.Sprintf(
		"policy=%s model=%s effort=%s confidence=%.2f task_type=%s matches=%d",
		safeLogID(policy.PolicyVersion), safeLogID(result.Model), result.Effort,
		result.Confidence, safeClassifierLabel(result.TaskType), len(matches),
	))
	return decision
}

func (settings routingModelSettings) ModelIsConfigured() bool {
	return settings.Type != "" || settings.Effort != ""
}

func (router *dynamicSmartRouter) hasAvailabilityList() bool {
	return router != nil && router.available != nil
}

func (router *dynamicSmartRouter) modelAvailable(model string, policy routingPolicy) bool {
	if !router.hasAvailabilityList() {
		return true
	}
	if !router.available[model] {
		return false
	}
	if settings, ok := policy.ModelSettings[model]; ok && settings.ModelIsConfigured() && !settings.Enabled {
		return false
	}
	return true
}

func (router *dynamicSmartRouter) filterUnavailableDecision(decision smartRouteDecision, matches []routingRule, policy routingPolicy) (smartRouteDecision, []routingRule) {
	if !router.hasAvailabilityList() {
		return decision, matches
	}
	availableMatches := make([]routingRule, 0, len(matches))
	for _, rule := range matches {
		if router.modelAvailable(rule.Model, policy) {
			availableMatches = append(availableMatches, rule)
		}
	}
	if router.modelAvailable(decision.Model, policy) {
		return decision, availableMatches
	}
	if len(availableMatches) > 0 {
		selected := availableMatches[0]
		decision.Model, decision.Effort, decision.Reason = selected.Model, selected.Effort, selected.Reason
		return decision, availableMatches
	}
	for _, model := range smartRouterTargets {
		if router.modelAvailable(model, policy) {
			decision.Model = model
			decision.Effort = effortForModel(policy, model)
			decision.Reason = "available_model_fallback"
			return decision, availableMatches
		}
	}
	return decision, availableMatches
}

func (router *dynamicSmartRouter) availableModelNames() []string {
	if !router.hasAvailabilityList() {
		return append([]string(nil), smartRouterTargets...)
	}
	result := make([]string, 0, len(smartRouterTargets))
	for _, model := range smartRouterTargets {
		if router.available[model] {
			result = append(result, model)
		}
	}
	return result
}

func (router *dynamicSmartRouter) classify(parent context.Context, input map[string]any, policy routingPolicy, matches []routingRule) (classifierResult, error) {
	if router.baseURL == "" || router.apiKey == "" {
		return classifierResult{}, errors.New("分类器缺少 API 地址或 Key")
	}
	text, fileCount := smartRoutingInput(input["input"])
	text = truncateRunes(strings.TrimSpace(text), policy.Classifier.MaxInputChars)
	matchNames := make([]string, 0, len(matches))
	for _, rule := range matches {
		matchNames = append(matchNames, rule.Reason)
	}
	weights := make([]string, 0, len(policy.Distribution.Weights))
	for model, weight := range policy.Distribution.Weights {
		weights = append(weights, fmt.Sprintf("%s=%d", model, weight))
	}
	sort.Strings(weights)
	requestText := fmt.Sprintf(
		"用户任务：\n%s\n\n元数据：文字字符数=%d，附件数=%d，规则候选=%s，可用模型=%s，业务倾向权重=%s",
		text, len([]rune(text)), fileCount, strings.Join(matchNames, ","), strings.Join(router.availableModelNames(), ","), strings.Join(weights, ","),
	)
	instructions := "你是云桥 Codex 的任务路由器。只判断应由哪个模型执行，不执行任务本身。" +
		"可选模型：grok-4.6 负责查找、修改、复制粘贴、上传和普通操作；gpt-5.6-luna 负责字段提取、表格、清单和结构化；" +
		"gpt-5.6-terra 负责大量文件、OCR、跨文档和长上下文；gpt-5.6-sol 负责施工组织设计、复杂技术方案和深度重写。" +
		"只能从可用模型中选择；缺少某个模型时不要返回它。权重只在多个模型同样适合时作为偏好，不能让不适合的模型承担任务。" +
		"仅返回 JSON：{\"task_type\":\"简短英文分类\",\"model\":\"模型名\",\"effort\":\"low|medium|high\",\"confidence\":0到1,\"reason\":\"简短中文原因\"}。"
	payload, err := json.Marshal(map[string]any{
		"model":             policy.Classifier.Model,
		"reasoning":         map[string]any{"effort": policy.Classifier.Effort},
		"instructions":      instructions,
		"input":             requestText,
		"max_output_tokens": 220,
		"stream":            false,
		"store":             false,
	})
	if err != nil {
		return classifierResult{}, err
	}
	timeout := time.Duration(policy.Classifier.TimeoutSeconds) * time.Second
	if parent == nil {
		parent = context.Background()
	}
	contextValue, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	request, err := http.NewRequestWithContext(contextValue, http.MethodPost, router.baseURL+"/responses", bytes.NewReader(payload))
	if err != nil {
		return classifierResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+router.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion+" Router")
	response, err := router.client.Do(request)
	if err != nil {
		return classifierResult{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return classifierResult{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return classifierResult{}, fmt.Errorf("分类器返回 HTTP %d：%s", response.StatusCode, safeLogBody(body))
	}
	output := extractModelOutputText(body)
	result, err := parseClassifierResult(output)
	if err != nil {
		return classifierResult{}, err
	}
	if !isSmartRouterTarget(result.Model) {
		return classifierResult{}, fmt.Errorf("分类器返回了不允许的模型 %q", result.Model)
	}
	target := routingTarget{Model: result.Model, Effort: result.Effort}
	if err := validateRoutingTarget("classifier.result", &target); err != nil {
		return classifierResult{}, err
	}
	result.Model, result.Effort = target.Model, target.Effort
	if result.Confidence < 0 || result.Confidence > 1 {
		return classifierResult{}, errors.New("分类器 confidence 不在 0 到 1 之间")
	}
	return result, nil
}

func extractModelOutputText(body []byte) string {
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return ""
	}
	if text := strings.TrimSpace(stringValue(root["output_text"])); text != "" {
		return text
	}
	var parts []string
	var walk func(any, int)
	walk = func(value any, depth int) {
		if depth > 8 || value == nil {
			return
		}
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				walk(child, depth+1)
			}
		case map[string]any:
			typeName := strings.ToLower(stringValue(item["type"]))
			if typeName == "output_text" || typeName == "text" {
				if text := strings.TrimSpace(stringValue(item["text"])); text != "" {
					parts = append(parts, text)
				}
			}
			// Accept the Chat Completions shape as a compatibility fallback.
			if message, ok := item["message"].(map[string]any); ok {
				if text := strings.TrimSpace(stringValue(message["content"])); text != "" {
					parts = append(parts, text)
				}
			}
			for _, key := range []string{"output", "content", "message", "choices"} {
				walk(item[key], depth+1)
			}
		}
	}
	walk(root, 0)
	return strings.Join(parts, "\n")
}

func parseClassifierResult(text string) (classifierResult, error) {
	text = strings.TrimSpace(text)
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return classifierResult{}, errors.New("分类器没有返回 JSON")
	}
	decoder := json.NewDecoder(strings.NewReader(text[start : end+1]))
	decoder.DisallowUnknownFields()
	var result classifierResult
	if err := decoder.Decode(&result); err != nil {
		return classifierResult{}, fmt.Errorf("分类器 JSON 无效：%w", err)
	}
	result.TaskType = safeClassifierLabel(result.TaskType)
	result.Model = strings.TrimSpace(result.Model)
	result.Effort = strings.ToLower(strings.TrimSpace(result.Effort))
	result.Reason = strings.TrimSpace(result.Reason)
	return result, nil
}

func safeClassifierLabel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var result strings.Builder
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '_' || character == '-' {
			result.WriteRune(character)
		}
		if result.Len() >= 40 {
			break
		}
	}
	if result.Len() == 0 {
		return "unknown"
	}
	return result.String()
}

func weightedRoutingDecision(input map[string]any, policy routingPolicy, fallback smartRouteDecision) smartRouteDecision {
	total := 0
	for _, model := range smartRouterTargets {
		if weight := policy.Distribution.Weights[model]; weight > 0 {
			total += weight
		}
	}
	if total == 0 {
		return fallback
	}
	text, _ := smartRoutingInput(input["input"])
	key := strings.TrimSpace(stringValue(input["prompt_cache_key"]))
	if key == "" {
		key = text
	}
	sum := sha256.Sum256([]byte(key))
	bucket := int(binary.BigEndian.Uint64(sum[:8]) % uint64(total))
	for _, model := range smartRouterTargets {
		weight := policy.Distribution.Weights[model]
		if weight <= 0 {
			continue
		}
		if bucket < weight {
			fallback.Model = model
			fallback.Effort = effortForModel(policy, model)
			fallback.Reason = "ambiguous_distribution"
			return fallback
		}
		bucket -= weight
	}
	return fallback
}

func effortForModel(policy routingPolicy, model string) string {
	if settings, ok := policy.ModelSettings[model]; ok && settings.Effort != "" {
		return settings.Effort
	}
	for _, rule := range policy.Rules {
		if rule.Model == model {
			return rule.Effort
		}
	}
	if policy.Default.Model == model {
		return policy.Default.Effort
	}
	switch model {
	case smartRouteGrok:
		return "low"
	case smartRouteSol:
		return "high"
	default:
		return "medium"
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func (router *dynamicSmartRouter) log(event, detail string) {
	if router != nil && router.logger != nil {
		router.logger(event, detail)
	}
}
