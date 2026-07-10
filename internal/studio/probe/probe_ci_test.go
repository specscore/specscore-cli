package probe

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// execCall records one ExecCommandFn invocation so tests can assert what the
// ci kind shelled out.
type execCall struct {
	dir  string
	name string
	args []string
}

// stubExec replaces ExecCommandFn for the test with fn and records every call
// into the returned slice pointer.
func stubExec(t *testing.T, fn func(dir, name string, args []string) ([]byte, error)) *[]execCall {
	t.Helper()
	var calls []execCall
	old := ExecCommandFn
	ExecCommandFn = func(dir, name string, args ...string) ([]byte, error) {
		calls = append(calls, execCall{dir: dir, name: name, args: args})
		return fn(dir, name, args)
	}
	t.Cleanup(func() { ExecCommandFn = old })
	return &calls
}

// stubLookPath replaces LookPathFn for the test.
func stubLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	old := LookPathFn
	LookPathFn = fn
	t.Cleanup(func() { LookPathFn = old })
}

// ghFound stubs LookPathFn so `gh` resolves.
func ghFound(t *testing.T) {
	stubLookPath(t, func(name string) (string, error) { return "/usr/bin/" + name, nil })
}

// runsBody is a minimal `gh api .../actions/runs` JSON body with one run of the
// given conclusion.
func runsBody(conclusion string) []byte {
	return []byte(`{"workflow_runs":[{"conclusion":"` + conclusion + `"}]}`)
}

func widget() CIRepo {
	return CIRepo{Dir: "/ws/widget", Slug: "widget", Ecosystem: "demo"}
}

// ciExec dispatches a stub exec response by (name, first-arg) so tests can wire
// git and the two gh api calls (default-branch, then runs) independently.
func ciExec(remote, branch string, runs []byte) func(dir, name string, args []string) ([]byte, error) {
	return func(_, name string, args []string) ([]byte, error) {
		switch {
		case name == "git":
			return []byte(remote), nil
		case name == "gh" && len(args) >= 2 && args[1] == "repos/acme/widget":
			return []byte(branch), nil
		case name == "gh":
			return runs, nil
		}
		return nil, errors.New("unexpected exec call")
	}
}

func TestRunCI_EmitsConclusionFact(t *testing.T) {
	ghFound(t)
	calls := stubExec(t, ciExec("https://github.com/acme/widget.git", "main", runsBody("success")))
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)

	res := RunCI([]CIRepo{widget()}, "9.9.9", now)

	if len(res.Kinds) != 1 || res.Kinds[0] != KindCI {
		t.Fatalf("Kinds = %v, want [%s]", res.Kinds, KindCI)
	}
	if len(res.Facts) != 1 {
		t.Fatalf("got %d facts, want 1; warnings=%v", len(res.Facts), res.Warnings)
	}
	f := res.Facts[0]
	if f.Subject != "widget" || f.Predicate != CIStatusPredicate || f.Object != "success" {
		t.Errorf("triple = (%s,%s,%s), want (widget,ci-status,success)", f.Subject, f.Predicate, f.Object)
	}
	if f.Class != fact.VerifiedBehavior {
		t.Errorf("class = %s, want verified-behavior", f.Class)
	}
	if f.Adapter.ID != CIAdapterID || f.Adapter.Version != "9.9.9" {
		t.Errorf("adapter = %+v, want {probe-ci 9.9.9}", f.Adapter)
	}
	wantPath := "repos/acme/widget/actions/runs?branch=main&per_page=1"
	if f.Pointer != wantPath {
		t.Errorf("pointer = %q, want the queried gh api path %q", f.Pointer, wantPath)
	}
	stamp := now.Format(time.RFC3339)
	if f.ObservedAt != stamp || f.VerifiedAt != stamp {
		t.Errorf("stamps = (%s,%s), want both %s", f.ObservedAt, f.VerifiedAt, stamp)
	}
	if f.Ecosystem != "demo" {
		t.Errorf("ecosystem = %q, want demo", f.Ecosystem)
	}
	// git run in the repo dir, gh api queried the runs endpoint.
	if (*calls)[0].name != "git" || (*calls)[0].dir != "/ws/widget" {
		t.Errorf("first call = %+v, want git in /ws/widget", (*calls)[0])
	}
}

func TestRunCI_GhAbsentSkipsKind(t *testing.T) {
	stubLookPath(t, func(string) (string, error) { return "", errors.New("not found") })
	stubExec(t, func(string, string, []string) ([]byte, error) {
		t.Fatal("no exec should run when gh is absent")
		return nil, nil
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 when gh is absent", len(res.Facts))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "gh") {
		t.Errorf("warnings = %v, want one naming gh", res.Warnings)
	}
	if len(res.Kinds) != 1 || res.Kinds[0] != KindCI {
		t.Errorf("Kinds = %v, want [ci] even when skipped", res.Kinds)
	}
}

func TestRunCI_NoOriginRemoteSkipped(t *testing.T) {
	ghFound(t)
	stubExec(t, func(_, name string, _ []string) ([]byte, error) {
		if name == "git" {
			return nil, errors.New("fatal: No such remote 'origin'")
		}
		t.Fatal("gh should not run for a repo with no origin remote")
		return nil, nil
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 for a repo with no remote", len(res.Facts))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "widget") ||
		!strings.Contains(res.Warnings[0], "GitHub") {
		t.Errorf("warnings = %v, want one per-repo GitHub skip notice", res.Warnings)
	}
}

func TestRunCI_NonGitHubRemoteSkipped(t *testing.T) {
	ghFound(t)
	stubExec(t, func(_, name string, _ []string) ([]byte, error) {
		if name == "git" {
			return []byte("https://gitlab.com/acme/widget.git\n"), nil
		}
		t.Fatal("gh should not run for a non-GitHub remote")
		return nil, nil
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 for a non-GitHub remote", len(res.Facts))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "GitHub") {
		t.Errorf("warnings = %v, want one non-GitHub skip notice", res.Warnings)
	}
}

func TestRunCI_GhAPIFailureWarnsNoFact(t *testing.T) {
	ghFound(t)
	stubExec(t, func(_, name string, args []string) ([]byte, error) {
		if name == "git" {
			return []byte("git@github.com:acme/widget.git\n"), nil
		}
		// default-branch resolves, but the runs query fails (rate limit / 404).
		if len(args) >= 2 && args[1] == "repos/acme/widget" {
			return []byte("main\n"), nil
		}
		return nil, errors.New("gh: HTTP 403 rate limited")
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 when gh api fails", len(res.Facts))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "widget") {
		t.Errorf("warnings = %v, want one per-repo gh-api-failure warning", res.Warnings)
	}
}

func TestRunCI_DefaultBranchFailureSkips(t *testing.T) {
	ghFound(t)
	stubExec(t, func(_, name string, _ []string) ([]byte, error) {
		if name == "git" {
			return []byte("https://github.com/acme/widget.git"), nil
		}
		// The default-branch gh api call itself fails.
		return nil, errors.New("gh: repo not found")
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 when default-branch resolution fails", len(res.Facts))
	}
	if len(res.Warnings) != 1 {
		t.Errorf("warnings = %v, want one skip warning", res.Warnings)
	}
}

func TestRunCI_EmptyDefaultBranchSkips(t *testing.T) {
	ghFound(t)
	stubExec(t, func(_, name string, args []string) ([]byte, error) {
		if name == "git" {
			return []byte("https://github.com/acme/widget.git"), nil
		}
		if len(args) >= 2 && args[1] == "repos/acme/widget" {
			return []byte("  \n"), nil // blank default branch
		}
		t.Fatal("runs query should not run with an empty default branch")
		return nil, nil
	})

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 || len(res.Warnings) != 1 {
		t.Errorf("facts=%d warnings=%v, want 0 facts and one skip warning", len(res.Facts), res.Warnings)
	}
}

func TestRunCI_NoRunsSkips(t *testing.T) {
	ghFound(t)
	stubExec(t, ciExec("https://github.com/acme/widget.git", "main", []byte(`{"workflow_runs":[]}`)))

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0 for a repo with no runs", len(res.Facts))
	}
	if len(res.Warnings) != 1 {
		t.Errorf("warnings = %v, want one no-runs warning", res.Warnings)
	}
}

func TestRunCI_MalformedRunsBodySkips(t *testing.T) {
	ghFound(t)
	stubExec(t, ciExec("https://github.com/acme/widget.git", "main", []byte("not json")))

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 || len(res.Warnings) != 1 {
		t.Errorf("facts=%d warnings=%v, want 0 facts and one warning for a malformed body", len(res.Facts), res.Warnings)
	}
}

func TestRunCI_EmptyConclusionSkips(t *testing.T) {
	ghFound(t)
	stubExec(t, ciExec("https://github.com/acme/widget.git", "main", runsBody("")))

	res := RunCI([]CIRepo{widget()}, "1", time.Now())

	if len(res.Facts) != 0 || len(res.Warnings) != 1 {
		t.Errorf("facts=%d warnings=%v, want 0 facts and one warning for an in-progress run", len(res.Facts), res.Warnings)
	}
}

func TestRunCI_NoReposNoFacts(t *testing.T) {
	ghFound(t)
	stubExec(t, func(string, string, []string) ([]byte, error) {
		t.Fatal("no exec should run with zero repos")
		return nil, nil
	})

	res := RunCI(nil, "1", time.Now())

	if len(res.Facts) != 0 {
		t.Errorf("got %d facts, want 0", len(res.Facts))
	}
	if len(res.Kinds) != 1 || res.Kinds[0] != KindCI {
		t.Errorf("Kinds = %v, want [ci] even with no repos", res.Kinds)
	}
}

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct {
		name      string
		url       string
		org, repo string
		ok        bool
	}{
		{"https with .git", "https://github.com/acme/widget.git", "acme", "widget", true},
		{"https without .git", "https://github.com/acme/widget", "acme", "widget", true},
		{"https trailing slash", "https://github.com/acme/widget/", "acme", "widget", true},
		{"http scheme", "http://github.com/acme/widget.git", "acme", "widget", true},
		{"scp-like ssh", "git@github.com:acme/widget.git", "acme", "widget", true},
		{"ssh url form", "ssh://git@github.com/acme/widget", "acme", "widget", true},
		{"non-github host https", "https://gitlab.com/acme/widget.git", "", "", false},
		{"non-github host scp", "git@bitbucket.org:acme/widget.git", "", "", false},
		{"malformed git@ no colon", "git@github.com/acme/widget", "", "", false},
		{"missing name", "https://github.com/acme", "", "", false},
		{"empty org", "https://github.com//widget", "", "", false},
		{"trailing path segment", "https://github.com/acme/widget/extra", "", "", false},
		{"unknown scheme", "ftp://github.com/acme/widget", "", "", false},
		{"bare host no scheme", "github.com/acme/widget", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			org, repo, ok := parseGitHubRemote(tc.url)
			if ok != tc.ok || org != tc.org || repo != tc.repo {
				t.Errorf("parseGitHubRemote(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tc.url, org, repo, ok, tc.org, tc.repo, tc.ok)
			}
		})
	}
}

// TestDefaultExecCommandFn exercises the real (unstubbed) exec seam against a
// binary that exits non-zero so the default path is covered without a real
// git/gh dependency: the command runs and returns an error, no panic.
func TestDefaultExecCommandFn(t *testing.T) {
	// `false` is a POSIX utility that exits 1; if it is not present the lookup
	// below skips the test rather than failing on the environment.
	if _, err := exec.LookPath("false"); err != nil {
		t.Skip("`false` not on PATH")
	}
	if _, err := ExecCommandFn(t.TempDir(), "false"); err == nil {
		t.Error("expected a non-zero exit error from `false`")
	}
}
