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

// LifecycleTransactionLockIgnorePattern keeps crash-leftover flock identity
// files out of source control. Clean transactions remove their per-artifact
// lock after the stable project lock has serialized cleanup; an interrupted
// process can still leave one behind for the next run to reclaim safely.
const LifecycleTransactionLockIgnorePattern = "**/.*.lifecycle-transaction.lock"

// LifecycleProjectLockIgnorePattern keeps the stable cross-process lifecycle
// fence out of source control. It is intentionally retained after a clean
// release so an interrupted process can be distinguished from a missing lock
// identity without relying on pathname creation races.
const LifecycleProjectLockIgnorePattern = ".specscore-lifecycle.lock"

func runGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}

// localIsTracked reports whether specscore.local.yaml is tracked by git in repoRoot.
func localIsTracked(repoRoot string) bool {
	return runGitFn(repoRoot, "ls-files", "--error-unmatch", LocalFile) == nil
}

// EnsureLocalGitignored makes sure SpecScore's user-local configuration and
// persistent lifecycle lock identities are git-ignored in repoRoot, appending
// missing entries to .gitignore. It returns whether any entry was added and a
// non-empty warning when specscore.local.yaml is already git-tracked (which
// defeats its per-user purpose).
func EnsureLocalGitignored(repoRoot string) (added bool, warning string, err error) {
	path := filepath.Join(repoRoot, ".gitignore")
	data, rerr := os.ReadFile(path)
	if rerr != nil && !os.IsNotExist(rerr) {
		return false, "", fmt.Errorf("reading .gitignore: %w", rerr)
	}
	existing := string(data)

	missing := missingGitignoreEntries(existing)
	if len(missing) > 0 {
		var b strings.Builder
		b.WriteString(existing)
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			b.WriteString("\n")
		}
		for _, entry := range missing {
			b.WriteString(entry + "\n")
		}
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

func missingGitignoreEntries(content string) []string {
	required := []string{LocalFile, LifecycleTransactionLockIgnorePattern, LifecycleProjectLockIgnorePattern}
	present := make(map[string]bool, len(required))
	for _, line := range strings.Split(content, "\n") {
		present[strings.TrimSpace(line)] = true
	}
	missing := make([]string, 0, len(required))
	for _, entry := range required {
		if !present[entry] {
			missing = append(missing, entry)
		}
	}
	return missing
}
