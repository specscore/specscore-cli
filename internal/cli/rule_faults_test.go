package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/rule"
	"github.com/spf13/cobra"
)

// Every rule verb validates its inputs before touching the tree, so the I/O and
// re-parse failures that follow a successful preflight cannot be produced from
// filesystem state alone. These tests drive them through the package's seams —
// the same approach the ideapromote and entity verbs already use.

var errRuleFault = errors.New("injected rule failure")

// ruleSeams snapshots every seam the rule verbs use and restores it afterwards.
func ruleSeams(t *testing.T, apply func()) {
	t.Helper()
	scaffold, parse := ruleScaffoldDetailFn, ruleParseDetailFn
	writeFile, upsert, remove := ruleWriteFileAtomicFn, ruleUpsertIndexRowFn, ruleRemoveIndexRowFn
	ensure, edits, promote := ruleEnsureIndexFn, ruleApplyFieldEditsFn, ruleSetLessonPromotesToFn
	readIndex := ruleReadIndexFn
	resolveLesson, discoverLessons := lessonResolveLessonFileFn, lessonDiscoverFn
	discoverFeatures, runLint := featureDiscoverFn, lintRunFn
	mkdir, removeAll, readFile, stat, getenv := osMkdirAllCLI, osRemoveAllCLI, osReadFileCLI, osStatCLI, osGetenvCLI
	readDir := osReadDirCLI
	t.Cleanup(func() {
		ruleScaffoldDetailFn, ruleParseDetailFn = scaffold, parse
		ruleWriteFileAtomicFn, ruleUpsertIndexRowFn, ruleRemoveIndexRowFn = writeFile, upsert, remove
		ruleEnsureIndexFn, ruleApplyFieldEditsFn, ruleSetLessonPromotesToFn = ensure, edits, promote
		ruleReadIndexFn = readIndex
		lessonResolveLessonFileFn, lessonDiscoverFn = resolveLesson, discoverLessons
		featureDiscoverFn, lintRunFn = discoverFeatures, runLint
		osMkdirAllCLI, osRemoveAllCLI, osReadFileCLI, osStatCLI, osGetenvCLI = mkdir, removeAll, readFile, stat, getenv
		osReadDirCLI = readDir
	})
	apply()
}

// failingYAML makes the shared YAML encoder fail, so every verb's encode branch
// is exercised through one seam.
func withFailingYAMLEnc(t *testing.T) {
	t.Helper()
	prev := newYAMLEnc
	newYAMLEnc = func(io.Writer) yamlEnc { return failingYAMLEncoder{} }
	t.Cleanup(func() { newYAMLEnc = prev })
}

type failingYAMLEncoder struct{}

func (failingYAMLEncoder) Encode(any) error { return errRuleFault }
func (failingYAMLEncoder) Close() error     { return errRuleFault }

// ----- new / expand -----

func TestRuleNewPropagatesFailures(t *testing.T) {
	cases := []struct {
		name string
		args []string
		set  func()
	}{
		{name: "ancestor index write fails", args: []string{"new", "x"},
			set: func() { ruleEnsureIndexFn = func(string) error { return errRuleFault } }},
		{name: "row write fails", args: []string{"new", "x"},
			set: func() { ruleUpsertIndexRowFn = func(string, rule.Row) error { return errRuleFault } }},
		{name: "detail preflight fails", args: []string{"new", "x", "--detailed"},
			set: func() { osStatCLI = func(string) (os.FileInfo, error) { return nil, errRuleFault } }},
		{name: "scaffold fails", args: []string{"new", "x", "--detailed"},
			set: func() {
				ruleScaffoldDetailFn = func(rule.Options) ([]byte, error) { return nil, errRuleFault }
			}},
		{name: "mkdir fails", args: []string{"new", "x", "--detailed"},
			set: func() { osMkdirAllCLI = func(string, os.FileMode) error { return errRuleFault } }},
		{name: "detail write fails", args: []string{"new", "x", "--detailed"},
			set: func() { ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleFault } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, tc.args...); err == nil {
				t.Fatal("the verb should have failed")
			}
		})
	}
}

// A detail document that already exists is a conflict, not a silent overwrite,
// even when the index has no row for it.
func TestRuleNewRefusesAnOrphanDetailDocument(t *testing.T) {
	root := setupRuleProject(t)
	dir := filepath.Join(rule.RulesDir(root), "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Rule: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runRule(t, root, "new", "x", "--detailed")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (err=%v)", got, exitcode.Conflict, err)
	}
}

func TestRuleExpandPropagatesFailures(t *testing.T) {
	cases := []struct {
		name string
		set  func()
	}{
		{name: "preflight fails", set: func() {
			osStatCLI = func(string) (os.FileInfo, error) { return nil, errRuleFault }
		}},
		{name: "detail write fails", set: func() {
			ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleFault }
		}},
		{name: "row relink fails", set: func() {
			ruleUpsertIndexRowFn = func(string, rule.Row) error { return errRuleFault }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			if _, _, err := runRule(t, root, "new", "x", "--statement", "s"); err != nil {
				t.Fatal(err)
			}
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, "expand", "x"); err == nil {
				t.Fatal("expand should have failed")
			}
		})
	}
}

// A row whose own fields are invalid cannot seed a document.
func TestRuleExpandRejectsAnInvalidRow(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--statement", "s"); err != nil {
		t.Fatal(err)
	}
	index := strings.Replace(readRuleIndex(t, root), "| fleet |", "| team:platform |", 1)
	if err := os.WriteFile(rule.IndexPath(rule.RulesDir(root)), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "expand", "x"); exitCodeOf(err) != exitcode.InvalidArgs {
		t.Fatalf("expand should reject a row with an invalid scope, got %v", err)
	}
}

func TestEnsureRuleAncestorIndexesPropagatesFailure(t *testing.T) {
	// A `spec` that is a file makes the ancestor stat fail for a reason other
	// than absence, which must surface rather than be read as "not there yet".
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "spec"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureRuleAncestorIndexes(root); err == nil {
		t.Fatal("ensureRuleAncestorIndexes should propagate a write failure")
	}
}

// ----- update -----

func TestRuleUpdatePropagatesFailures(t *testing.T) {
	cases := []struct {
		name string
		args []string
		set  func()
	}{
		{name: "row write fails", args: []string{"update", "x", "--status", "Active"},
			set: func() { ruleUpsertIndexRowFn = func(string, rule.Row) error { return errRuleFault } }},
		{name: "document read fails", args: []string{"update", "x", "--status", "Active"},
			set: func() { osReadFileCLI = func(string) ([]byte, error) { return nil, errRuleFault } }},
		{name: "document edit fails", args: []string{"update", "x", "--status", "Active"},
			set: func() {
				ruleApplyFieldEditsFn = func([]byte, []rule.FieldEdit) ([]byte, error) { return nil, errRuleFault }
			}},
		{name: "document write fails", args: []string{"update", "x", "--status", "Active"},
			set: func() { ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleFault } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
				t.Fatal(err)
			}
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, tc.args...); err == nil {
				t.Fatal("update should have failed")
			}
		})
	}
}

// An explicitly empty --scope list has nothing to write.
func TestRuleUpdateRejectsAnEmptyScopeList(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatal(err)
	}
	cmd := ruleUpdateCommand()
	if err := cmd.Flags().Set("scope", ""); err != nil {
		t.Fatal(err)
	}
	row := rule.NewRow("x", false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)
	if _, err := applyRuleRowEdits(cmd, row); err == nil {
		t.Fatal("an empty --scope entry must be rejected")
	}
}

// An empty --title is a no-op rather than a heading that reads `# Rule:`.
func TestRuleUpdateEmptyTitleIsIgnored(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--title", "Original", "--detailed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "update", "x", "--title", "   "); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(readRuleDetail(t, root, "x"), "# Rule: Original") {
		t.Fatalf("an empty title must not blank the heading:\n%s", readRuleDetail(t, root, "x"))
	}
}

func TestSentinelOrValue(t *testing.T) {
	if got := sentinelOrValue("  "); got != rule.Sentinel {
		t.Fatalf("sentinelOrValue(blank) = %q", got)
	}
	if got := sentinelOrValue(" x "); got != "x" {
		t.Fatalf("sentinelOrValue = %q", got)
	}
}

// ----- promote -----

func TestRulePromotePropagatesFailures(t *testing.T) {
	cases := []struct {
		name string
		set  func()
	}{
		{name: "lesson parse fails", set: func() {
			lessonResolveLessonFileFn = func(string, string) (string, error) { return "not-a-file", nil }
		}},
		{name: "ancestor index write fails", set: func() {
			ruleEnsureIndexFn = func(string) error { return errRuleFault }
		}},
		{name: "detail write fails", set: func() {
			ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleFault }
		}},
		{name: "row write fails", set: func() {
			ruleUpsertIndexRowFn = func(string, rule.Row) error { return errRuleFault }
		}},
		{name: "lesson pointer write fails", set: func() {
			ruleSetLessonPromotesToFn = func(string, string) error { return errRuleFault }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			writeCanonicalLesson(t, root, "kinder-fake", "c")
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "x"); err == nil {
				t.Fatal("promote should have failed")
			}
		})
	}
}

// With no owner on the Lesson and no --owner, the environment supplies it.
func TestRulePromoteFallsBackToTheEnvironmentOwner(t *testing.T) {
	root := setupRuleProject(t)
	dir := filepath.Join(root, "spec", "lessons", "l")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n\n## Enforcement\n\n**Control:** Do the thing.\n\n## Open Questions\n\nNone.\n"
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		osGetenvCLI = func(string) string { return "env-owner" }
	})
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "l", "x"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !strings.Contains(readRuleDetail(t, root, "x"), "**Owner:** env-owner") {
		t.Fatalf("owner not taken from the environment:\n%s", readRuleDetail(t, root, "x"))
	}
}

func TestFirstNonSentinelFallsThrough(t *testing.T) {
	if got := firstNonSentinel(rule.Sentinel, "", "-"); got != "" {
		t.Fatalf("firstNonSentinel = %q, want empty", got)
	}
}

func TestRuleSkillsPathReadsTheOverride(t *testing.T) {
	root := t.TempDir()
	if got := ruleSkillsPath(root); got != "" {
		t.Fatalf("no config should yield the default, got %q", got)
	}
	base := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nversion: 1\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ruleSkillsPath(root); got != "" {
		t.Fatalf("a config without a rules block should yield the default, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"),
		[]byte(base+"rules:\n  skills_path: tools/skills\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ruleSkillsPath(root); got != "tools/skills" {
		t.Fatalf("ruleSkillsPath = %q", got)
	}
}

// ----- delete -----

func TestRuleDeletePropagatesFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		args  []string
		set   func()
	}{
		{name: "directory removal fails", args: []string{"delete", "x"},
			set: func() { osRemoveAllCLI = func(string) error { return errRuleFault } }},
		{name: "row removal fails", args: []string{"delete", "x"},
			set: func() { ruleRemoveIndexRowFn = func(string, string) error { return errRuleFault } }},
		{name: "successor source write fails", args: []string{"delete", "x", "--supersede-with", "successor"},
			setup: func(t *testing.T, root string) {
				writeCanonicalLesson(t, root, "l", "c")
				if _, _, err := runRule(t, root, "promote", "--from-lesson", "l", "x", "--force"); err != nil {
					t.Fatal(err)
				}
			},
			set: func() { ruleUpsertIndexRowFn = func(string, rule.Row) error { return errRuleFault } }},
		{name: "lesson resolution fails", args: []string{"delete", "x", "--supersede-with", "successor"},
			setup: func(t *testing.T, root string) {
				writeCanonicalLesson(t, root, "l", "c")
				if _, _, err := runRule(t, root, "promote", "--from-lesson", "l", "x", "--force"); err != nil {
					t.Fatal(err)
				}
			},
			set: func() {
				lessonResolveLessonFileFn = func(string, string) (string, error) { return "", errRuleFault }
			}},
		{name: "successor mirror write fails", args: []string{"delete", "x", "--supersede-with", "successor"},
			setup: func(t *testing.T, root string) {
				writeCanonicalLesson(t, root, "l", "c")
				if _, _, err := runRule(t, root, "promote", "--from-lesson", "l", "x", "--force"); err != nil {
					t.Fatal(err)
				}
			},
			set: func() {
				ruleApplyFieldEditsFn = func([]byte, []rule.FieldEdit) ([]byte, error) { return nil, errRuleFault }
			}},
		{name: "lesson repoint fails", args: []string{"delete", "x", "--supersede-with", "successor"},
			setup: func(t *testing.T, root string) {
				writeCanonicalLesson(t, root, "l", "c")
				if _, _, err := runRule(t, root, "promote", "--from-lesson", "l", "x", "--force"); err != nil {
					t.Fatal(err)
				}
			},
			set: func() { ruleSetLessonPromotesToFn = func(string, string) error { return errRuleFault } }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
				t.Fatal(err)
			}
			if _, _, err := runRule(t, root, "new", "successor", "--detailed"); err != nil {
				t.Fatal(err)
			}
			if tc.setup != nil {
				tc.setup(t, root)
			}
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, tc.args...); err == nil {
				t.Fatal("delete should have failed")
			}
		})
	}
}

func TestRuleDeleteRelationRepointFailures(t *testing.T) {
	cases := []struct {
		name string
		set  func()
	}{
		{name: "related rule parse fails", set: func() {
			ruleParseDetailFn = func(string) (*rule.Detail, error) { return nil, errRuleFault }
		}},
		{name: "related rule edit fails", set: func() {
			ruleApplyFieldEditsFn = func([]byte, []rule.FieldEdit) ([]byte, error) { return nil, errRuleFault }
		}},
		{name: "related rule write fails", set: func() {
			ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleFault }
		}},
		{name: "related rule read fails", set: func() {
			osReadFileCLI = func(string) ([]byte, error) { return nil, errRuleFault }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupRuleProject(t)
			for _, slug := range []string{"old", "successor", "newer"} {
				if _, _, err := runRule(t, root, "new", slug, "--detailed"); err != nil {
					t.Fatal(err)
				}
			}
			if _, _, err := runRule(t, root, "update", "newer", "--supersedes", "old"); err != nil {
				t.Fatal(err)
			}
			ruleSeams(t, tc.set)
			if _, _, err := runRule(t, root, "delete", "old", "--supersede-with", "successor"); err == nil {
				t.Fatal("delete should have failed")
			}
		})
	}
}

// Both halves of a supersession relation are repointed, not just Supersedes.
func TestRuleDeleteRepointsSupersededByToo(t *testing.T) {
	root := setupRuleProject(t)
	for _, slug := range []string{"old", "successor", "older-still"} {
		if _, _, err := runRule(t, root, "new", slug, "--detailed"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runRule(t, root, "update", "older-still", "--superseded-by", "old"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "delete", "old", "--supersede-with", "successor"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !strings.Contains(readRuleDetail(t, root, "older-still"), "**Superseded By:** successor") {
		t.Fatalf("Superseded By not repointed:\n%s", readRuleDetail(t, root, "older-still"))
	}
}

// A feature citation blocks a bare delete, exactly like a lesson or a skill.
func TestRuleDeleteRefusesWhileAFeatureCites(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatal(err)
	}
	featureDir := filepath.Join(root, "spec", "features", "some-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"),
		[]byte("# Feature: Some\n\nBound by rule:x.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runRule(t, root, "delete", "x")
	if got := exitCodeOf(err); got != exitcode.InvalidState || !strings.Contains(err.Error(), "feature:some-feature") {
		t.Fatalf("exit = %d err = %v", got, err)
	}
}

// Discovery failures degrade to "no references" rather than aborting a delete.
func TestRuleDeleteToleratesDiscoveryFailures(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		lessonDiscoverFn = func(string) ([]*lesson.Lesson, error) { return nil, errRuleFault }
		featureDiscoverFn = func(string) ([]feature.Feature, error) { return nil, errRuleFault }
	})
	if _, _, err := runRule(t, root, "delete", "x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

// A rules directory that cannot be read yields no related rules rather than a
// panic, and a skills directory that is a file yields no skills.
func TestRuleReferenceScansToleratesUnreadableTrees(t *testing.T) {
	if got := rulesReferencing(filepath.Join(t.TempDir(), "missing"), "x"); got != nil {
		t.Fatalf("rulesReferencing = %v", got)
	}
	// A README.md that is a directory makes detail discovery fail; the scan
	// degrades to "no related rules" rather than aborting the delete.
	broken := t.TempDir()
	if err := os.MkdirAll(filepath.Join(rule.RulesDir(broken), "y", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := rulesReferencing(rule.RulesDir(broken), "x"); got != nil {
		t.Fatalf("rulesReferencing = %v", got)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rule.DefaultSkillsPath), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := skillsReferencing(root, "x"); got != nil {
		t.Fatalf("skillsReferencing = %v", got)
	}
}

func TestApplyRuleDetailEditsNoEditsIsANoOp(t *testing.T) {
	if err := applyRuleDetailEdits("does-not-exist", nil); err != nil {
		t.Fatalf("applyRuleDetailEdits with no edits = %v", err)
	}
}

func TestPrefixEachAndContainsString(t *testing.T) {
	got := prefixEach("skill:", []string{"a", "b"})
	if len(got) != 2 || got[0] != "skill:a" {
		t.Fatalf("prefixEach = %v", got)
	}
	if !containsString([]string{"a"}, "a") || containsString([]string{"a"}, "b") {
		t.Fatal("containsString is wrong")
	}
}

// ----- show -----

func TestRuleShowPropagatesDetailParseFailure(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		ruleParseDetailFn = func(string) (*rule.Detail, error) { return nil, errRuleFault }
	})
	if _, _, err := runRule(t, root, "show", "x"); err == nil {
		t.Fatal("show should propagate a parse failure")
	}
}

func TestRuleShowToleratesDiscoveryFailures(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		lessonDiscoverFn = func(string) ([]*lesson.Lesson, error) { return nil, errRuleFault }
		featureDiscoverFn = func(string) ([]feature.Feature, error) { return nil, errRuleFault }
	})
	if _, _, err := runRule(t, root, "show", "x"); err != nil {
		t.Fatalf("show: %v", err)
	}
}

// A Feature README that cannot be read is skipped, not fatal: one unreadable
// file must not blind the citation report.
func TestFeaturesCitingRuleSkipsUnreadableReadme(t *testing.T) {
	root := setupRuleProject(t)
	featureDir := filepath.Join(root, "spec", "features", "some-feature")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte("# Feature: Some\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		osReadFileCLI = func(string) ([]byte, error) { return nil, errRuleFault }
	})
	if got := featuresCitingRule(filepath.Join(root, "spec"), "x"); len(got) != 0 {
		t.Fatalf("featuresCitingRule = %v", got)
	}
}

// A malformed source reference is unresolvable by definition.
func TestUnresolvedRuleSourcesReportsMalformedReferences(t *testing.T) {
	got := unresolvedRuleSources(t.TempDir(), []string{"bogus"})
	if len(got) != 1 || got[0] != "bogus" {
		t.Fatalf("unresolvedRuleSources = %v", got)
	}
}

func TestWriteRuleShowTextPrintsUnresolvedLinks(t *testing.T) {
	var out bytes.Buffer
	err := writeRuleShowText(&out, ruleShowDoc{Slug: "x", UnresolvedLinks: []string{"lesson:ghost"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Unresolved:") {
		t.Fatalf("text output = %q", out.String())
	}
}

// ----- list / lint -----

func TestRuleListPropagatesIndexReadFailure(t *testing.T) {
	root := setupRuleProject(t)
	if _, _, err := runRule(t, root, "new", "x"); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		ruleReadIndexFn = func(string) (rule.IndexReport, error) { return rule.IndexReport{}, errRuleFault }
	})
	if _, _, err := runRule(t, root, "list"); err == nil {
		t.Fatal("list should propagate an index read failure")
	}
}

func TestRuleLintPropagatesRunFailure(t *testing.T) {
	root := setupRuleProject(t)
	ruleSeams(t, func() {
		lintRunFn = func(lint.Options) ([]lint.Violation, error) { return nil, errRuleFault }
	})
	if _, _, err := runRule(t, root, "lint"); err == nil {
		t.Fatal("lint should propagate a run failure")
	}
}

// ----- encoder failures -----

// Every verb that can emit YAML must surface an encoder failure rather than
// exiting 0 with truncated output.
func TestRuleVerbsPropagateYAMLEncodeFailure(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "c")
	if _, _, err := runRule(t, root, "new", "x", "--detailed"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runRule(t, root, "new", "doomed"); err != nil {
		t.Fatal(err)
	}
	cases := [][]string{
		{"new", "fresh", "--format", "yaml"},
		{"expand", "doomed", "--format", "yaml"},
		{"list", "--format", "yaml"},
		{"show", "x", "--format", "yaml"},
		{"update", "x", "--status", "Active", "--format", "yaml"},
		{"promote", "--from-lesson", "kinder-fake", "promoted", "--format", "yaml"},
		{"lint", "--format", "yaml"},
		{"delete", "x", "--format", "yaml"},
	}
	for _, args := range cases {
		t.Run(args[0], func(t *testing.T) {
			withFailingYAMLEnc(t)
			if _, _, err := runRule(t, root, args...); err == nil {
				t.Fatalf("%v should surface the encoder failure", args)
			}
		})
	}
}

func TestRuleLintYAMLCloseFailure(t *testing.T) {
	root := setupRuleProject(t)
	prev := newYAMLEnc
	newYAMLEnc = func(io.Writer) yamlEnc { return closeFailingYAMLEncoder{} }
	t.Cleanup(func() { newYAMLEnc = prev })
	if _, _, err := runRule(t, root, "lint", "--format", "yaml"); err == nil {
		t.Fatal("a Close failure must surface")
	}
}

type closeFailingYAMLEncoder struct{}

func (closeFailingYAMLEncoder) Encode(any) error { return nil }
func (closeFailingYAMLEncoder) Close() error     { return errRuleFault }

func TestRuleLintJSONEncodeFailure(t *testing.T) {
	root := setupRuleProject(t)
	prev := newJSONEnc
	newJSONEnc = func(io.Writer) jsonEnc { return failingJSONEncoder{} }
	t.Cleanup(func() { newJSONEnc = prev })
	if _, _, err := runRule(t, root, "lint", "--format", "json"); err == nil {
		t.Fatal("a JSON encode failure must surface")
	}
}

type failingJSONEncoder struct{}

func (failingJSONEncoder) Encode(any) error { return errRuleFault }

func TestWriteRuleResultYAMLFailure(t *testing.T) {
	withFailingYAMLEnc(t)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	if err := writeRuleResult(cmd, "yaml", ruleWriteResult{Slug: "x"}, t.TempDir()); err == nil {
		t.Fatal("writeRuleResult must surface an encoder failure")
	}
}

// ----- fix-round branches -----

// A project with no index yet has nothing to preflight, and an index that
// cannot be read for any other reason is surfaced rather than waved through.
func TestPreflightRuleIndexHandlesMissingAndUnreadable(t *testing.T) {
	if err := preflightRuleIndex(filepath.Join(t.TempDir(), "rules")); err != nil {
		t.Fatalf("a missing index must not block a verb: %v", err)
	}
	ruleSeams(t, func() {
		ruleReadIndexFn = func(string) (rule.IndexReport, error) { return rule.IndexReport{}, errRuleFault }
	})
	if err := preflightRuleIndex(filepath.Join(t.TempDir(), "rules")); err == nil {
		t.Fatal("an unreadable index must surface")
	}
}

// Promote refuses the same two writes `new` does.
func TestRulePromoteRefusesUnresolvableSourceAndInlineSuperseded(t *testing.T) {
	root := setupRuleProject(t)
	writeCanonicalLesson(t, root, "kinder-fake", "Assert against the real adapter.")
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "x",
		"--source", "decision:9999"); exitCodeOf(err) != exitcode.InvalidArgs {
		t.Fatal("an unresolvable --source must be refused")
	}
	if _, _, err := runRule(t, root, "promote", "--from-lesson", "kinder-fake", "x",
		"--inline", "--status", "Superseded"); exitCodeOf(err) != exitcode.InvalidState {
		t.Fatal("an inline promotion at Superseded must be refused")
	}
}

// Repointing a rule that is superseded BY the deleted one edits that half too,
// and a failure there surfaces.
func TestRuleDeleteRepointFailureOnSupersededBy(t *testing.T) {
	root := setupRuleProject(t)
	for _, slug := range []string{"old", "successor", "older-still"} {
		if _, _, err := runRule(t, root, "new", slug, "--detailed"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := runRule(t, root, "update", "older-still", "--superseded-by", "old"); err != nil {
		t.Fatal(err)
	}
	ruleSeams(t, func() {
		ruleApplyFieldEditsFn = func(content []byte, edits []rule.FieldEdit) ([]byte, error) {
			for _, e := range edits {
				if e.Name == "Superseded By" {
					return nil, errRuleFault
				}
			}
			return rule.ApplyFieldEdits(content, edits)
		}
	})
	if _, _, err := runRule(t, root, "delete", "old", "--supersede-with", "successor"); err == nil {
		t.Fatal("delete should have failed")
	}
}

// An idea recorded in the archived directory resolves as a source, and a
// decision reference skips directories and non-markdown files.
func TestRuleSourceResolutionEdgeCases(t *testing.T) {
	root := setupRuleProject(t)
	specSub := filepath.Join(root, "spec")
	archived := filepath.Join(specSub, "ideas", "archived")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archived, "retired.md"), []byte("# Idea: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ruleIdeaSourceExists(specSub, "retired") {
		t.Fatal("an archived Idea must resolve")
	}
	decisions := filepath.Join(specSub, "decisions")
	if err := os.MkdirAll(filepath.Join(decisions, "0012-a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decisions, "0012-notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ruleDecisionSourceExists(specSub, "0012") {
		t.Fatal("only a .md file may resolve a decision reference")
	}
	if err := os.WriteFile(filepath.Join(decisions, "0012-real.md"), []byte("# Decision: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ruleDecisionSourceExists(specSub, "0012") {
		t.Fatal("a real decision must resolve")
	}
	// An unreadable decisions directory yields no match rather than a panic.
	ruleSeams(t, func() {
		osReadDirCLI = func(string) ([]os.DirEntry, error) { return nil, errRuleFault }
	})
	if ruleDecisionSourceExists(specSub, "0012") {
		t.Fatal("an unreadable decisions tree must not resolve")
	}
}

// A path outside the project keeps its absolute form rather than becoming a
// `../..` ladder that means nothing to a consumer.
func TestRepoRelativeFallsBackForOutsidePaths(t *testing.T) {
	if got := repoRelative("/proj", "/elsewhere/x.md"); got != "/elsewhere/x.md" {
		t.Fatalf("repoRelative = %q", got)
	}
	if got := repoRelative("/proj", ""); got != "" {
		t.Fatalf("repoRelative(empty) = %q", got)
	}
	if got := repoRelative("/proj", "/proj/spec/x.md"); got != "spec/x.md" {
		t.Fatalf("repoRelative = %q", got)
	}
}

// The create guard has the same two I/O outcomes the preflight does.
func TestRefuseExistingRowHandlesMissingAndUnreadableIndex(t *testing.T) {
	if err := refuseExistingRow(filepath.Join(t.TempDir(), "rules"), "x", false); err != nil {
		t.Fatalf("a missing index must not block a create: %v", err)
	}
	ruleSeams(t, func() {
		ruleReadIndexFn = func(string) (rule.IndexReport, error) { return rule.IndexReport{}, errRuleFault }
	})
	if err := refuseExistingRow(filepath.Join(t.TempDir(), "rules"), "x", false); err == nil {
		t.Fatal("an unreadable index must surface")
	}
	// --force short-circuits before any read.
	if err := refuseExistingRow(filepath.Join(t.TempDir(), "rules"), "x", true); err != nil {
		t.Fatalf("--force must not read the index: %v", err)
	}
}
