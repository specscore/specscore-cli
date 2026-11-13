package projectdef

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readStorageLayoutConfig(t *testing.T, block string) SpecConfig {
	t.Helper()
	dir := t.TempDir()
	body := SchemaHeader + "\n\nproject:\n  title: T\n" + block
	if err := os.WriteFile(filepath.Join(dir, SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadSpecConfig(dir)
	if err != nil {
		t.Fatalf("a storage_layout block must never fail the whole config read: %v", err)
	}
	return cfg
}

func TestStorageLayout_AbsentIsOff(t *testing.T) {
	var cfg SpecConfig
	if cfg.StorageLayoutRequired() {
		t.Fatal("absent storage_layout block must leave the rules off")
	}
	if got := cfg.StorageLayoutGuideURL(); got != "" {
		t.Fatalf("absent block guide URL = %q, want empty", got)
	}
	if _, ok := cfg.StorageLayoutExistingUntil(); ok {
		t.Fatal("absent block has no existing_until")
	}
	if cfg.StorageLayoutShapeError() != "" {
		t.Fatal("absent block has no shape error")
	}
}

func TestStorageLayout_ParsedFromYAML(t *testing.T) {
	cfg := readStorageLayoutConfig(t, "storage_layout:\n  required: true\n  guide_url: \" https://example.com/storage \"\n  existing_until: 2026-11-13\n")
	if !cfg.StorageLayoutRequired() {
		t.Fatalf("storage_layout.required: true must turn the rules on (shape: %q)", cfg.StorageLayoutShapeError())
	}
	if got := cfg.StorageLayoutGuideURL(); got != "https://example.com/storage" {
		t.Fatalf("guide URL = %q", got)
	}
	until, ok := cfg.StorageLayoutExistingUntil()
	if !ok || until.Format("2006-01-02") != "2026-11-13" {
		t.Fatalf("existing_until = %v, %v", until, ok)
	}
	if _, leaked := cfg.Extras["storage_layout"]; leaked {
		t.Fatal("storage_layout must parse into the typed field, not Extras")
	}
}

func TestStorageLayout_RequiredFalseIsOff(t *testing.T) {
	cfg := readStorageLayoutConfig(t, "storage_layout:\n  required: false\n  guide_url: http://example.com\n")
	if cfg.StorageLayoutRequired() {
		t.Fatal("required: false must leave the rules off")
	}
	if cfg.StorageLayoutShapeError() != "" {
		t.Fatalf("well-formed block reported shape error %q", cfg.StorageLayoutShapeError())
	}
}

func TestStorageLayout_ShapeErrorsRecordedNotFatal(t *testing.T) {
	cases := map[string]struct{ block, want string }{
		"scalar block":      {"storage_layout: true\n", "must be a mapping"},
		"sequence block":    {"storage_layout: [required]\n", "must be a mapping"},
		"string required":   {"storage_layout:\n  required: \"true\"\n", "`storage_layout.required` must be true or false"},
		"bad bool":          {"storage_layout:\n  required: !!bool maybe\n", "`storage_layout.required` must be true or false"},
		"list guide":        {"storage_layout:\n  required: true\n  guide_url: [1, 2]\n", "`storage_layout.guide_url` must be an http(s) URL"},
		"non-url guide":     {"storage_layout:\n  required: true\n  guide_url: docs/storage.md\n", "`storage_layout.guide_url` must be an http(s) URL"},
		"bad date":          {"storage_layout:\n  required: true\n  existing_until: 13/11/2026\n", "`storage_layout.existing_until` must be a YYYY-MM-DD date"},
		"list date":         {"storage_layout:\n  required: true\n  existing_until: [2026-11-13]\n", "`storage_layout.existing_until` must be a YYYY-MM-DD date"},
		"misspelled key":    {"storage_layout:\n  requird: true\n", "unknown key `storage_layout.requird`"},
		"two problems join": {"storage_layout:\n  required: yes please\n  extra: 1\n", "; unknown key `storage_layout.extra`"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := readStorageLayoutConfig(t, tc.block)
			if got := cfg.StorageLayoutShapeError(); !strings.Contains(got, tc.want) {
				t.Fatalf("shape error = %q, want it to contain %q", got, tc.want)
			}
			if cfg.StorageLayoutRequired() {
				t.Fatal("a malformed block must not count as a valid opt-in")
			}
		})
	}
}

func TestStorageLayout_UnknownKeyPreservedInExtras(t *testing.T) {
	cfg := readStorageLayoutConfig(t, "storage_layout:\n  required: true\n  owner_note: keep me\n")
	if got := cfg.StorageLayout.Extras["owner_note"]; got != "keep me" {
		t.Fatalf("unknown key must round-trip through Extras, got %v", got)
	}
}
