//go:build darwin

package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

var stageDarwinFsetattrlist = fsetStagedEntryModificationTime

// setStagedEntryTimes calls Darwin fsetattrlist directly because the exposed
// futimes wrapper is limited to microsecond precision. fsetattrlist accepts
// ATTR_CMN_MODTIME and ATTR_CMN_ACCTIME as nanosecond Timespec values and,
// unlike setattrlist(path), operates on the already-held descriptor.
func setStagedEntryTimes(fd int, accessed, modified time.Time) error {
	if accessed.IsZero() {
		accessed = modified
	}
	if modified.IsZero() {
		modified = accessed
	}
	if accessed.Equal(modified) {
		return stageDarwinFsetattrlist(fd, modified)
	}
	return fsetStagedEntryTimes(fd, accessed, modified)
}

// setStagedEntryModificationTime remains a focused-test compatibility helper.
func setStagedEntryModificationTime(fd int, modified time.Time) error {
	return stageDarwinFsetattrlist(fd, modified)
}

func snapshotEntryTimes(info os.FileInfo) (time.Time, time.Time, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	return time.Unix(stat.Atimespec.Sec, stat.Atimespec.Nsec), time.Unix(stat.Mtimespec.Sec, stat.Mtimespec.Nsec), nil
}

func fsetStagedEntryModificationTime(fd int, modified time.Time) error {
	return fsetStagedEntryTimes(fd, modified, modified)
}

func fsetStagedEntryTimes(fd int, accessed, modified time.Time) error {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_MODTIME | unix.ATTR_CMN_ACCTIME}
	// Darwin orders common attributes by their bitmap value, so MODTIME
	// precedes ACCTIME in the data buffer.
	timestamps := struct {
		Modified unix.Timespec
		Accessed unix.Timespec
	}{
		Modified: unix.NsecToTimespec(modified.UnixNano()),
		Accessed: unix.NsecToTimespec(accessed.UnixNano()),
	}
	//nolint:staticcheck // Darwin exposes no descriptor-based libSystem wrapper; setattrlist(path) would reintroduce a pathname race.
	_, _, errno := unix.Syscall6(
		unix.SYS_FSETATTRLIST,
		uintptr(fd),
		uintptr(unsafe.Pointer(&attrs)),
		uintptr(unsafe.Pointer(&timestamps)),
		unsafe.Sizeof(timestamps),
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("fsetattrlist: %w", errno)
	}
	return nil
}
