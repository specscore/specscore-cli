package ideapromote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFileT(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// AC: backlink-discovery-and-classification — same-repo (bare relative
// path) vs cross-repo (<repo-slug>: qualified) classification.
func TestDiscoverBackLinks_Classification(t *testing.T) {
	root := t.TempDir()

	// Same-repo back-link: bare relative path target.
	writeFileT(t, filepath.Join(root, "spec", "features", "a", "README.md"),
		"# Feature: A\n\n## Sidekick Seeds Generated\n\n"+
			"- [bar](../../ideas/seeds/bar.md) — captured 2026-06-01\n")
	// Cross-repo back-link: repo-slug-qualified target.
	writeFileT(t, filepath.Join(root, "spec", "features", "b", "README.md"),
		"# Feature: B\n\n## Sidekick Seeds Generated\n\n"+
			"- [bar](other-repo:spec/ideas/seeds/bar.md) — captured 2026-06-01\n")
	// A link OUTSIDE the section must be ignored.
	writeFileT(t, filepath.Join(root, "spec", "features", "c", "README.md"),
		"# Feature: C\n\n## Summary\n\nSee [bar](../../ideas/seeds/bar.md).\n")
	// A link to a DIFFERENT seed must be ignored.
	writeFileT(t, filepath.Join(root, "spec", "features", "d", "README.md"),
		"# Feature: D\n\n## Sidekick Seeds Generated\n\n"+
			"- [baz](../../ideas/seeds/baz.md) — captured 2026-06-01\n")

	links, err := DiscoverBackLinks([]RepoRef{{Path: root}}, "bar")
	if err != nil {
		t.Fatalf("DiscoverBackLinks: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("expected 2 back-links to bar, got %d: %+v", len(links), links)
	}

	var sameRepo, crossRepo int
	for _, l := range links {
		if l.CrossRepo {
			crossRepo++
		} else {
			sameRepo++
		}
	}
	if sameRepo != 1 || crossRepo != 1 {
		t.Errorf("expected 1 same-repo + 1 cross-repo; got same=%d cross=%d", sameRepo, crossRepo)
	}
	if !HasCrossRepo(links) {
		t.Errorf("HasCrossRepo should be true when a cross-repo link exists")
	}
}

func TestReconcileSameRepoBackLinks_RewritesOnlySameRepo(t *testing.T) {
	root := t.TempDir()
	sameRepoFile := filepath.Join(root, "spec", "features", "a", "README.md")
	writeFileT(t, sameRepoFile,
		"# Feature: A\n\n## Sidekick Seeds Generated\n\n"+
			"- [bar](../../ideas/seeds/bar.md) — captured\n")
	crossRepoFile := filepath.Join(root, "spec", "features", "b", "README.md")
	crossBody := "# Feature: B\n\n## Sidekick Seeds Generated\n\n" +
		"- [bar](other-repo:spec/ideas/seeds/bar.md) — captured\n"
	writeFileT(t, crossRepoFile, crossBody)

	links, err := DiscoverBackLinks([]RepoRef{{Path: root}}, "bar")
	if err != nil {
		t.Fatalf("DiscoverBackLinks: %v", err)
	}
	results, err := ReconcileSameRepoBackLinks(links, "bar")
	if err != nil {
		t.Fatalf("ReconcileSameRepoBackLinks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 reconciled file, got %d", len(results))
	}

	gotSame, _ := os.ReadFile(sameRepoFile)
	if want := "(../../ideas/bar.md)"; !strings.Contains(string(gotSame), want) {
		t.Errorf("same-repo back-link not rewritten to %q; got:\n%s", want, gotSame)
	}
	if strings.Contains(string(gotSame), "ideas/seeds/bar.md") {
		t.Errorf("same-repo seeds path should be gone; got:\n%s", gotSame)
	}
	// Cross-repo file untouched, byte-for-byte.
	gotCross, _ := os.ReadFile(crossRepoFile)
	if string(gotCross) != crossBody {
		t.Errorf("cross-repo back-link must be untouched;\nwant:\n%s\ngot:\n%s", crossBody, gotCross)
	}
}
