//go:build windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Only the registered application identity is retained. An update may move the
// package directory, and WindowsApps executables must not be launched directly.
type codexInstallation struct { AUMID string }

var codexApplicationID = regexp.MustCompile(`^OpenAI\.Codex(?:Beta)?_[A-Za-z0-9]+![A-Za-z0-9._-]+$`)

// Select the UI entry, never the command runner. No access to WindowsApps is
// needed to resolve the identity from the registered package manifest.
const codexAppManifestSelectionScript = `$ui = @($m.Package.Applications.Application) | Where-Object { ([string]$_.Executable) -match '(?i)(^|[\\/])(Codex|ChatGPT)\.exe$' } | Select-Object -First 1; if ($ui) { [PSCustomObject]@{ PackageFamilyName=[string]$p.PackageFamilyName; AppID=[string]$ui.Id } | ConvertTo-Json -Compress }`

const codexApplicationDiscoveryScript = `$ErrorActionPreference='Stop'; [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false); $found=$false; $packages=@(Get-AppxPackage | Where-Object { $_.Name -in @('OpenAI.Codex','OpenAI.CodexBeta') } | Sort-Object { [version]$_.Version } -Descending); foreach ($p in $packages) { try { $m=Get-AppxPackageManifest $p; ` + codexAppManifestSelectionScript + `; if ($ui) { $found=$true; break } } catch { } }; if (-not $found) { $entry=Get-StartApps | Where-Object { $_.AppID -match '^OpenAI\.Codex(Beta)?_[A-Za-z0-9]+![A-Za-z0-9._-]+$' } | Sort-Object AppID | Select-Object -First 1; if ($entry) { [PSCustomObject]@{ AUMID=[string]$entry.AppID } | ConvertTo-Json -Compress } }`

func findCodexInstallation() (codexInstallation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", codexApplicationDiscoveryScript)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := command.Output()
	if err != nil {
		diagnosticLog("launch.identity_query_failed", err.Error())
		return codexInstallation{}, fmt.Errorf("读取官方 Codex 应用入口失败：%w", err)
	}
	if len(bytes.TrimSpace(output)) == 0 {
		return codexInstallation{}, errors.New("未找到官方 Codex 的 Windows 应用入口，请先安装并直接打开一次官方客户端")
	}
	return parseCodexPackageInstallation(output)
}

func parseCodexPackageInstallation(output []byte) (codexInstallation, error) {
	var pkg struct { AUMID, PackageFamilyName, AppID string }
	if err := json.Unmarshal(bytes.TrimPrefix(bytes.TrimSpace(output), []byte{0xef, 0xbb, 0xbf}), &pkg); err != nil {
		return codexInstallation{}, fmt.Errorf("Codex 应用入口信息解析失败：%w", err)
	}
	aumid := pkg.AUMID
	if aumid == "" { aumid = pkg.PackageFamilyName + "!" + pkg.AppID }
	if !codexApplicationID.MatchString(aumid) {
		return codexInstallation{}, errors.New("Codex 应用包缺少有效的官方 AppUserModelID")
	}
	return codexInstallation{AUMID: aumid}, nil
}

func launchCodex(install codexInstallation) error {
	arguments := fmt.Sprintf("--remote-debugging-port=%d --remote-allow-origins=http://127.0.0.1:%d --inspect=127.0.0.1:%d --lang=zh-CN", cdpPort, cdpPort, inspectorPort)
	return launchCodexApplication(install, arguments)
}

func launchCodexNative(install codexInstallation) error {
	return launchCodexApplication(install, "")
}

// Native and routed modes share activation and process confirmation. CODEX_HOME
// does not affect the launch mechanism, and a missing CDP port never triggers an
// executable fallback.
func launchCodexApplication(install codexInstallation, arguments string) error {
	if !codexApplicationID.MatchString(install.AUMID) {
		return errors.New("无法启动：官方 Codex 应用入口无效")
	}
	if err := stopTrackedCodexForRestart(); err != nil { return err }
	diagnosticLog("launch.application_entry", install.AUMID)
	pid, err := activateAndConfirmCodex(install.AUMID, arguments, activateApplication, officialCodexProcessAlive, 10*time.Second)
	if err != nil {
		rememberCodexPID(0)
		diagnosticLog("launch.activation_failed", err.Error())
		return err
	}
	rememberCodexPID(pid)
	if !codexIsRunning() {
		rememberCodexPID(0)
		return errors.New("官方 Codex 进程启动后已退出，请重试或导出诊断日志")
	}
	diagnosticLog("launch.process_confirmed", fmt.Sprintf("aumid=%s pid=%d", install.AUMID, pid))
	return nil
}

func activateAndConfirmCodex(aumid, arguments string, activate func(string, string) (uint32, error), alive func(uint32, string) bool, timeout time.Duration) (uint32, error) {
	pid, err := activate(aumid, arguments)
	if err != nil { return 0, fmt.Errorf("通过 Windows 应用入口启动官方 Codex 失败：%w", err) }
	if pid == 0 { return 0, errors.New("Windows 未返回官方 Codex 进程 ID") }
	deadline := time.Now().Add(timeout)
	for {
		if alive(pid, aumid) { return pid, nil }
		if !time.Now().Before(deadline) { break }
		time.Sleep(100*time.Millisecond)
	}
	return 0, errors.New("未检测到官方 Codex 进程，启动未确认，请重试或导出诊断日志")
}

var (
	codexProcessMu sync.Mutex
	codexProcessPID uint32
	codexProcessHandle syscall.Handle
	openCodexProcess = kernel32.NewProc("OpenProcess")
	waitCodexProcess = kernel32.NewProc("WaitForSingleObject")
	closeCodexProcess = kernel32.NewProc("CloseHandle")
	queryCodexImage = kernel32.NewProc("QueryFullProcessImageNameW")
	queryCodexPackage = kernel32.NewProc("GetPackageFamilyName")
)

func openProcessForTracking(pid uint32) syscall.Handle {
	if pid == 0 { return 0 }
	// SYNCHRONIZE | PROCESS_QUERY_LIMITED_INFORMATION
	h, _, _ := openCodexProcess.Call(0x100000|0x1000, 0, uintptr(pid))
	return syscall.Handle(h)
}

func processHandleAlive(handle syscall.Handle) bool {
	if handle == 0 { return false }
	result, _, _ := waitCodexProcess.Call(uintptr(handle), 0)
	return result == 258 // WAIT_TIMEOUT: the process has not exited.
}

func officialCodexProcessAlive(pid uint32, aumid string) bool {
	h := openProcessForTracking(pid)
	if h == 0 { return false }
	defer closeCodexProcess.Call(uintptr(h))
	if !processHandleAlive(h) { return false }
	var image [32768]uint16
	length := uint32(len(image))
	ok, _, _ := queryCodexImage.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&image[0])), uintptr(unsafe.Pointer(&length)))
	if ok == 0 { return false }
	path := strings.ToLower(syscall.UTF16ToString(image[:length]))
	if !strings.HasSuffix(path, `\chatgpt.exe`) && !strings.HasSuffix(path, `\codex.exe`) { return false }
	var family [256]uint16
	length = uint32(len(family))
	result, _, _ := queryCodexPackage.Call(uintptr(h), uintptr(unsafe.Pointer(&length)), uintptr(unsafe.Pointer(&family[0])))
	return result == 0 && strings.EqualFold(syscall.UTF16ToString(family[:]), strings.SplitN(aumid, "!", 2)[0])
}

// Keep the process handle until exit, so PID reuse cannot leave the button
// disabled because an unrelated process inherited the same numeric ID.
func rememberCodexPID(pid uint32) {
	codexProcessMu.Lock()
	defer codexProcessMu.Unlock()
	if codexProcessHandle != 0 { closeCodexProcess.Call(uintptr(codexProcessHandle)) }
	codexProcessPID = pid
	codexProcessHandle = openProcessForTracking(pid)
}

func codexIsRunning() bool {
	codexProcessMu.Lock()
	defer codexProcessMu.Unlock()
	if processHandleAlive(codexProcessHandle) { return true }
	if codexProcessHandle != 0 { closeCodexProcess.Call(uintptr(codexProcessHandle)) }
	codexProcessHandle, codexProcessPID = 0, 0
	return false
}

func stopTrackedCodexForRestart() error {
	if !codexIsRunning() { return nil }
	codexProcessMu.Lock()
	pid := codexProcessPID
	codexProcessMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "taskkill.exe", "/F", "/T", "/PID", fmt.Sprint(pid))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Run(); err != nil && codexIsRunning() { return fmt.Errorf("重启前关闭官方 Codex 进程失败：%w", err) }
	deadline := time.Now().Add(5*time.Second)
	for codexIsRunning() {
		if !time.Now().Before(deadline) { return errors.New("官方 Codex 尚未退出，请稍后重试") }
		time.Sleep(100*time.Millisecond)
	}
	return nil
}

