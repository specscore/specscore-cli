// Package runner discovers rehearse scenario files, executes their step
// blocks through the block-executor registry, and renders the run report
// (REQ: scenario-discovery, REQ: scenario-shape, REQ: run-report).
package runner

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
)

// Discover resolves the run's scenario files (REQ: scenario-discovery).
// Each path may be a file, a directory (scanned recursively for *.md,
// excluding README.md), or a glob. With no paths, it defaults to every
// scenario under spec/features/**/_tests/ of the SpecScore repo enclosing
// cwd. Explicit paths need no specscore.yaml (standalone mode). Zero
// discovered scenarios is a config error (exit 2), not an empty pass.
func Discover(paths []string, cwd string) ([]string, error) {
	var files []string
	if len(paths) == 0 {
		defaults, err := discoverDefault(cwd)
		if err != nil {
			return nil, err
		}
		files = defaults
	}
	for _, path := range paths {
		expanded, err := expandArg(path)
		if err != nil {
			return nil, err
		}
		files = append(files, expanded...)
	}
	files = dedupe(files)
	if len(files) == 0 {
		return nil, exitcode.InvalidArgsError(
			"no scenario files discovered — an empty run is a config error; pass scenario files, directories, or globs")
	}
	return files, nil
}

// discoverDefault resolves the no-argument default: all scenario files
// under spec/features/**/_tests/ of the enclosing SpecScore repo.
func discoverDefault(cwd string) ([]string, error) {
	root, err := feature.FindSpecRepoRoot(cwd)
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf(
			"no paths given and no SpecScore repo found from %s — pass scenario files explicitly (standalone mode) or run inside a repo with specscore.yaml", cwd)
	}
	featuresDir := filepath.Join(root, feature.DefaultSpecDir, feature.FeaturesSubDir)
	if _, err := osStatFn(featuresDir); err != nil {
		return nil, nil // no spec/features tree: zero discovered, reported by Discover
	}
	return collectScenarios(featuresDir, true)
}

// expandArg resolves one path argument into scenario files.
func expandArg(arg string) ([]string, error) {
	if strings.ContainsAny(arg, "*?[") {
		return expandGlob(arg)
	}
	info, err := osStatFn(arg)
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf("path not found: %s", arg)
	}
	if info.IsDir() {
		return collectScenarios(arg, false)
	}
	return []string{arg}, nil
}

// expandGlob resolves a glob argument; matched directories are scanned like
// directory arguments.
func expandGlob(pattern string) ([]string, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf("bad glob pattern %q: %v", pattern, err)
	}
	var files []string
	for _, m := range matches {
		info, err := osStatFn(m)
		if err != nil {
			return nil, exitcode.InvalidArgsErrorf("resolving glob match %s: %v", m, err)
		}
		if info.IsDir() {
			nested, err := collectScenarios(m, false)
			if err != nil {
				return nil, err
			}
			files = append(files, nested...)
			continue
		}
		files = append(files, m)
	}
	return files, nil
}

// collectScenarios walks dir recursively collecting *.md scenario files,
// excluding README.md. With testsOnly set, only files under a `_tests`
// directory segment qualify (the in-repo default discovery).
func collectScenarios(dir string, testsOnly bool) ([]string, error) {
	var files []string
	err := walkDirFn(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		// `.check.md` files are reusable checks (Feature: cli/rehearse/reusable-checks),
		// not scenarios — they are invoked via `**Use:**`, never run directly.
		if !strings.HasSuffix(name, ".md") || name == "README.md" || strings.HasSuffix(name, ".check.md") {
			return nil
		}
		if testsOnly && !strings.Contains(filepath.ToSlash(path), "/_tests/") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, exitcode.InvalidArgsErrorf("scanning %s: %v", dir, err)
	}
	sort.Strings(files)
	return files, nil
}

// dedupe removes duplicate paths, keeping first-seen order.
func dedupe(files []string) []string {
	seen := make(map[string]bool, len(files))
	out := files[:0]
	for _, f := range files {
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}
