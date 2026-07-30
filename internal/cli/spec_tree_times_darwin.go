//go:build darwin

package cli

import (
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

var stageDarwinFsetattrlist = fsetStagedEntryModificationTime

// setStagedEntryModificationTime calls Darwin fsetattrlist directly because
// the exposed futimes wrapper is limited to microsecond precision.  The
// fsetattrlist syscall accepts ATTR_CMN_MODTIME as a nanosecond Timespec and,
// unlike setattrlist(path), operates on the already-held descriptor.
func setStagedEntryModificationTime(fd int, modified time.Time) error {
	return stageDarwinFsetattrlist(fd, modified)
}

func fsetStagedEntryModificationTime(fd int, modified time.Time) error {
	attrs := unix.Attrlist{Bitmapcount: 5, Commonattr: unix.ATTR_CMN_MODTIME}
	timestamp := unix.NsecToTimespec(modified.UnixNano())
	//nolint:staticcheck // Darwin exposes no descriptor-based libSystem wrapper; setattrlist(path) would reintroduce a pathname race.
	_, _, errno := unix.Syscall6(
		unix.SYS_FSETATTRLIST,
		uintptr(fd),
		uintptr(unsafe.Pointer(&attrs)),
		uintptr(unsafe.Pointer(&timestamp)),
		unsafe.Sizeof(timestamp),
		0,
		0,
	)
	if errno != 0 {
		return fmt.Errorf("fsetattrlist: %w", errno)
	}
	return nil
}
