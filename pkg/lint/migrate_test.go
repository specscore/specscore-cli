package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/plan"
)

func featureDoc(status, footerURL string) string {
	return "# Feature: Foo\n\n**Status:** " + status + "\n\n## Summary\n\nx\n\n---\n*This document follows the " + footerURL + "*\n"
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// AC: backfills-format-and-status — status-bearing artifacts gain format: +
// status: (mirrored from the body).
func TestMigrate_BackfillsFeatureAndIdea(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": featureDoc("Approved", "https://specscore.md/feature-specification"),
		"ideas/bar.md":           "# Idea: Bar\n\n**Status:** Draft\n\n---\n*This document follows the https://specscore.md/idea-specification*\n",
	})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Errorf("expected 2 changed files, got %v", changed)
	}
	feat := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.HasPrefix(feat, "---\nformat: https://specscore.md/feature-specification\nstatus: Approved\n---\n\n# Feature: Foo") {
		t.Errorf("feature not migrated:\n%s", feat)
	}
	idea := readFile(t, filepath.Join(specRoot, "ideas", "bar.md"))
	if !strings.HasPrefix(idea, "---\nformat: https://specscore.md/idea-specification\nstatus: Draft\n---") {
		t.Errorf("idea not migrated:\n%s", idea)
	}
}

// AC: status-less-types-excluded — an index README gets format: but no status:.
func TestMigrate_StatusLessIndexGetsFormatOnly(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md": "# Features\n\n## Index\n\n| Feature | Status |\n|---|---|\n\n## Open Questions\n\nNone.\n\n---\n*This document follows the https://specscore.md/features-index-specification*\n",
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	idx := readFile(t, filepath.Join(specRoot, "features", "README.md"))
	if !strings.HasPrefix(idx, "---\nformat: https://specscore.md/features-index-specification\n---") {
		t.Errorf("index not migrated to format-only:\n%s", idx)
	}
	if strings.Contains(idx, "status:") {
		t.Errorf("status-less index must not gain status:\n%s", idx)
	}
}

// AC: migration-idempotent — a second run changes nothing.
func TestMigrate_Idempotent(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("second run should change nothing, got %v", changed)
	}
	if readFile(t, filepath.Join(specRoot, "features", "foo", "README.md")) != after {
		t.Error("idempotent re-run altered the file")
	}
}

// AC: footer-aligned-to-format — a wrong footer URL is realigned to format:.
func TestMigrate_AlignsFooter(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		// footer carries the WRONG (plan) URL.
		"features/foo/README.md": featureDoc("Draft", "https://specscore.md/plan-specification"),
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.Contains(got, "*This document follows the https://specscore.md/feature-specification*") {
		t.Errorf("footer not aligned to format:\n%s", got)
	}
	if strings.Contains(got, "plan-specification") {
		t.Errorf("stale footer URL remains:\n%s", got)
	}
}

// A status-bearing artifact with no body **Status:** gets format: only.
func TestMigrate_StatusBearingNoBodyStatus(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": "# Feature: Foo\n\nNo status line.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n",
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.HasPrefix(got, "---\nformat: https://specscore.md/feature-specification\n---") {
		t.Errorf("expected format-only frontmatter:\n%s", got)
	}
	if strings.Contains(got, "status:") {
		t.Errorf("no status should be added without a body status:\n%s", got)
	}
}

func TestMigrate_FlatNonPlanIsIgnored(t *testing.T) {
	original := "# Random notes\n\n**Status:** Draft\n"
	specRoot := writeSpec(t, map[string]string{"plans/notes.md": original})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("non-Plan flat markdown must not gain Plan frontmatter: %v", changed)
	}
	if got := readFile(t, filepath.Join(specRoot, "plans", "notes.md")); got != original {
		t.Fatalf("non-Plan flat markdown was mutated:\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestMigrate_PlanIgnoresFauxBodyStatus(t *testing.T) {
	original := "---\nformat: https://specscore.md/plan-specification\n---\n\n" +
		"**Status:** Pre-title example\n\n" +
		"```markdown\n**Status:** Fenced example\n```\n\n" +
		"# Plan: Genuine\n\n## Summary\n\nNo actual status.\n"
	specRoot := writeSpec(t, map[string]string{"plans/genuine.md": original})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("faux Plan body statuses must not create a mirror: %v", changed)
	}
	if got := readFile(t, filepath.Join(specRoot, "plans", "genuine.md")); got != original {
		t.Fatalf("migration rewrote a Plan from faux metadata:\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestMigrate_PlanStatusParserErrorIsReturned(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"plans/genuine.md": "# Plan: Genuine\n\n**Status:** Draft\n",
	})
	original := parsePlanForArtifactStatus
	parsePlanForArtifactStatus = func(string) (*plan.Plan, error) {
		return nil, errors.New("forced Plan parser failure")
	}
	t.Cleanup(func() { parsePlanForArtifactStatus = original })
	if _, err := Migrate(specRoot); err == nil {
		t.Fatal("expected Plan parser error from migration")
	}
}

func TestMigrate_WalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()
	if _, err := Migrate(specRoot); err == nil {
		t.Error("expected walk error")
	}
}

func TestMigrate_WriteError(t *testing.T) {
	// Two read-only features needing migration under one walker: the first
	// write fails (setting writeErr); the second exercises the early return.
	specRoot := writeSpec(t, map[string]string{
		"features/a/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
		"features/b/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
	})
	for _, s := range []string{"a", "b"} {
		p := filepath.Join(specRoot, "features", s, "README.md")
		if err := os.Chmod(p, 0o444); err != nil {
			t.Skip("cannot change permissions")
		}
		defer func() { _ = os.Chmod(p, 0o644) }()
	}
	if _, err := Migrate(specRoot); err == nil {
		t.Error("expected write error on read-only artifacts needing migration")
	}
}

func TestEnsureFrontmatter(t *testing.T) {
	fields := [][2]string{{"format", "u"}, {"status", "Draft"}}
	t.Run("no block prepends", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("# Title\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n---\n\n# Title\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("existing block upserts", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: old\n---\n\n# T\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n---\n\n# T\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("dotted closer is preserved", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: old\n...\n\n# T\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n...\n\n# T\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("opening fence without closing prepends", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: x\nno close\n"), fields))
		if !strings.HasPrefix(got, "---\nformat: u\nstatus: Draft\n---\n\n---\n") {
			t.Errorf("got %q", got)
		}
	})
}

func TestHasClosingFence(t *testing.T) {
	if !hasClosingFence([]string{"---", "k: v", "---"}) {
		t.Error("expected true with closing fence")
	}
	if !hasClosingFence([]string{"---", "k: v", "..."}) {
		t.Error("expected dotted closer to close frontmatter")
	}
	if hasClosingFence([]string{"---", "k: v"}) {
		t.Error("expected false without closing fence")
	}
}
