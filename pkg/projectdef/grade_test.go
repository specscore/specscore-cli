package projectdef

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func parseCfg(t *testing.T, src string) SpecConfig {
	t.Helper()
	var cfg SpecConfig
	if err := yaml.Unmarshal([]byte(src), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return cfg
}

func TestEffectiveGradeValues_DefaultWhenNoGradeBlock(t *testing.T) {
	cfg := parseCfg(t, "project:\n  title: X\n")
	got := cfg.EffectiveGradeValues()
	if !reflect.DeepEqual(got, []string{"A", "B", "C", "D", "F"}) {
		t.Fatalf("default mismatch: %v", got)
	}
	if cfg.GradeShapeError() != "" {
		t.Fatalf("unexpected shape error: %q", cfg.GradeShapeError())
	}
	if cfg.GradeValuesHasDuplicates() {
		t.Fatal("no grade block should not report duplicates")
	}
}

func TestEffectiveGradeValues_NullGradeBlock(t *testing.T) {
	cfg := parseCfg(t, "grade: ~\n")
	if cfg.Grade != nil {
		t.Fatalf("expected nil Grade for null block, got %+v", cfg.Grade)
	}
	got := cfg.EffectiveGradeValues()
	if !reflect.DeepEqual(got, DefaultGradeValues) {
		t.Fatalf("default mismatch: %v", got)
	}
}

func TestEffectiveGradeValues_ConfiguredSet(t *testing.T) {
	cfg := parseCfg(t, "grade:\n  values: [1, 2, 3, 4, 5]\n")
	got := cfg.EffectiveGradeValues()
	if !reflect.DeepEqual(got, []string{"1", "2", "3", "4", "5"}) {
		t.Fatalf("configured mismatch: %v", got)
	}
	if cfg.GradeShapeError() != "" {
		t.Fatalf("unexpected shape error: %q", cfg.GradeShapeError())
	}
}

func TestEffectiveGradeValues_DeduplicatesConfiguredSet(t *testing.T) {
	cfg := parseCfg(t, "grade:\n  values: [A, B, A, C]\n")
	if !cfg.GradeValuesHasDuplicates() {
		t.Fatal("expected duplicates to be reported")
	}
	got := cfg.EffectiveGradeValues()
	if !reflect.DeepEqual(got, []string{"A", "B", "C"}) {
		t.Fatalf("dedup mismatch: %v", got)
	}
}

func TestEffectiveGradeValues_GradeBlockWithoutValuesKey(t *testing.T) {
	// A grade mapping that has no `values:` key → valuesPresent stays false →
	// default applies, no shape error, no duplicates.
	cfg := parseCfg(t, "grade:\n  other: thing\n")
	if cfg.Grade == nil {
		t.Fatal("expected non-nil Grade for a mapping block")
	}
	if cfg.GradeShapeError() != "" {
		t.Fatalf("unexpected shape error: %q", cfg.GradeShapeError())
	}
	if cfg.GradeValuesHasDuplicates() {
		t.Fatal("values-absent must not report duplicates")
	}
	if !reflect.DeepEqual(cfg.EffectiveGradeValues(), DefaultGradeValues) {
		t.Fatalf("expected default set, got %v", cfg.EffectiveGradeValues())
	}
}

func TestGradeShapeError_Cases(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"non-mapping grade", "grade: hello\n"},
		{"scalar values", "grade:\n  values: A\n"},
		{"empty list", "grade:\n  values: []\n"},
		{"non-scalar entry", "grade:\n  values:\n    - [nested]\n"},
		{"empty entry", "grade:\n  values: [\"\", A]\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := parseCfg(t, tc.src)
			if cfg.GradeShapeError() == "" {
				t.Fatalf("expected a shape error for %q", tc.src)
			}
		})
	}
}

func TestGradeValuesHasDuplicates_NoDuplicates(t *testing.T) {
	cfg := parseCfg(t, "grade:\n  values: [A, B, C]\n")
	if cfg.GradeValuesHasDuplicates() {
		t.Fatal("expected no duplicates")
	}
}
