package lint

import (
	"fmt"
	"sort"
	"strings"
)

// CheckRegistryParity verifies that the rule registry and the set of rule IDs a
// checker can emit are in full parity. It compares the registry ID set (keys of
// the canonical registry, as returned by AllRuleNames) against the emittable ID
// set (the rule names a freshly built default linter registers checkers under).
//
// A default linter (newLinter(Options{})) registers no custom checkers, so the
// emittable set is the built-in surface only.
//
// It returns a non-nil error naming the offending IDs when either an orphan
// registry entry (an ID no checker can emit) or an unregistered emission (an ID
// a checker can emit that is absent from the registry) exists. On full parity it
// returns nil.
func CheckRegistryParity() error {
	registryIDs := AllRuleNames()

	emittableIDs := make(map[string]bool)
	for id := range newLinter(Options{}).ruleSet {
		emittableIDs[id] = true
	}

	return checkParity(registryIDs, emittableIDs)
}

// checkParity compares a registry ID set against an emittable ID set and returns
// a non-nil error naming the offending IDs (sorted, deduplicated) when the two
// sets diverge. Orphan entries (in registry, not emittable) and unregistered
// emissions (emittable, not in registry) are reported as distinct categories.
func checkParity(registryIDs, emittableIDs map[string]bool) error {
	var orphans []string
	for id := range registryIDs {
		if !emittableIDs[id] {
			orphans = append(orphans, id)
		}
	}

	var unregistered []string
	for id := range emittableIDs {
		if !registryIDs[id] {
			unregistered = append(unregistered, id)
		}
	}

	if len(orphans) == 0 && len(unregistered) == 0 {
		return nil
	}

	sort.Strings(orphans)
	sort.Strings(unregistered)

	var parts []string
	if len(orphans) > 0 {
		parts = append(parts, fmt.Sprintf("orphan registry entries (no emitting checker): %s", strings.Join(orphans, ", ")))
	}
	if len(unregistered) > 0 {
		parts = append(parts, fmt.Sprintf("unregistered emitted rule IDs (no registry entry): %s", strings.Join(unregistered, ", ")))
	}

	return fmt.Errorf("registry/checker parity check failed: %s", strings.Join(parts, "; "))
}
