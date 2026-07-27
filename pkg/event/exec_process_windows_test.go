//go:build windows

package event

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

const windowsStillActive = 259

func TestExecWindowsResumeLookupFailurePreventsStart(t *testing.T) {
	originalFind := findWindowsNtResumeProcess
	originalClose := closeWindowsHandle
	t.Cleanup(func() {
		findWindowsNtResumeProcess = originalFind
		closeWindowsHandle = originalClose
	})

	sentinel := errors.New("NtResumeProcess is unavailable")
	findWindowsNtResumeProcess = func() error { return sentinel }
	closeCalls := 0
	closeWindowsHandle = func(handle windows.Handle) error {
		if handle == 0 {
			t.Error("closed an invalid Windows Job Object handle")
		}
		closeCalls++
		return originalClose(handle)
	}

	marker := filepath.Join(t.TempDir(), "command-started")
	sub := NewExec(
		[]string{
			"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command",
			"[System.IO.File]::WriteAllText($env:SPECSCORE_RESUME_LOOKUP_MARKER, 'started')",
		},
		map[string]string{"SPECSCORE_RESUME_LOOKUP_MARKER": marker},
		time.Second,
	)

	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if !errors.Is(err, sentinel) {
		t.Fatalf("Deliver error = %v, want %v", err, sentinel)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("command ran despite failed NtResumeProcess lookup: stat %q: %v", marker, err)
	}
	if closeCalls != 1 {
		t.Fatalf("closed %d Job Object handles, want 1", closeCalls)
	}
}

// TestExecTimeoutKillsHungProcess verifies the Windows half of
// AC:exec-timeout-kills-hung-process on a real Windows runner. The retained
// child handle proves that timeout terminates the descendant, not only its
// direct PowerShell parent.
func TestExecTimeoutKillsHungProcess(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	script := `$child = Start-Process -FilePath 'powershell.exe' ` +
		`-ArgumentList '-NoLogo -NoProfile -NonInteractive -Command Start-Sleep -Seconds 30' ` +
		`-PassThru; ` +
		`[System.IO.File]::WriteAllText($env:SPECSCORE_CHILD_PID_FILE, [string]$child.Id); ` +
		`Wait-Process -Id $child.Id`
	sub := NewExec(
		[]string{"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script},
		map[string]string{"SPECSCORE_CHILD_PID_FILE": childPIDFile},
		5*time.Second,
	)

	result := make(chan error, 1)
	start := time.Now()
	go func() {
		result <- sub.Deliver(context.Background(), execSampleEvent(t))
	}()

	pid := waitForWindowsChildPID(t, childPIDFile, result, 4*time.Second)
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		t.Fatalf("open child process %d: %v", pid, err)
	}
	t.Cleanup(func() {
		_ = windows.TerminateProcess(handle, 1)
		_ = windows.CloseHandle(handle)
	})

	err = <-result
	elapsed := time.Since(start)
	var timeoutErr *ExecTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Deliver error type = %T (%v), want *ExecTimeoutError", err, err)
	}
	if elapsed < 5*time.Second {
		t.Fatalf("elapsed = %v, want >= 5s", elapsed)
	}
	if elapsed > 7*time.Second {
		t.Fatalf("elapsed = %v, want <= 7s", elapsed)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		var exitCode uint32
		if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
			t.Fatalf("query child process %d: %v", pid, err)
		}
		if exitCode != windowsStillActive {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child process %d survived Windows Job Object timeout", pid)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// TestExecSuccessfulCommandLeavesBackgroundProcessRunning proves that closing
// the private Job Object after a successful command does not turn success into
// timeout-style cleanup. The child remains in the job but must stay alive once
// SpecScore releases its final job handle.
func TestExecSuccessfulCommandLeavesBackgroundProcessRunning(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	script := `$child = Start-Process -FilePath 'powershell.exe' ` +
		`-ArgumentList '-NoLogo -NoProfile -NonInteractive -Command Start-Sleep -Seconds 30' ` +
		`-PassThru; ` +
		`[System.IO.File]::WriteAllText($env:SPECSCORE_CHILD_PID_FILE, [string]$child.Id); ` +
		`exit 0`
	sub := NewExec(
		[]string{"powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script},
		map[string]string{"SPECSCORE_CHILD_PID_FILE": childPIDFile},
		5*time.Second,
	)

	if err := sub.Deliver(context.Background(), execSampleEvent(t)); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}

	pid := waitForWindowsChildPID(t, childPIDFile, nil, 2*time.Second)
	handle, err := windows.OpenProcess(
		windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE,
		false,
		uint32(pid),
	)
	if err != nil {
		t.Fatalf("open child process %d: %v", pid, err)
	}
	t.Cleanup(func() {
		_ = windows.TerminateProcess(handle, 1)
		_ = windows.CloseHandle(handle)
	})

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		var exitCode uint32
		if err := windows.GetExitCodeProcess(handle, &exitCode); err != nil {
			t.Fatalf("query child process %d: %v", pid, err)
		}
		if exitCode != windowsStillActive {
			t.Fatalf("background child %d exited after successful command with code %d", pid, exitCode)
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func waitForWindowsChildPID(
	t *testing.T,
	path string,
	result <-chan error,
	timeout time.Duration,
) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		pidBytes, err := os.ReadFile(path)
		if err == nil {
			pid, parseErr := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("child pid = %q, want a positive integer: %v", pidBytes, parseErr)
			}
			return pid
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read child pid: %v", err)
		}
		if result != nil {
			select {
			case deliveryErr := <-result:
				t.Fatalf("Deliver returned before child PID was recorded: %v", deliveryErr)
			default:
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("child PID was not recorded within %v", timeout)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
