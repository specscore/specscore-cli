package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Injectable for tests.
var (
	runGitFn    = runGit
	writeFileFn = os.WriteFile
)

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

// localIsTracked reports whether specscore.local.yaml is tracked by git in repoRoot.
func localIsTracked(repoRoot string) bool {
	return runGitFn(repoRoot, "ls-files", "--error-unmatch", LocalFile) == nil
}

// EnsureLocalGitignored makes sure specscore.local.yaml is git-ignored in
// repoRoot, appending it to .gitignore when absent. It returns whether the
// entry was added and a non-empty warning when the file is already git-tracked
// (which defeats its per-user purpose).
func EnsureLocalGitignored(repoRoot string) (added bool, warning string, err error) {
	path := filepath.Join(repoRoot, ".gitignore")
	data, rerr := os.ReadFile(path)
	if rerr != nil && !os.IsNotExist(rerr) {
		return false, "", fmt.Errorf("reading .gitignore: %w", rerr)
	}
	existing := string(data)

	if !ignoresLocal(existing) {
		var b strings.Builder
		b.WriteString(existing)
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			b.WriteString("\n")
		}
		b.WriteString(LocalFile + "\n")
		if werr := writeFileFn(path, []byte(b.String()), 0o644); werr != nil {
			return false, "", fmt.Errorf("writing .gitignore: %w", werr)
		}
		added = true
	}

	if localIsTracked(repoRoot) {
		warning = fmt.Sprintf("%s is tracked by git; run `git rm --cached %s` so it is never committed", LocalFile, LocalFile)
	}
	return added, warning, nil
}

// EnsureLocalGitignoredMsg ensures the ignore entry and returns a single
// advisory message for callers to surface (empty when there is nothing to
// report). It never returns an error — a failure is folded into the message —
// so callers need only check for a non-empty string.
func EnsureLocalGitignoredMsg(repoRoot string) string {
	_, warning, err := EnsureLocalGitignored(repoRoot)
	if err != nil {
		return "could not update .gitignore: " + err.Error()
	}
	return warning
}

// ignoresLocal reports whether content already ignores specscore.local.yaml.
func ignoresLocal(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == LocalFile {
			return true
		}
	}
	return false
}
