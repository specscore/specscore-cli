package projectdef

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// configurable-ideas-path#req:ideas-path-default
func TestEffectiveIdeasPath_DefaultWhenUnset(t *testing.T) {
	m := ModuleConfig{}
	if got := m.EffectiveIdeasPath(); got != "spec/ideas" {
		t.Errorf("default ideas path = %q, want spec/ideas", got)
	}
	mWithEmpty := ModuleConfig{PathOverrides: &PathOverrides{}}
	if got := mWithEmpty.EffectiveIdeasPath(); got != "spec/ideas" {
		t.Errorf("empty override ideas path = %q, want spec/ideas", got)
	}
}

// configurable-ideas-path#req:ideas-path-override
func TestEffectiveIdeasPath_Override(t *testing.T) {
	m := ModuleConfig{PathOverrides: &PathOverrides{IdeasPath: "ideas"}}
	if got := m.EffectiveIdeasPath(); got != "ideas" {
		t.Errorf("override ideas path = %q, want ideas", got)
	}
	nested := ModuleConfig{PathOverrides: &PathOverrides{IdeasPath: "docs/ideas/"}}
	if got := nested.EffectiveIdeasPath(); got != "docs/ideas" {
		t.Errorf("nested override = %q, want docs/ideas", got)
	}
}

// configurable-ideas-path#req:seeds-follow-ideas
func TestEffectiveSeedsPath(t *testing.T) {
	def := ModuleConfig{}
	if got := def.EffectiveSeedsPath(); got != "spec/ideas/seeds" {
		t.Errorf("default seeds path = %q, want spec/ideas/seeds", got)
	}
	override := ModuleConfig{PathOverrides: &PathOverrides{IdeasPath: "ideas"}}
	if got := override.EffectiveSeedsPath(); got != "ideas/seeds" {
		t.Errorf("override seeds path = %q, want ideas/seeds", got)
	}
}

// configurable-ideas-path#req:ideas-path-relative-to-module
func TestValidate_IdeasPathRejectsAbsoluteAndEscaping(t *testing.T) {
	cases := []struct{ name, path string }{
		{"absolute", "/ideas"},
		{"escaping", "../ideas"},
		{"escaping-nested", "../../x/ideas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := SpecConfig{Modules: []ModuleConfig{
				{Path: "backend", PathOverrides: &PathOverrides{IdeasPath: tc.path}},
			}}
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected error for ideas_path %q, got nil", tc.path)
			}
			if !strings.Contains(err.Error(), "ideas-path-relative-to-module") {
				t.Errorf("error %q should cite the REQ", err.Error())
			}
			if !strings.Contains(err.Error(), "backend") {
				t.Errorf("error %q should name the offending module", err.Error())
			}
		})
	}
}

// configurable-ideas-path#req:ideas-path-relative-to-module — valid relative paths pass.
func TestValidate_IdeasPathAcceptsRelative(t *testing.T) {
	c := SpecConfig{Modules: []ModuleConfig{
		{PathOverrides: &PathOverrides{IdeasPath: "ideas"}},
		{Path: "backend", PathOverrides: &PathOverrides{IdeasPath: "docs/ideas"}},
	}}
	if err := c.Validate(); err != nil {
		t.Errorf("relative ideas paths should validate, got %v", err)
	}
}

// repo-config#req:path-overrides-optional — unknown keys round-trip.
func TestPathOverrides_UnknownKeysRoundTrip(t *testing.T) {
	in := "ideas_path: ideas\nfuture_path: somewhere\n"
	var po PathOverrides
	if err := yaml.Unmarshal([]byte(in), &po); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if po.IdeasPath != "ideas" {
		t.Errorf("ideas_path = %q, want ideas", po.IdeasPath)
	}
	out, err := yaml.Marshal(po)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), "future_path") {
		t.Errorf("unknown key future_path should round-trip; got:\n%s", out)
	}
}
