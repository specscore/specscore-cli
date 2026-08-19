//go:build !linux && !darwin && !windows

package lifecycle

import "fmt"

func artifactRenameNoReplace(oldPath, newPath string) error {
	return fmt.Errorf("atomic no-replace artifact publication is unsupported on this platform: %s -> %s", oldPath, newPath)
}
