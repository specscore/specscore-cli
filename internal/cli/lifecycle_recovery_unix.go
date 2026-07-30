//go:build darwin || linux

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

var (
	lifecycleRecoveryOpenProject = openLifecycleProjectNoFollow
	lifecycleRecoveryOpenChild   = openLifecycleProjectChildNoFollow
	lifecycleRecoveryOpenFileAt  = unix.Openat
	lifecycleRecoveryFileStat    = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	lifecycleRecoveryReadAll     = io.ReadAll
	lifecycleRecoveryBeforeRead  = func(*os.File) error { return nil }
	lifecycleRecoveryReadDir     = func(file *os.File, count int) ([]os.DirEntry, error) { return file.ReadDir(count) }
)

func openLifecycleRecoveryHandleNoFollow(projectRoot string) (*lifecycleRecoveryHandle, error) {
	project, err := lifecycleRecoveryOpenProject(projectRoot)
	if err != nil {
		return nil, err
	}
	journal, err := lifecycleRecoveryOpenChild(project, ".specscore-recovery")
	if err != nil {
		_ = closeStagedSpecTree(project)
		return nil, err
	}
	return &lifecycleRecoveryHandle{project: project, journal: journal}, nil
}

func closeLifecycleRecoveryHandle(handle *lifecycleRecoveryHandle) error {
	if handle == nil {
		return nil
	}
	var closeErr error
	if handle.journal != nil {
		closeErr = closeStagedSpecTree(handle.journal)
	}
	if handle.project != nil {
		if err := closeStagedSpecTree(handle.project); closeErr == nil {
			closeErr = err
		}
	}
	return closeErr
}

// readLifecycleRecoveryEntriesNoFollow keeps the journal descriptor open
// through subsequent canonical/intent pair validation.
func readLifecycleRecoveryEntriesNoFollow(handle *lifecycleRecoveryHandle) ([]os.DirEntry, error) {
	if handle == nil || handle.journal == nil || handle.journal.root == nil {
		return nil, fmt.Errorf("lifecycle recovery journal descriptor is closed")
	}
	return lifecycleRecoveryReadDir(handle.journal.root, -1)
}

// readLifecycleRecoveryRegularFileNoFollow opens the requested journal entry
// under a held journal descriptor. O_NONBLOCK prevents FIFOs from stalling
// recovery before their post-open regular-file check.
func readLifecycleRecoveryRegularFileNoFollow(handle *lifecycleRecoveryHandle, name string) ([]byte, error) {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return nil, fmt.Errorf("invalid lifecycle recovery filename %q", name)
	}
	if handle == nil || handle.journal == nil || handle.journal.root == nil {
		return nil, fmt.Errorf("lifecycle recovery journal descriptor is closed")
	}
	fd, err := lifecycleRecoveryOpenFileAt(
		int(handle.journal.root.Fd()),
		name,
		unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer func() { _ = file.Close() }()
	before, err := lifecycleRecoveryFileStat(file)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("lifecycle recovery entry %q is not a regular file", name)
	}
	if err := lifecycleRecoveryBeforeRead(file); err != nil {
		return nil, err
	}
	data, err := lifecycleRecoveryReadAll(file)
	if err != nil {
		return nil, err
	}
	after, err := lifecycleRecoveryFileStat(file)
	if err != nil {
		return nil, err
	}
	if before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("lifecycle recovery entry %q changed while reading", name)
	}
	return data, nil
}
