package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// configureSameRepoPlans makes root a git repository with an "origin" remote
// and a lint-clean specscore.yaml that explicitly self-routes Plans to the
// source repository (repo-config#req:plans-repo-project-selection: "omission
// does not imply same-repository storage"). It also isolates $HOME to a
// throwaway directory for the duration of the test so plan-store resolution
// never reads or is influenced by the invoking user's real ~/.specscore.yaml.
//
// Idempotent: safe to call more than once on the same root (e.g. after a test
// calls projectdef.WriteSpecConfig itself, which does a fresh marshal with no
// plans_repo field and so silently drops the self-route this helper wrote).
func configureSameRepoPlans(t *testing.T, root string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cmd := exec.Command("git", "-C", root, "init", "-b", "main")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	remoteAdd := exec.Command("git", "-C", root, "remote", "add", "origin", "git@github.com:specscore/test-fixture.git")
	if out, err := remoteAdd.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "already exists") {
			t.Fatalf("git remote: %v\n%s", err, out)
		}
	}
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: specscore\n  repo: test-fixture\nplans_repo: specscore/test-fixture\nlessons:\n  classifications: [process]\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
}
