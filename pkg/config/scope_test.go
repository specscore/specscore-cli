package config

import (
	"path/filepath"
	"testing"
)

func TestCheckCommittedScope_FlagsUserScopedKeyInProject(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "recaps:\n  repo: ~/work-log\n")

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	if len(vs) != 1 || vs[0].Key != "recaps.repo" {
		t.Fatalf("violations = %+v, want one for recaps.repo", vs)
	}
}

func TestCheckCommittedScope_AllowsNonScopedKeys(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "recaps:\n  enabled: true\n")

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none (recaps.enabled is allowed)", vs)
	}
}

func TestCheckCommittedScope_ScalarParentIsNotAKey(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "recaps: off\n")

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none (recaps is a scalar, no nested repo)", vs)
	}
}

func TestCheckCommittedScope_FlagsMultipleKeys(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"),
		"journal:\n  repo: ~/log\n  stream: mine\n")

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	keys := map[string]bool{}
	for _, v := range vs {
		keys[v.Key] = true
	}
	if !keys["journal.repo"] || !keys["journal.stream"] {
		t.Fatalf("violations = %+v, want journal.repo and journal.stream", vs)
	}
}

func TestCheckCommittedScope_OnlyChecksCommittedFile(t *testing.T) {
	repo := t.TempDir()
	// User-scoped key in the LOCAL (uncommitted) layer is allowed.
	writeLayer(t, filepath.Join(repo, "specscore.local.yaml"), "recaps:\n  repo: ~/work-log\n")
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "recaps:\n  enabled: true\n")

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none (recaps.repo in local is allowed)", vs)
	}
}

func TestCheckCommittedScope_NoProjectFileNoViolation(t *testing.T) {
	repo := t.TempDir()

	vs, err := CheckCommittedScope(repo)
	if err != nil {
		t.Fatalf("CheckCommittedScope: %v", err)
	}
	if len(vs) != 0 {
		t.Fatalf("violations = %+v, want none when no specscore.yaml", vs)
	}
}

func TestCheckCommittedScope_ParseErrorSurfaces(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "recaps: [unclosed\n")

	_, err := CheckCommittedScope(repo)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
