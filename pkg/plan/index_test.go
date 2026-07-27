package plan

import (
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
