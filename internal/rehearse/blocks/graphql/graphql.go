// Package graphql implements the ```graphql rehearse step block: a GraphQL
// query with `url=<endpoint>` in the info string, an optional `-- variables:
// {...}` directive, plus `-- assert-jsonpath: <path> == <json-value>` and
// `-- capture-jsonpath: <name> = <path>` directives. The block is compiled
// onto a generated Hurl document — a POST of the JSON body {query,variables}
// with `HTTP 200` asserted and the jsonpath asserts/captures in Hurl syntax —
// and executed via the hurl package's delegation, inheriting its
// missing-binary skip semantics and context-bag handling (REQ: graphql-block,
// REQ: context-bag).
package graphql

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/directives"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/hurl"
)

// Executor runs graphql step blocks.
type Executor struct{}

// New returns the graphql block executor.
func New() *Executor { return &Executor{} }

// Kind returns "graphql".
func (*Executor) Kind() string { return "graphql" }

// RequiredBinary names the external binary the compiled document is
// delegated to, feeding the runner's upfront missing-binary scan — a
// scenario with a graphql block skips exactly like one with a hurl block
// when hurl is not on PATH (REQ: graphql-block).
func (*Executor) RequiredBinary() string { return hurl.Binary }

// Run compiles the block onto a Hurl document and delegates it to the hurl
// engine. The context bag flows through untouched as --variable flags — the
// graphql body gets no textual interpolation (REQ: context-bag).
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	doc, err := Compile(ctx.Params["url"], ctx.Body)
	if err != nil {
		return blocks.StepResult{Status: blocks.StatusFail, Detail: fmt.Sprintf("graphql step failed: %v", err)}
	}
	output, captures, err := hurl.Execute(ctx.WorkDir, doc, ctx.Vars)
	if err != nil {
		return blocks.StepResult{
			Status: blocks.StatusFail,
			Detail: fmt.Sprintf("graphql step failed: %v", err),
			Output: blocks.Truncate(output),
		}
	}
	return blocks.StepResult{
		Status:   blocks.StatusPass,
		Output:   blocks.Truncate(output),
		Captures: captures,
	}
}

// jsonpathAssert is one parsed `-- assert-jsonpath: <path> == <json-value>`
// directive; Literal is the value already rendered in Hurl predicate syntax.
type jsonpathAssert struct {
	Path    string
	Literal string
}

// jsonpathCapture is one parsed `-- capture-jsonpath: <name> = <path>`
// directive.
type jsonpathCapture struct {
	Name string
	Path string
}

// blockSpec is the parsed directive set of one graphql block.
type blockSpec struct {
	// Variables is the `-- variables:` JSON object; nil when absent (the
	// POST body then omits the variables key).
	Variables map[string]any
	Asserts   []jsonpathAssert
	Captures  []jsonpathCapture
}

// Compile turns a graphql block (endpoint URL + query + directives) into a
// Hurl document: POST of the JSON body {query,variables}, `HTTP 200`
// asserted, jsonpath captures/asserts in Hurl syntax (REQ: graphql-block).
func Compile(url, body string) (string, error) {
	if url == "" {
		return "", errors.New("graphql block requires url=<endpoint> in its info string (e.g. ```graphql url=http://localhost:8080/graphql)")
	}
	query, dirs := directives.Split(body)
	query = strings.TrimSpace(query)
	if query == "" {
		return "", errors.New("graphql block contains no query")
	}
	spec, err := parseDirectives(dirs)
	if err != nil {
		return "", err
	}

	payload := map[string]any{"query": query}
	if spec.Variables != nil {
		payload["variables"] = spec.Variables
	}

	var b strings.Builder
	b.WriteString("POST " + url + "\n")
	b.WriteString(encodeBody(payload) + "\n")
	b.WriteString("HTTP 200\n")
	if len(spec.Captures) > 0 {
		b.WriteString("[Captures]\n")
		for _, c := range spec.Captures {
			fmt.Fprintf(&b, "%s: jsonpath %s\n", c.Name, hurlString(c.Path))
		}
	}
	if len(spec.Asserts) > 0 {
		b.WriteString("[Asserts]\n")
		for _, a := range spec.Asserts {
			fmt.Fprintf(&b, "jsonpath %s == %s\n", hurlString(a.Path), a.Literal)
		}
	}
	return b.String(), nil
}

// parseDirectives interprets the block's trailing directives. Unknown keys
// are an error so typos fail loudly instead of silently skipping an
// assertion (house rule shared with the sql/dtql directive parser).
func parseDirectives(dirs []directives.Directive) (blockSpec, error) {
	var spec blockSpec
	for _, d := range dirs {
		switch d.Key {
		case "variables":
			obj := map[string]any{}
			if err := json.Unmarshal([]byte(d.Value), &obj); err != nil {
				return blockSpec{}, fmt.Errorf("-- variables: %q is not a JSON object: %v", d.Value, err)
			}
			spec.Variables = obj
		case "assert-jsonpath":
			path, value, ok := strings.Cut(d.Value, "==")
			path, value = strings.TrimSpace(path), strings.TrimSpace(value)
			if !ok || path == "" || value == "" {
				return blockSpec{}, fmt.Errorf("-- assert-jsonpath: %q does not match `<path> == <json-value>`", d.Value)
			}
			literal, err := hurlLiteral(value)
			if err != nil {
				return blockSpec{}, err
			}
			spec.Asserts = append(spec.Asserts, jsonpathAssert{Path: path, Literal: literal})
		case "capture-jsonpath":
			name, path, ok := strings.Cut(d.Value, "=")
			name, path = strings.TrimSpace(name), strings.TrimSpace(path)
			if !ok || name == "" || path == "" {
				return blockSpec{}, fmt.Errorf("-- capture-jsonpath: %q does not match `<name> = <path>`", d.Value)
			}
			spec.Captures = append(spec.Captures, jsonpathCapture{Name: name, Path: path})
		default:
			return blockSpec{}, fmt.Errorf("unknown directive -- %s: (supported: variables, assert-jsonpath, capture-jsonpath)", d.Key)
		}
	}
	return spec, nil
}

// encodeBody renders the POST payload as one-line JSON without HTML
// escaping, so GraphQL operators (`<`, `>`, `&`) stay verbatim in the query.
// The payload is always JSON-safe (a query string plus a JSON-decoded
// variables object), so encoding cannot fail.
func encodeBody(payload map[string]any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
	return strings.TrimRight(buf.String(), "\n")
}

// hurlLiteral renders one JSON value in Hurl predicate syntax. v0.3 supports
// JSON scalars — Hurl's jsonpath equality predicates take scalar values.
func hurlLiteral(jsonValue string) (string, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(jsonValue))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("assert-jsonpath value %q is not valid JSON: %v", jsonValue, err)
	}
	switch t := v.(type) {
	case string:
		return hurlString(t), nil
	case json.Number:
		return t.String(), nil
	case bool:
		return strconv.FormatBool(t), nil
	case nil:
		return "null", nil
	default:
		return "", fmt.Errorf("assert-jsonpath value %s must be a JSON scalar (string, number, boolean or null)", jsonValue)
	}
}

// hurlString renders a Hurl quoted string. Hurl's quoted-string escapes
// match JSON's, so the JSON encoding of the string is valid Hurl.
func hurlString(s string) string {
	// Marshalling a plain string cannot fail.
	data, _ := json.Marshal(s)
	return string(data)
}
