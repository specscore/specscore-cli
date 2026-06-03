package consilium

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes body to <dir>/specscore.yaml and returns the dir, which is
// the projectRoot the resolver reads from.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	return dir
}

// findEntry returns the entry with the given name, or false when absent.
func findEntry(roster []RosterEntry, name string) (RosterEntry, bool) {
	for _, e := range roster {
		if e.Name == name {
			return e, true
		}
	}
	return RosterEntry{}, false
}

// AC:roster-resolves-defaults-minus-exclude-plus-custom
func TestResolveRoster_DefaultsMinusExcludePlusCustom(t *testing.T) {
	root := writeConfig(t, `consilium:
  roster:
    exclude: [marketing]
    custom:
      - name: accessibility
        group: customers
        path: .specscore/roles/accessibility.md
`)

	roster, err := ResolveRoster(root)
	if err != nil {
		t.Fatalf("ResolveRoster: %v", err)
	}

	if len(roster) != 9 {
		t.Fatalf("expected 9 roles (9 defaults - 1 excluded + 1 custom), got %d: %+v", len(roster), roster)
	}

	if _, ok := findEntry(roster, "marketing"); ok {
		t.Errorf("marketing MUST be absent from resolved roster")
	}

	acc, ok := findEntry(roster, "accessibility")
	if !ok {
		t.Fatalf("accessibility MUST be present in resolved roster")
	}
	if acc.Group != GroupCustomers {
		t.Errorf("accessibility group = %q, want %q", acc.Group, GroupCustomers)
	}
	if acc.Path != ".specscore/roles/accessibility.md" {
		t.Errorf("accessibility path = %q, want .specscore/roles/accessibility.md", acc.Path)
	}
}

// No consilium.roster block → the full 9-role default set, in declared order,
// each with an empty path (defaults carry no custom-role file).
func TestResolveRoster_DefaultsWhenNoRosterBlock(t *testing.T) {
	root := writeConfig(t, "")

	roster, err := ResolveRoster(root)
	if err != nil {
		t.Fatalf("ResolveRoster: %v", err)
	}
	if len(roster) != 9 {
		t.Fatalf("expected 9 default roles, got %d", len(roster))
	}
	for _, e := range roster {
		if e.Path != "" {
			t.Errorf("default role %q should carry no path, got %q", e.Name, e.Path)
		}
	}
}

// An out-of-enum custom group is a config-load error naming the key path.
func TestResolveRoster_CustomGroupOutOfEnum(t *testing.T) {
	root := writeConfig(t, `consilium:
  roster:
    custom:
      - name: accessibility
        group: stakeholders
        path: .specscore/roles/accessibility.md
`)

	if _, err := ResolveRoster(root); err == nil {
		t.Fatalf("expected error for out-of-enum custom group, got nil")
	}
}
