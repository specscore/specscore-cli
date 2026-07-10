// Package ingr writes SpecScore Studio facts as INGR recordsets — the same
// fixed-header/one-JSON-value-per-line encoding the CodeGrapher snapshots
// committed under codegraph/ use (and the codegraph adapter parses).
//
// Feature: cli/studio/index (REQ: ingr-export)
//
// Layout: one directory per repo slug under the export root, each holding a
// single facts.ingr recordset with one record per fact attributed to that
// repo. The file starts with the fixed header
//
//	# INGR.io | facts: subject, predicate, object, evidence_class, evidence_pointer, adapter_id, adapter_version, observed_at, ecosystem
//
// followed by one JSON string per line (nine lines per record, in header
// order) and a trailing `# <n> records` comment. The export root is replaced
// wholesale on every run — like the fact store, it is a disposable cache.
package ingr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	osRemoveAllFn = os.RemoveAll
	osMkdirAllFn  = os.MkdirAll
	osWriteFileFn = os.WriteFile
)

// header is the fixed first line of every facts.ingr recordset: the field
// layout, one field per value line, in this order.
const header = "# INGR.io | facts: subject, predicate, object, evidence_class, evidence_pointer, adapter_id, adapter_version, observed_at, ecosystem"

// Repo is one repo's slice of an export: its stable slug (the directory name
// under the export root) and the facts the ingestion run attributed to it.
type Repo struct {
	Slug  string
	Facts []fact.Fact
}

// Export replaces the recordset tree at dir with one directory per repo,
// each containing a facts.ingr recordset holding exactly that repo's facts
// (a repo with zero facts gets an empty recordset, keeping the per-repo
// record counts comparable with the index summary). Any failure aborts the
// export and returns an error for the caller to downgrade to a warning
// (REQ: ingr-export — the store stays the query surface).
func Export(dir string, repos []Repo) error {
	if err := osRemoveAllFn(dir); err != nil {
		return fmt.Errorf("clearing INGR export directory %s: %w", dir, err)
	}
	for _, repo := range repos {
		repoDir := filepath.Join(dir, repo.Slug)
		if err := osMkdirAllFn(repoDir, 0o755); err != nil {
			return fmt.Errorf("creating INGR export directory %s: %w", repoDir, err)
		}
		path := filepath.Join(repoDir, "facts.ingr")
		if err := osWriteFileFn(path, encode(repo.Facts), 0o644); err != nil {
			return fmt.Errorf("writing INGR recordset %s: %w", path, err)
		}
	}
	return nil
}

// encode renders one facts recordset: header, nine JSON-string lines per
// fact, `# <n> records` trailer.
func encode(facts []fact.Fact) []byte {
	var b strings.Builder
	b.WriteString(header + "\n")
	for _, f := range facts {
		for _, v := range []string{
			f.Subject, f.Predicate, f.Object,
			string(f.Class), f.Pointer,
			f.Adapter.ID, f.Adapter.Version,
			f.ObservedAt, f.Ecosystem,
		} {
			line, _ := json.Marshal(v) // marshaling a string never fails
			b.Write(line)
			b.WriteByte('\n')
		}
	}
	fmt.Fprintf(&b, "# %d records\n", len(facts))
	return []byte(b.String())
}
