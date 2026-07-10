package runner

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	walkDirFn   = filepath.WalkDir
	osStatFn    = os.Stat
	mkdirTempFn = os.MkdirTemp
	lookPathFn  = exec.LookPath

	// ExecCommandFn is the test seam for git invocations: it runs name with
	// args in dir and returns stdout. Exported so CLI-layer tests can control
	// git provenance output without touching the filesystem.
	// Feature: cli/rehearse/evidence (REQ: report-provenance)
	ExecCommandFn = execCommandOutput

	// execNewCommandFn is the test seam for constructing exec.Cmd instances.
	// Feature: cli/rehearse/evidence (REQ: report-provenance)
	execNewCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command(name, args...)
	}
)
