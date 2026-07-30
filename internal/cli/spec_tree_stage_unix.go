//go:build darwin || linux

package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// stagedSpecTreeWorkingDirectoryMu serialises the short fchdir handoff used
// for the in-process linter.  Darwin does not support traversing a directory
// through /dev/fd/N (the descriptor is exposed as a non-directory file), so
// passing a descriptor-derived path is not portable.  fchdir anchors "." to
// the already-open O_NOFOLLOW descriptor instead; a pathname replacement of
// stage.path cannot redirect the linter's reads or writes.
//
// The CLI executes lifecycle commands synchronously.  The mutex also makes
// that process-global CWD handoff explicit for future in-process callers.
var stagedSpecTreeWorkingDirectoryMu sync.Mutex

var (
	stageUnixOpen    = unix.Open
	stageUnixClose   = unix.Close
	stageUnixFchdir  = unix.Fchdir
	stageUnixFchmod  = unix.Fchmod
	stageUnixMkdirat = unix.Mkdirat
	stageUnixOpenat  = unix.Openat
	stageUnixDup     = unix.Dup
	stageUnixWrite   = unix.Write
)

func openStagedSpecTreeNoFollow(path string) (*stagedSpecTree, error) {
	fd, err := stageUnixOpen(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return &stagedSpecTree{path: path, root: os.NewFile(uintptr(fd), path)}, nil
}

func closeStagedSpecTree(stage *stagedSpecTree) error {
	if stage == nil || stage.root == nil {
		return nil
	}
	err := stage.root.Close()
	stage.root = nil
	return err
}

// runLintInStagedSpecTree invokes run with "." after changing the process
// working directory directly to the held stage descriptor.  The previous CWD
// is also held by descriptor so restoring it has no pathname race.
func runLintInStagedSpecTree(stage *stagedSpecTree, run func(string) error) error {
	if stage == nil || stage.root == nil {
		return fmt.Errorf("staged spec tree descriptor is closed")
	}
	stagedSpecTreeWorkingDirectoryMu.Lock()
	defer stagedSpecTreeWorkingDirectoryMu.Unlock()

	previousFD, err := stageUnixOpen(".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("opening current directory before isolated lint: %w", err)
	}
	defer func() { _ = stageUnixClose(previousFD) }()
	if err := stageUnixFchdir(int(stage.root.Fd())); err != nil {
		return fmt.Errorf("entering isolated lint tree by descriptor: %w", err)
	}

	runErr := run(".")
	if restoreErr := stageUnixFchdir(previousFD); restoreErr != nil {
		if runErr != nil {
			return fmt.Errorf("%w; restoring working directory after isolated lint: %v", runErr, restoreErr)
		}
		return fmt.Errorf("restoring working directory after isolated lint: %w", restoreErr)
	}
	return runErr
}

func materializeStagedSpecTreeNoFollow(stage *stagedSpecTree, snapshot specTreeSnapshot) error {
	if err := validateSpecTreeSnapshot(snapshot); err != nil {
		return err
	}
	if mode, ok := snapshot.directories["."]; ok {
		if err := stageUnixFchmod(int(stage.root.Fd()), uint32(mode.mode.Perm())); err != nil {
			return fmt.Errorf("setting stage root mode: %w", err)
		}
	}
	directories := make([]string, 0, len(snapshot.directories))
	for rel := range snapshot.directories {
		if rel != "." {
			directories = append(directories, rel)
		}
	}
	sort.Slice(directories, func(i, j int) bool {
		a, b := strings.Count(directories[i], string(os.PathSeparator)), strings.Count(directories[j], string(os.PathSeparator))
		return a < b || (a == b && directories[i] < directories[j])
	})
	for _, rel := range directories {
		parentFD, name, err := stageParentDirectoryFD(stage, rel)
		if err != nil {
			return fmt.Errorf("opening stage parent for %s: %w", rel, err)
		}
		if err := stageUnixMkdirat(parentFD, name, uint32(snapshot.directories[rel].mode.Perm())); err != nil {
			_ = stageUnixClose(parentFD)
			return fmt.Errorf("creating stage directory %s: %w", rel, err)
		}
		childFD, err := stageUnixOpenat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = stageUnixClose(parentFD)
		if err != nil {
			return fmt.Errorf("opening created stage directory %s: %w", rel, err)
		}
		if err := stageUnixFchmod(childFD, uint32(snapshot.directories[rel].mode.Perm())); err != nil {
			_ = stageUnixClose(childFD)
			return fmt.Errorf("setting stage directory mode %s: %w", rel, err)
		}
		if err := stageUnixClose(childFD); err != nil {
			return fmt.Errorf("closing stage directory %s: %w", rel, err)
		}
	}
	files := make([]string, 0, len(snapshot.files))
	for rel := range snapshot.files {
		files = append(files, rel)
	}
	sort.Strings(files)
	for _, rel := range files {
		parentFD, name, err := stageParentDirectoryFD(stage, rel)
		if err != nil {
			return fmt.Errorf("opening stage file parent for %s: %w", rel, err)
		}
		file := snapshot.files[rel]
		fd, err := stageUnixOpenat(parentFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(file.mode.Perm()))
		_ = stageUnixClose(parentFD)
		if err != nil {
			return fmt.Errorf("creating stage file %s: %w", rel, err)
		}
		if err := writeAllAtFD(fd, file.content); err != nil {
			_ = stageUnixClose(fd)
			return fmt.Errorf("writing stage file %s: %w", rel, err)
		}
		if err := stageUnixFchmod(fd, uint32(file.mode.Perm())); err != nil {
			_ = stageUnixClose(fd)
			return fmt.Errorf("setting stage file mode %s: %w", rel, err)
		}
		if err := applyStagedEntryMetadata(fd, file.metadata); err != nil {
			_ = stageUnixClose(fd)
			return fmt.Errorf("setting stage file metadata %s: %w", rel, err)
		}
		if err := stageUnixClose(fd); err != nil {
			return fmt.Errorf("closing stage file %s: %w", rel, err)
		}
	}
	// A child creation changes its parent modification time. Restore directory
	// metadata only after the complete subtree exists, deepest-first, so a
	// no-op lint preserves nanosecond timestamps instead of manufacturing a
	// manifest difference.
	directories = append(directories, ".")
	sort.Slice(directories, func(i, j int) bool {
		a, b := strings.Count(directories[i], string(os.PathSeparator)), strings.Count(directories[j], string(os.PathSeparator))
		return a > b || (a == b && directories[i] > directories[j])
	})
	for _, rel := range directories {
		fd, err := openStageDirectoryFD(stage, rel)
		if err != nil {
			return fmt.Errorf("opening stage directory metadata %s: %w", rel, err)
		}
		entry := snapshot.directories[rel]
		if err := stageUnixFchmod(fd, uint32(entry.mode.Perm())); err != nil {
			_ = stageUnixClose(fd)
			return fmt.Errorf("setting stage directory final mode %s: %w", rel, err)
		}
		if err := applyStagedEntryMetadata(fd, entry.metadata); err != nil {
			_ = stageUnixClose(fd)
			return fmt.Errorf("setting stage directory metadata %s: %w", rel, err)
		}
		if err := stageUnixClose(fd); err != nil {
			return fmt.Errorf("closing stage directory metadata %s: %w", rel, err)
		}
	}
	return nil
}

func openStageDirectoryFD(stage *stagedSpecTree, rel string) (int, error) {
	if rel == "." {
		return stageUnixDup(int(stage.root.Fd()))
	}
	if err := validateSnapshotRelativePath(rel); err != nil {
		return -1, err
	}
	fd, err := stageUnixDup(int(stage.root.Fd()))
	if err != nil {
		return -1, err
	}
	for _, component := range strings.Split(rel, string(os.PathSeparator)) {
		nextFD, err := stageUnixOpenat(fd, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = stageUnixClose(fd)
		if err != nil {
			return -1, err
		}
		fd = nextFD
	}
	return fd, nil
}

func stageParentDirectoryFD(stage *stagedSpecTree, rel string) (int, string, error) {
	if err := validateSnapshotRelativePath(rel); err != nil {
		return -1, "", err
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	fd, err := stageUnixDup(int(stage.root.Fd()))
	if err != nil {
		return -1, "", err
	}
	for _, part := range parts[:len(parts)-1] {
		nextFD, err := stageUnixOpenat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		_ = stageUnixClose(fd)
		if err != nil {
			return -1, "", err
		}
		fd = nextFD
	}
	return fd, parts[len(parts)-1], nil
}

func writeAllAtFD(fd int, content []byte) error {
	for len(content) > 0 {
		n, err := stageUnixWrite(fd, content)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		content = content[n:]
	}
	return nil
}
