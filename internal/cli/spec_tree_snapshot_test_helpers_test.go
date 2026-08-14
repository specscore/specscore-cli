package cli

import (
	"os"
	"testing"
)

var lifecycleTransactionTestWriteSet = []string{
	"expected.md",
	"new.md",
	"published.md",
}

func resetSpecTreeSnapshotSeams(t *testing.T) {
	t.Helper()
	readFile, openFile := transactionReadFile, transactionOpenFile
	closeFile, lstat := transactionCloseFile, transactionLstat
	mkdirTemp, removeAll := transactionMkdirTemp, transactionRemoveAll
	closeStaged := transactionCloseStagedTree
	lockFile, alive := transactionLockFile, transactionProcessAlive
	platform := transactionPlatformSupportsSecureMutation
	t.Cleanup(func() {
		transactionReadFile, transactionOpenFile = readFile, openFile
		transactionCloseFile, transactionLstat = closeFile, lstat
		transactionMkdirTemp, transactionRemoveAll = mkdirTemp, removeAll
		transactionCloseStagedTree = closeStaged
		transactionLockFile, transactionProcessAlive = lockFile, alive
		transactionPlatformSupportsSecureMutation = platform
	})
}

func rootSnapshot(files map[string]string, dirs ...string) specTreeSnapshot {
	directories := map[string]specTreeDirectory{".": {mode: os.ModeDir | 0o755}}
	for _, dir := range dirs {
		directories[dir] = specTreeDirectory{mode: os.ModeDir | 0o755}
	}
	result := specTreeSnapshot{directories: directories, files: map[string]specTreeFile{}}
	for name, content := range files {
		result.files[name] = specTreeFile{content: []byte(content), mode: 0o644}
	}
	return result
}
