//go:build windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestHelperSleep is the process kept running by the replacement test.
func TestHelperSleep(t *testing.T) {
	if os.Getenv("YUNQIAO_HELPER_SLEEP") != "1" {
		t.Skip("helper process only")
	}
	time.Sleep(30 * time.Second)
}

func TestInstallStagedFileReplacesRunningExecutable(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	current := filepath.Join(dir, "YunqiaoCodexBridge (2).exe")
	if err := copyFile(self, current); err != nil {
		t.Fatal(err)
	}
	running := exec.Command(current, "-test.run=^TestHelperSleep$")
	running.Env = append(os.Environ(), "YUNQIAO_HELPER_SLEEP=1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = running.Process.Kill(); _, _ = running.Process.Wait() }()
	time.Sleep(time.Second)
	if err := replaceWindowsFile(filepath.Join(dir, "missing"), current); err == nil {
		t.Fatal("sanity: replacing from a missing file should fail")
	}
	newContent := []byte("new version")
	newPath := current + ".new"
	if err := os.WriteFile(newPath, newContent, 0o700); err != nil {
		t.Fatal(err)
	}
	// The plain replace is what failed for users ("Access is denied").
	if err := replaceWindowsFile(newPath, current); err == nil {
		t.Fatal("plain replace of a running exe succeeded; the failure users hit was not reproduced")
	} else {
		t.Logf("plain replace of the running exe fails as for users: %v", err)
	}
	var logged []string
	err = installStagedFile(newPath, current, func(format string, items ...any) { logged = append(logged, format) })
	if err != nil {
		t.Fatalf("installStagedFile: %v", err)
	}
	got, err := os.ReadFile(current)
	if err != nil || !bytes.Equal(got, newContent) {
		t.Fatalf("current not replaced: %q %v", got, err)
	}
	if asides, _ := filepath.Glob(current + ".old-*"); len(asides) != 1 || len(logged) == 0 {
		t.Fatalf("expected one moved-aside file and a log line, got %v %v", asides, logged)
	}
}
