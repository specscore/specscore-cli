package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// The command-level cases deliberately exercise every generic lifecycle kind
// with both a resolvable and missing id. Besides guarding the public command
// contract, this keeps each kind's resolver wiring covered instead of relying
// on a Feature-only smoke test.
func TestTransitions_AllGenericKindsResolveYAMLAndRejectMissingIDs(t *testing.T) {
	tests := []struct {
		name  string
		stage func(t *testing.T) string
		run   func(t *testing.T, args ...string) (string, string, error)
	}{
		{
			name: "decision",
			stage: func(t *testing.T) string {
				_, slug := stageDecisionCLI(t, "auth", "Draft")
				return slug
			},
			run: runDecision,
		},
		{
			name: "feature",
			stage: func(t *testing.T) string {
				setupFeatureSpec(t, "Draft")
				return "auth"
			},
			run: runFeature,
		},
		{
			name: "idea",
			stage: func(t *testing.T) string {
				stageActiveIdea(t, "auth", "Draft", "")
				return "auth"
			},
			run: runIdea,
		},
		{
			name: "lesson",
			stage: func(t *testing.T) string {
				stageLesson(t, "auth", "Recorded")
				return "auth"
			},
			run: runLesson,
		},
		{
			name: "plan",
			stage: func(t *testing.T) string {
				stagePlan(t, "auth", "Draft")
				return "auth"
			},
			run: runPlan,
		},
		{
			name: "task",
			stage: func(t *testing.T) string {
				stageTaskWithStatus(t, "auth", "planning")
				return "auth"
			},
			run: runTask,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.stage(t)
			out, stderr, err := tt.run(t, "transitions", id, "--format=yaml")
			if err != nil {
				t.Fatalf("transitions %s: %v\nstderr=%s", id, err, stderr)
			}
			if !strings.Contains(out, "status:") {
				t.Fatalf("YAML output missing status:\n%s", out)
			}
			if _, _, err := tt.run(t, "transitions", "missing"); err == nil {
				t.Fatal("missing id unexpectedly resolved")
			}
		})
	}
}

func TestTransitions_RenderAllFormats(t *testing.T) {
	for _, format := range []string{"text", "json", "yaml"} {
		t.Run(format, func(t *testing.T) {
			var out bytes.Buffer
			if err := printKindMatrix(&out, lifecycle.KindFeature, format); err != nil {
				t.Fatalf("printKindMatrix: %v", err)
			}
			if out.Len() == 0 {
				t.Fatal("empty matrix output")
			}
			out.Reset()
			if err := printArtifactEdge(&out, "auth", lifecycle.Status("Draft"), nil, []lifecycle.Status{lifecycle.Status("Approved")}, format); err != nil {
				t.Fatalf("printArtifactEdge: %v", err)
			}
			if out.Len() == 0 {
				t.Fatal("empty artifact output")
			}
		})
	}
}
