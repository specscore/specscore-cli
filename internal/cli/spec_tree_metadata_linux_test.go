//go:build linux

package cli

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
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

func TestSnapshotEntryTimes_Linux(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "timestamps")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	accessed, modified, err := snapshotEntryTimes(info)
	if err != nil || accessed.IsZero() || modified.IsZero() {
		t.Fatalf("snapshotEntryTimes = %v, %v, %v", accessed, modified, err)
	}
	if _, _, err := snapshotEntryTimes(lifecycleTestFileInfo{mode: 0o644}); err == nil {
		t.Fatal("unsupported timestamp stat accepted")
	}
}

func TestSetStagedEntryTimes_Linux(t *testing.T) {
	original := stageLinuxUtimesNanoAt
	called := 0
	stageLinuxUtimesNanoAt = func(_ int, _ string, _ []unix.Timespec, _ int) error {
		called++
		return nil
	}
	t.Cleanup(func() { stageLinuxUtimesNanoAt = original })

	now := time.Unix(1_700_000_000, 123_456_789)
	if err := setStagedEntryTimes(-1, time.Time{}, now); err != nil || called != 1 {
		t.Fatalf("zero access fallback = %v, calls=%d", err, called)
	}
	if err := setStagedEntryTimes(-1, now, time.Time{}); err != nil || called != 2 {
		t.Fatalf("zero modification fallback = %v, calls=%d", err, called)
	}
	if err := setStagedEntryModificationTime(-1, now); err != nil || called != 3 {
		t.Fatalf("modification time helper = %v, calls=%d", err, called)
	}
}
