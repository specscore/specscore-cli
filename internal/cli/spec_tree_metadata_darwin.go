//go:build darwin

package cli

import (
	"fmt"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

var (
	stageDarwinFstat    = unix.Fstat
	stageDarwinFchflags = unix.Fchflags
)

func snapshotPlatformFlags(_ int, info os.FileInfo) (uint32, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	return stat.Flags, nil
}

// applyStagedPlatformFlags accepts inherited Darwin flags without requiring a
// privileged write. When inheritance did not reproduce the snapshot, it uses
// the held descriptor and verifies the exact result before publication.
func applyStagedPlatformFlags(fd int, expected uint32) error {
	var current unix.Stat_t
	if err := stageDarwinFstat(fd, &current); err != nil {
		return fmt.Errorf("inspecting staged filesystem flags: %w", err)
	}
	if current.Flags == expected {
		return nil
	}
	if err := stageDarwinFchflags(fd, int(expected)); err != nil {
		return fmt.Errorf("preserving filesystem flags %#x from staged value %#x: %w", expected, current.Flags, err)
	}
	if err := stageDarwinFstat(fd, &current); err != nil {
		return fmt.Errorf("verifying staged filesystem flags: %w", err)
	}
	if current.Flags != expected {
		return fmt.Errorf("staged filesystem flags are %#x after preservation; want %#x", current.Flags, expected)
	}
	return nil
}
