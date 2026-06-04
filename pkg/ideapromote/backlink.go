package ideapromote

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// BackLink is one `## Sidekick Seeds Generated` entry that references the
// promoted seed.
type BackLink struct {
	// RepoRoot is the absolute root of the repo containing the entry.
	RepoRoot string
	// RelPath is the repo-relative path of the file containing the entry.
	RelPath string
	// Line is the 0-based index of the entry line within the file.
	Line int
	// Target is the raw markdown link target (the URL between the
	// parentheses).
	Target string
	// CrossRepo is true when the target is `<repo-slug>:`-qualified
	// (per sidekick-capture's cross-repo back-link format); false when
	// it is a bare relative path (same-repo).
	CrossRepo bool
}

// repoQualifiedRe matches a leading `<repo-slug>:` prefix on a link
// target, where <repo-slug> is the SpecScore slug grammar. A bare URL
// scheme like `https://` is NOT a repo-slug prefix (the `//` after the
// colon disqualifies it), so it is classified same-repo handling skips
// it anyway since it won't end in the seed path.
var repoQualifiedRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*:(?:[^/]|$)`)

// DiscoverBackLinks scans the `## Sidekick Seeds Generated` sections of
// every Markdown file under spec/ in each repo for list entries whose
// link target references spec/ideas/seeds/<slug>.md, returning each as a
// classified BackLink. repos is the set of repos to scan (source +
// siblings).
func DiscoverBackLinks(repos []RepoRef, slug string) ([]BackLink, error) {
	var out []BackLink
	for _, repo := range repos {
		links, err := discoverBackLinksInRepo(repo.Path, slug)
		if err != nil {
			return nil, err
		}
		out = append(out, links...)
	}
	return out, nil
}

// RepoRef is a minimal repo handle for back-link scanning.
type RepoRef struct {
	Path string
}

// linkRe captures the target of a markdown link: [text](target).
var linkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)

// filepathWalkFn and filepathRelFn are seams over the stdlib so the
// otherwise-defensive walk-error and relativize-error branches are testable.
var (
	filepathWalkFn = filepath.Walk
	filepathRelFn  = filepath.Rel
)

func discoverBackLinksInRepo(repoRoot, slug string) ([]BackLink, error) {
	specDir := filepath.Join(repoRoot, "spec")
	info, err := os.Stat(specDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}
	seedSuffix := "ideas/seeds/" + slug + ".md"

	var out []BackLink
	walkErr := filepathWalkFn(specDir, func(path string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(body), "\n")
		inSection := false
		for i, line := range lines {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "## ") {
				inSection = strings.EqualFold(
					strings.TrimSpace(strings.TrimPrefix(t, "## ")),
					"Sidekick Seeds Generated")
				continue
			}
			if !inSection {
				continue
			}
			m := linkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			target := m[1]
			if !targetReferencesSeed(target, seedSuffix, slug) {
				continue
			}
			rel, err := filepathRelFn(repoRoot, path)
			if err != nil {
				continue
			}
			out = append(out, BackLink{
				RepoRoot:  repoRoot,
				RelPath:   filepath.ToSlash(rel),
				Line:      i,
				Target:    target,
				CrossRepo: repoQualifiedRe.MatchString(target),
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, exitcode.UnexpectedErrorf("scanning %s for back-links: %v", specDir, walkErr)
	}
	return out, nil
}

// targetReferencesSeed reports whether a link target points at the
// promoted seed. A same-repo target ends in ideas/seeds/<slug>.md; a
// cross-repo target is `<repo-slug>:`-qualified and also ends in
// ideas/seeds/<slug>.md after the prefix.
func targetReferencesSeed(target, seedSuffix, slug string) bool {
	clean := target
	if repoQualifiedRe.MatchString(target) {
		if idx := strings.IndexByte(target, ':'); idx >= 0 {
			clean = target[idx+1:]
		}
	}
	clean = strings.TrimSuffix(clean, "/")
	return strings.HasSuffix(clean, seedSuffix) ||
		strings.HasSuffix(clean, "/"+slug+".md") && strings.Contains(clean, "ideas/seeds/")
}

// HasCrossRepo reports whether any back-link is cross-repo. Cross-repo
// presence selects the cross-repo promotion path.
func HasCrossRepo(links []BackLink) bool {
	for _, l := range links {
		if l.CrossRepo {
			return true
		}
	}
	return false
}

// ReconcileResult records a same-repo back-link rewrite for the stdout
// summary.
type ReconcileResult struct {
	RepoRoot string
	RelPath  string
}

// ReconcileSameRepoBackLinks rewrites every same-repo back-link entry
// that pointed at spec/ideas/seeds/<slug>.md so it points at the new
// spec/ideas/<slug>.md, preserving each entry's relative-path depth.
// Cross-repo entries are left untouched. Returns the list of files
// updated.
func ReconcileSameRepoBackLinks(links []BackLink, slug string) ([]ReconcileResult, error) {
	// Group same-repo links by file so each file is rewritten once.
	byFile := map[string][]BackLink{}
	var order []string
	for _, l := range links {
		if l.CrossRepo {
			continue
		}
		key := l.RepoRoot + "\x00" + l.RelPath
		if _, ok := byFile[key]; !ok {
			order = append(order, key)
		}
		byFile[key] = append(byFile[key], l)
	}

	var results []ReconcileResult
	for _, key := range order {
		fileLinks := byFile[key]
		repoRoot := fileLinks[0].RepoRoot
		relPath := fileLinks[0].RelPath
		abs := filepath.Join(repoRoot, relPath)
		body, err := os.ReadFile(abs)
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("reading back-link file %s: %v", abs, err)
		}
		lines := strings.Split(string(body), "\n")
		modified := false
		for _, l := range fileLinks {
			if l.Line < 0 || l.Line >= len(lines) {
				continue
			}
			newTarget := rewriteSeedTargetToIdea(l.Target, slug)
			if newTarget == l.Target {
				continue
			}
			lines[l.Line] = strings.Replace(lines[l.Line], l.Target, newTarget, 1)
			modified = true
		}
		if modified {
			if err := os.WriteFile(abs, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return nil, exitcode.UnexpectedErrorf("writing back-link file %s: %v", abs, err)
			}
			results = append(results, ReconcileResult{RepoRoot: repoRoot, RelPath: relPath})
		}
	}
	return results, nil
}

// rewriteSeedTargetToIdea turns a same-repo seed-target path into the
// promoted Idea path at the same relative depth: it replaces the trailing
// `ideas/seeds/<slug>.md` with `ideas/<slug>.md`, preserving the leading
// relative prefix (e.g. ../../../).
func rewriteSeedTargetToIdea(target, slug string) string {
	oldSuffix := "ideas/seeds/" + slug + ".md"
	newSuffix := "ideas/" + slug + ".md"
	if idx := strings.LastIndex(target, oldSuffix); idx >= 0 {
		return target[:idx] + newSuffix + target[idx+len(oldSuffix):]
	}
	return target
}
