package rule

import "os"

// Filesystem seams. Every I/O call in this package goes through one of these
// vars so tests can drive the error branches that a real filesystem will not
// reliably produce (a rename that fails after a successful write, a directory
// that cannot be fsynced, a short write). Production always holds the stdlib
// function; tests restore the original with a t.Cleanup.
var (
	osStat       = os.Stat
	osReadFile   = os.ReadFile
	osWriteFile  = os.WriteFile
	osMkdirAll   = os.MkdirAll
	osReadDir    = os.ReadDir
	osOpen       = os.Open
	osRename     = os.Rename
	osRemove     = os.Remove
	osCreateTemp = func(dir, pattern string) (tempFile, error) { return os.CreateTemp(dir, pattern) }
)

// tempFile is the subset of *os.File the atomic writer needs, so a test can
// substitute a file whose Write, Sync, Chmod, or Close fails.
type tempFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

// syncCloser is the subset of *os.File the atomic writer needs from the
// containing directory handle.
type syncCloser interface {
	Sync() error
	Close() error
}

// osOpenDir is separated from osOpen so a test can fail the directory fsync
// without also failing every artifact read.
var osOpenDir = func(path string) (syncCloser, error) { return os.Open(path) }
