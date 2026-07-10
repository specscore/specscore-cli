// Package adapters defines the ingestion-adapter contract for SpecScore
// Studio, the registry of built-in adapters, and the pipeline that runs
// every adapter over every resolved workspace repo and stamps the shared
// fact fields centrally.
//
// Feature: cli/studio/index (REQ: fact-shape, REQ: partial-tolerance,
// REQ: adapter-specscore, REQ: adapter-codegraph, REQ: adapter-manifests,
// REQ: adapter-registries)
package adapters

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/adapters/codegraph"
	"github.com/specscore/specscore-cli/internal/studio/adapters/manifests"
	"github.com/specscore/specscore-cli/internal/studio/adapters/registries"
	"github.com/specscore/specscore-cli/internal/studio/adapters/specscore"
	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var nowFn = time.Now

// Warning is a non-fatal ingestion problem collected by the pipeline. It is
// an alias of fact.Warning so adapter implementations depend only on the
// leaf fact package (the registry below imports them; a Warning type owned
// by this package would be an import cycle).
type Warning = fact.Warning

// Adapter is one ingestion source. Implementations are pure functions of the
// repo path: no store access, independently unit-testable against fixture
// repos. Emitted facts carry subject/predicate/object and Evidence only;
// subjects and objects that start with "#" are repo-relative spec refs and
// are prefixed with the repo slug by Run, which also stamps adapter id +
// version, observed_at, and the ecosystem name centrally.
type Adapter interface {
	ID() string
	Version() string
	Ingest(repoPath string) ([]fact.Fact, []Warning)
}

// Options carries workspace-level configuration into the adapters that need
// it; the zero value is a valid default.
type Options struct {
	// Registries maps a resolved repo path to the extra registry file paths
	// (repo-relative) configured via the per-repo `registries:` entry form
	// in studio.yaml (workspace.Workspace.ResolveRegistries).
	Registries map[string][]string
}

// All returns the built-in adapters in run order — one per line, so each
// sibling adapter lands as a one-line append.
func All(opts Options) []Adapter {
	return []Adapter{
		specscore.New(),
		codegraph.New(),
		manifests.New(),
		registries.New(opts.Registries),
	}
}

// Result is the outcome of one ingestion run across all repos and adapters.
type Result struct {
	// Facts are the fully stamped facts, in repo-then-adapter emission order.
	Facts []fact.Fact
	// Warnings are the collected non-fatal problems, stamped with the repo
	// slug and adapter id they came from. Repo-level warnings (a repo path
	// that is not an existing directory) carry an empty Adapter.
	Warnings []Warning
	// FactsByAdapter counts the facts each adapter emitted, keyed by adapter
	// id; every adapter that ran has an entry (zero included).
	FactsByAdapter map[string]int
	// FactsByRepo groups the stamped facts by the slug of the repo whose
	// adapter run produced them — the provenance the INGR export needs to
	// attribute global-coordinate facts (e.g. registry domains) to a repo
	// (REQ: ingr-export). Skipped repos have no entry (zero facts).
	FactsByRepo map[string][]fact.Fact
	// Repos are the per-repo run summaries, in input repo order; skipped
	// repos are included (zero facts, one repo-level warning).
	Repos []RepoSummary
}

// RepoSummary is the per-repo rollup for the run summary: how many facts and
// warnings one repo path contributed (REQ: partial-tolerance).
type RepoSummary struct {
	// Path is the repo path as passed to Run.
	Path string
	// Slug is the stable repo slug minted for the path.
	Slug string
	// Facts is the number of facts ingested from the repo.
	Facts int
	// Warnings is the number of warnings collected for the repo.
	Warnings int
}

// Run executes every adapter over every repo (sequentially) and stamps the
// shared fact fields centrally: the repo slug onto "#"-prefixed subjects and
// objects, the adapter id + version, one observed_at timestamp for the whole
// run (UTC, RFC 3339), and the ecosystem name.
//
// Partial tolerance (REQ: partial-tolerance) is enforced at repo and adapter
// granularity here (adapters handle file granularity themselves): a repo
// path that is not an existing directory becomes one repo-level warning and
// is skipped; a panicking adapter is recovered into a warning naming the
// adapter and repo, and the remaining adapters still run.
func Run(adapters []Adapter, repos []string, ecosystem string) Result {
	observedAt := nowFn().UTC().Format(time.RFC3339)
	slugger := fact.NewRepoSlugger()
	res := Result{
		FactsByAdapter: make(map[string]int, len(adapters)),
		FactsByRepo:    make(map[string][]fact.Fact, len(repos)),
	}
	for _, a := range adapters {
		res.FactsByAdapter[a.ID()] = 0
	}
	for _, repo := range repos {
		slug := slugger.Slug(repo)
		summary := RepoSummary{Path: repo, Slug: slug}
		if info, err := os.Stat(repo); err != nil || !info.IsDir() {
			res.Warnings = append(res.Warnings, Warning{Repo: slug,
				Message: fmt.Sprintf("repo directory does not exist: %s — repo skipped", repo)})
			summary.Warnings++
			res.Repos = append(res.Repos, summary)
			continue
		}
		for _, a := range adapters {
			facts, warnings := safeIngest(a, repo)
			for _, f := range facts {
				if strings.HasPrefix(f.Subject, "#") {
					f.Subject = slug + f.Subject
				}
				if strings.HasPrefix(f.Object, "#") {
					f.Object = slug + f.Object
				}
				f.Adapter = fact.Adapter{ID: a.ID(), Version: a.Version()}
				f.ObservedAt = observedAt
				f.Ecosystem = ecosystem
				res.Facts = append(res.Facts, f)
				res.FactsByAdapter[a.ID()]++
				res.FactsByRepo[slug] = append(res.FactsByRepo[slug], f)
				summary.Facts++
			}
			for _, w := range warnings {
				w.Repo = slug
				w.Adapter = a.ID()
				res.Warnings = append(res.Warnings, w)
				summary.Warnings++
			}
		}
		res.Repos = append(res.Repos, summary)
	}
	return res
}

// safeIngest runs one adapter over one repo, recovering a panic into a
// warning so the rest of the run completes (REQ: partial-tolerance, adapter
// granularity). On panic no facts survive for that adapter+repo; the caller
// stamps the warning with the adapter id and repo slug like any other.
func safeIngest(a Adapter, repo string) (facts []fact.Fact, warnings []Warning) {
	defer func() {
		if r := recover(); r != nil {
			facts = nil
			warnings = []Warning{{Message: fmt.Sprintf("adapter panicked: %v — adapter skipped for this repo", r)}}
		}
	}()
	return a.Ingest(repo)
}
