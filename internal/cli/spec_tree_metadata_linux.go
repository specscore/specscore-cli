//go:build linux

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// snapshotPlatformFlags rejects filesystem flags rather than
// dropping them. Setting immutable/append-only flags is privilege-sensitive,
// so a copy-on-write transaction cannot prove it will restore them exactly.
func snapshotPlatformFlags(fd int, _ os.FileInfo) (uint32, error) {
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return 0, fmt.Errorf("inspecting filesystem flags: %w", err)
	}
	if flags != 0 {
		return 0, fmt.Errorf("filesystem flags %#x cannot be preserved by isolated lifecycle transaction", flags)
	}
	return 0, nil
}

func applyStagedPlatformFlags(fd int, expected uint32) error {
	flags, err := unix.IoctlGetInt(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return fmt.Errorf("inspecting staged filesystem flags: %w", err)
	}
	if uint32(flags) != expected {
		return fmt.Errorf("staged filesystem flags are %#x; want %#x", flags, expected)
	}
	return nil
}
