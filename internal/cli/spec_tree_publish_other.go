//go:build !darwin && !linux && !windows

package cli

import "fmt"

func publishSpecTreeNoReplace(specRoot, stageRoot string) (string, error) {
	return "", fmt.Errorf("secure atomic lifecycle publication is unavailable on this platform for %s; refusing filesystem mutation", specRoot)
}
