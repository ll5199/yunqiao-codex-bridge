package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// keyUsage is the balance and spend of one API key, read from the relay's
// key-authenticated GET /v1/usage endpoint.
type keyUsage struct {
	HasBalance    bool            `json:"hasBalance"`
	Balance       float64         `json:"balance"`
	Unit          string          `json:"unit"`
	Plan          string          `json:"plan"`
	TodayCost     float64         `json:"todayCost"`
	TodayRequests int64           `json:"todayRequests"`
	TotalCost     float64         `json:"totalCost"`
	TotalRequests int64           `json:"totalRequests"`
	TotalTokens   int64           `json:"totalTokens"`
	Daily         []keyUsageDay   `json:"daily"`
	Models        []keyUsageModel `json:"models"`
	UpdatedAt     int64           `json:"updatedAt"`
}

type keyUsageDay struct {
	Date     string  `json:"date"`
	Requests int64   `json:"requests"`
	Cost     float64 `json:"cost"`
}

type keyUsageModel struct {
	Model    string  `json:"model"`
	Requests int64   `json:"requests"`
	Cost     float64 `json:"cost"`
}

type usageCounter struct {
	Requests    int64    `json:"requests"`
	TotalTokens int64    `json:"total_tokens"`
	Cost        float64  `json:"cost"`
	ActualCost  *float64 `json:"actual_cost"`
}

func (counter usageCounter) spent() float64 {
	if counter.ActualCost != nil {
		return *counter.ActualCost
	}
	return counter.Cost
}

func parseKeyUsage(body []byte) (keyUsage, error) {
	var raw struct {
		IsValid   *bool    `json:"isValid"`
		Balance   *float64 `json:"balance"`
		Remaining *float64 `json:"remaining"`
		Unit      string   `json:"unit"`
		PlanName  string   `json:"planName"`
		Usage     struct {
			Today usageCounter `json:"today"`
			Total usageCounter `json:"total"`
		} `json:"usage"`
		Daily []struct {
			Date string `json:"date"`
			usageCounter
		} `json:"daily_usage"`
		Models []struct {
			Model string `json:"model"`
			usageCounter
		} `json:"model_stats"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return keyUsage{}, errors.New("用量接口返回的不是有效 JSON")
	}
	if raw.IsValid != nil && !*raw.IsValid {
		return keyUsage{}, errors.New("API Key 无效或已停用")
	}
	usage := keyUsage{
		Unit: strings.TrimSpace(raw.Unit), Plan: strings.TrimSpace(raw.PlanName),
		TodayCost: raw.Usage.Today.spent(), TodayRequests: raw.Usage.Today.Requests,
		TotalCost: raw.Usage.Total.spent(), TotalRequests: raw.Usage.Total.Requests, TotalTokens: raw.Usage.Total.TotalTokens,
		UpdatedAt: time.Now().UnixMilli(),
	}
	if usage.Unit == "" {
		usage.Unit = "USD"
	}
	if raw.Remaining != nil {
		usage.HasBalance, usage.Balance = true, *raw.Remaining
	} else if raw.Balance != nil {
		usage.HasBalance, usage.Balance = true, *raw.Balance
	}
	for _, day := range raw.Daily {
		usage.Daily = append(usage.Daily, keyUsageDay{Date: day.Date, Requests: day.Requests, Cost: day.spent()})
	}
	sort.Slice(usage.Daily, func(i, j int) bool { return usage.Daily[i].Date < usage.Daily[j].Date })
	for _, model := range raw.Models {
		usage.Models = append(usage.Models, keyUsageModel{Model: model.Model, Requests: model.Requests, Cost: model.spent()})
	}
	sort.SliceStable(usage.Models, func(i, j int) bool { return usage.Models[i].Cost > usage.Models[j].Cost })
	return usage, nil
}

// fetchKeyUsage reads <baseURL>/usage with the API key, e.g.
// https://api.velyn65.com/v1/usage.
func fetchKeyUsage(baseURL, apiKey string) (keyUsage, error) {
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return keyUsage{}, err
	}
	if strings.TrimSpace(apiKey) == "" {
		return keyUsage{}, errors.New("请输入 API Key")
	}
	request, err := http.NewRequest(http.MethodGet, baseURL+"/usage", nil)
	if err != nil {
		return keyUsage{}, err
	}
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return keyUsage{}, fmt.Errorf("连接用量接口失败：%w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return keyUsage{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return keyUsage{}, fmt.Errorf("用量接口返回 HTTP %d", response.StatusCode)
	}
	return parseKeyUsage(body)
}
