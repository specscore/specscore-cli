// Package dryrun is the shared mechanism behind `--dry-run` on every
// `specscore <kind> change-status` verb (see
// spec/features/cli/lifecycle-transitions/README.md#req-dry-run-mode).
//
// The per-kind ChangeStatus orchestrators (pkg/feature, pkg/idea, pkg/plan,
// pkg/lesson, pkg/issue, pkg/sidekick) are NOT unified behind one function —
// each kind has its own artifact layout and Options shape. What IS shared is
// how a preview is produced: Sandbox copies the project's spec/ subtree into
// a throwaway temporary directory, lets the caller run the EXACT SAME
// mutation code it would run for real — just pointed at the copy — and
// diffs the result. Because dry-run and the real path share the identical
// mutation function, the reported file list cannot drift from what a
// subsequent real run actually touches; it is not a separate, hand-maintained
// prediction of what "should" happen.
//
// The real project root is never opened for writing by this package.
package dryrun

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ChangeKind classifies how a file differs between the pre-mutation and
// post-mutation snapshot of the spec tree, mirroring `git status --short`'s
// single-letter vocabulary.
type ChangeKind string

const (
	// Modified means the file exists on both sides with different content.
	Modified ChangeKind = "M"
	// Added means the file exists only in the post-mutation snapshot.
	Added ChangeKind = "A"
	// Removed means the file exists only in the pre-mutation snapshot.
	Removed ChangeKind = "D"
)

// Change names one file a mutation would create, modify, or delete. Path is
// relative to the project root (it carries the leading "spec/" segment), so
// it can be handed directly to `git diff <path>` or `git add <path>`.
type Change struct {
	Kind ChangeKind
	Path string
}

// String renders a Change the way PrintReport does: "<kind> <path>".
func (c Change) String() string {
	return string(c.Kind) + " " + c.Path
}

// exitCoder mirrors the unexported interface internal/cli/root.go's Fatal
// uses to map an error to a process exit code. Matching its shape (rather
// than depending on internal/cli) lets Sandbox preserve the exact exit code
// a real invocation would have produced.
type exitCoder interface {
	ExitCode() int
}

// Sandbox copies the "spec" subdirectory of root into a temporary directory,
// invokes mutate with that temporary directory's path standing in for root,
// computes the file-level Changes mutate made (diffing the temporary spec/
// tree against root's spec/ tree AFTER mutate returns), and removes the
// temporary directory before returning. root — and everything under it — is
// NEVER written to; mutate MUST perform all of its I/O by deriving paths
// from the sandboxRoot argument it receives, exactly as the real (non-dry-
// run) call site derives them from root.
//
// If mutate returns an error, Sandbox returns that error with any occurrence
// of the sandbox's temporary path rewritten back to root, so the message
// reads identically to what a real, non-sandboxed invocation would have
// printed — a caller comparing dry-run output to real output (or a script
// testing transition legality via --dry-run) sees no sandbox artifact leak
// through. The error's exit code (via an ExitCode() int method, e.g.
// *pkg/exitcode.Error) is preserved.
func Sandbox[T any](root string, mutate func(sandboxRoot string) (T, error)) (T, []Change, error) {
	var zero T

	realSpecDir := filepath.Join(root, "spec")
	tempRoot, err := os.MkdirTemp("", "specscore-dry-run-*")
	if err != nil {
		return zero, nil, fmt.Errorf("dry-run: creating sandbox: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()

	sandboxSpecDir := filepath.Join(tempRoot, "spec")
	if err := copyTree(realSpecDir, sandboxSpecDir); err != nil {
		return zero, nil, fmt.Errorf("dry-run: staging sandbox copy: %w", err)
	}

	result, mutateErr := mutate(tempRoot)
	if mutateErr != nil {
		return zero, nil, rewriteSandboxPath(mutateErr, tempRoot, root)
	}

	changes, err := diffTrees(realSpecDir, sandboxSpecDir)
	if err != nil {
		return zero, nil, fmt.Errorf("dry-run: computing changed files: %w", err)
	}
	return result, changes, nil
}

// rewriteSandboxPath replaces every occurrence of sandboxRoot in err's
// message with realRoot, preserving the error's exit code (if any) via
// exitcode.New. An error whose message contains no sandbox-path occurrence
// is returned unchanged (still wrapped in a plain error if it originally
// carried no exit code, so callers see no observable difference).
func rewriteSandboxPath(err error, sandboxRoot, realRoot string) error {
	msg := err.Error()
	rewritten := strings.ReplaceAll(msg, sandboxRoot, realRoot)
	var ec exitCoder
	if errors.As(err, &ec) {
		return exitcode.New(ec.ExitCode(), rewritten)
	}
	if rewritten != msg {
		return errors.New(rewritten)
	}
	return err
}

// copyTree recursively copies src to dst, preserving file mode. A missing
// src is treated as an empty tree (dst is simply not created) rather than an
// error, since a brand-new project may not yet have every ancestor
// directory a kind expects.
func copyTree(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("dry-run: %s is not a directory", src)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, fi.Mode().Perm()|0o700)
		}
		return copyFile(path, target, d)
	})
}

// copyFile copies one regular file, preserving its mode bits.
func copyFile(src, dst string, d os.DirEntry) error {
	fi, err := d.Info()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// diffTrees walks oldDir and newDir (the pre- and post-mutation copies of
// the same "spec" subtree) and returns every path that differs, sorted
// lexically. Returned paths are prefixed with "spec/" so they read as
// project-root-relative, matching what a caller would pass to git.
func diffTrees(oldDir, newDir string) ([]Change, error) {
	oldFiles, err := listFiles(oldDir)
	if err != nil {
		return nil, err
	}
	newFiles, err := listFiles(newDir)
	if err != nil {
		return nil, err
	}

	var changes []Change
	for rel := range newFiles {
		projPath := filepath.ToSlash(filepath.Join("spec", rel))
		if _, existed := oldFiles[rel]; !existed {
			changes = append(changes, Change{Kind: Added, Path: projPath})
			continue
		}
		oldBytes, err := os.ReadFile(filepath.Join(oldDir, rel))
		if err != nil {
			return nil, err
		}
		newBytes, err := os.ReadFile(filepath.Join(newDir, rel))
		if err != nil {
			return nil, err
		}
		if string(oldBytes) != string(newBytes) {
			changes = append(changes, Change{Kind: Modified, Path: projPath})
		}
	}
	for rel := range oldFiles {
		if _, still := newFiles[rel]; !still {
			projPath := filepath.ToSlash(filepath.Join("spec", rel))
			changes = append(changes, Change{Kind: Removed, Path: projPath})
		}
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, nil
}

// listFiles returns the set of regular-file paths under dir, relative to
// dir, using forward slashes. A missing dir yields an empty set.
func listFiles(dir string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// PrintReport writes the standard dry-run report to w: a first line shaped
// like the real success line ("<id>: <from> → <to>"), annotated with a
// dry-run marker and the changed-file count, followed by one indented
// git-status-style line per Change ("  M spec/ideas/foo.md"), in Changes'
// existing (path-sorted) order.
func PrintReport(w io.Writer, id, from, to string, changes []Change) {
	fmt.Fprintf(w, "%s: %s → %s (dry-run; would touch %d file(s))\n", id, from, to, len(changes))
	for _, c := range changes {
		fmt.Fprintf(w, "  %s\n", c.String())
	}
}
