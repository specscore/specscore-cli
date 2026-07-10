package graphql

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

func TestExecutor_KindAndRequiredBinary(t *testing.T) {
	e := New()
	if e.Kind() != "graphql" {
		t.Errorf("Kind() = %q, want graphql", e.Kind())
	}
	if e.RequiredBinary() != "hurl" {
		t.Errorf("RequiredBinary() = %q, want hurl (the compiled document delegates to it)", e.RequiredBinary())
	}
}

// TestCompile_FullBlock pins the exact generated Hurl document: POST with the
// {query,variables} JSON body, HTTP 200, [Captures] then [Asserts] in Hurl
// syntax (REQ: graphql-block).
func TestCompile_FullBlock(t *testing.T) {
	doc, err := Compile("http://127.0.0.1:8080/graphql",
		"query User($id: ID!) { user(id: $id) { name } }\n"+
			"-- variables: {\"id\": \"42\"}\n"+
			"-- capture-jsonpath: userName = $.data.user.name\n"+
			"-- assert-jsonpath: $.data.user.name == \"alice\"\n"+
			"-- assert-jsonpath: $.data.ok == true\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "POST http://127.0.0.1:8080/graphql\n" +
		`{"query":"query User($id: ID!) { user(id: $id) { name } }","variables":{"id":"42"}}` + "\n" +
		"HTTP 200\n" +
		"[Captures]\n" +
		"userName: jsonpath \"$.data.user.name\"\n" +
		"[Asserts]\n" +
		"jsonpath \"$.data.user.name\" == \"alice\"\n" +
		"jsonpath \"$.data.ok\" == true\n"
	if doc != want {
		t.Errorf("compiled document:\n%s\nwant:\n%s", doc, want)
	}
}

// TestCompile_MinimalBlock: no directives → just the POST (variables key
// omitted) and the HTTP 200 assert.
func TestCompile_MinimalBlock(t *testing.T) {
	doc, err := Compile("http://h/graphql", "query { ok }\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "POST http://h/graphql\n{\"query\":\"query { ok }\"}\nHTTP 200\n"
	if doc != want {
		t.Errorf("compiled document:\n%s\nwant:\n%s", doc, want)
	}
}

// TestCompile_NoHTMLEscaping: GraphQL operators stay verbatim in the body.
func TestCompile_NoHTMLEscaping(t *testing.T) {
	doc, err := Compile("http://h/graphql", "query { items(filter: \"a<b&c>d\") }\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, `a<b&c>d`) {
		t.Errorf("the query's <, > or & was escaped:\n%s", doc)
	}
}

func TestCompile_AssertValueScalars(t *testing.T) {
	cases := []struct{ value, literal string }{
		{"true", "true"},
		{"false", "false"},
		{"42", "42"},
		{"4.5", "4.5"},
		{`"text"`, `"text"`},
		{"null", "null"},
	}
	for _, c := range cases {
		doc, err := Compile("http://h/g", "query { ok }\n-- assert-jsonpath: $.x == "+c.value+"\n")
		if err != nil {
			t.Errorf("value %s: %v", c.value, err)
			continue
		}
		if want := "jsonpath \"$.x\" == " + c.literal + "\n"; !strings.Contains(doc, want) {
			t.Errorf("value %s: document lacks %q:\n%s", c.value, want, doc)
		}
	}
}

func TestCompile_Errors(t *testing.T) {
	cases := []struct {
		name, url, body, wantErr string
	}{
		{"missing url", "", "query { ok }\n", "requires url=<endpoint>"},
		{"empty query", "http://h/g", "\n-- assert-jsonpath: $.x == true\n", "contains no query"},
		{"bad variables", "http://h/g", "query { ok }\n-- variables: [1,2]\n", "is not a JSON object"},
		{"assert without ==", "http://h/g", "query { ok }\n-- assert-jsonpath: $.x true\n", "does not match `<path> == <json-value>`"},
		{"assert empty path", "http://h/g", "query { ok }\n-- assert-jsonpath: == true\n", "does not match `<path> == <json-value>`"},
		{"assert invalid json value", "http://h/g", "query { ok }\n-- assert-jsonpath: $.x == truthy\n", "is not valid JSON"},
		{"assert composite value", "http://h/g", "query { ok }\n-- assert-jsonpath: $.x == {\"a\":1}\n", "must be a JSON scalar"},
		{"capture without =", "http://h/g", "query { ok }\n-- capture-jsonpath: name $.x\n", "does not match `<name> = <path>`"},
		{"capture empty name", "http://h/g", "query { ok }\n-- capture-jsonpath: = $.x\n", "does not match `<name> = <path>`"},
		{"unknown directive", "http://h/g", "query { ok }\n-- assert-rows: 1\n", "unknown directive -- assert-rows"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Compile(c.url, c.body)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("err = %v, want it to contain %q", err, c.wantErr)
			}
		})
	}
}

// fakeHurl installs a fake `hurl` binary as the only entry on PATH. It
// answers the capability probe with --report-json support, copies the .hurl
// document it received to docFile, writes $FAKE_REPORT_JSON as the report and
// exits $FAKE_EXIT.
func fakeHurl(t *testing.T) (docFile string) {
	t.Helper()
	dir := t.TempDir()
	docFile = filepath.Join(dir, "doc.hurl")
	script := `#!/bin/bash
if [ "$1" = "--help" ]; then echo "--report-json <DIR>"; exit 0; fi
prev=""
report=""
for a in "$@"; do
  if [ "$prev" = "--report-json" ]; then report="$a"; fi
  prev="$a"
done
last="${@: -1}"
while IFS= read -r line || [ -n "$line" ]; do printf '%s\n' "$line"; done < "$last" > "` + docFile + `"
printf '%s' "$FAKE_REPORT_JSON" > "$report/report.json"
exit "${FAKE_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(dir, "hurl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_REPORT_JSON", `[{"entries":[{"captures":[]}]}]`)
	t.Setenv("FAKE_EXIT", "0")
	return docFile
}

// TestRun_CompilesAndDelegatesToHurl covers the composition seam: the
// executor hands the compiled document to the hurl engine and merges the
// report's captures back — the capture-jsonpath flow (REQ: graphql-block,
// REQ: context-bag).
func TestRun_CompilesAndDelegatesToHurl(t *testing.T) {
	docFile := fakeHurl(t)
	t.Setenv("FAKE_REPORT_JSON", `[{"entries":[{"captures":[{"name":"userName","value":"alice"}]}]}]`)

	res := New().Run(blocks.StepCtx{
		WorkDir: t.TempDir(),
		Params:  map[string]string{"url": "http://127.0.0.1:9/graphql"},
		Body: "query { ok }\n" +
			"-- capture-jsonpath: userName = $.data.user.name\n" +
			"-- assert-jsonpath: $.data.ok == true\n",
		Vars: []blocks.Capture{{Name: "uid", Value: "42"}},
	})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if want := []blocks.Capture{{Name: "userName", Value: "alice"}}; !reflect.DeepEqual(res.Captures, want) {
		t.Errorf("captures = %v, want %v", res.Captures, want)
	}
	doc, err := os.ReadFile(docFile)
	if err != nil {
		t.Fatalf("the fake hurl never received a document: %v", err)
	}
	for _, want := range []string{
		"POST http://127.0.0.1:9/graphql",
		`{"query":"query { ok }"}`,
		"HTTP 200",
		"userName: jsonpath \"$.data.user.name\"",
		"jsonpath \"$.data.ok\" == true",
	} {
		if !strings.Contains(string(doc), want) {
			t.Errorf("delegated document lacks %q:\n%s", want, doc)
		}
	}
}

func TestRun_CompileErrorIsStepFail(t *testing.T) {
	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "query { ok }\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "graphql step failed") ||
		!strings.Contains(res.Detail, "requires url=<endpoint>") {
		t.Errorf("res = %+v, want a compile failure", res)
	}
}

func TestRun_HurlFailureIsStepFail(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_EXIT", "3")

	res := New().Run(blocks.StepCtx{
		WorkDir: t.TempDir(),
		Params:  map[string]string{"url": "http://127.0.0.1:9/graphql"},
		Body:    "query { ok }\n",
	})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "graphql step failed") {
		t.Errorf("res = %+v, want a delegation failure", res)
	}
}
