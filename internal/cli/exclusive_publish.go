package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

type exclusivePublishFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type exclusivePublishOps struct {
	mkdirAll   func(string, os.FileMode) error
	createTemp func(string, string) (exclusivePublishFile, error)
	link       func(string, string) error
	remove     func(string) error
	open       func(string) (exclusivePublishFile, error)
}

func defaultExclusivePublishOps() exclusivePublishOps {
	return exclusivePublishOps{
		mkdirAll: os.MkdirAll,
		createTemp: func(dir, pattern string) (exclusivePublishFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		link:   os.Link,
		remove: os.Remove,
		open: func(path string) (exclusivePublishFile, error) {
			return os.Open(path)
		},
	}
}

func publishFileExclusive(path string, data []byte, mode os.FileMode) error {
	return publishFileExclusiveWithOps(path, data, mode, defaultExclusivePublishOps())
}

func publishFileExclusiveWithOps(path string, data []byte, mode os.FileMode, ops exclusivePublishOps) error {
	dir := filepath.Dir(path)
	if err := ops.mkdirAll(dir, 0o755); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: err}
	}
	f, err := ops.createTemp(dir, ".specscore-publish-")
	if err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: err}
	}
	tmp := f.Name()
	defer func() { _ = ops.remove(tmp) }()
	prepublication := func(err error) error {
		_ = f.Close()
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: err}
	}
	if err := f.Chmod(mode); err != nil {
		return prepublication(err)
	}
	if n, err := f.Write(data); err != nil {
		return prepublication(err)
	} else if n != len(data) {
		return prepublication(io.ErrShortWrite)
	}
	if err := f.Sync(); err != nil {
		return prepublication(err)
	}
	if err := f.Close(); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: err}
	}
	if err := ops.link(tmp, path); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: err}
	}
	d, err := ops.open(dir)
	if err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("opening publication directory after exclusive link: %w", err)}
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("syncing publication directory after exclusive link: %w", err)}
	}
	if err := d.Close(); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("closing publication directory after exclusive link: %w", err)}
	}
	return nil
}

func isExclusivePublishCollision(err error) bool {
	return errors.Is(err, fs.ErrExist)
}

type ownedMarkerOps struct {
	link    func(string, string) error
	remove  func(string) error
	syncDir func(string) error
	read    func(string) ([]byte, error)
	stat    func(string) (os.FileInfo, error)
}

func defaultOwnedMarkerOps() ownedMarkerOps {
	return ownedMarkerOps{link: os.Link, remove: os.Remove, syncDir: syncOwnedMarkerDir, read: os.ReadFile, stat: os.Stat}
}

func committedMarkerPath(path string) string { return path + ".committed" }

type syncCloseFile interface {
	Sync() error
	Close() error
}

func syncOwnedMarkerDir(dir string) error {
	return syncOwnedMarkerDirWithOpen(dir, func(path string) (syncCloseFile, error) { return os.Open(path) })
}

func syncOwnedMarkerDirWithOpen(dir string, open func(string) (syncCloseFile, error)) error {
	d, err := open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

// removeOwnedFileDurable finalizes a private prepared marker without an
// unrecoverable remove-then-sync gap. It first creates and durably fences an
// exclusive hard-linked committed receipt carrying the same owned bytes. Only
// then may it remove the prepared name. Any failure before cleanup leaves at
// least one exact receipt for retry; a foreign receipt is never overwritten.
// Once removal of the final receipt succeeds, a following directory-sync
// failure can at worst resurrect that harmless committed receipt after a
// crash, so it is intentionally best-effort rather than a false recovery error.
func removeOwnedFileDurable(path string, expected []byte) error {
	return removeOwnedFileDurableWithOps(path, expected, defaultOwnedMarkerOps())
}

func removeOwnedFileDurableWithOps(path string, expected []byte, ops ownedMarkerOps) error {
	finalPath := committedMarkerPath(path)
	prepared, preparedErr := ops.read(path)
	committed, committedErr := ops.read(finalPath)
	preparedExists := preparedErr == nil
	committedExists := committedErr == nil
	if preparedErr != nil && !os.IsNotExist(preparedErr) {
		return preparedErr
	}
	if committedErr != nil && !os.IsNotExist(committedErr) {
		return committedErr
	}
	if !preparedExists && !committedExists {
		return os.ErrNotExist
	}
	if (preparedExists && !bytes.Equal(prepared, expected)) || (committedExists && !bytes.Equal(committed, expected)) {
		return fmt.Errorf("prepared marker changed before finalization")
	}
	dir := filepath.Dir(path)
	if !committedExists {
		// Revalidate immediately before linking the pathname. A non-cooperating
		// writer may have replaced it after the initial read.
		if err := verifyOwnedMarker(ops, path, expected); err != nil {
			return err
		}
		if err := ops.link(path, finalPath); err != nil {
			return err
		}
		committedExists = true
	}
	// The two visible names must still carry the owned bytes and, while both
	// exist, identify the same inode created by the hard-link receipt step.
	// This detects a pathname replacement before either name is removed.
	if preparedExists {
		if err := verifyOwnedMarkerPair(ops, path, finalPath, expected); err != nil {
			return err
		}
	} else if err := verifyOwnedMarker(ops, finalPath, expected); err != nil {
		return err
	}
	if err := ops.syncDir(dir); err != nil {
		return err
	}
	if preparedExists {
		if err := verifyOwnedMarkerPair(ops, path, finalPath, expected); err != nil {
			return err
		}
		if err := ops.remove(path); err != nil {
			return err
		}
		if err := ops.syncDir(dir); err != nil {
			return err
		}
	}
	if committedExists {
		if err := verifyOwnedMarker(ops, finalPath, expected); err != nil {
			return err
		}
		if err := ops.remove(finalPath); err != nil {
			return err
		}
		// The committed receipt was durably established above. If this final
		// cleanup fence fails, returning an error would demand a retry without
		// leaving a currently visible ownership receipt. Treat deletion as done;
		// a crash may only bring the harmless receipt back for later cleanup.
		_ = ops.syncDir(dir)
	}
	return nil
}

func verifyOwnedMarker(ops ownedMarkerOps, path string, expected []byte) error {
	got, err := ops.read(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, expected) {
		return fmt.Errorf("prepared marker changed before finalization")
	}
	return nil
}

func verifyOwnedMarkerPair(ops ownedMarkerOps, preparedPath, committedPath string, expected []byte) error {
	if err := verifyOwnedMarker(ops, preparedPath, expected); err != nil {
		return err
	}
	if err := verifyOwnedMarker(ops, committedPath, expected); err != nil {
		return err
	}
	preparedInfo, err := ops.stat(preparedPath)
	if err != nil {
		return err
	}
	committedInfo, err := ops.stat(committedPath)
	if err != nil {
		return err
	}
	if !os.SameFile(preparedInfo, committedInfo) {
		return fmt.Errorf("prepared marker identity changed before finalization")
	}
	return nil
}
