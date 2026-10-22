//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCodexProcessTrackingDetectsExit(t *testing.T) {
	defer rememberCodexPID(0)
	rememberCodexPID(uint32(os.Getpid()))
	if !codexIsRunning() {
		t.Fatal("running process reported stopped")
	}
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	rememberCodexPID(uint32(command.Process.Pid))
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if codexIsRunning() {
		t.Fatal("exited process still reported running")
	}
	if officialCodexProcessAlive(uint32(os.Getpid()), "OpenAI.Codex_test!Main") {
		t.Fatal("accepted unrelated process as official Codex")
	}
}

func TestCodexManifestMultipleApplicationsSelectsOnlyUI(t *testing.T) {
	// No executable is created, and the versioned WindowsApps path is purposely
	// inaccessible. Resolving an application entry must not inspect that path.
	script := `$p=[PSCustomObject]@{InstallLocation='Z:\WindowsApps\OpenAI.Codex_999.1.0.0';PackageFamilyName='OpenAI.Codex_test'}; [xml]$m='<Package><Applications><Application Id="Runner" Executable="app\resources\codex-command-runner.exe"/><Application Id="ChangedMainID" Executable="app\ChatGPT.exe"/></Applications></Package>'; ` + codexAppManifestSelectionScript
	command := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	install, err := parseCodexPackageInstallation(output)
	if err != nil {
		t.Fatal(err)
	}
	if install.AUMID != "OpenAI.Codex_test!ChangedMainID" {
		t.Fatalf("wrong manifest entry: %#v", install)
	}
}

func TestCodexDiscoveryUsesNewestRegisteredIdentity(t *testing.T) {
	script := `function Get-AppxPackage { @([PSCustomObject]@{Name='OpenAI.Codex';Version='26.9.0.0';PackageFamilyName='OpenAI.Codex_test';Entry='OldMain'},[PSCustomObject]@{Name='OpenAI.Codex';Version='26.100.0.0';PackageFamilyName='OpenAI.Codex_test';Entry='NewMain'}) }; function Get-AppxPackageManifest($p) { [xml]('<Package><Applications><Application Id="Runner" Executable="app\resources\codex-command-runner.exe"/><Application Id="'+$p.Entry+'" Executable="app\Codex.exe"/></Applications></Package>') }; ` + codexApplicationDiscoveryScript
	output, err := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	install, err := parseCodexPackageInstallation(output)
	if err != nil || install.AUMID != "OpenAI.Codex_test!NewMain" {
		t.Fatalf("wrong discovery: %#v %v %s", install, err, output)
	}
}

func TestCodexDiscoveryFallsBackToRegisteredStartEntry(t *testing.T) {
	script := `function Get-AppxPackage { }; function Get-StartApps { @([PSCustomObject]@{Name='Other';AppID='Other.App_test!Main'},[PSCustomObject]@{Name='Codex';AppID='OpenAI.Codex_test!DynamicUI'}) }; ` + codexApplicationDiscoveryScript
	output, err := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).Output()
	if err != nil {
		t.Fatal(err)
	}
	install, err := parseCodexPackageInstallation(output)
	if err != nil || install.AUMID != "OpenAI.Codex_test!DynamicUI" {
		t.Fatalf("wrong StartApps entry: %#v %v", install, err)
	}
}

func TestCodexPackageRejectsInvalidIdentities(t *testing.T) {
	for _, id := range []string{"", "Other.App_test!Main", "OpenAI.Codex_test!", `C:\WindowsApps\ChatGPT.exe`, "OpenAI.Codex_test!Main Other"} {
		output, _ := json.Marshal(map[string]string{"AUMID": id})
		if _, err := parseCodexPackageInstallation(output); err == nil {
			t.Fatalf("accepted invalid identity %q", id)
		}
	}
}

func TestCodexActivationConfirmsProcessWithoutDebugPort(t *testing.T) {
	t.Setenv("CODEX_HOME", `C:\custom codex home`)
	for _, arguments := range []string{"", "--remote-debugging-port=9229 --lang=zh-CN"} {
		t.Run(arguments, func(t *testing.T) {
			calls := 0
			pid, err := activateAndConfirmCodex("OpenAI.Codex_test!DynamicID", arguments, func(id, args string) (uint32, error) {
				calls++
				if id != "OpenAI.Codex_test!DynamicID" || args != arguments {
					t.Fatalf("wrong activation: %s %s", id, args)
				}
				return 123, nil
			}, func(pid uint32, id string) bool { return pid == 123 && id == "OpenAI.Codex_test!DynamicID" }, 0)
			if err != nil || pid != 123 || calls != 1 {
				t.Fatalf("activation failed: pid=%d calls=%d err=%v", pid, calls, err)
			}
		})
	}
}

func TestCodexActivationFailureNeverReportsRunning(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pid   uint32
		err   error
		alive bool
	}{
		{"activation denied", 0, errors.New("access denied"), false},
		{"no process", 0, nil, true},
		{"unrelated or exited process", 123, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pid, err := activateAndConfirmCodex("OpenAI.Codex_test!Main", "", func(string, string) (uint32, error) { return tc.pid, tc.err }, func(uint32, string) bool { return tc.alive }, 0)
			if err == nil || pid != 0 {
				t.Fatalf("failed launch accepted: %d %v", pid, err)
			}
		})
	}
}

func TestCodexActivationWaitsForProcessIdentity(t *testing.T) {
	checks := 0
	pid, err := activateAndConfirmCodex("OpenAI.Codex_test!Main", "", func(string, string) (uint32, error) { return 123, nil }, func(uint32, string) bool { checks++; return checks > 1 }, time.Second)
	if err != nil || pid != 123 || checks < 2 {
		t.Fatalf("identity was not awaited: %d %v", pid, err)
	}
}

func TestCodexLaunchHasNoExecutableFallback(t *testing.T) {
	data, err := os.ReadFile("launch_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"LOCALAPPDATA", "exec.Command(install.", "exec.CommandContext(ctx, install.", "waitForCodexDebug", "Getenv(\"CODEX_HOME\")"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("found legacy executable dependency: %s", forbidden)
		}
	}
}
