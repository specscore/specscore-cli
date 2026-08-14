//go:build !darwin && !linux

package cli

import (
	"fmt"
	"os"
)

func openLifecycleRecoveryHandleNoFollow(string) (*lifecycleRecoveryHandle, error) {
	return nil, fmt.Errorf("secure lifecycle recovery reads are unavailable on this platform")
}

func closeLifecycleRecoveryHandle(*lifecycleRecoveryHandle) error { return nil }

func readLifecycleRecoveryEntriesNoFollow(*lifecycleRecoveryHandle) ([]os.DirEntry, error) {
	return nil, fmt.Errorf("secure lifecycle recovery reads are unavailable on this platform")
}

func readLifecycleRecoveryRegularFileNoFollow(*lifecycleRecoveryHandle, string) ([]byte, error) {
	return nil, fmt.Errorf("secure lifecycle recovery reads are unavailable on this platform")
}
