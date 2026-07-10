package runner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/bash"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/sqlblock"
)

// fakeBlock is a scriptable executor for wiring tests.
type fakeBlock struct {
	kind string
	run  func(blocks.StepCtx) blocks.StepResult
}

func (f fakeBlock) Kind() string                             { return f.kind }
func (f fakeBlock) Run(ctx blocks.StepCtx) blocks.StepResult { return f.run(ctx) }

// TestRun_CapturedValuesFlowIntoLaterBlocks mirrors AC context-bag-chains
// with the real bash and sql executors: a bash capture feeds an sql query
// whose capture feeds a bash assertion (REQ: context-bag).
func TestRun_CapturedValuesFlowIntoLaterBlocks(t *testing.T) {
	file := filepath.Join(t.TempDir(), "chain.md")
	writeFile(t, file, "**Verifies:** demo/x#ac:chain\n\n"+
		"```bash\necho \"uid=42\" >> \"$REHEARSE_CAPTURES\"\n```\n\n"+
		"```sql dsn=sqlite:fixture.db\n"+
		"CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT);\n"+
		"INSERT INTO users (id, username) VALUES (42, 'alice');\n"+
		"SELECT username FROM users WHERE id = {{uid}};\n"+
		"-- assert-rows: 1\n"+
		"-- capture: name = username\n"+
		"```\n\n"+
		"```bash\n[ \"{{name}}\" = \"alice\" ]\n```\n")

	r := Run(blocks.NewRegistry(bash.New(), sqlblock.New()), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s, steps: %+v)", r.Status, r.Detail, r.Steps)
	}
	if want := map[string]string{"uid": "42", "name": "alice"}; !reflect.DeepEqual(r.Bag, want) {
		t.Errorf("bag = %v, want %v", r.Bag, want)
	}
}

func TestRun_UnknownVariableInBodyFailsStepNamingIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "unknown.md")
	writeFile(t, file, "```bash\necho \"uid=42\" >> \"$REHEARSE_CAPTURES\"\n```\n\n"+
		"```bash\necho {{nope}}\n```\n\n```bash\necho never\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[1].Detail, "unknown variable {{nope}}") {
		t.Errorf("step detail does not name the variable: %q", r.Steps[1].Detail)
	}
	if r.Steps[2].Status != StepStatusSkipped {
		t.Errorf("step after the failure = %q, want %q", r.Steps[2].Status, StepStatusSkipped)
	}
	// The bag's state so far is still reported on failure.
	if want := map[string]string{"uid": "42"}; !reflect.DeepEqual(r.Bag, want) {
		t.Errorf("bag = %v, want %v", r.Bag, want)
	}
}

func TestRun_InfoStringParamsAreInterpolated(t *testing.T) {
	var gotParams map[string]string
	reg := blocks.NewRegistry(
		bash.New(),
		fakeBlock{kind: "sql", run: func(ctx blocks.StepCtx) blocks.StepResult {
			gotParams = ctx.Params
			return blocks.StepResult{Status: blocks.StatusPass}
		}},
	)
	file := filepath.Join(t.TempDir(), "params.md")
	writeFile(t, file, "```bash\necho \"db=app.db\" >> \"$REHEARSE_CAPTURES\"\n```\n\n"+
		"```sql dsn=sqlite:{{db}}\nSELECT 1;\n```\n")

	r := Run(reg, []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps: %+v)", r.Status, r.Steps)
	}
	if gotParams["dsn"] != "sqlite:app.db" {
		t.Errorf("dsn param = %q, want sqlite:app.db", gotParams["dsn"])
	}
}

func TestRun_UnknownVariableInParamFailsStepNamingIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "param-unknown.md")
	writeFile(t, file, "```sql dsn=sqlite:{{nodb}}\nSELECT 1;\n```\n")

	executed := false
	reg := blocks.NewRegistry(fakeBlock{kind: "sql", run: func(blocks.StepCtx) blocks.StepResult {
		executed = true
		return blocks.StepResult{Status: blocks.StatusPass}
	}})
	r := Run(reg, []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[0].Detail, "unknown variable {{nodb}}") {
		t.Errorf("step detail does not name the variable: %q", r.Steps[0].Detail)
	}
	if executed {
		t.Error("executor ran despite the interpolation failure")
	}
}

// TestRun_HurlDerivedBlocksGetVarsNotTextInterpolation pins the Task 5 seam:
// a non-interpolated kind keeps {{name}} verbatim in its body and receives
// the bag as ordered StepCtx.Vars for --variable flags (REQ: context-bag).
func TestRun_HurlDerivedBlocksGetVarsNotTextInterpolation(t *testing.T) {
	var gotCtx blocks.StepCtx
	reg := blocks.NewRegistry(
		bash.New(),
		fakeBlock{kind: "hurl", run: func(ctx blocks.StepCtx) blocks.StepResult {
			gotCtx = ctx
			return blocks.StepResult{Status: blocks.StatusPass, Captures: []blocks.Capture{{Name: "token", Value: "t-1"}}}
		}},
	)
	file := filepath.Join(t.TempDir(), "hurl.md")
	writeFile(t, file, "```bash\necho \"uid=42\" >> \"$REHEARSE_CAPTURES\"\necho \"name=alice\" >> \"$REHEARSE_CAPTURES\"\n```\n\n"+
		"```hurl\nGET http://x/{{uid}}\n```\n")

	r := Run(reg, []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps: %+v)", r.Status, r.Steps)
	}
	if !strings.Contains(gotCtx.Body, "{{uid}}") {
		t.Errorf("hurl body was interpolated: %q", gotCtx.Body)
	}
	wantVars := []blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}}
	if !reflect.DeepEqual(gotCtx.Vars, wantVars) {
		t.Errorf("vars = %v, want %v", gotCtx.Vars, wantVars)
	}
	// The hurl step's own captures merge into the bag too.
	if want := map[string]string{"uid": "42", "name": "alice", "token": "t-1"}; !reflect.DeepEqual(r.Bag, want) {
		t.Errorf("bag = %v, want %v", r.Bag, want)
	}
}
