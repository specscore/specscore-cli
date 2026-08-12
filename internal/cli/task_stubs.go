package cli

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// task_stubs.go provides var-based seams for task.go error-path testing.

var (
	osReadFileFn              = os.ReadFile
	osWriteFileFn             = os.WriteFile
	osGetwdFn                 = os.Getwd
	filepathAbsFn             = filepath.Abs
	rewriteBoardTaskFn        = rewriteBoardTask
	taskTransformArtifactFn   = lifecycle.TransformArtifact
	taskWithArtifactTxFn      = lifecycle.WithArtifactTransaction
	taskNewPublishExclusiveFn = publishFileExclusive
	taskNewRemoveMarkerFn     = removeOwnedFileDurable
	taskNewCommitBoardFn      = func(tx *lifecycle.ArtifactTransaction, after []byte) error { return tx.Commit(after) }
	taskNewStatFn             = os.Stat
)
