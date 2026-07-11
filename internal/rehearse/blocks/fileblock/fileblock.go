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

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

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
// evaluation function based on fa.Kind.
func Eval(fa scenario.FileAssertion, workDir string) (bool, string) {
	path := filepath.Join(workDir, fa.Path)
	if filepath.IsAbs(fa.Path) {
		path = fa.Path
	}

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
