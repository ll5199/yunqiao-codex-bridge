//go:build windows

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

func runUpdateHelperIfRequested() bool {
	args := os.Args[1:]
	if len(args) == 0 || args[0] != "--apply-update" {
		return false
	}
	values := map[string]string{}
	for i := 1; i+1 < len(args); i += 2 {
		values[args[i]] = args[i+1]
	}
	logFile, _ := os.OpenFile(values["--log"], os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if logFile != nil {
		defer logFile.Close()
	}
	log := func(format string, items ...any) {
		if strings.Contains(format, "failed") {
			_ = os.WriteFile(filepath.Join(applicationDirectory(), "update-result.txt"), []byte("更新失败，旧版备份已保留。请导出诊断日志查看详情。"), 0600)
		}
		if logFile != nil {
			fmt.Fprintf(logFile, time.Now().Format(time.RFC3339)+" "+format+"\r\n", items...)
		}
	}
	parent, _ := strconv.ParseUint(values["--parent"], 10, 32)
	waitForWindowsProcess(uint32(parent), 60*time.Second)
	current, staged, backup := values["--current"], values["--staged"], values["--backup"]
	log("installing staged=%s current=%s", staged, current)
	if err := copyFile(current, backup); err != nil {
		log("backup failed: %v", err)
		restartBridge(current, log)
		return true
	}
	expected, hashErr := updateFileHash(staged)
	if hashErr != nil {
		log("staged hash failed: %v", hashErr)
		_ = restartBridge(current, log)
		return true
	}
	newPath := current + ".new"
	if err := copyFile(staged, newPath); err != nil {
		log("stage copy failed: %v", err)
		restartBridge(current, log)
		return true
	}
	if err := verifyFileSHA256(newPath, expected); err != nil {
		log("staged copy verification failed: %v", err)
		_ = restartBridge(current, log)
		return true
	}
	if err := replaceWindowsFile(newPath, current); err != nil {
		log("replace failed: %v", err)
		restartBridge(current, log)
		return true
	}
	_ = os.Remove(staged)
	if err := verifyFileSHA256(current, expected); err != nil {
		log("installed hash failed: %v", err)
		_ = copyFile(backup, newPath)
		_ = replaceWindowsFile(newPath, current)
		_ = restartBridge(current, log)
		return true
	}
	_ = os.WriteFile(filepath.Join(applicationDirectory(), "update-result.txt"), []byte("更新安装成功。"), 0600)
	log("replace successful")
	if err := restartBridge(current, log); err != nil {
		log("restoring previous version after restart failure")
		if e := copyFile(backup, newPath); e == nil {
			if e = replaceWindowsFile(newPath, current); e == nil {
				_ = restartBridge(current, log)
			} else {
				log("rollback replace failed: %v", e)
			}
		} else {
			log("rollback copy failed: %v", e)
		}
	}
	return true
}

func replaceWindowsFile(source, destination string) error {
	kernel := syscall.NewLazyDLL("kernel32.dll")
	moveFileEx := kernel.NewProc("MoveFileExW")
	sourcePointer, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPointer, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	const moveFileReplaceExisting = 0x1
	const moveFileWriteThrough = 0x8
	result, _, callErr := moveFileEx.Call(
		uintptr(unsafe.Pointer(sourcePointer)),
		uintptr(unsafe.Pointer(destinationPointer)),
		moveFileReplaceExisting|moveFileWriteThrough,
	)
	if result == 0 {
		return callErr
	}
	return nil
}

func waitForWindowsProcess(pid uint32, timeout time.Duration) {
	if pid == 0 {
		return
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	openProcess := kernel.NewProc("OpenProcess")
	waitForSingleObject := kernel.NewProc("WaitForSingleObject")
	closeHandle := kernel.NewProc("CloseHandle")
	handle, _, _ := openProcess.Call(0x00100000, 0, uintptr(pid))
	if handle == 0 {
		return
	}
	defer closeHandle.Call(handle)
	waitForSingleObject.Call(handle, uintptr(timeout/time.Millisecond))
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func restartBridge(path string, log func(string, ...any)) error {
	command := exec.Command(path)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: false, CreationFlags: 0x00000008}
	if err := command.Start(); err != nil {
		log("restart failed: %v", err)
		return err
	} else {
		log("restart successful")
	}
	return nil
}

func updateFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
