//go:build windows

package cli

import "fmt"

func platformSupportsSecureLifecycleTransaction() bool { return false }

// Windows does not expose an equivalent descriptor-relative no-reparse-point
// traversal through the portable Go API used by this CLI.  Refuse lifecycle
// filesystem transactions rather than traversing a path that a junction or
// reparse-point swap could redirect.  Normal non-mutating lint remains
// available; a future Windows implementation must use NtCreateFile relative
// handles before enabling this path.
func snapshotSpecTreeNoFollow(specRoot string) (specTreeSnapshot, error) {
	return specTreeSnapshot{}, fmt.Errorf("secure lifecycle snapshots are unavailable on Windows for %s; refusing filesystem mutation", specRoot)
}
