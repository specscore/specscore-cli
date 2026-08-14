package cli

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// task_stubs.go provides var-based seams for task.go error-path testing.

var (
	osReadFileFn  = os.ReadFile
	osWriteFileFn = os.WriteFile
	osGetwdFn     = os.Getwd
	filepathAbsFn = filepath.Abs
)

// taskMutationDeps keeps every transaction/fault dependency scoped to one
// command tree. Tests construct an isolated tree instead of mutating package
// globals, so concurrent command tests cannot leak fault injection.
type taskMutationDeps struct {
	rewriteBoardTask       func(string, lifecycle.Status, []string) (lifecycle.Status, error)
	transformArtifact      func(string, func([]byte) ([]byte, error)) error
	withArtifactTx         func(string, func(*lifecycle.ArtifactTransaction) error) error
	publishExclusive       func(string, []byte, os.FileMode) error
	removeMarker           func(string, []byte) error
	commitBoard            func(*lifecycle.ArtifactTransaction, []byte) error
	stat                   func(string) (os.FileInfo, error)
	annotationAmendmentNow func() time.Time
	rewritePlanTaskStatus  func([]byte, int, lifecycle.Status, []string) ([]byte, error)
	amendPlanProvenance    func([]byte, int, string) ([]byte, error)
	newYAMLEncoder         func(io.Writer) yamlEnc
	newJSONEncoder         func(io.Writer) jsonEnc
}

func defaultTaskMutationDeps() taskMutationDeps {
	return taskMutationDeps{
		rewriteBoardTask:       rewriteBoardTask,
		transformArtifact:      lifecycle.TransformArtifact,
		withArtifactTx:         lifecycle.WithArtifactTransaction,
		publishExclusive:       publishFileExclusive,
		removeMarker:           removeOwnedFileDurable,
		commitBoard:            func(tx *lifecycle.ArtifactTransaction, after []byte) error { return tx.Commit(after) },
		stat:                   os.Stat,
		annotationAmendmentNow: time.Now,
		rewritePlanTaskStatus:  rewritePlanTaskStatusLineBytes,
		amendPlanProvenance:    amendPlanImplementedByBytes,
		newYAMLEncoder:         newYAMLEnc,
		newJSONEncoder:         newJSONEnc,
	}
}

func (d taskMutationDeps) withDefaults() taskMutationDeps {
	defaults := defaultTaskMutationDeps()
	if d.rewriteBoardTask == nil {
		d.rewriteBoardTask = defaults.rewriteBoardTask
	}
	if d.transformArtifact == nil {
		d.transformArtifact = defaults.transformArtifact
	}
	if d.withArtifactTx == nil {
		d.withArtifactTx = defaults.withArtifactTx
	}
	if d.publishExclusive == nil {
		d.publishExclusive = defaults.publishExclusive
	}
	if d.removeMarker == nil {
		d.removeMarker = defaults.removeMarker
	}
	if d.commitBoard == nil {
		d.commitBoard = defaults.commitBoard
	}
	if d.stat == nil {
		d.stat = defaults.stat
	}
	if d.annotationAmendmentNow == nil {
		d.annotationAmendmentNow = defaults.annotationAmendmentNow
	}
	if d.rewritePlanTaskStatus == nil {
		d.rewritePlanTaskStatus = defaults.rewritePlanTaskStatus
	}
	if d.amendPlanProvenance == nil {
		d.amendPlanProvenance = defaults.amendPlanProvenance
	}
	if d.newYAMLEncoder == nil {
		d.newYAMLEncoder = defaults.newYAMLEncoder
	}
	if d.newJSONEncoder == nil {
		d.newJSONEncoder = defaults.newJSONEncoder
	}
	return d
}
