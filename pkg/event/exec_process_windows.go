//go:build windows

package event

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const windowsJobTimeoutExitCode = 1

type windowsExecProcessTree struct {
	cmd      *exec.Cmd
	job      windows.Handle
	ready    chan struct{}
	setupErr error
}

// configureExecProcessTree creates an owned Job Object before the command
// starts. KILL_ON_JOB_CLOSE is a final safety net for every exit path.
func configureExecProcessTree(cmd *exec.Cmd) (execProcessTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create Windows Job Object: %w", err)
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return nil, fmt.Errorf("configure Windows Job Object: %w", err)
	}

	tree := &windowsExecProcessTree{
		cmd:   cmd,
		job:   job,
		ready: make(chan struct{}),
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.Cancel = tree.cancel
	return tree, nil
}

// afterStart assigns the exact os.Process handle to the retained job.
// WithHandle pins process identity through assignment, so no PID lookup or
// reuse window exists. Cancel waits for assignment before terminating the job.
func (t *windowsExecProcessTree) afterStart() error {
	var assignErr error
	handleErr := t.cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(t.job, windows.Handle(handle))
	})
	if handleErr != nil {
		t.setupErr = handleErr
	} else {
		t.setupErr = assignErr
	}
	close(t.ready)
	return t.setupErr
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
	err := windows.CloseHandle(t.job)
	t.job = 0
	return err
}
