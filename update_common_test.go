package main

import (
	"errors"
	"strings"
	"testing"
)

func TestHostAllowed(t *testing.T) {
	allowed := []string{"index.velyn65.com"}
	for _, host := range []string{"index.velyn65.com", "INDEX.VELYN65.COM"} {
		if !hostAllowed(host, allowed) {
			t.Fatalf("expected %q to be allowed", host)
		}
	}
	for _, host := range []string{"velyn65.com", "evilindex.velyn65.com", "index.velyn65.com.example.org", "gitee.com"} {
		if hostAllowed(host, allowed) {
			t.Fatalf("expected %q to be rejected", host)
		}
	}
}

func TestParseBridgeUpdateManifest(t *testing.T) {
	body := []byte(`{"version":"v1.3.6","download_url":"https://index.velyn65.com/download/YunqiaoCodexBridge.exe","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","notes":"test"}`)
	manifest, err := parseBridgeUpdateManifest(body)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.3.6" || manifest.Notes != "test" {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestParseBridgeUpdateManifestRejectsInvalidHash(t *testing.T) {
	_, err := parseBridgeUpdateManifest([]byte(`{"version":"1.3.6","download_url":"https://index.velyn65.com/download/app.exe","sha256":"bad"}`))
	if err == nil {
		t.Fatal("expected invalid hash to fail")
	}
}

func updateManifestJSON(version, extra string) []byte {
	return []byte(`{"version":"` + version + `","download_url":"https://cdn.example.com/app.exe","sha256":"` +
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" + `"` + extra + `}`)
}

func TestResolveUpdateManifestPicksNewestSource(t *testing.T) {
	bodies := map[string][]byte{
		"https://a/latest.json": updateManifestJSON("1.6.5", ""),
		"https://b/latest.json": updateManifestJSON("1.6.7", ""),
	}
	fetch := func(source string) ([]byte, error) {
		if body, ok := bodies[source]; ok {
			return body, nil
		}
		return nil, errors.New("down")
	}
	manifest, source, err := resolveUpdateManifest([]string{"https://down/latest.json", "https://a/latest.json", "https://b/latest.json"}, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.6.7" || source != "https://b/latest.json" {
		t.Fatalf("got %s from %s", manifest.Version, source)
	}
}

func TestResolveUpdateManifestFollowsManifestURL(t *testing.T) {
	bodies := map[string][]byte{
		"https://old/latest.json": updateManifestJSON("1.6.6", `,"manifest_url":"https://new/latest.json"`),
		"https://new/latest.json": updateManifestJSON("1.7.0", ""),
	}
	manifest, source, err := resolveUpdateManifest([]string{"https://old/latest.json"}, func(source string) ([]byte, error) {
		return bodies[source], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Version != "1.7.0" || source != "https://new/latest.json" {
		t.Fatalf("got %s from %s", manifest.Version, source)
	}
}

func TestResolveUpdateManifestAllSourcesDown(t *testing.T) {
	_, _, err := resolveUpdateManifest([]string{"https://a/latest.json"}, func(string) ([]byte, error) { return nil, errors.New("down") })
	if err == nil {
		t.Fatal("expected failure")
	}
}

func TestParseBridgeUpdateManifestRejectsPlainHTTP(t *testing.T) {
	_, err := parseBridgeUpdateManifest([]byte(`{"version":"1.6.7","download_url":"http://cdn.example.com/app.exe","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	if err == nil {
		t.Fatal("expected http download url to fail")
	}
	manifest, err := parseBridgeUpdateManifest(updateManifestJSON("1.6.7", `,"manifest_url":"http://insecure/latest.json"`))
	if err != nil || manifest.ManifestURL != "" {
		t.Fatalf("insecure manifest_url should be dropped: %v %q", err, manifest.ManifestURL)
	}
}

func TestUpdateManifestSourcesOrder(t *testing.T) {
	sources := updateManifestSources([]string{"https://configured/latest.json"}, "https://remembered/latest.json")
	want := append([]string{"https://configured/latest.json", "https://remembered/latest.json"}, builtinUpdateManifestURLs...)
	if strings.Join(sources, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected sources %v", sources)
	}
	if got := updateManifestSources(builtinUpdateManifestURLs, builtinUpdateManifestURLs[0]); len(got) != len(builtinUpdateManifestURLs) {
		t.Fatalf("duplicates not removed: %v", got)
	}
}

func TestParseUpdateConfig(t *testing.T) {
	urls, err := parseUpdateConfig([]byte(`{"manifest_urls":["https://a/latest.json"," http://insecure/latest.json ","https://b/latest.json"]}`))
	if err != nil || strings.Join(urls, ",") != "https://a/latest.json,https://b/latest.json" {
		t.Fatalf("got %v %v", urls, err)
	}
	for _, body := range []string{`{}`, `{"manifest_urls":["http://x/latest.json"]}`, `not json`} {
		if _, err := parseUpdateConfig([]byte(body)); err == nil {
			t.Fatalf("expected %s to fail", body)
		}
	}
}
