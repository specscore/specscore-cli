//go:build linux

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func publishSpecTreeNoReplace(specRoot, stageRoot string) (string, error) {
	parent := filepath.Dir(specRoot)
	rootName := filepath.Base(specRoot)
	if filepath.Dir(stageRoot) != parent {
		return "", fmt.Errorf("staged spec tree must be a sibling of %s", specRoot)
	}
	parentFD, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", fmt.Errorf("opening spec parent without following links: %w", err)
	}
	defer func() { _ = unix.Close(parentFD) }()
	recoveryPath, err := os.MkdirTemp(parent, ".specscore-lifecycle-recovery-")
	if err != nil {
		return "", fmt.Errorf("reserving lifecycle recovery path: %w", err)
	}
	recoveryName := filepath.Base(recoveryPath)
	if err := unix.Unlinkat(parentFD, recoveryName, unix.AT_REMOVEDIR); err != nil {
		return "", fmt.Errorf("opening lifecycle recovery destination: %w", err)
	}
	if err := unix.Renameat2(parentFD, rootName, parentFD, recoveryName, unix.RENAME_NOREPLACE); err != nil {
		return "", fmt.Errorf("claiming current spec tree for safe publication: %w", err)
	}
	transactionAfterRecoveryClaim()
	if err := unix.Renameat2(parentFD, filepath.Base(stageRoot), parentFD, rootName, unix.RENAME_NOREPLACE); err != nil {
		return recoveryPath, fmt.Errorf("publishing staged spec tree without replacing a successor: %w; recover the preserved tree at %s", err, recoveryPath)
	}
	return recoveryPath, nil
}
