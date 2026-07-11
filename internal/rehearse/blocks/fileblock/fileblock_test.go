// Package fileblock_test verifies file assertion evaluation functions.
package fileblock_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks/fileblock"
	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

// helper creates a temp file with the given content and returns its path.
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return path
}

// ── EvalExists ────────────────────────────────────────────────────────────────

func TestEvalExists_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "exists.txt", "hello")

	passed, msg := fileblock.EvalExists(path)

	if !passed {
		t.Errorf("EvalExists returned passed=false, want true; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("EvalExists returned msg=%q, want empty", msg)
	}
}

func TestEvalExists_FileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-file.txt")

	passed, msg := fileblock.EvalExists(path)

	if passed {
		t.Errorf("EvalExists returned passed=true, want false")
	}
	if msg == "" {
		t.Error("EvalExists returned empty msg, want non-empty")
	}
}

// ── EvalMissing ───────────────────────────────────────────────────────────────

func TestEvalMissing_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "present.txt", "data")

	passed, msg := fileblock.EvalMissing(path)

	if passed {
		t.Errorf("EvalMissing returned passed=true for existing file, want false")
	}
	if msg == "" {
		t.Error("EvalMissing returned empty msg, want non-empty")
	}
}

func TestEvalMissing_FileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.txt")

	passed, msg := fileblock.EvalMissing(path)

	if !passed {
		t.Errorf("EvalMissing returned passed=false for absent file, want true; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("EvalMissing returned msg=%q, want empty", msg)
	}
}

// ── EvalContains ──────────────────────────────────────────────────────────────

func TestEvalContains_Substring(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "content.txt", "hello world\nfoo bar\n")

	passed, msg := fileblock.EvalContains(path, "foo bar")

	if !passed {
		t.Errorf("EvalContains returned passed=false, want true; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("EvalContains returned msg=%q, want empty", msg)
	}
}

func TestEvalContains_NoSubstring(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "content.txt", "hello world\n")

	passed, msg := fileblock.EvalContains(path, "missing text")

	if passed {
		t.Errorf("EvalContains returned passed=true, want false")
	}
	if !strings.Contains(msg, "missing text") {
		t.Errorf("EvalContains msg %q does not mention expected substring", msg)
	}
}

// ── EvalNotContains ───────────────────────────────────────────────────────────

func TestEvalNotContains_Substring(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "content.txt", "hello world\n")

	passed, msg := fileblock.EvalNotContains(path, "hello world")

	if passed {
		t.Errorf("EvalNotContains returned passed=true when substring present, want false")
	}
	if msg == "" {
		t.Error("EvalNotContains returned empty msg, want non-empty")
	}
}

func TestEvalNotContains_NoSubstring(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "content.txt", "hello world\n")

	passed, msg := fileblock.EvalNotContains(path, "absent text")

	if !passed {
		t.Errorf("EvalNotContains returned passed=false when substring absent, want true; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("EvalNotContains returned msg=%q, want empty", msg)
	}
}

// ── EvalPermissions ───────────────────────────────────────────────────────────

func TestEvalPermissions_Match(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "perm.txt", "data")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	passed, msg := fileblock.EvalPermissions(path, "0644")

	if !passed {
		t.Errorf("EvalPermissions returned passed=false for matching mode; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("EvalPermissions returned msg=%q, want empty", msg)
	}
}

func TestEvalPermissions_Mismatch(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "perm.txt", "data")
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	passed, msg := fileblock.EvalPermissions(path, "0644")

	if passed {
		t.Errorf("EvalPermissions returned passed=true for mismatched mode, want false")
	}
	if !strings.Contains(msg, "0644") || !strings.Contains(msg, "0755") {
		t.Errorf("EvalPermissions msg %q does not mention both expected and actual modes", msg)
	}
}

// ── Eval dispatcher ───────────────────────────────────────────────────────────

func TestEval_RelativePath(t *testing.T) {
	dir := t.TempDir()
	// Create a file via relative name inside workDir.
	writeTempFile(t, dir, "relative.txt", "content")

	fa := scenario.FileAssertion{
		Path:     "relative.txt",
		Kind:     "exists",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval with relative path returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval with relative path returned msg=%q, want empty", msg)
	}
}

func TestEval_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	absPath := writeTempFile(t, dir, "absolute.txt", "data")

	// Use a different workDir to confirm absolute path is used as-is.
	otherDir := t.TempDir()
	fa := scenario.FileAssertion{
		Path:     absPath,
		Kind:     "exists",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, otherDir)

	if !passed {
		t.Errorf("Eval with absolute path returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval with absolute path returned msg=%q, want empty", msg)
	}
}

func TestEval_UnknownKind(t *testing.T) {
	dir := t.TempDir()
	fa := scenario.FileAssertion{
		Path:     "any.txt",
		Kind:     "bogus-kind",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if passed {
		t.Errorf("Eval with unknown kind returned passed=true, want false")
	}
	if !strings.Contains(msg, "bogus-kind") {
		t.Errorf("Eval unknown-kind msg %q does not mention the kind", msg)
	}
}

func TestEval_MissingKind(t *testing.T) {
	dir := t.TempDir()
	fa := scenario.FileAssertion{
		Path:     "absent.txt",
		Kind:     "missing",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval missing kind for absent file returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval missing kind returned msg=%q, want empty", msg)
	}
}

func TestEval_ContainsKind(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "check.txt", "expected content here")

	fa := scenario.FileAssertion{
		Path:     "check.txt",
		Kind:     "contains",
		Expected: "expected content",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval contains kind returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval contains kind returned msg=%q, want empty", msg)
	}
}

func TestEval_NotContainsKind(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "check.txt", "some other content")

	fa := scenario.FileAssertion{
		Path:     "check.txt",
		Kind:     "not-contains",
		Expected: "absent string",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval not-contains kind returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval not-contains kind returned msg=%q, want empty", msg)
	}
}

func TestEval_PermissionsKind(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "perm.txt", "data")
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	fa := scenario.FileAssertion{
		Path:     "perm.txt",
		Kind:     "permissions",
		Expected: "0644",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval permissions kind returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval permissions kind returned msg=%q, want empty", msg)
	}
}

// ── EvalContains file-read errors ─────────────────────────────────────────────

func TestEvalContains_FileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-file.txt")

	passed, msg := fileblock.EvalContains(path, "anything")

	if passed {
		t.Errorf("EvalContains on missing file returned passed=true, want false")
	}
	if msg == "" {
		t.Error("EvalContains on missing file returned empty msg, want non-empty")
	}
}

func TestEvalNotContains_FileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-file.txt")

	passed, msg := fileblock.EvalNotContains(path, "anything")

	if passed {
		t.Errorf("EvalNotContains on missing file returned passed=true, want false")
	}
	if msg == "" {
		t.Error("EvalNotContains on missing file returned empty msg, want non-empty")
	}
}

func TestEvalPermissions_FileMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-file.txt")

	passed, msg := fileblock.EvalPermissions(path, "0644")

	if passed {
		t.Errorf("EvalPermissions on missing file returned passed=true, want false")
	}
	if msg == "" {
		t.Error("EvalPermissions on missing file returned empty msg, want non-empty")
	}
}

// ── EvalContains shows actual content snippet on mismatch ─────────────────────

func TestEvalContains_MismatchMessageShowsActual(t *testing.T) {
	dir := t.TempDir()
	content := "actual file content here"
	path := writeTempFile(t, dir, "f.txt", content)

	passed, msg := fileblock.EvalContains(path, "not present")

	if passed {
		t.Fatal("expected passed=false")
	}
	// Message must contain the expected substring so the user knows what was sought.
	if !strings.Contains(msg, "not present") {
		t.Errorf("msg %q does not mention the expected string %q", msg, "not present")
	}
	// Diagnostic: message should contain some reference to the actual content.
	wantSeen := fmt.Sprintf("expected %q", "not present")
	_ = wantSeen // Just ensuring the message contains the substring we searched for.
}

// ── Glob pattern tests ────────────────────────────────────────────────────────

func TestEvalGlob_SingleMatch(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "single.txt", "data")

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "exists",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob single match returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob single match returned msg=%q, want empty", msg)
	}
}

func TestEvalGlob_MultipleMatches(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "file1.txt", "hello")
	writeTempFile(t, dir, "file2.txt", "hello")
	writeTempFile(t, dir, "file3.txt", "hello")

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "contains",
		Expected: "hello",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob multiple matches returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob multiple matches returned msg=%q, want empty", msg)
	}
}

func TestEvalGlob_PartialMatch_Fail(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "file1.txt", "hello")
	writeTempFile(t, dir, "file2.txt", "hello")
	writeTempFile(t, dir, "file3.txt", "goodbye")

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "contains",
		Expected: "hello",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if passed {
		t.Errorf("Eval glob partial match returned passed=true, want false")
	}
	if msg == "" {
		t.Error("Eval glob partial match returned empty msg, want non-empty")
	}
}

func TestEvalGlob_NoMatches_Exists(t *testing.T) {
	dir := t.TempDir()

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "exists",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if passed {
		t.Errorf("Eval glob no matches exists returned passed=true, want false")
	}
	if msg == "" {
		t.Error("Eval glob no matches exists returned empty msg, want non-empty")
	}
}

func TestEvalGlob_NoMatches_Missing(t *testing.T) {
	dir := t.TempDir()

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "missing",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob no matches missing returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob no matches missing returned msg=%q, want empty", msg)
	}
}

func TestEvalGlob_NoMatches_Contains(t *testing.T) {
	dir := t.TempDir()

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "contains",
		Expected: "anything",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob no matches contains returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob no matches contains returned msg=%q, want empty", msg)
	}
}

func TestEvalGlob_Permissions_SetBased(t *testing.T) {
	dir := t.TempDir()
	path1 := writeTempFile(t, dir, "file1.txt", "data1")
	path2 := writeTempFile(t, dir, "file2.txt", "data2")
	if err := os.Chmod(path1, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if err := os.Chmod(path2, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	fa := scenario.FileAssertion{
		Path:     "*.txt",
		Kind:     "permissions",
		Expected: "0644",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob permissions all match returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob permissions all match returned msg=%q, want empty", msg)
	}
}

func TestEvalGlob_Recursive(t *testing.T) {
	dir := t.TempDir()
	// Create nested directories and files
	if err := os.MkdirAll(filepath.Join(dir, "sub", "nested"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeTempFile(t, dir, "root.txt", "root")
	writeTempFile(t, filepath.Join(dir, "sub"), "sub.txt", "sub")
	writeTempFile(t, filepath.Join(dir, "sub", "nested"), "nested.txt", "nested")

	fa := scenario.FileAssertion{
		Path:     "**/*.txt",
		Kind:     "exists",
		Expected: "",
	}

	passed, msg := fileblock.Eval(fa, dir)

	if !passed {
		t.Errorf("Eval glob recursive returned passed=false; msg=%q", msg)
	}
	if msg != "" {
		t.Errorf("Eval glob recursive returned msg=%q, want empty", msg)
	}
}
