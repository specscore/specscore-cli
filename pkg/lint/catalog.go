package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CatalogPath is the canonical location of the rendered lint-rule catalog,
// relative to the repository root (cli/rules#req:catalog-canonical-path).
const CatalogPath = "docs/lint-rules.md"

// RenderCatalog builds the full markdown lint-rule catalog from the registry.
//
// The output is deterministic: it derives entirely from AllRules() (itself an
// ID-sorted copy of the registry) and carries no timestamps, host, path, or
// other environment-dependent content. Families are emitted in sorted order,
// and rules within each family are listed in ID order, so re-rendering an
// unchanged registry yields byte-identical output
// (cli/rules#req:catalog-deterministic). The document is grouped by family,
// each rule showing its id and description (cli/rules#req:catalog-canonical-path).
func RenderCatalog() string {
	rules := append(AllRules(), GraphRules()...)

	// Group rules by family. AllRules() is already ID-sorted, so each family's
	// slice preserves stable ID ordering.
	byFamily := map[string][]Rule{}
	for _, r := range rules {
		byFamily[r.Family] = append(byFamily[r.Family], r)
	}

	families := make([]string, 0, len(byFamily))
	for fam := range byFamily {
		families = append(families, fam)
	}
	sort.Strings(families)

	var b strings.Builder
	b.WriteString("# Lint Rule Catalog\n\n")
	b.WriteString("Generated from the lint rule registry. Do not edit by hand.\n\n")
	b.WriteString(fmt.Sprintf("Total rules: %d\n", len(rules)))

	for _, fam := range families {
		b.WriteString("\n## ")
		b.WriteString(fam)
		b.WriteString("\n\n")
		b.WriteString("| Rule | Severity | Description |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, r := range byFamily[fam] {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", r.ID, r.Severity, r.Description)
		}
	}

	return b.String()
}

// WriteCatalog renders the catalog and writes it to CatalogPath under repoRoot,
// creating the parent directory if needed. It is a reusable helper for the
// commands that own the `--write`/`--check` behavior; it does not itself wire
// to any flag.
func WriteCatalog(repoRoot string) error {
	dest := filepath.Join(repoRoot, CatalogPath)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, []byte(RenderCatalog()), 0o644)
}
