//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	bridgeManifestURL = "https://index.velyn65.com/download/latest.json"
)

var bridgeUpdateAllowedHosts = []string{"index.velyn65.com"}

func startBridgeUpdate() {
	setBusy(true, "正在检查 Bridge 更新…")
	postUpdate(uiUpdate{Progress: 0, SetProgress: true})
	go func() {
		manifest, err := fetchBridgeUpdateManifest()
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		latest := manifest.Version
		if compareVersions(latest, appVersion) <= 0 {
			postUpdate(uiUpdate{Status: fmt.Sprintf("当前 Bridge v%s 已是最新版。", appVersion), Done: true})
			return
		}

		notes := manifest.Notes
		if len(notes) > 700 {
			notes = notes[:700] + "…"
		}
		question := fmt.Sprintf("检测到 Bridge v%s（当前 v%s）。\n\n%s\n\n是否下载、安装并重启 Bridge？", latest, appVersion, notes)
		if messageBoxResult(question, 0x24) != 6 {
			postUpdate(uiUpdate{Status: "已取消 Bridge 更新。", Done: true})
			return
		}

		postUpdate(uiUpdate{Status: fmt.Sprintf("正在下载 Bridge v%s…", latest), Progress: 5, SetProgress: true})
		staged, err := downloadBridgeUpdate(manifest, func(percent int) {
			postUpdate(uiUpdate{Status: fmt.Sprintf("正在下载 Bridge v%s… %d%%", latest, percent), Progress: 5 + percent*65/100, SetProgress: true})
		})
		if err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "正在校验更新文件…", Progress: 78, SetProgress: true})
		if err := verifyFileSHA256(staged, manifest.SHA256); err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		postUpdate(uiUpdate{Status: "正在准备安装更新…", Progress: 88, SetProgress: true})
		if err := scheduleBridgeReplacement(staged, latest); err != nil {
			postUpdate(uiUpdate{Error: err, Done: true})
			return
		}
		diagnosticLog("update.ready", fmt.Sprintf("source=YunqiaoServer from=%s to=%s sha256=%s", appVersion, latest, manifest.SHA256))
		postUpdate(uiUpdate{Status: "更新已下载并校验，正在关闭旧版并安装…", Progress: 95, SetProgress: true})
		time.Sleep(350 * time.Millisecond)
		stopActiveProxy()
		procPostMessageW.Call(mainWindow, wmClose, 0, 0)
	}()
}

func fetchBridgeUpdateManifest() (bridgeUpdateManifest, error) {
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}
		manifest, err := fetchBridgeUpdateManifestOnce()
		if err == nil {
			return manifest, nil
		}
		lastErr = err
	}
	return bridgeUpdateManifest{}, fmt.Errorf("连续 3 次连接更新服务失败：%w", lastErr)
}

func fetchBridgeUpdateManifestOnce() (bridgeUpdateManifest, error) {
	request, err := http.NewRequest(http.MethodGet, bridgeManifestURL, nil)
	if err != nil {
		return bridgeUpdateManifest{}, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	client := &http.Client{Timeout: 45 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return bridgeUpdateManifest{}, fmt.Errorf("连接云桥更新服务失败：%w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return bridgeUpdateManifest{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return bridgeUpdateManifest{}, fmt.Errorf("云桥更新服务返回 HTTP %d", response.StatusCode)
	}
	return parseBridgeUpdateManifest(body)
}

func downloadBridgeUpdate(manifest bridgeUpdateManifest, progress func(int)) (string, error) {
	parsed, err := url.Parse(manifest.DownloadURL)
	if err != nil || parsed.Scheme != "https" || !hostAllowed(parsed.Hostname(), bridgeUpdateAllowedHosts) {
		return "", errors.New("更新清单中的下载地址不安全")
	}
	updates := filepath.Join(applicationDirectory(), "updates")
	if err := os.MkdirAll(updates, 0700); err != nil {
		return "", err
	}
	executablePath := filepath.Join(updates, "YunqiaoCodexBridge-v"+manifest.Version+".exe")
	if err := downloadReleaseAsset(manifest.DownloadURL, executablePath, maxBridgeUpdateBytes, bridgeUpdateAllowedHosts, progress); err != nil {
		return "", fmt.Errorf("下载 Bridge 失败：%w", err)
	}
	return executablePath, nil
}

func downloadReleaseAsset(rawURL, destination string, limit int64, allowedHosts []string, progress func(int)) error {
	partialPath := destination + ".part"
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}
		if err := downloadReleaseAssetOnce(rawURL, partialPath, limit, allowedHosts, progress); err != nil {
			lastErr = err
			continue
		}
		if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.Rename(partialPath, destination); err != nil {
			return err
		}
		return nil
	}
	_ = os.Remove(partialPath)
	return fmt.Errorf("连续 3 次下载失败：%w", lastErr)
}

func downloadReleaseAssetOnce(rawURL, destination string, limit int64, allowedHosts []string, progress func(int)) error {
	response, err := openReleaseAsset(rawURL, allowedHosts)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	reader := &progressReader{reader: io.LimitReader(response.Body, limit+1), total: response.ContentLength, progress: progress}
	written, err := io.Copy(file, reader)
	if err != nil {
		return err
	}
	if response.ContentLength >= 0 && written != response.ContentLength {
		return fmt.Errorf("下载内容不完整：应为 %d 字节，实际 %d 字节", response.ContentLength, written)
	}
	if written == 0 {
		return errors.New("下载文件为空")
	}
	if written > limit {
		return errors.New("下载文件超过大小限制")
	}
	if progress != nil {
		progress(100)
	}
	return file.Sync()
}

type progressReader struct {
	reader      io.Reader
	total, read int64
	progress    func(int)
	last        int
}

func (reader *progressReader) Read(buffer []byte) (int, error) {
	n, err := reader.reader.Read(buffer)
	reader.read += int64(n)
	if reader.progress != nil && reader.total > 0 {
		percent := int(reader.read * 100 / reader.total)
		if percent > 100 {
			percent = 100
		}
		if percent != reader.last {
			reader.last = percent
			reader.progress(percent)
		}
	}
	return n, err
}

func downloadReleaseAssetBytes(rawURL string, limit int64, allowedHosts []string) ([]byte, error) {
	response, err := openReleaseAsset(rawURL, allowedHosts)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errors.New("下载文件超过大小限制")
	}
	return body, nil
}

func openReleaseAsset(rawURL string, allowedHosts []string) (*http.Response, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || !hostAllowed(parsed.Hostname(), allowedHosts) {
		return nil, errors.New("Release 下载地址不安全")
	}
	request, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "YunqiaoCodexBridge/"+appVersion)
	client := &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) >= 5 || next.URL.Scheme != "https" || !hostAllowed(next.URL.Hostname(), allowedHosts) {
				return errors.New("Release 下载重定向不安全")
			}
			return nil
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		response.Body.Close()
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	return response, nil
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
	helper := filepath.Join(updates, "YunqiaoBridgeUpdater-v"+version+".exe")
	if err := copyFile(current, helper); err != nil {
		return fmt.Errorf("创建更新助手失败：%w", err)
	}
	logPath := filepath.Join(updates, "update-v"+version+".log")
	command := exec.Command(helper, "--apply-update", "--parent", fmt.Sprint(os.Getpid()), "--current", current, "--staged", staged, "--backup", backup, "--log", logPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x00000008}
	if err := command.Start(); err != nil {
		return fmt.Errorf("启动 Bridge 更新程序失败：%w", err)
	}
	return nil
}
