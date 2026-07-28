//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package event

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestExecTimeoutKillsHungProcess verifies AC:exec-timeout-kills-hung-process.
// The descendant ignores SIGTERM, making the delayed SIGKILL escalation
// load-bearing rather than merely verifying process-group setup.
func TestExecTimeoutKillsHungProcess(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	sub := NewExec([]string{
		"sh", "-c",
		`sh -c 'trap "" TERM; printf "%s" "$$" > "$1"; exec sleep 30' child "$1" & wait`,
		"sh", childPIDFile,
	}, nil, 200*time.Millisecond)

	start := time.Now()
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("Deliver returned nil, want timeout error")
	}
	var timeoutErr *ExecTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Deliver error type = %T (%v), want *ExecTimeoutError", err, err)
	}
	if timeoutErr.Timeout != 200*time.Millisecond {
		t.Fatalf("timeoutErr.Timeout = %v, want 200ms", timeoutErr.Timeout)
	}
	if elapsed < 200*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 200ms", elapsed)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("elapsed = %v, want <= 800ms", elapsed)
	}

	pid := waitForUnixChildPID(t, childPIDFile, time.Second)
	t.Cleanup(func() {
		// Ensure a regressed implementation does not leave a test process behind.
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("check child process %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("TERM-ignoring child process %d survived exec timeout", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestExecTimeoutWhenGuardNeverReportsReadiness proves the guard's setup
// handshake is part of Exec's wall-clock budget. In particular, the command
// must not start and the setup guard must be reaped when its stdout stays
// silent forever.
func TestExecTimeoutWhenGuardNeverReportsReadiness(t *testing.T) {
	guardPIDFile := filepath.Join(t.TempDir(), "guard.pid")
	commandMarker := filepath.Join(t.TempDir(), "command-started")

	originalNewGuard := newUnixProcessGroupGuardCmd
	t.Cleanup(func() { newUnixProcessGroupGuardCmd = originalNewGuard })
	newUnixProcessGroupGuardCmd = func() *exec.Cmd {
		return exec.Command(
			"/bin/sh", "-c",
			`printf "%s" "$$" > "$1"; trap '' TERM; read -r _`,
			"guard", guardPIDFile,
		)
	}

	sub := NewExec(
		[]string{"sh", "-c", `printf started > "$1"`, "sh", commandMarker},
		nil,
		100*time.Millisecond,
	)

	start := time.Now()
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	elapsed := time.Since(start)
	var timeoutErr *ExecTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("Deliver error type = %T (%v), want *ExecTimeoutError", err, err)
	}
	if timeoutErr.Timeout != 100*time.Millisecond {
		t.Fatalf("timeoutErr.Timeout = %v, want 100ms", timeoutErr.Timeout)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= 100ms", elapsed)
	}
	if elapsed > 800*time.Millisecond {
		t.Fatalf("elapsed = %v, want <= 800ms", elapsed)
	}
	if _, err := os.Stat(commandMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("subscriber command ran despite silent guard: stat %q: %v", commandMarker, err)
	}

	pid := waitForUnixChildPID(t, guardPIDFile, time.Second)
	deadline := time.Now().Add(time.Second)
	for {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			t.Fatalf("check guard process %d: %v", pid, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("silent process-group guard %d survived exec timeout", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestExecSuccessfulCommandLeavesBackgroundProcessRunning verifies that the
// guard used to pin Unix group identity is released without signalling an
// otherwise successful command's background descendants.
func TestExecSuccessfulCommandLeavesBackgroundProcessRunning(t *testing.T) {
	childPIDFile := filepath.Join(t.TempDir(), "child.pid")
	sub := NewExec([]string{
		"sh", "-c",
		`cat >/dev/null; sh -c 'printf "%s" "$$" > "$1"; exec sleep 30 </dev/null >/dev/null 2>&1' child "$1" &`,
		"sh", childPIDFile,
	}, nil, time.Second)

	if err := sub.Deliver(context.Background(), execSampleEvent(t)); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}

	pid := waitForUnixChildPID(t, childPIDFile, time.Second)
	t.Cleanup(func() {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("background child %d did not survive successful command: %v", pid, err)
	}
}

func waitForUnixChildPID(t *testing.T, path string, timeout time.Duration) int {
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
		if time.Now().After(deadline) {
			t.Fatalf("child PID was not recorded within %v", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSignalProcessGroupRejectsNonPositivePGID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if err := signalProcessGroup(pid, syscall.SIGKILL); err == nil {
			t.Fatalf("signalProcessGroup(%d) returned nil error", pid)
		}
	}
}

func TestUnixProcessTreeRejectsInvalidStartedPID(t *testing.T) {
	tree := &unixExecProcessTree{
		cmd:   &exec.Cmd{},
		ready: make(chan struct{}),
	}
	if err := tree.afterStart(); err == nil {
		t.Fatal("afterStart returned nil error for pid 0")
	}
	if err := tree.cancel(); err == nil {
		t.Fatal("cancel returned nil after setup failure")
	}
}

func TestUnixProcessTreeCancelErrorPaths(t *testing.T) {
	sentinel := errors.New("signal failure")
	tests := []struct {
		name    string
		setup   error
		results []error
		wantErr error
	}{
		{name: "setup failure", setup: sentinel, wantErr: sentinel},
		{name: "term failure still kills", results: []error{sentinel, nil}, wantErr: sentinel},
		{name: "kill failure", results: []error{nil, sentinel}, wantErr: sentinel},
		{name: "both failures", results: []error{sentinel, sentinel}, wantErr: sentinel},
		{name: "success", results: []error{nil, nil}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ready := make(chan struct{})
			close(ready)
			tree := &unixExecProcessTree{
				ready:    ready,
				pgid:     123,
				setupErr: tc.setup,
			}

			originalSignal := signalUnixProcessGroup
			t.Cleanup(func() { signalUnixProcessGroup = originalSignal })
			call := 0
			signalUnixProcessGroup = func(_ int, _ syscall.Signal) error {
				if call >= len(tc.results) {
					t.Fatalf("unexpected signal call %d", call)
				}
				result := tc.results[call]
				call++
				return result
			}

			err := tree.cancel()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("cancel error = %v, want %v", err, tc.wantErr)
			}
			if tc.setup == nil && call != 2 {
				t.Fatalf("signal calls = %d, want TERM then KILL", call)
			}
		})
	}
}

// TestExecTimeoutStillKillsGroupAfterSignalFailure is an end-to-end proof of
// the fail-closed cancellation path. The TERM and KILL seams deliberately do
// not send real signals, leaving a descendant alive with an inherited stdout
// pipe. The direct shell waits for that child, so it is still alive when the
// delivery context expires. Cmd.WaitDelay must then close the pipe and kill
// the direct command rather than letting an OS process-control failure make
// Deliver wait forever.
func TestExecTimeoutStillKillsGroupAfterSignalFailure(t *testing.T) {
	tests := []struct {
		name     string
		failTERM bool
		failKILL bool
	}{
		{name: "TERM", failTERM: true},
		{name: "KILL", failKILL: true},
		{name: "TERM and KILL", failTERM: true, failKILL: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			originalSignal := signalUnixProcessGroup
			t.Cleanup(func() { signalUnixProcessGroup = originalSignal })
			originalStdout := execStdout
			t.Cleanup(func() { execStdout = originalStdout })
			execStdout = &bytes.Buffer{}

			childPIDFile := filepath.Join(t.TempDir(), "child.pid")
			// Register cleanup before starting the command so an assertion failure
			// after the child starts cannot leave the deliberately unsignalled
			// descendant alive for its full sleep duration.
			t.Cleanup(func() {
				pidBytes, err := os.ReadFile(childPIDFile)
				if err != nil {
					return
				}
				pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
				if err == nil && pid > 0 {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			})

			sentinel := errors.New("injected process-group signal failure")
			calls := make([]syscall.Signal, 0, 2)
			cancelObserved := false
			signalUnixProcessGroup = func(_ int, sig syscall.Signal) error {
				cancelObserved = true
				calls = append(calls, sig)
				// This seam intentionally never delegates to originalSignal: every
				// path models a real process-group signalling failure. WaitDelay is
				// the load-bearing fallback that stops the direct shell and closes
				// the inherited output pipe.
				if (sig == syscall.SIGTERM && tc.failTERM) || (sig == syscall.SIGKILL && tc.failKILL) {
					return sentinel
				}
				return nil
			}

			timeout := 75 * time.Millisecond
			start := time.Now()
			err := NewExec([]string{
				"sh", "-c",
				`sh -c 'printf "%s" "$$" > "$1"; trap "" TERM; exec sleep 30' child "$1" & wait`,
				"sh", childPIDFile,
			}, nil, timeout).
				Deliver(context.Background(), execSampleEvent(t))
			elapsed := time.Since(start)
			var timeoutErr *ExecTimeoutError
			if !errors.As(err, &timeoutErr) {
				t.Fatalf("Deliver error type = %T (%v), want *ExecTimeoutError", err, err)
			}
			if elapsed < execProcessCleanupWait-50*time.Millisecond {
				t.Fatalf("Deliver elapsed = %v, want WaitDelay to close the descendant's stdout pipe", elapsed)
			}
			if max := timeout + execSIGTERMGrace + execProcessCleanupWait + 500*time.Millisecond; elapsed > max {
				t.Fatalf("Deliver elapsed = %v, want bounded cleanup within %v", elapsed, max)
			}
			if !cancelObserved {
				t.Fatal("process-tree cancellation was not observed")
			}
			if len(calls) != 2 || calls[0] != syscall.SIGTERM || calls[1] != syscall.SIGKILL {
				t.Fatalf("signals = %v, want [SIGTERM SIGKILL]", calls)
			}

			pid := waitForUnixChildPID(t, childPIDFile, time.Second)
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatalf("descendant %d did not outlive the suppressed group signals: %v", pid, err)
			}
		})
	}
}

func TestUnixProcessGroupSignalSpecialCases(t *testing.T) {
	originalSignal := signalUnixProcessGroup
	t.Cleanup(func() { signalUnixProcessGroup = originalSignal })

	signalUnixProcessGroup = func(_ int, _ syscall.Signal) error {
		return errors.New("signal failure")
	}
	if err := signalProcessGroup(123, syscall.SIGKILL); err == nil {
		t.Fatal("signalProcessGroup returned nil for a signal failure")
	}

	signalUnixProcessGroup = func(_ int, _ syscall.Signal) error {
		return syscall.ESRCH
	}
	if err := signalProcessGroup(123, syscall.SIGKILL); err != nil {
		t.Fatalf("signalProcessGroup with ESRCH: %v", err)
	}
}

func TestUnixProcessTreeClose(t *testing.T) {
	if err := (&unixExecProcessTree{}).close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestUnixProcessTreeCloseBoundsGuardReap(t *testing.T) {
	originalWait := waitUnixProcessGroupGuard
	t.Cleanup(func() { waitUnixProcessGroupGuard = originalWait })

	blocked := make(chan struct{})
	reaped := make(chan struct{})
	waitUnixProcessGroupGuard = func(*exec.Cmd) error {
		<-blocked
		close(reaped)
		return nil
	}

	start := time.Now()
	if err := (&unixExecProcessTree{guard: &exec.Cmd{Process: &os.Process{}}}).close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if elapsed := time.Since(start); elapsed < execProcessCleanupWait {
		t.Fatalf("close returned in %v, want it to honor cleanup bound", elapsed)
	}
	close(blocked)
	<-reaped
}

func TestWaitForUnixProcessGroupGuardExitBoundsReap(t *testing.T) {
	originalWait := waitUnixProcessGroupGuard
	t.Cleanup(func() { waitUnixProcessGroupGuard = originalWait })

	// Nil guards are valid for a partially failed setup and must be a no-op.
	waitForUnixProcessGroupGuardExit(nil)

	blocked := make(chan struct{})
	reaped := make(chan struct{})
	waitUnixProcessGroupGuard = func(*exec.Cmd) error {
		<-blocked
		close(reaped)
		return nil
	}
	start := time.Now()
	waitForUnixProcessGroupGuardExit(&exec.Cmd{Process: &os.Process{}})
	if elapsed := time.Since(start); elapsed < execProcessCleanupWait {
		t.Fatalf("guard reap returned in %v, want it to honor cleanup bound", elapsed)
	}
	close(blocked)
	<-reaped
}

func TestConfigureUnixProcessTreePropagatesGuardSetupFailure(t *testing.T) {
	originalNewGuard := newUnixProcessGroupGuardCmd
	t.Cleanup(func() { newUnixProcessGroupGuardCmd = originalNewGuard })
	newUnixProcessGroupGuardCmd = func() *exec.Cmd {
		return exec.Command("/definitely/not/a-process-group-guard")
	}

	if _, err := configureExecProcessTree(context.Background(), &exec.Cmd{}); err == nil {
		t.Fatal("configureExecProcessTree returned nil error")
	}
}

func TestStartUnixProcessGroupGuardFailures(t *testing.T) {
	originalNewGuard := newUnixProcessGroupGuardCmd
	t.Cleanup(func() { newUnixProcessGroupGuardCmd = originalNewGuard })

	tests := []struct {
		name string
		cmd  func() *exec.Cmd
	}{
		{
			name: "stdin pipe",
			cmd: func() *exec.Cmd {
				cmd := exec.Command("/bin/sh", "-c", "exit 0")
				cmd.Stdin = os.Stdin
				return cmd
			},
		},
		{
			name: "stdout pipe",
			cmd: func() *exec.Cmd {
				cmd := exec.Command("/bin/sh", "-c", "exit 0")
				cmd.Stdout = os.Stdout
				return cmd
			},
		},
		{
			name: "start",
			cmd: func() *exec.Cmd {
				return exec.Command("/definitely/not/a/process-group-guard")
			},
		},
		{
			name: "readiness",
			cmd: func() *exec.Cmd {
				return exec.Command("/bin/sh", "-c", "exit 0")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newUnixProcessGroupGuardCmd = tc.cmd
			if _, _, err := startUnixProcessGroupGuard(context.Background()); err == nil {
				t.Fatal("startUnixProcessGroupGuard returned nil error")
			}
		})
	}
}
