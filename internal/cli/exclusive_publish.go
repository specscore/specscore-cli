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

// removeOwnedFileDurable removes a private prepared marker only while its
// bytes still match the owning transaction, then syncs the containing
// directory. A mismatch is retained for explicit recovery rather than deleting
// a foreign replacement.
func removeOwnedFileDurable(path string, expected []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return fmt.Errorf("prepared marker changed before finalization")
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}
