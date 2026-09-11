package planstore

// This file closes out statement-coverage gaps left by resolve_test.go's
// happy-path/precedence focus: every remaining error branch in resolve.go
// (malformed layer files, unsafe or ambiguous identities, checkout
// validation failures, and the secure-path-join escape checks) gets a
// dedicated, minimal reproduction here so the repo's 100% coverage gate
// (scripts/coverage-gate.sh) passes without weakening any check.
//
// installFakeGit lets a handful of tests simulate git-plumbing output
// combinations (a toplevel/common-dir that resolves to a missing or
// non-".git" path) that are impractical to reproduce with a real
// repository, while every other git invocation during that test still runs
// the real binary.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGitOverride pairs a substring match against the invoked git argv
// (joined with spaces, "-C <dir>" included) with a canned stdout and exit
// status.
type fakeGitOverride struct {
	match  string
	stdout string
	exit   int
}

// installFakeGit puts a shell-script "git" shim ahead of the real binary on
// PATH for the duration of the calling test. Any invocation whose argv
// contains one of overrides' match substrings gets the corresponding canned
// stdout/exit instead of running real git; everything else passes through
// to the real binary untouched.
func installFakeGit(t *testing.T, overrides ...fakeGitOverride) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("#!/bin/sh\nargs=\"$*\"\ncase \"$args\" in\n")
	for _, o := range overrides {
		fmt.Fprintf(&b, "  *\"%s\"*)\n    printf '%%s' \"%s\"\n    exit %d\n    ;;\n", o.match, o.stdout, o.exit)
	}
	fmt.Fprintf(&b, "  *)\n    exec '%s' \"$@\"\n    ;;\nesac\n", real)

	dir := t.TempDir()
	shim := filepath.Join(dir, "git")
	if err := os.WriteFile(shim, []byte(b.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// --- OrgConfigPath -----------------------------------------------------

func TestOrgConfigPathGitRepo(t *testing.T) {
	projects := t.TempDir()
	root := filepath.Join(projects, "acme", "app")
	initRepo(t, root, "git@github.com:acme/app.git")
	got, err := OrgConfigPath(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realPath(t, projects), "acme", OrgConfigFile)
	if got != want {
		t.Fatalf("OrgConfigPath = %s, want %s", got, want)
	}
}

func TestOrgConfigPathNonGitFallback(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "not-a-repo")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := OrgConfigPath(sub)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(filepath.Dir(sub), OrgConfigFile)
	if got != want {
		t.Fatalf("OrgConfigPath fallback = %s, want %s", got, want)
	}
}

// --- Resolve: top-level plumbing and layer-read failures ----------------

func TestResolveNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Resolve(dir, ReadOnly); err == nil {
		t.Fatal("expected error resolving a non-git directory")
	}
}

func TestResolveSourceIdentityUnresolvable(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	// No "origin" remote at all.
	if _, err := Resolve(root, ReadOnly); err == nil || !strings.Contains(err.Error(), "resolve source project identity") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveUserHomeDirError(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", "")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "resolve user home") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveUserLayerParseError(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, UserConfigFile), "a: [1,2\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveOrgLayerParseError(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(projects, "acme", OrgConfigFile), "a: [1,2\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveLocalLayerParseError(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, LocalConfigFile), "a: [1,2\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "parse config") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRepoLayerUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot simulate an unreadable file while running as root")
	}
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	cfgPath := filepath.Join(source, RepoConfigFile)
	write(t, cfgPath, "plans_repo: acme/app\n")
	if err := os.Chmod(cfgPath, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(cfgPath, 0o644) }()
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("err = %v", err)
	}
}

// --- Resolve: same-repository plans-dir construction ---------------------

func TestResolveSameRepoPlansDirEscapesThroughSymlink(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/app\n")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(source, "spec")); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "escapes checkout through symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

// --- Resolve: external checkout validation --------------------------------

func TestResolveExternalNoCheckoutConfigured(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	destinationRepo := filepath.Join(projects, "acme", "plans")
	initRepo(t, source, "git@github.com:acme/app.git")
	initRepo(t, destinationRepo, "git@github.com:acme/plans.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "has no local checkout") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveExternalCheckoutNotAGitRepo(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	notGit := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	write(t, filepath.Join(source, LocalConfigFile), "repo_checkouts:\n  acme/plans: "+notGit+"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "plans checkout") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveExternalCheckoutBareRepoRootFails(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	bare := filepath.Join(projects, "plans.git")
	git(t, projects, "init", "--bare", "-b", "main", "plans.git")
	git(t, bare, "remote", "add", "origin", "git@github.com:acme/plans.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	write(t, filepath.Join(source, LocalConfigFile), "repo_checkouts:\n  acme/plans: "+bare+"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "resolve plans checkout root") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveExternalCheckoutNestedPathRejected(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	destRepo := filepath.Join(projects, "acme", "plans")
	initRepo(t, destRepo, "git@github.com:acme/plans.git")
	nested := filepath.Join(destRepo, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	write(t, filepath.Join(source, LocalConfigFile), "repo_checkouts:\n  acme/plans: "+nested+"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "must name the repository root") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveExternalPlansDirEscapesThroughSymlink(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	destRepo := filepath.Join(projects, "acme", "plans")
	initRepo(t, destRepo, "git@github.com:acme/plans.git")
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(destRepo, "spec")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	write(t, filepath.Join(source, LocalConfigFile), "repo_checkouts:\n  acme/plans: "+destRepo+"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "escapes checkout through symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolvePlansRepoEmptyIdentity(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: \"\"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "non-empty repository identity") {
		t.Fatalf("err = %v", err)
	}
}

// --- matchPlanRepos direct unit tests --------------------------------------

func TestMatchPlanReposValidationErrors(t *testing.T) {
	const source = "github.com/acme/app"
	cases := []struct {
		name    string
		data    map[string]any
		wantErr string
		found   bool
	}{
		{"absent", map[string]any{}, "", false},
		{"not a map", map[string]any{"plan_repos": "oops"}, "must map destination repositories", false},
		{"invalid destination", map[string]any{"plan_repos": map[string]any{"///": []any{"acme/app"}}}, "invalid plan_repos destination", false},
		{"not a list", map[string]any{"plan_repos": map[string]any{"acme/plans": "notalist"}}, "must be a list", false},
		{"entry not a string", map[string]any{"plan_repos": map[string]any{"acme/plans": []any{123}}}, "entries must be repository identities", false},
		{"invalid source entry", map[string]any{"plan_repos": map[string]any{"acme/plans": []any{"///"}}}, "invalid source repository", false},
		{"no matching source", map[string]any{"plan_repos": map[string]any{"acme/plans": []any{"other/repo"}}}, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := layer{path: "test.yaml", data: c.data}
			dest, found, err := matchPlanRepos(l, source)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected err: %v", err)
				}
				if found != c.found {
					t.Fatalf("found = %v, dest = %s", found, dest)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, c.wantErr)
			}
		})
	}
}

// --- resolveCheckout direct unit tests -------------------------------------

func TestResolveCheckoutValidationErrors(t *testing.T) {
	const destination = "github.com/acme/plans"
	const source = "github.com/acme/app"
	cases := []struct {
		name    string
		data    map[string]any
		wantErr string
	}{
		{"not a map", map[string]any{"repo_checkouts": "oops"}, "must map repository identities"},
		{"invalid key", map[string]any{"repo_checkouts": map[string]any{"///": "/abs"}}, "invalid repo_checkouts key"},
		{"not absolute", map[string]any{"repo_checkouts": map[string]any{"acme/plans": "relative"}}, "must be an absolute path"},
		{"duplicate normalized entries", map[string]any{"repo_checkouts": map[string]any{"ACME/plans": "/abs/one", "acme/PLANS": "/abs/two"}}, "duplicate normalized checkout entries"},
		{"nonexistent checkout path", map[string]any{"repo_checkouts": map[string]any{"acme/plans": "/definitely/does/not/exist/checkout-zzz"}}, "resolve plans checkout"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := layer{path: "test.yaml", data: c.data}
			if _, _, err := resolveCheckout(destination, source, l); err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("err = %v, want contains %q", err, c.wantErr)
			}
		})
	}
}

func TestResolveCheckoutNoneConfigured(t *testing.T) {
	if _, _, err := resolveCheckout("github.com/acme/plans", "github.com/acme/app"); err == nil || !strings.Contains(err.Error(), "has no local checkout") {
		t.Fatalf("err = %v", err)
	}
	// A layer with unrelated (or absent) repo_checkouts data also falls
	// through to the same "no local checkout" outcome.
	l := layer{path: "test.yaml", data: map[string]any{}}
	if _, _, err := resolveCheckout("github.com/acme/plans", "github.com/acme/app", l); err == nil || !strings.Contains(err.Error(), "has no local checkout") {
		t.Fatalf("err = %v", err)
	}
	// A single (non-ambiguous, so deterministic regardless of Go's map
	// iteration order) entry naming a repository other than destination must
	// be skipped, not mistaken for a match.
	nonMatching := layer{path: "test.yaml", data: map[string]any{"repo_checkouts": map[string]any{"other/repo": "/abs/unrelated"}}}
	if _, _, err := resolveCheckout("github.com/acme/plans", "github.com/acme/app", nonMatching); err == nil || !strings.Contains(err.Error(), "has no local checkout") {
		t.Fatalf("err = %v", err)
	}
}

// --- normalizeRepo direct unit tests ---------------------------------------

func TestNormalizeRepoValidationErrors(t *testing.T) {
	cases := []struct{ raw, wantErr string }{
		{"a/b/c/d", "owner/repo or host/owner/repo"},
		{"a/..", "unsafe repository identity segment"},
	}
	for _, c := range cases {
		if _, err := normalizeRepo(c.raw, "github.com"); err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("normalizeRepo(%q) err = %v, want contains %q", c.raw, err, c.wantErr)
		}
	}
}

// --- repositoryIdentity direct unit tests -----------------------------------

func TestRepositoryIdentityNoOriginRemote(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	if _, err := repositoryIdentity(root); err == nil {
		t.Fatal("expected error for a repository without an origin remote")
	}
}

func TestRepositoryIdentityUnparseableOrigin(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "remote", "add", "origin", "/local/path/not/a/url")
	if _, err := repositoryIdentity(root); err == nil || !strings.Contains(err.Error(), "unsupported origin remote") {
		t.Fatalf("err = %v", err)
	}
}

// --- repositoryRoots direct unit tests, including fake-git plumbing --------

func TestRepositoryRootsNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := repositoryRoots(dir); err == nil || !strings.Contains(err.Error(), "resolve source repository root") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryRootsRootSymlinkResolutionFails(t *testing.T) {
	installFakeGit(t, fakeGitOverride{match: "rev-parse --show-toplevel", stdout: "/definitely/does/not/exist/root-zzz", exit: 0})
	if _, _, err := repositoryRoots(t.TempDir()); err == nil || !strings.Contains(err.Error(), "resolve source repository path") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryRootsCommonDirCommandFails(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, "git@github.com:acme/app.git")
	installFakeGit(t, fakeGitOverride{match: "--git-common-dir", exit: 1})
	if _, _, err := repositoryRoots(root); err == nil || !strings.Contains(err.Error(), "resolve source common git directory") {
		t.Fatalf("err = %v", err)
	}
}

func TestRepositoryRootsCommonDirSymlinkResolutionFails(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, "git@github.com:acme/app.git")
	installFakeGit(t, fakeGitOverride{match: "--git-common-dir", stdout: "/definitely/does/not/exist/common-zzz", exit: 0})
	if _, _, err := repositoryRoots(root); err == nil || !strings.Contains(err.Error(), "resolve source common git directory") {
		t.Fatalf("err = %v", err)
	}
}

// --- secureJoin direct unit tests ------------------------------------------

func TestSecureJoinUnsafeSegment(t *testing.T) {
	if _, err := secureJoin(t.TempDir(), "..", "plans"); err == nil || !strings.Contains(err.Error(), "unsafe plans path segment") {
		t.Fatalf("err = %v", err)
	}
}

func TestSecureJoinRootSymlinkResolutionFails(t *testing.T) {
	if _, err := secureJoin("/definitely/does/not/exist/secure-root-zzz", "plans"); err == nil || !strings.Contains(err.Error(), "resolve checkout") {
		t.Fatalf("err = %v", err)
	}
}

func TestSecureJoinInspectAncestorPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	root := t.TempDir()
	restricted := filepath.Join(root, "restricted")
	if err := os.Mkdir(restricted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restricted, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(restricted, 0o755) }()
	// "plans" does not exist under "restricted", and "restricted" itself
	// lacks the search (x) permission needed to even ask whether it does —
	// os.Lstat surfaces that as a permission error, not "not exist".
	if _, err := secureJoin(root, "restricted", "plans"); err == nil || !strings.Contains(err.Error(), "inspect plans path") {
		t.Fatalf("err = %v", err)
	}
}

// --- Hard-to-reach defensive branches, exercised via a deleted cwd --------

// withDeletedCwd chdirs into a throwaway directory, deletes it out from
// under the process, runs fn, and restores the original working directory
// afterward. It lets tests exercise the (normally unreachable in practice)
// error paths that fire when os.Getwd/lstat-relative-to-cwd operations fail
// because the working directory no longer exists.
func withDeletedCwd(t *testing.T, fn func()) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dead, err := os.MkdirTemp("", "planstore-dead-cwd-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dead); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(orig); err != nil {
			t.Fatal(err)
		}
	}()
	if err := os.RemoveAll(dead); err != nil {
		t.Fatal(err)
	}
	fn()
}

func TestOrgConfigPathAbsFailsWhenCwdRemoved(t *testing.T) {
	withDeletedCwd(t, func() {
		if _, err := OrgConfigPath("relative-nonexistent-source"); err == nil {
			t.Skip("filepath.Abs did not fail with a deleted cwd on this platform")
		}
	})
}

func TestSecureJoinAncestorSymlinkResolutionFails(t *testing.T) {
	root := t.TempDir()
	// "brokenlink" exists (as a symlink) but its target does not: Lstat
	// on the missing "brokenlink/child" fails not-exist, so the ancestor
	// walk backs up to "brokenlink" itself, where Lstat succeeds (the
	// symlink file is really there) — but resolving it fully still fails,
	// since its target doesn't exist.
	if err := os.Symlink(filepath.Join(root, "does-not-exist-zzz"), filepath.Join(root, "brokenlink")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "brokenlink", "child")
	if _, err := secureJoin(root, "brokenlink", "child"); err == nil || !strings.Contains(err.Error(), "resolve plans path ancestor") {
		t.Fatalf("secureJoin(%q) err = %v, want \"resolve plans path ancestor\"", target, err)
	}
}

func TestRepositoryRootsNonGitCommonDirRejected(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root, "git@github.com:acme/app.git")
	// A common-dir that resolves to a real, existing directory whose base
	// name isn't ".git" (e.g. a bare repository's own directory) must be
	// rejected as "not a non-bare checkout" rather than accepted.
	installFakeGit(t, fakeGitOverride{match: "--git-common-dir", stdout: root, exit: 0})
	if _, _, err := repositoryRoots(root); err == nil || !strings.Contains(err.Error(), "not a non-bare checkout") {
		t.Fatalf("err = %v", err)
	}
}
