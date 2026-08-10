package lesson

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryLegacy_DuplicateIDsAreDistinctAndLossless(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "LESSONS-LEARNED.md")
	raw := "# log\n\n## L7 — first issue\n\n**Status:** Recorded\n\nbody\n\n## L7 — second issue\n\n**Status:** prose variant\n\nmore\n"
	if err := os.WriteFile(source, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := InventoryLegacy(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Entries) != 2 || inv.Entries[0].Key != "L7#1" || inv.Entries[1].Key != "L7#2" {
		t.Fatalf("entries = %#v", inv.Entries)
	}
	if inv.Entries[0].Raw == "" || inv.Entries[0].BytesSHA256 == "" {
		t.Fatalf("lossless evidence missing: %#v", inv.Entries[0])
	}
	if len(inv.Entries[1].Warnings) == 0 {
		t.Fatal("duplicate legacy ID must be visible, not conflated")
	}
}

func TestApplyLegacy_ReviewedMappingIdempotent(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — use a check\n\n**Status:** Recorded\n\ntext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := InventoryLegacy(source)
	if err != nil {
		t.Fatal(err)
	}
	mapping := LegacyMapping{SourceSHA256: inv.SourceSHA256, Entries: []LegacyMappingEntry{{Key: "L1#1", Action: "new", Slug: "use-a-check"}}}
	first, err := ApplyLegacy(lessonsDir, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.CreatedLessons) != 1 {
		t.Fatalf("first = %#v", first)
	}
	second, err := ApplyLegacy(lessonsDir, inv, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.CreatedLessons) != 0 || len(second.Skipped) != 1 {
		t.Fatalf("second = %#v", second)
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, "use-a-check", "legacy", inv.Entries[0].BytesSHA256+".md")); err != nil {
		t.Fatalf("raw legacy entry not retained: %v", err)
	}
}

func TestApplyLegacy_InvalidMappingIsWriteFree(t *testing.T) {
	dir := t.TempDir()
	lessonsDir := filepath.Join(dir, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "legacy.md")
	if err := os.WriteFile(source, []byte("## L1 — rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv, err := InventoryLegacy(source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ApplyLegacy(lessonsDir, inv, LegacyMapping{SourceSHA256: inv.SourceSHA256, Entries: []LegacyMappingEntry{{Key: "L1#1", Action: "new", Slug: "Bad_Slug"}}})
	if err == nil {
		t.Fatal("expected invalid mapping")
	}
	if _, err := os.Stat(filepath.Join(lessonsDir, ".legacy-import")); !os.IsNotExist(err) {
		t.Fatalf("invalid mapping wrote manifest: %v", err)
	}
}
