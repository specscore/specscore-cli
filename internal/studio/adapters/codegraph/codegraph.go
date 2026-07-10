// Package codegraph is the CodeGrapher-snapshot ingestion adapter: it reads
// committed `codegraph/` snapshot recordsets (INGR encoding, inGitDB layout)
// and emits package entities and `derived` facts for `imports` edges at
// package granularity.
//
// Feature: cli/studio/index (REQ: adapter-codegraph)
//
// Emitted predicates:
//   - imports — one fact per distinct (importing package, imported package)
//     pair found in the snapshot's `imports` edges
//
// Snapshots usually store imports at file granularity (edge source
// `file:<path>`, target an `import:` node whose name is the import path);
// the adapter derives the package edge in the same single pass: the subject
// is the source file's directory as a repo-relative package (`#<dir>`, `#.`
// for the repo root) and the object is the imported path verbatim (a global
// package coordinate, matching the manifests adapter's object vocabulary).
// Where the snapshot already provides package-granularity nodes/edges
// (`package:` refs) they pass through as repo-relative packages (`#<name>`)
// with no extra parsing pass. Malformed or unreadable snapshot files become
// warnings and skip only that file (REQ: partial-tolerance); edges whose
// endpoints cannot be resolved to packages are counted into one warning per
// edges file.
package codegraph

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	adapterID = "codegraph"
	// adapterVersion bumps when the emitted fact vocabulary or shapes change.
	adapterVersion = "0.1.0"
	// snapshotDir is the committed CodeGrapher export directory at a repo root.
	snapshotDir = "codegraph"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	osReadDirFn  = os.ReadDir
	osReadFileFn = os.ReadFile
)

// Adapter is the codegraph-snapshot ingestion adapter. The zero value is
// ready to use.
type Adapter struct{}

// New returns the adapter.
func New() Adapter { return Adapter{} }

// ID returns the stable adapter id stamped into every emitted fact.
func (Adapter) ID() string { return adapterID }

// Version returns the adapter version stamped into every emitted fact.
func (Adapter) Version() string { return adapterVersion }

// Ingest reads the committed codegraph/ snapshot of the repo at repoPath. A
// repo without a snapshot yields no facts and no warnings. Malformed or
// unreadable recordset files become warnings and skip only that file
// (REQ: partial-tolerance).
func (Adapter) Ingest(repoPath string) ([]fact.Fact, []fact.Warning) {
	var warnings []fact.Warning
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fact.Warning{Message: fmt.Sprintf(format, args...)})
	}

	nodes := loadNodes(repoPath, warnf)

	var facts []fact.Fact
	seen := map[string]bool{} // dedupe across all edges files
	for _, rel := range listRecordsets(repoPath, "edges", warnf) {
		facts = append(facts, ingestEdges(repoPath, rel, nodes, seen, warnf)...)
	}
	return facts, warnings
}

// node is the slice of a snapshot node record the adapter needs.
type node struct {
	kind     string
	name     string
	filePath string
}

// listRecordsets returns the repo-relative slash paths of the .ingr files in
// codegraph/<sub>, sorted by name. A missing directory is not an error (the
// repo has no snapshot, or an incomplete one with nothing to derive).
func listRecordsets(repoPath, sub string, warnf func(string, ...any)) []string {
	rel := path.Join(snapshotDir, sub)
	entries, err := osReadDirFn(filepath.Join(repoPath, snapshotDir, sub))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		warnf("reading %s: %v", rel, err)
		return nil
	}
	var out []string
	for _, e := range entries { // os.ReadDir sorts by name: deterministic
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ingr") {
			continue
		}
		out = append(out, path.Join(rel, e.Name()))
	}
	return out
}

// readRecordset reads and parses one .ingr file, requiring the named fields.
// Any failure warns and returns nil (skip file).
func readRecordset(repoPath, rel string, required []string, warnf func(string, ...any)) *recordset {
	data, err := osReadFileFn(filepath.Join(repoPath, filepath.FromSlash(rel)))
	if err != nil {
		warnf("reading %s: %v", rel, err)
		return nil
	}
	rs, err := parseRecordset(data)
	if err == nil {
		err = rs.require(required...)
	}
	if err != nil {
		warnf("parsing %s: %v", rel, err)
		return nil
	}
	return rs
}

// loadNodes reads every nodes recordset into one id → node map.
func loadNodes(repoPath string, warnf func(string, ...any)) map[string]node {
	nodes := map[string]node{}
	for _, rel := range listRecordsets(repoPath, "nodes", warnf) {
		rs := readRecordset(repoPath, rel, []string{"$ID", "kind", "name", "file_path"}, warnf)
		if rs == nil {
			continue
		}
		for rec := 0; rec < rs.records(); rec++ {
			id, ok := rs.stringField(rec, "$ID")
			if !ok || id == "" {
				continue // unidentifiable record; edges cannot reference it
			}
			kind, _ := rs.stringField(rec, "kind")
			name, _ := rs.stringField(rec, "name")
			filePath, _ := rs.stringField(rec, "file_path")
			nodes[id] = node{kind: kind, name: name, filePath: filePath}
		}
	}
	return nodes
}

// ingestEdges emits one derived imports fact per distinct package pair found
// in the edges recordset at rel. Edges whose endpoints cannot be resolved to
// packages are skipped and summarized in a single warning for the file.
func ingestEdges(repoPath, rel string, nodes map[string]node, seen map[string]bool, warnf func(string, ...any)) []fact.Fact {
	rs := readRecordset(repoPath, rel, []string{"source", "target", "kind"}, warnf)
	if rs == nil {
		return nil
	}
	var facts []fact.Fact
	total, skipped := 0, 0
	for rec := 0; rec < rs.records(); rec++ {
		if kind, _ := rs.stringField(rec, "kind"); kind != "imports" {
			continue
		}
		total++
		source, _ := rs.stringField(rec, "source")
		target, _ := rs.stringField(rec, "target")
		subject, okS := sourcePackage(source, nodes)
		object, okT := targetPackage(target, nodes)
		if !okS || !okT {
			skipped++
			continue
		}
		key := subject + "\x00" + object
		if seen[key] {
			continue
		}
		seen[key] = true
		facts = append(facts, fact.Fact{
			Subject:   subject,
			Predicate: "imports",
			Object:    object,
			Evidence:  fact.Evidence{Class: fact.Derived, Pointer: rel},
		})
	}
	if skipped > 0 {
		warnf("skipped %d of %d imports edges with unresolvable endpoints in %s", skipped, total, rel)
	}
	return facts
}

// sourcePackage resolves an edge source reference to the importing package:
// repo-relative (`#<dir>` from the source file's directory, `#.` for the repo
// root, or `#<name>` for a snapshot-provided package node).
func sourcePackage(source string, nodes map[string]node) (string, bool) {
	if p, ok := strings.CutPrefix(source, "file:"); ok && p != "" {
		return "#" + path.Dir(p), true
	}
	n, ok := nodes[source]
	switch {
	case !ok:
		return "", false
	case n.kind == "package" && n.name != "":
		return "#" + n.name, true
	case n.filePath != "": // symbol-level source: its file's directory
		return "#" + path.Dir(n.filePath), true
	}
	return "", false
}

// targetPackage resolves an edge target reference to the imported package:
// an import node's name verbatim (a global import path), or a
// snapshot-provided package node as a repo-relative package (`#<name>`).
func targetPackage(target string, nodes map[string]node) (string, bool) {
	n, ok := nodes[target]
	switch {
	case !ok || n.name == "":
		return "", false
	case n.kind == "import":
		return n.name, true
	case n.kind == "package":
		return "#" + n.name, true
	}
	return "", false
}

// --- INGR recordset decoding ---

// recordset is one parsed INGR file: the header's field layout plus the raw
// JSON value lines (one field per line), decoded lazily per needed field.
type recordset struct {
	index map[string]int // field name → position within a record
	n     int            // fields per record
	lines []string       // JSON value lines; len is a multiple of n
}

// parseRecordset decodes the INGR framing: a fixed first line
// `# INGR.io | <name>: <field>[, <field>...]` (types suffixed `:<type>`),
// then one JSON value per line, one record per <n> lines. Comment lines
// (`# ...`, e.g. the trailing record count) and blank lines are skipped —
// JSON values never start with `#`.
func parseRecordset(data []byte) (*recordset, error) {
	lines := strings.Split(string(data), "\n")
	header := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(header, "# INGR.io") {
		return nil, errors.New("not an INGR recordset (missing `# INGR.io` header)")
	}
	_, after, ok := strings.Cut(header, "|")
	if ok {
		_, after, ok = strings.Cut(after, ":")
	}
	if !ok {
		return nil, errors.New("malformed INGR header (want `# INGR.io | <name>: <fields>`)")
	}
	rs := &recordset{index: map[string]int{}}
	for _, f := range strings.Split(after, ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(f), ":") // strip `:<type>`
		if name != "" {
			rs.index[name] = rs.n
			rs.n++
		}
	}
	if rs.n == 0 {
		return nil, errors.New("INGR header declares no fields")
	}
	for _, line := range lines[1:] {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		rs.lines = append(rs.lines, t)
	}
	if len(rs.lines)%rs.n != 0 {
		return nil, fmt.Errorf("%d value lines is not a multiple of %d fields (truncated record?)", len(rs.lines), rs.n)
	}
	return rs, nil
}

// require verifies the header declares every named field.
func (rs *recordset) require(fields ...string) error {
	var missing []string
	for _, f := range fields {
		if _, ok := rs.index[f]; !ok {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("header lacks required field(s) %s", strings.Join(missing, ", "))
	}
	return nil
}

// records returns the number of records in the set.
func (rs *recordset) records() int { return len(rs.lines) / rs.n }

// stringField decodes field name of record rec as a string; ok is false when
// the value is absent, null, or not a JSON string (the record is then
// unresolvable rather than the whole file malformed).
func (rs *recordset) stringField(rec int, name string) (string, bool) {
	var s string
	if err := json.Unmarshal([]byte(rs.lines[rec*rs.n+rs.index[name]]), &s); err != nil {
		return "", false
	}
	return s, true
}
