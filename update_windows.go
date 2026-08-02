//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	bridgeReleaseAPI = "https://api.github.com/repos/ll5199/yunqiao-codex-bridge/releases/latest"
	bridgeExeAsset   = "YunqiaoCodexBridge.exe"
	bridgeHashAsset  = "YunqiaoCodexBridge.exe.sha256"
)

type githubReleaseAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

type githubRelease struct {
	TagName string               `json:"tag_name"`
	Name    string               `json:"name"`
	Body    string               `json:"body"`
	Assets  []githubReleaseAsset `json:"assets"`
}

func startBridgeUpdate() {
	setBusy(true, "正在检查 Bridge 更新…")
	go func() {
		release, err := fetchLatestBridgeRelease()
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
		if compareVersions(latest, appVersion) <= 0 {
			postUpdate(uiUpdate{Status: fmt.Sprintf("当前 Bridge v%s 已是最新版。", appVersion), Done: true})
			return
		}

		notes := strings.TrimSpace(release.Body)
		if len(notes) > 700 {
			notes = notes[:700] + "…"
		}
		question := fmt.Sprintf("检测到 Bridge v%s（当前 v%s）。\n\n%s\n\n是否下载、安装并重启 Bridge？", latest, appVersion, notes)
		if messageBoxResult(question, 0x24) != 6 {
			postUpdate(uiUpdate{Status: "已取消 Bridge 更新。", Done: true})
			return
		}

		postUpdate(uiUpdate{Status: fmt.Sprintf("正在下载 Bridge v%s…", latest)})
		staged, expectedHash, err := downloadBridgeRelease(release, latest)
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		if err := verifyFileSHA256(staged, expectedHash); err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		if err := scheduleBridgeReplacement(staged, latest); err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		diagnosticLog("update.ready", fmt.Sprintf("from=%s to=%s sha256=%s", appVersion, latest, expectedHash))
		postUpdate(uiUpdate{Status: "更新已校验，Bridge 即将重启…"})
		time.Sleep(350 * time.Millisecond)
		stopActiveProxy()
		procPostMessageW.Call(mainWindow, wmClose, 0, 0)
	}()
}

func fetchLatestBridgeRelease() (githubRelease, error) {
	request, err := http.NewRequest(http.MethodGet, bridgeReleaseAPI, nil)
	if err != nil {
		return githubRelease{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	client := &http.Client{Timeout: 20 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return githubRelease{}, fmt.Errorf("连接 Bridge 更新服务失败：%w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return githubRelease{}, err
	}
	if response.StatusCode == http.StatusNotFound {
		return githubRelease{}, errors.New("Bridge 更新仓库尚未发布 Release")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return githubRelease{}, fmt.Errorf("Bridge 更新服务返回 HTTP %d", response.StatusCode)
	}
	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubRelease{}, fmt.Errorf("更新信息格式无效：%w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return githubRelease{}, errors.New("最新 Release 没有版本号")
	}
	return release, nil
}

func downloadBridgeRelease(release githubRelease, version string) (string, string, error) {
	var executableAsset, hashAsset *githubReleaseAsset
	for index := range release.Assets {
		switch release.Assets[index].Name {
		case bridgeExeAsset:
			executableAsset = &release.Assets[index]
		case bridgeHashAsset:
			hashAsset = &release.Assets[index]
		}
	}
	if executableAsset == nil || hashAsset == nil {
		return "", "", errors.New("Release 缺少 Bridge EXE 或 SHA-256 校验文件")
	}
	if executableAsset.Size <= 0 || executableAsset.Size > 100<<20 {
		return "", "", errors.New("Release 中的 Bridge EXE 大小异常")
	}
	updates := filepath.Join(applicationDirectory(), "updates")
	if err := os.MkdirAll(updates, 0700); err != nil {
		return "", "", err
	}
	executablePath := filepath.Join(updates, "YunqiaoCodexBridge-v"+version+".exe")
	if err := downloadReleaseAsset(executableAsset.DownloadURL, executablePath, 100<<20); err != nil {
		return "", "", fmt.Errorf("下载 Bridge 失败：%w", err)
	}
	hashBody, err := downloadReleaseAssetBytes(hashAsset.DownloadURL, 16<<10)
	if err != nil {
		return "", "", fmt.Errorf("下载校验文件失败：%w", err)
	}
	hashFields := strings.Fields(string(hashBody))
	if len(hashFields) == 0 {
		return "", "", errors.New("Release SHA-256 文件为空")
	}
	expectedHash := strings.ToLower(hashFields[0])
	if len(expectedHash) != 64 {
		return "", "", errors.New("Release SHA-256 格式无效")
	}
	if _, err := hex.DecodeString(expectedHash); err != nil {
		return "", "", errors.New("Release SHA-256 不是有效十六进制")
	}
	return executablePath, expectedHash, nil
}

func downloadReleaseAsset(rawURL, destination string, limit int64) error {
	body, err := downloadReleaseAssetBytes(rawURL, limit)
	if err != nil {
		return err
	}
	return os.WriteFile(destination, body, 0600)
}

func downloadReleaseAssetBytes(rawURL string, limit int64) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" {
		return nil, errors.New("Release 下载地址不安全")
	}
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	client := &http.Client{Timeout: 90 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("下载文件超过大小限制")
	}
	return body, nil
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("Bridge 更新校验失败：期望 %s，实际 %s", expected, actual)
	}
	return nil
}

func scheduleBridgeReplacement(staged, version string) error {
	current, err := os.Executable()
	if err != nil {
		return err
	}
	current, err = filepath.Abs(current)
	if err != nil {
		return err
	}
	updates := filepath.Join(applicationDirectory(), "updates")
	backup := filepath.Join(updates, "YunqiaoCodexBridge-v"+appVersion+".backup.exe")
	scriptPath := filepath.Join(updates, "apply-update-v"+version+".ps1")
	script := `param([int]$ProcessId,[string]$Current,[string]$Staged,[string]$Backup)
$ErrorActionPreference = 'Stop'
try { Wait-Process -Id $ProcessId -Timeout 30 -ErrorAction SilentlyContinue } catch {}
try {
  Copy-Item -LiteralPath $Current -Destination $Backup -Force
  Copy-Item -LiteralPath $Staged -Destination $Current -Force
  Start-Process -FilePath $Current
  Remove-Item -LiteralPath $Staged -Force -ErrorAction SilentlyContinue
} catch {
  if (Test-Path -LiteralPath $Backup) { Copy-Item -LiteralPath $Backup -Destination $Current -Force }
  try { Start-Process -FilePath $Current } catch {}
}
Remove-Item -LiteralPath $PSCommandPath -Force -ErrorAction SilentlyContinue
`
	if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
		return err
	}
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-ProcessId", strconv.Itoa(os.Getpid()),
		"-Current", current,
		"-Staged", staged,
		"-Backup", backup,
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 Bridge 更新程序失败：%w", err)
	}
	return nil
}
