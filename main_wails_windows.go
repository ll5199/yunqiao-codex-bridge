//go:build windows

package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
	mu           sync.Mutex
	opMu         sync.Mutex
	ctx          context.Context
	baseURL      string
	apiKey       string
	models       []string
	model        string
	account      *accountClient
	entitlements accountEntitlements
	cloudModels  map[string][]string
	status       string
}

type bridgeViewState struct {
	BaseURL        string         `json:"baseURL"`
	HasAPIKey      bool           `json:"hasAPIKey"`
	Models         []string       `json:"models"`
	SelectedModel  string         `json:"selectedModel"`
	Version        string         `json:"version"`
	UpstreamStatus string         `json:"upstreamStatus"`
	Running        bool           `json:"running"`
	ProxyRunning   bool           `json:"proxyRunning"`
	Status         string         `json:"status"`
	Connection     string         `json:"connection"`
	Cloud          cloudViewState `json:"cloud"`
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
		Width:            1000,
		Height:           680,
		MinWidth:         900,
		MinHeight:        600,
		Frameless:        true,
		BackgroundColour: &options.RGBA{R: 246, G: 248, B: 245, A: 255},
		AssetServer:      &assetserver.Options{Assets: wailsAssets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
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
	if result, e := os.ReadFile(filepath.Join(applicationDirectory(), "update-result.txt")); e == nil {
		app.setStatus(string(result))
		_ = os.Remove(filepath.Join(applicationDirectory(), "update-result.txt"))
		return
	}
	if app.apiKey != "" {
		app.setStatus(fmt.Sprintf("已载入本机加密配置和 %d 个模型。", len(app.models)))
	}
}

func (app *BridgeApp) shutdown(context.Context) {
	stopActiveProxy()
}

// Close uses the bound Go API so the titlebar button has a reliable exit path.
func (app *BridgeApp) Close() {
	wailsruntime.Quit(app.context())
}

func (app *BridgeApp) GetState() bridgeViewState {
	app.mu.Lock()
	defer app.mu.Unlock()
	connection := "api"
	if app.account != nil {
		connection = "bridge"
	}
	return bridgeViewState{
		BaseURL:        app.baseURL,
		HasAPIKey:      app.apiKey != "",
		Models:         append([]string(nil), app.models...),
		SelectedModel:  app.model,
		Version:        appVersion,
		Status:         app.status,
		Running:        codexIsRunning(),
		ProxyRunning:   bridgeProxyRunning(),
		UpstreamStatus: bridgeUpstreamStatus(),
		Connection:     connection,
		Cloud:          app.cloudStateLocked(),
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
	if err := saveApplicationConfig(resolved, apiKey, models, selectedModel, false); err != nil {
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
	_, models, err := fetchModelsWithRemotePolicy(defaultBaseURL, key)
	if err != nil {
		return nil, err
	}
	app.mu.Lock()
	app.cloudModels[provider] = append([]string(nil), models...)
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
	if codexIsRunning() {
		return "Codex 已在运行，请先退出 Codex 后再启动。", nil
	}
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
		return app.launchWithCredentials(baseURL, apiKey, models, selectedModel, false)
	}
	if connection != "api" {
		return "", errors.New("未知的连接方式")
	}
	apiKey = app.resolveAPIKey(apiKey)
	resolved, models, err := fetchModelsWithRemotePolicy(baseURL, apiKey)
	if err != nil {
		return "", err
	}
	return app.launchWithCredentials(resolved, apiKey, models, selectedModel, true)
}

func (app *BridgeApp) launchWithCredentials(baseURL, apiKey string, models []string, selectedModel string, persistAPI bool) (string, error) {
	models = withSmartRouterModel(models)
	selectedModel = strings.TrimSpace(selectedModel)
	if !containsString(models, selectedModel) {
		selectedModel = chooseDefaultModel(models, app.model)
	}
	baseURL, err := normalizeBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	// A cloud provider credential must never replace the user's saved external API Key.
	savedURL, savedKey, savedModels := baseURL, apiKey, models
	if !persistAPI {
		app.mu.Lock()
		savedURL, savedKey = app.baseURL, app.apiKey
		savedModels = append([]string(nil), app.models...)
		app.mu.Unlock()
	}
	if err := saveApplicationConfig(savedURL, savedKey, savedModels, smartRouterModel, false); err != nil {
		return "", fmt.Errorf("保存配置失败：%w", err)
	}
	if persistAPI {
		app.mu.Lock()
		app.baseURL, app.apiKey = baseURL, apiKey
		app.models, app.model = append([]string(nil), models...), selectedModel
		app.mu.Unlock()
	}

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
	keepProxy = true
	injectErr := injectIntoCodex(cdpPort, models, selectedModel, func(s string) { app.setStatus(s) })
	if injectErr != nil {
		diagnosticLog("launch.injection_failed", injectErr.Error())
		if !codexIsRunning() {
			keepProxy = false
			return "", fmt.Errorf("Codex 进程已退出：%w", injectErr)
		}
	}
	go syncProxyImagesToCodex(cdpPort, proxy)
	go maintainCodexInjection(cdpPort, models, selectedModel, proxy.done, diagnosticLog)
	if injectErr != nil {
		message := "Codex 已打开，本机代理运行中；模型列表与汉化未就绪，请导出诊断日志。"
		app.setStatus(message)
		return message, nil
	}

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
			diagnosticLog("launch.reinjection_failed", err.Error())
			message := "Codex 已重启，代理运行中；重新注入未就绪。"
			app.setStatus(message)
			return message, nil
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
		diagnosticLog("launch.native_injection_failed", err.Error())
		if !codexIsRunning() {
			return "", err
		}
		message := "官方 Codex 已打开，汉化未就绪。"
		app.setStatus(message)
		return message, nil
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
		Notes:     manifest.Notes,
	}, nil
}

func (app *BridgeApp) InstallUpdate() (string, error) {
	app.opMu.Lock()
	defer app.opMu.Unlock()
	app.emitUpdate("checking", 0, "正在检查更新…")
	manifest, err := fetchBridgeUpdateManifest()
	if err != nil {
		app.emitUpdate("error", 0, "检查更新失败："+err.Error())
		return "", err
	}
	if compareVersions(manifest.Version, appVersion) <= 0 {
		app.emitUpdate("idle", 0, "当前已是最新版。")
		return "当前已是最新版。", nil
	}
	app.emitUpdate("downloading", 0, fmt.Sprintf("正在下载 v%s…", manifest.Version))
	staged, err := downloadBridgeUpdate(manifest, func(percent int) {
		// Downloading fills the bar up to 90%; verification and install follow.
		app.emitUpdate("downloading", percent*90/100, fmt.Sprintf("正在下载 v%s…", manifest.Version))
	})
	if err != nil {
		app.emitUpdate("error", 0, "下载更新失败："+err.Error())
		return "", err
	}
	app.emitUpdate("verifying", 93, "正在校验更新文件（SHA-256）…")
	if err := verifyFileSHA256(staged, manifest.SHA256); err != nil {
		app.emitUpdate("error", 0, err.Error())
		return "", err
	}
	app.emitUpdate("installing", 97, "正在准备安装…")
	if err := scheduleBridgeReplacement(staged, manifest.Version); err != nil {
		app.emitUpdate("error", 0, "安装更新失败："+err.Error())
		return "", err
	}
	app.emitUpdate("restarting", 100, "更新已通过校验，正在安装并重启…")
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

// updateProgress drives the update panel: stage is checking, downloading,
// verifying, installing, restarting, idle or error; percent is 0-100.
type updateProgress struct {
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
	Message string `json:"message"`
}

func (app *BridgeApp) emitUpdate(stage string, percent int, message string) {
	app.setStatus(message)
	app.mu.Lock()
	ctx := app.ctx
	app.mu.Unlock()
	if ctx != nil {
		wailsruntime.EventsEmit(ctx, "bridge:update", updateProgress{Stage: stage, Percent: percent, Message: message})
	}
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

func bridgeProxyRunning() bool {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	if activeProxy == nil {
		return false
	}
	select {
	case <-activeProxy.done:
		return false
	default:
		return true
	}
}
func (app *BridgeApp) ExportDiagnostics() (string, error) {
	path, err := wailsruntime.SaveFileDialog(app.context(), wailsruntime.SaveDialogOptions{Title: "导出诊断日志", DefaultFilename: "Yunqiao-diagnostics.txt", Filters: []wailsruntime.FileFilter{{DisplayName: "文本日志", Pattern: "*.txt"}}})
	if err != nil || path == "" {
		return "", err
	}
	data, err := os.ReadFile(diagnosticLogPath())
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if len(data) > 2<<20 {
		data = data[len(data)-(2<<20):]
	}
	app.mu.Lock()
	secrets := []string{app.apiKey}
	for _, key := range app.entitlements.ProviderKeys {
		secrets = append(secrets, key)
	}
	app.mu.Unlock()
	state := app.GetState()
	header := fmt.Sprintf("Yunqiao version=%s running=%t proxy=%t\n", appVersion, state.Running, state.ProxyRunning)
	text := redactDiagnostics(header+string(data), secrets...)
	logs, _ := filepath.Glob(filepath.Join(applicationDirectory(), "updates", "*.log"))
	for _, name := range logs {
		b, e := os.ReadFile(name)
		if e == nil {
			if len(b) > 64<<10 {
				b = b[len(b)-(64<<10):]
			}
			text += "\nUpdate log:\n" + redactDiagnostics(string(b), secrets...)
		}
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		return "", err
	}
	return "诊断日志已导出（凭据已脱敏）。", nil
}

func bridgeUpstreamStatus() string {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	if activeProxy != nil && activeProxy.upstreamSnapshot != nil {
		return activeProxy.upstreamSnapshot()
	}
	return ""
}
