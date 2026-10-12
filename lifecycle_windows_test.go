//go:build windows

package main

import (
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "encoding/json"
 "testing"
)

func TestCodexProcessTrackingDetectsExit(t *testing.T) {
 defer rememberCodexPID(0)
 rememberCodexPID(uint32(os.Getpid()))
 if !codexIsRunning() { t.Fatal("running process reported stopped") }
 command:=exec.Command("cmd.exe","/c","exit","0")
 if err:=command.Start();err!=nil {t.Fatal(err)}
 rememberCodexPID(uint32(command.Process.Pid))
 if err:=command.Wait();err!=nil {t.Fatal(err)}
 if codexIsRunning() {t.Fatal("exited process still reported running")}
}

func TestCodexManifestMultipleApplicationsSelectsOnlyUI(t *testing.T) {
 root:=filepath.Join(t.TempDir(),"package with spaces"); _=os.MkdirAll(filepath.Join(root,"app"),0700); _=os.WriteFile(filepath.Join(root,"app","ChatGPT.exe"),[]byte("test"),0600)
 script:=`$p=[PSCustomObject]@{InstallLocation=$env:YUNQIAO_TEST_PACKAGE_ROOT;PackageFamilyName="OpenAI.Codex_test"}; [xml]$m='<Package><Applications><Application Id="Runner" Executable="app\resources\codex-command-runner.exe"/><Application Id="CodexMain" Executable="app\ChatGPT.exe"/></Applications></Package>'; `+codexAppManifestSelectionScript
 command:=exec.Command("powershell.exe","-NoLogo","-NoProfile","-NonInteractive","-Command",script)
 command.Env=append(os.Environ(),"YUNQIAO_TEST_PACKAGE_ROOT="+root)
 output,err:=command.Output(); if err!=nil { t.Fatal(err) }
 install,err:=parseCodexPackageInstallation(output); if err!=nil {t.Fatal(err)}
 if install.Executable!=filepath.Join(root,"app","ChatGPT.exe") || install.AUMID!="OpenAI.Codex_test!CodexMain" || strings.Contains(install.Executable,"runner") { t.Fatalf("wrong manifest entry: %#v",install) }
}
func TestCodexPackageRejectsJoinedOrMissingExecutable(t *testing.T) {
 for _,exe:=range []string{`app\ChatGPT.exe app\resources\codex-command-runner.exe`,`app\ChatGPT.exe`} {
  output,_:=json.Marshal(map[string]string{"InstallLocation":t.TempDir(),"PackageFamilyName":"test","Executable":exe,"AppID":"Main"})
  if _,err:=parseCodexPackageInstallation(output);err==nil {t.Fatalf("accepted invalid executable %q",exe)}
 }
}
