package cli

// Feature: cli/studio/answers (REQ: benchmark-file)
// Verifies: cli/studio/answers#ac:benchmark-file-has-50
//
// The benchmark composition check as a Go unit test (Testing Strategy: "The
// benchmark's composition check (table ↔ questions.jsonl) is a unit test"). It
// parses the committed benchmark/questions.jsonl and asserts it holds exactly 50
// well-formed instances whose per-template counts match the `## Benchmark
// composition` table (41 answerable / 9 expected-unanswerable). The runner
// (benchmark/run.sh) enforces the same invariant at runtime; this test guards it
// in the Go suite so a drift fails `go test` before CI ever runs the corpus.

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// benchmarkComposition is the authoritative `## Benchmark composition` table:
// answerable instances per template. The answerable total is 41; the file also
// carries 9 expected-unanswerable instances (empty template), summing to 50.
var benchmarkComposition = map[string]int{
	"who-fronts":           4,
	"what-repos-implement": 4,
	"status-of":            5,
	"aliases-of":           3,
	"member-of":            3,
	"is-it-live":           4,
	"ci-status-of":         3,
	"what-verifies":        3,
	"contradictions-for":   3,
	"freshness-of":         2,
	"what-uses":            3,
	"version-pins":         2,
	"aliases-resolve":      2,
}

const (
	benchmarkAnswerable   = 41
	benchmarkUnanswerable = 9
	benchmarkTotal        = 50
)

type benchmarkInstance struct {
	ID          string `json:"id"`
	Question    string `json:"question"`
	Template    string `json:"template"`
	Parameter   string `json:"parameter"`
	Expectation string `json:"expectation"`
}

// benchmarkQuestionsPath locates the committed questions.jsonl relative to this
// test file, so the test works from any working directory.
func benchmarkQuestionsPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/cli/ -> repo root is two dirs up.
	root := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(root, "spec", "features", "cli", "studio", "answers",
		"benchmark", "questions.jsonl")
}

func TestBenchmarkComposition_Has50MatchingTheTable(t *testing.T) {
	path := benchmarkQuestionsPath(t)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	documented := map[string]bool{"": true}
	for tmpl := range benchmarkComposition {
		documented[tmpl] = true
	}

	byTemplate := map[string]int{}
	exp := map[string]int{}
	ids := map[string]bool{}

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		text := sc.Text()
		if text == "" {
			continue
		}
		line++
		var inst benchmarkInstance
		if err := json.Unmarshal([]byte(text), &inst); err != nil {
			t.Fatalf("line %d: not valid JSON: %v", line, err)
		}
		if inst.ID == "" || inst.Question == "" || inst.Parameter == "" {
			t.Fatalf("line %d: missing id/question/parameter: %+v", line, inst)
		}
		if ids[inst.ID] {
			t.Fatalf("line %d: duplicate id %q", line, inst.ID)
		}
		ids[inst.ID] = true
		if !documented[inst.Template] {
			t.Fatalf("line %d: template %q is not one of the documented templates", line, inst.Template)
		}
		switch inst.Expectation {
		case "answerable":
			if inst.Template == "" {
				t.Fatalf("line %d: answerable instance has an empty template", line)
			}
			byTemplate[inst.Template]++
		case "expected-unanswerable":
			if inst.Template != "" {
				t.Fatalf("line %d: expected-unanswerable instance names a template %q", line, inst.Template)
			}
		default:
			t.Fatalf("line %d: bad expectation %q", line, inst.Expectation)
		}
		exp[inst.Expectation]++
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanning %s: %v", path, err)
	}

	if line != benchmarkTotal {
		t.Fatalf("instance count = %d, want %d", line, benchmarkTotal)
	}
	if exp["answerable"] != benchmarkAnswerable {
		t.Errorf("answerable count = %d, want %d", exp["answerable"], benchmarkAnswerable)
	}
	if exp["expected-unanswerable"] != benchmarkUnanswerable {
		t.Errorf("expected-unanswerable count = %d, want %d",
			exp["expected-unanswerable"], benchmarkUnanswerable)
	}
	for tmpl, want := range benchmarkComposition {
		if got := byTemplate[tmpl]; got != want {
			t.Errorf("template %q: file has %d, table wants %d", tmpl, got, want)
		}
	}
	for tmpl := range byTemplate {
		if _, ok := benchmarkComposition[tmpl]; !ok {
			t.Errorf("file has undocumented template %q", tmpl)
		}
	}
}
