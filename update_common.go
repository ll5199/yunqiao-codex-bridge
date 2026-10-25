package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const maxBridgeUpdateBytes int64 = 100 << 20

// builtinUpdateManifestURLs are the entry points compiled into the client.
// Everything else (where the installer lives, a new manifest location) is
// read from latest.json, so moving servers only needs a manifest edit.
var builtinUpdateManifestURLs = []string{
	"https://bika223.26.59.139.yunqiao.xyz/yunqiao-update/latest.json",
	"https://index.velyn65.com/download/latest.json",
}

type bridgeUpdateManifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	Notes       string `json:"notes"`
	// ManifestURL optionally moves the update source: clients remember it and
	// check it first from then on.
	ManifestURL string `json:"manifest_url,omitempty"`
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
	manifest.ManifestURL = strings.TrimSpace(manifest.ManifestURL)
	if manifest.ManifestURL != "" && !isHTTPSURL(manifest.ManifestURL) {
		manifest.ManifestURL = ""
	}
	if manifest.Version == "" || manifest.DownloadURL == "" {
		return manifest, errors.New("更新清单缺少版本号或下载地址")
	}
	if !isHTTPSURL(manifest.DownloadURL) {
		return manifest, errors.New("更新清单中的下载地址必须是 https")
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

func isHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

// updateManifestSources lists where to look: the remembered source first,
// then the built-in entry points, without duplicates.
func updateManifestSources(remembered string) []string {
	sources := make([]string, 0, len(builtinUpdateManifestURLs)+1)
	seen := make(map[string]bool)
	for _, source := range append([]string{strings.TrimSpace(remembered)}, builtinUpdateManifestURLs...) {
		if source != "" && isHTTPSURL(source) && !seen[source] {
			seen[source] = true
			sources = append(sources, source)
		}
	}
	return sources
}

// resolveUpdateManifest reads every source (following one manifest_url hop
// each) and returns the newest valid manifest with the URL it came from, so
// a server left on an old version can never hide a newer release.
func resolveUpdateManifest(sources []string, fetch func(string) ([]byte, error)) (bridgeUpdateManifest, string, error) {
	var best bridgeUpdateManifest
	bestSource := ""
	var failures []string
	visited := make(map[string]bool)
	succeeded := make(map[string]bool)
	consider := func(source string) (bridgeUpdateManifest, bool) {
		if visited[source] {
			return bridgeUpdateManifest{}, false
		}
		visited[source] = true
		body, err := fetch(source)
		if err == nil {
			var manifest bridgeUpdateManifest
			if manifest, err = parseBridgeUpdateManifest(body); err == nil {
				succeeded[source] = true
				if bestSource == "" || compareVersions(manifest.Version, best.Version) > 0 {
					best, bestSource = manifest, source
				}
				return manifest, true
			}
		}
		failures = append(failures, fmt.Sprintf("%s：%v", source, err))
		return bridgeUpdateManifest{}, false
	}
	for _, source := range sources {
		if manifest, ok := consider(source); ok && manifest.ManifestURL != "" {
			consider(manifest.ManifestURL)
		}
	}
	if bestSource == "" {
		return best, "", fmt.Errorf("所有更新源都不可用：%s", strings.Join(failures, "；"))
	}
	// Prefer the location the newest manifest points at for future checks.
	if best.ManifestURL != "" && succeeded[best.ManifestURL] {
		bestSource = best.ManifestURL
	}
	return best, bestSource, nil
}
