package planstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T, root, remote string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@example.com")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "remote", "add", "origin", remote)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "README.md")
	git(t, root, "commit", "-m", "fixture")
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestResolveExternalStoreAndExplicitWriteCheckout(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "datatug", "datatug")
	destination := filepath.Join(projects, "sneat-co", "workbench")
	initRepo(t, source, "git@github.com:datatug/datatug.git")
	initRepo(t, destination, "https://github.com/sneat-co/workbench.git")
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, UserConfigFile), "plan_repos:\n  sneat-co/workbench: [datatug/datatug]\nrepo_checkouts:\n  sneat-co/workbench: "+destination+"\n")

	got, err := Resolve(source, ReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(realPath(t, destination), "spec", "plans", "github.com", "datatug", "datatug")
	if got.PlansDir != want || got.SourceRepo != "github.com/datatug/datatug" || got.PlansRepo != "github.com/sneat-co/workbench" || !got.External {
		t.Fatalf("resolution = %#v, want plans dir %s", got, want)
	}
	writeResolution, err := Resolve(source, Write)
	if err != nil {
		t.Fatal(err)
	}
	if writeResolution.PlansCheckout != realPath(t, destination) {
		t.Fatalf("explicit canonical checkout should be usable by a non-WB client: %#v", writeResolution)
	}

	linked := filepath.Join(projects, "workbench-write")
	git(t, destination, "worktree", "add", "-b", "plan-write", linked)
	write(t, filepath.Join(source, LocalConfigFile), "repo_checkouts:\n  sneat-co/workbench: "+linked+"\n")
	got, err = Resolve(source, Write)
	if err != nil {
		t.Fatal(err)
	}
	if got.PlansCheckout != realPath(t, linked) {
		t.Fatalf("write checkout = %s, want %s", got.PlansCheckout, realPath(t, linked))
	}
}

func TestResolvePrecedenceAmbiguityMissingAndSameRepo(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, UserConfigFile), "plan_repos:\n  acme/plans: [acme/app]\n  acme/other: [github.com/acme/app]\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "multiple plans repositories") {
		t.Fatalf("ambiguity error = %v", err)
	}

	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/app\n")
	got, err := Resolve(source, Write)
	if err != nil {
		t.Fatal(err)
	}
	if got.External || got.PlansDir != filepath.Join(realPath(t, source), "spec", "plans") {
		t.Fatalf("same-repo resolution = %#v", got)
	}
	if err := os.Remove(filepath.Join(source, RepoConfigFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, UserConfigFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "no plans repository is configured") {
		t.Fatalf("missing route error = %v", err)
	}
}

func TestOrgLayerAnchorsAtCanonicalOwnerForSourceWorktree(t *testing.T) {
	projects := t.TempDir()
	canonical := filepath.Join(projects, "acme", "app")
	initRepo(t, canonical, "git@github.com:acme/app.git")
	worktree := filepath.Join(canonical, ".worktrees", "feature")
	git(t, canonical, "worktree", "add", "-b", "feature", worktree)
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(projects, "acme", OrgConfigFile), "plan_repos:\n  acme/app: [acme/app]\n")
	got, err := Resolve(worktree, Write)
	if err != nil {
		t.Fatal(err)
	}
	if got.RouteConfigPath != filepath.Join(realPath(t, projects), "acme", OrgConfigFile) || got.PlansDir != filepath.Join(realPath(t, worktree), "spec", "plans") {
		t.Fatalf("worktree resolution = %#v", got)
	}
}

func TestResolveRejectsUnsafeAndWrongCheckout(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	wrong := filepath.Join(projects, "acme", "wrong")
	initRepo(t, source, "git@github.com:acme/app.git")
	initRepo(t, wrong, "git@github.com:acme/wrong.git")
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/plans\n")
	write(t, filepath.Join(home, UserConfigFile), "repo_checkouts:\n  acme/plans: "+wrong+"\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "has origin") {
		t.Fatalf("wrong checkout error = %v", err)
	}
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: /acme/plans\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("unsafe identity error = %v", err)
	}
}

func TestResolveRejectsCommittedCheckoutPath(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: acme/app\nrepo_checkouts:\n  acme/app: /tmp/app\n")
	if _, err := Resolve(source, ReadOnly); err == nil || !strings.Contains(err.Error(), "machine-local") {
		t.Fatalf("committed checkout error = %v", err)
	}
}
