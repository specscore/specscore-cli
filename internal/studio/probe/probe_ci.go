package probe

// Feature: cli/studio/probe (REQ: ci-state, REQ: probe-verb, REQ: probe-merge)
//
// The ci kind (adapter id `probe-ci`) reads each workspace repo's latest
// default-branch Actions run via `gh api` (the GitHub CLI, house exec-seam
// pattern) and emits a `(<repo-slug>, ci-status, <conclusion>)`
// verified-behavior fact. Targets are the workspace repos, not store facts:
// the store's repo ids are `fact.RepoSlugger` basename slugs with no remote
// coordinates, so the ci probe re-mints those same slugs over the same
// ResolveRepos order the index run used (the slug-join invariant) and resolves
// each repo's GitHub coordinate from `git remote get-url origin`. A repo with
// no `origin` remote or a non-GitHub remote is skipped with a visible per-repo
// notice; a repo whose `gh api` call fails records a per-repo warning and no
// fact. `gh` absent from PATH skips the whole kind upfront with one warning,
// mirroring rehearse's missing-binary skip.

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	// ExecCommandFn is the ci kind's exec seam: it runs name with args in dir
	// and returns stdout. Every `git`/`gh` invocation goes through this var so
	// tests drive git/gh outcomes without touching the filesystem or the
	// network (house exec-seam pattern, mirroring internal/rehearse/runner).
	ExecCommandFn = execCommandOutput

	// execNewCommandFn constructs the *exec.Cmd behind ExecCommandFn; a seam so
	// the default (unstubbed) exec path stays testable.
	execNewCommandFn = func(name string, args ...string) *exec.Cmd {
		return exec.Command(name, args...)
	}

	// LookPathFn is the seam for the upfront `gh` presence check: an absent
	// binary skips the ci kind entirely with one summary warning, exactly as
	// rehearse skips a scenario needing a missing binary.
	LookPathFn = exec.LookPath
)

const (
	// CIAdapterID is the adapter id stamped on every ci-kind fact. It is part of
	// the store merge key and must differ from the domain kind's id and from
	// every index adapter's id, so one kind's merge never clobbers another's
	// (REQ: probe-verb, REQ: probe-merge).
	CIAdapterID = "probe-ci"

	// CIStatusPredicate is the predicate the ci kind emits: a repo's latest
	// default-branch CI run conclusion, joinable against the repo's other facts
	// by its store slug subject (REQ: ci-state).
	CIStatusPredicate = "ci-status"

	// KindCI names the ci-state probe kind in the run summary.
	KindCI = "ci"

	// ghBinary is the GitHub CLI the ci kind shells out to.
	ghBinary = "gh"

	// gitBinary resolves each repo's origin remote coordinate.
	gitBinary = "git"
)

// CIRepo is one CI target: the local repo directory to run git/gh in, and the
// store slug the emitted fact's subject must carry so consumers can join CI
// state against the repo's other facts (REQ: ci-state — the slug-join
// invariant). The verb re-mints slugs over the same ResolveRepos order the
// index run used.
type CIRepo struct {
	// Dir is the absolute repo directory `git remote get-url origin` runs in.
	Dir string
	// Slug is the store's repo slug — the emitted fact's subject.
	Slug string
	// Ecosystem is carried onto the emitted fact so it joins the same ecosystem
	// as the repo's index facts.
	Ecosystem string
}

// RunCI runs the ci-state kind over the given repos. When `gh` is not on PATH
// the whole kind is skipped upfront with one warning and no facts (mirroring
// rehearse's missing-binary skip). For each repo it resolves the GitHub
// org/name from `git remote get-url origin`; a repo with no origin remote or a
// non-GitHub remote is skipped with a per-repo notice. For each GitHub-remoted
// repo it queries the latest default-branch Actions run conclusion via
// `gh api` and emits one verified-behavior fact under adapter id probe-ci
// (version = cliVersion), with the evidence pointer set to the queried gh api
// path and observed_at/verified_at both stamped at now. A repo with no runs or
// a failing `gh api` call records a per-repo warning and emits no fact — an
// unreachable CI is "not probed", not "down" (REQ: ci-state).
func RunCI(repos []CIRepo, cliVersion string, now time.Time) Result {
	res := Result{Kinds: []string{KindCI}}
	if _, err := LookPathFn(ghBinary); err != nil {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("ci kind skipped: %q was not found on PATH", ghBinary))
		return res
	}
	stamp := now.UTC().Format(time.RFC3339)
	for _, r := range repos {
		if f, warning, ok := probeCI(r, cliVersion, stamp); ok {
			res.Facts = append(res.Facts, f)
		} else if warning != "" {
			res.Warnings = append(res.Warnings, warning)
		}
	}
	return res
}

// probeCI resolves one repo's CI conclusion. It returns (fact, "", true) on a
// successful probe, (zero, notice, false) when the repo is skipped or its
// gh api call fails (the notice going to the run summary), and
// (zero, "", false) never (a skip always carries a notice).
func probeCI(r CIRepo, cliVersion, stamp string) (fact.Fact, string, bool) {
	org, name, ok := githubCoordinate(r.Dir)
	if !ok {
		return fact.Fact{}, fmt.Sprintf(
			"%s: skipped — no GitHub origin remote (a repo without a GitHub remote is not a CI target)", r.Slug), false
	}
	conclusion, path, ok := latestRunConclusion(r.Dir, org, name)
	if !ok {
		return fact.Fact{}, fmt.Sprintf(
			"%s: skipped — could not read latest CI run for %s/%s via gh api (unauthenticated, rate-limited, no runs, or repo not found)", r.Slug, org, name), false
	}
	return fact.Fact{
		Subject:   r.Slug,
		Predicate: CIStatusPredicate,
		Object:    conclusion,
		Evidence:  fact.Evidence{Class: fact.VerifiedBehavior, Pointer: path},
		Adapter:   fact.Adapter{ID: CIAdapterID, Version: cliVersion},
		// A fresh observation stamps observed_at and verified_at equal; the
		// store's Merge refreshes verified_at (preserving observed_at) when a
		// re-probe re-verifies an unchanged conclusion (REQ: verified-at-field).
		ObservedAt: stamp,
		VerifiedAt: stamp,
		Ecosystem:  r.Ecosystem,
	}, "", true
}

// githubCoordinate runs `git remote get-url origin` in dir and parses the
// GitHub org/name from the remote URL. It returns ok=false when git fails (no
// origin remote) or when the URL is not a GitHub remote. Both https and ssh
// remote URL forms are accepted:
//
//	https://github.com/acme/widget.git      → acme, widget
//	git@github.com:acme/widget.git          → acme, widget
//	ssh://git@github.com/acme/widget        → acme, widget
func githubCoordinate(dir string) (org, name string, ok bool) {
	out, err := ExecCommandFn(dir, gitBinary, "remote", "get-url", "origin")
	if err != nil {
		return "", "", false
	}
	return parseGitHubRemote(strings.TrimSpace(string(out)))
}

// parseGitHubRemote extracts the GitHub org/name from a remote URL in https or
// ssh form. A non-GitHub host, or a URL that does not name an org/name pair,
// returns ok=false.
func parseGitHubRemote(url string) (org, name string, ok bool) {
	const host = "github.com"
	var path string
	switch {
	case strings.HasPrefix(url, "git@"):
		// git@github.com:acme/widget.git
		rest, found := strings.CutPrefix(url, "git@")
		if !found {
			return "", "", false
		}
		h, p, found := strings.Cut(rest, ":")
		if !found || h != host {
			return "", "", false
		}
		path = p
	case strings.HasPrefix(url, "https://"), strings.HasPrefix(url, "http://"), strings.HasPrefix(url, "ssh://"):
		rest := url
		for _, scheme := range []string{"https://", "http://", "ssh://"} {
			if r, found := strings.CutPrefix(rest, scheme); found {
				rest = r
				break
			}
		}
		// rest is now host[/or user@host]/org/name; drop any user@ prefix.
		if _, after, found := strings.Cut(rest, "@"); found {
			rest = after
		}
		h, p, found := strings.Cut(rest, "/")
		if !found || h != host {
			return "", "", false
		}
		path = p
	default:
		return "", "", false
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	org, name, found := strings.Cut(path, "/")
	if !found || org == "" || name == "" || strings.Contains(name, "/") {
		return "", "", false
	}
	return org, name, true
}

// latestRunConclusion queries the repo's latest default-branch Actions run
// conclusion via `gh api` and returns it alongside the gh api path used as the
// evidence pointer. It first resolves the repo's default branch, then queries
// the latest run on that branch. ok=false on any gh api failure or when there
// is no run to read (REQ: ci-state).
func latestRunConclusion(dir, org, name string) (conclusion, pointer string, ok bool) {
	branch, ok := defaultBranch(dir, org, name)
	if !ok {
		return "", "", false
	}
	// Endpoint per REQ ci-state: the latest run on the default branch.
	path := fmt.Sprintf("repos/%s/%s/actions/runs?branch=%s&per_page=1", org, name, branch)
	out, err := ExecCommandFn(dir, ghBinary, "api", path)
	if err != nil {
		return "", "", false
	}
	conclusion, ok = firstRunConclusion(out)
	if !ok {
		return "", "", false
	}
	return conclusion, path, true
}

// defaultBranch resolves the repo's default branch via `gh api`. ok=false when
// the gh api call fails or returns an empty branch.
func defaultBranch(dir, org, name string) (string, bool) {
	out, err := ExecCommandFn(dir, ghBinary, "api",
		fmt.Sprintf("repos/%s/%s", org, name), "--jq", ".default_branch")
	if err != nil {
		return "", false
	}
	branch := strings.TrimSpace(string(out))
	if branch == "" {
		return "", false
	}
	return branch, true
}

// ghRunsResponse is the slice of the `gh api .../actions/runs` JSON body the
// ci kind reads: the conclusion of the latest workflow run.
type ghRunsResponse struct {
	WorkflowRuns []struct {
		Conclusion string `json:"conclusion"`
	} `json:"workflow_runs"`
}

// firstRunConclusion parses the `gh api .../actions/runs` JSON body and returns
// the latest run's conclusion. ok=false when the body does not parse, lists no
// runs, or the latest run has an empty conclusion (still in progress) — an
// unreadable conclusion is "not probed", not a fact.
func firstRunConclusion(body []byte) (string, bool) {
	var resp ghRunsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", false
	}
	if len(resp.WorkflowRuns) == 0 {
		return "", false
	}
	c := resp.WorkflowRuns[0].Conclusion
	if c == "" {
		return "", false
	}
	return c, true
}

// execCommandOutput runs a command in dir and returns its stdout. This is the
// production implementation behind ExecCommandFn.
func execCommandOutput(dir, name string, args ...string) ([]byte, error) {
	cmd := execNewCommandFn(name, args...)
	cmd.Dir = dir
	return cmd.Output()
}
