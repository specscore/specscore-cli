package cli

import (
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

const conflictedLessonRows = "<<<<<<< HEAD\n| [kinder-fake](kinder-fake/README.md) | Stated |\n=======\n| [kinder-fake](kinder-fake/README.md) | Withdrawn |\n>>>>>>> topic\n"

// treeDigest hashes every regular file under root, keyed by its path.
func treeDigest(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		b, err := os.ReadFile(p)
		out[p] = sha256.Sum256(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-fix-refused-byte-identical — `spec lint --fix`
// over a tree with a conflicted file modifies nothing (not even the unrelated
// autofixable file), names the conflicted file on stderr, and exits non-zero.
func TestSpecLint_FixRefusedWhileConflictMarkersRemain(t *testing.T) {
	root := setupAutofixProject(t)
	conflicted := filepath.Join(root, "spec", "features", "README.md")
	if err := os.WriteFile(conflicted, []byte("# Features\n\n| Feature | Status |\n|---|---|\n"+strings.ReplaceAll(conflictedLessonRows, "kinder-fake", "needsfix")), 0o644); err != nil {
		t.Fatal(err)
	}
	before := treeDigest(t, root)

	stdout, stderr, err := runSpec(t, "lint", "--project", root, "--fix")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (violations); err=%v", got, exitcode.Conflict, err)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("spec lint --fix modified files while merge-conflict markers were present")
	}
	if !strings.Contains(stderr, "spec lint --fix refused") || !strings.Contains(stderr, "spec/features/README.md") {
		t.Fatalf("stderr must name the refusal and the conflicted file, got: %q", stderr)
	}
	if strings.Contains(stderr, "Fixed") {
		t.Fatalf("no Fixed summary may be printed, got: %q", stderr)
	}
	if !strings.Contains(stdout, "merge-conflict-markers") || strings.Contains(stdout, "adherence-footer") {
		t.Fatalf("stdout must report only the marker findings, got: %q", stdout)
	}

	// Read-only lint reports the same findings, exits non-zero, and prints
	// no --fix refusal line.
	stdout, stderr, err = runSpec(t, "lint", "--project", root)
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("read-only exit = %d, want %d; err=%v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(stdout, "features/README.md:5") || strings.Contains(stderr, "refused") {
		t.Fatalf("read-only lint: stdout=%q stderr=%q", stdout, stderr)
	}
}

// specscore:verifies feature/cli/lesson/change-status#ac:conflicted-lessons-index-fails-closed — a status transition over a
// conflicted Lessons index is refused by the shared precondition before the
// Lesson file is rewritten or any event is prepared: it exits 1 naming the
// index, and the whole tree — Lesson file, index, event ledger — stays
// byte-identical. Covered: a conflict on the Lesson's own row, one elsewhere
// in the index, a one-sided block, and a conflict inside the legacy
// five-column table (which the index upsert would otherwise regenerate,
// erasing the markers).
func TestLessonChangeStatus_ConflictedIndexFailsClosed_CLI(t *testing.T) {
	ownRow := func(index string) (string, bool) {
		var b strings.Builder
		replaced := false
		for _, line := range strings.SplitAfter(index, "\n") {
			if !replaced && strings.HasPrefix(line, "| ") && strings.Contains(line, "kinder-fake") {
				b.WriteString(conflictedLessonRows)
				replaced = true
				continue
			}
			b.WriteString(line)
		}
		return b.String(), replaced
	}
	cases := []struct {
		name     string
		conflict func(index string) (string, bool)
	}{
		{"own-row", ownRow},
		{"elsewhere-in-index", func(index string) (string, bool) {
			const oq = "## Open Questions\n"
			i := strings.Index(index, oq)
			if i < 0 {
				return index, false
			}
			return index[:i+len(oq)] + "\n<<<<<<< HEAD\nNone yet.\n=======\nOne from topic.\n>>>>>>> topic\n" + index[i+len(oq):], true
		}},
		{"one-sided-block", func(index string) (string, bool) {
			const sep = "|---|---|---|---:|---|\n"
			return strings.Replace(index, sep, sep+"<<<<<<< HEAD\n| [other](other/README.md) | Recorded | — | 0 | — |\n=======\n>>>>>>> topic\n", 1), strings.Contains(index, sep)
		}},
		{"legacy-table", func(index string) (string, bool) {
			i := strings.Index(index, "| Lesson | Status |")
			if i < 0 {
				return index, false
			}
			return index[:i] + "| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n" +
				"<<<<<<< HEAD\n| [kinder-fake](kinder-fake/README.md) | Recorded | 0 | 2026-11-27 | tester |\n=======\n| [other](other.md) | Recorded | 0 | 2026-11-27 | x |\n>>>>>>> topic\n", true
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := stageLesson(t, "kinder-fake", "Recorded")
			indexPath := filepath.Join(root, "spec", "lessons", "README.md")
			raw, err := os.ReadFile(indexPath)
			if err != nil {
				t.Fatal(err)
			}
			conflicted, ok := tc.conflict(string(raw))
			if !ok {
				t.Fatalf("fixture index lacks the region to conflict:\n%s", raw)
			}
			if err := os.WriteFile(indexPath, []byte(conflicted), 0o644); err != nil {
				t.Fatal(err)
			}
			before := treeDigest(t, root)

			stdout, _, err := runLesson(t, "change-status", "kinder-fake", "--to=stated")
			if got := exitCodeOfErr(err); got != exitcode.Conflict {
				t.Fatalf("exit = %d, want %d; err=%v stdout=%q", got, exitcode.Conflict, err, stdout)
			}
			if !strings.Contains(err.Error(), "spec/lessons/README.md") {
				t.Errorf("error must name the conflicted index: %v", err)
			}
			if !reflect.DeepEqual(treeDigest(t, root), before) {
				t.Fatal("the Lesson file, index, or event ledger changed before the refusal")
			}
		})
	}
}

// specscore:verifies feature/cli/spec/lint#ac:producer-output-round-trips-through-fix — output written by the
// producer verbs passes the validator unchanged: lint reports no error and a
// following `spec lint --fix` modifies zero bytes.
func TestProducerOutput_LintThenFixIsByteIdentical(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Recorded")
	steps := [][]string{
		{"new", "second-lesson", "--owner", "tester"},
		{"change-status", "kinder-fake", "--to=stated"},
		{"change-status", "kinder-fake", "--to=recorded", "--note", "Enforcement section named no binding control"},
	}
	for _, step := range steps {
		if _, stderr, err := runLesson(t, step...); err != nil {
			t.Fatalf("lesson %v: %v (stderr=%s)", step, err, stderr)
		}
	}
	if _, stderr, err := runIdea(t, "new", "round-trip-idea"); err != nil {
		t.Fatalf("idea new: %v (stderr=%s)", err, stderr)
	}

	specRoot := filepath.Join(root, "spec")
	vs, err := lint.Lint(lint.Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("producer output fails lint: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	before := treeDigest(t, root)
	res, err := lint.LintWithResult(lint.Options{SpecRoot: specRoot, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Fixed) != 0 || len(res.FixRefused) != 0 {
		t.Errorf("--fix after producers: Fixed=%v FixRefused=%v, want both empty", res.Fixed, res.FixRefused)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Error("--fix changed bytes the producers wrote")
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-fix-refused-byte-identical — the index merge
// driver relies on the --fix pass; when that pass is refused because a
// sibling file is still conflict-marked, the driver fails closed (leaves
// <ours> untouched, exits non-zero naming the file) instead of copying the
// unregenerated index over <ours> and reporting a resolution.
func TestMergeDriverIndex_FailsClosedWhenFixRefusedOverConflictMarkers(t *testing.T) {
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	writeSpecscoreYAML(t, root, "")
	withCwd(t, root)
	source := filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("# Lesson: Kinder Fake\n<<<<<<< HEAD\n**Status:** Stated\n=======\n**Status:** Recorded\n>>>>>>> topic\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A stale on-disk index exists, so without the FixRefused check the
	// driver would copy it over <ours> and exit 0.
	staleIndex := filepath.Join(root, "spec", "lessons", "README.md")
	if err := os.WriteFile(staleIndex, []byte("# Lessons\n\nstale on-disk index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ours := filepath.Join(t.TempDir(), "git-temp-a")
	oursBefore := []byte("ours index before the merge\n")
	if err := os.WriteFile(ours, oursBefore, 0o644); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(t.TempDir(), "git-temp-o")
	theirs := filepath.Join(t.TempDir(), "git-temp-b")
	_ = os.WriteFile(base, nil, 0o644)
	_ = os.WriteFile(theirs, nil, 0o644)

	stdout, errOut, err := runMergeDriver(t, "index", base, ours, theirs, "spec/lessons/README.md")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(errOut, "lessons/kinder-fake/README.md") || strings.Contains(stdout, "regenerated=") {
		t.Fatalf("stdout=%q stderr=%q, want a refusal naming the conflicted file and no regenerated= line", stdout, errOut)
	}
	got, err := os.ReadFile(ours)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(oursBefore) {
		t.Fatalf("<ours> was rewritten: %q", got)
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-fix-refused-byte-identical — under
// `--fix --format json` the refusal is carried in the envelope's
// `fix_refused` key and no human text is written to stderr.
func TestSpecLint_FixRefusedJSONEnvelope(t *testing.T) {
	root := setupAutofixProject(t)
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte("# Features\n<<<<<<< HEAD\n=======\n>>>>>>> t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := runSpec(t, "lint", "--project", root, "--fix", "--format=json")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d; err=%v", got, err)
	}
	var env struct {
		Fixed      []string         `json:"fixed"`
		FixRefused []string         `json:"fix_refused"`
		Violations []lint.Violation `json:"violations"`
	}
	if err := json.Unmarshal([]byte(stdout), &env); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, stdout)
	}
	if !reflect.DeepEqual(env.FixRefused, []string{"spec/features/README.md"}) || len(env.Fixed) != 0 || len(env.Violations) != 3 {
		t.Fatalf("envelope = %+v", env)
	}
	if strings.Contains(stderr, "refused") {
		t.Fatalf("structured output must not write human refusal text to stderr: %q", stderr)
	}
}

func TestProjectRelativeSpecPaths_KeepsAbsoluteFallback(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "x", "README.md")
	got := projectRelativeSpecPaths(filepath.Join("/proj", "spec"), []string{"features/README.md", abs, "../tasks/README.md"})
	want := []string{"spec/features/README.md", filepath.ToSlash(abs), "tasks/README.md"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
