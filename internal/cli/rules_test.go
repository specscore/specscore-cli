package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

func runRulesCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := rulesCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestRules_ExitsZeroWithNonEmptyListing(t *testing.T) {
	out, _, err := runRulesCmd(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected non-empty rules listing, got empty output")
	}
}

func TestRules_EveryRuleIDAppears(t *testing.T) {
	out, _, err := runRulesCmd(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, r := range lint.AllRules() {
		if !strings.Contains(out, r.ID) {
			t.Errorf("output missing rule id %q", r.ID)
		}
	}
}

func TestRules_LineCarriesFamilyAndDescription(t *testing.T) {
	out, _, err := runRulesCmd(t)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert a couple of known rules appear with family + description substring,
	// each on a single line.
	checks := []struct {
		id     string
		family string
		desc   string
	}{
		{"readme-exists", "core", "Requires a README.md in every spec directory."},
		{"idea-location", "idea", "Requires idea files to live under spec/ideas/."},
	}

	lines := strings.Split(out, "\n")
	for _, c := range checks {
		var found bool
		for _, line := range lines {
			if strings.Contains(line, c.id) {
				found = true
				if !strings.Contains(line, c.family) {
					t.Errorf("line for %q missing family %q: %q", c.id, c.family, line)
				}
				if !strings.Contains(line, c.desc) {
					t.Errorf("line for %q missing description %q: %q", c.id, c.desc, line)
				}
				break
			}
		}
		if !found {
			t.Errorf("no line found for rule id %q", c.id)
		}
	}
}

func TestRules_DeterministicAndSorted(t *testing.T) {
	out1, _, err1 := runRulesCmd(t)
	out2, _, err2 := runRulesCmd(t)
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if out1 != out2 {
		t.Error("output is not byte-identical across runs")
	}

	// IDs must appear in sorted order.
	var ids []string
	for _, r := range lint.AllRules() {
		ids = append(ids, r.ID)
	}
	want := append([]string(nil), ids...)
	sort.Strings(want)

	// Locate each ID's position in the output and confirm increasing offsets.
	prev := -1
	for _, id := range want {
		idx := strings.Index(out1, id)
		if idx < 0 {
			t.Fatalf("id %q not found in output", id)
		}
		if idx < prev {
			t.Errorf("id %q appears out of sorted order (offset %d < %d)", id, idx, prev)
		}
		prev = idx
	}
}

type ruleJSON struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Family      string `json:"family"`
	Severity    string `json:"severity"`
}

func TestRules_FamilyIdeaFormatJSON(t *testing.T) {
	out, _, err := runRulesCmd(t, "--family", "idea", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []ruleJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if len(got) == 0 {
		t.Fatal("expected non-empty idea-family JSON result set")
	}
	for _, r := range got {
		if r.Family != "idea" {
			t.Errorf("rule %q has family %q, want \"idea\"", r.ID, r.Family)
		}
		if !strings.HasPrefix(r.ID, "idea-") {
			t.Errorf("rule id %q does not start with \"idea-\"", r.ID)
		}
		if r.ID == "" || r.Description == "" || r.Family == "" || r.Severity == "" {
			t.Errorf("rule object missing a field: %+v", r)
		}
	}
}

func TestRules_FormatJSONEmitsAllRules(t *testing.T) {
	out, _, err := runRulesCmd(t, "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var got []ruleJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if want := len(lint.AllRules()); len(got) != want {
		t.Errorf("got %d JSON rules, want %d", len(got), want)
	}
}

func TestRules_FamilyCoreText(t *testing.T) {
	out, _, err := runRulesCmd(t, "--family", "core")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatal("expected non-empty core-family listing")
	}
	for _, line := range lines {
		if !strings.Contains(line, "[core]") {
			t.Errorf("non-core line in --family core output: %q", line)
		}
	}
}

func TestRules_FormatBogusErrors(t *testing.T) {
	_, _, err := runRulesCmd(t, "--format", "bogus")
	if err == nil {
		t.Fatal("expected an invalid-args error for --format bogus, got nil")
	}
}

func TestRules_FormatJSONDeterministic(t *testing.T) {
	out1, _, err1 := runRulesCmd(t, "--format", "json")
	out2, _, err2 := runRulesCmd(t, "--format", "json")
	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected errors: %v, %v", err1, err2)
	}
	if out1 != out2 {
		t.Error("JSON output is not byte-identical across runs")
	}
}

// newTempRepo creates a temp dir holding a minimal specscore.yaml and points
// osGetwdFn at it so findRepoConfigRoot resolves to the temp repo. The seam is
// restored via t.Cleanup.
func newTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "specscore.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatalf("writing specscore.yaml: %v", err)
	}
	orig := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = orig })
	return dir
}

func TestRules_WriteGeneratesCatalog(t *testing.T) {
	dir := newTempRepo(t)

	_, _, err := runRulesCmd(t, "--write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := os.ReadFile(filepath.Join(dir, lint.CatalogPath))
	if readErr != nil {
		t.Fatalf("catalog file not written: %v", readErr)
	}
	if string(got) != lint.RenderCatalog() {
		t.Errorf("catalog content does not match RenderCatalog()")
	}
}

func TestRules_WriteRegeneratesStaleCatalog(t *testing.T) {
	dir := newTempRepo(t)

	// Pre-write a stale catalog so the run must overwrite it.
	dest := filepath.Join(dir, lint.CatalogPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("creating docs dir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("old content\n"), 0o644); err != nil {
		t.Fatalf("writing stale catalog: %v", err)
	}

	_, _, err := runRulesCmd(t, "--write")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := os.ReadFile(dest)
	if readErr != nil {
		t.Fatalf("catalog file not readable: %v", readErr)
	}
	if strings.Contains(string(got), "old content") {
		t.Error("stale content still present after --write")
	}
	if string(got) != lint.RenderCatalog() {
		t.Errorf("catalog content does not match RenderCatalog() after regeneration")
	}
}

func TestRules_CheckMatchExitsZeroNoOutput(t *testing.T) {
	dir := newTempRepo(t)

	// Bring the catalog in sync first.
	if err := lint.WriteCatalog(dir); err != nil {
		t.Fatalf("seeding catalog: %v", err)
	}

	out, errOut, err := runRulesCmd(t, "--check")
	if err != nil {
		t.Fatalf("expected exit 0 on in-sync catalog, got error: %v", err)
	}
	if out != "" || errOut != "" {
		t.Errorf("expected no output on match, got stdout=%q stderr=%q", out, errOut)
	}
}

func TestRules_CheckDriftReportsAndErrors(t *testing.T) {
	dir := newTempRepo(t)

	// Write a stale catalog that diverges from the registry rendering.
	dest := filepath.Join(dir, lint.CatalogPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("creating docs dir: %v", err)
	}
	if err := os.WriteFile(dest, []byte("old content\n"), 0o644); err != nil {
		t.Fatalf("writing stale catalog: %v", err)
	}

	_, _, err := runRulesCmd(t, "--check")
	if err == nil {
		t.Fatal("expected a drift error, got nil")
	}
	if !strings.Contains(err.Error(), lint.CatalogPath) {
		t.Errorf("drift error should name %q, got: %v", lint.CatalogPath, err)
	}
	if !strings.Contains(err.Error(), "--write") {
		t.Errorf("drift error should advise running --write, got: %v", err)
	}
}

func TestRules_CheckMissingFileReportsAndErrors(t *testing.T) {
	newTempRepo(t)

	_, _, err := runRulesCmd(t, "--check")
	if err == nil {
		t.Fatal("expected an error when docs/lint-rules.md is missing, got nil")
	}
	if !strings.Contains(err.Error(), lint.CatalogPath) {
		t.Errorf("missing-file error should name %q, got: %v", lint.CatalogPath, err)
	}
}

func TestRules_CheckAndWriteConflict(t *testing.T) {
	newTempRepo(t)

	_, _, err := runRulesCmd(t, "--check", "--write")
	if err == nil {
		t.Fatal("expected a conflicting-args error for --check --write, got nil")
	}
}
