package consilium

import (
	"fmt"
	"strings"
)

// rosterMaxSize is the upper bound on the active roster, per
// REQ:roster-validation.
const rosterMaxSize = 12

// ValidateRoster enforces REQ:roster-validation over an already-resolved
// roster (defaults − exclude ∪ custom): each of the three groups
// (builders/customers/adversaries) MUST have ≥1 member; the total roster MUST
// be ≤12; no custom name may collide with a default slug (case-insensitive);
// and every custom entry (one carrying a Path) MUST resolve to a markdown file
// conforming to the custom-role contract. The first violation is returned as a
// single clear error naming the specific violation; a valid roster returns nil.
func ValidateRoster(roster []RosterEntry) error {
	if err := checkGroupFloors(roster); err != nil {
		return err
	}
	if len(roster) > rosterMaxSize {
		return fmt.Errorf("roster has %d members; ≤%d allowed", len(roster), rosterMaxSize)
	}
	if err := checkNoDefaultCollision(roster); err != nil {
		return err
	}
	return checkCustomFilesConform(roster)
}

// checkGroupFloors verifies each of the three groups has at least one member.
// Groups are checked in declared order so the error is deterministic; the
// message shape is exactly `<group> group has 0 members; ≥1 required`.
func checkGroupFloors(roster []RosterEntry) error {
	counts := make(map[Group]int, len(groupEnum))
	for _, e := range roster {
		counts[e.Group]++
	}
	for _, g := range groupEnum {
		if counts[g] == 0 {
			return fmt.Errorf("%s group has 0 members; ≥1 required", g)
		}
	}
	return nil
}

// checkNoDefaultCollision verifies that no custom entry's name collides
// (case-insensitively) with a default roster slug. A default entry carries an
// empty Path; a custom entry carries a non-empty Path. The error names the
// colliding slug.
func checkNoDefaultCollision(roster []RosterEntry) error {
	defaults := make(map[string]bool, len(defaultRoster))
	for _, d := range defaultRoster {
		defaults[strings.ToLower(d.Name)] = true
	}
	for _, e := range roster {
		if e.Path == "" {
			continue // default entry
		}
		if defaults[strings.ToLower(e.Name)] {
			return fmt.Errorf("custom role %q collides with a default role (case-insensitive)", e.Name)
		}
	}
	return nil
}

// checkCustomFilesConform validates that every custom entry's Path resolves to
// a markdown file conforming to the custom-role contract, delegating to
// ParseCustomRole. The first non-conforming file's error (which names the field
// and the file path) is returned verbatim.
func checkCustomFilesConform(roster []RosterEntry) error {
	for _, e := range roster {
		if e.Path == "" {
			continue // default entry
		}
		if _, err := ParseCustomRole(e.Path); err != nil {
			return err
		}
	}
	return nil
}
