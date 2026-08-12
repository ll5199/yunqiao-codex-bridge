//go:build windows

package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	appTitle      = "云桥（熙楠）"
	cdpPort       = 9229
	inspectorPort = 9329

	wmCreate           = 0x0001
	wmDestroy          = 0x0002
	wmSize             = 0x0005
	wmGetMinMaxInfo    = 0x0024
	wmCommand          = 0x0111
	wmKeyDown          = 0x0100
	wmClose            = 0x0010
	wmSetFont          = 0x0030
	wmAppResult        = 0x8001
	wmAppAdvertisement = 0x8002
	wmCtlColorEdit     = 0x0133
	wmCtlColorList     = 0x0134
	wmCtlColorBtn      = 0x0135
	wmCtlColorText     = 0x0138
	emSetPasswordChar  = 0x00CC
	lbAddString        = 0x0180
	lbReset            = 0x0184
	lbSetCurSel        = 0x0186
	lbGetCurSel        = 0x0188
	cbAddString        = 0x0143
	cbResetContent     = 0x014B
	cbGetCurSel        = 0x0147
	cbSetCurSel        = 0x014E
	bmSetCheck         = 0x00F1
	emSetCueBanner     = 0x1501
	pbmSetPos          = 0x0402
	pbmSetRange32      = 0x0406
	swHide             = 0
	swShow             = 5
	colorWindow        = 5
	idcArrow           = 32512
	defaultGUIFont     = 17

	wsOverlappedWindow  = 0x00CF0000
	wsVisible           = 0x10000000
	wsChild             = 0x40000000
	wsTabStop           = 0x00010000
	wsVScroll           = 0x00200000
	wsBorder            = 0x00800000
	esAutoHScroll       = 0x0080
	esPassword          = 0x0020
	esReadOnly          = 0x0800
	lbsNotify           = 0x0001
	ssCenterImage       = 0x0200
	bsAutoRadioButton   = 0x0009
	bsDefaultPushButton = 0x0001
	cbsDropDownList     = 0x0003
	cbsHasStrings       = 0x0200
	bnClicked           = 0
	cbnSelChange        = 1
	enSetFocus          = 0x0100
	enKillFocus         = 0x0200
	vkReturn            = 0x0D

	controlFetch         = 101
	controlLaunch        = 102
	controlUpdate        = 103
	controlAdvertisement = 104
	controlModeAccount   = 105
	controlModeExternal  = 106
	controlLogin         = 107
	controlLogout        = 108
	controlProvider      = 109
	controlAccountModel  = 110
	controlAccountEdit   = 111
	controlPasswordEdit  = 112
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	crypt32  = syscall.NewLazyDLL("crypt32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procIsDialogMessageW   = user32.NewProc("IsDialogMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procSendMessageW       = user32.NewProc("SendMessageW")
	procSetWindowTextW     = user32.NewProc("SetWindowTextW")
	procMoveWindow         = user32.NewProc("MoveWindow")
	procGetWindowTextW     = user32.NewProc("GetWindowTextW")
	procGetWindowTextLen   = user32.NewProc("GetWindowTextLengthW")
	procMessageBoxW        = user32.NewProc("MessageBoxW")
	procEnableWindow       = user32.NewProc("EnableWindow")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procLoadCursorW        = user32.NewProc("LoadCursorW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procGetStockObject     = gdi32.NewProc("GetStockObject")
	procCreateSolidBrush   = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor       = gdi32.NewProc("SetTextColor")
	procSetBkColor         = gdi32.NewProc("SetBkColor")
	procSetBkMode          = gdi32.NewProc("SetBkMode")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotect     = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
	procCoInitializeEx     = ole32.NewProc("CoInitializeEx")
	procCoUninitialize     = ole32.NewProc("CoUninitialize")
	procCoCreateInstance   = ole32.NewProc("CoCreateInstance")
	procInitCommonControls = comctl32.NewProc("InitCommonControls")
	procShellExecuteW      = shell32.NewProc("ShellExecuteW")
)

type point struct {
	X int32
	Y int32
}

type message struct {
	HWnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type windowClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}

type dataBlob struct {
	Size uint32
	Data *byte
}

type minMaxInfo struct {
	Reserved, MaxSize, MaxPosition, MinTrackSize, MaxTrackSize point
}

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type activationManager struct {
	VTable *activationManagerVTable
}

type activationManagerVTable struct {
	QueryInterface      uintptr
	AddRef              uintptr
	Release             uintptr
	ActivateApplication uintptr
	ActivateForFile     uintptr
	ActivateForProtocol uintptr
}

type uiUpdate struct {
	Status      string
	Models      []string
	Error       error
	Done        bool
	Progress    int
	SetProgress bool
	Account     *accountEntitlements
	Provider    string
}

var (
	mainWindow                uintptr
	baseEdit                  uintptr
	keyEdit                   uintptr
	modelList                 uintptr
	statusLabel               uintptr
	fetchButton               uintptr
	launchButton              uintptr
	updateButton              uintptr
	advertisementLabel        uintptr
	advertisementButton       uintptr
	currentAdvertisement      advertisementConfig
	pendingAdvertisement      advertisementConfig
	advertisementMutex        sync.Mutex
	currentModels             []string
	currentConfig             appConfig
	activeProxy               *apiProxy
	proxyMutex                sync.Mutex
	updateMutex               sync.Mutex
	pendingUpdate             uiUpdate
	lastStatus                string
	backgroundBrush           uintptr
	controlBrush              uintptr
	statusBrush               uintptr
	brandLabel                uintptr
	subtitleLabel             uintptr
	baseLabel                 uintptr
	keyLabel                  uintptr
	modelsLabel               uintptr
	statusTitle               uintptr
	footerLabel               uintptr
	progressBar               uintptr
	progressLabel             uintptr
	modeAccountButton         uintptr
	modeExternalButton        uintptr
	accountEdit               uintptr
	passwordEdit              uintptr
	loginButton               uintptr
	logoutButton              uintptr
	memberInfoLabel           uintptr
	providerLabel             uintptr
	providerCombo             uintptr
	accountModelLabel         uintptr
	accountModelCombo         uintptr
	usageTitle                uintptr
	usageList                 uintptr
	accountMode               = true
	loggedIn                  bool
	accountProviderKeys       = make(map[string]string)
	accountModels             = make(map[string][]string)
	lastClientWidth           int
	lastClientHeight          int
	accountPlaceholderActive  bool
	passwordPlaceholderActive bool
)

const (
	accountPlaceholder  = "用户"
	passwordPlaceholder = "密码"
)

const (
	// Win32 COLORREF values use BGR byte order. This palette mirrors the
	// restrained Codex light appearance: warm canvas, white surfaces,
	// charcoal text and a muted green accent.
	colorBackground uintptr = 0xF4F7F7 // #F7F7F4
	colorControl    uintptr = 0xFFFFFF // #FFFFFF
	colorStatus     uintptr = 0xEAF3EE // #EEF3EA
	colorText       uintptr = 0x1B1F1F // #1F1F1B
	colorAccent     uintptr = 0x4F6035 // #35604F
)

func main() {
	if runUpdateHelperIfRequested() {
		return
	}
	runtime.LockOSThread()
	loadSavedConfiguration()
	procInitCommonControls.Call()

	instance, _, _ := procGetModuleHandleW.Call(0)
	backgroundBrush, _, _ = procCreateSolidBrush.Call(colorBackground)
	controlBrush, _, _ = procCreateSolidBrush.Call(colorControl)
	statusBrush, _, _ = procCreateSolidBrush.Call(colorStatus)
	className := utf16("YunqiaoCodexBridgeWindow")
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	class := windowClassEx{
		Size:       uint32(unsafe.Sizeof(windowClassEx{})),
		WndProc:    syscall.NewCallback(windowProc),
		Instance:   instance,
		Cursor:     cursor,
		Background: backgroundBrush,
		ClassName:  className,
	}
	if result, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		messageBox("无法注册程序窗口。", 0x10)
		return
	}

	mainWindow, _, _ = procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(utf16(appTitle))),
		wsOverlappedWindow|wsVisible,
		0x80000000, 0x80000000, 760, 810,
		0, 0, instance, 0,
	)
	if mainWindow == 0 {
		messageBox("无法创建程序窗口。", 0x10)
		return
	}
	procShowWindow.Call(mainWindow, swShow)
	procUpdateWindow.Call(mainWindow)

	var msg message
	for {
		result, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) <= 0 {
			break
		}
		if msg.Message == wmKeyDown && msg.WParam == vkReturn && msg.LParam&(1<<30) == 0 && accountMode && (msg.HWnd == accountEdit || msg.HWnd == passwordEdit) {
			startAccountLogin()
			continue
		}
		if handled, _, _ := procIsDialogMessageW.Call(mainWindow, uintptr(unsafe.Pointer(&msg))); handled != 0 {
			continue
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmCreate:
		createControls(hwnd)
		return 0
	case wmSize:
		layoutControls(int(uint16(lParam&0xffff)), int(uint16((lParam>>16)&0xffff)))
		return 0
	case wmGetMinMaxInfo:
		info := (*minMaxInfo)(unsafe.Pointer(lParam))
		info.MinTrackSize = point{X: 540, Y: 760}
		return 0
	case wmCommand:
		controlID := int(wParam & 0xffff)
		notification := int((wParam >> 16) & 0xffff)
		switch controlID {
		case controlFetch:
			startFetch()
		case controlLaunch:
			startSaveAndLaunch()
		case controlUpdate:
			startBridgeUpdate()
		case controlAdvertisement:
			openAdvertisementDetails()
		case controlModeAccount:
			setConnectionMode(true)
		case controlModeExternal:
			setConnectionMode(false)
		case controlLogin:
			if notification == bnClicked {
				startAccountLogin()
			}
		case controlLogout:
			if notification == bnClicked {
				logoutAccount()
			}
		case controlProvider:
			if notification == cbnSelChange {
				loadSelectedProviderModels()
			}
		case controlAccountEdit:
			handleLoginPlaceholderFocus(accountEdit, notification == enSetFocus, notification == enKillFocus)
		case controlPasswordEdit:
			handleLoginPlaceholderFocus(passwordEdit, notification == enSetFocus, notification == enKillFocus)
		}
		return 0
	case wmAppResult:
		applyPendingUpdate()
		return 0
	case wmAppAdvertisement:
		applyPendingAdvertisement()
		return 0
	case wmCtlColorText:
		if lParam == advertisementLabel {
			procSetTextColor.Call(wParam, colorAccent)
			procSetBkColor.Call(wParam, colorStatus)
			return statusBrush
		}
		if lParam == brandLabel || lParam == footerLabel {
			procSetTextColor.Call(wParam, colorAccent)
		} else {
			procSetTextColor.Call(wParam, colorText)
		}
		procSetBkMode.Call(wParam, 1)
		return backgroundBrush
	case wmCtlColorEdit, wmCtlColorList, wmCtlColorBtn:
		if lParam == statusLabel {
			procSetTextColor.Call(wParam, colorAccent)
			procSetBkColor.Call(wParam, colorStatus)
			return statusBrush
		}
		procSetTextColor.Call(wParam, colorText)
		procSetBkColor.Call(wParam, colorControl)
		return controlBrush
	case wmClose:
		if hasActiveProxy() {
			result := messageBoxResult("Codex 正在通过 Bridge 连接中转 API。\n关闭 Bridge 后当前 Codex 对话会中断，确定退出吗？", 0x34)
			if result != 6 {
				return 0
			}
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		stopActiveProxy()
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func createControls(hwnd uintptr) {
	font, _, _ := procGetStockObject.Call(defaultGUIFont)
	brandLabel = createLabel(hwnd, "云桥（熙楠）", 28, 22, 240, 26, font)
	subtitleLabel = createLabel(hwnd, "Codex Bridge  ·  安全连接中转 API", 28, 50, 360, 22, font)
	modeAccountButton = createControl(hwnd, "BUTTON", "云桥账号", wsChild|wsVisible|wsTabStop|bsAutoRadioButton, 28, 82, 112, 30, controlModeAccount)
	modeExternalButton = createControl(hwnd, "BUTTON", "外部 API", wsChild|wsVisible|wsTabStop|bsAutoRadioButton, 154, 82, 112, 30, controlModeExternal)
	procSendMessageW.Call(modeAccountButton, bmSetCheck, 1, 0)

	accountEdit = createControl(hwnd, "EDIT", "", wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll, 48, 120, 300, 34, controlAccountEdit)
	passwordEdit = createControl(hwnd, "EDIT", "", wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll|esPassword, 358, 120, 300, 34, controlPasswordEdit)
	showLoginPlaceholder(accountEdit)
	showLoginPlaceholder(passwordEdit)
	loginButton = createControl(hwnd, "BUTTON", "登录", wsChild|wsVisible|wsTabStop|bsDefaultPushButton, 242, 164, 126, 34, controlLogin)
	logoutButton = createControl(hwnd, "BUTTON", "退出", wsChild|wsVisible|wsTabStop, 378, 164, 126, 34, controlLogout)
	memberInfoLabel = createControl(hwnd, "STATIC", "未登录", wsChild|wsVisible|wsBorder|ssCenterImage, 28, 208, 690, 28, 0)
	providerLabel = createLabel(hwnd, "上游", 28, 246, 100, 20, font)
	accountModelLabel = createLabel(hwnd, "模型", 380, 246, 100, 20, font)
	providerCombo = createControl(hwnd, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropDownList|cbsHasStrings, 28, 268, 336, 160, controlProvider)
	accountModelCombo = createControl(hwnd, "COMBOBOX", "", wsChild|wsVisible|wsTabStop|wsVScroll|cbsDropDownList|cbsHasStrings, 378, 268, 340, 160, controlAccountModel)
	usageTitle = createLabel(hwnd, "订阅用量", 28, 440, 120, 22, font)
	usageList = createControl(hwnd, "LISTBOX", "", wsChild|wsVisible|wsBorder|wsVScroll, 28, 466, 690, 82, 0)
	fillUsage([3]string{"每日用量    登录后显示", "每周用量    登录后显示", "每月用量    登录后显示"})

	currentAdvertisement = defaultAdvertisementConfig()
	if cached, err := loadAdvertisementCache(advertisementCachePath()); err == nil {
		currentAdvertisement = cached
	}
	advertisementLabel = createControl(hwnd, "STATIC", currentAdvertisement.labelText(), wsChild|wsVisible|wsBorder|ssCenterImage, 28, 318, 538, 48, 0)
	advertisementButton = createControl(hwnd, "BUTTON", consultationButtonText, wsChild|wsVisible|wsTabStop, 576, 324, 142, 36, controlAdvertisement)
	baseLabel = createLabel(hwnd, "API 接口", 28, 152, 100, 24, font)
	baseEdit = createControl(hwnd, "EDIT", defaultBaseURL, wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll, 28, 178, 690, 32, 0)
	keyLabel = createLabel(hwnd, "API Key（使用 Windows DPAPI 加密，仅保存在本机）", 28, 226, 420, 24, font)
	keyEdit = createControl(hwnd, "EDIT", "", wsChild|wsVisible|wsTabStop|wsBorder|esAutoHScroll|esPassword, 28, 252, 690, 32, 0)
	fetchButton = createControl(hwnd, "BUTTON", "获取模型", wsChild|wsVisible|wsTabStop, 28, 304, 142, 38, controlFetch)
	launchButton = createControl(hwnd, "BUTTON", "保存并启动 Codex", wsChild|wsVisible|wsTabStop, 174, 304, 192, 38, controlLaunch)
	updateButton = createControl(hwnd, "BUTTON", "检查云桥更新", wsChild|wsVisible|wsTabStop, 380, 304, 176, 38, controlUpdate)
	modelsLabel = createLabel(hwnd, "API 返回的模型", 28, 366, 180, 24, font)
	modelList = createControl(hwnd, "LISTBOX", "", wsChild|wsVisible|wsBorder|wsVScroll|lbsNotify, 28, 392, 690, 220, 0)
	statusTitle = createLabel(hwnd, "运行状态", 28, 636, 100, 22, font)
	statusLabel = createControl(hwnd, "EDIT", "填写接口和 Key 后点击“获取模型”。", wsChild|wsVisible|wsBorder|esAutoHScroll|esReadOnly, 28, 662, 690, 32, 0)
	progressLabel = createLabel(hwnd, "更新进度  0%", 28, 706, 150, 20, font)
	progressBar = createControl(hwnd, "msctls_progress32", "", wsChild|wsVisible|wsBorder, 28, 730, 690, 18, 0)
	procSendMessageW.Call(progressBar, pbmSetRange32, 0, 100)
	footerLabel = createLabel(hwnd, fmt.Sprintf("云桥服务器更新源  ·  Bridge v%s", appVersion), 28, 758, 360, 20, font)
	layoutControls(744, 771)

	for _, handle := range []uintptr{modeAccountButton, modeExternalButton, accountEdit, passwordEdit, loginButton, logoutButton, memberInfoLabel, providerCombo, accountModelCombo, usageList, advertisementLabel, advertisementButton, baseEdit, keyEdit, fetchButton, launchButton, updateButton, modelList, statusLabel} {
		procSendMessageW.Call(handle, wmSetFont, font, 1)
	}
	if currentConfig.BaseURL != "" {
		setText(baseEdit, currentConfig.BaseURL)
	}
	if key, err := decryptText(currentConfig.EncryptedKey); err == nil {
		setText(keyEdit, key)
	}
	if len(currentConfig.Models) > 0 {
		currentModels = append([]string(nil), currentConfig.Models...)
		fillModels(currentModels)
		setStatus(fmt.Sprintf("已载入上次保存的 %d 个模型。", len(currentModels)))
	}
	setAdvertisementVisibility(currentAdvertisement.Enabled)
	setConnectionMode(true)
	startAdvertisementRefresh()
}

func layoutControls(width, height int) {
	if brandLabel == 0 || width <= 0 || height <= 0 {
		return
	}
	lastClientWidth, lastClientHeight = width, height
	layout := calculateWindowLayoutForMode(width, height, currentAdvertisement.Enabled, accountMode)
	items := []struct {
		handle uintptr
		rect   controlRect
	}{
		{brandLabel, layout.Brand}, {subtitleLabel, layout.Subtitle},
		{modeAccountButton, layout.ModeAccount}, {modeExternalButton, layout.ModeExternal},
		{advertisementLabel, layout.Advertisement}, {advertisementButton, layout.AdvertisementButton},
		{baseLabel, layout.BaseLabel}, {baseEdit, layout.BaseEdit},
		{keyLabel, layout.KeyLabel}, {keyEdit, layout.KeyEdit},
		{fetchButton, layout.FetchButton}, {launchButton, layout.LaunchButton},
		{updateButton, layout.UpdateButton}, {modelsLabel, layout.ModelsLabel},
		{modelList, layout.ModelsList}, {statusTitle, layout.StatusTitle},
		{statusLabel, layout.StatusEdit}, {progressLabel, layout.ProgressLabel},
		{progressBar, layout.ProgressBar}, {footerLabel, layout.Footer},
		{accountEdit, layout.AccountEdit}, {passwordEdit, layout.PasswordEdit},
		{loginButton, layout.LoginButton}, {logoutButton, layout.LogoutButton},
		{memberInfoLabel, layout.MemberInfo}, {providerLabel, layout.ProviderLabel},
		{providerCombo, layout.ProviderCombo}, {accountModelLabel, layout.AccountModelLabel},
		{accountModelCombo, layout.AccountModelCombo}, {usageTitle, layout.UsageTitle},
		{usageList, layout.UsageList},
	}
	for _, item := range items {
		if item.handle == 0 || item.rect.Width <= 0 || item.rect.Height <= 0 {
			continue
		}
		height := item.rect.Height
		// A Win32 COMBOBOX uses its total window height for the opened list.
		// The responsive row height is only the collapsed height; applying it
		// directly made a populated model dropdown display one item at a time.
		if item.handle == providerCombo {
			height = 130
		} else if item.handle == accountModelCombo {
			height = 260
		}
		procMoveWindow.Call(item.handle, uintptr(item.rect.X), uintptr(item.rect.Y), uintptr(item.rect.Width), uintptr(height), 1)
	}
}

func openAdvertisementDetails() {
	if !currentAdvertisement.Enabled || currentAdvertisement.URL == "" {
		return
	}
	detailURL := currentAdvertisement.URL
	result, _, _ := procShellExecuteW.Call(
		mainWindow,
		uintptr(unsafe.Pointer(utf16("open"))),
		uintptr(unsafe.Pointer(utf16(detailURL))),
		0, 0, swShow,
	)
	if result <= 32 {
		messageBox("无法打开详情页面，请稍后重试。\n"+detailURL, 0x10)
	}
}

func advertisementCachePath() string {
	return filepath.Join(applicationDirectory(), "ad.json")
}

func startAdvertisementRefresh() {
	go func() {
		config, _, err := fetchAdvertisementConfig(newAdvertisementHTTPClient(), advertisementConfigURL)
		if err != nil {
			diagnosticLog("advertisement.remote_failed", err.Error())
			return
		}
		if err := saveAdvertisementCache(advertisementCachePath(), config); err != nil {
			diagnosticLog("advertisement.cache_failed", err.Error())
		}
		advertisementMutex.Lock()
		pendingAdvertisement = config
		advertisementMutex.Unlock()
		procPostMessageW.Call(mainWindow, wmAppAdvertisement, 0, 0)
	}()
}

func applyPendingAdvertisement() {
	advertisementMutex.Lock()
	config := pendingAdvertisement
	pendingAdvertisement = advertisementConfig{}
	advertisementMutex.Unlock()
	currentAdvertisement = config
	setText(advertisementLabel, config.labelText())
	// The consultation action is product UI, not remotely configurable ad copy.
	setText(advertisementButton, consultationButtonText)
	setAdvertisementVisibility(config.Enabled)
}

func setAdvertisementVisibility(visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swShow
	}
	procShowWindow.Call(advertisementLabel, command)
	procShowWindow.Call(advertisementButton, command)
	if lastClientWidth > 0 && lastClientHeight > 0 {
		layoutControls(lastClientWidth, lastClientHeight)
	}
}

func createLabel(parent uintptr, text string, x, y, width, height int, font uintptr) uintptr {
	handle := createControl(parent, "STATIC", text, wsChild|wsVisible, x, y, width, height, 0)
	procSendMessageW.Call(handle, wmSetFont, font, 1)
	return handle
}

func createControl(parent uintptr, class, text string, style uintptr, x, y, width, height, id int) uintptr {
	handle, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16(class))),
		uintptr(unsafe.Pointer(utf16(text))),
		style,
		uintptr(x), uintptr(y), uintptr(width), uintptr(height),
		parent, uintptr(id), 0, 0,
	)
	return handle
}

func setConnectionMode(useAccount bool) {
	accountMode = useAccount
	procSendMessageW.Call(modeAccountButton, bmSetCheck, boolToUintptr(useAccount), 0)
	procSendMessageW.Call(modeExternalButton, bmSetCheck, boolToUintptr(!useAccount), 0)
	accountControls := []uintptr{accountEdit, passwordEdit, loginButton, logoutButton, memberInfoLabel, providerLabel, providerCombo, accountModelLabel, accountModelCombo, usageTitle, usageList}
	externalControls := []uintptr{baseLabel, baseEdit, keyLabel, keyEdit, fetchButton, modelsLabel, modelList}
	showControls(accountControls, useAccount)
	showControls(externalControls, !useAccount)
	if useAccount {
		setStatus("登录后选择上游和模型。")
	} else {
		setStatus("填入 API 地址和 Key，点击“获取模型”。")
	}
	if lastClientWidth > 0 && lastClientHeight > 0 {
		layoutControls(lastClientWidth, lastClientHeight)
	}
}

func showControls(controls []uintptr, visible bool) {
	command := uintptr(swHide)
	if visible {
		command = swShow
	}
	for _, handle := range controls {
		procShowWindow.Call(handle, command)
	}
}

func boolToUintptr(value bool) uintptr {
	if value {
		return 1
	}
	return 0
}

func startAccountLogin() {
	user := strings.TrimSpace(loginFieldValue(accountEdit))
	password := loginFieldValue(passwordEdit)
	if user == "" || password == "" {
		messageBox("请输入用户和密码。", 0x30)
		return
	}
	setAccountBusy(true)
	setStatus("正在登录云桥账号…")
	go func() {
		client := newAccountClient()
		account, err := client.login(user, password)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "登录成功，正在读取订阅、分组和 Key…"})
		entitlements, err := client.entitlements(account)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Account: &entitlements, Status: "账号资料已加载。", Done: true})
	}()
}

func setAccountBusy(busy bool) {
	enabled := uintptr(1)
	if busy {
		enabled = 0
	}
	for _, handle := range []uintptr{accountEdit, passwordEdit, loginButton, logoutButton, modeAccountButton, modeExternalButton} {
		procEnableWindow.Call(handle, enabled)
	}
}

func applyAccountEntitlements(entitlements accountEntitlements) {
	loggedIn = true
	accountProviderKeys = entitlements.ProviderKeys
	accountModels = make(map[string][]string)
	setText(memberInfoLabel, membershipText(entitlements))
	fillUsage(usageLines(entitlements.Progress))
	providers := make([]string, 0, 3)
	for _, provider := range []string{"ChatGPT", "Gemini", "Grok"} {
		if entitlements.ProviderKeys[provider] != "" {
			providers = append(providers, provider)
		}
	}
	fillCombo(providerCombo, providers)
	if len(providers) > 0 {
		procSendMessageW.Call(providerCombo, cbSetCurSel, 0, 0)
		loadSelectedProviderModels()
	}
}

func logoutAccount() {
	loggedIn = false
	accountProviderKeys = make(map[string]string)
	accountModels = make(map[string][]string)
	setText(accountEdit, "")
	setText(passwordEdit, "")
	showLoginPlaceholder(accountEdit)
	showLoginPlaceholder(passwordEdit)
	setText(memberInfoLabel, "未登录")
	fillCombo(providerCombo, nil)
	fillCombo(accountModelCombo, nil)
	fillUsage([3]string{"每日用量    登录后显示", "每周用量    登录后显示", "每月用量    登录后显示"})
	setStatus("已退出云桥账号。")
}

func loginFieldValue(handle uintptr) string {
	if handle == accountEdit && accountPlaceholderActive {
		return ""
	}
	if handle == passwordEdit && passwordPlaceholderActive {
		return ""
	}
	return getText(handle)
}

func showLoginPlaceholder(handle uintptr) {
	if handle == accountEdit {
		if getText(handle) == "" {
			accountPlaceholderActive = true
			setText(handle, accountPlaceholder)
		}
		return
	}
	if handle == passwordEdit && getText(handle) == "" {
		passwordPlaceholderActive = true
		procSendMessageW.Call(handle, emSetPasswordChar, 0, 0)
		setText(handle, passwordPlaceholder)
	}
}

func handleLoginPlaceholderFocus(handle uintptr, focused, blurred bool) {
	if focused {
		if handle == accountEdit && accountPlaceholderActive {
			accountPlaceholderActive = false
			setText(handle, "")
		}
		if handle == passwordEdit && passwordPlaceholderActive {
			passwordPlaceholderActive = false
			setText(handle, "")
			procSendMessageW.Call(handle, emSetPasswordChar, uintptr('*'), 0)
		}
		return
	}
	if blurred {
		showLoginPlaceholder(handle)
	}
}

func fillCombo(handle uintptr, values []string) {
	procSendMessageW.Call(handle, cbResetContent, 0, 0)
	for _, value := range values {
		procSendMessageW.Call(handle, cbAddString, 0, uintptr(unsafe.Pointer(utf16(value))))
	}
}

func fillUsage(lines [3]string) {
	procSendMessageW.Call(usageList, lbReset, 0, 0)
	for _, line := range lines {
		procSendMessageW.Call(usageList, lbAddString, 0, uintptr(unsafe.Pointer(utf16(line))))
	}
}

func loadSelectedProviderModels() {
	if !loggedIn {
		return
	}
	provider := getText(providerCombo)
	key := accountProviderKeys[provider]
	if key == "" {
		return
	}
	if cached := accountModels[provider]; len(cached) > 0 {
		currentModels = append([]string(nil), cached...)
		fillCombo(accountModelCombo, cached)
		procSendMessageW.Call(accountModelCombo, cbSetCurSel, 0, 0)
		return
	}
	setStatus("正在获取" + provider + "分组模型…")
	go func(provider, key string) {
		models, err := fetchModels(defaultBaseURL, key)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Models: models, Provider: provider, Status: fmt.Sprintf("%s 已获取 %d 个模型。", provider, len(models)), Done: true})
	}(provider, key)
}

func startFetch() {
	baseURL := getText(baseEdit)
	apiKey := getText(keyEdit)
	setBusy(true, "正在连接模型接口…")
	go func() {
		models, err := fetchModels(baseURL, apiKey)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{
			Status: fmt.Sprintf("成功获取 %d 个模型。", len(models)),
			Models: models,
			Done:   true,
		})
	}()
}

func startSaveAndLaunch() {
	baseURL := getText(baseEdit)
	apiKey := getText(keyEdit)
	if accountMode {
		if !loggedIn {
			messageBox("请先登录云桥账号。", 0x30)
			return
		}
		provider := getText(providerCombo)
		apiKey = accountProviderKeys[provider]
		baseURL = defaultBaseURL
		currentModels = append([]string(nil), accountModels[provider]...)
		if apiKey == "" || len(currentModels) == 0 {
			messageBox("请等待当前上游的模型加载完成。", 0x30)
			return
		}
	}
	setBusy(true, "正在保存配置…")
	go func() {
		models := append([]string(nil), currentModels...)
		var err error
		if len(models) == 0 {
			postUpdate(uiUpdate{Status: "正在自动获取模型…"})
			models, err = fetchModels(baseURL, apiKey)
			if err != nil {
				postUpdate(uiUpdate{Error: err, Done: true})
				return
			}
		}
		baseURL, err = normalizeBaseURL(baseURL)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		defaultModel := chooseDefaultModel(models, currentConfig.DefaultModel)
		if err := saveApplicationConfig(baseURL, apiKey, models, defaultModel); err != nil {
			postUpdate(uiUpdate{Error: fmt.Errorf("保存程序配置失败：%w", err), Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "正在启动本机 API 代理…", Models: models})
		proxy, err := replaceAPIProxy(baseURL, apiKey)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "正在写入官方 Codex 供应商配置…"})
		if err := writeCodexProviderConfig(codexProxyBase, defaultModel); err != nil {
			stopAPIProxy(proxy)
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "正在启动官方 Codex…"})
		install, err := findCodexInstallation()
		if err != nil {
			stopAPIProxy(proxy)
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		if err := launchCodex(install); err != nil {
			stopAPIProxy(proxy)
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		menuResult := make(chan error, 1)
		go func() {
			menuResult <- localizeNativeMenu(inspectorPort)
		}()
		err = injectIntoCodex(cdpPort, models, defaultModel, func(status string) {
			postUpdate(uiUpdate{Status: status})
		})
		if err != nil {
			stopAPIProxy(proxy)
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		go syncProxyImagesToCodex(cdpPort, proxy)
		go maintainCodexInjection(cdpPort, models, defaultModel, proxy.done, diagnosticLog)
		menuErr := <-menuResult
		if menuErr != nil {
			diagnosticLog("menu.localization_failed", menuErr.Error())
			postUpdate(uiUpdate{
				Status: fmt.Sprintf("模型与图片桥接成功，但原生菜单汉化失败；详情见 %s", diagnosticLogPath()),
				Models: models,
				Done:   true,
			})
			return
		}
		diagnosticLog("launch.ready", fmt.Sprintf("models=%d", len(models)))
		postUpdate(uiUpdate{
			Status: fmt.Sprintf("启动成功：中文界面、图片桥接和 %d 个模型已就绪。请保持 Bridge 运行。", len(models)),
			Models: models,
			Done:   true,
		})
	}()
}

func postUpdate(update uiUpdate) {
	updateMutex.Lock()
	if update.Status != "" {
		pendingUpdate.Status = update.Status
	}
	if update.Models != nil {
		pendingUpdate.Models = append([]string(nil), update.Models...)
	}
	if update.Provider != "" {
		pendingUpdate.Provider = update.Provider
	}
	if update.Account != nil {
		copy := *update.Account
		pendingUpdate.Account = &copy
	}
	if update.Error != nil {
		pendingUpdate.Error = update.Error
	}
	if update.SetProgress {
		setUpdateProgress(update.Progress)
	}
	if update.Done {
		pendingUpdate.Done = true
	}
	updateMutex.Unlock()
	procPostMessageW.Call(mainWindow, wmAppResult, 0, 0)
}

func setUpdateProgress(value int) {
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	procSendMessageW.Call(progressBar, pbmSetPos, uintptr(value), 0)
	setText(progressLabel, fmt.Sprintf("更新进度  %d%%", value))
}

func applyPendingUpdate() {
	updateMutex.Lock()
	update := pendingUpdate
	pendingUpdate = uiUpdate{}
	updateMutex.Unlock()
	if update.Status != "" {
		setStatus(update.Status)
	}
	if update.Account != nil {
		applyAccountEntitlements(*update.Account)
	}
	if update.Models != nil {
		currentModels = append([]string(nil), update.Models...)
		if update.Provider != "" {
			accountModels[update.Provider] = append([]string(nil), update.Models...)
			if getText(providerCombo) == update.Provider {
				fillCombo(accountModelCombo, update.Models)
				procSendMessageW.Call(accountModelCombo, cbSetCurSel, 0, 0)
			}
		} else {
			fillModels(currentModels)
		}
	}
	if update.Error != nil {
		diagnosticLog("operation.failed", update.Error.Error())
		message := update.Error.Error() + "\n\n诊断日志：" + diagnosticLogPath()
		setStatus("操作失败：" + statusSummary(update.Error.Error(), 96))
		messageBox(message, 0x10)
	}
	if update.Done {
		setBusy(false, "")
		setAccountBusy(false)
	}
}

func setBusy(busy bool, status string) {
	enabled := uintptr(1)
	if busy {
		enabled = 0
	}
	procEnableWindow.Call(fetchButton, enabled)
	procEnableWindow.Call(launchButton, enabled)
	procEnableWindow.Call(updateButton, enabled)
	if status != "" {
		setStatus(status)
	}
}

func setStatus(value string) {
	value = statusSummary(value, 120)
	if value == "" || value == lastStatus {
		return
	}
	lastStatus = value
	setText(statusLabel, value)
}

func fillModels(models []string) {
	procSendMessageW.Call(modelList, lbReset, 0, 0)
	for _, model := range models {
		procSendMessageW.Call(modelList, lbAddString, 0, uintptr(unsafe.Pointer(utf16(model))))
	}
	if len(models) > 0 {
		procSendMessageW.Call(modelList, lbSetCurSel, 0, 0)
	}
}

func setText(handle uintptr, value string) {
	procSetWindowTextW.Call(handle, uintptr(unsafe.Pointer(utf16(value))))
}

func getText(handle uintptr) string {
	length, _, _ := procGetWindowTextLen.Call(handle)
	buffer := make([]uint16, int(length)+1)
	procGetWindowTextW.Call(handle, uintptr(unsafe.Pointer(&buffer[0])), length+1)
	return syscall.UTF16ToString(buffer)
}

func messageBox(text string, flags uintptr) {
	procMessageBoxW.Call(mainWindow, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(appTitle))), flags)
}

func messageBoxResult(text string, flags uintptr) uintptr {
	result, _, _ := procMessageBoxW.Call(mainWindow, uintptr(unsafe.Pointer(utf16(text))), uintptr(unsafe.Pointer(utf16(appTitle))), flags)
	return result
}

func utf16(value string) *uint16 {
	pointer, _ := syscall.UTF16PtrFromString(value)
	return pointer
}

func applicationDirectory() string {
	root := os.Getenv("LOCALAPPDATA")
	if root == "" {
		root = os.TempDir()
	}
	return filepath.Join(root, "YunqiaoCodexBridge")
}

func loadSavedConfiguration() {
	body, err := os.ReadFile(filepath.Join(applicationDirectory(), "config.json"))
	if err == nil {
		_ = json.Unmarshal(body, &currentConfig)
	}
	if currentConfig.BaseURL == "" {
		currentConfig.BaseURL = defaultBaseURL
	}
}

func saveApplicationConfig(baseURL, apiKey string, models []string, defaultModel string) error {
	encrypted, err := encryptText(apiKey)
	if err != nil {
		return err
	}
	config := appConfig{
		BaseURL:      baseURL,
		EncryptedKey: encrypted,
		Models:       append([]string(nil), models...),
		DefaultModel: defaultModel,
	}
	body, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(applicationDirectory(), 0700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(applicationDirectory(), "config.json"), append(body, '\n'), 0600); err != nil {
		return err
	}
	currentConfig = config
	return nil
}

func encryptText(value string) (string, error) {
	data := []byte(value)
	if len(data) == 0 {
		return "", nil
	}
	input := dataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output dataBlob
	result, _, callErr := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&output)),
	)
	if result == 0 {
		return "", callErr
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.Data)))
	protected := unsafe.Slice(output.Data, int(output.Size))
	return base64.StdEncoding.EncodeToString(protected), nil
}

func decryptText(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	input := dataBlob{Size: uint32(len(data)), Data: &data[0]}
	var output dataBlob
	result, _, callErr := procCryptUnprotect.Call(
		uintptr(unsafe.Pointer(&input)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&output)),
	)
	if result == 0 {
		return "", callErr
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(output.Data)))
	plain := unsafe.Slice(output.Data, int(output.Size))
	return string(plain), nil
}

func writeCodexProviderConfig(baseURL, model string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	codexHome := filepath.Join(home, ".codex")
	configPath := filepath.Join(codexHome, "config.toml")
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return err
	}
	existing, _ := os.ReadFile(configPath)
	if len(existing) > 0 {
		backupDir := filepath.Join(applicationDirectory(), "backups")
		if err := os.MkdirAll(backupDir, 0700); err != nil {
			return err
		}
		backup := filepath.Join(backupDir, "config-"+time.Now().Format("20060102-150405")+".toml")
		if err := os.WriteFile(backup, existing, 0600); err != nil {
			return fmt.Errorf("备份 config.toml 失败：%w", err)
		}
	}
	updated := updateCodexConfig(string(existing), baseURL, model)
	if err := os.WriteFile(configPath, []byte(updated), 0600); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", configPath, err)
	}
	return nil
}

type codexInstallation struct {
	Executable string
	AUMID      string
}

func findCodexInstallation() (codexInstallation, error) {
	local := os.Getenv("LOCALAPPDATA")
	for _, path := range []string{
		filepath.Join(local, "OpenAI", "Codex", "bin", "Codex.exe"),
		filepath.Join(local, "OpenAI", "Codex", "bin", "ChatGPT.exe"),
		filepath.Join(local, "Programs", "OpenAI", "Codex", "Codex.exe"),
		filepath.Join(local, "Programs", "OpenAI", "Codex", "ChatGPT.exe"),
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return codexInstallation{Executable: path}, nil
		}
	}

	script := `$p = Get-AppxPackage | Where-Object { $_.Name -in @('OpenAI.Codex','OpenAI.CodexBeta') } | Sort-Object Version -Descending | Select-Object -First 1; if ($p) { $m = Get-AppxPackageManifest $p; $e = [string]$m.Package.Applications.Application.Executable; Write-Output ($p.InstallLocation + '|' + $p.PackageFamilyName + '|' + $e) }`
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(output)), "|")
		if len(parts) == 3 && parts[1] != "" {
			executable := ""
			names := []string{parts[2], "Codex.exe", "ChatGPT.exe", "codex.exe"}
			for _, name := range names {
				if strings.TrimSpace(name) == "" {
					continue
				}
				candidate := filepath.Join(parts[0], name)
				if _, statErr := os.Stat(candidate); statErr == nil {
					executable = candidate
					break
				}
			}
			if executable == "" && strings.TrimSpace(parts[2]) != "" {
				executable = filepath.Join(parts[0], parts[2])
			}
			return codexInstallation{Executable: executable, AUMID: parts[1] + "!App"}, nil
		}
	}
	return codexInstallation{}, errors.New("未找到官方 Codex Windows 客户端，请先安装并至少直接启动一次")
}

func launchCodex(install codexInstallation) error {
	workingDirectory, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法确定 Windows 用户目录：%w", err)
	}
	if strings.TrimSpace(workingDirectory) == "" {
		return errors.New("Windows 用户目录为空")
	}
	info, err := os.Stat(workingDirectory)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("Windows 用户目录不可用：%s", workingDirectory)
	}

	processName := filepath.Base(install.Executable)
	if processName == "" {
		processName = "Codex.exe"
	}
	kill := exec.Command("taskkill.exe", "/F", "/T", "/IM", processName)
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = kill.Run()
	time.Sleep(800 * time.Millisecond)

	previousDirectory, getDirectoryErr := os.Getwd()
	if err := os.Chdir(workingDirectory); err != nil {
		return fmt.Errorf("切换 Codex 默认工作目录失败：%w", err)
	}
	if getDirectoryErr == nil && previousDirectory != "" {
		defer func() { _ = os.Chdir(previousDirectory) }()
	}
	diagnosticLog("launch.working_directory", workingDirectory)

	arguments := fmt.Sprintf("--remote-debugging-port=%d --remote-allow-origins=http://127.0.0.1:%d --inspect=127.0.0.1:%d --lang=zh-CN", cdpPort, cdpPort, inspectorPort)
	if install.AUMID != "" {
		if _, err := activateApplication(install.AUMID, arguments); err == nil {
			return nil
		}
	}
	if install.Executable == "" {
		return errors.New("已找到 Codex 应用包，但无法激活其调试模式")
	}
	command := exec.Command(install.Executable,
		fmt.Sprintf("--remote-debugging-port=%d", cdpPort),
		fmt.Sprintf("--remote-allow-origins=http://127.0.0.1:%d", cdpPort),
		fmt.Sprintf("--inspect=127.0.0.1:%d", inspectorPort),
		"--lang=zh-CN",
	)
	command.Dir = workingDirectory
	command.Env = append(os.Environ(), "LANG=zh_CN.UTF-8")
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动官方 Codex 失败：%w", err)
	}
	return nil
}

func replaceAPIProxy(baseURL, apiKey string) (*apiProxy, error) {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	if activeProxy != nil {
		activeProxy.close()
		activeProxy = nil
		time.Sleep(150 * time.Millisecond)
	}
	proxy, err := startAPIProxy(baseURL, apiKey, diagnosticLog)
	if err != nil {
		return nil, err
	}
	activeProxy = proxy
	return proxy, nil
}

func hasActiveProxy() bool {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	return activeProxy != nil
}

func stopActiveProxy() {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	if activeProxy != nil {
		activeProxy.close()
		activeProxy = nil
	}
}

func stopAPIProxy(proxy *apiProxy) {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	if proxy != nil {
		proxy.close()
	}
	if activeProxy == proxy {
		activeProxy = nil
	}
}

func diagnosticLogPath() string {
	return filepath.Join(applicationDirectory(), "bridge.log")
}

func diagnosticLog(event, detail string) {
	_ = os.MkdirAll(applicationDirectory(), 0700)
	file, err := os.OpenFile(diagnosticLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer file.Close()
	detail = strings.ReplaceAll(detail, "\r", " ")
	detail = strings.ReplaceAll(detail, "\n", " ")
	_, _ = fmt.Fprintf(file, "%s [%s] %s\n", time.Now().Format(time.RFC3339), event, detail)
}

func activateApplication(aumid, arguments string) (uint32, error) {
	const (
		coinitApartmentThreaded = 0x2
		clsctxLocalServer       = 0x4
	)
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	initialized := int32(hr) >= 0
	if initialized {
		defer procCoUninitialize.Call()
	}

	clsid := guid{0x45BA127D, 0x10A8, 0x46EA, [8]byte{0x8A, 0xB7, 0x56, 0xEA, 0x90, 0x78, 0x94, 0x3C}}
	iid := guid{0x2E941141, 0x7F97, 0x4756, [8]byte{0xBA, 0x1D, 0x9D, 0xEC, 0xDE, 0x89, 0x4A, 0x3D}}
	var manager *activationManager
	hr, _, _ = procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsid)), 0, clsctxLocalServer,
		uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&manager)),
	)
	if int32(hr) < 0 || manager == nil {
		return 0, fmt.Errorf("创建应用激活器失败：0x%08X", uint32(hr))
	}
	defer syscall.SyscallN(manager.VTable.Release, uintptr(unsafe.Pointer(manager)))

	var pid uint32
	hr, _, _ = syscall.SyscallN(
		manager.VTable.ActivateApplication,
		uintptr(unsafe.Pointer(manager)),
		uintptr(unsafe.Pointer(utf16(aumid))),
		uintptr(unsafe.Pointer(utf16(arguments))),
		0,
		uintptr(unsafe.Pointer(&pid)),
	)
	if int32(hr) < 0 {
		return 0, fmt.Errorf("激活官方 Codex 失败：0x%08X", uint32(hr))
	}
	return pid, nil
}
