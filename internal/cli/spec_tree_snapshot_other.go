//go:build !darwin && !linux && !windows

package cli

import "fmt"

func platformSupportsSecureLifecycleTransaction() bool { return false }

func snapshotSpecTreeNoFollow(specRoot string) (specTreeSnapshot, error) {
	return specTreeSnapshot{}, fmt.Errorf("secure lifecycle snapshots are unavailable on this platform for %s; refusing filesystem mutation", specRoot)
}
