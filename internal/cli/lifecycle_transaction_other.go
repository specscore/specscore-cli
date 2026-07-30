//go:build !darwin && !linux

package cli

import "fmt"

func openLifecycleProjectNoFollow(string) (*stagedSpecTree, error) {
	return nil, fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func createLifecycleStageProjectNoFollow(*stagedSpecTree, string) (*stagedSpecTree, error) {
	return nil, fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func openLifecycleProjectChildNoFollow(*stagedSpecTree, string) (*stagedSpecTree, error) {
	return nil, fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func createLifecycleStageSpecNoFollow(*stagedSpecTree, specTreeSnapshot) (*stagedSpecTree, error) {
	return nil, fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func lifecycleProjectChildMatches(*stagedSpecTree, string, *stagedSpecTree) error {
	return fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func exchangeLifecycleProjectSpecs(*stagedSpecTree, *stagedSpecTree) error {
	return fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func runLifecycleInStagedProject(*stagedSpecTree, func(string) error) error {
	return fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func materializeLifecycleProjectContext(*stagedSpecTree, *stagedSpecTree) error {
	return fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
func writeLifecycleReceiptNoFollow(*stagedSpecTree, string, []byte) error {
	return fmt.Errorf("secure lifecycle transactions are unavailable on this platform")
}
