//go:build windows

package event

import (
	"fmt"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const windowsJobTimeoutExitCode = 1

var (
	windowsNtResumeProcess     = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
	findWindowsNtResumeProcess = windowsNtResumeProcess.Find
	closeWindowsHandle         = windows.CloseHandle
)

type windowsExecProcessTree struct {
	cmd      *exec.Cmd
	job      windows.Handle
	ready    chan struct{}
	setupErr error
}

// configureExecProcessTree creates a private Job Object. The command itself
// starts suspended; afterStart assigns its retained process handle to the job
// and resumes it. No subscriber instruction can run before job ownership is
// established, so an assignment failure cannot leave descendants behind.
func configureExecProcessTree(cmd *exec.Cmd) (execProcessTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create Windows Job Object: %w", err)
	}
	// LazyProc.Call panics when the procedure cannot be resolved. Resolve the
	// native resume entry point before cmd.Start so an unavailable API returns a
	// normal setup error with no suspended child process to clean up.
	if err := findWindowsNtResumeProcess(); err != nil {
		_ = closeWindowsHandle(job)
		return nil, fmt.Errorf("resolve Windows NtResumeProcess: %w", err)
	}

	tree := &windowsExecProcessTree{
		cmd:   cmd,
		job:   job,
		ready: make(chan struct{}),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	cmd.Cancel = tree.cancel
	return tree, nil
}

// afterStart makes CreateProcess -> AssignProcessToJobObject -> resume one
// ownership sequence. NtResumeProcess is the process-handle form of resume;
// os/exec closes CreateProcess's primary-thread handle before returning.
func (t *windowsExecProcessTree) afterStart() error {
	t.setupErr = t.cmd.Process.WithHandle(func(handle uintptr) {
		t.setupErr = windows.AssignProcessToJobObject(t.job, windows.Handle(handle))
		if t.setupErr != nil {
			return
		}
		t.setupErr = resumeWindowsProcess(windows.Handle(handle))
	})
	if t.setupErr != nil {
		// If assignment failed the process is still suspended. If resume failed
		// after assignment, terminating the job also covers that process.
		_ = windows.TerminateJobObject(t.job, windowsJobTimeoutExitCode)
	}
	close(t.ready)
	return t.setupErr
}

func resumeWindowsProcess(process windows.Handle) error {
	status, _, _ := windowsNtResumeProcess.Call(uintptr(process))
	if int32(status) < 0 {
		return windows.NTStatus(status)
	}
	return nil
}

func (t *windowsExecProcessTree) cancel() error {
	<-t.ready
	if t.setupErr != nil {
		return t.setupErr
	}
	return windows.TerminateJobObject(t.job, windowsJobTimeoutExitCode)
}

func (t *windowsExecProcessTree) close() error {
	if t.job == 0 {
		return nil
	}
	err := closeWindowsHandle(t.job)
	t.job = 0
	return err
}
