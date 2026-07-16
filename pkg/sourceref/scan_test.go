package sourceref

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input []string
		want  string
		name  string
	}{
		{[]string{}, "", "empty slice"},
		{[]string{"a"}, "a", "single element"},
		{[]string{"a", "b", "c"}, "a\nb\nc", "multiple elements"},
		{[]string{"", "b"}, "\nb", "empty first element"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinStrings(tt.input)
			if got != tt.want {
				t.Errorf("joinStrings(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchGlobPattern(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		want    bool
		name    string
	}{
		{"foo/bar.go", "**/*", true, "double-star-slash matches anything"},
		{"deep/nested/file.go", "**", true, "double-star matches anything"},
		{"main.go", "*.go", true, "simple glob match"},
		{"main.txt", "*.go", false, "simple glob no match"},
		{"src/main.go", "*.go", false, "glob does not cross directories"},
		{"foo.go", "foo.go", true, "exact match"},
		// Recursive "**" must match direct children as well as nested files
		// (cli/code/deps#req:path-glob, AC path-filter-works).
		{"pkg/main.go", "pkg/**/*.go", true, "double-star matches direct child"},
		{"pkg/sub/main.go", "pkg/**/*.go", true, "double-star matches nested"},
		{"pkg/sub/deep/x.go", "pkg/**/*.go", true, "double-star matches deeply nested"},
		{"other/main.go", "pkg/**/*.go", false, "double-star respects leading prefix"},
		{"pkg/main.txt", "pkg/**/*.go", false, "double-star respects trailing pattern"},
		{"pkg/handler.go", "pkg/**", true, "trailing double-star matches child"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchGlobPattern(tt.path, tt.pattern)
			if err != nil {
				t.Fatalf("matchGlobPattern(%q, %q) error = %v", tt.path, tt.pattern, err)
			}
			if got != tt.want {
				t.Errorf("matchGlobPattern(%q, %q) = %v, want %v", tt.path, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExpandGlobPattern(t *testing.T) {
	// Create a temp directory with some files
	tmpDir := t.TempDir()

	// Create structure:
	// tmpDir/main.go
	// tmpDir/util.go
	// tmpDir/sub/handler.go
	// tmpDir/sub/deep/nested.txt
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(tmpDir, "sub", "deep"), 0o755))
	must(os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main"), 0o644))
	must(os.WriteFile(filepath.Join(tmpDir, "util.go"), []byte("package main"), 0o644))
	must(os.WriteFile(filepath.Join(tmpDir, "sub", "handler.go"), []byte("package sub"), 0o644))
	must(os.WriteFile(filepath.Join(tmpDir, "sub", "deep", "nested.txt"), []byte("hello"), 0o644))

	// ExpandGlobPattern uses "." as the root, so we need to chdir
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	must(os.Chdir(tmpDir))
	defer func() { _ = os.Chdir(origDir) }()

	t.Run("recursive **/*", func(t *testing.T) {
		matches, err := ExpandGlobPattern("**/*")
		if err != nil {
			t.Fatalf("ExpandGlobPattern(**/*) error = %v", err)
		}
		if len(matches) != 4 {
			t.Errorf("expected 4 matches, got %d: %v", len(matches), matches)
		}
	})

	t.Run("empty defaults to **/*", func(t *testing.T) {
		matches, err := ExpandGlobPattern("")
		if err != nil {
			t.Fatalf("ExpandGlobPattern('') error = %v", err)
		}
		if len(matches) != 4 {
			t.Errorf("expected 4 matches, got %d: %v", len(matches), matches)
		}
	})

	t.Run("simple *.go glob", func(t *testing.T) {
		matches, err := ExpandGlobPattern("*.go")
		if err != nil {
			t.Fatalf("ExpandGlobPattern(*.go) error = %v", err)
		}
		if len(matches) != 2 {
			t.Errorf("expected 2 matches (main.go, util.go), got %d: %v", len(matches), matches)
		}
		for _, m := range matches {
			if !strings.HasSuffix(m, ".go") {
				t.Errorf("unexpected match: %s", m)
			}
		}
	})

	t.Run("results are sorted", func(t *testing.T) {
		matches, err := ExpandGlobPattern("**/*")
		if err != nil {
			t.Fatal(err)
		}
		for i := 1; i < len(matches); i++ {
			if matches[i] < matches[i-1] {
				t.Errorf("results not sorted: %v", matches)
				break
			}
		}
	})

	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := ExpandGlobPattern("[invalid")
		if err == nil {
			t.Error("expected error for invalid pattern, got nil")
		}
	})
}

func TestScanFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create files with annotations
	file1Content := `package main

// specscore:feature/auth
func Login() {}

// specscore:plan/rollout
func Deploy() {}
`
	file2Content := `package handlers

// specscore:feature/auth
// specscore:feature/dashboard
func Handle() {}
`
	file3Content := `package plain
// no annotations here
func Foo() {}
`
	file1 := filepath.Join(tmpDir, "auth.go")
	file2 := filepath.Join(tmpDir, "handlers.go")
	file3 := filepath.Join(tmpDir, "plain.go")

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(file1, []byte(file1Content), 0o644))
	must(os.WriteFile(file2, []byte(file2Content), 0o644))
	must(os.WriteFile(file3, []byte(file3Content), 0o644))

	t.Run("scans multiple files", func(t *testing.T) {
		result, err := ScanFiles([]string{file1, file2, file3})
		if err != nil {
			t.Fatalf("ScanFiles error = %v", err)
		}
		if len(result.FileRefs) != 2 {
			t.Errorf("expected 2 files with refs, got %d", len(result.FileRefs))
		}
		if refs, ok := result.FileRefs[file1]; !ok {
			t.Error("expected file1 in results")
		} else if len(refs) != 2 {
			t.Errorf("expected 2 refs in file1, got %d", len(refs))
		}
		if refs, ok := result.FileRefs[file2]; !ok {
			t.Error("expected file2 in results")
		} else if len(refs) != 2 {
			t.Errorf("expected 2 refs in file2, got %d", len(refs))
		}
		if _, ok := result.FileRefs[file3]; ok {
			t.Error("expected file3 NOT in results (no annotations)")
		}
	})

	t.Run("nonexistent file returns partial result with error", func(t *testing.T) {
		result, err := ScanFiles([]string{file1, "/nonexistent/path.go"})
		if err == nil {
			t.Error("expected error for nonexistent file")
		}
		// Should still have results from file1
		if _, ok := result.FileRefs[file1]; !ok {
			t.Error("expected partial results from file1")
		}
	})

	t.Run("empty file list", func(t *testing.T) {
		result, err := ScanFiles([]string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.FileRefs) != 0 {
			t.Errorf("expected 0 file refs, got %d", len(result.FileRefs))
		}
	})

	t.Run("deduplicates within a file", func(t *testing.T) {
		dupeContent := `package x
// specscore:feature/auth
// specscore:feature/auth
func A() {}
`
		dupeFile := filepath.Join(tmpDir, "dupe.go")
		must(os.WriteFile(dupeFile, []byte(dupeContent), 0o644))

		result, err := ScanFiles([]string{dupeFile})
		if err != nil {
			t.Fatal(err)
		}
		refs := result.FileRefs[dupeFile]
		if len(refs) != 1 {
			t.Errorf("expected 1 unique ref (deduped), got %d", len(refs))
		}
	})
}

func TestScanFilesWithExpandedURL(t *testing.T) {
	tmpDir := t.TempDir()

	content := `package main

// https://specscore.org/github.com/acme/project/spec/features/cli/task
func TaskClaim() {}
`
	file := filepath.Join(tmpDir, "task.go")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ScanFiles([]string{file})
	if err != nil {
		t.Fatalf("ScanFiles error = %v", err)
	}
	refs, ok := result.FileRefs[file]
	if !ok {
		t.Fatal("expected file in results")
	}
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].Type != "feature" {
		t.Errorf("expected type 'feature', got %q", refs[0].Type)
	}
	if refs[0].CrossRepoSuffix != "@github.com/acme/project" {
		t.Errorf("expected cross-repo suffix '@github.com/acme/project', got %q", refs[0].CrossRepoSuffix)
	}
}

func TestGetUniqueReferences(t *testing.T) {
	result := &ScanResult{
		FileRefs: map[string][]*Reference{
			"file1.go": {
				{ResolvedPath: "spec/features/auth", Type: "feature"},
				{ResolvedPath: "spec/plans/rollout", Type: "plan"},
			},
			"file2.go": {
				{ResolvedPath: "spec/features/auth", Type: "feature"},
				{ResolvedPath: "spec/features/dashboard", Type: "feature"},
			},
		},
	}

	t.Run("no filter returns all unique", func(t *testing.T) {
		refs := GetUniqueReferences(result, "")
		if len(refs) != 3 {
			t.Errorf("expected 3 unique refs, got %d", len(refs))
		}
	})

	t.Run("filter by feature", func(t *testing.T) {
		refs := GetUniqueReferences(result, "feature")
		if len(refs) != 2 {
			t.Errorf("expected 2 feature refs, got %d", len(refs))
		}
		for _, ref := range refs {
			if ref.Type != "feature" {
				t.Errorf("expected type 'feature', got %q", ref.Type)
			}
		}
	})

	t.Run("filter by plan", func(t *testing.T) {
		refs := GetUniqueReferences(result, "plan")
		if len(refs) != 1 {
			t.Errorf("expected 1 plan ref, got %d", len(refs))
		}
	})

	t.Run("filter with no matches", func(t *testing.T) {
		refs := GetUniqueReferences(result, "doc")
		if len(refs) != 0 {
			t.Errorf("expected 0 doc refs, got %d", len(refs))
		}
	})

	t.Run("results are sorted", func(t *testing.T) {
		refs := GetUniqueReferences(result, "")
		for i := 1; i < len(refs); i++ {
			keyI := refs[i-1].ResolvedPath + refs[i-1].CrossRepoSuffix
			keyJ := refs[i].ResolvedPath + refs[i].CrossRepoSuffix
			if keyJ < keyI {
				t.Errorf("results not sorted: %q after %q", keyJ, keyI)
			}
		}
	})

	t.Run("empty result", func(t *testing.T) {
		empty := &ScanResult{FileRefs: map[string][]*Reference{}}
		refs := GetUniqueReferences(empty, "")
		if len(refs) != 0 {
			t.Errorf("expected 0 refs from empty result, got %d", len(refs))
		}
	})
}

func TestFormatOutput(t *testing.T) {
	result := &ScanResult{
		FileRefs: map[string][]*Reference{
			"src/auth.go": {
				{ResolvedPath: "spec/features/auth", Type: "feature"},
				{ResolvedPath: "spec/plans/rollout", Type: "plan"},
			},
			"src/handler.go": {
				{ResolvedPath: "spec/features/dashboard", Type: "feature"},
			},
		},
	}

	t.Run("single file mode", func(t *testing.T) {
		output := FormatOutput(result, true, "")
		if output == "" {
			t.Fatal("expected non-empty output")
		}
		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		if len(lines) != 3 {
			t.Errorf("expected 3 lines, got %d: %v", len(lines), lines)
		}
		// Should be sorted
		for i := 1; i < len(lines); i++ {
			if lines[i] < lines[i-1] {
				t.Errorf("output not sorted: %v", lines)
				break
			}
		}
	})

	t.Run("single file mode with type filter", func(t *testing.T) {
		output := FormatOutput(result, true, "feature")
		lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
		if len(lines) != 2 {
			t.Errorf("expected 2 feature lines, got %d: %v", len(lines), lines)
		}
	})

	t.Run("multi-file mode", func(t *testing.T) {
		output := FormatOutput(result, false, "")
		if output == "" {
			t.Fatal("expected non-empty output")
		}
		// Should contain file headers
		if !strings.Contains(output, "src/auth.go") {
			t.Error("expected file header 'src/auth.go'")
		}
		if !strings.Contains(output, "src/handler.go") {
			t.Error("expected file header 'src/handler.go'")
		}
		// Refs should be indented
		if !strings.Contains(output, "  spec/features/auth") {
			t.Error("expected indented ref 'spec/features/auth'")
		}
	})

	t.Run("multi-file mode with type filter", func(t *testing.T) {
		output := FormatOutput(result, false, "plan")
		if !strings.Contains(output, "spec/plans/rollout") {
			t.Error("expected plan ref in output")
		}
		if strings.Contains(output, "spec/features/") {
			t.Error("feature refs should be filtered out")
		}
	})

	t.Run("empty result", func(t *testing.T) {
		empty := &ScanResult{FileRefs: map[string][]*Reference{}}
		output := FormatOutput(empty, true, "")
		if output != "" {
			t.Errorf("expected empty output for empty result, got %q", output)
		}
	})

	t.Run("cross-repo suffix in output", func(t *testing.T) {
		crossResult := &ScanResult{
			FileRefs: map[string][]*Reference{
				"main.go": {
					{ResolvedPath: "spec/features/auth", CrossRepoSuffix: "@github.com/acme/proj", Type: "feature"},
				},
			},
		}
		output := FormatOutput(crossResult, true, "")
		if !strings.Contains(output, "spec/features/auth@github.com/acme/proj") {
			t.Errorf("expected cross-repo suffix in output, got %q", output)
		}
	})
}

func TestParseExpandedURL(t *testing.T) {
	tests := []struct {
		url       string
		urlPrefix string
		wantPath  string
		wantType  string
		wantCross string
		wantErr   bool
		name      string
	}{
		{
			"https://specscore.org/github.com/acme/project/spec/features/cli/task",
			"https://specscore.org/",
			"spec/features/cli/task",
			"feature",
			"@github.com/acme/project",
			false,
			"Feature URL",
		},
		{
			"https://specscore.org/github.com/acme/project/spec/plans/rollout",
			"https://specscore.org/",
			"spec/plans/rollout",
			"plan",
			"@github.com/acme/project",
			false,
			"Plan URL",
		},
		{
			"https://specscore.org/github.com/acme/project/docs/api",
			"https://specscore.org/",
			"docs/api",
			"doc",
			"@github.com/acme/project",
			false,
			"Doc URL",
		},
		{
			"https://specscore.org/github.com/org/repo/spec/features/x",
			"https://specscore.org/",
			"spec/features/x",
			"feature",
			"@github.com/org/repo",
			false,
			"Specscore domain URL",
		},
		{
			"https://specscore.org/github.com/acme/project/spec/features/deep/nested/path",
			"https://specscore.org/",
			"spec/features/deep/nested/path",
			"feature",
			"@github.com/acme/project",
			false,
			"Deep nested path",
		},
		{
			"https://specscore.org/too/few",
			"https://specscore.org/",
			"",
			"",
			"",
			true,
			"Too few path segments",
		},
		{
			"https://specscore.org/a/b/c",
			"https://specscore.org/",
			"",
			"",
			"",
			true,
			"Exactly 3 segments (still too few)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExpandedURL(tt.url, tt.urlPrefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseExpandedURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if got.ResolvedPath != tt.wantPath {
				t.Errorf("parseExpandedURL(%q).ResolvedPath = %q, want %q", tt.url, got.ResolvedPath, tt.wantPath)
			}
			if got.Type != tt.wantType {
				t.Errorf("parseExpandedURL(%q).Type = %q, want %q", tt.url, got.Type, tt.wantType)
			}
			if got.CrossRepoSuffix != tt.wantCross {
				t.Errorf("parseExpandedURL(%q).CrossRepoSuffix = %q, want %q", tt.url, got.CrossRepoSuffix, tt.wantCross)
			}
		})
	}
}
