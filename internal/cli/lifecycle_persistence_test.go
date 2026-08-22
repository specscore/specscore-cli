package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// Regression coverage for the self-concealing transition failure: a
// `change-status` verb printed `<slug>: <from> → <to>` and exited 0 while the
// artifact on disk was byte-identical to its pre-invocation state, because the
// post-mutation `spec lint --fix` pass rewrote the very status line the verb
// had just written (idea-sync-lint-strict re-derives Idea statuses from the
// Features that promote them).
//
// The tests below pin three things:
//
//  1. Every step of the Idea lifecycle actually lands on disk — asserted on
//     BOTH status surfaces (body `**Status:**` and the frontmatter `status:`
//     mirror) and on the file's bytes changing.
//  2. The same for the Feature lifecycle, whose matrix shares the code path.
//  3. A transition that cannot survive the index sync fails LOUDLY (non-zero,
//     empty stdout, file restored) instead of reporting a success that did not
//     happen.

// statusSurfaces returns the two places an artifact records its status: the
// body `**Status:**` line and the YAML frontmatter `status:` mirror. Both MUST
// agree with the requested target after a transition; a stale mirror is the
// same defect class as a reverted body.
func statusSurfaces(t *testing.T, path string) (body, frontmatter string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	inFrontmatter := false
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if i == 0 && trimmed == "---" {
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			if trimmed == "---" || trimmed == "..." {
				inFrontmatter = false
				continue
			}
			if v, ok := strings.CutPrefix(trimmed, "status:"); ok && frontmatter == "" {
				frontmatter = strings.TrimSpace(v)
			}
			continue
		}
		if v, ok := strings.CutPrefix(trimmed, "**Status:**"); ok && body == "" {
			body = strings.TrimSpace(v)
		}
	}
	return body, frontmatter
}

// assertPersisted fails unless path's bytes differ from before AND both status
// surfaces read want. Returns the file's new bytes for the next step's
// before-snapshot.
func assertPersisted(t *testing.T, step, path string, before []byte, want string) []byte {
	t.Helper()
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read %s: %v", step, path, err)
	}
	if string(after) == string(before) {
		t.Fatalf("%s: %s is byte-identical after the transition — nothing was written", step, path)
	}
	body, frontmatter := statusSurfaces(t, path)
	if body != want {
		t.Errorf("%s: body **Status:** = %q; want %q", step, body, want)
	}
	if frontmatter != want {
		t.Errorf("%s: frontmatter status: = %q; want %q", step, frontmatter, want)
	}
	return after
}

func readFileT(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return raw
}

// stageLifecycleProject stages a real, lint-clean project (via `specscore
// init`) holding one Idea and one Feature, and chdirs into it. The Feature's
// `**Source Ideas:**` names the Idea, which is what makes the Idea's forward
// specification band derivable.
func stageLifecycleProject(t *testing.T, ideaSlug, featureSlug string) (root, ideaPath, featurePath string) {
	t.Helper()
	root = t.TempDir()
	withCwd(t, root)

	if _, errOut, err := runInitCmd(t, nil,
		"--project", root,
		"--host", "github.com",
		"--org", "acme",
		"--repo", "widget",
	); err != nil {
		t.Fatalf("init: %v\nstderr=%s", err, errOut)
	}
	if _, errOut, err := runIdea(t, "new", ideaSlug, "--title", "Lifecycle Demo", "--owner", "tester"); err != nil {
		t.Fatalf("idea new: %v\nstderr=%s", err, errOut)
	}
	if _, errOut, err := runFeature(t, "new",
		"--title", "Lifecycle Demo Feature",
		"--slug", featureSlug,
		"--description", "Feature that specifies the demo idea",
	); err != nil {
		t.Fatalf("feature new: %v\nstderr=%s", err, errOut)
	}
	ideaPath = filepath.Join(root, "spec", "ideas", ideaSlug+".md")
	featurePath = filepath.Join(root, "spec", "features", featureSlug, "README.md")
	return root, ideaPath, featurePath
}

// linkFeatureToIdea points the Feature's `**Source Ideas:**` at the Idea. This
// is the on-disk fact the idea-sync derivation reads: from here on, the Idea's
// forward band follows the Feature's status.
func linkFeatureToIdea(t *testing.T, featurePath, ideaSlug string) {
	t.Helper()
	raw := string(readFileT(t, featurePath))
	updated := strings.Replace(raw, "**Source Ideas:** —", "**Source Ideas:** "+ideaSlug, 1)
	if updated == raw {
		t.Fatalf("feature README has no `**Source Ideas:** —` line to link:\n%s", raw)
	}
	if err := os.WriteFile(featurePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("write feature README: %v", err)
	}
}

// TestIdeaLifecycle_EveryStepPersistsToDisk walks the whole Idea lifecycle —
// Draft → Approved → Specifying → Specified → Implementing → Implemented — and
// asserts after EVERY step that the file changed and that both status surfaces
// carry the new status.
//
// The prep band (Draft → Approved) is driven by `idea change-status`. The
// forward specification band is DERIVED from the promoting Feature's status
// (idea-sync-lint-strict), so it is driven here the way a real project drives
// it: by transitioning the Feature and letting the index sync advance the
// Idea. Both halves are asserted identically — a status that does not reach
// disk is a failure whichever verb was asked for it.
func TestIdeaLifecycle_EveryStepPersistsToDisk(t *testing.T) {
	const ideaSlug, featureSlug = "lifecycle-demo", "lifecycle-demo-feature"
	_, ideaPath, featurePath := stageLifecycleProject(t, ideaSlug, featureSlug)

	if body, frontmatter := statusSurfaces(t, ideaPath); body != "Draft" || frontmatter != "Draft" {
		t.Fatalf("scaffolded idea = (%q, %q); want Draft on both surfaces", body, frontmatter)
	}

	before := readFileT(t, ideaPath)

	// Draft → Approved, via the Idea verb itself.
	stdout, stderr, err := runIdea(t, "change-status", ideaSlug, "--to=approved")
	if err != nil {
		t.Fatalf("idea change-status approved: %v\nstderr=%s", err, stderr)
	}
	if want := ideaSlug + ": Draft → Approved\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	before = assertPersisted(t, "Draft → Approved", ideaPath, before, "Approved")

	// From here the Idea's status follows its promoting Feature.
	linkFeatureToIdea(t, featurePath, ideaSlug)

	for _, step := range []struct {
		featureTo string
		ideaWant  string
	}{
		{"In Review", "Specifying"}, // Feature being written  → Specifying
		{"Approved", "Specified"},   // Feature agreed         → Specified
		{"Implementing", "Implementing"},
		{"Stable", "Implemented"},
	} {
		featureBefore := readFileT(t, featurePath)
		if _, stderr, err := runFeature(t, "change-status", featureSlug, "--to", step.featureTo); err != nil {
			t.Fatalf("feature change-status %s: %v\nstderr=%s", step.featureTo, err, stderr)
		}
		assertPersisted(t, "feature → "+step.featureTo, featurePath, featureBefore, step.featureTo)
		before = assertPersisted(t, "idea → "+step.ideaWant, ideaPath, before, step.ideaWant)
	}
}

// TestFeatureLifecycle_EveryStepPersistsToDisk is the same walk for the
// Feature matrix — Draft → In Review → Approved → Implementing → Stable —
// which shares the change-status code path and could share the defect.
func TestFeatureLifecycle_EveryStepPersistsToDisk(t *testing.T) {
	const featureSlug = "feature-lifecycle-demo"
	root := t.TempDir()
	withCwd(t, root)
	if _, errOut, err := runInitCmd(t, nil,
		"--project", root, "--host", "github.com", "--org", "acme", "--repo", "widget",
	); err != nil {
		t.Fatalf("init: %v\nstderr=%s", err, errOut)
	}
	if _, errOut, err := runFeature(t, "new",
		"--title", "Feature Lifecycle Demo",
		"--slug", featureSlug,
		"--description", "Walks its own lifecycle",
	); err != nil {
		t.Fatalf("feature new: %v\nstderr=%s", err, errOut)
	}
	featurePath := filepath.Join(root, "spec", "features", featureSlug, "README.md")

	before := readFileT(t, featurePath)
	for _, to := range []string{"In Review", "Approved", "Implementing", "Stable"} {
		stdout, stderr, err := runFeature(t, "change-status", featureSlug, "--to", to)
		if err != nil {
			t.Fatalf("feature change-status %s: %v\nstderr=%s", to, err, stderr)
		}
		if !strings.HasSuffix(stdout, "→ "+to+"\n") {
			t.Errorf("stdout = %q; want a line ending in %q", stdout, "→ "+to+"\n")
		}
		before = assertPersisted(t, "feature → "+to, featurePath, before, to)
	}
}

// TestIdeaChangeStatus_DerivedBandWithoutPromotingFeatureRefused pins the
// exact reported bug: `--to=Specifying` on an Approved Idea that no Feature
// promotes used to print `<slug>: Approved → Specifying` and exit 0 while
// leaving the file untouched. It must now be refused before any write, with a
// non-zero exit, empty stdout, and a message that says what to do instead.
func TestIdeaChangeStatus_DerivedBandWithoutPromotingFeatureRefused(t *testing.T) {
	const ideaSlug, featureSlug = "unpromoted-idea", "unrelated-feature"
	// The project holds a Feature, but nothing links it to this Idea — so the
	// Idea has no promoting Feature and its **Promotes To:** stays empty.
	_, ideaPath, _ := stageLifecycleProject(t, ideaSlug, featureSlug)
	if _, errOut, err := runIdea(t, "change-status", ideaSlug, "--to=approved"); err != nil {
		t.Fatalf("idea change-status approved: %v\nstderr=%s", err, errOut)
	}

	before := readFileT(t, ideaPath)

	stdout, _, err := runIdea(t, "change-status", ideaSlug, "--to=Specifying")
	if err == nil {
		t.Fatalf("change-status --to=Specifying succeeded on an Idea no Feature promotes; want a refusal")
	}
	if code := exitCodeOfErr(err); code != 4 {
		t.Errorf("exit code = %d; want 4 (invalid state)", code)
	}
	if strings.Contains(stdout, ideaSlug+":") {
		t.Errorf("stdout carries a success line on a failed transition: %q", stdout)
	}
	if msg := err.Error(); !strings.Contains(msg, "Promotes To") || !strings.Contains(msg, "Source Ideas") {
		t.Errorf("error message does not explain the fix: %q", msg)
	}
	if got := readFileT(t, ideaPath); string(got) != string(before) {
		t.Errorf("idea file changed on a refused transition:\n%s", got)
	}
	if body, frontmatter := statusSurfaces(t, ideaPath); body != "Approved" || frontmatter != "Approved" {
		t.Errorf("status surfaces = (%q, %q); want Approved on both", body, frontmatter)
	}
}

// TestIdeaChangeStatus_IndexSyncRevertFailsLoudly covers the general defect
// class behind the report: the transition is written, the post-mutation index
// sync rewrites that same line to a different value, and the verb must NOT
// report success. Here the Idea is promoted by an Approved Feature (so it
// derives `Specified`) and the operator asks for `Implementing` — a status
// only the Feature's own advance can produce.
func TestIdeaChangeStatus_IndexSyncRevertFailsLoudly(t *testing.T) {
	const ideaSlug, featureSlug = "sync-revert-idea", "sync-revert-feature"
	_, ideaPath, featurePath := stageLifecycleProject(t, ideaSlug, featureSlug)

	if _, errOut, err := runIdea(t, "change-status", ideaSlug, "--to=approved"); err != nil {
		t.Fatalf("idea change-status approved: %v\nstderr=%s", err, errOut)
	}
	linkFeatureToIdea(t, featurePath, ideaSlug)
	for _, to := range []string{"In Review", "Approved"} {
		if _, errOut, err := runFeature(t, "change-status", featureSlug, "--to", to); err != nil {
			t.Fatalf("feature change-status %s: %v\nstderr=%s", to, err, errOut)
		}
	}
	if body, _ := statusSurfaces(t, ideaPath); body != "Specified" {
		t.Fatalf("idea = %q; want Specified (derived from the Approved Feature)", body)
	}

	before := readFileT(t, ideaPath)
	stdout, _, err := runIdea(t, "change-status", ideaSlug, "--to=implementing")
	if err == nil {
		t.Fatalf("change-status --to=implementing succeeded; want a loud failure, the index sync reverts it")
	}
	if code := exitCodeOfErr(err); code != 10 {
		t.Errorf("exit code = %d; want 10", code)
	}
	if strings.Contains(stdout, ideaSlug+":") {
		t.Errorf("stdout carries a success line on a failed transition: %q", stdout)
	}
	if msg := err.Error(); !strings.Contains(msg, "did not persist") {
		t.Errorf("error message does not name the failure: %q", msg)
	}
	if got := readFileT(t, ideaPath); string(got) != string(before) {
		t.Errorf("idea file was left mutated after a failed transition:\n%s", got)
	}
	if body, frontmatter := statusSurfaces(t, ideaPath); body != "Specified" || frontmatter != "Specified" {
		t.Errorf("status surfaces = (%q, %q); want Specified on both after rollback", body, frontmatter)
	}
}

// TestFeatureChangeStatus_IndexSyncRevertFailsLoudly drives the Feature verb's
// half of the persistence guard: `spec lint --fix` succeeds but rewrites the
// status line it was meant to sync around. The verb must roll back and exit
// 10 instead of printing `<feature_id>: <from> → <to>`.
func TestFeatureChangeStatus_IndexSyncRevertFailsLoudly(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")
	readme := filepath.Join(root, "spec", "features", "auth", "README.md")

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			// Simulate a --fix rule that re-derives the status back.
			raw := readFileT(t, readme)
			reverted := strings.Replace(string(raw), "**Status:** In Review", "**Status:** Draft", 1)
			if err := os.WriteFile(readme, []byte(reverted), 0o644); err != nil {
				t.Fatalf("revert write: %v", err)
			}
		}
		return orig(opts)
	}
	t.Cleanup(func() { lintLintFn = orig })

	stdout, _, err := runFeature(t, "change-status", "auth", "--to=In Review")
	if err == nil {
		t.Fatal("expected a failure when the index sync reverts the status")
	}
	if code := exitCodeOfErr(err); code != exitcode.Unexpected {
		t.Errorf("exit code = %d; want %d", code, exitcode.Unexpected)
	}
	if strings.Contains(stdout, "auth:") {
		t.Errorf("stdout carries a success line on a failed transition: %q", stdout)
	}
	if !strings.Contains(err.Error(), "did not persist") {
		t.Errorf("error does not name the failure: %v", err)
	}
	if body, _ := statusSurfaces(t, readme); body != "Draft" {
		t.Errorf("feature status = %q; want Draft after rollback", body)
	}
}

// When the artifact is mangled so badly that the rollback itself cannot run,
// both failures must be reported together — never swallowed into a success.
func TestFeatureChangeStatus_PersistenceCheckAndRollbackBothFail(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")
	readme := filepath.Join(root, "spec", "features", "auth", "README.md")

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			// A --fix pass that drops the **Status:** line entirely: the
			// persistence check cannot find it, and neither can the rollback.
			if err := os.WriteFile(readme, []byte("# Feature: Auth\n\n## Summary\n\nGone.\n"), 0o644); err != nil {
				t.Fatalf("mangle write: %v", err)
			}
			return nil, nil
		}
		return nil, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runFeature(t, "change-status", "auth", "--to=In Review")
	if err == nil {
		t.Fatal("expected a failure when the status line vanishes")
	}
	if !strings.Contains(err.Error(), "rollback also failed") {
		t.Errorf("error does not report the failed rollback: %v", err)
	}
}
