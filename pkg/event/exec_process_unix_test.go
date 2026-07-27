//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package event

import (
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

	pidBytes, err := os.ReadFile(childPIDFile)
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || pid <= 0 {
		t.Fatalf("child pid = %q, want a positive integer: %v", pidBytes, err)
	}
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

func TestProcessGroupAliveRejectsNonPositivePID(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if _, err := processGroupAlive(pid); err == nil {
			t.Fatalf("processGroupAlive(%d) returned nil error", pid)
		}
		if err := signalProcessGroup(pid, syscall.SIGKILL); err == nil {
			t.Fatalf("signalProcessGroup(%d) returned nil error", pid)
		}
	}
}

func TestUnixProcessTreeRejectsInvalidStartedPID(t *testing.T) {
	tree := &unixExecProcessTree{
		cmd:   &exec.Cmd{Process: &os.Process{Pid: 0}},
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
		{name: "already exited", results: []error{syscall.ESRCH}, wantErr: os.ErrProcessDone},
		{name: "initial check failure", results: []error{sentinel}, wantErr: sentinel},
		{name: "term failure", results: []error{nil, sentinel}, wantErr: sentinel},
		{name: "drained during grace", results: []error{nil, nil, syscall.ESRCH}},
		{name: "grace recheck failure", results: []error{nil, nil, sentinel}, wantErr: sentinel},
		{name: "kill failure", results: []error{nil, nil, nil, sentinel}, wantErr: sentinel},
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
		})
	}
}

func TestUnixProcessGroupSignalSpecialCases(t *testing.T) {
	originalSignal := signalUnixProcessGroup
	t.Cleanup(func() { signalUnixProcessGroup = originalSignal })

	signalUnixProcessGroup = func(_ int, _ syscall.Signal) error {
		return syscall.EPERM
	}
	alive, err := processGroupAlive(123)
	if err != nil || !alive {
		t.Fatalf("processGroupAlive with EPERM = (%v, %v), want (true, nil)", alive, err)
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
