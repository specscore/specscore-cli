package cli

// Cross-kind acceptance tests for `--dry-run` on `<kind> change-status`
// (closes LESSONS-LEARNED L151: there was previously no way to learn what a
// change-status call would touch without actually running it).
//
// Every case in the table below proves the same three properties, generic
// across kind:
//
//  1. The working tree is byte-for-byte unchanged after a --dry-run call
//     (snapshotDir before/after, full diff — not just "the target file").
//  2. The exact set of spec-tree files --dry-run reports (parsed from its own
//     stdout) matches the exact spec-tree set a SUBSEQUENT real
//     (non---dry-run) call on the SAME fixture actually changes. Durable event
//     ledger entries live under .specscore/ and carry fresh event UUIDs, so
//     they are deliberately outside the artifact-preview contract.
//  3. An illegal transition under --dry-run exits with the SAME non-zero
//     code the real command would use, writing nothing.
//
// Covering seven kinds (feature, idea, plan, lesson, decision, sidekick,
// issue) in one table also satisfies "the flag works on more than one
// artifact kind" directly, rather than as an accidental byproduct of
// per-kind test files.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// snapshotDir walks root and returns every regular file's project-relative
// (forward-slash) path mapped to its content, for byte-for-byte before/after
// comparison.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDir(%s): %v", root, err)
	}
	return out
}

// diffSnapshotPaths returns every path that differs (added, removed, or
// modified) between before and after, sorted.
func diffSnapshotPaths(before, after map[string]string) []string {
	seen := map[string]bool{}
	for path, content := range before {
		if after[path] != content {
			seen[path] = true
		}
	}
	for path := range after {
		if _, existed := before[path]; !existed {
			seen[path] = true
		}
	}
	out := make([]string, 0, len(seen))
	for path := range seen {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func specTreePaths(paths []string) []string {
	var out []string
	for _, path := range paths {
		if strings.HasPrefix(path, "spec/") {
			out = append(out, path)
		}
	}
	return out
}

// parseDryRunPaths extracts the "  <M|A|D> <path>" lines dryrun.PrintReport
// emits after its first line, stripping the single-letter kind prefix so the
// result is directly comparable to diffSnapshotPaths' output.
func parseDryRunPaths(stdout string) []string {
	var out []string
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		parts := strings.SplitN(trimmed, " ", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "M", "A", "D":
			out = append(out, parts[1])
		}
	}
	sort.Strings(out)
	return out
}

// changeStatusDryRunCase describes one kind's dry-run acceptance scenario.
type changeStatusDryRunCase struct {
	name string
	// stage bootstraps a fresh fixture and returns its project root
	// (cwd already set to it).
	stage func(t *testing.T) string
	// run invokes the kind's cobra command tree in-process.
	run func(t *testing.T, args ...string) (stdout, stderr string, err error)
	// legalArgs is the full change-status argument list (after
	// "change-status") for a LEGAL transition, e.g.
	// []string{"auth", "--to=in review"}.
	legalArgs []string
	// illegalArgs is the full change-status argument list for a transition
	// the kind's matrix rejects, exercised against the SAME fixture
	// (before the legal transition runs).
	illegalArgs []string
	illegalExit int
}

func changeStatusDryRunCases(t *testing.T) []changeStatusDryRunCase {
	return []changeStatusDryRunCase{
		{
			name:        "feature",
			stage:       func(t *testing.T) string { return setupFeatureSpec(t, "Stable") },
			run:         runFeature,
			legalArgs:   []string{"auth", "--to=Amending"},
			illegalArgs: []string{"auth", "--to=Draft"},
			illegalExit: 2, // Draft is a recognized status but not a legal --to target.
		},
		{
			name: "idea",
			stage: func(t *testing.T) string {
				root := setupSpecRoot(t)
				withCwd(t, root)
				if _, _, err := runIdea(t, "new", "foo", "--owner", "tester"); err != nil {
					t.Fatalf("idea new: %v", err)
				}
				// setupSpecRoot creates spec/features/ but not its index
				// README — mirrors stagePlan/stageLesson/stageQueuedSeed's
				// same hand-written minimal Features index.
				featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
				if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
					t.Fatalf("write features README: %v", err)
				}
				migrateTree(t, root)
				return root
			},
			run:         runIdea,
			legalArgs:   []string{"foo", "--to=approved"},
			illegalArgs: []string{"foo", "--to=implemented"},
			illegalExit: 4,
		},
		{
			name:        "plan",
			stage:       func(t *testing.T) string { return stagePlan(t, "auth", "Draft") },
			run:         runPlan,
			legalArgs:   []string{"auth", "--to=in review"},
			illegalArgs: []string{"auth", "--to=implemented"},
			illegalExit: 2, // Implemented is lint-derived, not a human --to target.
		},
		{
			name:        "lesson",
			stage:       func(t *testing.T) string { return stageLesson(t, "kinder-fake", "Recorded") },
			run:         runLesson,
			legalArgs:   []string{"kinder-fake", "--to=stated"},
			illegalArgs: []string{"kinder-fake", "--to=enforced-typo"},
			illegalExit: 2,
		},
		{
			name: "decision",
			stage: func(t *testing.T) string {
				root, slug := stageDecisionCLI(t, "auth", "Draft")
				decisionDryRunSlug = slug
				return root
			},
			run:         runDecision,
			legalArgs:   nil, // filled in below once the slug is known.
			illegalArgs: nil,
			illegalExit: 2,
		},
		{
			name: "sidekick",
			stage: func(t *testing.T) string {
				return stageQueuedSeed(t, "foo")
			},
			run:         runSidekick,
			legalArgs:   []string{"foo", "--to=implemented"},
			illegalArgs: []string{"foo", "--to=banana"},
			illegalExit: 2,
		},
		{
			name: "issue",
			stage: func(t *testing.T) string {
				root := setupIssueSpecRoot(t)
				withCwd(t, root)
				// Use the production scaffold (not the bare hand-written
				// writeIssueFixture) so the fixture is lint-clean under the
				// REAL lint --fix pass this test exercises, matching how
				// stagePlan/stageLesson/stageQueuedSeed scaffold via their
				// kind's own `new` verb rather than hand-writing a body.
				if _, _, err := runIssue(t, "new", "timeout-bug", "--severity=high", "--captured-by", "tester"); err != nil {
					t.Fatalf("issue new: %v", err)
				}
				return root
			},
			run:         runIssue,
			legalArgs:   []string{"timeout-bug", "--to=investigating", "--severity=high"},
			illegalArgs: []string{"timeout-bug", "--to=bogus-status"},
			illegalExit: 2,
		},
	}
}

// decisionDryRunSlug carries the full (sequence-numbered) decision slug from
// a case's stage() to its legalArgs/illegalArgs, since the slug isn't known
// until stageDecisionCLI runs. Cleared/rebuilt per subtest.
var decisionDryRunSlug string

func TestChangeStatusDryRun_LeavesWorkingTreeUnchanged(t *testing.T) {
	for _, tc := range changeStatusDryRunCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.stage(t)
			args := tc.legalArgs
			if tc.name == "decision" {
				args = []string{decisionDryRunSlug, "--to=in review"}
			}

			before := snapshotDir(t, root)
			stdout, stderr, err := tc.run(t, append([]string{"change-status"}, append(append([]string{}, args...), "--dry-run")...)...)
			if err != nil {
				t.Fatalf("%s: dry-run failed: %v\nstdout=%s\nstderr=%s", tc.name, err, stdout, stderr)
			}
			after := snapshotDir(t, root)

			if diff := diffSnapshotPaths(before, after); len(diff) != 0 {
				t.Fatalf("%s: working tree changed under --dry-run: %v", tc.name, diff)
			}
			if !strings.Contains(stdout, "dry-run") {
				t.Errorf("%s: stdout does not mention dry-run: %q", tc.name, stdout)
			}
		})
	}
}

func TestChangeStatusDryRun_FileListMatchesRealRun(t *testing.T) {
	for _, tc := range changeStatusDryRunCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			root := tc.stage(t)
			args := tc.legalArgs
			if tc.name == "decision" {
				args = []string{decisionDryRunSlug, "--to=in review"}
			}

			before := snapshotDir(t, root)

			dryArgs := append([]string{"change-status"}, append(append([]string{}, args...), "--dry-run")...)
			dryOut, dryErr, err := tc.run(t, dryArgs...)
			if err != nil {
				t.Fatalf("%s: dry-run failed: %v\nstderr=%s", tc.name, err, dryErr)
			}
			reported := parseDryRunPaths(dryOut)
			if len(reported) == 0 {
				t.Fatalf("%s: dry-run reported zero changed files (stdout=%q)", tc.name, dryOut)
			}

			realArgs := append([]string{"change-status"}, args...)
			if _, realErr, err := tc.run(t, realArgs...); err != nil {
				t.Fatalf("%s: real run failed: %v\nstderr=%s", tc.name, err, realErr)
			}
			after := snapshotDir(t, root)
			actual := specTreePaths(diffSnapshotPaths(before, after))

			if strings.Join(reported, ",") != strings.Join(actual, ",") {
				t.Errorf("%s: dry-run reported %v, real run actually changed %v", tc.name, reported, actual)
			}
		})
	}
}

func TestChangeStatusDryRun_IllegalTransitionExitsNonZeroWithoutWriting(t *testing.T) {
	for _, tc := range changeStatusDryRunCases(t) {
		if tc.illegalArgs == nil && tc.name != "decision" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			root := tc.stage(t)
			args := tc.illegalArgs
			if tc.name == "decision" {
				args = []string{decisionDryRunSlug, "--to=implemented"} // not a Decision status at all
			}

			before := snapshotDir(t, root)
			dryArgs := append([]string{"change-status"}, append(append([]string{}, args...), "--dry-run")...)
			_, _, dryErr := tc.run(t, dryArgs...)
			if dryErr == nil {
				t.Fatalf("%s: expected --dry-run to fail on an illegal transition", tc.name)
			}
			if got := exitCodeOfErr(dryErr); got != tc.illegalExit {
				t.Errorf("%s: dry-run illegal-transition exit = %d, want %d (err=%v)", tc.name, got, tc.illegalExit, dryErr)
			}
			after := snapshotDir(t, root)
			if diff := diffSnapshotPaths(before, after); len(diff) != 0 {
				t.Fatalf("%s: working tree changed on a REJECTED --dry-run transition: %v", tc.name, diff)
			}

			// The same call WITHOUT --dry-run must fail with the identical
			// exit code and message — --dry-run must not invent its own
			// rejection format.
			realArgs := append([]string{"change-status"}, args...)
			_, _, realErr := tc.run(t, realArgs...)
			if realErr == nil {
				t.Fatalf("%s: expected the real (non-dry-run) call to also fail", tc.name)
			}
			if exitCodeOfErr(realErr) != exitCodeOfErr(dryErr) {
				t.Errorf("%s: dry-run exit %d != real-run exit %d", tc.name, exitCodeOfErr(dryErr), exitCodeOfErr(realErr))
			}
			if dryErr.Error() != realErr.Error() {
				t.Errorf("%s: dry-run message %q != real-run message %q", tc.name, dryErr.Error(), realErr.Error())
			}
		})
	}
}
