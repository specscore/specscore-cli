package graph

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_ValidFixture(t *testing.T) {
	g := loadValid(t)
	if len(g.Modules) != 5 {
		t.Fatalf("expected 5 modules, got %d", len(g.Modules))
	}
	res := g.ModuleByID("reservations")
	if res == nil {
		t.Fatal("reservations missing")
	}
	if len(res.DependsOn) != 3 {
		t.Fatalf("reservations dependsOn: %v", res.DependsOn)
	}
	if res.Model == nil || !res.Model.HasConcept("Booking") {
		t.Fatal("reservations model missing Booking")
	}
	sched := g.ModuleByID("scheduling")
	if sched == nil || len(sched.Artifacts) != 0 || sched.Model == nil || !sched.Model.HasConcept("TimeWindow") {
		t.Fatal("scheduling should be a models-only module with TimeWindow")
	}
	if g.ModuleByID("ghost") != nil {
		t.Fatal("ghost module should be nil")
	}
	all := g.AllArtifacts()
	if len(all) == 0 {
		t.Fatal("no artifacts")
	}
	// READMEs in collection dirs are skipped as artifacts.
	for _, a := range all {
		if a.CollectionDir != "" && a.Stem == "README" {
			t.Fatalf("collection README discovered as artifact: %s", a.Path)
		}
	}
}

func TestLoad_UnionMultiRoot(t *testing.T) {
	g, ok, err := Load("testdata/multiroot", "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(g.Roots) != 2 {
		t.Fatalf("expected 2 roots, got %v", g.Roots)
	}
	if g.ModuleByID("core") == nil || g.ModuleByID("ext") == nil {
		t.Fatal("expected modules from both roots")
	}
}

func TestLoad_NoRoot(t *testing.T) {
	root := repoWith(t, map[string]string{})
	_, ok, err := Load(root, "")
	if err != nil || ok {
		t.Fatalf("expected no root: ok=%v err=%v", ok, err)
	}
}

func TestLoad_RootOverride(t *testing.T) {
	root := repoWith(t, map[string]string{
		"custom/graph/modules/m/README.md": fmModule("m", "[]"),
	})
	g, ok, err := Load(root, "custom/graph")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if g.ModuleByID("m") == nil {
		t.Fatal("override root module missing")
	}
}

func TestLoad_EmptyGraphRootNoModulesDir(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/.keep": "",
	})
	g, ok, err := Load(root, "")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if len(g.Modules) != 0 {
		t.Fatal("expected zero modules")
	}
}

func TestLoad_SkipsNonDirModuleEntries(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/README.md":   "# stray file\n",
		"spec/graph/modules/m/README.md": fmModule("m", "[]"),
	})
	g, ok, err := Load(root, "")
	if err != nil || !ok || len(g.Modules) != 1 {
		t.Fatalf("expected 1 module: %v %v", ok, err)
	}
}

func TestLoad_ModuleWithoutReadme(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/entities/x.md": fmArt("entity", "x"),
	})
	g, _, err := Load(root, "")
	if err != nil {
		t.Fatal(err)
	}
	m := g.ModuleByID("m")
	if m == nil || m.Readme != nil || len(m.Artifacts) != 1 {
		t.Fatalf("module without readme: %+v", m)
	}
}

func TestLoad_ReadDirErrorOnModules(t *testing.T) {
	root := repoWith(t, map[string]string{"spec/graph/modules": "a file"})
	_, _, err := Load(root, "")
	if err == nil {
		t.Fatal("expected error when modules is a file")
	}
}

func TestLoad_ParseErrorPropagates(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":     fmModule("m", "[]"),
		"spec/graph/modules/m/entities/x.md": fmArt("entity", "x"),
	})
	orig := readFileFn
	readFileFn = func(path string) ([]byte, error) {
		if strings.HasSuffix(path, "x.md") {
			return nil, errors.New("boom")
		}
		return os.ReadFile(path)
	}
	defer func() { readFileFn = orig }()
	_, _, err := Load(root, "")
	if err == nil {
		t.Fatal("expected artifact parse error to propagate")
	}
}

func TestLoad_ReadmeParseErrorPropagates(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md": fmModule("m", "[]"),
	})
	orig := readFileFn
	readFileFn = func(path string) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { readFileFn = orig }()
	_, _, err := Load(root, "")
	if err == nil {
		t.Fatal("expected readme parse error to propagate")
	}
}

func TestLoad_ModelLoadErrorPropagates(t *testing.T) {
	root := repoWith(t, map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/a.hcl": "entity \"A\" {}\n",
	})
	orig := readDirFn
	readDirFn = func(dir string) ([]os.DirEntry, error) {
		if filepath.Base(dir) == "models" {
			return nil, errors.New("boom")
		}
		return os.ReadDir(dir)
	}
	defer func() { readDirFn = orig }()
	_, _, err := Load(root, "")
	if err == nil {
		t.Fatal("expected model load error to propagate")
	}
}

func TestFindGraphRoot(t *testing.T) {
	root := repoWith(t, map[string]string{"spec/graph/.keep": ""})
	abs, ok := FindGraphRoot(root, "")
	if !ok || !strings.HasSuffix(filepath.ToSlash(abs), "spec/graph") {
		t.Fatalf("abs=%q ok=%v", abs, ok)
	}
	_, ok = FindGraphRoot(root, "nope")
	if ok {
		t.Fatal("expected missing override root")
	}
}

func TestGraphRootDirs_NoConfig(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml at all
	writeFile(t, filepath.Join(dir, "spec/graph/.keep"), "")
	roots := GraphRootDirs(dir, "")
	if len(roots) != 1 {
		t.Fatalf("expected repo-level root only, got %v", roots)
	}
}

func TestGraphRootDirs_DedupesRootModule(t *testing.T) {
	// Implicit root module (path ".") yields spec/graph — same as repo-level.
	root := repoWith(t, map[string]string{
		"specscore.yaml":   "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\nmodules:\n  - name: root\n",
		"spec/graph/.keep": "",
	})
	roots := GraphRootDirs(root, "")
	if len(roots) != 1 {
		t.Fatalf("expected deduped single root, got %v", roots)
	}
}
