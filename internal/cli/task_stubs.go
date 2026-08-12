package cli

import (
	"os"
	"path/filepath"
)

// task_stubs.go provides var-based seams for task.go error-path testing.

var (
	osReadFileFn                = os.ReadFile
	osWriteFileFn               = os.WriteFile
	osGetwdFn                   = os.Getwd
	filepathAbsFn               = filepath.Abs
	rewriteBoardTaskFn          = rewriteBoardTask
	rewritePlanTaskStatusLineFn = rewritePlanTaskStatusLine
)
