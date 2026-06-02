package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// gradeSpecRoot creates a temp project with the given raw specscore.yaml body
// (after the schema header) and returns the spec root. An empty cfgBody writes
// no specscore.yaml at all (exercises the read-error → default-set path).
func gradeSpecRoot(t *testing.T, cfgBody string) string {
	t.Helper()
	root := t.TempDir()
	if cfgBody != "" {
		body := projectdef.SchemaHeader + "\n" + cfgBody
		if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "spec")
}

func writeMD(t *testing.T, specRoot, rel, content string) string {
	t.Helper()
	p := filepath.Join(specRoot, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func gradeViolations(t *testing.T, specRoot string) []Violation {
	t.Helper()
	v, err := newGradeChecker().check(specRoot)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	return v
}

func rulesOf(vs []Violation) []string {
	out := make([]string, 0, len(vs))
	for _, v := range vs {
		out = append(out, v.Rule)
	}
	return out
}

const featHdr = "# Feature: X\n\n"

// --- AC: grade-absent-is-valid -------------------------------------------------

func TestGrade_AbsentIsValid(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/foo/README.md", featHdr+"**Status:** Approved\n\n## Summary\n\nx\n")
	if vs := gradeViolations(t, sr); len(vs) != 0 {
		t.Fatalf("expected no violations, got %v", vs)
	}
}

// --- AC: grade-default-scale-validated ----------------------------------------

func TestGrade_DefaultScale(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	writeMD(t, sr, "features/ok/README.md", featHdr+"**Status:** Approved\n**Grade:** A\n\n## Summary\n\nx\n")
	writeMD(t, sr, "features/bad/README.md", featHdr+"**Status:** Approved\n**Grade:** Z\n\n## Summary\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-value" {
		t.Fatalf("expected one grade-value violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Message, "A, B, C, D, F") {
		t.Fatalf("message should name default set: %s", vs[0].Message)
	}
}

// --- AC: grade-custom-scale-validated -----------------------------------------

func TestGrade_CustomScale(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\ngrade:\n  values: [1, 2, 3, 4, 5]\n")
	writeMD(t, sr, "features/ok/README.md", featHdr+"**Status:** Approved\n**Grade:** 3\n\n## Summary\n\nx\n")
	writeMD(t, sr, "features/bad/README.md", featHdr+"**Status:** Approved\n**Grade:** A\n\n## Summary\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-value" {
		t.Fatalf("expected one grade-value violation, got %v", vs)
	}
	if !strings.Contains(vs[0].Message, "1, 2, 3, 4, 5") {
		t.Fatalf("message should name configured set: %s", vs[0].Message)
	}
}

// --- AC: grade-values-shape-errors --------------------------------------------

func TestGrade_ShapeErrorSkipsPerFileChecks(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\ngrade:\n  values: A\n") // scalar = malformed
	// A bad grade here would normally be flagged, but shape error suppresses it.
	writeMD(t, sr, "features/bad/README.md", featHdr+"**Status:** Approved\n**Grade:** Z\n\n## Summary\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-values-shape" || vs[0].Severity != "error" {
		t.Fatalf("expected one grade-values-shape error, got %v", vs)
	}
}

// --- AC: grade-single-token-enforced ------------------------------------------

func TestGrade_SingleValue(t *testing.T) {
	cases := []struct{ name, grade string }{
		{"empty", "**Grade:**"},
		{"multi-token", "**Grade:** A, B"},
		{"whitespace-token", "**Grade:** A B"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sr := gradeSpecRoot(t, "project:\n  title: T\n")
			writeMD(t, sr, "features/f/README.md", featHdr+"**Status:** Approved\n"+tc.grade+"\n\n## Summary\n\nx\n")
			vs := gradeViolations(t, sr)
			if len(vs) != 1 || vs[0].Rule != "grade-single-value" {
				t.Fatalf("expected one grade-single-value violation, got %v", vs)
			}
		})
	}
}

// --- AC: grade-placement-enforced ---------------------------------------------

func TestGrade_Placement(t *testing.T) {
	t.Run("not-last", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		writeMD(t, sr, "features/f/README.md", featHdr+"**Grade:** A\n**Status:** Approved\n\n## Summary\n\nx\n")
		vs := gradeViolations(t, sr)
		if len(vs) != 1 || vs[0].Rule != "grade-placement" {
			t.Fatalf("expected one grade-placement violation, got %v", vs)
		}
	})
	t.Run("outside-block", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		writeMD(t, sr, "features/f/README.md", featHdr+"**Status:** Approved\n\n## Summary\n\n**Grade:** A\n")
		vs := gradeViolations(t, sr)
		if len(vs) != 1 || vs[0].Rule != "grade-placement" {
			t.Fatalf("expected one grade-placement violation, got %v", vs)
		}
	})
	t.Run("multiple", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		writeMD(t, sr, "features/f/README.md", featHdr+"**Status:** Approved\n**Grade:** A\n**Grade:** B\n\n## Summary\n\nx\n")
		vs := gradeViolations(t, sr)
		if len(vs) != 1 || vs[0].Rule != "grade-placement" {
			t.Fatalf("expected one grade-placement violation, got %v", vs)
		}
	})
}

// --- AC: grade-on-any-header-block-kind & grade-decoupled-from-workflow-and-status

func TestGrade_KindAgnosticAndDecoupled(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\n") // no reviewer gates configured
	// A Plan (non-Feature) with a Draft status and a valid grade — must pass.
	writeMD(t, sr, "plans/p.md", "# Plan: P\n\n**Status:** Draft\n**Grade:** B\n\n## Summary\n\nx\n")
	// An Idea (another kind), Draft, invalid grade — must be validated the same.
	writeMD(t, sr, "ideas/i.md", "# Idea: I\n\n**Status:** Draft\n**Grade:** Q\n\n## Problem Statement\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-value" || !strings.HasPrefix(vs[0].File, "ideas/") {
		t.Fatalf("expected one grade-value violation on the idea, got %v", vs)
	}
}

// --- duplicates advisory (REQ:grade-values-shape SHOULD) ----------------------

func TestGrade_DuplicateValuesAdvisory(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\ngrade:\n  values: [A, A, B]\n")
	writeMD(t, sr, "features/f/README.md", featHdr+"**Status:** Approved\n**Grade:** A\n\n## Summary\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-values-shape" || vs[0].Severity != "warning" {
		t.Fatalf("expected one grade-values-shape warning, got %v", vs)
	}
}

// --- read-error / no specscore.yaml → default set applies ---------------------

func TestGrade_NoConfigUsesDefault(t *testing.T) {
	sr := gradeSpecRoot(t, "") // no specscore.yaml written
	writeMD(t, sr, "features/ok/README.md", featHdr+"**Status:** Approved\n**Grade:** B\n\n## Summary\n\nx\n")
	writeMD(t, sr, "features/bad/README.md", featHdr+"**Status:** Approved\n**Grade:** Z\n\n## Summary\n\nx\n")
	vs := gradeViolations(t, sr)
	if len(vs) != 1 || vs[0].Rule != "grade-value" {
		t.Fatalf("expected one grade-value violation against default set, got %v", vs)
	}
}

// --- --fix placement normalization --------------------------------------------

func TestGradeFix_NormalizesPlacement(t *testing.T) {
	t.Run("reorders-within-block", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		p := writeMD(t, sr, "features/f/README.md", featHdr+"**Grade:** A\n**Status:** Approved\n\n## Summary\n\nx\n")
		if err := newGradeChecker().fix(sr); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		want := featHdr + "**Status:** Approved\n**Grade:** A\n\n## Summary\n\nx\n"
		if string(got) != want {
			t.Fatalf("fix mismatch:\n got: %q\nwant: %q", got, want)
		}
		// And it now lints clean.
		if vs := gradeViolations(t, sr); len(vs) != 0 {
			t.Fatalf("expected clean after fix, got %v", vs)
		}
	})
	t.Run("moves-out-of-block-grade-into-header", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		p := writeMD(t, sr, "features/f/README.md", featHdr+"**Status:** Approved\n\n## Summary\n\n**Grade:** A\n")
		if err := newGradeChecker().fix(sr); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if !strings.Contains(string(got), "**Status:** Approved\n**Grade:** A\n") {
			t.Fatalf("grade not normalized into header block: %q", got)
		}
	})
	t.Run("idempotent-when-already-canonical", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		orig := featHdr + "**Status:** Approved\n**Grade:** A\n\n## Summary\n\nx\n"
		p := writeMD(t, sr, "features/f/README.md", orig)
		if err := newGradeChecker().fix(sr); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if string(got) != orig {
			t.Fatalf("fix mutated a canonical file:\n got: %q\nwant: %q", got, orig)
		}
	})
	t.Run("noop-on-multiple-grades", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		orig := featHdr + "**Status:** Approved\n**Grade:** A\n**Grade:** B\n\n## Summary\n\nx\n"
		p := writeMD(t, sr, "features/f/README.md", orig)
		if err := newGradeChecker().fix(sr); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if string(got) != orig {
			t.Fatalf("fix should not touch multi-grade files")
		}
	})
	t.Run("noop-when-grade-is-only-metadata", func(t *testing.T) {
		sr := gradeSpecRoot(t, "project:\n  title: T\n")
		// No other metadata line, grade floating after a heading → removing it
		// leaves no header block to anchor to → no-op.
		orig := "# Feature: X\n\n## Summary\n\n**Grade:** A\n"
		p := writeMD(t, sr, "features/f/README.md", orig)
		if err := newGradeChecker().fix(sr); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(p)
		if string(got) != orig {
			t.Fatalf("fix should no-op when there is no header block: %q", got)
		}
	})
}

// --- helper-level coverage: headerBlockRange edge cases -----------------------

func TestHeaderBlockRange_NoMetadata(t *testing.T) {
	if s, e := headerBlockRange([]string{"# Feature: X", "", "## Summary", "prose"}); s != -1 || e != -1 {
		t.Fatalf("expected (-1,-1), got (%d,%d)", s, e)
	}
}

func TestParseMetaLine_NonMeta(t *testing.T) {
	if _, _, ok := parseMetaLine("just prose"); ok {
		t.Fatal("prose should not parse as a metadata line")
	}
}

func TestEqualLines_Lengths(t *testing.T) {
	if equalLines([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("different lengths must not be equal")
	}
}

func TestGradeChecker_NameAndSeverity(t *testing.T) {
	c := newGradeChecker()
	if c.name() != "grade-value" {
		t.Fatalf("name() = %q", c.name())
	}
	if c.severity() != "error" {
		t.Fatalf("severity() = %q", c.severity())
	}
}

func TestGrade_WalkErrorPropagates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based walk error is not reproducible as root")
	}
	sr := gradeSpecRoot(t, "project:\n  title: T\n")
	// An unreadable subdirectory makes filepath.Walk surface an error to the
	// walk callback, which propagates out of check().
	bad := filepath.Join(sr, "features", "locked")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o755) })

	if _, err := newGradeChecker().check(sr); err == nil {
		t.Fatal("expected a walk error from the unreadable directory")
	}
}
