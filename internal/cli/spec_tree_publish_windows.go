//go:build windows

package cli

import "fmt"

func publishSpecTreeNoReplace(specRoot, stageRoot string) (string, error) {
	return "", fmt.Errorf("secure atomic lifecycle publication is unavailable on Windows for %s; refusing filesystem mutation", specRoot)
}
