package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
)

func writeReadinessPlanCLI(t *testing.T, plansDir, slug, status, prerequisite, taskStatus string) {
	t.Helper()
	prerequisiteLine := ""
	if prerequisite != "" {
		prerequisiteLine = "**Prerequisite Plans:** " + prerequisite + "\n"
	}
	task := ""
	if taskStatus != "" {
		task = "\n## Tasks\n\n### Task 1: Work\n\n**Status:** " + taskStatus + "\n"
	}
	body := "# Plan: " + slug + "\n\n**Status:** " + status + "\n" + prerequisiteLine + task
	if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

func TestPlanReadiness_StructuredUnmetPrerequisites(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writeReadinessPlanCLI(t, plansDir, "foundation", "Approved", "", "queued")
	writeReadinessPlanCLI(t, plansDir, "integration", "Executing", "", "in_progress")
	writeReadinessPlanCLI(t, plansDir, "delivery", "Approved", "foundation, integration", "queued")

	stdout, _, err := runPlan(t, "readiness", "delivery", "--format", "json")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	var doc struct {
		Slug  string `json:"slug"`
		Ready bool   `json:"ready"`
		Unmet []struct {
			Slug          string `json:"slug"`
			Status        string `json:"status"`
			DerivedStatus string `json:"derived_status"`
		} `json:"unmet_prerequisites"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode %q: %v", stdout, err)
	}
	if doc.Slug != "delivery" || doc.Ready || len(doc.Unmet) != 2 {
		t.Fatalf("doc = %+v, want delivery with two unmet prerequisites", doc)
	}
	if doc.Unmet[0].Slug != "foundation" || doc.Unmet[0].Status != "Approved" || doc.Unmet[1].Slug != "integration" || doc.Unmet[1].Status != "Executing" || doc.Unmet[1].DerivedStatus != "Executing" {
		t.Errorf("unmet = %+v", doc.Unmet)
	}
}

func TestPlanReadiness_NoPrerequisitesAndMalformedDeclaration(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writeReadinessPlanCLI(t, plansDir, "none", "Approved", "", "queued")
	stdout, _, err := runPlan(t, "readiness", "none", "--format", "text")
	if err != nil {
		t.Fatalf("readiness no prereqs: %v", err)
	}
	if !strings.Contains(stdout, "Ready: true") || !strings.Contains(stdout, "Unmet prerequisites: none") {
		t.Errorf("unexpected no-prereq text: %s", stdout)
	}

	writeReadinessPlanCLI(t, plansDir, "malformed", "Approved", "foundation,", "queued")
	stdout, _, err = runPlan(t, "readiness", "malformed")
	if err != nil {
		t.Fatalf("readiness malformed: %v", err)
	}
	if !strings.Contains(stdout, "ready: false") || !strings.Contains(stdout, "malformed prerequisite declaration") {
		t.Errorf("malformed declaration must not be ready: %s", stdout)
	}

	writeReadinessPlanCLI(t, plansDir, "unmet", "Approved", "foundation", "queued")
	writeReadinessPlanCLI(t, plansDir, "foundation", "Executing", "", "in_progress")
	stdout, _, err = runPlan(t, "readiness", "unmet", "--format", "text")
	if err != nil {
		t.Fatalf("readiness text unmet: %v", err)
	}
	if !strings.Contains(stdout, "foundation (status Executing; derived Executing)") {
		t.Errorf("unexpected unmet text: %s", stdout)
	}

	writeReadinessPlanCLI(t, plansDir, "missing", "Approved", "not-here", "queued")
	stdout, _, err = runPlan(t, "readiness", "missing", "--format", "text")
	if err != nil {
		t.Fatalf("readiness missing prerequisite: %v", err)
	}
	if !strings.Contains(stdout, "derived indeterminate") {
		t.Errorf("missing prerequisite should render indeterminate derived state: %s", stdout)
	}
}

func TestPlanReadiness_CycleIsStableUnreadyQueryData(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writeReadinessPlanCLI(t, plansDir, "alpha", "Implemented", "beta", "complete")
	writeReadinessPlanCLI(t, plansDir, "beta", "Implemented", "alpha", "complete")

	stdout, _, err := runPlan(t, "readiness", "alpha", "--format", "json")
	if err != nil {
		t.Fatalf("cycle readiness: %v", err)
	}
	var doc struct {
		Ready bool `json:"ready"`
		Unmet []struct {
			Slug          string `json:"slug"`
			Status        string `json:"status"`
			DerivedStatus string `json:"derived_status"`
			Reason        string `json:"reason"`
		} `json:"unmet_prerequisites"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Ready || len(doc.Unmet) != 1 || doc.Unmet[0].Status != "invalid" || !strings.Contains(doc.Unmet[0].Reason, "alpha -> beta -> alpha") {
		t.Fatalf("cycle response = %s", stdout)
	}
	if doc.Unmet[0].DerivedStatus != "" || strings.Contains(stdout, "derived_status") {
		t.Errorf("indeterminate cycle must omit derived_status: %s", stdout)
	}
}

func TestPlanReadiness_ArgumentErrors(t *testing.T) {
	setupPlansSpec(t)
	for _, args := range [][]string{{"readiness"}, {"readiness", "a", "b"}, {"readiness", "Bad_Slug"}, {"readiness", "a", "--format", "xml"}} {
		_, _, err := runPlan(t, args...)
		if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
			t.Errorf("args %v exit = %d, want %d; err=%v", args, got, exitcode.InvalidArgs, err)
		}
	}
}

type readinessErrorWriter struct{}

func (readinessErrorWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

type readinessCloseErrorYAMLEnc struct{}

func (readinessCloseErrorYAMLEnc) Encode(any) error { return nil }
func (readinessCloseErrorYAMLEnc) Close() error     { return errors.New("close failed") }

func TestPlanReadiness_ResolveAndEncodeErrors(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writeReadinessPlanCLI(t, plansDir, "ready", "Approved", "", "queued")
	if _, _, err := runPlan(t, "readiness", "missing"); exitCodeOfErr(err) != exitcode.NotFound {
		t.Errorf("missing readiness exit = %d, want %d; err=%v", exitCodeOfErr(err), exitcode.NotFound, err)
	}

	cmd := planReadinessCommand()
	cmd.SetOut(readinessErrorWriter{})
	cmd.SetArgs([]string{"ready"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected YAML encoding write error")
	} else if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("YAML encoding exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}

	bare := t.TempDir()
	_, _, err := runPlan(t, "readiness", "ready", "--project", bare)
	if err == nil {
		t.Fatal("expected project resolution error")
	}
}

func TestPlanReadiness_OutputFailuresUseUnexpectedExit(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writeReadinessPlanCLI(t, plansDir, "ready", "Approved", "", "queued")

	t.Run("json encode", func(t *testing.T) {
		swapJSON(t, errors.New("json failed"))
		cmd := planReadinessCommand()
		cmd.SetArgs([]string{"ready", "--format=json"})
		if err := cmd.Execute(); exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("exit = %d, want %d; err=%v", exitCodeOfErr(err), exitcode.Unexpected, err)
		}
	})

	t.Run("yaml close", func(t *testing.T) {
		old := newYAMLEnc
		newYAMLEnc = func(io.Writer) yamlEnc { return readinessCloseErrorYAMLEnc{} }
		t.Cleanup(func() { newYAMLEnc = old })
		cmd := planReadinessCommand()
		cmd.SetArgs([]string{"ready", "--format=yaml"})
		if err := cmd.Execute(); exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("exit = %d, want %d; err=%v", exitCodeOfErr(err), exitcode.Unexpected, err)
		}
	})

	t.Run("text flush", func(t *testing.T) {
		cmd := planReadinessCommand()
		cmd.SetOut(readinessErrorWriter{})
		cmd.SetArgs([]string{"ready", "--format=text"})
		if err := cmd.Execute(); exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("exit = %d, want %d; err=%v", exitCodeOfErr(err), exitcode.Unexpected, err)
		}
	})
}

func TestReadinessCLIError_PreservesTypedErrorsAndMapsUntypedFailures(t *testing.T) {
	typed := exitcode.NotFoundError("missing")
	if got := readinessCLIError(typed); got != typed {
		t.Errorf("typed error = %v, want original %v", got, typed)
	}
	if got := exitCodeOfErr(readinessCLIError(errors.New("read failed"))); got != exitcode.Unexpected {
		t.Errorf("untyped readiness error exit = %d, want %d", got, exitcode.Unexpected)
	}
}

func TestWritePlanReadinessText_InvalidGraphReason(t *testing.T) {
	var out bytes.Buffer
	err := writePlanReadinessText(&out, planReadinessDoc{
		Slug:  "alpha",
		Ready: false,
		UnmetPrerequisites: []plan.UnmetPrerequisite{{
			Slug: "beta", Status: "invalid", Reason: "prerequisite cycle: alpha -> beta -> alpha",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "status invalid; prerequisite cycle") {
		t.Errorf("text output = %q", out.String())
	}
}
