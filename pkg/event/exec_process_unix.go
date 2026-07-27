//go:build aix || darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd || solaris

package event

import (
	"fmt"
	"io"
	"os/exec"
	"syscall"
	"time"
)

type unixExecProcessTree struct {
	cmd        *exec.Cmd
	guard      *exec.Cmd
	guardStdin io.WriteCloser
	ready      chan struct{}
	pgid       int
	setupErr   error
}

var signalUnixProcessGroup = syscall.Kill

// newUnixProcessGroupGuardCmd is a seam for the setup-failure tests. The
// production command uses a shell builtin only: after reporting readiness it
// waits on stdin, so normal cleanup can release exactly this guard process
// without signalling successful command descendants.
var newUnixProcessGroupGuardCmd = func() *exec.Cmd {
	return exec.Command("/bin/sh", "-c", `trap '' TERM; printf .; read -r _`)
}

// configureExecProcessTree puts the command in a group led by an owned guard.
// The guard ignores SIGTERM and stays in the group until cleanup, pinning the
// PGID across the TERM-to-KILL grace window. That removes the otherwise unsafe
// race in which an empty process group ID could be reused before SIGKILL.
func configureExecProcessTree(cmd *exec.Cmd) (execProcessTree, error) {
	guard, guardStdin, err := startUnixProcessGroupGuard()
	if err != nil {
		return nil, err
	}

	tree := &unixExecProcessTree{
		cmd:        cmd,
		guard:      guard,
		guardStdin: guardStdin,
		ready:      make(chan struct{}),
		pgid:       guard.Process.Pid,
	}
	// setpgid runs in the forked child before exec. The command therefore joins
	// the already-live guard group before any subscriber code can run.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: tree.pgid}
	cmd.Cancel = tree.cancel
	return tree, nil
}

func startUnixProcessGroupGuard() (*exec.Cmd, io.WriteCloser, error) {
	guard := newUnixProcessGroupGuardCmd()
	guard.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := guard.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("create process-group guard stdin: %w", err)
	}
	stdout, err := guard.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("create process-group guard stdout: %w", err)
	}
	if err := guard.Start(); err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("start process-group guard: %w", err)
	}

	ready := []byte{0}
	if _, err := io.ReadFull(stdout, ready); err != nil {
		_ = stdin.Close()
		_ = guard.Process.Kill()
		_ = guard.Wait()
		return nil, nil, fmt.Errorf("wait for process-group guard readiness: %w", err)
	}
	return guard, stdin, nil
}

func (t *unixExecProcessTree) afterStart() error {
	if t.cmd.Process == nil || t.cmd.Process.Pid <= 0 {
		t.setupErr = fmt.Errorf("invalid command process")
	}
	close(t.ready)
	return t.setupErr
}

func (t *unixExecProcessTree) cancel() error {
	<-t.ready
	if t.setupErr != nil {
		return t.setupErr
	}
	if err := signalProcessGroup(t.pgid, syscall.SIGTERM); err != nil {
		return err
	}

	time.Sleep(execSIGTERMGrace)
	// The TERM-ignoring guard remains in this group, so this numeric target is
	// still our group even if every command process exited during the grace.
	return signalProcessGroup(t.pgid, syscall.SIGKILL)
}

func (t *unixExecProcessTree) close() error {
	if t.guardStdin != nil {
		_ = t.guardStdin.Close()
	}
	if t.guard != nil && t.guard.Process != nil {
		// The guard is a single shell process (its read is a builtin), so this
		// only reaps the guard; it cannot signal successful descendants that
		// share its group. Killing it also bounds cleanup if an inherited stdin
		// descriptor ever delays the shell's EOF observation.
		_ = t.guard.Process.Kill()
		_ = t.guard.Wait()
	}
	return nil
}

func signalProcessGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return fmt.Errorf("exec: invalid process group leader pid %d", pgid)
	}
	err := signalUnixProcessGroup(-pgid, sig)
	if err == syscall.ESRCH {
		return nil
	}
	if err != nil {
		return fmt.Errorf("exec: signal process group %d: %w", pgid, err)
	}
	return nil
}
