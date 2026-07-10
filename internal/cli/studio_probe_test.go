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

// --- Task 3: merge semantics end-to-end and verb guards ---

// AC: probe-preserves-index-facts — probing merges rather than rebuilds:
// the pre-existing declared and derived facts from studio index are still
// present alongside the new verified-behavior facts after studio probe runs.
func TestStudioProbe_PreservesIndexFacts(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a")
	wsDir := filepath.Dir(wsPath)
	newSpecScoreRepo(t, wsDir, "repo-a") // produces a declared has-status fact

	// Build the store via the real studio index verb.
	if _, _, err := runStudioCmd(t, "index", "--workspace", wsPath, "--no-ingr"); err != nil {
		t.Fatalf("studio index: %v", err)
	}

	// Seed a serves-status declared fact so the domain probe has a target.
	// We merge it into the existing store (rather than rebuild) to keep
	// the index facts present.
	dbPath := filepath.Join(wsDir, ".specscore-studio", "facts.db")
	if _, err := store.Merge(dbPath, []fact.Fact{declaredServesStatusFact("example.app", "200")}); err != nil {
		t.Fatalf("seeding serves-status: %v", err)
	}

	// Stub HTTP so the domain probe succeeds without touching the network.
	stubProbeHTTP(t, func(string) (*http.Response, error) { return httpOK(200), nil })

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe: %v", err)
	}

	// The declared has-status fact written by studio index must still be present.
	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--adapter", "specscore", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts --adapter specscore: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	if len(facts) == 0 {
		t.Errorf("studio probe wiped the index-written specscore facts — want them preserved")
	}
	for _, f := range facts {
		if f.Class != fact.Declared {
			t.Errorf("index fact class = %q, want declared: %+v", f.Class, f)
		}
	}
}

// AC: reprobe-refreshes-verified-at — re-verifying an unchanged fact advances
// verified_at but not observed_at.
//
// Given a store already probed at T1 with a serves-status 200 verified-behavior
// fact for example.app whose observed_at and verified_at both equal T1, when
// the domain probe is stubbed to again return 200 at a later time T2 and
// studio probe runs, then the fact's observed_at is still T1 and verified_at
// is T2 (REQ: verified-at-field, REQ: probe-merge).
func TestStudioProbe_ReprobeRefreshesVerifiedAt(t *testing.T) {
	t1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)

	wsPath := newStudioWorkspace(t, "repo-a")

	// Seed the store with a declared serves-status fact (probe target) and an
	// already-probed verified-behavior fact at T1.
	t1Stamp := t1.UTC().Format(time.RFC3339)
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredServesStatusFact("example.app", "200"),
		{
			Subject:    "example.app",
			Predicate:  "serves-status",
			Object:     "200",
			Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: "https://example.app/"},
			Adapter:    fact.Adapter{ID: probe.DomainAdapterID, Version: "0.0.0"},
			ObservedAt: t1Stamp,
			VerifiedAt: t1Stamp,
			Ecosystem:  "demo",
		},
	})

	// Control the clock so the second probe runs at T2.
	oldNow := studioNowFn
	studioNowFn = func() time.Time { return t2 }
	t.Cleanup(func() { studioNowFn = oldNow })

	stubProbeHTTP(t, func(string) (*http.Response, error) { return httpOK(200), nil })

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe at T2: %v", err)
	}

	// Read back the verified-behavior fact and check the timestamp fields.
	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--subject", "example.app", "--class", "verified-behavior", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}
	// There must be exactly one verified-behavior 200 fact (refresh, not a new
	// insertion).
	var got200 []fact.Fact
	for _, f := range facts {
		if f.Object == "200" {
			got200 = append(got200, f)
		}
	}
	if len(got200) != 1 {
		t.Fatalf("got %d verified-behavior 200 facts, want exactly 1 (a refresh, not a new observation): %+v", len(got200), facts)
	}
	f := got200[0]
	t2Stamp := t2.UTC().Format(time.RFC3339)
	if f.ObservedAt != t1Stamp {
		t.Errorf("ObservedAt = %q, want T1 %q (must be preserved on re-verification)", f.ObservedAt, t1Stamp)
	}
	if f.VerifiedAt != t2Stamp {
		t.Errorf("VerifiedAt = %q, want T2 %q (must advance on re-verification)", f.VerifiedAt, t2Stamp)
	}
}

// AC: changed-object-new-observation — a changed probe result is a fresh
// observation, not a re-verification.
//
// Given a store with a prior verified-behavior serves-status 200 fact at T1,
// when the domain probe returns a connection error at T2, then a serves-status
// down fact exists whose observed_at and verified_at both equal T2, and the
// prior 200 fact is still present (append-only supersession per REQ: probe-merge).
func TestStudioProbe_ChangedObjectNewObservation(t *testing.T) {
	t1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	wsPath := newStudioWorkspace(t, "repo-a")

	t1Stamp := t1.UTC().Format(time.RFC3339)
	// Seed with the declared target and a prior verified 200 at T1.
	seedProbeStore(t, wsPath, []fact.Fact{
		declaredServesStatusFact("example.app", "200"),
		{
			Subject:    "example.app",
			Predicate:  "serves-status",
			Object:     "200",
			Evidence:   fact.Evidence{Class: fact.VerifiedBehavior, Pointer: "https://example.app/"},
			Adapter:    fact.Adapter{ID: probe.DomainAdapterID, Version: "0.0.0"},
			ObservedAt: t1Stamp,
			VerifiedAt: t1Stamp,
			Ecosystem:  "demo",
		},
	})

	// At T2 the domain is unreachable.
	oldNow := studioNowFn
	studioNowFn = func() time.Time { return t2 }
	t.Cleanup(func() { studioNowFn = oldNow })

	stubProbeHTTP(t, func(string) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})

	if _, _, err := runStudioCmd(t, "probe", "--workspace", wsPath, "--kind", "domain"); err != nil {
		t.Fatalf("studio probe at T2: %v", err)
	}

	// The store must now contain both the old 200 fact and the new down fact.
	out, _, err := runStudioCmd(t, "facts", "--workspace", wsPath,
		"--subject", "example.app", "--class", "verified-behavior", "--format", "json")
	if err != nil {
		t.Fatalf("studio facts: %v", err)
	}
	var facts []fact.Fact
	if err := json.Unmarshal([]byte(out), &facts); err != nil {
		t.Fatalf("facts output is not JSON: %v\n%s", err, out)
	}

	byObject := map[string]fact.Fact{}
	for _, f := range facts {
		byObject[f.Object] = f
	}
	if _, ok := byObject["200"]; !ok {
		t.Error("prior 200 fact is gone — must be preserved (append-only supersession)")
	}
	downFact, ok := byObject[probe.DownObject]
	if !ok {
		t.Fatalf("no down fact found after a failed probe at T2; got: %+v", facts)
	}
	t2Stamp := t2.UTC().Format(time.RFC3339)
	if downFact.ObservedAt != t2Stamp {
		t.Errorf("down fact ObservedAt = %q, want T2 %q (fresh observation)", downFact.ObservedAt, t2Stamp)
	}
	if downFact.VerifiedAt != t2Stamp {
		t.Errorf("down fact VerifiedAt = %q, want T2 %q (fresh observation)", downFact.VerifiedAt, t2Stamp)
	}
}

// AC: probe-without-index-errors — studio probe before studio index exits 2
// with the same actionable guidance studio facts gives: the message names the
// expected store path and suggests specscore studio index.
//
// Message parity is verified by running both verbs against the same missing
// store and checking they carry identical key substrings (REQ: probe-verb,
// REQ: facts-query).
func TestStudioProbe_WithoutIndexExits2SameGuidanceAsFacts(t *testing.T) {
	wsPath := newStudioWorkspace(t, "repo-a") // no studio index run
	wantDB := filepath.Join(filepath.Dir(wsPath), ".specscore-studio", "facts.db")

	_, _, probeErr := runStudioCmd(t, "probe", "--workspace", wsPath)
	if code := studioExit(t, probeErr); code != 2 {
		t.Errorf("probe exit code = %d, want 2", code)
	}

	_, _, factsErr := runStudioCmd(t, "facts", "--workspace", wsPath)
	if code := studioExit(t, factsErr); code != 2 {
		t.Errorf("facts exit code = %d, want 2", code)
	}

	// Both messages must name the store path and suggest studio index.
	for _, tc := range []struct {
		verb string
		err  error
	}{
		{"probe", probeErr},
		{"facts", factsErr},
	} {
		msg := tc.err.Error()
		if !strings.Contains(msg, wantDB) {
			t.Errorf("%s error %q does not name the expected store path %q", tc.verb, msg, wantDB)
		}
		if !strings.Contains(msg, "studio index") {
			t.Errorf("%s error %q does not suggest `studio index`", tc.verb, msg)
		}
	}

	// Message parity: both errors must carry the same store path and the same
	// suggestion keyword — the guidance is identical whether you ran probe or facts.
	probeParts := extractGuidanceParts(probeErr.Error())
	factsParts := extractGuidanceParts(factsErr.Error())
	if probeParts != factsParts {
		t.Errorf("probe and facts give different guidance:\n  probe: %q\n  facts: %q",
			probeErr.Error(), factsErr.Error())
	}
}

// extractGuidanceParts returns the key guidance tokens from a missing-store
// error message — the store path and the "studio index" suggestion — so tests
// can verify probe and facts carry identical guidance without being brittle
// about phrasing differences.
func extractGuidanceParts(msg string) [2]bool {
	return [2]bool{
		strings.Contains(msg, ".specscore-studio"),
		strings.Contains(msg, "studio index"),
	}
}
