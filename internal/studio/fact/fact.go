// Package fact defines the SpecScore Studio fact model — the single record
// shape every ingestion adapter emits and the fact store persists — plus the
// stable-ID helpers used to mint entity references.
//
// Feature: cli/studio/index (REQ: fact-shape)
//
// The package depends on nothing else in studio: adapters and the store both
// consume it.
package fact

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Class is the evidence class of a fact: how directly the artifact asserts it.
type Class string

const (
	// Declared facts are asserted verbatim by an artifact (e.g. a spec's
	// `**Status:**` line, a registry entry).
	Declared Class = "declared"
	// Derived facts are computed from an artifact (e.g. an import edge read
	// out of a codegraph snapshot, a dependency parsed from go.mod).
	Derived Class = "derived"
	// VerifiedBehavior facts are produced by executing the system and
	// observing its outcome — the top rung of the evidence ladder, above
	// declared and derived.
	//
	// Feature: cli/rehearse/evidence (REQ: verified-behavior-class)
	VerifiedBehavior Class = "verified-behavior"
)

// Evidence is the provenance of a fact: its class plus a pointer to the
// artifact that yielded it. Pointer is a repo-relative file path, optionally
// suffixed with a `:<line>` or `:<record>` locator where one is cheap.
type Evidence struct {
	Class   Class  `json:"evidence_class"`
	Pointer string `json:"evidence_pointer"`
}

// Adapter identifies the ingestion adapter (and its version) that emitted a
// fact.
type Adapter struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// Fact is one subject–predicate–object triple with full provenance. Evidence
// is embedded so its fields marshal flat (evidence_class, evidence_pointer)
// per the fact-shape REQ.
type Fact struct {
	Subject   string `json:"subject"`
	Predicate string `json:"predicate"`
	Object    string `json:"object"`
	Evidence
	Adapter    Adapter `json:"adapter"`
	ObservedAt string  `json:"observed_at"`
	// VerifiedAt is the timestamp of the fact's most recent successful
	// re-verification, distinct from ObservedAt (when the behavior was first
	// observed). On first write the two are equal; re-verifying an unchanged
	// fact refreshes VerifiedAt while preserving the original ObservedAt.
	//
	// Feature: cli/studio/probe (REQ: verified-at-field)
	VerifiedAt string `json:"verified_at"`
	Ecosystem  string `json:"ecosystem"`
}

// Warning is a non-fatal ingestion problem: a file, adapter, or repo that
// could not be ingested and was skipped at the smallest possible granularity
// (REQ: partial-tolerance). Adapters fill Message only; the pipeline stamps
// Repo (slug) and Adapter (id) centrally. The type lives here — not in the
// adapters package — so adapter implementations depend only on this leaf
// package while the adapters registry can import them without a cycle.
type Warning struct {
	Repo    string `json:"repo"`
	Adapter string `json:"adapter"`
	Message string `json:"message"`
}

// --- stable-ID helpers ---

// RepoSlugger mints stable IDs for local-only repos (no remote host/org/name
// coordinates): the directory basename, with a `-N` collision counter when
// two paths share a basename. The same path always returns the same slug
// within one slugger; the first path to claim a basename keeps it bare.
type RepoSlugger struct {
	byPath map[string]string
	taken  map[string]bool
}

// NewRepoSlugger returns an empty slugger.
func NewRepoSlugger() *RepoSlugger {
	return &RepoSlugger{byPath: map[string]string{}, taken: map[string]bool{}}
}

// Slug returns the stable slug for the local repo directory at path.
func (s *RepoSlugger) Slug(path string) string {
	clean := filepath.Clean(path)
	if slug, ok := s.byPath[clean]; ok {
		return slug
	}
	base := filepath.Base(clean)
	slug := base
	for n := 2; s.taken[slug]; n++ {
		slug = fmt.Sprintf("%s-%d", base, n)
	}
	s.byPath[clean] = slug
	s.taken[slug] = true
	return slug
}

// SpecID returns the stable ID of a spec artifact: `<repo>#<path-slug>`.
func SpecID(repoID, pathSlug string) string {
	return repoID + "#" + pathSlug
}

// PackageID returns the stable ID of a package/module from its registry
// coordinates: `<module>@<version>`, or just the module when the version is
// unknown.
func PackageID(module, version string) string {
	if version == "" {
		return module
	}
	return module + "@" + version
}

// DomainID returns the stable ID of a domain: the lower-cased fqdn without a
// trailing dot.
func DomainID(fqdn string) string {
	return strings.ToLower(strings.TrimSuffix(fqdn, "."))
}
