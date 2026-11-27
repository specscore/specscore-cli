package lint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// conflictedIndex is a generated Features index whose rows were edited on two
// branches: git left both row-shaped halves between conflict markers, each of
// which parses as an ordinary Markdown table row.
const conflictedIndex = "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n" +
	"<<<<<<< HEAD\n| [alpha](alpha/README.md) | Draft |\n=======\n| [alpha](alpha/README.md) | Approved |\n>>>>>>> feature-branch\n" +
	"\n## Open Questions\n\nNone at this time.\n"

func markerFindings(vs []Violation) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Rule == mergeConflictMarkersRule {
			out = append(out, v)
		}
	}
	return out
}

func writeTreeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-in-artifact-flagged — a conflict inside an
// artifact is reported with file and line, and it is the only finding even
// though other rules (missing footer, missing OQ section) would fire.
func TestMergeConflictMarkers_ArtifactFlaggedAndOnlyFinding(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "features/alpha/README.md",
		"# Feature: Alpha\n\n<<<<<<< HEAD\n**Status:** Draft\n=======\n**Status:** Approved\n>>>>>>> topic\n")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 {
		t.Fatalf("want exactly the 3 marker findings, got %d: %+v", len(vs), vs)
	}
	for i, wantLine := range []int{3, 5, 7} {
		v := vs[i]
		if v.Rule != mergeConflictMarkersRule || v.Severity != "error" || v.File != "features/alpha/README.md" || v.Line != wantLine {
			t.Errorf("finding %d = %+v, want %s error at features/alpha/README.md:%d", i, v, mergeConflictMarkersRule, wantLine)
		}
	}
	if !strings.Contains(vs[0].Message, "<<<<<<<") || !strings.Contains(vs[0].Message, "--fix") {
		t.Errorf("message must name the marker and the --fix refusal: %q", vs[0].Message)
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-in-generated-index-flagged — both halves of a
// conflicted generated index are row-shaped Markdown; the gate still flags
// the markers, and runs regardless of --rules / --ignore.
func TestMergeConflictMarkers_GeneratedIndexFlagged(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "features/README.md", conflictedIndex)
	for _, opts := range []Options{
		{SpecRoot: specRoot},
		{SpecRoot: specRoot, Rules: []string{"index-entries"}},
		{SpecRoot: specRoot, Ignore: []string{mergeConflictMarkersRule}},
		{SpecRoot: specRoot, Severity: "error"},
	} {
		vs, err := Lint(opts)
		if err != nil {
			t.Fatal(err)
		}
		if got := markerFindings(vs); len(got) != 3 || len(vs) != 3 {
			t.Fatalf("opts %+v: want only 3 marker findings, got %+v", opts, vs)
		}
		if vs[0].File != "features/README.md" || vs[0].Line != 7 {
			t.Errorf("first finding = %+v, want features/README.md:7", vs[0])
		}
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-setext-heading-not-flagged — a bare `=======`
// setext H1 underline outside a `<<<<<<<` block is ordinary Markdown.
func TestMergeConflictMarkers_SetextHeadingNotFlagged(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "research/README.md", "Research\n=======\n\nNotes.\n")
	c := newMergeConflictMarkerChecker("", "", false)
	vs, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("setext heading must not be flagged, got %+v", vs)
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-diff3-flagged — the diff3/zdiff3 base marker is
// reported alongside the other three.
func TestMergeConflictMarkers_Diff3Flagged(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "lessons/README.md",
		"# Lessons\n<<<<<<< HEAD\n| a |\n||||||| merged common ancestors\n| base |\n=======\n| b |\n>>>>>>> topic\n")
	vs, err := newMergeConflictMarkerChecker("", "", false).check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	var markers []string
	for _, v := range vs {
		markers = append(markers, strings.SplitN(v.Message, "\"", 3)[1])
	}
	if want := []string{"<<<<<<<", "|||||||", "=======", ">>>>>>>"}; !reflect.DeepEqual(markers, want) {
		t.Fatalf("markers = %v, want %v (%+v)", markers, want, vs)
	}
}

func snapshotBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		out[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-fix-refused-byte-identical — with markers
// present, --fix runs no fixer at all: the conflicted file AND an unrelated
// file that would otherwise be autofixed stay byte-identical, Fixed is empty,
// FixRefused names the conflicted file, and the marker findings remain (so
// the CLI exits non-zero).
func TestMergeConflictMarkers_FixRefusedLeavesTreeByteIdentical(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "features/README.md", conflictedIndex)
	writeFeatureReadme(t, specRoot, "needsfix", "# Feature: Needs Fix\n\n**Status:** Draft\n\n## Summary\nNo footer here.\n")
	before := snapshotBytes(t, specRoot)

	res, err := LintWithResult(Options{SpecRoot: specRoot, Fix: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshotBytes(t, specRoot), before) {
		t.Fatal("--fix modified the tree while merge-conflict markers were present")
	}
	if len(res.Fixed) != 0 || len(res.Reconciled) != 0 {
		t.Errorf("Fixed=%v Reconciled=%v, want both empty", res.Fixed, res.Reconciled)
	}
	if want := []string{"features/README.md"}; !reflect.DeepEqual(res.FixRefused, want) {
		t.Errorf("FixRefused = %v, want %v", res.FixRefused, want)
	}
	if CountFailing(res.Violations) != 3 || len(markerFindings(res.Violations)) != 3 {
		t.Errorf("violations = %+v, want exactly the 3 failing marker findings", res.Violations)
	}

	// Without --fix the result never carries FixRefused.
	res, err = LintWithResult(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatal(err)
	}
	if res.FixRefused != nil {
		t.Errorf("read-only lint FixRefused = %v, want nil", res.FixRefused)
	}
}

// AC: merge-conflict-markers-detected — scope: the project-root tasks/ board
// and an externally routed Plans directory are scanned; the local spec/plans
// mirror is skipped once Plans are routed away (or the route is broken);
// other hidden directories are scanned, but nested .git directories,
// benchmark fixtures, binary files and symlinks are not.
func TestMergeConflictMarkers_Scope(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	marked := "<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> t\n"
	writeTreeFile(t, project, "tasks/README.md", marked)
	writeTreeFile(t, specRoot, "plans/README.md", marked)
	writeTreeFile(t, specRoot, ".hidden/README.md", marked)
	writeTreeFile(t, specRoot, "features/x/.git/MERGE_MSG", marked)
	writeTreeFile(t, specRoot, "features/x/benchmark/fixture.md", marked)
	writeTreeFile(t, specRoot, "features/x/image.bin", "\x00"+marked)
	if err := os.Symlink(filepath.Join(project, "tasks", "README.md"), filepath.Join(specRoot, "features", "x", "link.md")); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "plans")
	writeTreeFile(t, external, "README.md", marked)

	files := func(c *mergeConflictMarkerChecker) []string {
		t.Helper()
		vs, err := c.check(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, v := range vs {
			if v.Line == 1 {
				out = append(out, v.File)
			}
		}
		return out
	}

	if got, want := files(newMergeConflictMarkerChecker(project, "", false)), []string{".hidden/README.md", "plans/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same-repo plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, filepath.Join(specRoot, "plans"), false)), []string{".hidden/README.md", "plans/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("plans routed to spec/plans: files = %v, want %v", got, want)
	}
	extRel, _ := filepath.Rel(specRoot, filepath.Join(external, "README.md"))
	if got, want := files(newMergeConflictMarkerChecker(project, external, false)), []string{".hidden/README.md", filepath.ToSlash(extRel), "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("external plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, filepath.Join(project, "missing"), false)), []string{".hidden/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("missing external plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, "", true)), []string{".hidden/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("broken route: files = %v, want %v", got, want)
	}
}

// A tasks/ directory that lies inside the spec tree is scanned once, not
// twice (project root == spec root).
func TestMergeConflictMarkers_TasksWithinSpecRootScannedOnce(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "tasks/README.md", "<<<<<<< HEAD\n")
	vs, err := newMergeConflictMarkerChecker(specRoot, "", false).check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].File != "tasks/README.md" {
		t.Fatalf("violations = %+v, want one finding in tasks/README.md", vs)
	}
}

func TestMergeConflictMarkers_RegisteredInCoreFamily(t *testing.T) {
	var found *Rule
	for _, r := range AllRules() {
		if r.ID == mergeConflictMarkersRule {
			r := r
			found = &r
		}
	}
	if found == nil || found.Family != "core" || found.Severity != "error" || found.Advisory {
		t.Fatalf("registry entry = %+v, want core/error/non-advisory", found)
	}
	if err := ValidateRuleNames([]string{mergeConflictMarkersRule}); err != nil {
		t.Fatal(err)
	}
	c := newMergeConflictMarkerChecker("", "", false)
	if c.name() != mergeConflictMarkersRule || c.severity() != "error" {
		t.Fatalf("checker identity = %s/%s", c.name(), c.severity())
	}
}

// The gate fails closed on every I/O error. Each fixture reaches its error
// branch through real filesystem state, not a swapped function.
func TestMergeConflictMarkers_ErrorPathsFailClosed(t *testing.T) {
	t.Run("walk-error", func(t *testing.T) {
		specRoot := t.TempDir()
		locked := filepath.Join(specRoot, "locked")
		writeTreeFile(t, specRoot, "locked/README.md", "# x\n")
		if err := os.Chmod(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		if _, err := newMergeConflictMarkerChecker("", "", false).check(specRoot); err == nil {
			t.Fatal("want the unreadable-directory error")
		}
		if _, err := LintWithResult(Options{SpecRoot: specRoot, Fix: true}); err == nil || !strings.Contains(err.Error(), mergeConflictMarkersRule) {
			t.Fatalf("LintWithResult err = %v, want the gate error naming the rule", err)
		}
	})
	t.Run("read-error", func(t *testing.T) {
		specRoot := t.TempDir()
		p := writeTreeFile(t, specRoot, "README.md", "# x\n")
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := newMergeConflictMarkerChecker("", "", false).check(specRoot); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("err = %v, want the permission error", err)
		}
	})
	t.Run("config-read-error", func(t *testing.T) {
		project := t.TempDir()
		specRoot := filepath.Join(project, "spec")
		writeTreeFile(t, specRoot, "README.md", "# Spec\n")
		cfg := writeTreeFile(t, project, "specscore.yaml", "project: {}\n")
		if err := os.Chmod(cfg, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := newMergeConflictMarkerChecker(project, "", false).check(specRoot); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("err = %v, want the config permission error", err)
		}
	})
	t.Run("skills-dir-read-error", func(t *testing.T) {
		project := t.TempDir()
		specRoot := filepath.Join(project, "spec")
		writeTreeFile(t, specRoot, "rules/README.md", "# Rules\n")
		writeTreeFile(t, project, "ai/skills/demo/SKILL.md", "# Skill\n")
		skills := filepath.Join(project, "ai", "skills")
		if err := os.Chmod(skills, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(skills, 0o755) })
		if _, err := newMergeConflictMarkerChecker(project, "", false).check(specRoot); !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("err = %v, want the skills-directory permission error", err)
		}
	})
	t.Run("symlink-loop-root", func(t *testing.T) {
		project := t.TempDir()
		specRoot := filepath.Join(project, "spec")
		writeTreeFile(t, specRoot, "README.md", "# Spec\n")
		if err := os.Symlink("tasks", filepath.Join(project, "tasks")); err != nil {
			t.Fatal(err)
		}
		if _, err := newMergeConflictMarkerChecker(project, "", false).check(specRoot); err == nil {
			t.Fatal("want the symlink-loop error for tasks/")
		}
	})
	t.Run("rel-error-falls-back-to-path", func(t *testing.T) {
		// A relative spec root beside an absolute project root: the tasks/
		// board cannot be expressed relative to the spec root, so the
		// finding names its absolute path.
		project := t.TempDir()
		writeTreeFile(t, project, "spec/README.md", "# Spec\n")
		board := writeTreeFile(t, project, "tasks/README.md", "<<<<<<< HEAD\n")
		t.Chdir(project)
		vs, err := newMergeConflictMarkerChecker(project, "", false).check("spec")
		if err != nil {
			t.Fatal(err)
		}
		if len(vs) != 1 || vs[0].File != filepath.ToSlash(board) {
			t.Fatalf("violations = %+v, want the absolute path fallback", vs)
		}
	})
}

func TestConflictMarkerFiles_SortedUnique(t *testing.T) {
	got := conflictMarkerFiles([]Violation{{File: "b.md"}, {File: "a.md"}, {File: "b.md"}})
	if want := []string{"a.md", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("conflictMarkerFiles = %v, want %v", got, want)
	}
}

func checkerFiles(t *testing.T, c *mergeConflictMarkerChecker, specRoot string) []string {
	t.Helper()
	vs, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	return conflictMarkerFiles(vs)
}

// AC: merge-conflict-markers-detected — a symlinked spec root (and a
// symlinked tasks/ board) is resolved before the walk, so its files are
// scanned rather than silently skipped; a conflicted specscore.yaml is
// reported too.
func TestMergeConflictMarkers_SymlinkedRootsAndConfig(t *testing.T) {
	real := t.TempDir()
	marked := "<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> t\n"
	writeTreeFile(t, real, "spec-real/features/README.md", marked)
	writeTreeFile(t, real, "tasks-real/README.md", marked)
	project := t.TempDir()
	if err := os.Symlink(filepath.Join(real, "spec-real"), filepath.Join(project, "spec")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "tasks-real"), filepath.Join(project, "tasks")); err != nil {
		t.Fatal(err)
	}
	writeTreeFile(t, project, "specscore.yaml", "project:\n<<<<<<< HEAD\n  repo: a\n=======\n  repo: b\n>>>>>>> t\n")

	got := checkerFiles(t, newMergeConflictMarkerChecker("", "", false), filepath.Join(project, "spec"))
	if want := []string{"../specscore.yaml", "../tasks/README.md", "features/README.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

// AC: merge-conflict-markers-detected — the paired skill files the rule
// rules read (<skills_path>/<name>/SKILL.md) are scanned when the project
// has a Rules tree; specscore.yaml is scanned once even when the project
// root is the spec root.
func TestMergeConflictMarkers_SkillsAndConfigScannedOnce(t *testing.T) {
	marked := "<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> t\n"
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	writeTreeFile(t, specRoot, "README.md", "# Spec\n")
	writeTreeFile(t, project, "ai/skills/demo/SKILL.md", marked)
	writeTreeFile(t, project, "ai/skills/demo/notes.md", marked) // not read by lint
	writeTreeFile(t, project, "ai/skills/README.md", marked)     // not a skill pair
	writeTreeFile(t, project, "tasks", marked)                   // not a board directory
	if got := checkerFiles(t, newMergeConflictMarkerChecker(project, "", false), specRoot); len(got) != 0 {
		t.Fatalf("no Rules tree: files = %v, want none", got)
	}
	writeTreeFile(t, specRoot, "rules/README.md", "# Rules\n")
	if got, want := checkerFiles(t, newMergeConflictMarkerChecker(project, "", false), specRoot), []string{"../ai/skills/demo/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
	writeTreeFile(t, project, "specscore.yaml", projectdef.SchemaHeader+"\nrules:\n  skills_path: custom\n")
	writeTreeFile(t, project, "custom/other/SKILL.md", marked)
	if got, want := checkerFiles(t, newMergeConflictMarkerChecker(project, "", false), specRoot), []string{"../custom/other/SKILL.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("configured skills_path: files = %v, want %v", got, want)
	}

	flat := t.TempDir()
	writeTreeFile(t, flat, "specscore.yaml", marked)
	vs, err := newMergeConflictMarkerChecker(flat, "", false).check(flat)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 3 {
		t.Fatalf("violations = %+v, want specscore.yaml's 3 markers reported once", vs)
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-in-code-fence-flagged — Markdown code fences are
// not special: a conflict inside a fenced block, or straddling a fence
// boundary, is flagged; only a marker indented off column 0 (the documented
// way to quote one) is not.
func TestMergeConflictMarkers_CodeFenceConflictFlagged(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "features/a/README.md", "# A\n\n```bash\n<<<<<<< HEAD\nrun a\n=======\nrun b\n>>>>>>> topic\n```\n")
	writeTreeFile(t, specRoot, "features/b/README.md", "# B\n\n```go\n<<<<<<< HEAD\nx()\n```\nprose\n=======\n```\n>>>>>>> topic\n```\n")
	writeTreeFile(t, specRoot, "research/README.md", "# Resolving conflicts\n\n    <<<<<<< HEAD\n    ours\n    =======\n    theirs\n    >>>>>>> topic\n")
	vs, err := newMergeConflictMarkerChecker("", "", false).check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := conflictMarkerFiles(vs), []string{"features/a/README.md", "features/b/README.md"}; !reflect.DeepEqual(got, want) || len(vs) != 6 {
		t.Fatalf("files = %v (%d findings), want %v with 6 findings", got, len(vs), want)
	}
}

// specscore:verifies feature/cli/spec/lint#ac:merge-conflict-markers-fix-refused-byte-identical — Lint (the API every
// in-process Fix: true caller uses) returns *FixRefusedError rather than an
// empty, success-shaped violation list.
func TestLint_FixRefusedIsAnError(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	writeTreeFile(t, specRoot, "features/README.md", conflictedIndex)
	before := snapshotBytes(t, project)

	_, err := Lint(Options{SpecRoot: specRoot, Fix: true})
	var refused *FixRefusedError
	if !errors.As(err, &refused) || !reflect.DeepEqual(refused.Files, []string{"features/README.md"}) || !strings.Contains(err.Error(), "features/README.md") {
		t.Fatalf("Lint(Fix) err = %v, want *FixRefusedError naming features/README.md", err)
	}
	if vs, err := Lint(Options{SpecRoot: specRoot}); err != nil || len(vs) != 3 {
		t.Fatalf("read-only Lint = %v, %v; want the 3 marker findings and no error", vs, err)
	}
	if !reflect.DeepEqual(snapshotBytes(t, project), before) {
		t.Fatal("a refused fix modified the tree")
	}
}

// AC: merge-conflict-markers-detected — the fixers that walk the tree skip
// exactly what the gate skips (nested .git, benchmark/, symlinks), so --fix
// never rewrites a file the gate did not read.
func TestTreeFixers_SkipWhatTheGateSkips(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	legacy := "# Notes\n\n## Outstanding Questions\n\nNone.\n"
	skipped := []string{
		writeTreeFile(t, specRoot, "features/x/benchmark/README.md", legacy),
		writeTreeFile(t, specRoot, "features/x/.git/README.md", legacy),
	}
	target := writeTreeFile(t, project, "outside.md", legacy)
	link := filepath.Join(specRoot, "features", "x", "link.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := oqFixWalk(project, specRoot, specRoot, nil); err != nil {
		t.Fatal(err)
	}
	for _, p := range append(skipped, target) {
		if b, _ := os.ReadFile(p); string(b) != legacy {
			t.Errorf("oqFixWalk rewrote %s", p)
		}
	}

	ideas := filepath.Join(specRoot, "ideas")
	legacyIdea := "# Idea: X\n\n**Status:** Archived\n"
	ideaSkipped := []string{
		writeTreeFile(t, ideas, "benchmark/x.md", legacyIdea),
		writeTreeFile(t, ideas, ".git/x.md", legacyIdea),
	}
	ideaTarget := writeTreeFile(t, project, "outside-idea.md", legacyIdea)
	if err := os.Symlink(ideaTarget, filepath.Join(ideas, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := fixLegacyStatusesInTree(ideas, legacyIdeaStatusMap, true); err != nil {
		t.Fatal(err)
	}
	for _, p := range append(ideaSkipped, ideaTarget) {
		if b, _ := os.ReadFile(p); string(b) != legacyIdea {
			t.Errorf("fixLegacyStatusesInTree rewrote %s", p)
		}
	}
}

// specscore:verifies feature/cli/spec/lint#ac:writers-refuse-conflicted-index — UpsertLessonIndexRow refuses a
// conflicted Lessons index before both the row rewrite and the legacy
// five-column table migration (which would replace the whole table and erase
// the markers); the index stays byte-identical. The canonical case uses a
// one-sided conflict block (one branch added a row, the other added nothing).
func TestUpsertLessonIndexRow_RefusesConflictedIndex(t *testing.T) {
	cases := map[string]string{
		"canonical-one-sided": "# Lessons\n\n## Lessons\n\n| Lesson | Status | Classifications | Occurrences | Last Occurred |\n|---|---|---|---:|---|\n" +
			"<<<<<<< HEAD\n| [other](other/README.md) | Recorded | process | 0 | — |\n=======\n>>>>>>> topic\n",
		"legacy-table": "# Lessons\n\n## Lessons\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n" +
			"<<<<<<< HEAD\n| [a](a.md) | Recorded | 0 | 2026-08-01 | x |\n=======\n| [b](b.md) | Recorded | 0 | 2026-08-01 | y |\n>>>>>>> topic\n",
	}
	for name, index := range cases {
		t.Run(name, func(t *testing.T) {
			specRoot := filepath.Join(t.TempDir(), "spec")
			l := canonicalLessonFixture(t, specRoot, "kinder-fake")
			indexPath := writeTreeFile(t, specRoot, "lessons/README.md", index)
			err := UpsertLessonIndexRow(specRoot, l)
			if !errors.Is(err, conflictmarker.ErrConflicted) {
				t.Fatalf("err = %v, want conflictmarker.ErrConflicted", err)
			}
			if b, _ := os.ReadFile(indexPath); string(b) != index {
				t.Fatalf("index rewritten despite conflict:\n%s", b)
			}
		})
	}
}

func TestLessonControlViolationsAs(t *testing.T) {
	l := &lesson.Lesson{Status: "Enforced", Date: "2026-10-01", DateLine: 3, Control: "CI blocks it", ControlLine: 7}
	vs := LessonControlViolationsAs(l, "Recorded")
	if len(vs) != 1 || vs[0].Rule != "L-010" || !strings.Contains(vs[0].Message, "Recorded Lesson") {
		t.Fatalf("violations = %+v, want one L-010 for the Recorded projection", vs)
	}
	if l.Status != "Enforced" {
		t.Fatalf("projection mutated the Lesson: %q", l.Status)
	}
	l.Control = "none-yet: the check is manual review"
	if vs := LessonControlViolationsAs(l, "Recorded"); len(vs) != 0 {
		t.Fatalf("a none-yet Control must pass, got %+v", vs)
	}
}
