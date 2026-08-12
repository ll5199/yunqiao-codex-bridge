package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const accountAPIBase = "https://api.velyn65.com/api/v1"

type accountUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type accountGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type accountKey struct {
	Key     string        `json:"key"`
	Status  string        `json:"status"`
	GroupID *int64        `json:"group_id"`
	Group   *accountGroup `json:"group"`
}

type accountSubscription struct {
	GroupID   int64         `json:"group_id"`
	Status    string        `json:"status"`
	ExpiresAt *string       `json:"expires_at"`
	Group     *accountGroup `json:"group"`
}

type usageWindow struct {
	Used       float64  `json:"used"`
	Limit      *float64 `json:"limit"`
	Percentage float64  `json:"percentage"`
}

// UnmarshalJSON accepts both Sub2API response formats. Older releases expose
// used/limit, while current releases expose used_usd/limit_usd.
func (window *usageWindow) UnmarshalJSON(data []byte) error {
	var value struct {
		Used       *float64 `json:"used"`
		Limit      *float64 `json:"limit"`
		UsedUSD    *float64 `json:"used_usd"`
		LimitUSD   *float64 `json:"limit_usd"`
		Percentage float64  `json:"percentage"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	window.Percentage = value.Percentage
	if value.UsedUSD != nil {
		window.Used = *value.UsedUSD
	} else if value.Used != nil {
		window.Used = *value.Used
	}
	if value.LimitUSD != nil {
		window.Limit = value.LimitUSD
	} else {
		window.Limit = value.Limit
	}
	return nil
}

type subscriptionProgress struct {
	Daily   *usageWindow `json:"daily"`
	Weekly  *usageWindow `json:"weekly"`
	Monthly *usageWindow `json:"monthly"`
}

type progressItem struct {
	Subscription *accountSubscription `json:"subscription"`
	Progress     subscriptionProgress `json:"progress"`
}

type accountEntitlements struct {
	User          accountUser
	Subscriptions []accountSubscription
	Keys          []accountKey
	Progress      []progressItem
	ProviderKeys  map[string]string
}

type accountClient struct {
	client *http.Client
	token  string
}

func newAccountClient() *accountClient {
	return &accountClient{client: &http.Client{Timeout: 25 * time.Second}}
}

func (client *accountClient) login(email, password string) (accountUser, error) {
	payload := map[string]string{"email": strings.TrimSpace(email), "password": password}
	var result struct {
		AccessToken string      `json:"access_token"`
		Requires2FA bool        `json:"requires_2fa"`
		User        accountUser `json:"user"`
	}
	if err := client.request(http.MethodPost, "/auth/login", payload, &result); err != nil {
		return accountUser{}, err
	}
	if result.Requires2FA {
		return accountUser{}, errors.New("此账号启用了两步验证，请暂时使用外部 API 模式")
	}
	if result.AccessToken == "" {
		return accountUser{}, errors.New("登录成功，但服务器没有返回登录令牌")
	}
	client.token = result.AccessToken
	return result.User, nil
}

func (client *accountClient) entitlements(user accountUser) (accountEntitlements, error) {
	result := accountEntitlements{User: user, ProviderKeys: make(map[string]string)}
	if err := client.request(http.MethodGet, "/subscriptions/active", nil, &result.Subscriptions); err != nil {
		return result, err
	}
	var keyPage struct {
		Items []accountKey `json:"items"`
	}
	if err := client.request(http.MethodGet, "/keys?page=1&page_size=100", nil, &keyPage); err != nil {
		return result, err
	}
	result.Keys = keyPage.Items
	// Usage is useful but must not make a successful login fail.
	_ = client.request(http.MethodGet, "/subscriptions/progress", nil, &result.Progress)
	activeGroups := make(map[int64]bool)
	groupsByID := make(map[int64]*accountGroup)
	for _, subscription := range result.Subscriptions {
		if strings.EqualFold(subscription.Status, "active") {
			activeGroups[subscription.GroupID] = true
			if subscription.Group != nil {
				groupsByID[subscription.GroupID] = subscription.Group
			}
		}
	}
	result.ProviderKeys = providerKeysForEntitlements(result.Keys, activeGroups, groupsByID)
	if len(result.ProviderKeys) == 0 {
		return result, errors.New("账号已登录，但有效订阅分组下没有可用 Key")
	}
	return result, nil
}

func providerKeysForEntitlements(keys []accountKey, activeGroups map[int64]bool, groupsByID map[int64]*accountGroup) map[string]string {
	providerKeys := make(map[string]string)
	for _, key := range keys {
		if !strings.EqualFold(key.Status, "active") || strings.TrimSpace(key.Key) == "" {
			continue
		}
		group := key.Group
		groupID := int64(0)
		if key.GroupID != nil {
			groupID = *key.GroupID
		}
		if group == nil && groupID != 0 {
			group = groupsByID[groupID]
		}
		if group == nil {
			continue
		}
		if groupID == 0 {
			groupID = group.ID
		}
		if len(activeGroups) > 0 && !activeGroups[groupID] {
			continue
		}
		provider := normalizeProvider(group.Platform)
		if provider != "" {
			if _, exists := providerKeys[provider]; !exists {
				providerKeys[provider] = strings.TrimSpace(key.Key)
			}
		}
	}
	return providerKeys
}

func (client *accountClient) request(method, path string, payload any, destination any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, accountAPIBase+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("连接云桥账号服务失败：%w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return err
	}
	var envelope struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("云桥账号服务返回了无效数据（HTTP %d）", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = fmt.Sprintf("HTTP %d", response.StatusCode)
		}
		return errors.New(message)
	}
	if destination != nil && len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		if err := json.Unmarshal(envelope.Data, destination); err != nil {
			return fmt.Errorf("无法解析云桥账号数据：%w", err)
		}
	}
	return nil
}

func normalizeProvider(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "openai":
		return "ChatGPT"
	case "gemini", "antigravity":
		return "Gemini"
	case "grok":
		return "Grok"
	default:
		return ""
	}
}

func membershipText(entitlements accountEntitlements) string {
	identity := strings.TrimSpace(entitlements.User.Email)
	if identity == "" {
		identity = strings.TrimSpace(entitlements.User.Username)
	}
	groups := make([]string, 0, len(entitlements.Subscriptions))
	for _, subscription := range entitlements.Subscriptions {
		if subscription.Group != nil && subscription.Group.Name != "" {
			groups = append(groups, subscription.Group.Name)
		}
	}
	if len(groups) == 0 {
		return identity
	}
	return identity + "  ·  " + strings.Join(groups, "、")
}

func usageLines(progress []progressItem) [3]string {
	var daily, weekly, monthly *usageWindow
	for _, item := range progress {
		if item.Progress.Daily != nil {
			daily = addUsage(daily, item.Progress.Daily)
		}
		if item.Progress.Weekly != nil {
			weekly = addUsage(weekly, item.Progress.Weekly)
		}
		if item.Progress.Monthly != nil {
			monthly = addUsage(monthly, item.Progress.Monthly)
		}
	}
	return [3]string{formatUsage("每日", daily), formatUsage("每周", weekly), formatUsage("每月", monthly)}
}

func addUsage(total, value *usageWindow) *usageWindow {
	if total == nil {
		copy := *value
		return &copy
	}
	total.Used += value.Used
	if value.Limit != nil {
		if total.Limit == nil {
			zero := 0.0
			total.Limit = &zero
		}
		*total.Limit += *value.Limit
	}
	if total.Limit != nil && *total.Limit > 0 {
		total.Percentage = total.Used / *total.Limit * 100
	}
	return total
}

func formatUsage(period string, value *usageWindow) string {
	if value == nil {
		return period + "用量    暂无数据"
	}
	if value.Limit == nil || *value.Limit <= 0 {
		return fmt.Sprintf("%s用量    已用 $%.2f    未设置额度", period, value.Used)
	}
	return fmt.Sprintf("%s用量    $%.2f / $%.2f    %.0f%%", period, value.Used, *value.Limit, value.Percentage)
}
