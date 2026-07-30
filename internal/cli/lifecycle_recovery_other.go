//go:build !darwin && !linux

package cli

import (
	"fmt"
	"os"
)

func readLifecycleRecoveryEntriesNoFollow(string) ([]os.DirEntry, error) {
	return nil, fmt.Errorf("secure lifecycle recovery reads are unavailable on this platform")
}

func readLifecycleRecoveryRegularFileNoFollow(string, string) ([]byte, error) {
	return nil, fmt.Errorf("secure lifecycle recovery reads are unavailable on this platform")
}
