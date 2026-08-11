package lint

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Task 2 — parse the "**Implements:**" line by reusing the source-references
// specscore: grammar, including the cross-repo "@{host}/{org}/{repo}" suffix.
func TestParseImplementsRef(t *testing.T) {
	cases := []struct {
		name          string
		line          string
		wantSlug      string
		wantCrossRepo bool
		wantErr       bool
	}{
		{
			name:     "same-repo feature reference",
			line:     "**Implements:** specscore:feature/dashboards",
			wantSlug: "dashboards",
		},
		{
			name:          "cross-repo authority reference carries suffix",
			line:          "**Implements:** specscore://github.com/datatug/datatug/feature/dashboards",
			wantSlug:      "dashboards",
			wantCrossRepo: true,
		},
		{
			name:    "legacy cross-repo suffix form is an error",
			line:    "**Implements:** specscore:feature/dashboards@github.com/datatug/datatug",
			wantErr: true,
		},
		{
			name:     "tolerates leading whitespace and trailing text",
			line:     "  **Implements:** specscore:feature/dashboards  (the capability)",
			wantSlug: "dashboards",
		},
		{
			name:    "no recognizable reference",
			line:    "**Implements:** dashboards",
			wantErr: true,
		},
		{
			name:    "empty short notation is malformed",
			line:    "**Implements:** specscore:",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseImplementsRef(tc.line)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseImplementsRef(%q) = %+v, want error", tc.line, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseImplementsRef(%q) unexpected error: %v", tc.line, err)
			}
			if got.crossRepo != tc.wantCrossRepo {
				t.Errorf("crossRepo = %v, want %v", got.crossRepo, tc.wantCrossRepo)
			}
			if got.featureSlug() != tc.wantSlug {
				t.Errorf("featureSlug = %q, want %q", got.featureSlug(), tc.wantSlug)
			}
		})
	}
}

// Task 2 — a same-repo Implements reference that resolves to an existing
// Capability reports no violation (capability-and-platform-implementations#
// ac:implements-single-same-repo).
func TestImplementsReference_SameRepoResolvesToCapability_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards",
		"# Feature: Dashboards\n\n**Status:** Approved\n\n## Implementation Matrix\n\n| Platform | Status | Brief | Link |\n| --- | --- | --- | --- |\n")
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/dashboards\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// An unconfigured cross-repo authority fails closed instead of silently being
// accepted. Lint remains fully offline and asks callers to configure a mirror.
func TestImplementsReference_CrossRepoAuthority_UnavailableErrors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore://github.com/datatug/datatug/feature/dashboards\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 || !strings.Contains(violations[0].Message, "unavailable") {
		t.Fatalf("expected one unavailable-repository violation, got %+v", violations)
	}
}

func TestImplementsReference_RequirementAnchors(t *testing.T) {
	tmp := t.TempDir()
	capability := "# Feature: Dashboards\n\n**Status:** Approved\n\n## Implementation Matrix\n\n#### REQ: listed\n\nworks\n"
	writeFeatureReadme(t, tmp, "dashboards", capability)
	for _, tc := range []struct {
		name    string
		ref     string
		content string
		want    string
	}{
		{"exact heading resolves", "specscore:feature/dashboards#REQ:listed", capability, ""},
		{"renamed requirement fails", "specscore:feature/dashboards#REQ:deleted", capability, "missing exact heading"},
		{"duplicate exact heading fails", "specscore:feature/dashboards#REQ:listed", capability + "\n#### REQ: listed\n", "ambiguous requirement anchor"},
		{"malformed requirement anchor fails", "specscore:feature/dashboards#REQ:", capability, "malformed #REQ anchor"},
		{"non requirement fragment fails", "specscore:feature/dashboards#section", capability, "only #REQ:<id>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFeatureReadme(t, tmp, "dashboards", tc.content)
			writeFeatureReadme(t, tmp, "dashboards-cli", "# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** "+tc.ref+"\n\n## Summary\n")
			violations := runImplementsReferenceCheck(t, tmp)
			if tc.want == "" {
				if len(violations) != 0 {
					t.Fatalf("expected no violations, got %+v", violations)
				}
				return
			}
			if len(violations) != 1 || !strings.Contains(violations[0].Message, tc.want) {
				t.Fatalf("expected %q, got %+v", tc.want, violations)
			}
		})
	}
}

func TestImplementsReference_CrossRepoRequirementResolution(t *testing.T) {
	root := t.TempDir()
	localSpec := filepath.Join(root, "spec")
	mirror := filepath.Join(root, "mirror")
	writeProjectConfig(t, root, "github.com", "acme", "consumer", "./mirror")
	writeProjectConfig(t, mirror, "github.com", "acme", "provider")
	writeFeatureReadme(t, mirror+"/spec", "capability", "# Feature: Capability\n\n## Implementation Matrix\n\n#### REQ: retained\n")
	gitCommitAll(t, mirror)
	writeFeatureReadme(t, localSpec, "implementation", "# Feature: Implementation\n\n**Implements:** specscore://github.com/acme/provider/feature/capability#REQ:deleted\n")
	violations := runImplementsReferenceCheck(t, localSpec)
	if len(violations) != 1 || !strings.Contains(violations[0].Message, "missing exact heading") {
		t.Fatalf("expected deleted cross-repo REQ to fail, got %+v", violations)
	}

	writeFeatureReadme(t, localSpec, "implementation", "# Feature: Implementation\n\n**Implements:** specscore://github.com/acme/provider/feature/capability#REQ:retained\n")
	if got := runImplementsReferenceCheck(t, localSpec); len(got) != 0 {
		t.Fatalf("expected configured exact-identity mirror to resolve, got %+v", got)
	}

	writeProjectConfig(t, mirror, "github.com", "acme", "renamed-provider")
	violations = runImplementsReferenceCheck(t, localSpec)
	if len(violations) != 1 || !strings.Contains(violations[0].Message, "unavailable") {
		t.Fatalf("expected identity mismatch to fail closed, got %+v", violations)
	}
}

func TestImplementsReference_CrossRepoAmbiguousMirrorErrors(t *testing.T) {
	root := t.TempDir()
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	writeProjectConfig(t, root, "github.com", "acme", "consumer", "./first", "./second")
	writeProjectConfig(t, first, "github.com", "acme", "provider")
	writeProjectConfig(t, second, "github.com", "acme", "provider")
	writeFeatureReadme(t, filepath.Join(root, "spec"), "implementation", "# Feature: Implementation\n\n**Implements:** specscore://github.com/acme/provider/feature/capability\n")
	got := runImplementsReferenceCheck(t, filepath.Join(root, "spec"))
	if len(got) != 1 || !strings.Contains(got[0].Message, "ambiguous") {
		t.Fatalf("expected ambiguous mirror violation, got %+v", got)
	}
}

func writeProjectConfig(t *testing.T, dir, host, org, repo string, projects ...string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n")
	fmt.Fprintf(&b, "  host: %s\n  org: %s\n  repo: %s\n", host, org, repo)
	if len(projects) > 0 {
		b.WriteString("projects:\n")
		for _, project := range projects {
			fmt.Fprintf(&b, "  - %s\n", project)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitCommitAll(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "lint@example.test"}, {"config", "user.name", "lint test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// The legacy cross-repo suffix form is reported as an error carrying the exact
// authority-form rewrite, and `--fix` rewrites it in place (decision 0010).
func TestImplementsReference_LegacySuffix_ReportedAndFixed(t *testing.T) {
	tmp := t.TempDir()
	body := "# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/dashboards@github.com/datatug/datatug\n\n## Summary\n"
	writeFeatureReadme(t, tmp, "dashboards-cli", body)

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 || violations[0].FixTarget != "implements-reference" {
		t.Fatalf("expected one fixable legacy-suffix violation, got %+v", violations)
	}
	want := "specscore://github.com/datatug/datatug/feature/dashboards"
	if !strings.Contains(violations[0].Message, want) {
		t.Fatalf("message must carry the rewrite %q: %q", want, violations[0].Message)
	}

	// The fixer rewrites the line in place, preserving surrounding content.
	c := &implementsReferenceChecker{}
	if err := c.fix(tmp); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(tmp, "features", "dashboards-cli", "README.md"))
	if !strings.Contains(string(data), "**Implements:** "+want) {
		t.Fatalf("line not rewritten:\n%s", data)
	}
	if !strings.Contains(string(data), "## Summary") {
		t.Fatalf("surrounding content not preserved:\n%s", data)
	}
	// Idempotent: a second fix pass makes no change, and re-linting is clean.
	if _, changed := rewriteLegacyImplementsLine(data); changed {
		t.Fatal("second fix pass should be a no-op")
	}
}

// The fixer leaves non-Implementation features and authority-form references
// untouched.
func TestImplementsReference_FixSkipsNonLegacy(t *testing.T) {
	if out, changed := rewriteLegacyImplementsLine([]byte("# Feature: Plain\n\n## Summary\n")); changed || string(out) == "" {
		t.Fatal("non-implementation content must be left unchanged")
	}
	authority := "# F\n\n**Implements:** specscore://github.com/o/r/feature/x\n"
	if _, changed := rewriteLegacyImplementsLine([]byte(authority)); changed {
		t.Fatal("authority-form reference must not be rewritten")
	}
	malformed := "# F\n\n**Implements:** not-a-reference\n"
	if _, changed := rewriteLegacyImplementsLine([]byte(malformed)); changed {
		t.Fatal("line without a specscore token must not be rewritten")
	}
}

// Task 3 — a same-repo Implements reference that resolves to nothing is an
// error (capability-and-platform-implementations#ac:implements-unresolved-same-repo).
func TestImplementsReference_UnresolvedSameRepo_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/no-such-capability\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Rule != "implements-reference" {
		t.Errorf("Rule = %q, want implements-reference", v.Rule)
	}
	if !strings.Contains(v.Message, "does not resolve") {
		t.Errorf("Message = %q, want it to mention the reference does not resolve", v.Message)
	}
}

// Task 3 — a same-repo reference that resolves to an existing Feature lacking
// an "## Implementation Matrix" section is an error
// (capability-and-platform-implementations#ac:implements-non-capability).
func TestImplementsReference_TargetNotCapability_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "source-references",
		"# Feature: Source References\n\n**Status:** Approved\n\n## Summary\nNo Implementation Matrix here.\n")
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/source-references\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "not a Capability") {
		t.Errorf("Message = %q, want it to mention the target is not a Capability", violations[0].Message)
	}
}

// A same-repo reference whose resolved path is not a feature at all (e.g. a
// plan reference) cannot point at a Capability — reported as unresolved.
func TestImplementsReference_NonFeatureTarget_Unresolved(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:plan/some-plan\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "does not resolve") {
		t.Errorf("Message = %q, want unresolved", violations[0].Message)
	}
}

// An Implementation whose "**Implements:**" line carries no recognizable
// specscore: reference is a malformed-notation error.
func TestImplementsReference_MalformedNotation_Errors(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** dashboards\n\n## Summary\n")

	violations := runImplementsReferenceCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if !strings.Contains(violations[0].Message, "well-formed") {
		t.Errorf("Message = %q, want malformed-notation message", violations[0].Message)
	}
}

// Metadata: the checker reports under the implements-reference rule at error severity.
func TestImplementsReference_Metadata(t *testing.T) {
	c := newImplementsReferenceChecker()
	if c.name() != "implements-reference" {
		t.Errorf("name = %q", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity = %q", c.severity())
	}
}

// The walk error from an unreadable features subtree propagates out of check.
func TestImplementsReference_WalkError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Implements:** specscore:feature/dashboards\n")
	featDir := filepath.Join(tmp, "features", "dashboards-cli")
	if err := os.Chmod(featDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(featDir, 0o755) }()

	c := newImplementsReferenceChecker()
	if _, err := c.check(tmp); err == nil {
		t.Fatal("expected a walk error from the unreadable features subtree")
	}
}

func runImplementsReferenceCheck(t *testing.T, specRoot string) []Violation {
	t.Helper()
	c := newImplementsReferenceChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	return violations
}
