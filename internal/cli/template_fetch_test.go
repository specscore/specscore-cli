package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testIdeaTemplate mirrors the published new/idea.md gallery template: a bare
// skeleton with placeholder tokens. Substituting title/date/owner must yield a
// lint-clean Idea.
const testIdeaTemplate = `# Idea: <Idea Name>

**Status:** Draft
**Date:** YYYY-MM-DD
**Owner:** <your-handle>
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement

<!-- One "How Might We…" sentence framing the problem. -->

## Context

<!-- Triggering observation, related specs, prior art. -->

## Recommended Direction

<!-- 2–3 paragraphs: what and why, over the alternatives. -->

## Alternatives Considered

<!-- 2–3 directions that lost, and why each lost. -->

## MVP Scope

<!-- The single job the MVP nails. Timeboxed, not feature-listed. -->

## Not Doing (and Why)

- <thing not being done> — <why>

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|------|------------|-----------------|
| Must-be-true | <dealbreaker assumption> | <how to validate> |
| Should-be-true | … | … |
| Might-be-true | … | … |

## SpecScore Integration

- **New Features this would create:** <list or "TBD at spec time">
- **Existing Features affected:** <list or "none">
- **Dependencies:** <other Ideas or in-flight work>

## Open Questions

- <question that must be answered before promotion to a Feature>

---
*This document follows the https://specscore.md/idea-specification*
`

// AC:maps-type-to-url — each type resolves to <base>/new/<type>.md.
func TestFetchTemplate_MapsTypeToURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	for _, typ := range []string{"idea", "feature", "decision", "issue", "proposal"} {
		if _, err := fetchTemplate(typ); err != nil {
			t.Fatalf("fetchTemplate(%q): %v", typ, err)
		}
		if want := "/new/" + typ + ".md"; gotPath != want {
			t.Errorf("type %q fetched %q, want %q", typ, gotPath, want)
		}
	}
}

// A non-200 response is treated as a fetch failure (→ embedded fallback).
func TestFetchTemplate_Non200IsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	if _, err := fetchTemplate("idea"); err == nil {
		t.Error("expected error on 404, got nil")
	}
}

// AC:timeout-bounds-the-wait — a server that accepts but never responds must
// not hang the fetch; it aborts near the timeout and the error drives fallback.
func TestFetchTemplate_TimesOut(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-block // never respond until cleanup
	}))
	defer srv.Close()
	defer close(block)
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	orig := templateFetchTimeout
	templateFetchTimeout = 150 * time.Millisecond
	defer func() { templateFetchTimeout = orig }()

	start := time.Now()
	_, err := fetchTemplate("idea")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > time.Second {
		t.Errorf("fetch took %v; expected it to abort near the %v timeout", elapsed, templateFetchTimeout)
	}
}

// resolveBareTemplate substitutes tokens on success and signals ok=false offline.
func TestResolveBareTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("# Idea: <Idea Name>\nby <your-handle>\n"))
	}))
	defer srv.Close()

	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)
	body, ok := resolveBareTemplate("idea", map[string]string{
		"<Idea Name>":   "Demo",
		"<your-handle>": "alex",
	})
	if !ok {
		t.Fatal("expected ok=true on reachable server")
	}
	if got := string(body); got != "# Idea: Demo\nby alex\n" {
		t.Errorf("substitution = %q", got)
	}

	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", "http://127.0.0.1:0")
	if _, ok := resolveBareTemplate("idea", nil); ok {
		t.Error("expected ok=false when offline")
	}
}

// AC:fetches-published-template, AC:maps-type-to-url, AC:fills-known-fields.
func TestIdeaNew_FetchesPublishedTemplate(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(testIdeaTemplate))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	root := setupSpecRoot(t)
	withCwd(t, root)

	_, stderr, err := runIdea(t, "new", "fetched-idea", "--owner", "alex")
	if err != nil {
		t.Fatalf("idea new: %v (stderr=%s)", err, stderr)
	}
	if gotPath != "/new/idea.md" {
		t.Errorf("fetched path = %q, want /new/idea.md", gotPath)
	}
	if stderr != "" {
		t.Errorf("unexpected fallback warning on a successful fetch: %q", stderr)
	}

	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "fetched-idea.md"))
	s := string(body)
	for _, want := range []string{"# Idea: Fetched Idea", "**Owner:** alex"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in created file:\n%s", want, s)
		}
	}
	for _, ph := range []string{"<Idea Name>", "YYYY-MM-DD", "<your-handle>"} {
		if strings.Contains(s, ph) {
			t.Errorf("placeholder %q was not substituted", ph)
		}
	}
	// A marker unique to the web template proves the fetched (not embedded)
	// template was written.
	if !strings.Contains(s, "<thing not being done>") {
		t.Errorf("expected fetched web-template markers; got:\n%s", s)
	}
}

// AC:falls-back-when-offline, AC:warns-on-fallback.
func TestIdeaNew_FallsBackWhenOffline(t *testing.T) {
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", "http://127.0.0.1:0")

	root := setupSpecRoot(t)
	withCwd(t, root)

	_, stderr, err := runIdea(t, "new", "offline-idea", "--owner", "alex")
	if err != nil {
		t.Fatalf("idea new: %v", err)
	}
	if !strings.Contains(stderr, "used built-in template") {
		t.Errorf("expected fallback warning on stderr, got %q", stderr)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "offline-idea.md"))
	if !strings.Contains(string(body), "**Owner:** alex") {
		t.Errorf("owner not set in embedded fallback output")
	}
}

// Authored scaffolds (content flags) must NOT fetch — the embedded scaffolder
// fills those fields, which a static web template cannot.
func TestIdeaNew_ContentFlagsSkipFetch(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = w.Write([]byte(testIdeaTemplate))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "authored-idea", "--hmw", "How might we test this?")
	if err != nil {
		t.Fatalf("idea new: %v", err)
	}
	if hit {
		t.Error("gallery was fetched for an authored scaffold; expected the embedded path (no fetch)")
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "authored-idea.md"))
	if !strings.Contains(string(body), "How might we test this?") {
		t.Error("authored HMW missing — embedded scaffolder should fill it")
	}
}
