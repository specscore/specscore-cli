package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncIndex_DerivesAndRepairsPlanRows(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "# Plans\n\n" + plansIndexHeader + "\n|---|---|---|---|---|\n\n## Recently Closed\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := Scaffold(ScaffoldOptions{Slug: "alpha", SourceFeature: "source", Date: "2026-07-27", Owner: "alex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "alpha.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	changed, err := SyncIndex(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first sync must add the Plan row")
	}
	got, err := os.ReadFile(filepath.Join(plansDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantRow := "| [alpha](alpha.md) | Draft | source | 2026-07-27 | alex |"
	if !strings.Contains(string(got), wantRow) {
		t.Errorf("index missing derived row %q:\n%s", wantRow, got)
	}
	if !strings.Contains(string(got), "## Recently Closed") {
		t.Errorf("sync must preserve non-table content:\n%s", got)
	}

	changed, err = SyncIndex(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("second sync must be idempotent")
	}
}

func TestIndexContent_PreservesLegacyIndexesAndValidatesCanonicalTable(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}

	legacy := []byte("# Plans\n\n- historical hand-maintained index\n")
	updated, changed, err := IndexContent(plansDir, legacy)
	if err != nil {
		t.Fatal(err)
	}
	if changed || string(updated) != string(legacy) {
		t.Fatalf("legacy index changed=%v content=%q, want byte-for-byte preservation", changed, updated)
	}

	malformed := []byte("# Plans\n\n" + plansIndexHeader + "\n| not a separator |\n")
	if _, _, err := IndexContent(plansDir, malformed); err == nil {
		t.Fatal("canonical header without its separator must be rejected")
	}
}

func TestIndexContent_RendersMetadataFallbacks(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for slug, body := range map[string]string{
		"minimal": "# Plan: Minimal\n",
		"idea":    "# Plan: Idea\n\n**Source:** idea:seed\n",
	} {
		if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("# Plans\n\n" + plansIndexHeader + "\n|---|---|---|---|---|\n")
	updated, changed, err := IndexContent(plansDir, content)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("derived rows must be added")
	}
	got := string(updated)
	if !strings.Contains(got, "| [minimal](minimal.md) | — | — | — | — |") {
		t.Errorf("minimal plan must render metadata fallbacks:\n%s", got)
	}
	if !strings.Contains(got, "| [idea](idea.md) | — | idea:seed | — | — |") {
		t.Errorf("source-form plan must preserve its source value:\n%s", got)
	}
}

func TestSyncIndex_MissingIndexReturnsReadError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if changed, err := SyncIndex(plansDir); err == nil || changed {
		t.Fatalf("missing index changed=%v err=%v, want read error", changed, err)
	}
}

func TestIndexContent_MissingPlansDirReturnsDiscoverError(t *testing.T) {
	plansPath := filepath.Join(t.TempDir(), "plans.md")
	if err := os.WriteFile(plansPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := IndexContent(plansPath, []byte("# Plans\n")); err == nil {
		t.Fatal("non-directory plans path must return its discovery error")
	}
}

func TestSyncIndex_ReportsWriteError(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "# Plans\n\n" + plansIndexHeader + "\n|---|---|---|---|---|\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := Scaffold(ScaffoldOptions{Slug: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "alpha.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	originalWrite := syncIndexWriteFile
	syncIndexWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { syncIndexWriteFile = originalWrite })
	if changed, err := SyncIndex(plansDir); err == nil || changed {
		t.Fatalf("failed index write changed=%v err=%v, want write error", changed, err)
	}
}

func TestIsPlansIndexSeparator(t *testing.T) {
	for _, tc := range []struct {
		line string
		want bool
	}{
		{"|---|---|---|---|---|", true},
		{"|---|---|", false},
		{"|---| text |---|---|---|", false},
	} {
		if got := isPlansIndexSeparator(tc.line); got != tc.want {
			t.Errorf("isPlansIndexSeparator(%q) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
