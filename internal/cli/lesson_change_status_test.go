package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// stageLesson bootstraps a spec repo, scaffolds a lint-clean lesson via
// `lesson new`, patches its **Status:** to the requested value, runs lint
// --fix to sync the lessons index, and returns the project root (cwd set to
// it) — mirroring stagePlan.
func stageLesson(t *testing.T, slug, status string) string {
	t.Helper()
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runLesson(t, "new", slug, "--owner", "tester"); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	// Minimal lint-clean Features index (init's job; lesson new does not
	// write it) — mirrors stagePlan.
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}

	path := filepath.Join(root, "spec", "lessons", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lesson: %v", err)
	}
	patched := strings.Replace(string(raw), "**Status:** Recorded", "**Status:** "+status, 1)
	patched = strings.Replace(patched, "status: Recorded", "status: "+status, 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched lesson: %v", err)
	}

	// Backfill frontmatter on the hand-written features index so the tree is
	// lint-clean before the verb under test runs.
	migrateTree(t, root)

	if _, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true}); err != nil {
		t.Fatalf("initial lint --fix: %v", err)
	}
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("verify lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Fatalf("pre-existing lint error in fixture: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	return root
}

func TestLessonChangeStatus_RecordedToStated_CLI(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Recorded")
	stdout, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=stated")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "kinder-fake: Recorded → Stated\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake.md"))
	if !strings.Contains(string(body), "**Status:** Stated") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

func TestLessonChangeStatus_RecordedToEnforced_SkipAhead_CLI(t *testing.T) {
	stageLesson(t, "kinder-fake", "Recorded")
	stdout, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=ENFORCED")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "kinder-fake: Recorded → Enforced\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
}

func TestLessonChangeStatus_Withdrawn_CLI(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Stated")
	_, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=withdrawn", "--note", "turned out to be a one-off")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake.md"))
	if !strings.Contains(string(body), "**Status:** Withdrawn") || !strings.Contains(string(body), "turned out to be a one-off") {
		t.Errorf("withdrawn/resolution not written:\n%s", body)
	}
}

func TestLessonChangeStatus_Superseded_CLI(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Enforced")
	if _, _, err := runLesson(t, "new", "kinder-fake-v2", "--owner", "tester"); err != nil {
		t.Fatalf("lesson new kinder-fake-v2: %v", err)
	}
	_, stderr, err := runLesson(t, "change-status", "kinder-fake", "--to=superseded", "--note", "generalized", "--successor", "kinder-fake-v2")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake.md"))
	for _, want := range []string{"**Status:** Superseded", "**Superseded By:** kinder-fake-v2", "generalized"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q:\n%s", want, body)
		}
	}
}

func TestLessonChangeStatus_ArgErrors_CLI(t *testing.T) {
	stageLesson(t, "kinder-fake", "Enforced")
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing-slug", []string{"change-status", "--to=stated"}, exitcode.InvalidArgs},
		{"too-many-args", []string{"change-status", "kinder-fake", "extra", "--to=stated"}, exitcode.InvalidArgs},
		{"bad-slug", []string{"change-status", "Bad_Slug", "--to=stated"}, exitcode.InvalidArgs},
		{"missing-to", []string{"change-status", "kinder-fake"}, exitcode.InvalidArgs},
		{"unrecognized-to", []string{"change-status", "kinder-fake", "--to=banana"}, exitcode.InvalidArgs},
		{"withdrawn-no-note", []string{"change-status", "kinder-fake", "--to=withdrawn"}, exitcode.InvalidArgs},
		{"superseded-no-note", []string{"change-status", "kinder-fake", "--to=superseded"}, exitcode.InvalidArgs},
		{"superseded-no-successor", []string{"change-status", "kinder-fake", "--to=superseded", "--note", "x"}, exitcode.InvalidArgs},
		{"successor-on-non-superseded", []string{"change-status", "kinder-fake", "--to=withdrawn", "--note", "x", "--successor", "kinder-fake"}, exitcode.InvalidArgs},
		{"illegal-transition", []string{"change-status", "kinder-fake", "--to=stated"}, exitcode.InvalidState},
		{"not-found", []string{"change-status", "ghost", "--to=stated"}, exitcode.NotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runLesson(t, tc.args...)
			if got := exitCodeOfErr(err); got != tc.want {
				t.Errorf("exit = %d, want %d; err=%v", got, tc.want, err)
			}
		})
	}
}

// AC: superseded-requires-successor — unresolvable successor exits 2.
func TestLessonChangeStatus_SupersededUnresolvableSuccessor_CLI(t *testing.T) {
	stageLesson(t, "kinder-fake", "Enforced")
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=superseded", "--note", "x", "--successor", "nope")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: lint-failure-rolls-back — inject a lint failure via the lintLintFn seam.
func TestLessonChangeStatus_LintFailureRollsBack_CLI(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Recorded")
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return []lint.Violation{{File: "x", Line: 1, Rule: "L-001", Severity: "error", Message: "boom"}}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=stated")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake.md"))
	if !strings.Contains(string(body), "**Status:** Recorded") {
		t.Errorf("status not rolled back after lint failure:\n%s", body)
	}
}

func TestLessonChangeStatus_ProjectResolveError_CLI(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=stated", "--project", bare)
	if err == nil {
		t.Fatal("expected resolveSpecRoot error, got nil")
	}
}
