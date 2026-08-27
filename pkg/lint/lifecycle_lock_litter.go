package lint

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/config"
)

// lifecycleLockLitterChecker flags a per-artifact lifecycle-transaction lock
// file that has been committed to git.
//
// pkg/lifecycle.TransformArtifact (used by every artifact-mutating command,
// including `specscore spec lint --fix`) acquires an flock on a sibling file
// named ".<artifact>.lifecycle-transaction.lock". Clean transactions remove
// it while holding the stable project lock; an interrupted transaction may
// leave it behind for the next run to reclaim safely. That is harmless as long
// as the file stays untracked: config.EnsureLocalGitignored writes the required
// ignore pattern during `specscore init`, but nothing re-checks it on later
// commands, so a repo initialized before that pattern existed (or one where
// .gitignore drifted) can have a `git add -A`/`git add .` sweep these zero-byte
// lock files into a commit. That is exactly what happened to
// sneat-co/backstage on 2026-08-26 (90 files landed by a `lint --fix` sync
// commit). This rule makes that failure mode fail lint/CI instead of landing
// silently.
//
// Untracked crash leftovers are expected, harmless disk state and are never
// reported — only a committed one is a defect.
type lifecycleLockLitterChecker struct {
	isTracked func(repoRoot, relPath string) (bool, error)
}

func newLifecycleLockLitterChecker() checker {
	return &lifecycleLockLitterChecker{isTracked: gitFileTracked}
}

func (c *lifecycleLockLitterChecker) name() string     { return "lifecycle-lock-committed" }
func (c *lifecycleLockLitterChecker) severity() string { return "error" }

// isLifecycleTransactionLockName reports whether name matches the lock
// filename pkg/lifecycle/cas.go derives for an artifact: a leading dot, the
// literal artifact basename, and a ".lifecycle-transaction.lock" suffix.
func isLifecycleTransactionLockName(name string) bool {
	return strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".lifecycle-transaction.lock")
}

// Package-level seams, swapped in tests to drive defensive error branches
// that real filesystem/git state cannot reliably reproduce on every host
// (e.g. chmod 000 is a no-op when tests run as root; see
// test_seams_decision.go for the same convention used elsewhere in this
// package).
var (
	lifecycleLockReadDir      = os.ReadDir
	lifecycleLockEvalSymlinks = filepath.EvalSymlinks
	lifecycleLockRel          = filepath.Rel
)

// gitFileTracked reports whether relPath (relative to repoRoot) is tracked by
// git at repoRoot. Injectable for testing.
var gitFileTracked = func(repoRoot, relPath string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", "--", relPath)
	cmd.Dir = repoRoot
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Non-zero exit from `ls-files --error-unmatch` means untracked;
			// any other failure (git missing, not a repo) is surfaced.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *lifecycleLockLitterChecker) check(specRoot string) ([]Violation, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = specRoot
	topOut, err := cmd.Output()
	if err != nil {
		// Not a git repo (or git unavailable): a committed-file check has
		// nothing to compare against, so it stays silent rather than error.
		return nil, nil
	}
	repoRoot := strings.TrimSpace(string(topOut))

	var violations []Violation
	walkErr := walkSpecDirs(specRoot, func(dirPath, relPath string) error {
		entries, rerr := lifecycleLockReadDir(dirPath)
		if rerr != nil {
			return rerr
		}
		for _, entry := range entries {
			if entry.IsDir() || !isLifecycleTransactionLockName(entry.Name()) {
				continue
			}
			full := filepath.Join(dirPath, entry.Name())
			// repoRoot came back from `git rev-parse`, which resolves
			// symlinks (e.g. macOS /tmp -> /private/tmp); canonicalize full
			// the same way or filepath.Rel computes garbage across mismatched
			// prefixes and the rule silently no-ops.
			canonFull, evalErr := lifecycleLockEvalSymlinks(full)
			if evalErr != nil {
				canonFull = full
			}
			relToRepo, relErr := lifecycleLockRel(repoRoot, canonFull)
			if relErr != nil {
				continue
			}
			relToRepo = filepath.ToSlash(relToRepo)
			tracked, trackErr := c.isTracked(repoRoot, relToRepo)
			if trackErr != nil {
				return trackErr
			}
			if !tracked {
				continue
			}
			violations = append(violations, Violation{
				File:     filepath.ToSlash(filepath.Join(relPath, entry.Name())),
				Line:     0,
				Severity: "error",
				Rule:     "lifecycle-lock-committed",
				Message: fmt.Sprintf(
					"%s is a committed lifecycle-transaction lock file (a zero-byte flock identity, never spec content); after confirming no lifecycle transaction currently holds it, run `git rm --cached -- %s` and ensure .gitignore carries %q",
					relToRepo, relToRepo, config.LifecycleTransactionLockIgnorePattern,
				),
			})
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].File < violations[j].File })
	return violations, nil
}
