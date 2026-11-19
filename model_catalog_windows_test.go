//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsRelayWritesNativeCatalogBeforeLaunch(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t.Setenv("LOCALAPPDATA", t.TempDir())
	if err := writeCodexProviderConfig(codexProxyBase, "new-model", []string{"new-model"}); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil || !strings.Contains(string(config), "model_catalog_json = ") {
		t.Fatalf("missing native catalog configuration: %s (%v)", config, err)
	}
	catalog, err := os.ReadFile(filepath.Join(home, bridgeModelCatalogName))
	if err != nil || !strings.Contains(string(catalog), "\"slug\": \"new-model\"") {
		t.Fatalf("missing new model in native catalog: %s (%v)", catalog, err)
	}
	if err := restoreNativeCodexConfig(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(filepath.Join(home, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("official mode kept relay-only configuration: %v", err)
	}
}
