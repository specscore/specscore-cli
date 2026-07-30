//go:build darwin

package cli

import (
	"fmt"
	"os"
	"syscall"
)

// Darwin exposes BSD file flags in stat metadata. Recreating those flags can
// require elevated authority, therefore nonzero flags fail closed.
func validateSnapshotPlatformMetadata(_ int, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	if stat.Flags != 0 {
		return fmt.Errorf("filesystem flags %#x cannot be preserved by isolated lifecycle transaction", stat.Flags)
	}
	return nil
}
