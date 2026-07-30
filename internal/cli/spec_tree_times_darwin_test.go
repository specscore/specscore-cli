//go:build darwin

package cli

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
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

func TestSnapshotMetadataRejectsDarwinFlagsAndACLXattrs(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "metadata")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSnapshotPlatformMetadata(int(file.Fd()), info); err != nil {
		t.Fatalf("ordinary file metadata rejected: %v", err)
	}
	flagged := fileInfoWithSys{FileInfo: info, sys: &syscall.Stat_t{Flags: 1}}
	if err := validateSnapshotPlatformMetadata(int(file.Fd()), flagged); err == nil {
		t.Fatal("Darwin filesystem flag accepted")
	}
	if err := validateSnapshotPlatformMetadata(int(file.Fd()), fileInfoWithSys{FileInfo: info}); err == nil {
		t.Fatal("unsupported stat metadata accepted")
	}
	platformStat := *(info.Sys().(*syscall.Stat_t))
	platformStat.Flags = 1
	if _, err := captureSnapshotEntryMetadata(file, fileInfoWithSys{FileInfo: info, sys: &platformStat}); err == nil || !contains(err, "filesystem flags") {
		t.Fatalf("capture accepted filesystem flags: %v", err)
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

type fileInfoWithSys struct {
	os.FileInfo
	sys any
}

func (info fileInfoWithSys) Sys() any { return info.sys }
