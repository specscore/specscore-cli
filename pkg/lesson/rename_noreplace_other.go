//go:build !linux && !darwin && !windows

package lesson

import (
	"fmt"
)

func renameNoReplace(oldPath, newPath string) error {
	return fmt.Errorf("atomic no-replace move is unsupported on this platform: %s -> %s", oldPath, newPath)
}
