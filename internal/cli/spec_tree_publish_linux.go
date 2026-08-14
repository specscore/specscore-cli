//go:build linux

package cli

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

var (
	publishLinuxOpenParent = unix.Open
	publishLinuxCloseFD    = unix.Close
	publishLinuxExchange   = unix.Renameat2
)

func publishSpecTreeNoReplace(specRoot, stageRoot string) (string, error) {
	parent := filepath.Dir(specRoot)
	rootName := filepath.Base(specRoot)
	if filepath.Dir(stageRoot) != parent {
		return "", fmt.Errorf("staged spec tree must be a sibling of %s", specRoot)
	}
	parentFD, err := publishLinuxOpenParent(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("opening spec parent without following links: %w", err)
	}
	defer func() { _ = publishLinuxCloseFD(parentFD) }()
	transactionAfterRecoveryClaim()
	if err := publishLinuxExchange(parentFD, filepath.Base(stageRoot), parentFD, rootName, unix.RENAME_EXCHANGE); err != nil {
		return "", fmt.Errorf("atomically exchanging staged spec tree: %w", err)
	}
	return stageRoot, nil
}
