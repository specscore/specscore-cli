package idea

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, projectRoot, body string) {
	t.Helper()
	const header = "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n"
	if err := os.WriteFile(filepath.Join(projectRoot, "specscore.yaml"), []byte(header+body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// configurable-ideas-path#req:ideas-path-default — no config → spec/ideas.
func TestResolveIdeasDir_NoConfigDefaults(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	want := filepath.Join(specDir, "ideas")
	if got := ResolveIdeasDir(specDir); got != want {
		t.Errorf("ResolveIdeasDir = %q, want %q", got, want)
	}
	if got := ResolveSeedsDir(specDir); got != filepath.Join(want, "seeds") {
		t.Errorf("ResolveSeedsDir = %q, want %q/seeds", got, want)
	}
}

// configurable-ideas-path#req:ideas-path-default — empty config → spec/ideas.
func TestResolveIdeasDir_DefaultWithConfig(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "")
	specDir := filepath.Join(root, "spec")
	want := filepath.Join(root, "spec", "ideas")
	if got := ResolveIdeasDir(specDir); got != want {
		t.Errorf("ResolveIdeasDir = %q, want %q", got, want)
	}
}

// configurable-ideas-path#req:ideas-path-override — root module override.
func TestResolveIdeasDir_Override(t *testing.T) {
	root := t.TempDir()
	writeConfig(t, root, "modules:\n  - path: \"\"\n    path_overrides:\n      ideas_path: ideas\n")
	specDir := filepath.Join(root, "spec")
	want := filepath.Join(root, "ideas")
	if got := ResolveIdeasDir(specDir); got != want {
		t.Errorf("ResolveIdeasDir = %q, want %q", got, want)
	}
	if got := ResolveSeedsDir(specDir); got != filepath.Join(root, "ideas", "seeds") {
		t.Errorf("ResolveSeedsDir = %q, want %q", got, filepath.Join(root, "ideas", "seeds"))
	}
}
