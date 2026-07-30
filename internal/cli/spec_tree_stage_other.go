//go:build !darwin && !linux

package cli

import "fmt"

func openStagedSpecTreeNoFollow(string) (*stagedSpecTree, error) {
	return nil, fmt.Errorf("secure staged trees are unavailable on this platform")
}

func closeStagedSpecTree(stage *stagedSpecTree) error {
	if stage == nil || stage.root == nil {
		return nil
	}
	err := stage.root.Close()
	stage.root = nil
	return err
}

func runLintInStagedSpecTree(stage *stagedSpecTree, run func(string) error) error {
	return run(stage.path)
}

func materializeStagedSpecTreeNoFollow(*stagedSpecTree, specTreeSnapshot) error {
	return fmt.Errorf("secure staged trees are unavailable on this platform")
}

func snapshotStagedSpecTreeNoFollow(*stagedSpecTree) (specTreeSnapshot, error) {
	return specTreeSnapshot{}, fmt.Errorf("secure staged trees are unavailable on this platform")
}

func stagedSpecTreeMatchesPath(*stagedSpecTree) (bool, error) {
	return false, fmt.Errorf("secure staged trees are unavailable on this platform")
}

func stagedSpecTreePublishedAt(*stagedSpecTree, string) (bool, error) {
	return false, fmt.Errorf("secure staged trees are unavailable on this platform")
}
