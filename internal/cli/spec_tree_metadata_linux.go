//go:build linux

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// validateSnapshotPlatformMetadata rejects filesystem flags rather than
// dropping them. Setting immutable/append-only flags is privilege-sensitive,
// so a copy-on-write transaction cannot prove it will restore them exactly.
func validateSnapshotPlatformMetadata(fd int, _ os.FileInfo) error {
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return fmt.Errorf("inspecting filesystem flags: %w", err)
	}
	if flags != 0 {
		return fmt.Errorf("filesystem flags %#x cannot be preserved by isolated lifecycle transaction", flags)
	}
	return nil
}
