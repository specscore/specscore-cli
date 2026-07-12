package runner

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// swap replaces a package-level seam for the duration of one test.
func swap[T any](t *testing.T, target *T, replacement T) {
	t.Helper()
	old := *target
	*target = replacement
	t.Cleanup(func() { *target = old })
}

// writeFile creates path (and parents) with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error carries no exit code: %v", err)
	}
	return ec.ExitCode()
}

func TestDiscover_ExplicitFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.md")
	writeFile(t, file, "x")

	got, err := Discover([]string{file}, dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !reflect.DeepEqual(got, []string{file}) {
		t.Errorf("Discover = %v", got)
	}
}

func TestDiscover_DirRecursiveMDMinusREADME(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "x")
	writeFile(t, filepath.Join(dir, "nested", "b.md"), "x")
	writeFile(t, filepath.Join(dir, "README.md"), "x")
	writeFile(t, filepath.Join(dir, "nested", "README.md"), "x")
	writeFile(t, filepath.Join(dir, "notes.txt"), "x")
	// `.check.md` files are reusable checks, not scenarios — never discovered.
	writeFile(t, filepath.Join(dir, "helper.check.md"), "x")

	got, err := Discover([]string{dir}, dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "nested", "b.md")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

func TestDiscover_GlobFilesAndDirs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "x")
	writeFile(t, filepath.Join(dir, "sub", "b.md"), "x")
	writeFile(t, filepath.Join(dir, "sub", "README.md"), "x")

	got, err := Discover([]string{filepath.Join(dir, "*")}, dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{filepath.Join(dir, "a.md"), filepath.Join(dir, "sub", "b.md")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discover = %v, want %v", got, want)
	}
}

func TestDiscover_DeduplicatesRepeatedPaths(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.md")
	writeFile(t, file, "x")

	got, err := Discover([]string{file, file, dir}, dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !reflect.DeepEqual(got, []string{file}) {
		t.Errorf("Discover = %v, want just %s once", got, file)
	}
}

func TestDiscover_MissingPathExits2(t *testing.T) {
	dir := t.TempDir()
	_, err := Discover([]string{filepath.Join(dir, "absent.md")}, dir)
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "path not found") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_BadGlobPatternExits2(t *testing.T) {
	_, err := Discover([]string{"["}, t.TempDir())
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "bad glob pattern") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_ZeroDiscoveredExits2(t *testing.T) {
	for name, paths := range map[string][]string{
		"empty dir":          {t.TempDir()},
		"glob with no match": {filepath.Join(t.TempDir(), "*.md")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Discover(paths, t.TempDir())
			if code := exitCodeOf(t, err); code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if !strings.Contains(err.Error(), "no scenario files discovered") {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestDiscover_DefaultInsideRepoFindsTestsScenarios(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "specscore.yaml"), "project:\n  title: Fixture\n")
	inTests := filepath.Join(root, "spec", "features", "x", "_tests", "one.md")
	writeFile(t, inTests, "x")
	writeFile(t, filepath.Join(root, "spec", "features", "x", "_tests", "README.md"), "x")
	writeFile(t, filepath.Join(root, "spec", "features", "x", "README.md"), "x")
	writeFile(t, filepath.Join(root, "spec", "features", "x", "notes.md"), "x")

	got, err := Discover(nil, filepath.Join(root, "spec", "features"))
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !reflect.DeepEqual(got, []string{inTests}) {
		t.Errorf("Discover = %v, want %v", got, []string{inTests})
	}
}

func TestDiscover_DefaultOutsideRepoExits2(t *testing.T) {
	_, err := Discover(nil, t.TempDir())
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "no SpecScore repo found") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_DefaultRepoWithoutFeaturesTreeExits2(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "specscore.yaml"), "project:\n  title: Fixture\n")

	_, err := Discover(nil, root)
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "no scenario files discovered") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_WalkErrorExits2(t *testing.T) {
	swap(t, &walkDirFn, func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("disk on fire"))
	})
	dir := t.TempDir()
	_, err := Discover([]string{dir}, dir)
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_GlobDirWalkErrorExits2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "sub", "b.md"), "x")
	swap(t, &walkDirFn, func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, errors.New("walk broke"))
	})
	_, err := Discover([]string{filepath.Join(dir, "s*")}, dir)
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "walk broke") {
		t.Errorf("error = %v", err)
	}
}

func TestDiscover_GlobMatchStatErrorExits2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "x")
	swap(t, &osStatFn, func(string) (os.FileInfo, error) {
		return nil, errors.New("stat broke")
	})
	_, err := Discover([]string{filepath.Join(dir, "*.md")}, dir)
	if code := exitCodeOf(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if !strings.Contains(err.Error(), "resolving glob match") {
		t.Errorf("error = %v", err)
	}
}
