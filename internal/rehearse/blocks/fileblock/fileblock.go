// Package fileblock implements evaluation of file assertions parsed from
// rehearse scenario `### Assert: file` headings. Each evaluation function is a
// pure function (no global state, no side effects beyond reading the
// filesystem) that returns (passed bool, message string).
package fileblock

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

// resolveGlob expands a glob pattern to matching paths. Patterns needing
// doublestar semantics — recursive `**` (Feature:
// cli/rehearse/file-assertions-glob-recursive) or `{a,b}` brace expansion
// (Feature: cli/rehearse/file-assertions-glob-braces) — are resolved via
// doublestar; all other globs use the stdlib filepath.Glob, so plain
// single-level glob behavior is byte-for-byte unchanged from v0.7.1.
func resolveGlob(pattern string) ([]string, error) {
	if strings.Contains(pattern, "**") || strings.Contains(pattern, "{") {
		return doublestar.FilepathGlob(pattern)
	}
	return filepath.Glob(pattern)
}

// EvalExists returns (true, "") when path exists on the filesystem,
// or (false, message) when it does not.
func EvalExists(path string) (bool, string) {
	if _, err := os.Stat(path); err == nil {
		return true, ""
	}
	return false, fmt.Sprintf("file does not exist: %s", path)
}

// EvalMissing is the inverse of EvalExists: returns (true, "") when path is
// absent, or (false, message) when the file exists.
func EvalMissing(path string) (bool, string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true, ""
	}
	return false, fmt.Sprintf("file exists but was expected to be missing: %s", path)
}

// EvalContains returns (true, "") when the file at path contains the expected
// substring, or (false, message) when it does not. Returns (false, message)
// when the file cannot be read.
func EvalContains(path, expected string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("cannot read file %s: %v", path, err)
	}
	content := string(data)
	if strings.Contains(content, expected) {
		return true, ""
	}
	return false, fmt.Sprintf("file %s does not contain expected %q", path, expected)
}

// EvalNotContains is the inverse of EvalContains: returns (true, "") when the
// file does not contain the substring, or (false, message) when it does.
// Returns (false, message) when the file cannot be read.
func EvalNotContains(path, expected string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Sprintf("cannot read file %s: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, expected) {
		return true, ""
	}
	return false, fmt.Sprintf("file %s contains %q but was expected not to", path, expected)
}

// EvalPermissions returns (true, "") when the file's permission bits match the
// expected octal string (e.g. "0644"), or (false, message) when they differ.
// Returns (false, message) when the file cannot be stat'd.
func EvalPermissions(path, expected string) (bool, string) {
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Sprintf("cannot stat file %s: %v", path, err)
	}
	actual := fmt.Sprintf("%04o", info.Mode().Perm())
	if actual == expected {
		return true, ""
	}
	return false, fmt.Sprintf("file %s has permissions %s, expected %s", path, actual, expected)
}

// Eval is the dispatcher: it resolves the path (relative paths are joined to
// workDir; absolute paths are used as-is) and delegates to the appropriate
// evaluation function based on fa.Kind. If the path contains glob characters
// (*?[), it expands the glob and applies set-based logic to all matched files.
func Eval(fa scenario.FileAssertion, workDir string) (bool, string) {
	path := filepath.Join(workDir, fa.Path)
	if filepath.IsAbs(fa.Path) {
		path = fa.Path
	}

	// Check if path contains glob characters (`{` enables brace expansion via
	// doublestar — Feature: cli/rehearse/file-assertions-glob-braces).
	if strings.ContainsAny(fa.Path, "*?[{") {
		// Resolve the glob (recursive `**` via doublestar, else filepath.Glob).
		matches, err := resolveGlob(path)
		if err != nil {
			return false, fmt.Sprintf("invalid glob pattern %q: %v", fa.Path, err)
		}

		// Apply set-based logic based on kind.
		switch fa.Kind {
		case "exists":
			// exists: return true only if at least one file matches
			if len(matches) > 0 {
				return true, ""
			}
			return false, fmt.Sprintf("glob pattern %q matches no files", fa.Path)

		case "missing":
			// missing: return true only if no files match (vacuously true)
			if len(matches) == 0 {
				return true, ""
			}
			return false, fmt.Sprintf("glob pattern %q matches %d files but was expected to match none", fa.Path, len(matches))

		case "contains", "not-contains", "permissions":
			// For these kinds: if zero matches, pass (vacuously true).
			// If one or more matches, call the kind function on EACH file;
			// ALL files must pass.
			if len(matches) == 0 {
				return true, ""
			}

			for _, matchedPath := range matches {
				var passed bool
				var msg string

				switch fa.Kind {
				case "contains":
					passed, msg = EvalContains(matchedPath, fa.Expected)
				case "not-contains":
					passed, msg = EvalNotContains(matchedPath, fa.Expected)
				case "permissions":
					passed, msg = EvalPermissions(matchedPath, fa.Expected)
				}

				// If any file fails, the entire assertion fails.
				if !passed {
					return false, msg
				}
			}
			return true, ""

		default:
			return false, fmt.Sprintf("unknown assertion kind: %s", fa.Kind)
		}
	}

	// Non-glob path: proceed as normal.
	switch fa.Kind {
	case "exists":
		return EvalExists(path)
	case "missing":
		return EvalMissing(path)
	case "contains":
		return EvalContains(path, fa.Expected)
	case "not-contains":
		return EvalNotContains(path, fa.Expected)
	case "permissions":
		return EvalPermissions(path, fa.Expected)
	default:
		return false, fmt.Sprintf("unknown assertion kind: %s", fa.Kind)
	}
}
