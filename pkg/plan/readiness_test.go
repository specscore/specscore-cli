package plan

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func readinessPlanBody(status, prerequisite, taskStatus string) string {
	prerequisiteLine := ""
	if prerequisite != "" {
		prerequisiteLine = "**Prerequisite Plans:** " + prerequisite + "\n"
	}
	tasks := ""
	if taskStatus != "" {
		tasks = "\n## Tasks\n\n### Task 1: Work\n\n**Status:** " + taskStatus + "\n"
	}
	return "# Plan: Test\n\n**Status:** " + status + "\n" + prerequisiteLine + tasks
}

func writeReadinessPlan(t *testing.T, plansDir, slug, status, prerequisite, taskStatus string) {
	t.Helper()
	body := strings.Replace(readinessPlanBody(status, prerequisite, taskStatus), "# Plan: Test", "# Plan: "+slug, 1)
	if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plan %s: %v", slug, err)
	}
}

func TestPlanReadiness_TraversesMultiHopUnmetPrerequisite(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "integration", "Implemented", "foundation", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "integration", "queued")

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) != 1 || got.Unmet[0].Slug != "foundation" || got.Unmet[0].Status != "Approved" {
		t.Fatalf("readiness = %+v, want reachable foundation prerequisite", got)
	}
}

func TestPlanReadiness_H2BeforeTitleCannotBypassPrerequisite(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "queued")
	delivery := `## Preface

# Plan: Delivery

**Status:** Approved
**Prerequisite Plans:** foundation

## Tasks

### Task 1: Work

**Status:** queued
`
	if err := os.WriteFile(filepath.Join(plansDir, "delivery.md"), []byte(delivery), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	want := []UnmetPrerequisite{{Slug: "foundation", Status: "Approved"}}
	if got.Ready || !slices.Equal(got.Unmet, want) {
		t.Fatalf("readiness = %+v, want unmet prerequisite %+v", got, want)
	}
}

func TestPlanReadiness_CycleIsUnreadyEvenWhenDirectPrerequisiteIsImplemented(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "alpha", "Implemented", "beta", "complete")
	writeReadinessPlan(t, plansDir, "beta", "Implemented", "alpha", "complete")

	got, err := PlanReadiness(root, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) != 1 || got.Unmet[0].Status != "invalid" || !strings.Contains(got.Unmet[0].Reason, "alpha -> beta -> alpha") {
		t.Fatalf("readiness = %+v, want invalid cycle", got)
	}
	if got.Unmet[0].DerivedStatus != "" {
		t.Errorf("cycle derived status = %q, want omitted/indeterminate", got.Unmet[0].DerivedStatus)
	}
	if !strings.Contains(got.UnmetMessage(), "prerequisite cycle: alpha -> beta -> alpha") {
		t.Errorf("cycle diagnostic = %q", got.UnmetMessage())
	}
}

func TestPlanReadiness_DeduplicatesSharedUnmetLeafInFirstReachableOrder(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "first", "Implemented", "shared", "complete")
	writeReadinessPlan(t, plansDir, "shared", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "second", "Implemented", "shared, other", "complete")
	writeReadinessPlan(t, plansDir, "other", "Executing", "", "in_progress")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "first, second", "queued")

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	want := []UnmetPrerequisite{
		{Slug: "shared", Status: "Approved"},
		{Slug: "other", Status: "Executing", DerivedStatus: "Executing"},
	}
	if !slices.Equal(got.Unmet, want) {
		t.Fatalf("unmet = %+v, want %+v", got.Unmet, want)
	}
}

func TestPlanReadiness_ReportsUnmetPrerequisiteBeforeItsUnmetDescendants(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// integration is itself unready and also depends on foundation. The
	// readiness response must name both blockers in pre-order, rather than
	// hiding integration behind its nested prerequisite.
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "integration", "Executing", "foundation", "in_progress")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "integration", "queued")

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	want := []UnmetPrerequisite{
		{Slug: "integration", Status: "Executing", DerivedStatus: "Executing"},
		{Slug: "foundation", Status: "Approved"},
	}
	if !slices.Equal(got.Unmet, want) {
		t.Fatalf("unmet = %+v, want %+v", got.Unmet, want)
	}
}

func TestPlanReadiness_ReportsIndeterminateDirectPrerequisiteBeforeUnmetDescendant(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// intermediate has a pre-execution task, so its own rollup is
	// indeterminate. Its declared leaf is also unmet. Both are direct,
	// actionable blockers for delivery and must remain visible.
	writeReadinessPlan(t, plansDir, "leaf", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "intermediate", "Planning", "leaf", "queued")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "intermediate", "queued")

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	want := []UnmetPrerequisite{
		{Slug: "intermediate", Status: "Planning"},
		{Slug: "leaf", Status: "Approved"},
	}
	if !slices.Equal(got.Unmet, want) {
		t.Fatalf("unmet = %+v, want %+v", got.Unmet, want)
	}
}

func TestPlanReadiness_DeduplicatesSharedCycleAndPreservesBranchedCycles(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "first", "Implemented", "shared", "complete")
	writeReadinessPlan(t, plansDir, "second", "Implemented", "shared", "complete")
	writeReadinessPlan(t, plansDir, "shared", "Implemented", "first", "complete")
	writeReadinessPlan(t, plansDir, "third", "Implemented", "fourth", "complete")
	writeReadinessPlan(t, plansDir, "fourth", "Implemented", "third", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "first, second, third", "queued")

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) != 2 {
		t.Fatalf("readiness = %+v, want two unique cycle diagnostics", got)
	}
	for i, want := range []string{
		"first -> shared -> first",
		"third -> fourth -> third",
	} {
		if got.Unmet[i].Status != "invalid" || !strings.Contains(got.Unmet[i].Reason, want) {
			t.Errorf("unmet[%d] = %+v, want cycle %q", i, got.Unmet[i], want)
		}
	}
}

func TestPrerequisiteReadiness_BoundsSharedDependencyAndAcceptsEmptyRootSlug(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Implemented", "", "complete")
	writeReadinessPlan(t, plansDir, "alpha", "Implemented", "foundation", "complete")
	writeReadinessPlan(t, plansDir, "beta", "Implemented", "foundation", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "alpha, beta", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	p.Slug = ""
	if got, err := p.PrerequisiteReadiness(plansDir); err != nil || !got.Ready {
		t.Fatalf("readiness = %+v, %v; want ready DAG", got, err)
	}
}

func TestPrerequisiteReadiness_AllImplemented(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The recorded status deliberately stays Approved: readiness relies on the
	// derived task rollup, not this field.
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "integration", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation, integration", "queued")

	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || len(got.Unmet) != 0 {
		t.Errorf("readiness = %+v, want ready", got)
	}
}

func TestPrerequisiteReadiness_ReportsEveryUnmetSlugAndStatus(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "integration", "Executing", "", "in_progress")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation, integration", "queued")

	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) != 2 {
		t.Fatalf("readiness = %+v, want two unmet prerequisites", got)
	}
	if got.Unmet[0] != (UnmetPrerequisite{Slug: "foundation", Status: "Approved"}) {
		t.Errorf("first unmet = %+v", got.Unmet[0])
	}
	if got.Unmet[1] != (UnmetPrerequisite{Slug: "integration", Status: "Executing", DerivedStatus: "Executing"}) {
		t.Errorf("second unmet = %+v", got.Unmet[1])
	}
	for _, want := range []string{"foundation", "Approved", "integration", "Executing"} {
		if !strings.Contains(got.UnmetMessage(), want) {
			t.Errorf("UnmetMessage %q missing %q", got.UnmetMessage(), want)
		}
	}
}

func TestPrerequisiteReadiness_UsesUnsetForPrerequisiteWithoutRecordedStatus(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "", "", "queued")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unmet) != 1 || got.Unmet[0].Status != "unset" {
		t.Errorf("readiness = %+v, want prerequisite status unset", got)
	}
}

func TestPrerequisiteReadiness_NoPrerequisitesIsReady(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || len(got.Unmet) != 0 {
		t.Errorf("readiness = %+v, want ready", got)
	}
}

func TestPrerequisiteReadiness_MalformedDeclarationIsNeverReady(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation,", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) == 0 || got.Unmet[0].Status != "invalid" {
		t.Errorf("readiness = %+v, want malformed declaration to be unready", got)
	}
}

func TestPrerequisiteReadiness_MalformedDeclarationsDoNotResolveAnyEdge(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, prerequisite := range []string{"../../secret", "foundation\n**Prerequisite Plans:** replacement"} {
		t.Run(strings.ReplaceAll(prerequisite, "\n", "-"), func(t *testing.T) {
			writeReadinessPlan(t, plansDir, "delivery", "Approved", prerequisite, "queued")
			p, err := Parse(filepath.Join(plansDir, "delivery.md"))
			if err != nil {
				t.Fatal(err)
			}
			originalResolve := resolveReadinessPlanFile
			called := false
			resolveReadinessPlanFile = func(string, string) (string, error) {
				called = true
				return "", errors.New("resolver must not be called for malformed input")
			}
			t.Cleanup(func() { resolveReadinessPlanFile = originalResolve })

			got, err := p.PrerequisiteReadiness(plansDir)
			if err != nil {
				t.Fatal(err)
			}
			if called {
				t.Fatal("readiness resolved a malformed prerequisite edge")
			}
			if got.Ready || len(got.Unmet) != 1 || got.Unmet[0].Status != "invalid" {
				t.Fatalf("readiness = %+v, want one invalid declaration refusal", got)
			}
		})
	}
}

func TestPrerequisiteReadiness_DirectoryFormPrerequisite(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	dir := filepath.Join(plansDir, "foundation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readinessPlanBody("Approved", "", "complete")), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready {
		t.Errorf("directory-form prerequisite readiness = %+v, want ready", got)
	}
}

func TestPlanReadiness_RejectsNonPlanRootInDirectoryForm(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "spec", "plans", "delivery", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Notes\n\nnot a plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := PlanReadiness(root, "delivery"); !isInvalidState(err) {
		t.Fatalf("readiness error = %v, want invalid-state non-Plan refusal", err)
	}
}

func TestPlanReadiness_RejectsRootWhoseLaterH1LooksLikePlan(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "spec", "plans", "delivery.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Notes\n\n# Plan: Delivery\n\n**Status:** Approved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := PlanReadiness(root, "delivery"); !isInvalidState(err) {
		t.Fatalf("readiness error = %v, want invalid-state later-H1 root refusal", err)
	}
}

func TestPlanReadiness_RejectsRootWhoseFakeCodeHeadingLooksLikePlan(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "fenced backtick example",
			body: "```markdown\n# Plan: Delivery\n```\n# Notes\n",
		},
		{
			name: "fenced tilde example",
			body: "~~~markdown\n# Plan: Delivery\n~~~\n# Notes\n",
		},
		{
			name: "indented code example",
			body: "    # Plan: Delivery\n# Notes\n",
		},
		{
			name: "HTML comment example",
			body: "<!--\n# Plan: Delivery\n-->\n# Notes\n",
		},
		{
			name: "frontmatter example",
			body: "---\n# Plan: Delivery\n---\n# Notes\n",
		},
		{
			name: "earlier Setext H1",
			body: "Notes\n=====\n# Plan: Delivery\n",
		},
		{
			name: "tab-separated earlier ATX H1",
			body: "#\tNotes\n# Plan: Delivery\n",
		},
		{
			name: "three-space earlier ATX H1",
			body: "   # Notes\n# Plan: Delivery\n",
		},
		{
			name: "bare earlier ATX H1",
			body: "#\n# Plan: Delivery\n",
		},
		{
			name: "one-character earlier Setext H1",
			body: "Notes\n=\n# Plan: Delivery\n",
		},
		{
			name: "BOM-prefixed frontmatter title",
			body: "\ufeff---\n# Plan: Metadata fake\n<!-- comment -->\n---\n# Notes\n",
		},
		{
			name: "BOM-prefixed Notes title",
			body: "\ufeff# Notes\n# Plan: Delivery\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "spec", "plans", "delivery.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}

			if _, err := PlanReadiness(root, "delivery"); !isInvalidState(err) {
				t.Fatalf("readiness error = %v, want invalid-state fake-heading root refusal", err)
			}
		})
	}
}

func TestPlanReadiness_UsesFirstRealH1AfterIndentedCode(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "delivery.md"), []byte("    # Notes\n# Plan: Delivery\n\n**Status:** Approved\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := PlanReadiness(root, "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready {
		t.Fatalf("readiness = %+v, want first real H1 Plan accepted", got)
	}
}

func TestPrerequisiteReadiness_RejectsNonPlanPrerequisite(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "foundation.md"), []byte("# Notes\n\nnot a plan\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.PrerequisiteReadiness(plansDir); !isInvalidState(err) {
		t.Fatalf("readiness error = %v, want invalid-state non-Plan refusal", err)
	}
}

func TestPrerequisiteReadiness_RejectsPrerequisiteWhoseLaterH1LooksLikePlan(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "foundation.md"), []byte("# Notes\n\n# Plan: Foundation\n\n**Status:** Approved\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := p.PrerequisiteReadiness(plansDir); !isInvalidState(err) {
		t.Fatalf("readiness error = %v, want invalid-state later-H1 prerequisite refusal", err)
	}
}

func TestPrerequisiteReadiness_RejectsPrerequisiteWhoseFakeCodeHeadingLooksLikePlan(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "fenced backtick example",
			body: "```markdown\n# Plan: Foundation\n```\n# Notes\n",
		},
		{
			name: "fenced tilde example",
			body: "~~~markdown\n# Plan: Foundation\n~~~\n# Notes\n",
		},
		{
			name: "indented code example",
			body: "    # Plan: Foundation\n# Notes\n",
		},
		{
			name: "HTML comment example",
			body: "<!--\n# Plan: Foundation\n-->\n# Notes\n",
		},
		{
			name: "frontmatter example",
			body: "---\n# Plan: Foundation\n---\n# Notes\n",
		},
		{
			name: "earlier Setext H1",
			body: "Notes\n=====\n# Plan: Foundation\n",
		},
		{
			name: "tab-separated earlier ATX H1",
			body: "#\tNotes\n# Plan: Foundation\n",
		},
		{
			name: "three-space earlier ATX H1",
			body: "   # Notes\n# Plan: Foundation\n",
		},
		{
			name: "bare earlier ATX H1",
			body: "#\n# Plan: Foundation\n",
		},
		{
			name: "one-character earlier Setext H1",
			body: "Notes\n=\n# Plan: Foundation\n",
		},
		{
			name: "BOM-prefixed frontmatter title",
			body: "\ufeff---\n# Plan: Metadata fake\n<!-- comment -->\n---\n# Notes\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plansDir := filepath.Join(t.TempDir(), "spec", "plans")
			if err := os.MkdirAll(plansDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(plansDir, "foundation.md"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
			p, err := Parse(filepath.Join(plansDir, "delivery.md"))
			if err != nil {
				t.Fatal(err)
			}

			if _, err := p.PrerequisiteReadiness(plansDir); !isInvalidState(err) {
				t.Fatalf("readiness error = %v, want invalid-state fake-heading prerequisite refusal", err)
			}
		})
	}
}

func TestMalformedPrerequisiteDeclaration(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{"absent", Plan{}, ""},
		{"em-dash", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "—"}, ""},
		{"empty", Plan{PrerequisiteLine: 1, PrerequisiteRaw: " "}, "empty"},
		{"invalid", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "Bad_Slug"}, "invalid slug"},
		{"duplicate", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "alpha, alpha"}, "duplicate slug"},
		{"duplicate field", Plan{PrerequisiteLine: 1, PrerequisiteLines: []int{1, 2}, PrerequisiteRaw: "alpha"}, "duplicate field"},
		{"self", Plan{Slug: "alpha", PrerequisiteLine: 1, PrerequisiteRaw: "alpha"}, "self reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := malformedPrerequisiteDeclaration(&tt.plan); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanReadiness_ResolvesAndReportsReadError(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "", "queued")
	got, err := PlanReadiness(root, "delivery")
	if err != nil || !got.Ready {
		t.Fatalf("PlanReadiness = %+v, %v", got, err)
	}

	writeReadinessPlan(t, plansDir, "blocked", "Approved", "", "queued")
	originalParse := parseReadinessPlan
	parseReadinessPlan = func(string) (*Plan, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { parseReadinessPlan = originalParse })
	if _, err := PlanReadiness(root, "blocked"); err == nil {
		t.Fatal("expected parse error")
	} else if !isUnexpected(err) {
		t.Fatalf("parse error = %v, want typed Unexpected", err)
	}
	parseReadinessPlan = originalParse
	if _, err := PlanReadiness(root, "missing"); err == nil {
		t.Fatal("expected missing plan error")
	}
}

func TestPrerequisiteReadiness_PrerequisiteParseFailure(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	originalParse := parseReadinessPlan
	parseReadinessPlan = func(string) (*Plan, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { parseReadinessPlan = originalParse })
	if _, err := p.PrerequisiteReadiness(plansDir); err == nil {
		t.Fatal("expected prerequisite parse error")
	} else if !isUnexpected(err) {
		t.Fatalf("parse error = %v, want typed Unexpected", err)
	}
}

func TestPrerequisiteReadiness_OnlyNotFoundIsRenderedMissing(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	originalResolve := resolveReadinessPlanFile
	resolveReadinessPlanFile = func(string, string) (string, error) {
		return "", exitcode.UnexpectedError("permission denied")
	}
	t.Cleanup(func() { resolveReadinessPlanFile = originalResolve })
	if _, err := p.PrerequisiteReadiness(plansDir); !isUnexpected(err) {
		t.Fatalf("resolution error = %v, want typed Unexpected", err)
	}

	resolveReadinessPlanFile = func(string, string) (string, error) {
		return "", exitcode.NotFoundError("missing")
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil || got.Ready || len(got.Unmet) != 1 || got.Unmet[0].Status != "missing" {
		t.Fatalf("NotFound readiness = %+v, %v; want normal missing prerequisite", got, err)
	}
}

func TestPrerequisiteReadiness_PropagatesNestedResolutionFailure(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "alpha", "Implemented", "foundation", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "alpha", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	originalResolve := resolveReadinessPlanFile
	resolveReadinessPlanFile = func(dir, slug string) (string, error) {
		if slug == "foundation" {
			return "", exitcode.UnexpectedError("I/O failure")
		}
		return resolvePlanFile(dir, slug)
	}
	t.Cleanup(func() { resolveReadinessPlanFile = originalResolve })
	if _, err := p.PrerequisiteReadiness(plansDir); !isUnexpected(err) {
		t.Fatalf("nested resolution error = %v, want typed Unexpected", err)
	}
}

func isUnexpected(err error) bool {
	var coded interface{ ExitCode() int }
	return errors.As(err, &coded) && coded.ExitCode() == exitcode.Unexpected
}

func isInvalidState(err error) bool {
	var coded interface{ ExitCode() int }
	return errors.As(err, &coded) && coded.ExitCode() == exitcode.InvalidState
}
