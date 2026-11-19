package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const bridgeModelCatalogName = "yunqiao-model-catalog.json"

type catalogReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type bridgeCatalogModel struct {
	Slug                       string                  `json:"slug"`
	DisplayName                string                  `json:"display_name"`
	Description                string                  `json:"description"`
	BaseInstructions           string                  `json:"base_instructions"`
	DefaultReasoningLevel      string                  `json:"default_reasoning_level"`
	SupportedReasoningLevels   []catalogReasoningLevel `json:"supported_reasoning_levels"`
	ShellType                  string                  `json:"shell_type"`
	Visibility                 string                  `json:"visibility"`
	SupportedInAPI             bool                    `json:"supported_in_api"`
	Priority                   int                     `json:"priority"`
	SupportVerbosity           bool                    `json:"support_verbosity"`
	TruncationPolicy           map[string]any          `json:"truncation_policy"`
	ExperimentalSupportedTools []string                `json:"experimental_supported_tools"`
}

func buildBridgeModelCatalog(models []string, selected string) ([]byte, error) {
	models = withSmartRouterModel(models)
	if !containsString(models, selected) {
		return nil, errors.New("当前模型不在已同步的列表中，请重新同步模型")
	}
	entries := make([]bridgeCatalogModel, 0, len(models))
	for _, slug := range models {
		name, description := slug, "云桥 API"
		if slug == smartRouterModel {
			name, description = "智能路由（Auto）", "根据任务自动选择可用模型"
		}
		priority := len(entries) + 2
		if slug == selected {
			priority = 0
		} else if slug == smartRouterModel {
			priority = 1
		}
		entries = append(entries, bridgeCatalogModel{
			Slug: slug, DisplayName: name, Description: description,
			BaseInstructions:         "You are Codex, a coding assistant. Help the user complete their tasks. Follow project instructions, inspect relevant context, and verify your work when possible.",
			DefaultReasoningLevel:    "medium",
			SupportedReasoningLevels: []catalogReasoningLevel{{"low", "Low"}, {"medium", "Medium"}, {"high", "High"}},
			ShellType:                "shell_command", Visibility: "list", SupportedInAPI: true,
			Priority: priority, TruncationPolicy: map[string]any{"mode": "tokens", "limit": 10000},
			ExperimentalSupportedTools: []string{},
		})
	}
	return json.MarshalIndent(struct {
		Models []bridgeCatalogModel `json:"models"`
	}{entries}, "", "  ")
}

func checkBridgeCatalogConflict(existing string) error {
	var config map[string]any
	if err := toml.Unmarshal([]byte(removeYunqiaoCodexConfig(existing)), &config); err != nil {
		return fmt.Errorf("读取现有 Codex 配置失败，未覆盖原文件：%w", err)
	}
	if _, exists := config["model_catalog_json"]; exists {
		return errors.New("Codex 已配置自定义 model_catalog_json。请先备份并移除此设置，再使用云桥同步模型")
	}
	return nil
}

func writeBridgeModelCatalog(codexHome string, models []string, selected string) (string, error) {
	body, err := buildBridgeModelCatalog(models, selected)
	if err != nil {
		return "", err
	}
	path := filepath.Join(codexHome, bridgeModelCatalogName)
	f, err := os.CreateTemp(codexHome, ".yunqiao-models-*.json")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(append(body, '\n'))
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return "", fmt.Errorf("替换 Codex 模型目录失败：%w", err)
	}
	return path, nil
}
