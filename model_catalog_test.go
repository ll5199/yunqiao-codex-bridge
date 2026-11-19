package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
)

func TestBridgeCatalogReplacesModelList(t *testing.T) {
	home := t.TempDir()
	path, err := writeBridgeModelCatalog(home, []string{"removed-model"}, smartRouterModel)
	if err != nil {
		t.Fatal(err)
	}
	next, err := writeBridgeModelCatalog(home, []string{"new-model", " new-model ", "retained-model", smartRouterModel}, "new-model")
	if err != nil || next != path {
		t.Fatalf("catalog was not replaced: %q %v", next, err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct{ Models []bridgeCatalogModel }
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	var slugs []string
	for _, model := range payload.Models {
		slugs = append(slugs, model.Slug)
		if model.Visibility != "list" || !model.SupportedInAPI {
			t.Fatalf("model hidden: %s", model.Slug)
		}
		if model.Slug == "new-model" && model.Priority != 0 {
			t.Fatal("selected model not first")
		}
	}
	want := []string{"new-model", "retained-model", smartRouterModel}
	if !reflect.DeepEqual(slugs, want) {
		t.Fatalf("got %v, want %v", slugs, want)
	}
	for _, models := range [][]string{nil, {smartRouterModel}, {"other"}} {
		if _, err := buildBridgeModelCatalog(models, "new-model"); err == nil {
			t.Fatal("accepted stale selected model")
		}
	}
}

func TestBridgeCatalogConfigRoundTrip(t *testing.T) {
	original := "[model_providers.other]\nname = \"Other\"\n"
	path := filepath.Join(t.TempDir(), "云桥 catalog.json")
	if err := checkBridgeCatalogConflict(original); err != nil {
		t.Fatal(err)
	}
	relay := updateCodexConfigWithAuth(original, "http://127.0.0.1:9230/v1", "new-model", false, path)
	if err := checkBridgeCatalogConflict(relay); err != nil {
		t.Fatal(err)
	}
	relay = updateCodexConfigWithAuth(relay, "http://127.0.0.1:9230/v1", "new-model", false, path)
	var decoded map[string]any
	if err := toml.Unmarshal([]byte(relay), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["model_catalog_json"] != path || decoded["model"] != "new-model" || decoded["model_provider"] != providerID {
		t.Fatalf("missing managed settings: %v", decoded)
	}
	if restored := removeYunqiaoCodexConfig(relay); restored != original {
		t.Fatalf("official config changed: %q", restored)
	}
	if err := checkBridgeCatalogConflict("'model_catalog_json' = 'custom.json'\n" + original); err == nil {
		t.Fatal("allowed overwrite of a custom catalog")
	}
}

// Verify the same JSON and TOML the launcher generates against Codex's actual
// app-server model/list, twice with different catalogs and a clean config home.
func TestBridgeCatalogCodexAppServer(t *testing.T) {
	binary := os.Getenv("YUNQIAO_CODEX_TEST_BINARY")
	if binary == "" {
		t.Skip("set YUNQIAO_CODEX_TEST_BINARY to run Codex app-server integration")
	}
	home := t.TempDir()
	configPath := filepath.Join(home, "config.toml")
	for _, model := range []string{"yunqiao-test-previous", "yunqiao-test-latest"} {
		path, err := writeBridgeModelCatalog(home, []string{model}, model)
		if err != nil {
			t.Fatal(err)
		}
		config := updateCodexConfigWithAuth("", "http://127.0.0.1:1/v1", model, false, path)
		if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
			t.Fatal(err)
		}
		t.Run(model, func(t *testing.T) {
			got := listAppServerModels(t, binary, home)
			want := []string{smartRouterModel, model}
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("model/list: got %v, want %v", got, want)
			}
		})
	}
}

func listAppServerModels(t *testing.T, binary, home string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "app-server")
	cmd.Dir = home
	cmd.Env = []string{"CODEX_HOME=" + home, "HOME=" + home, "USERPROFILE=" + home, "PATH=" + os.Getenv("PATH"), "SystemRoot=" + os.Getenv("SystemRoot")}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close(); cancel(); _ = cmd.Wait() }()
	send := func(payload string) {
		t.Helper()
		if _, err := fmt.Fprintln(in, payload); err != nil {
			t.Fatal(err)
		}
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4<<10), 4<<20)
	read := func(id int) json.RawMessage {
		t.Helper()
		for scanner.Scan() {
			var response struct {
				ID            int
				Result, Error json.RawMessage
			}
			if json.Unmarshal(scanner.Bytes(), &response) != nil || response.ID != id {
				continue
			}
			if len(response.Error) > 0 {
				t.Fatalf("app-server RPC %d: %s", id, response.Error)
			}
			return response.Result
		}
		t.Fatal(errors.Join(io.EOF, scanner.Err(), ctx.Err()))
		return nil
	}
	send(`{"id":1,"method":"initialize","params":{"clientInfo":{"name":"yunqiao-catalog-test","version":"1"}}}`)
	_ = read(1)
	send(`{"method":"initialized"}`)
	send(`{"id":2,"method":"model/list","params":{"includeHidden":false}}`)
	var result struct {
		Data []struct {
			Model  string
			Hidden bool
		}
	}
	if err := json.Unmarshal(read(2), &result); err != nil {
		t.Fatal(err)
	}
	var models []string
	for _, m := range result.Data {
		if m.Hidden || strings.TrimSpace(m.Model) == "" {
			t.Fatalf("invalid model/list entry: %+v", m)
		}
		models = append(models, m.Model)
	}
	sort.Strings(models)
	return models
}
