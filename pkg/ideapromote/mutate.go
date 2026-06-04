package ideapromote

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// gitMV runs `git -C <repoRoot> mv <srcRel> <dstRel>` so the rename is
// recorded for history follow. When the repo is not under git (no work
// tree), it falls back to a plain os.Rename so the verb still works in a
// non-git workspace, mirroring the relocate verb's git-tolerance.
func gitMV(repoRoot, srcRel, dstRel string) error {
	dstAbs := filepath.Join(repoRoot, dstRel)
	if err := os.MkdirAll(filepath.Dir(dstAbs), 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating dir %s: %v", filepath.Dir(dstAbs), err)
	}
	if isGitRepoFn(repoRoot) {
		cmd := exec.Command("git", "-C", repoRoot, "mv", srcRel, dstRel)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return exitcode.UnexpectedErrorf("git mv %s %s: %v: %s",
				srcRel, dstRel, err, stderr.String())
		}
		return nil
	}
	if err := os.Rename(filepath.Join(repoRoot, srcRel), dstAbs); err != nil {
		return exitcode.UnexpectedErrorf("rename %s -> %s: %v", srcRel, dstRel, err)
	}
	return nil
}

// writeFileAbs overwrites path with content (0644), creating parent dirs.
func writeFileAbs(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", path, err)
	}
	return nil
}

// SameRepoPromote performs the same-repo path: git-mv the seed to the
// Idea path then overwrite it with the transformed Idea body. Returns the
// absolute destination Idea path.
//
// In a git repo, the pure rename is committed BEFORE the transform is
// written, so the move is recorded as a 100%-similarity rename that
// `git log --follow spec/ideas/<slug>.md` reaches even though the
// subsequent transform rewrites the file substantially. The transform is
// then staged (committing it is left to the caller / lint-fix step). In a
// non-git workspace the rename is a plain os.Rename and history is moot.
func SameRepoPromote(specRoot, slug string, transformed []byte) (string, error) {
	paths := PathsFor(slug)
	if err := gitMV(specRoot, paths.SeedRel, paths.IdeaRel); err != nil {
		return "", err
	}
	dstAbs := filepath.Join(specRoot, paths.IdeaRel)

	// Commit the pure rename so default rename detection (--follow)
	// links the Idea path back to the seed across the upcoming transform.
	if isGitRepoFn(specRoot) {
		if err := gitCommit(specRoot,
			"chore(promote): move seed "+slug+" to idea (rename)"); err != nil {
			return "", err
		}
	}

	if err := writeFileAbs(dstAbs, transformed); err != nil {
		return "", err
	}
	if err := gitStage(specRoot, paths.IdeaRel); err != nil {
		return "", err
	}
	return dstAbs, nil
}

// gitCommit stages everything and commits with the given subject. A
// no-op outside a git work tree.
func gitCommit(repoRoot, subject string) error {
	if !isGitRepoFn(repoRoot) {
		return nil
	}
	if err := gitStageAll(repoRoot); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repoRoot, "commit", "--no-gpg-sign", "-m", subject)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return exitcode.UnexpectedErrorf("git commit: %v: %s", err, stderr.String())
	}
	return nil
}

// gitStageAll runs `git add -A`. A no-op outside a git work tree.
func gitStageAll(repoRoot string) error {
	if !isGitRepoFn(repoRoot) {
		return nil
	}
	cmd := exec.Command("git", "-C", repoRoot, "add", "-A")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return exitcode.UnexpectedErrorf("git add -A: %v: %s", err, stderr.String())
	}
	return nil
}

// gitStage runs `git add -- <relPaths>`. A no-op outside a git work tree.
func gitStage(repoRoot string, relPaths ...string) error {
	if !isGitRepoFn(repoRoot) || len(relPaths) == 0 {
		return nil
	}
	args := append([]string{"-C", repoRoot, "add", "--"}, relPaths...)
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return exitcode.UnexpectedErrorf("git add: %v: %s", err, stderr.String())
	}
	return nil
}

// isGitRepoFn reports whether repoRoot is inside a git work tree.
var isGitRepoFn = func(repoRoot string) bool {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree")
	return cmd.Run() == nil
}
