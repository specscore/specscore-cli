//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package event

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

type unixExecProcessTree struct {
	cmd      *exec.Cmd
	ready    chan struct{}
	pgid     int
	setupErr error
}

var signalUnixProcessGroup = syscall.Kill

// configureExecProcessTree isolates the command in a process group and makes
// context cancellation terminate the group, including descendants.
func configureExecProcessTree(cmd *exec.Cmd) (execProcessTree, error) {
	tree := &unixExecProcessTree{
		cmd:   cmd,
		ready: make(chan struct{}),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = tree.cancel
	return tree, nil
}

func (t *unixExecProcessTree) afterStart() error {
	t.pgid = t.cmd.Process.Pid
	if t.pgid <= 0 {
		t.setupErr = fmt.Errorf("invalid process group leader pid %d", t.pgid)
	}
	close(t.ready)
	return t.setupErr
}

func (t *unixExecProcessTree) cancel() error {
	<-t.ready
	if t.setupErr != nil {
		return t.setupErr
	}

	alive, err := processGroupAlive(t.pgid)
	if err != nil {
		return err
	}
	if !alive {
		return os.ErrProcessDone
	}
	if err := signalProcessGroup(t.pgid, syscall.SIGTERM); err != nil {
		return err
	}

	time.Sleep(execSIGTERMGrace)

	// Cmd.Wait may reap the group leader while Cancel is in this grace period.
	// POSIX keeps a PGID reserved while any original member remains, so a
	// TERM-ignoring descendant (the only reason escalation is needed) keeps
	// this target bound to the original command tree after its leader exits.
	// Recheck before KILL so a tree fully drained by TERM is not signaled again.
	alive, err = processGroupAlive(t.pgid)
	if err != nil {
		return err
	}
	if !alive {
		return nil
	}
	return signalProcessGroup(t.pgid, syscall.SIGKILL)
}

func (t *unixExecProcessTree) close() error {
	return nil
}

func processGroupAlive(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("exec: invalid process group leader pid %d", pid)
	}
	err := signalUnixProcessGroup(-pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	default:
		return false, fmt.Errorf("exec: check process group %d: %w", pid, err)
	}
}

func signalProcessGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("exec: invalid process group leader pid %d", pid)
	}
	err := signalUnixProcessGroup(-pid, sig)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("exec: signal process group %d: %w", pid, err)
	}
	return nil
}
