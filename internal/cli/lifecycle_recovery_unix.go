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

// readLifecycleRecoveryEntriesNoFollow holds the project and journal
// descriptors while enumerating recovery entries. Names are subsequently
// reopened through the same no-follow protocol before their contents are used.
func readLifecycleRecoveryEntriesNoFollow(projectRoot string) ([]os.DirEntry, error) {
	project, err := lifecycleRecoveryOpenProject(projectRoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeStagedSpecTree(project) }()
	journal, err := lifecycleRecoveryOpenChild(project, ".specscore-recovery")
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeStagedSpecTree(journal) }()
	return lifecycleRecoveryReadDir(journal.root, -1)
}

// readLifecycleRecoveryRegularFileNoFollow holds project and journal
// descriptors while opening the requested journal entry with O_NOFOLLOW. The
// entry is then read through that held descriptor, so a pathname replacement
// after open cannot redirect recovery to attacker-controlled content.
func readLifecycleRecoveryRegularFileNoFollow(projectRoot, name string) ([]byte, error) {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".json") {
		return nil, fmt.Errorf("invalid lifecycle recovery filename %q", name)
	}
	project, err := lifecycleRecoveryOpenProject(projectRoot)
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeStagedSpecTree(project) }()
	journal, err := lifecycleRecoveryOpenChild(project, ".specscore-recovery")
	if err != nil {
		return nil, err
	}
	defer func() { _ = closeStagedSpecTree(journal) }()
	fd, err := lifecycleRecoveryOpenFileAt(
		int(journal.root.Fd()),
		name,
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
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
