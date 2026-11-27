package lint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestConflictMarkerLines(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []conflictMarker
	}{
		{"two-way", "a\n<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> topic\nb\n",
			[]conflictMarker{{2, "<<<<<<<"}, {4, "======="}, {6, ">>>>>>>"}}},
		{"diff3", "<<<<<<< ours\nx\n||||||| base\nz\n=======\ny\n>>>>>>> theirs\n",
			[]conflictMarker{{1, "<<<<<<<"}, {3, "|||||||"}, {5, "======="}, {7, ">>>>>>>"}}},
		{"crlf", "<<<<<<< HEAD\r\nx\r\n=======\r\ny\r\n>>>>>>> t\r\n",
			[]conflictMarker{{1, "<<<<<<<"}, {3, "======="}, {5, ">>>>>>>"}}},
		{"setext-heading-alone", "Title\n=======\n\nbody\n", nil},
		{"setext-after-block-closed", "<<<<<<< a\n=======\n>>>>>>> b\nTitle\n=======\n",
			[]conflictMarker{{1, "<<<<<<<"}, {2, "======="}, {3, ">>>>>>>"}}},
		{"orphan-theirs-marker", "x\n>>>>>>> topic\n", []conflictMarker{{2, ">>>>>>>"}}},
		{"unlabelled-or-longer-runs", "<<<<<<<\n<<<<<<<< x\n>>>>>>>>> y\n|||||||\n", nil},
		{"indented-not-a-marker", " <<<<<<< HEAD\n", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conflictMarkerLines([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("conflictMarkerLines = %v, want %v", got, tc.want)
			}
		})
	}
}

// AC: merge-conflict-markers-in-artifact-flagged — a conflict inside an
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

// AC: merge-conflict-markers-in-generated-index-flagged — both halves of a
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

// AC: merge-conflict-markers-setext-heading-not-flagged — a bare `=======`
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

// AC: merge-conflict-markers-diff3-flagged — the diff3/zdiff3 base marker is
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

// AC: merge-conflict-markers-fix-refused-byte-identical — with markers
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
// hidden directories, benchmark fixtures, binary files and symlinks are not.
func TestMergeConflictMarkers_Scope(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	marked := "<<<<<<< HEAD\nx\n=======\ny\n>>>>>>> t\n"
	writeTreeFile(t, project, "tasks/README.md", marked)
	writeTreeFile(t, specRoot, "plans/README.md", marked)
	writeTreeFile(t, specRoot, ".hidden/README.md", marked)
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

	if got, want := files(newMergeConflictMarkerChecker(project, "", false)), []string{"plans/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("same-repo plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, filepath.Join(specRoot, "plans"), false)), []string{"plans/README.md", "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("plans routed to spec/plans: files = %v, want %v", got, want)
	}
	extRel, _ := filepath.Rel(specRoot, filepath.Join(external, "README.md"))
	if got, want := files(newMergeConflictMarkerChecker(project, external, false)), []string{filepath.ToSlash(extRel), "../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("external plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, filepath.Join(project, "missing"), false)), []string{"../tasks/README.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("missing external plans: files = %v, want %v", got, want)
	}
	if got, want := files(newMergeConflictMarkerChecker(project, "", true)), []string{"../tasks/README.md"}; !reflect.DeepEqual(got, want) {
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

func swapConflictMarkerSeams(t *testing.T, walk func(string, fs.WalkDirFunc) error, read func(string) ([]byte, error), rel func(string, string) (string, error)) {
	t.Helper()
	prevWalk, prevRead, prevRel := conflictMarkerWalkDir, conflictMarkerReadFile, conflictMarkerRel
	t.Cleanup(func() { conflictMarkerWalkDir, conflictMarkerReadFile, conflictMarkerRel = prevWalk, prevRead, prevRel })
	if walk != nil {
		conflictMarkerWalkDir = walk
	}
	if read != nil {
		conflictMarkerReadFile = read
	}
	if rel != nil {
		conflictMarkerRel = rel
	}
}

func TestMergeConflictMarkers_ErrorPathsFailClosed(t *testing.T) {
	specRoot := t.TempDir()
	writeTreeFile(t, specRoot, "README.md", "<<<<<<< HEAD\n")

	t.Run("walk-error", func(t *testing.T) {
		swapConflictMarkerSeams(t, func(root string, fn fs.WalkDirFunc) error {
			return fn(root, nil, errors.New("unreadable dir"))
		}, nil, nil)
		if _, err := newMergeConflictMarkerChecker("", "", false).check(specRoot); err == nil || !strings.Contains(err.Error(), "unreadable dir") {
			t.Fatalf("err = %v, want walk error", err)
		}
		if _, err := LintWithResult(Options{SpecRoot: specRoot, Fix: true}); err == nil || !strings.Contains(err.Error(), mergeConflictMarkersRule) {
			t.Fatalf("LintWithResult err = %v, want the gate error naming the rule", err)
		}
	})
	t.Run("read-error", func(t *testing.T) {
		swapConflictMarkerSeams(t, nil, func(string) ([]byte, error) { return nil, errors.New("unreadable file") }, nil)
		if _, err := newMergeConflictMarkerChecker("", "", false).check(specRoot); err == nil || !strings.Contains(err.Error(), "unreadable file") {
			t.Fatalf("err = %v, want read error", err)
		}
	})
	t.Run("rel-error-falls-back-to-path", func(t *testing.T) {
		swapConflictMarkerSeams(t, nil, nil, func(string, string) (string, error) { return "", errors.New("no rel") })
		vs, err := newMergeConflictMarkerChecker("", "", false).check(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(vs) != 1 || vs[0].File != filepath.ToSlash(filepath.Join(specRoot, "README.md")) {
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
