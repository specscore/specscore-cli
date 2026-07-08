package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFix_LegacyModelspecForms rewrites legacy references across model:,
// metadata, and inputs while leaving valid forms and the body untouched, and is
// idempotent.
func TestFix_LegacyModelspecForms(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[example.com/acme/repo]"),
		"spec/graph/modules/m/models/m.hcl": "entity \"A\" {}\n",
		// Entity with a legacy model ref plus a trailing comment on the body.
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://m.A") +
			"\nBody stays: modelspec://m.A is prose and must NOT change.\n",
		// Relationship metadata: one legacy, one already-valid cross-repo ref.
		"spec/graph/modules/m/entities/a.md":      fmArt("entity", "a"),
		"spec/graph/modules/m/entities/b.md":      fmArt("entity", "b"),
		"spec/graph/modules/m/relationships/r.md": fmArt("relationship", "r", "from: m.a", "to: m.b", "metadata:", "  role: modelspec://m.A", "  other: modelspec://example.com/acme/repo/m.A"),
		// Command inputs model ref (legacy).
		"spec/graph/modules/m/commands/c.md": fmArt("command", "c", "inputs:", "  - name: p", "    model: modelspec://m.A"),
	})

	res := lintRepo(t, root, func(o *LintOptions) { o.Fix = true })

	wantFixed := []string{
		"spec/graph/modules/m/commands/c.md",
		"spec/graph/modules/m/entities/e.md",
		"spec/graph/modules/m/relationships/r.md",
	}
	if strings.Join(res.Fixed, ",") != strings.Join(wantFixed, ",") {
		t.Fatalf("fixed = %v, want %v", res.Fixed, wantFixed)
	}
	if hasRule(res.Violations, "graph-model-legacy-form") {
		t.Fatalf("legacy form should be gone after fix: %+v", res.Violations)
	}

	// Frontmatter rewritten; body prose untouched.
	e, _ := os.ReadFile(filepath.Join(root, "spec/graph/modules/m/entities/e.md"))
	if !strings.Contains(string(e), "model: modelspec:///m.A") {
		t.Fatalf("model ref not rewritten:\n%s", e)
	}
	if !strings.Contains(string(e), "Body stays: modelspec://m.A is prose") {
		t.Fatalf("body prose must not be rewritten:\n%s", e)
	}
	// Valid cross-repo metadata ref left as-is.
	r, _ := os.ReadFile(filepath.Join(root, "spec/graph/modules/m/relationships/r.md"))
	if !strings.Contains(string(r), "modelspec://example.com/acme/repo/m.A") ||
		!strings.Contains(string(r), "role: modelspec:///m.A") {
		t.Fatalf("metadata rewrite wrong:\n%s", r)
	}

	// Idempotent: a second fix pass changes nothing.
	res2 := lintRepo(t, root, func(o *LintOptions) { o.Fix = true })
	if len(res2.Fixed) != 0 {
		t.Fatalf("second pass should change nothing, got %v", res2.Fixed)
	}
}

// TestFix_NoFrontmatter covers the early returns for files that have no
// frontmatter block or an unterminated one.
func TestFix_NoFrontmatter(t *testing.T) {
	for _, content := range []string{
		"no frontmatter here modelspec://m.A\n",
		"---\nkind: entity\nmodel: modelspec://m.A\n", // unterminated
		"",
	} {
		out, changed := rewriteLegacyModelspecFrontmatter([]byte(content))
		if changed || string(out) != content {
			t.Fatalf("expected no change for %q, got changed=%v", content, changed)
		}
	}
}

// TestFix_ReadError and TestFix_WriteError cover the defensive I/O branches via
// the injectable seams.
func TestFix_ReadError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://m.A"),
	})
	restore := readFileFn
	counts := map[string]int{}
	readFileFn = func(p string) ([]byte, error) {
		counts[p]++
		// Initial Load reads every file once; fail the fixer's read (the second
		// read of the module README) so the fixer's content-read branch is hit.
		if strings.HasSuffix(p, "README.md") && counts[p] == 2 {
			return nil, os.ErrPermission
		}
		return os.ReadFile(p)
	}
	defer func() { readFileFn = restore }()
	if _, err := Lint(LintOptions{RepoRoot: root, Fix: true}); err == nil {
		t.Fatal("expected read error to propagate")
	}
}

func TestFix_ReparseError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://m.A"),
	})
	restoreR, restoreW := readFileFn, writeFileFn
	wrote := false
	writeFileFn = func(p string, b []byte, m os.FileMode) error {
		wrote = true
		return os.WriteFile(p, b, m)
	}
	readFileFn = func(p string) ([]byte, error) {
		// The in-place re-parse is the only read that occurs after the write.
		if wrote && strings.HasSuffix(p, "e.md") {
			return nil, os.ErrPermission
		}
		return os.ReadFile(p)
	}
	defer func() { readFileFn, writeFileFn = restoreR, restoreW }()
	if _, err := Lint(LintOptions{RepoRoot: root, Fix: true}); err == nil {
		t.Fatal("expected re-parse read error to propagate")
	}
}

func TestFix_WriteError(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec://m.A"),
	})
	restore := writeFileFn
	writeFileFn = func(string, []byte, os.FileMode) error { return os.ErrPermission }
	defer func() { writeFileFn = restore }()
	if _, err := Lint(LintOptions{RepoRoot: root, Fix: true}); err == nil {
		t.Fatal("expected write error to propagate")
	}
}
