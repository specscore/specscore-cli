package lint

import (
	"os"
	"path/filepath"
	"testing"
)

func scopeSpecRoot(t *testing.T, root, configBody string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return specRoot
}

func TestConfigScope_Identity(t *testing.T) {
	c := newConfigScopeChecker()
	if c.name() != "config-user-scoped-key" {
		t.Errorf("name = %q", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity = %q", c.severity())
	}
}

func TestConfigScope_FlagsUserScopedKeyInCommittedFile(t *testing.T) {
	root := t.TempDir()
	specRoot := scopeSpecRoot(t, root, "recaps:\n  repo: ~/work-log\n")

	vs, err := newConfigScopeChecker().check(specRoot)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(vs) != 1 {
		t.Fatalf("violations = %+v, want exactly one", vs)
	}
	if vs[0].Rule != "config-user-scoped-key" {
		t.Errorf("Rule = %q, want config-user-scoped-key", vs[0].Rule)
	}
	if vs[0].File != "specscore.yaml" {
		t.Errorf("File = %q, want specscore.yaml", vs[0].File)
	}
	if vs[0].Severity != "error" {
		t.Errorf("Severity = %q, want error", vs[0].Severity)
	}
}

func TestConfigScope_CleanConfigNoViolation(t *testing.T) {
	root := t.TempDir()
	specRoot := scopeSpecRoot(t, root, "recaps:\n  enabled: true\n")

	vs, err := newConfigScopeChecker().check(specRoot)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none", vs)
	}
}

func TestConfigScope_MalformedConfigStaysSilent(t *testing.T) {
	root := t.TempDir()
	specRoot := scopeSpecRoot(t, root, "recaps: [unclosed\n")

	vs, err := newConfigScopeChecker().check(specRoot)
	if err != nil {
		t.Fatalf("check should stay silent on bad yaml, got err: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none (other rules surface bad yaml)", vs)
	}
}
