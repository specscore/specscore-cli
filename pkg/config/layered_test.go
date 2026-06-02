package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeLayer(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// getPath navigates nested map[string]any by successive keys.
func getPath(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func TestResolveDir_MostSpecificWins(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeLayer(t, filepath.Join(home, ".specscore.yaml"), "studio:\n  theme: dark\n")
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "studio:\n  theme: light\n")
	writeLayer(t, filepath.Join(repo, "specscore.local.yaml"), "studio:\n  theme: solarized\n")

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if v := getPath(got.Values, "studio", "theme"); v != "solarized" {
		t.Errorf("theme = %v, want solarized", v)
	}
	if o := got.Origin["studio.theme"]; o != "local" {
		t.Errorf("origin = %q, want local", o)
	}

	// Removing the local layer yields the project value.
	if err := os.Remove(filepath.Join(repo, "specscore.local.yaml")); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if v := getPath(got.Values, "studio", "theme"); v != "light" {
		t.Errorf("theme = %v, want light", v)
	}
	if o := got.Origin["studio.theme"]; o != "project" {
		t.Errorf("origin = %q, want project", o)
	}

	// Removing the project layer yields the home value.
	if err := os.Remove(filepath.Join(repo, "specscore.yaml")); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if v := getPath(got.Values, "studio", "theme"); v != "dark" {
		t.Errorf("theme = %v, want dark", v)
	}
	if o := got.Origin["studio.theme"]; o != "home" {
		t.Errorf("origin = %q, want home", o)
	}
}

func TestResolveDir_DeepMergeReplacesSequences(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeLayer(t, filepath.Join(home, ".specscore.yaml"),
		"journal:\n  enabled: true\n  aggregation_periods: [day, week]\n")
	writeLayer(t, filepath.Join(repo, "specscore.yaml"),
		"journal:\n  aggregation_periods: [day, week, month]\n")

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if v := getPath(got.Values, "journal", "enabled"); v != true {
		t.Errorf("journal.enabled = %v, want true", v)
	}
	if o := got.Origin["journal.enabled"]; o != "home" {
		t.Errorf("journal.enabled origin = %q, want home", o)
	}
	want := []any{"day", "week", "month"}
	if v := getPath(got.Values, "journal", "aggregation_periods"); !reflect.DeepEqual(v, want) {
		t.Errorf("aggregation_periods = %v, want %v (replaced, not concatenated)", v, want)
	}
	if o := got.Origin["journal.aggregation_periods"]; o != "project" {
		t.Errorf("aggregation_periods origin = %q, want project", o)
	}
}

func TestResolveDir_ExplicitNullClears(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeLayer(t, filepath.Join(home, ".specscore.yaml"), "journal:\n  enabled: true\n")
	writeLayer(t, filepath.Join(repo, "specscore.local.yaml"), "journal:\n  enabled: null\n")

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	j, ok := getPath(got.Values, "journal").(map[string]any)
	if !ok {
		t.Fatalf("journal not a map: %v", getPath(got.Values, "journal"))
	}
	if _, present := j["enabled"]; present {
		t.Errorf("journal.enabled should be cleared by explicit null, got %v", j["enabled"])
	}
	if _, present := got.Origin["journal.enabled"]; present {
		t.Errorf("cleared key should have no origin, got %q", got.Origin["journal.enabled"])
	}
}

func TestResolveDir_SingleFileUnchanged(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir() // empty: no ~/.specscore.yaml
	writeLayer(t, filepath.Join(repo, "specscore.yaml"),
		"specs_dir_name: spec\nstudio:\n  name: Acme\n")

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	want := map[string]any{
		"specs_dir_name": "spec",
		"studio":         map[string]any{"name": "Acme"},
	}
	if !reflect.DeepEqual(got.Values, want) {
		t.Errorf("Values = %#v, want %#v", got.Values, want)
	}
	if got.Origin["specs_dir_name"] != "project" || got.Origin["studio.name"] != "project" {
		t.Errorf("origins = %#v, want all project", got.Origin)
	}
}

func TestResolveDir_NoLayersIsEmptyNoError(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir with no layer files: %v", err)
	}
	if len(got.Values) != 0 {
		t.Errorf("Values = %#v, want empty", got.Values)
	}
	if len(got.Origin) != 0 {
		t.Errorf("Origin = %#v, want empty", got.Origin)
	}
}

func TestResolveDir_ParseErrorSurfaces(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeLayer(t, filepath.Join(repo, "specscore.yaml"), "foo: [unclosed\n")

	_, err := ResolveDir(repo, home)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestResolveDir_ReadErrorSurfaces(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	// A directory where a layer file is expected makes ReadFile fail with a
	// non-"not exist" error, which must surface rather than be skipped.
	if err := os.Mkdir(filepath.Join(repo, "specscore.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := ResolveDir(repo, home)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
}

func TestResolveDir_ScalarReplacesMapClearsNestedOrigin(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	writeLayer(t, filepath.Join(home, ".specscore.yaml"), "journal:\n  enabled: true\n")
	writeLayer(t, filepath.Join(repo, "specscore.local.yaml"), "journal: disabled\n")

	got, err := ResolveDir(repo, home)
	if err != nil {
		t.Fatalf("ResolveDir: %v", err)
	}
	if v := getPath(got.Values, "journal"); v != "disabled" {
		t.Errorf("journal = %v, want scalar \"disabled\"", v)
	}
	if o := got.Origin["journal"]; o != "local" {
		t.Errorf("journal origin = %q, want local", o)
	}
	if _, present := got.Origin["journal.enabled"]; present {
		t.Errorf("nested origin journal.enabled should be cleared when the map is replaced by a scalar")
	}
}
