package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// IsSingleFilePlanPath — edge cases
// ---------------------------------------------------------------------------

func TestIsSingleFilePlanPath_NotMdExtension(t *testing.T) {
	if IsSingleFilePlanPath("/plans", "/plans/foo.txt") {
		t.Error("non-.md file should return false")
	}
}

func TestIsSingleFilePlanPath_README(t *testing.T) {
	if IsSingleFilePlanPath("/plans", "/plans/README.md") {
		t.Error("README.md should return false")
	}
}

func TestIsSingleFilePlanPath_NestedFile(t *testing.T) {
	sep := string(os.PathSeparator)
	if IsSingleFilePlanPath("/plans", "/plans/sub"+sep+"nested.md") {
		t.Error("nested file should return false")
	}
}

func TestIsSingleFilePlanPath_NotUnderPlansDir(t *testing.T) {
	if IsSingleFilePlanPath("/plans", "/other/foo.md") {
		t.Error("file not under plans dir should return false")
	}
}

func TestIsSingleFilePlanPath_DirectChild(t *testing.T) {
	if !IsSingleFilePlanPath("/plans", "/plans/my-plan.md") {
		t.Error("direct child .md file should return true")
	}
}

func TestIsSingleFilePlanPath_BareREADME(t *testing.T) {
	// Edge case: filePath is just "README.md" (not under plansDir)
	if IsSingleFilePlanPath("/plans", "README.md") {
		t.Error("bare README.md should return false")
	}
}

// ---------------------------------------------------------------------------
// splitCommaList — edge cases
// ---------------------------------------------------------------------------

func TestSplitCommaList_EmDash(t *testing.T) {
	result := splitCommaList("—")
	if len(result) != 0 {
		t.Errorf("em-dash should return nil, got %v", result)
	}
}

func TestSplitCommaList_ASCIIDash(t *testing.T) {
	result := splitCommaList("-")
	if len(result) != 0 {
		t.Errorf("ASCII dash should return nil, got %v", result)
	}
}

func TestSplitCommaList_Empty(t *testing.T) {
	result := splitCommaList("")
	if len(result) != 0 {
		t.Errorf("empty should return nil, got %v", result)
	}
}

func TestSplitCommaList_TrailingComma(t *testing.T) {
	result := splitCommaList("a, b,")
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("trailing comma: got %v", result)
	}
}

// ---------------------------------------------------------------------------
// filepathRel — edge cases
// ---------------------------------------------------------------------------

func TestFilepathRel_NotUnderPlansDir(t *testing.T) {
	_, err := filepathRel("/plans", "/other/file.md")
	if err == nil {
		t.Error("expected error for file not under plans dir")
	}
}

func TestFilepathRel_PathIsPlansDirItself(t *testing.T) {
	_, err := filepathRel("/plans", "/plans")
	if err == nil {
		t.Error("expected error when path is plans dir itself")
	}
}

func TestFilepathRel_ValidRelative(t *testing.T) {
	sep := string(os.PathSeparator)
	rel, err := filepathRel("/plans", "/plans"+sep+"my-plan.md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rel != "my-plan.md" {
		t.Errorf("rel = %q, want %q", rel, "my-plan.md")
	}
}

// ---------------------------------------------------------------------------
// Parse — file not found
// ---------------------------------------------------------------------------

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "nonexistent.md"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestParse_PropagatesScannerErrorFromRealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.md")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", (1<<20)+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(path); err == nil {
		t.Fatal("expected scanner error for line larger than parser limit")
	}
}

// ---------------------------------------------------------------------------
// Parse — non-Plan file (no "# Plan:" prefix)
// ---------------------------------------------------------------------------

func TestParse_NonPlanFile(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	path := writePlan(t, plansDir, "not-a-plan", "# Something Else\n\nBody.\n")
	p, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.HasPlanTitle {
		t.Error("expected HasPlanTitle=false for non-plan file")
	}
}

// ---------------------------------------------------------------------------
// Parse — Plan with invalid Mode value
// ---------------------------------------------------------------------------

func TestParse_InvalidModeValueCoverage(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := "# Plan: Bad Mode\n\n**Mode:** bogus-xyz\n**Source Feature:** feat\n\n## Tasks\n\n### Task 1: Only task\n\n**Verifies:** feat#ac:one\n**Status:** pending\n"
	path := writePlan(t, plansDir, "bad-mode2", body)
	p, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !p.HasPlanTitle {
		t.Fatal("expected HasPlanTitle=true")
	}
	if p.ModeValueValid {
		t.Error("expected ModeValueValid=false for invalid mode")
	}
	if p.ModeRaw != "bogus-xyz" {
		t.Errorf("ModeRaw = %q", p.ModeRaw)
	}
	// Mode should default to full
	if p.Mode != ModeFull {
		t.Errorf("Mode = %q, want %q", p.Mode, ModeFull)
	}
}

// ---------------------------------------------------------------------------
// Parse — Plan with stub mode
// ---------------------------------------------------------------------------

func TestParse_StubMode(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := "# Plan: Stub Plan\n\n**Mode:** stub\n**Source Feature:** feat\n\n## Tasks\n\n### Task 1: Stub task\n\n**Verifies:** feat#ac:one\n**Status:** pending\n"
	path := writePlan(t, plansDir, "stub-plan", body)
	p, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if p.Mode != ModeStub {
		t.Errorf("Mode = %q, want %q", p.Mode, ModeStub)
	}
	if !p.ModeValueValid {
		t.Error("expected ModeValueValid=true for 'stub'")
	}
}
