//go:build darwin || linux

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Test seams for descriptor-relative traversal. The entry hook runs after a
// name was enumerated but before its descriptor is opened, which is precisely
// the historical pathname swap window. The reader receives the already-open
// descriptor and can never redirect traversal through a replacement path.
var (
	transactionBeforeSnapshotOpenat = func(directory, rel, name string) {}
	snapshotOpenRoot                = unix.Open
	snapshotOpenAt                  = unix.Openat
	snapshotFileStat                = func(file *os.File) (os.FileInfo, error) { return file.Stat() }
	snapshotReadDirNames            = func(file *os.File) ([]string, error) { return file.Readdirnames(-1) }
	snapshotClose                   = func(file *os.File) error { return file.Close() }
)

// snapshotSpecTreeNoFollow walks using directory descriptors and opens every
// child relative to its already-open parent with O_NOFOLLOW.  A pathname walk
// followed by ReadFile is not safe here: an untrusted writer can replace a
// component with a symlink between those two calls.  The resulting descriptor
// names a stable inode, so a replacement is either observed as a new directory
// entry on the next snapshot or is retained in the recovery tree at publish.
func snapshotSpecTreeNoFollow(specRoot string) (specTreeSnapshot, error) {
	fd, err := snapshotOpenRoot(specRoot, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("opening spec tree without following links: %w", err)
	}
	root := os.NewFile(uintptr(fd), specRoot)
	defer func() { _ = snapshotClose(root) }()
	info, err := snapshotFileStat(root)
	if err != nil {
		return specTreeSnapshot{}, fmt.Errorf("statting spec tree descriptor: %w", err)
	}
	snapshot := specTreeSnapshot{
		files:       make(map[string]specTreeFile),
		directories: map[string]os.FileMode{".": info.Mode()},
	}
	if err := snapshotDirectoryNoFollow(root, ".", &snapshot); err != nil {
		return specTreeSnapshot{}, fmt.Errorf("snapshotting spec tree: %w", err)
	}
	return snapshot, nil
}

func snapshotDirectoryNoFollow(directory *os.File, rel string, snapshot *specTreeSnapshot) error {
	names, err := snapshotReadDirNames(directory)
	if err != nil {
		return fmt.Errorf("reading directory %s: %w", rel, err)
	}
	for _, name := range names {
		// Readdirnames never returns these, but reject them defensively before
		// passing the name to openat.
		if name == "." || name == ".." || filepath.Base(name) != name {
			return fmt.Errorf("invalid directory entry %q under %s", name, rel)
		}
		transactionBeforeSnapshotOpenat(directory.Name(), rel, name)
		fd, err := snapshotOpenAt(int(directory.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			if err == unix.ELOOP {
				// Symlinks have never been part of a SpecScore transaction
				// snapshot.  Do not follow them and do not let them become an
				// error path that leaks their target.
				continue
			}
			return fmt.Errorf("opening %s without following links: %w", filepath.Join(rel, name), err)
		}
		child := os.NewFile(uintptr(fd), name)
		childInfo, statErr := snapshotFileStat(child)
		if statErr != nil {
			_ = snapshotClose(child)
			return fmt.Errorf("statting %s: %w", filepath.Join(rel, name), statErr)
		}
		childRel := filepath.Join(rel, name)
		if rel == "." {
			childRel = name
		}
		if childInfo.IsDir() {
			snapshot.directories[childRel] = childInfo.Mode()
			if err := snapshotDirectoryNoFollow(child, childRel, snapshot); err != nil {
				_ = snapshotClose(child)
				return err
			}
			if err := snapshotClose(child); err != nil {
				return fmt.Errorf("closing directory %s: %w", childRel, err)
			}
			continue
		}
		if !childInfo.Mode().IsRegular() {
			_ = snapshotClose(child)
			continue
		}
		content, readErr := transactionReadSnapshotFile(child)
		if readErr != nil {
			_ = snapshotClose(child)
			return fmt.Errorf("reading %s through stable descriptor: %w", childRel, readErr)
		}
		// Re-stat the same descriptor. A write that changes its observable
		// identity while we read is a concurrent mutation; fail rather than
		// applying a lint result derived from a torn input.
		afterRead, statErr := snapshotFileStat(child)
		closeErr := snapshotClose(child)
		if statErr != nil {
			return fmt.Errorf("re-statting %s: %w", childRel, statErr)
		}
		if closeErr != nil {
			return fmt.Errorf("closing %s: %w", childRel, closeErr)
		}
		if childInfo.Mode() != afterRead.Mode() || childInfo.Size() != afterRead.Size() || !childInfo.ModTime().Equal(afterRead.ModTime()) {
			return fmt.Errorf("concurrent modification while reading %s", childRel)
		}
		snapshot.files[childRel] = specTreeFile{content: content, mode: childInfo.Mode()}
	}
	return nil
}
