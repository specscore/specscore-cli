//go:build linux || darwin

package lifecycle

import (
	"os"

	"golang.org/x/sys/unix"
)

func artifactIdentityOpenFlags() int {
	return os.O_RDONLY | unix.O_NONBLOCK | unix.O_NOFOLLOW
}
