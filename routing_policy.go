package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const routingPolicyURL = "https://index.velyn65.com/download/routing-policy.json"

type routingTarget struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

type routingRule struct {
	Reason         string     `json:"reason"`
	Model          string     `json:"model"`
	Effort         string     `json:"effort"`
	Priority       int        `json:"priority"`
	MatchAny       []string   `json:"match_any,omitempty"`
	MatchAllGroups [][]string `json:"match_all_groups,omitempty"`
	MinTextChars   int        `json:"min_text_chars,omitempty"`
	MinFileCount   int        `json:"min_file_count,omitempty"`
}

type routingClassifierPolicy struct {
	Enabled             bool          `json:"enabled"`
	Model               string        `json:"model"`
	Effort              string        `json:"effort"`
	ConfidenceThreshold float64       `json:"confidence_threshold"`
	TimeoutSeconds      int           `json:"timeout_seconds"`
	MaxInputChars       int           `json:"max_input_chars"`
	LowConfidence       routingTarget `json:"low_confidence"`
}

type routingDistributionPolicy struct {
	Enabled bool           `json:"enabled"`
	Weights map[string]int `json:"weights"`
}

type routingPolicy struct {
	SchemaVersion  int                       `json:"schema_version"`
	PolicyVersion  string                    `json:"policy_version"`
	RefreshSeconds int                       `json:"refresh_seconds"`
	Default        routingTarget             `json:"default"`
	Rules          []routingRule             `json:"rules"`
	Classifier     routingClassifierPolicy   `json:"classifier"`
	Distribution   routingDistributionPolicy `json:"ambiguous_distribution"`
}

func defaultRoutingPolicy() routingPolicy {
	return routingPolicy{
		SchemaVersion: 1, PolicyVersion: "builtin-1.5.0", RefreshSeconds: 300,
		Default: routingTarget{Model: smartRouteGrok, Effort: "low"},
		Rules: []routingRule{
			{
				Reason: "routine_file_operation", Model: smartRouteGrok, Effort: "low", Priority: 100,
				MatchAny: []string{
					"find and replace", "copy and paste", "copy/paste", "replace company name", "replace date",
					"rename file", "find file", "search files", "locate file", "open file", "format document", "upload files",
					"复制粘贴", "复制并粘贴", "查找替换", "查找并替换", "批量替换", "全局替换",
					"替换公司名称", "修改公司名称", "替换日期", "修改日期", "修改页码", "调整格式",
					"统一格式", "套用格式", "整理目录", "更新目录", "重命名文件", "查找文件",
					"搜索文件", "定位文件", "打开文件", "上传文件", "批量上传", "上传网站",
				},
			},
			{
				Reason: "structured_batch", Model: smartRouteLuna, Effort: "medium", Priority: 90,
				MatchAny: []string{
					"extract", "classify", "categorize", "csv", "json", "table", "schema", "field mapping", "batch", "ocr",
					"批量", "提取", "分类", "字段", "表格", "格式转换", "结构化", "清单", "去重",
					"图片识别文字", "图片转文字", "文字识别", "扫描件识别",
				},
			},
			{
				Reason: "large_multi_document", Model: smartRouteTerra, Effort: "medium", Priority: 80,
				MinTextChars: 700000, MinFileCount: 6,
				MatchAny: []string{
					"all files", "entire folder", "directory", "multiple documents", "cross-document", "long document",
					"全部文件", "整个文件夹", "文件夹", "多份文档", "跨文档", "交叉分析", "长文档", "综合多份",
				},
			},
			{
				Reason: "complex_bid_drafting", Model: smartRouteSol, Effort: "high", Priority: 70,
				MatchAny: []string{
					"complex rewrite", "substantive rewrite", "scoring point response", "point-by-point response",
					"复杂改写", "深度重写", "评分点响应", "逐条响应",
				},
				MatchAllGroups: [][]string{
					{
						"draft", "write", "rewrite", "expand", "polish", "optimize", "create",
						"撰写", "编写", "起草", "重写", "改写", "扩写", "润色", "优化", "生成", "制定", "完善", "制作", "做一份",
					},
					{
						"construction organization design", "technical proposal", "method statement", "implementation plan",
						"technical response", "project execution plan", "quality assurance plan", "safety plan",
						"emergency response plan", "scoring criteria", "evaluation criteria", "technical section", "response text",
						"施工组织设计", "技术方案", "施工方案", "实施方案", "项目实施方案", "技术标",
						"技术章节", "质量保证措施", "质量保障措施", "安全保证措施", "安全保障措施",
						"安全文明施工", "环境保护措施", "环保措施", "应急预案", "应急保障措施",
						"项目重点", "项目难点", "技术难点", "评分标准", "评分办法", "评分细则",
						"响应内容", "技术内容", "核心章节", "核心段落",
					},
				},
			},
		},
		Classifier: routingClassifierPolicy{
			Enabled: true, Model: smartRouteLuna, Effort: "low", ConfidenceThreshold: 0.75,
			TimeoutSeconds: 8, MaxInputChars: 4000,
			LowConfidence: routingTarget{Model: smartRouteTerra, Effort: "medium"},
		},
		Distribution: routingDistributionPolicy{
			Enabled: false,
			Weights: map[string]int{smartRouteGrok: 70, smartRouteTerra: 15, smartRouteLuna: 10, smartRouteSol: 5},
		},
	}
}

var routingReasonPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

func parseRoutingPolicy(body []byte) (routingPolicy, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var policy routingPolicy
	if err := decoder.Decode(&policy); err != nil {
		return routingPolicy{}, fmt.Errorf("路由策略不是有效 JSON：%w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return routingPolicy{}, errors.New("路由策略只能包含一个 JSON 对象")
		}
		return routingPolicy{}, fmt.Errorf("路由策略末尾存在无效内容：%w", err)
	}
	if err := validateRoutingPolicy(&policy); err != nil {
		return routingPolicy{}, err
	}
	return policy, nil
}

func validateRoutingPolicy(policy *routingPolicy) error {
	if policy.SchemaVersion != 1 {
		return fmt.Errorf("不支持路由策略 schema_version=%d", policy.SchemaVersion)
	}
	policy.PolicyVersion = strings.TrimSpace(policy.PolicyVersion)
	if policy.PolicyVersion == "" || len(policy.PolicyVersion) > 64 {
		return errors.New("policy_version 不能为空且不能超过 64 字节")
	}
	if policy.RefreshSeconds < 60 || policy.RefreshSeconds > 3600 {
		return errors.New("refresh_seconds 必须在 60 到 3600 之间")
	}
	if err := validateRoutingTarget("default", &policy.Default); err != nil {
		return err
	}
	if len(policy.Rules) == 0 || len(policy.Rules) > 32 {
		return errors.New("rules 数量必须在 1 到 32 之间")
	}
	seenReasons := make(map[string]bool)
	termCount := 0
	for index := range policy.Rules {
		rule := &policy.Rules[index]
		rule.Reason = strings.ToLower(strings.TrimSpace(rule.Reason))
		if !routingReasonPattern.MatchString(rule.Reason) || seenReasons[rule.Reason] {
			return fmt.Errorf("rules[%d].reason 无效或重复", index)
		}
		seenReasons[rule.Reason] = true
		target := routingTarget{Model: rule.Model, Effort: rule.Effort}
		if err := validateRoutingTarget(fmt.Sprintf("rules[%d]", index), &target); err != nil {
			return err
		}
		rule.Model, rule.Effort = target.Model, target.Effort
		if rule.Priority < -1000 || rule.Priority > 1000 || rule.MinTextChars < 0 || rule.MinFileCount < 0 {
			return fmt.Errorf("rules[%d] 的优先级或阈值无效", index)
		}
		for termIndex := range rule.MatchAny {
			rule.MatchAny[termIndex] = strings.ToLower(strings.TrimSpace(rule.MatchAny[termIndex]))
			if rule.MatchAny[termIndex] == "" || len([]rune(rule.MatchAny[termIndex])) > 80 {
				return fmt.Errorf("rules[%d].match_any 包含无效关键词", index)
			}
			termCount++
		}
		for groupIndex := range rule.MatchAllGroups {
			if len(rule.MatchAllGroups[groupIndex]) == 0 {
				return fmt.Errorf("rules[%d].match_all_groups 包含空组", index)
			}
			for termIndex := range rule.MatchAllGroups[groupIndex] {
				term := strings.ToLower(strings.TrimSpace(rule.MatchAllGroups[groupIndex][termIndex]))
				if term == "" || len([]rune(term)) > 80 {
					return fmt.Errorf("rules[%d].match_all_groups 包含无效关键词", index)
				}
				rule.MatchAllGroups[groupIndex][termIndex] = term
				termCount++
			}
		}
		if len(rule.MatchAny) == 0 && len(rule.MatchAllGroups) == 0 && rule.MinTextChars == 0 && rule.MinFileCount == 0 {
			return fmt.Errorf("rules[%d] 没有任何匹配条件", index)
		}
	}
	if termCount > 512 {
		return errors.New("路由策略关键词总数不能超过 512")
	}
	if policy.Classifier.Enabled {
		target := routingTarget{Model: policy.Classifier.Model, Effort: policy.Classifier.Effort}
		if err := validateRoutingTarget("classifier", &target); err != nil {
			return err
		}
		policy.Classifier.Model, policy.Classifier.Effort = target.Model, target.Effort
		if err := validateRoutingTarget("classifier.low_confidence", &policy.Classifier.LowConfidence); err != nil {
			return err
		}
		if policy.Classifier.ConfidenceThreshold < 0.5 || policy.Classifier.ConfidenceThreshold > 0.99 {
			return errors.New("classifier.confidence_threshold 必须在 0.5 到 0.99 之间")
		}
		if policy.Classifier.TimeoutSeconds < 1 || policy.Classifier.TimeoutSeconds > 15 {
			return errors.New("classifier.timeout_seconds 必须在 1 到 15 之间")
		}
		if policy.Classifier.MaxInputChars < 256 || policy.Classifier.MaxInputChars > 8000 {
			return errors.New("classifier.max_input_chars 必须在 256 到 8000 之间")
		}
	}
	if len(policy.Distribution.Weights) > 0 {
		total := 0
		for model, weight := range policy.Distribution.Weights {
			if !isSmartRouterTarget(model) || weight < 0 || weight > 10000 {
				return fmt.Errorf("ambiguous_distribution.weights[%q] 无效", model)
			}
			total += weight
		}
		if policy.Distribution.Enabled && total == 0 {
			return errors.New("启用 ambiguous_distribution 时权重总和必须大于 0")
		}
	}
	return nil
}

func validateRoutingTarget(name string, target *routingTarget) error {
	target.Model = strings.TrimSpace(target.Model)
	target.Effort = strings.ToLower(strings.TrimSpace(target.Effort))
	if !isSmartRouterTarget(target.Model) {
		return fmt.Errorf("%s.model 不在允许的智能路由模型中", name)
	}
	if target.Effort != "low" && target.Effort != "medium" && target.Effort != "high" && target.Effort != "xhigh" {
		return fmt.Errorf("%s.effort 必须是 low、medium、high 或 xhigh", name)
	}
	if target.Effort == "xhigh" && target.Model != smartRouteGrok {
		return fmt.Errorf("%s 只有 Grok 4.6 可以配置 xhigh", name)
	}
	return nil
}

func isSmartRouterTarget(model string) bool {
	for _, target := range smartRouterTargets {
		if model == target {
			return true
		}
	}
	return false
}

func fetchRoutingPolicy(client *http.Client, endpoint string) (routingPolicy, []byte, error) {
	request, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return routingPolicy{}, nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	response, err := client.Do(request)
	if err != nil {
		return routingPolicy{}, nil, fmt.Errorf("读取远程路由策略失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return routingPolicy{}, nil, fmt.Errorf("远程路由策略返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256<<10))
	if err != nil {
		return routingPolicy{}, nil, fmt.Errorf("读取远程路由策略失败：%w", err)
	}
	policy, err := parseRoutingPolicy(body)
	if err != nil {
		return routingPolicy{}, nil, err
	}
	return policy, body, nil
}

func routingPolicyCachePath() string {
	root := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
	if root == "" {
		root, _ = os.UserConfigDir()
	}
	if root == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "YunqiaoCodexBridge", "routing-policy.json")
}

func loadRoutingPolicyCache(path string) (routingPolicy, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return routingPolicy{}, err
	}
	return parseRoutingPolicy(body)
}

func saveRoutingPolicyCache(path string, body []byte) error {
	if _, err := parseRoutingPolicy(body); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), "routing-policy-*.json")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(bytes.TrimSpace(body), '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

type routingPolicyManager struct {
	mu        sync.RWMutex
	policy    routingPolicy
	client    *http.Client
	endpoint  string
	cachePath string
	logger    func(string, string)
}

func newRoutingPolicyManager(endpoint, cachePath string, logger func(string, string)) *routingPolicyManager {
	manager := &routingPolicyManager{
		policy: defaultRoutingPolicy(), client: &http.Client{Timeout: 4 * time.Second},
		endpoint: endpoint, cachePath: cachePath, logger: logger,
	}
	if cached, err := loadRoutingPolicyCache(cachePath); err == nil {
		manager.policy = cached
		manager.log("router.policy_cache", "version="+safeLogID(cached.PolicyVersion))
	}
	return manager
}

func (manager *routingPolicyManager) snapshot() routingPolicy {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.policy
}

func (manager *routingPolicyManager) refresh() error {
	policy, body, err := fetchRoutingPolicy(manager.client, manager.endpoint)
	if err != nil {
		manager.log("router.policy_remote_failed", err.Error())
		return err
	}
	manager.mu.Lock()
	previous := manager.policy.PolicyVersion
	manager.policy = policy
	manager.mu.Unlock()
	if manager.cachePath != "" {
		if err := saveRoutingPolicyCache(manager.cachePath, body); err != nil {
			manager.log("router.policy_cache_failed", err.Error())
		}
	}
	if previous != policy.PolicyVersion {
		manager.log("router.policy_updated", "from="+safeLogID(previous)+" to="+safeLogID(policy.PolicyVersion))
	}
	return nil
}

func (manager *routingPolicyManager) start(done <-chan struct{}) {
	go func() {
		initial := time.NewTimer(500 * time.Millisecond)
		select {
		case <-done:
			initial.Stop()
			return
		case <-initial.C:
			_ = manager.refresh()
		}
		for {
			seconds := manager.snapshot().RefreshSeconds
			if seconds < 60 {
				seconds = 300
			}
			timer := time.NewTimer(time.Duration(seconds) * time.Second)
			select {
			case <-done:
				timer.Stop()
				return
			case <-timer.C:
				_ = manager.refresh()
			}
		}
	}()
}

func (manager *routingPolicyManager) log(event, detail string) {
	if manager != nil && manager.logger != nil {
		manager.logger(event, detail)
	}
}
