package runner

import (
	"os"
	"path/filepath"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	walkDirFn   = filepath.WalkDir
	osStatFn    = os.Stat
	mkdirTempFn = os.MkdirTemp
)
