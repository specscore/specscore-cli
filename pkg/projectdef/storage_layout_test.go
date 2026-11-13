package projectdef

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorageLayout_AbsentIsOff(t *testing.T) {
	var cfg SpecConfig
	if cfg.StorageLayoutRequired() {
		t.Fatal("absent storage_layout block must leave the rules off")
	}
	if got := cfg.StorageLayoutGuideURL(); got != "" {
		t.Fatalf("absent block guide URL = %q, want empty", got)
	}
}

func TestStorageLayout_ParsedFromYAML(t *testing.T) {
	dir := t.TempDir()
	body := SchemaHeader + "\n\nstorage_layout:\n  required: true\n  guide_url: \" https://example.com/storage \"\n"
	if err := os.WriteFile(filepath.Join(dir, SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadSpecConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.StorageLayoutRequired() {
		t.Fatal("storage_layout.required: true must turn the rules on")
	}
	if got := cfg.StorageLayoutGuideURL(); got != "https://example.com/storage" {
		t.Fatalf("guide URL = %q", got)
	}
	if _, leaked := cfg.Extras["storage_layout"]; leaked {
		t.Fatal("storage_layout must parse into the typed field, not Extras")
	}
}

func TestStorageLayout_RequiredFalseIsOff(t *testing.T) {
	cfg := SpecConfig{StorageLayout: &StorageLayoutConfig{Required: false, GuideURL: "u"}}
	if cfg.StorageLayoutRequired() {
		t.Fatal("required: false must leave the rules off")
	}
	if got := cfg.StorageLayoutGuideURL(); got != "u" {
		t.Fatalf("guide URL = %q", got)
	}
}
