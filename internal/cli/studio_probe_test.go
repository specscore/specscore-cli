package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/internal/studio/probe"
	"github.com/specscore/specscore-cli/internal/studio/store"
)

// seedProbeStore rebuilds the workspace's default store with the given facts
// and returns the store path.
func seedProbeStore(t *testing.T, wsPath string, facts []fact.Fact) string {
	t.Helper()
	dbPath := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")
	if err := store.Rebuild(dbPath, facts); err != nil {
		t.Fatal(err)
	}
	return dbPath
}

// declaredServesStatusFact is a declared serves-status fact for a domain — the
// registries adapter's output that the domain probe reads its targets from.
func declaredServesStatusFact(domain, status string) fact.Fact {
	return fact.Fact{
		Subject:    domain,
		Predicate:  "serves-status",
		Object:     status,
		Evidence:   fact.Evidence{Class: fact.Declared, Pointer: "domains.json"},
		Adapter:    fact.Adapter{ID: "registries", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

// stubProbeHTTP replaces the probe package's HTTP seam for one test.
func stubProbeHTTP(t *testing.T, fn func(url string) (*http.Response, error)) {
	t.Helper()
	old := probe.HTTPGetFn
	probe.HTTPGetFn = fn
	t.Cleanup(func() { probe.HTTPGetFn = old })
}

func httpOK(code int) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(""))}
}

// AC: probe-writes-verified-serves-status — a live 200 probe records a
// verified-behavior serves-status fact addressable by studio facts.
func TestStudioProbe_WritesVerifiedServesStatus(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	stubProbeHTTP(t, func(url string) (*http.Response, error) {
		if url != "https://example.app/" {
			t.Fatalf("unexpected URL %q — https first", url)
		}
		return httpOK(200), nil
	})

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe: %v", err)
	}

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "serves-status", "--class", "verified-behavior", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d verified facts, want 1", len(facts))
	}
	f := facts[0]
	if f.Subject != "example.app" || f.Predicate != "serves-status" || f.Object != "200" {
		t.Errorf("triple = (%s,%s,%s), want (example.app,serves-status,200)", f.Subject, f.Predicate, f.Object)
	}
	if f.Class != fact.VerifiedBehavior || f.Adapter.ID != probe.DomainAdapterID {
		t.Errorf("class/adapter = %s/%s, want verified-behavior/probe-domain", f.Class, f.Adapter.ID)
	}
	if f.Pointer != "https://example.app/" {
		t.Errorf("pointer = %q, want the probed URL", f.Pointer)
	}
	if f.ObservedAt == "" {
		t.Error("observed_at is empty, want a non-empty stamp")
	}
}

// AC: declared-and-verified-coexist — the probe fact appends alongside the
// declared fact rather than overwriting it.
func TestStudioProbe_DeclaredAndVerifiedCoexist(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	stubProbeHTTP(t, func(string) (*http.Response, error) { return httpOK(200), nil })

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe: %v", err)
	}

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--predicate", "serves-status", "--subject", "example.app", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	var declaredSeen, verifiedSeen bool
	for _, f := range facts {
		switch f.Class {
		case fact.Declared:
			if f.Pointer == "domains.json" {
				declaredSeen = true
			}
		case fact.VerifiedBehavior:
			if strings.HasPrefix(f.Pointer, "https://example.app/") {
				verifiedSeen = true
			}
		}
	}
	if !declaredSeen {
		t.Error("declared fact (pointer domains.json) missing — the probe overwrote it")
	}
	if !verifiedSeen {
		t.Error("verified-behavior fact (probed URL pointer) missing")
	}
}

// AC: network-failure-records-down — an unreachable domain records a
// verified-behavior serves-status=down fact.
func TestStudioProbe_NetworkFailureRecordsDown(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("dead.example", "200")})
	stubProbeHTTP(t, func(string) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe: %v", err)
	}

	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--subject", "dead.example", "--predicate", "serves-status",
		"--class", "verified-behavior", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	if len(facts) != 1 || facts[0].Object != probe.DownObject || facts[0].Class != fact.VerifiedBehavior {
		t.Fatalf("got %+v, want one verified-behavior down fact", facts)
	}
}

// The JSON run summary carries the documented shape.
func TestStudioProbe_JSONSummaryShape(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	stubProbeHTTP(t, func(string) (*http.Response, error) { return httpOK(200), nil })

	out, _, err := runStudioCmd(t, "probe", "--workspace", wsPath,
		"--kind", "domain", "--format", "json")
	if err != nil {
		t.Fatalf("studio probe: %v", err)
	}
	var s struct {
		Kinds           []string `json:"kinds"`
		FactsWritten    int      `json:"facts_written"`
		VerifiedRefresh int      `json:"verified_refreshed"`
		Warnings        []string `json:"warnings"`
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("summary is not JSON: %v\n%s", err, out)
	}
	if len(s.Kinds) != 1 || s.Kinds[0] != "domain" {
		t.Errorf("kinds = %v, want [domain]", s.Kinds)
	}
	if s.FactsWritten != 1 {
		t.Errorf("facts_written = %d, want 1", s.FactsWritten)
	}
	if s.VerifiedRefresh != 0 {
		t.Errorf("verified_refreshed = %d, want 0", s.VerifiedRefresh)
	}
	if s.Warnings == nil {
		t.Error("warnings must serialize as [] not null")
	}
	for _, key := range []string{`"kinds"`, `"facts_written"`, `"verified_refreshed"`, `"warnings"`} {
		if !strings.Contains(out, key) {
			t.Errorf("summary JSON missing %s:\n%s", key, out)
		}
	}
}

// Re-probing an unchanged fact refreshes verified_at (Refreshed), not Written —
// the human summary reports it.
func TestStudioProbe_HumanSummaryReportsRefresh(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	stubProbeHTTP(t, func(string) (*http.Response, error) { return httpOK(200), nil })

	// First probe writes the verified fact.
	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("first studio probe: %v", err)
	}
	// Second probe re-verifies the same object: a refresh, not a write.
	out, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain")
	if err != nil {
		t.Fatalf("second studio probe: %v", err)
	}
	if !strings.Contains(out, "Probe kinds: domain") {
		t.Errorf("human summary missing kinds line:\n%s", out)
	}
	if !strings.Contains(out, "Facts written: 0") {
		t.Errorf("human summary should report 0 facts written on a re-probe:\n%s", out)
	}
	if !strings.Contains(out, "Verified refreshed: 1") {
		t.Errorf("human summary should report 1 refreshed on a re-probe:\n%s", out)
	}
}

// --- guards & flag validation ---

func TestStudioProbe_MissingStoreExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no store built
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, err := runStudioCmd(t, "probe", "--workspace", wsPath)
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), wantDB) || !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q must name the store path and suggest `studio index`", err.Error())
	}
}

func TestStudioProbe_BadFormatExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})

	_, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--format", "yaml")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "yaml") {
		t.Errorf("error %q does not name the bad format", err.Error())
	}
}

func TestStudioProbe_BadKindExits2(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})

	_, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "banana")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error %q does not name the bad kind", err.Error())
	}
}

// --kind ci is accepted this phase but runs no kinds yet (Task 4 lands it): the
// run succeeds, issues no HTTP request, and writes no facts.
func TestStudioProbe_KindCIRunsNoKindsYet(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	stubProbeHTTP(t, func(string) (*http.Response, error) {
		t.Fatal("the ci kind must not issue an HTTP request")
		return nil, nil
	})

	out, _, err := runStudioCmd(t, "probe", "--workspace", wsPath,
		"--kind", "ci", "--format", "json")
	if err != nil {
		t.Fatalf("studio probe --kind ci: %v", err)
	}
	var s struct {
		Kinds        []string `json:"kinds"`
		FactsWritten int      `json:"facts_written"`
	}
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("summary is not JSON: %v", err)
	}
	if len(s.Kinds) != 0 || s.FactsWritten != 0 {
		t.Errorf("ci-only run = %+v, want no kinds and 0 facts this phase", s)
	}
}

// --kind all runs the implemented domain kind.
func TestStudioProbe_KindAllRunsDomain(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})
	var called bool
	stubProbeHTTP(t, func(string) (*http.Response, error) {
		called = true
		return httpOK(200), nil
	})

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "all"); err != nil {
		t.Fatalf("studio probe --kind all: %v", err)
	}
	if !called {
		t.Error("the domain kind did not run under --kind all")
	}
}

// A merge failure surfaces as a non-zero exit; the prior store is untouched.
func TestStudioProbe_MergeErrorPropagates(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	seedProbeStore(t, wsPath, []fact.Fact{declaredServesStatusFact("example.app", "200")})

	oldRun := probeRunDomainFn
	probeRunDomainFn = func([]fact.Fact, string, time.Time) probe.Result {
		return probe.Result{Kinds: []string{"domain"}}
	}
	t.Cleanup(func() { probeRunDomainFn = oldRun })

	oldMerge := storeMergeFn
	storeMergeFn = func(string, []fact.Fact) (store.MergeResult, error) {
		return store.MergeResult{}, errors.New("merge boom")
	}
	t.Cleanup(func() { storeMergeFn = oldMerge })

	_, _, err := runStudioCmd(t, "probe", "--workspace", wsPath)
	if err == nil {
		t.Fatal("expected the merge error to propagate")
	}
	if !strings.Contains(err.Error(), "merge boom") {
		t.Errorf("error %q does not carry the merge failure", err.Error())
	}
}

// A bad --db path (abs resolution failure) exits 2 before touching the store.
func TestStudioProbe_DBFlagAbsError(t *testing.T) {
	old := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = old })

	_, _, err := runStudioCmd(t, "probe", "--db", "rel.db")
	if code := studioExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}
