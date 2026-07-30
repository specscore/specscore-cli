//go:build darwin

package cli

import "golang.org/x/sys/unix"

func lifecycleExchangeSpecAt(stagedProjectFD, realProjectFD int) error {
	return unix.RenameatxNp(stagedProjectFD, "spec", realProjectFD, "spec", unix.RENAME_SWAP)
}
