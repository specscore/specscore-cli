//go:build darwin

package cli

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// publishSpecTreeNoReplace atomically exchanges the live and staged sibling
// trees. The historical live tree lands at stageRoot and is classified by the
// caller before any cleanup; unlike a two-step rename, spec/ is never absent
// for a raw writer to recreate as a successor.
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
	transactionAfterRecoveryClaim()
	if err := unix.RenameatxNp(parentFD, filepath.Base(stageRoot), parentFD, rootName, unix.RENAME_SWAP); err != nil {
		return "", fmt.Errorf("atomically exchanging staged spec tree: %w", err)
	}
	return stageRoot, nil
}
