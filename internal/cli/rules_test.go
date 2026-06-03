package cli

import (
	"bytes"
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
