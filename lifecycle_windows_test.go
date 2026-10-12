//go:build windows

package main

import (
 "os"
 "os/exec"
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
