package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// Runs against a live Chromium page (see scripts/test-image-card.cjs --cdp):
// YUNQIAO_CDP_E2E_PORT=<devtools port> go test -run TestCDPImageDeliveryAndDownload
func TestCDPImageDeliveryAndDownload(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv("YUNQIAO_CDP_E2E_PORT"))
	if err != nil {
		t.Skip("YUNQIAO_CDP_E2E_PORT not set")
	}
	downloads := t.TempDir()
	t.Setenv("USERPROFILE", filepath.Dir(downloads))
	store := newPersistentImageStore(t.TempDir(), nil)
	png := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	if !store.add(png) {
		t.Fatal("image not stored")
	}
	revealed := make(chan string, 1)
	revealSavedFile = func(path string) { revealed <- path }
	proxy := &apiProxy{logger: func(k, v string) { t.Log(k, v) }, store: store, done: make(chan struct{}), startedAt: time.Now().UnixMilli() - 1000}
	go syncProxyImagesToCodex(port, proxy)
	defer close(proxy.done)
	if path := os.Getenv("YUNQIAO_CDP_E2E_READY"); path != "" {
		_ = os.WriteFile(path, []byte("ready"), 0o644)
	}
	select {
	case path := <-revealed:
		if filepath.Dir(path) != filepath.Join(filepath.Dir(downloads), "Downloads") {
			t.Fatalf("saved outside Downloads: %s", path)
		}
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			t.Fatalf("saved file missing: %v", err)
		}
		t.Logf("saved %s", path)
	case <-time.After(60 * time.Second):
		t.Fatal("no download request reached the bridge")
	}
}
