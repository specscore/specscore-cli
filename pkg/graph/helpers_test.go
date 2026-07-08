package graph

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

const testConfig = "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nproject:\n  title: T\n"

// repoWith writes files (repo-relative paths) under a temp dir and returns the
// repo root. A default specscore.yaml is written unless one is supplied.
func repoWith(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if _, ok := files["specscore.yaml"]; !ok {
		writeFile(t, filepath.Join(root, "specscore.yaml"), testConfig)
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fmModule builds a minimal module README.
func fmModule(id, deps string) string {
	return fmt.Sprintf("---\nkind: module\nid: %s\nname: %s\nstatus: draft\ndependsOn: %s\nsummary: s\n---\n", id, id, deps)
}

// fmArt builds a minimal collection artifact with extra frontmatter lines.
func fmArt(kind, id string, extra ...string) string {
	b := "---\nkind: " + kind + "\nid: " + id + "\nname: " + id + "\nstatus: draft\n"
	for _, e := range extra {
		b += e + "\n"
	}
	b += "summary: s\n---\n"
	return b
}

func lintRepo(t *testing.T, root string, opts ...func(*LintOptions)) LintResult {
	t.Helper()
	o := LintOptions{RepoRoot: root}
	for _, fn := range opts {
		fn(&o)
	}
	res, err := Lint(o)
	if err != nil {
		t.Fatalf("Lint error: %v", err)
	}
	return res
}

func ruleCounts(vs []lint.Violation) map[string]int {
	m := map[string]int{}
	for _, v := range vs {
		m[v.Rule]++
	}
	return m
}

func hasRule(vs []lint.Violation, rule string) bool {
	return ruleCounts(vs)[rule] > 0
}
