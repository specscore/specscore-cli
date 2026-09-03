package rule

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var errInjected = errors.New("injected failure")

func swapStat(t *testing.T, fn func(string) (os.FileInfo, error)) {
	t.Helper()
	prev := osStat
	osStat = fn
	t.Cleanup(func() { osStat = prev })
}

func swapReadFile(t *testing.T, fn func(string) ([]byte, error)) {
	t.Helper()
	prev := osReadFile
	osReadFile = fn
	t.Cleanup(func() { osReadFile = prev })
}

func swapWriteFile(t *testing.T, fn func(string, []byte, os.FileMode) error) {
	t.Helper()
	prev := osWriteFile
	osWriteFile = fn
	t.Cleanup(func() { osWriteFile = prev })
}

func swapMkdirAll(t *testing.T, fn func(string, os.FileMode) error) {
	t.Helper()
	prev := osMkdirAll
	osMkdirAll = fn
	t.Cleanup(func() { osMkdirAll = prev })
}

func swapReadDir(t *testing.T, fn func(string) ([]os.DirEntry, error)) {
	t.Helper()
	prev := osReadDir
	osReadDir = fn
	t.Cleanup(func() { osReadDir = prev })
}

func swapCreateTemp(t *testing.T, fn func(string, string) (tempFile, error)) {
	t.Helper()
	prev := osCreateTemp
	osCreateTemp = fn
	t.Cleanup(func() { osCreateTemp = prev })
}

func swapRename(t *testing.T, fn func(string, string) error) {
	t.Helper()
	prev := osRename
	osRename = fn
	t.Cleanup(func() { osRename = prev })
}

func swapOpenDir(t *testing.T, fn func(string) (syncCloser, error)) {
	t.Helper()
	prev := osOpenDir
	osOpenDir = fn
	t.Cleanup(func() { osOpenDir = prev })
}

// ----- discovery error branches -----

func TestDiscoveryPropagatesReadDirError(t *testing.T) {
	swapReadDir(t, func(string) ([]os.DirEntry, error) { return nil, errInjected })
	if _, err := DiscoverDetails("anywhere"); !errors.Is(err, errInjected) {
		t.Errorf("DiscoverDetails error = %v", err)
	}
	if _, err := DetailsBySlug("anywhere"); !errors.Is(err, errInjected) {
		t.Errorf("DetailsBySlug error = %v", err)
	}
	if _, err := DiscoverSkills("anywhere"); !errors.Is(err, errInjected) {
		t.Errorf("DiscoverSkills error = %v", err)
	}
	if _, err := SkillsByName("anywhere"); !errors.Is(err, errInjected) {
		t.Errorf("SkillsByName error = %v", err)
	}
}

// A README that vanishes between enumeration and parsing must abort discovery
// rather than silently yielding a partial set the index would be rebuilt from.
func TestDiscoverDetailsPropagatesParseError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(RulesDir(root), "x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stat succeeds (the entry existed a moment ago) but the open fails.
	swapStat(t, func(string) (os.FileInfo, error) { return nil, nil })
	if _, err := DiscoverDetails(RulesDir(root)); err == nil {
		t.Fatal("DiscoverDetails should propagate a parse failure")
	}
}

// ----- EnsureIndex error branches -----

func TestEnsureIndexErrors(t *testing.T) {
	t.Run("mkdir fails", func(t *testing.T) {
		swapMkdirAll(t, func(string, os.FileMode) error { return errInjected })
		if err := EnsureIndex("dir"); !errors.Is(err, errInjected) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("stat fails for a reason other than absence", func(t *testing.T) {
		swapStat(t, func(string) (os.FileInfo, error) { return nil, errInjected })
		if err := EnsureIndex(t.TempDir()); !errors.Is(err, errInjected) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("write fails", func(t *testing.T) {
		swapStat(t, func(string) (os.FileInfo, error) { return nil, fs.ErrNotExist })
		swapWriteFile(t, func(string, []byte, os.FileMode) error { return errInjected })
		if err := EnsureIndex(t.TempDir()); !errors.Is(err, errInjected) {
			t.Fatalf("err = %v", err)
		}
	})
}

// ----- WriteFileAtomic error branches -----

// stubTempFile drives each failure point of the atomic publish independently.
type stubTempFile struct {
	name     string
	chmodErr error
	writeErr error
	shortN   bool
	syncErr  error
	closeErr error
}

func (f *stubTempFile) Name() string            { return f.name }
func (f *stubTempFile) Chmod(os.FileMode) error { return f.chmodErr }
func (f *stubTempFile) Sync() error             { return f.syncErr }
func (f *stubTempFile) Close() error            { return f.closeErr }
func (f *stubTempFile) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortN {
		return len(b) - 1, nil
	}
	return len(b), nil
}

type stubDir struct {
	syncErr  error
	closeErr error
}

func (d stubDir) Sync() error  { return d.syncErr }
func (d stubDir) Close() error { return d.closeErr }

func TestWriteFileAtomicErrorBranches(t *testing.T) {
	cases := []struct {
		name    string
		stub    *stubTempFile
		tempErr error
		dir     syncCloser
		dirErr  error
		renErr  error
	}{
		{name: "create temp fails", tempErr: errInjected},
		{name: "chmod fails", stub: &stubTempFile{chmodErr: errInjected}},
		{name: "write fails", stub: &stubTempFile{writeErr: errInjected}},
		{name: "short write", stub: &stubTempFile{shortN: true}},
		{name: "sync fails", stub: &stubTempFile{syncErr: errInjected}},
		{name: "close fails", stub: &stubTempFile{closeErr: errInjected}},
		{name: "rename fails", stub: &stubTempFile{}, renErr: errInjected},
		{name: "dir open fails", stub: &stubTempFile{}, dirErr: errInjected},
		{name: "dir sync fails", stub: &stubTempFile{}, dir: stubDir{syncErr: errInjected}},
		{name: "dir close fails", stub: &stubTempFile{}, dir: stubDir{closeErr: errInjected}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "README.md")
			stub := tc.stub
			swapCreateTemp(t, func(string, string) (tempFile, error) {
				if tc.tempErr != nil {
					return nil, tc.tempErr
				}
				stub.name = filepath.Join(dir, ".tmp")
				return stub, nil
			})
			swapRename(t, func(string, string) error { return tc.renErr })
			swapOpenDir(t, func(string) (syncCloser, error) {
				if tc.dirErr != nil {
					return nil, tc.dirErr
				}
				if tc.dir != nil {
					return tc.dir, nil
				}
				return stubDir{}, nil
			})
			if err := WriteFileAtomic(path, []byte("body")); err == nil {
				t.Fatal("WriteFileAtomic should have failed")
			}
		})
	}
}

// WriteFileAtomic reads the existing mode through the Stat seam; a stat failure
// falls back to 0644 rather than aborting the publish.
func TestWriteFileAtomicFallsBackToDefaultMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	swapStat(t, func(string) (os.FileInfo, error) { return nil, errInjected })
	if err := WriteFileAtomic(path, []byte("body")); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("mode = %o, want 644", info.Mode().Perm())
	}
}

// ----- write-through error branches -----

func TestIndexMutatorsPropagateWriteFailures(t *testing.T) {
	root := setupRulesTree(t)
	rulesDir := RulesDir(root)
	row := NewRow("x", false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)
	if err := UpsertRow(rulesDir, row); err != nil {
		t.Fatal(err)
	}
	lessonPath := writeLesson(t, root, "l", canonicalLesson)
	skillPath := writeSkill(t, root, "s", "---\nname: s\n---\n\n# S\n")

	swapCreateTemp(t, func(string, string) (tempFile, error) { return nil, errInjected })
	for name, call := range map[string]func() error{
		"WriteIndexRows": func() error { return WriteIndexRows(IndexPath(rulesDir), []Row{row}, nil) },
		"UpsertRow":      func() error { return UpsertRow(rulesDir, row) },
		"RemoveRow":      func() error { return RemoveRow(rulesDir, "x") },
		"SetPromotesTo":  func() error { return SetLessonPromotesTo(lessonPath, "y") },
		"SetSkillRules":  func() error { return SetSkillRules(skillPath, []string{"x"}) },
	} {
		if err := call(); !errors.Is(err, errInjected) {
			t.Errorf("%s error = %v, want the injected failure", name, err)
		}
	}
}

func TestIndexMutatorsPropagateReadFailures(t *testing.T) {
	swapReadFile(t, func(string) ([]byte, error) { return nil, errInjected })
	if err := WriteIndexRows("p", nil, nil); !errors.Is(err, errInjected) {
		t.Errorf("WriteIndexRows error = %v", err)
	}
	if _, err := ReadIndex("p"); !errors.Is(err, errInjected) {
		t.Errorf("ReadIndex error = %v", err)
	}
	if err := UpsertRow("dir", Row{Slug: "x"}); !errors.Is(err, errInjected) {
		t.Errorf("UpsertRow error = %v", err)
	}
	if err := RemoveRow("dir", "x"); !errors.Is(err, errInjected) {
		t.Errorf("RemoveRow error = %v", err)
	}
}

// A skill whose SKILL.md cannot be read is skipped, not fatal: one unreadable
// file must not blind the whole pairing check.
func TestDiscoverSkillsSkipsUnreadableSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, DefaultSkillsPath, "broken")
	if err := os.MkdirAll(filepath.Join(dir, "SKILL.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	skills, err := DiscoverSkills(SkillsDir(root, ""))
	if err != nil || len(skills) != 0 {
		t.Fatalf("DiscoverSkills = %v, %v", skills, err)
	}
}

// ----- small pure branches -----

func TestScopeMatchesUnknownKind(t *testing.T) {
	if (Scope{Kind: "team", Value: "x"}).Matches("any") {
		t.Fatal("an unknown scope kind must match nothing")
	}
}

func TestDetailFieldPositionRejectsUnknownField(t *testing.T) {
	if got := detailFieldPosition("Nonsense"); got != -1 {
		t.Fatalf("detailFieldPosition = %d, want -1", got)
	}
	if _, err := insertionPointFor([]string{"# Rule: X"}, "Nonsense"); err == nil {
		t.Fatal("insertionPointFor should reject a non-Rule field")
	}
}

func TestRewriteFrontmatterStatusIgnoresBodyLines(t *testing.T) {
	// A closed frontmatter block must not have a later body line rewritten as
	// if it were frontmatter.
	lines := []string{"---", "format: x", "---", "status: body"}
	rewriteFrontmatterStatus(lines, "Active")
	if lines[3] != "status: body" {
		t.Fatalf("a post-frontmatter line was rewritten: %v", lines)
	}
	// A document with no frontmatter at all is left alone.
	plain := []string{"# Rule: X", "status: body"}
	rewriteFrontmatterStatus(plain, "Active")
	if plain[1] != "status: body" {
		t.Fatalf("a frontmatter-less document was rewritten: %v", plain)
	}
	// An unterminated block with no status key is also a no-op.
	unterminated := []string{"---", "format: x"}
	rewriteFrontmatterStatus(unterminated, "Active")
	if !strings.HasPrefix(unterminated[1], "format:") {
		t.Fatalf("unterminated frontmatter was rewritten: %v", unterminated)
	}
}
