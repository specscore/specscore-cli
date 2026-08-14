//go:build linux

package cli

import "golang.org/x/sys/unix"

func lifecycleExchangeSpecAt(stagedProjectFD, realProjectFD int) error {
	return unix.Renameat2(stagedProjectFD, "spec", realProjectFD, "spec", unix.RENAME_EXCHANGE)
}
