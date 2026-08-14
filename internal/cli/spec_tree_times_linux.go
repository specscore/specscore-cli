//go:build linux

package cli

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

var stageLinuxUtimesNanoAt = unix.UtimesNanoAt

// setStagedEntryTimes uses AT_EMPTY_PATH so the kernel targets the
// held descriptor, never a re-resolved staging pathname.  utimensat accepts
// nanosecond times on the Linux filesystems supported by the CLI.
func setStagedEntryTimes(fd int, accessed, modified time.Time) error {
	if accessed.IsZero() {
		accessed = modified
	}
	if modified.IsZero() {
		modified = accessed
	}
	return stageLinuxUtimesNanoAt(fd, "", []unix.Timespec{
		unix.NsecToTimespec(accessed.UnixNano()),
		unix.NsecToTimespec(modified.UnixNano()),
	}, unix.AT_EMPTY_PATH)
}

// setStagedEntryModificationTime remains a focused-test compatibility helper.
func setStagedEntryModificationTime(fd int, modified time.Time) error {
	return setStagedEntryTimes(fd, modified, modified)
}

func snapshotEntryTimes(info os.FileInfo) (time.Time, time.Time, error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("unsupported stat metadata type %T", info.Sys())
	}
	return time.Unix(stat.Atim.Sec, stat.Atim.Nsec), time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec), nil
}
