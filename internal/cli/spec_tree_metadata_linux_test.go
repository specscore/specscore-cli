//go:build linux

package cli

import (
	"errors"
	"testing"
)

func TestLinuxFilesystemFlagContract(t *testing.T) {
	original := linuxFilesystemFlags
	t.Cleanup(func() { linuxFilesystemFlags = original })

	t.Run("kernel-managed extent layout is ignored", func(t *testing.T) {
		linuxFilesystemFlags = func(int, uint) (int, error) { return linuxFilesystemManagedFlags, nil }
		flags, err := snapshotPlatformFlags(3, nil)
		if err != nil || flags != 0 {
			t.Fatalf("snapshot flags = %#x, %v", flags, err)
		}
		if err := applyStagedPlatformFlags(3, 0); err != nil {
			t.Fatalf("apply managed flags: %v", err)
		}
	})

	t.Run("semantic flags remain fail closed", func(t *testing.T) {
		linuxFilesystemFlags = func(int, uint) (int, error) {
			const fsImmutableFlag = 0x00000010
			return linuxFilesystemManagedFlags | fsImmutableFlag, nil
		}
		if _, err := snapshotPlatformFlags(3, nil); err == nil || !contains(err, "cannot be preserved") {
			t.Fatalf("snapshot semantic flag error = %v", err)
		}
		if err := applyStagedPlatformFlags(3, 0); err == nil || !contains(err, "declarative filesystem flags") {
			t.Fatalf("apply semantic flag error = %v", err)
		}
	})

	t.Run("ioctl failures propagate", func(t *testing.T) {
		linuxFilesystemFlags = func(int, uint) (int, error) { return 0, errors.New("ioctl failed") }
		if _, err := snapshotPlatformFlags(3, nil); err == nil || !contains(err, "ioctl failed") {
			t.Fatalf("snapshot ioctl error = %v", err)
		}
		if err := applyStagedPlatformFlags(3, 0); err == nil || !contains(err, "ioctl failed") {
			t.Fatalf("apply ioctl error = %v", err)
		}
	})
}
