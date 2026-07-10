// Package manifests is the dependency-manifest ingestion adapter: it parses
// `go.mod` and `package.json` files at the repo root plus one nested level
// (e.g. frontend/, backend/, landings*/) into `derived` facts.
//
// Feature: cli/studio/index (REQ: adapter-manifests)
//
// Emitted predicates:
//   - publishes — the module/package identity a manifest declares (go.mod
//     `module` directive; package.json `name` + `version`)
//   - consumes  — one fact per direct dependency with its version specifier
//     (go.mod non-indirect `require` lines; package.json `dependencies` and
//     `devDependencies`)
//
// Subjects are emitted repo-relative (`#<manifest-path>`, e.g. `#go.mod`,
// `#frontend/package.json`); the pipeline prefixes them with the
// workspace-scoped repo slug. Objects are package IDs (`module@version`,
// version omitted when the manifest declares none). Dot-directories and
// node_modules are never scanned.
package manifests

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	adapterID = "manifests"
	// adapterVersion bumps when the emitted fact vocabulary or shapes change.
	adapterVersion = "0.1.0"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	osReadDirFn  = os.ReadDir
	osReadFileFn = os.ReadFile
)

// Adapter is the dependency-manifest ingestion adapter. The zero value is
// ready to use.
type Adapter struct{}

// New returns the adapter.
func New() Adapter { return Adapter{} }

// ID returns the stable adapter id stamped into every emitted fact.
func (Adapter) ID() string { return adapterID }

// Version returns the adapter version stamped into every emitted fact.
func (Adapter) Version() string { return adapterVersion }

// Ingest parses the manifests of the repo at repoPath: go.mod and
// package.json at the repo root and in each immediate subdirectory. A
// missing manifest is not an error; an unreadable or malformed one becomes a
// warning and skips only that file (REQ: partial-tolerance).
func (Adapter) Ingest(repoPath string) ([]fact.Fact, []fact.Warning) {
	var warnings []fact.Warning
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fact.Warning{Message: fmt.Sprintf(format, args...)})
	}

	entries, err := osReadDirFn(repoPath)
	if err != nil {
		warnf("reading repo %s: %v", repoPath, err)
		return nil, warnings
	}
	dirs := []string{""} // repo root first, then one nested level
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "node_modules" {
			continue
		}
		dirs = append(dirs, e.Name()) // os.ReadDir sorts by name: deterministic
	}

	var facts []fact.Fact
	for _, dir := range dirs {
		facts = append(facts, ingestGoMod(repoPath, dir, warnf)...)
		facts = append(facts, ingestPackageJSON(repoPath, dir, warnf)...)
	}
	return facts, warnings
}

// derived builds a derived-class fact; adapter/observed-at/ecosystem are
// stamped centrally by the pipeline.
func derived(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.Derived, Pointer: pointer},
	}
}

// readManifest reads the manifest named base in dir (repo-relative, "" =
// root). A missing file returns ok=false silently; a read failure warns.
func readManifest(repoPath, dir, base string, warnf func(string, ...any)) (rel string, data []byte, ok bool) {
	rel = path.Join(dir, base) // dir "" → just base
	data, err := osReadFileFn(filepath.Join(repoPath, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return rel, nil, false
	}
	if err != nil {
		warnf("reading %s: %v", rel, err)
		return rel, nil, false
	}
	return rel, data, true
}

// ingestGoMod emits publishes + consumes facts for the go.mod in dir, if any.
// Indirect requirements are not direct dependencies and are skipped.
func ingestGoMod(repoPath, dir string, warnf func(string, ...any)) []fact.Fact {
	rel, data, ok := readManifest(repoPath, dir, "go.mod", warnf)
	if !ok {
		return nil
	}
	f, err := modfile.Parse(rel, data, nil)
	if err != nil {
		warnf("parsing %s: %v", rel, err)
		return nil
	}
	subject := "#" + rel
	var facts []fact.Fact
	if f.Module != nil {
		facts = append(facts, derived(subject, "publishes", fact.PackageID(f.Module.Mod.Path, ""), rel))
	}
	for _, r := range f.Require {
		if r.Indirect {
			continue
		}
		facts = append(facts, derived(subject, "consumes", fact.PackageID(r.Mod.Path, r.Mod.Version), rel))
	}
	return facts
}

// packageJSON is the subset of package.json the adapter reads.
type packageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// ingestPackageJSON emits publishes + consumes facts for the package.json in
// dir, if any. Dependencies and devDependencies are both direct dependencies;
// each keeps its version specifier verbatim (e.g. `left-pad@^1.3.0`).
func ingestPackageJSON(repoPath, dir string, warnf func(string, ...any)) []fact.Fact {
	rel, data, ok := readManifest(repoPath, dir, "package.json", warnf)
	if !ok {
		return nil
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		warnf("parsing %s: %v", rel, err)
		return nil
	}
	subject := "#" + rel
	var facts []fact.Fact
	if pkg.Name != "" {
		facts = append(facts, derived(subject, "publishes", fact.PackageID(pkg.Name, pkg.Version), rel))
	}
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for _, name := range sortedKeys(deps) {
			facts = append(facts, derived(subject, "consumes", fact.PackageID(name, deps[name]), rel))
		}
	}
	return facts
}

// sortedKeys returns the map's keys in sorted order (JSON maps are unordered;
// emission must be deterministic).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
