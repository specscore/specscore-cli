//go:build linux

package cli

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// FS_EXTENT_FL describes ext4's internal on-disk allocation format, not
// user-authored file semantics. Ordinary files and directories may acquire it
// automatically, and a copy-on-write replacement may acquire it independently.
// Comparing or replaying it would reject normal ext4 trees without preserving
// any observable contract. Every other reported flag remains fail-closed.
const linuxFilesystemManagedFlags = 0x00080000

var linuxFilesystemFlags = unix.IoctlGetInt

func linuxDeclarativeFilesystemFlags(flags int) uint32 {
	return uint32(flags &^ linuxFilesystemManagedFlags)
}

// snapshotPlatformFlags rejects filesystem flags rather than
// dropping them. Setting immutable/append-only flags is privilege-sensitive,
// so a copy-on-write transaction cannot prove it will restore them exactly.
func snapshotPlatformFlags(fd int, _ os.FileInfo) (uint32, error) {
	flags, err := linuxFilesystemFlags(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return 0, fmt.Errorf("inspecting filesystem flags: %w", err)
	}
	if linuxDeclarativeFilesystemFlags(flags) != 0 {
		return 0, fmt.Errorf("filesystem flags %#x cannot be preserved by isolated lifecycle transaction", flags)
	}
	return 0, nil
}

func applyStagedPlatformFlags(fd int, expected uint32) error {
	flags, err := linuxFilesystemFlags(fd, unix.FS_IOC_GETFLAGS)
	if err != nil {
		return fmt.Errorf("inspecting staged filesystem flags: %w", err)
	}
	actual := linuxDeclarativeFilesystemFlags(flags)
	if actual != expected {
		return fmt.Errorf("staged declarative filesystem flags are %#x (raw %#x); want %#x", actual, flags, expected)
	}
	return nil
}
