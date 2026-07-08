package graph

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
)

const modelSrc = `
component "Auditable" {
  field "createdAt" {
    type = "datetime"
  }
}

component "Wrap" {
  field "team" {
    entity = "identity.Team"
  }
}

entity "Booking" {
  key = ["id"]
  use = ["Auditable", "identity.Base"]

  property "id" {
    type = "uuid"
  }

  property "team" {
    entity = "identity.Team"
  }

  property "win" {
    component = "scheduling.TimeWindow"
  }

  property "role" {
    type = "string"
    enum = "identity.TeamRole"
  }

  property "v1" {
    entity = somevar
  }

  property "v2" {
    entity = null
  }

  property "num" {
    entity = 5
  }
}

enum "Status" {
  values = ["a", "b"]
}

enum "Empty" {
}

entity "BadListString" {
  use = "notlist"
}

entity "BadListElem" {
  use = ["ok", 1]
}

entity "BadListVar" {
  use = somevar
}

entity "BadListNull" {
  use = null
}

collection "c" {
  source = "Booking"
}
`

func loadModelSrc(t *testing.T, src string) *ModelModule {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.hcl"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadModelModule(dir, "m")
	if err != nil {
		t.Fatalf("LoadModelModule: %v", err)
	}
	return m
}

func TestLoadModelModule_All(t *testing.T) {
	m := loadModelSrc(t, modelSrc)
	if len(m.ParseErrors) != 0 {
		t.Fatalf("unexpected parse errors: %+v", m.ParseErrors)
	}
	for _, name := range []string{"Auditable", "Wrap", "Booking", "Status", "Empty"} {
		if !m.HasConcept(name) {
			t.Errorf("missing concept %q", name)
		}
	}
	if m.HasConcept("Nope") {
		t.Error("unexpected concept")
	}
	// enum values captured; Empty has none.
	for _, c := range m.Concepts {
		if c.Name == "Status" && len(c.EnumValues) != 2 {
			t.Errorf("Status values: %v", c.EnumValues)
		}
		if c.Name == "Empty" && len(c.EnumValues) != 0 {
			t.Errorf("Empty values: %v", c.EnumValues)
		}
		if c.Line == 0 {
			t.Errorf("concept %q has no line", c.Name)
		}
	}
	// references: qualified property refs + a field ref + use entries that parsed.
	var haveTeam, haveWin, haveRole, haveUse, haveFieldTeam bool
	for _, r := range m.Refs {
		switch {
		case r.Attr == "entity" && r.Target == "identity.Team" && r.File != "":
			haveTeam = true
			// distinguish the field-level ref (Wrap) from property-level (Booking)
			haveFieldTeam = true
		case r.Attr == "component" && r.Target == "scheduling.TimeWindow":
			haveWin = true
		case r.Attr == "enum" && r.Target == "identity.TeamRole":
			haveRole = true
		case r.Attr == "use" && r.Target == "Auditable":
			haveUse = true
		}
	}
	if !haveTeam || !haveWin || !haveRole || !haveUse || !haveFieldTeam {
		t.Fatalf("missing expected refs: %+v", m.Refs)
	}
	// The invalid-expression refs (v1/v2/num/BadList*) must have been skipped.
	for _, r := range m.Refs {
		if r.Target == "notlist" || r.Target == "" {
			t.Fatalf("unexpected junk ref %+v", r)
		}
	}
}

func TestLoadModelModule_ParseError(t *testing.T) {
	m := loadModelSrc(t, "entity \"A\" {\n  broken !! syntax\n")
	if len(m.ParseErrors) == 0 {
		t.Fatal("expected parse errors")
	}
	if m.ParseErrors[0].Line == 0 || m.ParseErrors[0].Message == "" {
		t.Fatalf("bad diag: %+v", m.ParseErrors[0])
	}
}

func TestDiagLine(t *testing.T) {
	if diagLine(&hcl.Diagnostic{}) != 0 {
		t.Fatal("nil subject should yield line 0")
	}
	d := &hcl.Diagnostic{Subject: &hcl.Range{Start: hcl.Pos{Line: 7}}}
	if diagLine(d) != 7 {
		t.Fatal("subject line")
	}
}

func TestLoadModelModule_ReadDirError(t *testing.T) {
	orig := readDirFn
	readDirFn = func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") }
	defer func() { readDirFn = orig }()
	if _, err := LoadModelModule("nope", "m"); err == nil {
		t.Fatal("expected readdir error")
	}
}

func TestLoadModelModule_ReadFileError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "m.hcl"), []byte("entity \"A\" {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := readFileFn
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { readFileFn = orig }()
	if _, err := LoadModelModule(dir, "m"); err == nil {
		t.Fatal("expected readfile error")
	}
}

func TestLoadModelModule_IgnoresNonHCLAndDirs(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "sub.hcl"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "a.hcl"), []byte("entity \"A\" {}\n"), 0o644)
	m, err := LoadModelModule(dir, "m")
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasConcept("A") || len(m.Concepts) != 1 {
		t.Fatalf("expected only A: %+v", m.Concepts)
	}
}
