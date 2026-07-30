//go:build linux

package cli

import (
	"time"

	"golang.org/x/sys/unix"
)

var stageLinuxUtimesNanoAt = unix.UtimesNanoAt

// setStagedEntryModificationTime uses AT_EMPTY_PATH so the kernel targets the
// held descriptor, never a re-resolved staging pathname.  utimensat accepts
// nanosecond times on the Linux filesystems supported by the CLI.
func setStagedEntryModificationTime(fd int, modified time.Time) error {
	timestamp := unix.NsecToTimespec(modified.UnixNano())
	return stageLinuxUtimesNanoAt(fd, "", []unix.Timespec{timestamp, timestamp}, unix.AT_EMPTY_PATH)
}
