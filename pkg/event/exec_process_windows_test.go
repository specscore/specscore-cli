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
		select {
		case deliveryErr := <-result:
			t.Fatalf("Deliver returned before child PID was recorded: %v", deliveryErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("child PID was not recorded within %v", timeout)
		}
		time.Sleep(25 * time.Millisecond)
	}
}
