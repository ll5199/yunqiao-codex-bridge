package main

import "testing"

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
