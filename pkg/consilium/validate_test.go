package consilium

import (
	"strings"
	"testing"
)

// defaultEntries returns the 9-role default roster as []RosterEntry, the
// fixture the group-floor / size / collision checks operate over.
func defaultEntries() []RosterEntry {
	out := make([]RosterEntry, len(defaultRoster))
	copy(out, defaultRoster)
	return out
}

func TestValidateRoster_DefaultsPass(t *testing.T) {
	if err := ValidateRoster(defaultEntries()); err != nil {
		t.Fatalf("default roster should validate, got: %v", err)
	}
}

// AC:roster-group-floor-violation-rejected — excluding all three adversaries
// with no custom adversary leaves the group empty; validation must fail with
// the exact message and no verdict.
func TestValidateRoster_AC_GroupFloorViolation(t *testing.T) {
	var roster []RosterEntry
	for _, e := range defaultEntries() {
		if e.Group == GroupAdversaries {
			continue
		}
		roster = append(roster, e)
	}

	err := ValidateRoster(roster)
	if err == nil {
		t.Fatal("expected error for empty adversaries group, got nil")
	}
	want := "adversaries group has 0 members; ≥1 required"
	if err.Error() != want {
		t.Fatalf("error = %q, want exactly %q", err.Error(), want)
	}
}

func TestValidateRoster_EmptyBuildersFloor(t *testing.T) {
	var roster []RosterEntry
	for _, e := range defaultEntries() {
		if e.Group == GroupBuilders {
			continue
		}
		roster = append(roster, e)
	}
	err := ValidateRoster(roster)
	if err == nil || err.Error() != "builders group has 0 members; ≥1 required" {
		t.Fatalf("expected builders floor error, got: %v", err)
	}
}

func TestValidateRoster_OverTwelveRejected(t *testing.T) {
	roster := defaultEntries()
	// Add 4 custom builders → 13 total, exceeding the ≤12 cap.
	for i := 0; i < 4; i++ {
		roster = append(roster, RosterEntry{Name: "extra-" + string(rune('a'+i)), Group: GroupBuilders})
	}
	err := ValidateRoster(roster)
	if err == nil {
		t.Fatal("expected error for >12 roster, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "12") || !strings.Contains(msg, "13") {
		t.Errorf("error message should name the cap and the count; got: %s", msg)
	}
}

func TestValidateRoster_TwelveOK(t *testing.T) {
	roster := defaultEntries()
	for i := 0; i < 3; i++ {
		roster = append(roster, RosterEntry{Name: "extra-" + string(rune('a'+i)), Group: GroupBuilders})
	}
	// 12 total, all groups populated, no collisions, no custom paths.
	if err := ValidateRoster(roster); err != nil {
		t.Fatalf("12-role roster should validate, got: %v", err)
	}
}

func TestValidateRoster_CustomCollidesWithDefaultCaseInsensitive(t *testing.T) {
	roster := defaultEntries()
	roster = append(roster, RosterEntry{Name: "Engineer", Group: GroupBuilders, Path: "x.md"})
	err := ValidateRoster(roster)
	if err == nil {
		t.Fatal("expected collision error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "engineer") || !strings.Contains(strings.ToLower(msg), "collide") {
		t.Errorf("error message should name the colliding slug; got: %s", msg)
	}
}

func TestValidateRoster_CustomPathConforms(t *testing.T) {
	dir := t.TempDir()
	path := writeRole(t, dir, "accessibility", validRoleBody)

	roster := defaultEntries()
	roster = append(roster, RosterEntry{Name: "accessibility", Group: GroupCustomers, Path: path})

	if err := ValidateRoster(roster); err != nil {
		t.Fatalf("roster with a conforming custom file should validate, got: %v", err)
	}
}

func TestValidateRoster_CustomPathNonConforming(t *testing.T) {
	dir := t.TempDir()
	// Missing Group field → ParseCustomRole rejects; ValidateRoster must surface it.
	body := strings.Replace(validRoleBody, "**Group:** customers\n", "", 1)
	path := writeRole(t, dir, "accessibility", body)

	roster := defaultEntries()
	roster = append(roster, RosterEntry{Name: "accessibility", Group: GroupCustomers, Path: path})

	err := ValidateRoster(roster)
	if err == nil {
		t.Fatal("expected error for non-conforming custom file, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error message missing file path; got: %s", err.Error())
	}
}
