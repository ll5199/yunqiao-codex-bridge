package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

const maxBridgeUpdateBytes int64 = 100 << 20

type bridgeUpdateManifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Notes       string `json:"notes"`
}

func parseBridgeUpdateManifest(body []byte) (bridgeUpdateManifest, error) {
	var manifest bridgeUpdateManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return manifest, errors.New("更新清单格式无效")
	}
	manifest.Version = strings.TrimPrefix(strings.TrimSpace(manifest.Version), "v")
	manifest.DownloadURL = strings.TrimSpace(manifest.DownloadURL)
	manifest.SHA256 = strings.ToLower(strings.TrimSpace(manifest.SHA256))
	manifest.Notes = strings.TrimSpace(manifest.Notes)
	if manifest.Version == "" || manifest.DownloadURL == "" {
		return manifest, errors.New("更新清单缺少版本号或下载地址")
	}
	if len(manifest.SHA256) != 64 {
		return manifest, errors.New("更新清单 SHA-256 格式无效")
	}
	if _, err := hex.DecodeString(manifest.SHA256); err != nil {
		return manifest, errors.New("更新清单 SHA-256 不是有效十六进制")
	}
	return manifest, nil
}

func hostAllowed(host string, allowedHosts []string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	for _, pattern := range allowedHosts {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if strings.HasPrefix(pattern, "*.") {
			suffix := strings.TrimPrefix(pattern, "*")
			if strings.HasSuffix(host, suffix) && host != strings.TrimPrefix(suffix, ".") {
				return true
			}
			continue
		}
		if host == pattern {
			return true
		}
	}
	return false
}
