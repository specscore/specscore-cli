package graph

import "os"

// Test seams for filesystem reads. Production calls these vars; tests replace
// them (with cleanup-restored swaps) to cover the defensive I/O error branches
// that a real fixture tree cannot reach.
var (
	readFileFn  = os.ReadFile
	readDirFn   = os.ReadDir
	writeFileFn = os.WriteFile
)
