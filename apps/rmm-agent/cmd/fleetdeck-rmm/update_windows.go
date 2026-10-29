//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// triggerRMMUpdate launches a detached cmd script that stops the service,
// replaces the binary, and starts the service again. Credentials in stateDir are untouched.
func triggerRMMUpdate(stateDir string) error {
	pending := pendingUpdatePath(stateDir)
	if st, err := os.Stat(pending); err != nil || st.IsDir() || st.Size() == 0 {
		return fmt.Errorf("no pending update at %s", pending)
	}
	target, err := installBinaryPath()
	if err != nil {
		return err
	}
	bat := filepath.Join(stateDir, "apply-update.cmd")
	script := fmt.Sprintf("@echo off\r\n"+
		"timeout /t 3 /nobreak >nul\r\n"+
		"sc stop %s\r\n"+
		"timeout /t 4 /nobreak >nul\r\n"+
		"copy /Y \"%s\" \"%s\" >nul\r\n"+
		"sc start %s\r\n"+
		"del /F /Q \"%s\" >nul 2>&1\r\n"+
		"del /F /Q \"%%~f0\" >nul 2>&1\r\n",
		rmmServiceName, pending, target, rmmServiceName, pending)
	if err := os.WriteFile(bat, []byte(script), 0o700); err != nil {
		return err
	}

	cmd := exec.Command("cmd.exe", "/C", bat)
	cmd.Dir = stateDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	// Give the child a moment to detach before this process may be stopped.
	time.Sleep(200 * time.Millisecond)
	return nil
}
