// Package registries is the ops-registry ingestion adapter: it parses
// well-known ops registry files — `domains.json` (Sneat-ops shape) and
// curated `ecosystem*.yaml` maps — into domain/product entities and
// `declared` facts.
//
// Feature: cli/studio/index (REQ: adapter-registries)
//
// Emitted predicates:
//   - serves-status  — a domain's observed HTTP status from domains.json
//     (the `/` probe when present, else the first probed path)
//   - fronts         — domain → product, from a curated ecosystem map (THE
//     canonical source); when a domain appears only in domains.json, a
//     best-effort fallback emits domain → Cloudflare worker name instead
//   - aliased-as     — product → brand name
//   - implemented-by — product → repo name
//   - member-of      — product → vertical
//
// Registry files are discovered at the repo root and under data/
// (domains.json, ecosystem*.yaml), plus any per-repo `registries:` paths
// configured in studio.yaml. Both parsers read a tolerant subset of the real
// shapes: unknown keys are ignored; a malformed file becomes a warning and
// skips only that file (REQ: partial-tolerance).
//
// Domains and products are GLOBAL coordinates, so subjects and objects are
// emitted without the `#` repo-relative marker — the pipeline's repo-slug
// prefixing never applies to them.
package registries

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

const (
	adapterID = "registries"
	// adapterVersion bumps when the emitted fact vocabulary or shapes change.
	adapterVersion = "0.1.0"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	osReadFileFn   = os.ReadFile
	filepathGlobFn = filepath.Glob
)

// Adapter is the ops-registry ingestion adapter.
type Adapter struct {
	// extra maps a resolved repo path (exactly as the pipeline passes it to
	// Ingest) to additional repo-relative registry file paths configured via
	// the per-repo `registries:` entry form in studio.yaml.
	extra map[string][]string
}

// New returns the adapter. extra is the per-repo configured registry file
// map (workspace.Workspace.ResolveRegistries); nil is fine.
func New(extra map[string][]string) Adapter { return Adapter{extra: extra} }

// ID returns the stable adapter id stamped into every emitted fact.
func (Adapter) ID() string { return adapterID }

// Version returns the adapter version stamped into every emitted fact.
func (Adapter) Version() string { return adapterVersion }

// Ingest parses the registry files of the repo at repoPath. Repos without
// registry files yield no facts and no warnings. Ecosystem maps are parsed
// before domains files so their curated domain→product `fronts` facts
// suppress the worker-name fallback for the domains they cover.
//
// The emitted facts are deduplicated on (subject, predicate, object,
// evidence_class): when two registry files agree on the same relationship —
// e.g. data/ecosystem.yaml and data/ecosystem-map.yaml both declaring a
// product's `fronts`/`aliased-as`/`member-of` — a single representative fact
// is kept, with the pointer of the first file in discovery order (see
// dedup). The fact model carries one pointer, so cross-source agreement (a
// confidence signal) collapses to one fact rather than N identical copies.
func (a Adapter) Ingest(repoPath string) ([]fact.Fact, []fact.Warning) {
	var warnings []fact.Warning
	warnf := func(format string, args ...any) {
		warnings = append(warnings, fact.Warning{Message: fmt.Sprintf(format, args...)})
	}

	files := discover(repoPath, a.extra[repoPath], warnf)
	var facts []fact.Fact
	curated := map[string]bool{} // domain IDs fronted by a curated product
	for _, rel := range files.ecosystem {
		facts = append(facts, ingestEcosystem(repoPath, rel, curated, warnf)...)
	}
	for _, rel := range files.domains {
		facts = append(facts, ingestDomains(repoPath, rel, curated, warnf)...)
	}
	return dedup(facts), warnings
}

// dedup collapses facts identical in (subject, predicate, object,
// evidence_class) to a single representative, keeping the first occurrence
// and preserving its evidence pointer. Facts are emitted in a stable source
// order — ecosystem maps (root before data/) before domains.json — so the
// surviving pointer is deterministic: when two registry files declare the
// same relationship, the first file in that order wins. Pure duplicates
// (identical pointer too) likewise collapse to one. Output order is the
// input order with later duplicates dropped.
func dedup(facts []fact.Fact) []fact.Fact {
	type key struct {
		subject, predicate, object string
		class                      fact.Class
	}
	seen := make(map[key]bool, len(facts))
	out := facts[:0]
	for _, f := range facts {
		k := key{f.Subject, f.Predicate, f.Object, f.Class}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// declared builds a declared-class fact; adapter/observed-at/ecosystem are
// stamped centrally by the pipeline.
func declared(subject, predicate, object, pointer string) fact.Fact {
	return fact.Fact{
		Subject:   subject,
		Predicate: predicate,
		Object:    object,
		Evidence:  fact.Evidence{Class: fact.Declared, Pointer: pointer},
	}
}

// registryFiles is the classified, deduplicated set of registry files to
// parse, as repo-relative slash paths in deterministic order.
type registryFiles struct {
	domains   []string // domains.json shape
	ecosystem []string // ecosystem map shape
}

// discover collects the registry files of a repo: domains.json and
// ecosystem*.yaml at the repo root and under data/ (only when present), plus
// the configured extras (always attempted, so a missing configured file
// warns at read time). Files classify by extension: .json → domains shape,
// .yaml/.yml → ecosystem shape; anything else warns and is skipped.
func discover(repoPath string, extra []string, warnf func(string, ...any)) registryFiles {
	var rf registryFiles
	seen := map[string]bool{}
	add := func(rel string) {
		rel = path.Clean(filepath.ToSlash(rel))
		if seen[rel] {
			return
		}
		seen[rel] = true
		switch path.Ext(rel) {
		case ".json":
			rf.domains = append(rf.domains, rel)
		case ".yaml", ".yml":
			rf.ecosystem = append(rf.ecosystem, rel)
		default:
			warnf("registry file %s: unrecognized type — want .json (domains) or .yaml (ecosystem map)", rel)
		}
	}
	for _, dir := range []string{"", "data"} {
		if rel := path.Join(dir, "domains.json"); fileExists(filepath.Join(repoPath, filepath.FromSlash(rel))) {
			add(rel)
		}
		matches, err := filepathGlobFn(filepath.Join(repoPath, filepath.FromSlash(dir), "ecosystem*.yaml"))
		if err != nil {
			warnf("scanning %s for ecosystem maps: %v", path.Join(dir, "ecosystem*.yaml"), err)
			continue
		}
		for _, m := range matches { // filepath.Glob sorts: deterministic
			add(path.Join(dir, filepath.Base(m)))
		}
	}
	for _, rel := range extra {
		add(rel)
	}
	return rf
}

// fileExists reports whether p names an existing regular file.
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// readRegistry reads the registry file rel (repo-relative slash path); any
// failure — including a configured file that does not exist — warns and
// skips the file.
func readRegistry(repoPath, rel string, warnf func(string, ...any)) ([]byte, bool) {
	data, err := osReadFileFn(filepath.Join(repoPath, filepath.FromSlash(rel)))
	if err != nil {
		warnf("reading %s: %v", rel, err)
		return nil, false
	}
	return data, true
}

// --- domains.json (Sneat-ops shape) ---

// domainsFile is the tolerant subset of domains.json the adapter reads:
// {"domains": {"<fqdn>": {"status": {"<path>": {"http_status": N}},
// "cloudflare": {"workers": {"<worker>": "<route>"}}}}}.
type domainsFile struct {
	Domains map[string]domainEntry `json:"domains"`
}

type domainEntry struct {
	Status     map[string]statusEntry `json:"status"`
	Cloudflare struct {
		Workers map[string]string `json:"workers"`
	} `json:"cloudflare"`
}

type statusEntry struct {
	HTTPStatus int `json:"http_status"`
}

// ingestDomains emits serves-status facts per domain, plus fallback fronts
// facts (object = Cloudflare worker name) for domains no curated ecosystem
// map covers.
func ingestDomains(repoPath, rel string, curated map[string]bool, warnf func(string, ...any)) []fact.Fact {
	data, ok := readRegistry(repoPath, rel, warnf)
	if !ok {
		return nil
	}
	var df domainsFile
	if err := json.Unmarshal(data, &df); err != nil {
		warnf("parsing %s: %v", rel, errOneLine(err))
		return nil
	}
	var facts []fact.Fact
	for _, fqdn := range sortedKeys(df.Domains) {
		entry := df.Domains[fqdn]
		id := fact.DomainID(fqdn)
		if status, ok := pickStatus(entry.Status); ok {
			facts = append(facts, declared(id, "serves-status", strconv.Itoa(status), rel))
		}
		if curated[id] {
			continue // the curated map already says what this domain fronts
		}
		for _, worker := range sortedKeys(entry.Cloudflare.Workers) {
			facts = append(facts, declared(id, "fronts", worker, rel))
		}
	}
	return facts
}

// pickStatus selects the domain's representative HTTP status: the "/" probe
// when present, else the first probed path in sorted order.
func pickStatus(status map[string]statusEntry) (int, bool) {
	if len(status) == 0 {
		return 0, false
	}
	if s, ok := status["/"]; ok {
		return s.HTTPStatus, true
	}
	return status[sortedKeys(status)[0]].HTTPStatus, true
}

// --- ecosystem*.yaml (curated map shape) ---

// ecosystemFile is the tolerant subset of an ecosystem map the adapter
// reads: a top-level `products` list; everything else is ignored.
type ecosystemFile struct {
	Products []productEntry `yaml:"products"`
}

type productEntry struct {
	ID       string      `yaml:"id"`
	Brand    string      `yaml:"brand"`
	Repos    []string    `yaml:"repos"`
	Domains  []domainRef `yaml:"domains"`
	Vertical string      `yaml:"vertical"` // yaml null → ""
}

// domainRef accepts both curated-map (`domains: [example.app]`) and
// generated-registry (`domains: [{name: example.app, ...}]`) domain forms.
type domainRef struct {
	Name string `yaml:"name"`
}

// UnmarshalYAML decodes a scalar as the domain name itself and anything else
// as a mapping with a `name` key.
func (d *domainRef) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&d.Name)
	}
	type plain domainRef // drop methods to avoid recursing into this hook
	var p plain
	if err := node.Decode(&p); err != nil {
		return err
	}
	*d = domainRef(p)
	return nil
}

// ingestEcosystem emits product-entity facts (aliased-as, implemented-by,
// member-of) plus the canonical domain→product fronts facts, recording each
// fronted domain in curated so the domains.json fallback skips it.
func ingestEcosystem(repoPath, rel string, curated map[string]bool, warnf func(string, ...any)) []fact.Fact {
	data, ok := readRegistry(repoPath, rel, warnf)
	if !ok {
		return nil
	}
	var ef ecosystemFile
	if err := yaml.Unmarshal(data, &ef); err != nil {
		warnf("parsing %s: %v", rel, errOneLine(err))
		return nil
	}
	var facts []fact.Fact
	for i, p := range ef.Products {
		if p.ID == "" {
			warnf("%s: products[%d] has no id — skipped", rel, i)
			continue
		}
		if p.Brand != "" {
			facts = append(facts, declared(p.ID, "aliased-as", p.Brand, rel))
		}
		for _, repo := range p.Repos {
			facts = append(facts, declared(p.ID, "implemented-by", repo, rel))
		}
		if p.Vertical != "" {
			facts = append(facts, declared(p.ID, "member-of", p.Vertical, rel))
		}
		for _, d := range p.Domains {
			id := fact.DomainID(d.Name)
			facts = append(facts, declared(id, "fronts", p.ID, rel))
			curated[id] = true
		}
	}
	return facts
}

// sortedKeys returns the map's keys in sorted order (JSON/YAML maps are
// unordered; emission must be deterministic).
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// errOneLine collapses a (possibly multi-line) parse error into one line so
// warnings render as single lines.
func errOneLine(err error) string {
	return strings.Join(strings.Fields(err.Error()), " ")
}
