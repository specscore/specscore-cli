package lifecycle

import (
	"errors"
	"io"
	"os"
)

type artifactIdentityFile interface {
	Read([]byte) (int, error)
	Stat() (os.FileInfo, error)
	Close() error
}

type artifactTransactionFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type artifactTransactionOps struct {
	newLock         func(string) artifactLock
	readFile        func(string) ([]byte, error)
	lstat           func(string) (os.FileInfo, error)
	openIdentity    func(string) (artifactIdentityFile, error)
	createTemp      func(string, string) (artifactTransactionFile, error)
	copy            func(io.Writer, io.Reader) (int64, error)
	renameNoReplace func(string, string) error
	remove          func(string) error
	openDir         func(string) (artifactTransactionFile, error)
}

func defaultArtifactTransactionOps() artifactTransactionOps {
	return artifactTransactionOps{
		newLock:  newFlockArtifactLock,
		readFile: os.ReadFile,
		lstat:    os.Lstat,
		openIdentity: func(path string) (artifactIdentityFile, error) {
			return openArtifactIdentity(path)
		},
		createTemp: func(dir, pattern string) (artifactTransactionFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		copy:            io.Copy,
		renameNoReplace: artifactRenameNoReplace,
		remove:          os.Remove,
		openDir: func(path string) (artifactTransactionFile, error) {
			return os.Open(path)
		},
	}
}

type artifactRoot interface {
	OpenFile(string, int, os.FileMode) (*os.File, error)
	Close() error
}

func openArtifactIdentity(path string) (artifactIdentityFile, error) {
	return openArtifactIdentityWithRoot(path, func(dir string) (artifactRoot, error) {
		return os.OpenRoot(dir)
	})
}

func openArtifactIdentityWithRoot(path string, openRoot func(string) (artifactRoot, error)) (artifactIdentityFile, error) {
	root, err := openRoot(dirOf(path))
	if err != nil {
		return nil, err
	}
	f, openErr := root.OpenFile(baseOf(path), artifactIdentityOpenFlags(), 0)
	closeErr := root.Close()
	if openErr != nil {
		return nil, openErr
	}
	if closeErr != nil {
		return nil, errors.Join(closeErr, f.Close())
	}
	return f, nil
}
