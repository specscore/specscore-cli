//go:build darwin

package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSetStagedEntryModificationTime_DarwinError(t *testing.T) {
	original := stageDarwinFsetattrlist
	stageDarwinFsetattrlist = func(int, time.Time) error { return errors.New("fsetattrlist failed") }
	t.Cleanup(func() { stageDarwinFsetattrlist = original })
	if err := setStagedEntryModificationTime(-1, nowForLifecycleTimeTest()); err == nil || !contains(err, "fsetattrlist failed") {
		t.Fatalf("timestamp error = %v", err)
	}
	if err := applyStagedEntryMetadata(-1, specTreeEntryMetadata{modificationTime: nowForLifecycleTimeTest()}); err == nil || !contains(err, "fsetattrlist failed") {
		t.Fatalf("metadata timestamp error = %v", err)
	}
	if err := fsetStagedEntryModificationTime(-1, nowForLifecycleTimeTest()); err == nil {
		t.Fatal("raw fsetattrlist unexpectedly succeeded for an invalid descriptor")
	}
	called := 0
	stageDarwinFsetattrlist = func(_ int, value time.Time) error {
		called++
		if value.IsZero() {
			t.Fatal("zero timestamp passed to fsetattrlist")
		}
		return nil
	}
	now := nowForLifecycleTimeTest()
	if err := setStagedEntryTimes(-1, time.Time{}, now); err != nil || called != 1 {
		t.Fatalf("zero access fallback = %v, calls=%d", err, called)
	}
	if err := setStagedEntryTimes(-1, now, time.Time{}); err != nil || called != 2 {
		t.Fatalf("zero modification fallback = %v, calls=%d", err, called)
	}
	if err := setStagedEntryTimes(-1, now, now.Add(time.Nanosecond)); err == nil {
		t.Fatal("distinct timestamps unexpectedly succeeded for an invalid descriptor")
	}
}

func TestSnapshotEntryTimes_Darwin(t *testing.T) {
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
	if _, _, err := snapshotEntryTimes(fileInfoWithSys{FileInfo: info}); err == nil {
		t.Fatal("unsupported timestamp stat accepted")
	}
}

func TestSnapshotMetadataPreservesDarwinFlagsAndRejectsACLXattrs(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	flags, err := snapshotPlatformFlags(int(file.Fd()), info)
	if err != nil || flags != 0 {
		t.Fatalf("ordinary file metadata rejected: %v", err)
	}
	const restricted = uint32(0x00080000) // SF_RESTRICTED from sys/stat.h.
	flagged := fileInfoWithSys{FileInfo: info, sys: &syscall.Stat_t{Flags: restricted}}
	flags, err = snapshotPlatformFlags(int(file.Fd()), flagged)
	if err != nil || flags != restricted {
		t.Fatalf("Darwin filesystem flags = %#x, %v", flags, err)
	}
	if _, err := snapshotPlatformFlags(int(file.Fd()), fileInfoWithSys{FileInfo: info}); err == nil {
		t.Fatal("unsupported stat metadata accepted")
	}
	platformStat := *(info.Sys().(*syscall.Stat_t))
	platformStat.Flags = restricted
	metadata, err := captureSnapshotEntryMetadata(file, fileInfoWithSys{FileInfo: info, sys: &platformStat})
	if err != nil || metadata.platformFlags != restricted {
		t.Fatalf("captured filesystem flags = %#x, %v", metadata.platformFlags, err)
	}
	hardLinkedStat := *(info.Sys().(*syscall.Stat_t))
	hardLinkedStat.Nlink = 2
	if _, err := captureSnapshotEntryMetadata(file, fileInfoWithSys{FileInfo: info, sys: &hardLinkedStat}); err == nil || !contains(err, "hard-linked") {
		t.Fatalf("capture accepted hard-linked file: %v", err)
	}
	for _, name := range []string{"system.posix_acl_access", "system.posix_acl_default", "security.capability", "com.apple.macl"} {
		if !isUnpreservableSpecTreeXattr(name) {
			t.Fatalf("security xattr %q accepted", name)
		}
	}
	if isUnpreservableSpecTreeXattr("user.safe") {
		t.Fatal("ordinary xattr rejected")
	}
}

func TestApplyStagedPlatformFlagsDarwin(t *testing.T) {
	originalFstat, originalFchflags := stageDarwinFstat, stageDarwinFchflags
	t.Cleanup(func() {
		stageDarwinFstat, stageDarwinFchflags = originalFstat, originalFchflags
	})

	const restricted = uint32(0x00080000)
	current := restricted
	stageDarwinFstat = func(_ int, stat *unix.Stat_t) error {
		stat.Flags = current
		return nil
	}
	setCalls := 0
	stageDarwinFchflags = func(_ int, flags int) error {
		setCalls++
		current = uint32(flags)
		return nil
	}
	if err := applyStagedPlatformFlags(3, restricted); err != nil || setCalls != 0 {
		t.Fatalf("inherited flags = %v, set calls=%d", err, setCalls)
	}
	current = 0
	if err := applyStagedPlatformFlags(3, restricted); err != nil || setCalls != 1 {
		t.Fatalf("descriptor-preserved flags = %v, set calls=%d", err, setCalls)
	}

	current = 0
	stageDarwinFchflags = func(int, int) error { return errors.New("fchflags denied") }
	if err := applyStagedPlatformFlags(3, restricted); err == nil || !contains(err, "fchflags denied") {
		t.Fatalf("fchflags failure = %v", err)
	}
	stageDarwinFstat = func(int, *unix.Stat_t) error { return errors.New("fstat failed") }
	if err := applyStagedPlatformFlags(3, restricted); err == nil || !contains(err, "fstat failed") {
		t.Fatalf("fstat failure = %v", err)
	}
}

type fileInfoWithSys struct {
	os.FileInfo
	sys any
}

func (info fileInfoWithSys) Sys() any { return info.sys }
