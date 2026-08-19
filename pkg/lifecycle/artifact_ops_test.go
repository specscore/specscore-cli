package lifecycle

import (
	"os"
)

// transformToWithOps mirrors transformTo (coverage_test.go) but threads a
// caller-supplied artifactTransactionOps through the unexported entry point,
// so tests can inject a fault at exactly one seam (a single field override
// on top of defaultArtifactTransactionOps()) while every other operation
// still touches the real filesystem.
func transformToWithOps(path string, content []byte, ops artifactTransactionOps) error {
	return transformArtifactWithOps(path, ops, func([]byte) ([]byte, error) { return content, nil })
}

// identityFault wraps a real artifactIdentityFile (typically one opened via
// the real openArtifactIdentity, so the open itself and any untouched method
// still exercise real filesystem behavior) and substitutes a fixed error for
// one or more of Read/Stat/Close. Close always still runs the wrapped
// Close so no file descriptor leaks in tests, joining in the injected error
// when both are non-nil would only be needed if a test set both — none do.
type identityFault struct {
	artifactIdentityFile
	readErr  error
	statErr  error
	closeErr error
}

func (f identityFault) Read(p []byte) (int, error) {
	if f.readErr != nil {
		return 0, f.readErr
	}
	return f.artifactIdentityFile.Read(p)
}

func (f identityFault) Stat() (os.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	return f.artifactIdentityFile.Stat()
}

func (f identityFault) Close() error {
	err := f.artifactIdentityFile.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}

// txFault wraps a real artifactTransactionFile (a *os.File from a real
// os.CreateTemp/os.Open, so the file genuinely exists on disk) and
// substitutes a fixed error for one of Write/Chmod/Sync/Close, exercising
// writeFileAtomicExpectedWithOps's failure-handling around a temp file or
// directory handle without needing an in-memory fake for the rest of its
// behavior (Name still returns the real, cleanable path).
type txFault struct {
	artifactTransactionFile
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (f txFault) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.artifactTransactionFile.Write(p)
}

func (f txFault) Chmod(mode os.FileMode) error {
	if f.chmodErr != nil {
		return f.chmodErr
	}
	return f.artifactTransactionFile.Chmod(mode)
}

func (f txFault) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.artifactTransactionFile.Sync()
}

func (f txFault) Close() error {
	err := f.artifactTransactionFile.Close()
	if f.closeErr != nil {
		return f.closeErr
	}
	return err
}
