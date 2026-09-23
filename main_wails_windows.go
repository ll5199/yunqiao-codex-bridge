//go:build windows

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var wailsAssets embed.FS

type BridgeApp struct {
	mu          sync.Mutex
	opMu        sync.Mutex
	ctx         context.Context
	baseURL     string
	apiKey      string
	models      []string
	model       string
	account     *accountClient
	entitlements accountEntitlements
	cloudModels map[string][]string
	status      string
}

type bridgeViewState struct {
	BaseURL        string          `json:"baseURL"`
	HasAPIKey      bool            `json:"hasAPIKey"`
	Models         []string        `json:"models"`
	SelectedModel  string          `json:"selectedModel"`
	Version        string          `json:"version"`
	Status         string          `json:"status"`
	Connection     string          `json:"connection"`
	Cloud          cloudViewState  `json:"cloud"`
}

type cloudViewState struct {
	LoggedIn  bool     `json:"loggedIn"`
	Identity  string   `json:"identity"`
	Groups    string   `json:"groups"`
	Providers []string `json:"providers"`
	Usage     []string `json:"usage"`
}

type modelFetchResult struct {
	BaseURL string   `json:"baseURL"`
	Models  []string `json:"models"`
}

type updateViewState struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes"`
}

func main() {
	if runUpdateHelperIfRequested() {
		return
	}
	app := &BridgeApp{cloudModels: make(map[string][]string), status: "准备就绪"}
	err := wails.Run(&options.App{
		Title:            appTitle,
		Width:            1080,
		Height:           780,
		MinWidth:         850,
		MinHeight:        650,
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 245, A: 255},
		AssetServer:      &assetserver.Options{Assets: wailsAssets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind:             []interface{}{app},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func (app *BridgeApp) startup(ctx context.Context) {
	app.mu.Lock()
	app.ctx = ctx
	app.mu.Unlock()
	loadSavedConfiguration()
	key, err := decryptText(currentConfig.EncryptedKey)
	app.mu.Lock()
	app.baseURL = currentConfig.BaseURL
	app.apiKey = key
	app.models = append([]string(nil), currentConfig.Models...)
	app.model = currentConfig.DefaultModel
	app.mu.Unlock()
	if err != nil {
		app.setStatus("已读取配置，但 API Key 解密失败，请重新输入并保存。")
		return
	}
	if app.apiKey != "" {
		app.setStatus(fmt.Sprintf("已载入本机加密配置和 %d 个模型。", len(app.models)))
	}
}

func (app *BridgeApp) shutdown(context.Context) {
	stopActiveProxy()
}

func (app *BridgeApp) beforeClose(ctx context.Context) bool {
	if !hasActiveProxy() {
		return false
	}
	answer, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
		Type:    wailsruntime.QuestionDialog,
		Title:   "退出云桥客户端？",
		Message: "Codex 正通过云桥连接中转 API。退出后当前对话会中断，确定退出吗？",
		Buttons: []string{"继续使用", "退出"},
	})
	return err == nil && answer != "退出"
}

func (app *BridgeApp) GetState() bridgeViewState {
	app.mu.Lock()
	defer app.mu.Unlock()
	connection := "api"
	if app.account != nil {
		connection = "bridge"
	}
	return bridgeViewState{
		BaseURL:       app.baseURL,
		HasAPIKey:     app.apiKey != "",
		Models:        append([]string(nil), app.models...),
		SelectedModel: app.model,
		Version:       appVersion,
		Status:        app.status,
		Connection:    connection,
		Cloud:         app.cloudStateLocked(),
	}
}

func (app *BridgeApp) FetchModels(baseURL, apiKey string) (modelFetchResult, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	apiKey = app.resolveAPIKey(apiKey)
	resolved, models, err := fetchModelsWithRemotePolicy(baseURL, apiKey)
	if err != nil {
		return modelFetchResult{}, err
	}
	app.mu.Lock()
	app.baseURL, app.models = resolved, append([]string(nil), models...)
	if !containsString(models, app.model) {
		app.model = chooseDefaultModel(models, app.model)
	}
	app.mu.Unlock()
	app.setStatus(fmt.Sprintf("已连接接口，找到 %d 个模型。", len(models)))
	return modelFetchResult{BaseURL: resolved, Models: models}, nil
}

func (app *BridgeApp) SaveConnection(baseURL, apiKey, selectedModel string) (bridgeViewState, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	apiKey = app.resolveAPIKey(apiKey)
	resolved, models, err := fetchModelsWithRemotePolicy(baseURL, apiKey)
	if err != nil {
		return bridgeViewState{}, err
	}
	selectedModel = strings.TrimSpace(selectedModel)
	if !containsString(models, selectedModel) {
		selectedModel = chooseDefaultModel(models, app.model)
	}
	if err := saveApplicationConfig(resolved, apiKey, models, selectedModel); err != nil {
		return bridgeViewState{}, fmt.Errorf("保存配置失败：%w", err)
	}
	app.mu.Lock()
	app.baseURL, app.apiKey = resolved, apiKey
	app.models, app.model = append([]string(nil), models...), selectedModel
	app.mu.Unlock()
	app.setStatus(fmt.Sprintf("配置已加密保存，已同步 %d 个模型。", len(models)))
	return app.GetState(), nil
}

func (app *BridgeApp) LoginCloud(email, password string) (cloudViewState, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	client := newAccountClient()
	user, err := client.login(email, password)
	if err != nil {
		return cloudViewState{}, err
	}
	entitlements, err := client.entitlements(user)
	if err != nil {
		return cloudViewState{}, err
	}
	app.mu.Lock()
	app.account = client
	app.entitlements = entitlements
	app.cloudModels = make(map[string][]string)
	state := app.cloudStateLocked()
	app.mu.Unlock()
	app.setStatus("云桥账号已登录，选择上游并同步模型。")
	return state, nil
}

func (app *BridgeApp) FetchProviderModels(provider string) ([]string, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	app.mu.Lock()
	key := app.entitlements.ProviderKeys[provider]
	if cached := app.cloudModels[provider]; len(cached) > 0 {
		models := append([]string(nil), cached...)
		app.mu.Unlock()
		return models, nil
	}
	app.mu.Unlock()
	if key == "" {
		return nil, errors.New("请先登录云桥账号并选择有效上游")
	}
	baseURL, models, err := fetchModelsWithRemotePolicy(defaultBaseURL, key)
	if err != nil {
		return nil, err
	}
	app.mu.Lock()
	app.cloudModels[provider] = append([]string(nil), models...)
	app.baseURL = baseURL
	app.models = append([]string(nil), models...)
	app.model = chooseDefaultModel(models, app.model)
	app.mu.Unlock()
	app.setStatus(fmt.Sprintf("%s 上游已同步 %d 个模型。", provider, len(models)))
	return models, nil
}

func (app *BridgeApp) LogoutCloud() cloudViewState {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	app.mu.Lock()
	app.account = nil
	app.entitlements = accountEntitlements{}
	app.cloudModels = make(map[string][]string)
	state := app.cloudStateLocked()
	app.mu.Unlock()
	app.setStatus("已退出云桥账号。")
	return state
}

func (app *BridgeApp) Launch(connection, provider, selectedModel, baseURL, apiKey string) (string, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	if connection == "official" {
		return app.launchOfficial()
	}
	return app.launchRouted(connection, provider, selectedModel, baseURL, apiKey)
}

func (app *BridgeApp) launchRouted(connection, provider, selectedModel, baseURL, apiKey string) (string, error) {
	if connection == "bridge" {
		app.mu.Lock()
		apiKey = app.entitlements.ProviderKeys[provider]
		baseURL = defaultBaseURL
		models := append([]string(nil), app.cloudModels[provider]...)
		app.mu.Unlock()
		if apiKey == "" {
			return "", errors.New("请先登录云桥账号并选择上游")
		}
		if len(models) == 0 {
			resolved, fetched, err := fetchModelsWithRemotePolicy(baseURL, apiKey)
			if err != nil {
				return "", err
			}
			baseURL, models = resolved, fetched
		}
		return app.launchWithCredentials(baseURL, apiKey, models, selectedModel)
	}
	if connection != "api" {
		return "", errors.New("未知的连接方式")
	}
	apiKey = app.resolveAPIKey(apiKey)
	resolved, models, err := fetchModelsWithRemotePolicy(baseURL, apiKey)
	if err != nil {
		return "", err
	}
	return app.launchWithCredentials(resolved, apiKey, models, selectedModel)
}

func (app *BridgeApp) launchWithCredentials(baseURL, apiKey string, models []string, selectedModel string) (string, error) {
	models = withSmartRouterModel(models)
	selectedModel = strings.TrimSpace(selectedModel)
	if !containsString(models, selectedModel) {
		selectedModel = chooseDefaultModel(models, app.model)
	}
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	if err := saveApplicationConfig(baseURL, apiKey, models, selectedModel); err != nil {
		return "", fmt.Errorf("保存配置失败：%w", err)
	}
	app.mu.Lock()
	app.baseURL, app.apiKey = baseURL, apiKey
	app.models, app.model = append([]string(nil), models...), selectedModel
	app.mu.Unlock()

	app.setStatus("正在启动本机 API 代理…")
	proxy, err := replaceAPIProxy(baseURL, apiKey, models)
	if err != nil {
		return "", err
	}
	keepProxy := false
	defer func() {
		if !keepProxy {
			stopAPIProxy(proxy)
		}
	}()

	app.setStatus("正在准备 Codex 供应商配置…")
	if err := writeCodexProviderConfig(codexProxyBase, selectedModel); err != nil {
		return "", err
	}
	install, err := findCodexInstallation()
	if err != nil {
		return "", err
	}
	app.setStatus("正在启动 Codex…")
	if err := launchCodex(install); err != nil {
		return "", err
	}
	menuResult := make(chan error, 1)
	go func() { menuResult <- localizeNativeMenu(inspectorPort) }()
	if err := injectIntoCodex(cdpPort, models, selectedModel, func(s string) { app.setStatus(s) }); err != nil {
		return "", err
	}
	go syncProxyImagesToCodex(cdpPort, proxy)
	go maintainCodexInjection(cdpPort, models, selectedModel, proxy.done, diagnosticLog)
	keepProxy = true

	app.setStatus("正在确认 Codex 中文界面…")
	localeState, localeErr := waitForChineseLocalization(cdpPort, 15*time.Second)
	if localeErr == nil && localeState.RestartRequired {
		app.setStatus("正在完成首次中文设置并重启 Codex…")
		if err := launchCodex(install); err != nil {
			return "", fmt.Errorf("中文设置已写入，但重启 Codex 失败：%w", err)
		}
		menuResult = make(chan error, 1)
		go func() { menuResult <- localizeNativeMenu(inspectorPort) }()
		if err := injectIntoCodex(cdpPort, models, selectedModel, func(s string) { app.setStatus(s) }); err != nil {
			return "", fmt.Errorf("Codex 已重启，但重新注入失败：%w", err)
		}
		localeState, localeErr = waitForChineseLocalization(cdpPort, 15*time.Second)
	}
	menuErr := <-menuResult
	if localeErr != nil {
		diagnosticLog("locale.localization_failed", localeErr.Error())
		message := fmt.Sprintf("代理与模型已启动，但中文界面未确认；详情见 %s", diagnosticLogPath())
		app.setStatus(message)
		return message, nil
	}
	if menuErr != nil {
		diagnosticLog("menu.localization_failed", menuErr.Error())
		message := fmt.Sprintf("代理与模型已启动，但原生菜单汉化失败；详情见 %s", diagnosticLogPath())
		app.setStatus(message)
		return message, nil
	}
	diagnosticLog("launch.ready", fmt.Sprintf("models=%d", len(models)))
	message := fmt.Sprintf("启动成功：中文界面与 %d 个模型已就绪。请保持云桥客户端运行。", len(models))
	app.setStatus(message)
	return message, nil
}

func (app *BridgeApp) launchOfficial() (string, error) {
	stopActiveProxy()
	if err := restoreNativeCodexConfig(); err != nil {
		return "", err
	}
	install, err := findCodexInstallation()
	if err != nil {
		return "", err
	}
	app.setStatus("正在使用官方账号启动 Codex…")
	if err := launchCodex(install); err != nil {
		return "", err
	}
	menuResult := make(chan error, 1)
	go func() { menuResult <- localizeNativeMenu(inspectorPort) }()
	if err := injectLocalizationIntoCodex(cdpPort, func(s string) { app.setStatus(s) }); err != nil {
		return "", err
	}
	locale, err := waitForChineseLocalization(cdpPort, 15*time.Second)
	if err == nil && locale.RestartRequired {
		app.setStatus("正在完成首次中文设置并重启 Codex…")
		if err := launchCodex(install); err != nil {
			return "", err
		}
		menuResult = make(chan error, 1)
		go func() { menuResult <- localizeNativeMenu(inspectorPort) }()
		if err := injectLocalizationIntoCodex(cdpPort, func(s string) { app.setStatus(s) }); err != nil {
			return "", err
		}
		locale, err = waitForChineseLocalization(cdpPort, 15*time.Second)
	}
	menuErr := <-menuResult
	if err != nil {
		message := fmt.Sprintf("官方账号已启动，但中文界面未确认；详情见 %s", diagnosticLogPath())
		app.setStatus(message)
		return message, nil
	}
	if menuErr != nil {
		message := fmt.Sprintf("Codex 已启动，但原生菜单汉化失败；详情见 %s", diagnosticLogPath())
		app.setStatus(message)
		return message, nil
	}
	diagnosticLog("launch.native_ready", "provider=official")
	message := "已通过官方账号启动 Codex。"
	app.setStatus(message)
	return message, nil
}

func (app *BridgeApp) CheckUpdates() (updateViewState, error) {
	manifest, err := fetchBridgeUpdateManifest()
	if err != nil {
		return updateViewState{}, err
	}
	if len(manifest.Notes) > 700 {
		manifest.Notes = manifest.Notes[:700] + "…"
	}
	return updateViewState{
		Current: appVersion, Latest: manifest.Version,
		Available: compareVersions(manifest.Version, appVersion) > 0,
		Notes: manifest.Notes,
	}, nil
}

func (app *BridgeApp) InstallUpdate() (string, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	manifest, err := fetchBridgeUpdateManifest()
	if err != nil {
		return "", err
	}
	if compareVersions(manifest.Version, appVersion) <= 0 {
		return "当前已是最新版。", nil
	}
	app.setStatus("正在下载更新…")
	staged, err := downloadBridgeUpdate(manifest, func(percent int) {
		app.setStatus(fmt.Sprintf("正在下载更新… %d%%", percent))
	})
	if err != nil {
		return "", err
	}
	if err := verifyFileSHA256(staged, manifest.SHA256); err != nil {
		return "", err
	}
	if err := scheduleBridgeReplacement(staged, manifest.Version); err != nil {
		return "", err
	}
	app.setStatus("更新已通过 SHA-256 校验，正在安装并重启…")
	stopActiveProxy()
	wailsruntime.Quit(app.context())
	return "更新已开始安装。", nil
}

func (app *BridgeApp) resolveAPIKey(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.apiKey
}

func (app *BridgeApp) cloudStateLocked() cloudViewState {
	state := cloudViewState{Usage: []string{"每日用量  未登录", "每周用量  未登录", "每月用量  未登录"}}
	if app.account == nil {
		return state
	}
	state.LoggedIn = true
	state.Identity = app.entitlements.User.Email
	if state.Identity == "" {
		state.Identity = app.entitlements.User.Username
	}
	groups := make([]string, 0, len(app.entitlements.Subscriptions))
	for _, item := range app.entitlements.Subscriptions {
		if item.Group != nil && strings.TrimSpace(item.Group.Name) != "" {
			groups = append(groups, item.Group.Name)
		}
	}
	state.Groups = strings.Join(groups, "、")
	for _, name := range []string{"ChatGPT", "Gemini", "Grok"} {
		if app.entitlements.ProviderKeys[name] != "" {
			state.Providers = append(state.Providers, name)
		}
	}
	usage := usageLines(app.entitlements.Progress)
	state.Usage = []string{usage[0], usage[1], usage[2]}
	return state
}

func (app *BridgeApp) context() context.Context {
	app.mu.Lock()
	defer app.mu.Unlock()
	return app.ctx
}

func (app *BridgeApp) setStatus(status string) {
	app.mu.Lock()
	app.status = status
	ctx := app.ctx
	app.mu.Unlock()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, "bridge:status", status)
	}
}
