package selfupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newReleasesServer serves the given JSON body at the releases endpoint.
func newReleasesServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// AC: latest-stable-only — newest release is a prerelease; an older stable
// release exists. The resolved latest must be the stable one.
func TestLatestStableTag_SkipsPrerelease(t *testing.T) {
	body := `[
		{"tag_name": "v1.0.0-rc.1", "prerelease": true, "draft": false},
		{"tag_name": "v0.6.0", "prerelease": false, "draft": false}
	]`
	srv := newReleasesServer(t, body)

	r := Resolver{BaseURL: srv.URL, Client: srv.Client()}
	tag, err := r.LatestStableTag(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v0.6.0" {
		t.Fatalf("latest stable tag = %q, want %q", tag, "v0.6.0")
	}
}

// LatestStableTag must also skip drafts.
func TestLatestStableTag_SkipsDraft(t *testing.T) {
	body := `[
		{"tag_name": "v2.0.0", "prerelease": false, "draft": true},
		{"tag_name": "v1.5.0", "prerelease": false, "draft": false}
	]`
	srv := newReleasesServer(t, body)

	r := Resolver{BaseURL: srv.URL, Client: srv.Client()}
	tag, err := r.LatestStableTag(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tag != "v1.5.0" {
		t.Fatalf("latest stable tag = %q, want %q", tag, "v1.5.0")
	}
}

// A non-200 response must surface as a Go error.
func TestLatestStableTag_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	r := Resolver{BaseURL: srv.URL, Client: srv.Client()}
	if _, err := r.LatestStableTag(context.Background()); err == nil {
		t.Fatal("expected error on non-200 response, got nil")
	}
}

// AC: dev-build-is-undetermined — a dev build is Undetermined and exits 10.
func TestCompare_DevIsUndetermined(t *testing.T) {
	got := Compare("dev", "v0.6.0")
	if got.Verdict != Undetermined {
		t.Fatalf("verdict = %v, want Undetermined", got.Verdict)
	}
	if code := got.Verdict.ExitCode(); code != 10 {
		t.Fatalf("exit code = %d, want 10", code)
	}
}

func TestCompare_UpToDate(t *testing.T) {
	// Bare current vs v-prefixed latest must normalize and match.
	got := Compare("0.6.0", "v0.6.0")
	if got.Verdict != UpToDate {
		t.Fatalf("verdict = %v, want UpToDate", got.Verdict)
	}
	if code := got.Verdict.ExitCode(); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestCompare_Available(t *testing.T) {
	got := Compare("0.5.0", "v0.6.0")
	if got.Verdict != Available {
		t.Fatalf("verdict = %v, want Available", got.Verdict)
	}
	if code := got.Verdict.ExitCode(); code != 10 {
		t.Fatalf("exit code = %d, want 10", code)
	}
	if got.Current != "0.5.0" || got.Latest != "0.6.0" {
		t.Fatalf("result = %+v, want Current=0.5.0 Latest=0.6.0", got)
	}
}
