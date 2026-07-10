package blocks_test

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// fakeBlock is a minimal Block implementation for registry tests.
type fakeBlock struct{ kind string }

func (f fakeBlock) Kind() string { return f.kind }
func (f fakeBlock) Run(_ blocks.StepCtx) blocks.StepResult {
	return blocks.StepResult{Status: blocks.StatusPass}
}

func TestNewRegistry_KeysByKind(t *testing.T) {
	reg := blocks.NewRegistry(fakeBlock{kind: "bash"}, fakeBlock{kind: "hurl"})
	if len(reg) != 2 {
		t.Fatalf("registry size = %d, want 2", len(reg))
	}
	for _, kind := range []string{"bash", "hurl"} {
		b, ok := reg[kind]
		if !ok {
			t.Fatalf("registry lacks kind %q", kind)
		}
		if b.Kind() != kind {
			t.Errorf("registry[%q].Kind() = %q", kind, b.Kind())
		}
	}
	if _, ok := reg["sql"]; ok {
		t.Error("registry unexpectedly contains an unregistered kind")
	}
}

func TestTruncate_UnderBudgetUntouched(t *testing.T) {
	in := strings.Repeat("x", blocks.MaxStepOutput)
	if got := blocks.Truncate(in); got != in {
		t.Errorf("Truncate changed output that fits the budget (len %d)", len(got))
	}
}

func TestTruncate_OverBudgetCutWithNote(t *testing.T) {
	in := strings.Repeat("x", blocks.MaxStepOutput+1)
	got := blocks.Truncate(in)
	if !strings.HasPrefix(got, strings.Repeat("x", blocks.MaxStepOutput)) {
		t.Error("Truncate did not keep the first 8 KiB")
	}
	if !strings.HasSuffix(got, "[step output truncated at 8 KiB]") {
		t.Errorf("Truncate note missing; tail: %q", got[len(got)-40:])
	}
	if strings.Count(got, "x") != blocks.MaxStepOutput {
		t.Errorf("Truncate kept %d payload bytes, want %d", strings.Count(got, "x"), blocks.MaxStepOutput)
	}
}
