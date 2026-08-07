package decision

// index.go — direct, targeted edits to the two decisions-index surfaces
// (spec/decisions/README.md's active table and
// spec/decisions/archived/README.md's chronological list), used by
// ChangeStatus when a transition relocates a file.
//
// pkg/lint's decisions-index fixer (pkg/lint/decisions_index_rules.go) can
// ADD a missing row to the ACTIVE table from scratch (the mechanism `decision
// new` relies on) but has no fixer that removes a stale active row or adds a
// missing archived-list entry — unlike the ideas index, which fully
// regenerates both surfaces from discovered files on every `--fix` pass. This
// file closes that gap at the point where it actually needs closing: the one
// verb that relocates a Decision file. See transitions.go's package doc for
// why this lives here rather than as a change to the generic lint fixer.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// EnsureArchivedIndexStub guarantees that spec/decisions/archived/ exists and
// contains a lint-clean README.md index. It creates the directory if absent
// and writes archivedDecisionsIndexStub to archived/README.md ONLY when that
// file does not already exist. It returns created=true iff it wrote the stub,
// so callers can undo exactly what they materialized on rollback.
//
// Mirrors idea.EnsureArchivedIndexStub. specRoot is the project root that
// contains the spec/ subtree (NOT the spec/ directory itself).
func EnsureArchivedIndexStub(specRoot string) (created bool, err error) {
	archivedDir := filepath.Join(specRoot, "spec", "decisions", "archived")
	if err := osMkdirAllFn(archivedDir, 0o755); err != nil {
		return false, exitcode.UnexpectedErrorf("creating archived directory %s: %v", archivedDir, err)
	}
	archivedReadme := filepath.Join(archivedDir, "README.md")
	if _, statErr := osStatFn(archivedReadme); os.IsNotExist(statErr) {
		if werr := osWriteFileFn(archivedReadme, []byte(archivedDecisionsIndexStub), 0o644); werr != nil {
			return false, exitcode.UnexpectedErrorf("creating archived index stub %s: %v", archivedReadme, werr)
		}
		return true, nil
	} else if statErr != nil {
		return false, exitcode.UnexpectedErrorf("stat archived index %s: %v", archivedReadme, statErr)
	}
	return false, nil
}

// archivedDecisionsIndexStub is the minimal lint-clean content written to
// spec/decisions/archived/README.md on the first archival transition. Mirrors
// the shape decisionsIndexContent (internal/cli/init.go) uses for the active
// index, adapted to the archived list's bullet format (no table).
const archivedDecisionsIndexStub = `# Archived Decisions

_No archived decisions yet._

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/decisions-index-specification*
`

// activeIndexRowRe matches one data row of the active decisions index table
// for a specific slug: `| [<N>](<slug>.md) | ... |`. The bracket text is the
// bare zero-padded sequence number (canonicalDecisionIndexRow in
// pkg/lint/decisions_index_rules.go renders it as row.numStr, NOT the full
// slug) — only the parenthesized link target carries the full slug. Built
// per-call from the slug so the match is anchored to exactly that decision's
// row regardless of the bracket text.
func activeIndexRowRe(slug string) *regexp.Regexp {
	return regexp.MustCompile(`^\|\s*\[[^\]]*\]\(` + regexp.QuoteMeta(slug) + `\.md\)\s*\|`)
}

// removeActiveIndexRow deletes the row for slug from the `## Decisions` table
// in the active decisions index (spec/decisions/README.md). It returns the
// pre-edit file bytes (for rollback) and changed=true iff a row was actually
// removed. A missing index file or a slug with no matching row is a no-op
// (changed=false, err=nil) — defensive, since D-completeness should already
// guarantee the row exists, but ChangeStatus must not fail the whole
// transition over an index that is already out of sync for unrelated reasons.
func removeActiveIndexRow(indexPath, slug string) (original []byte, changed bool, err error) {
	orig, err := osReadFileFn(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	re := activeIndexRowRe(slug)
	lines := strings.Split(string(orig), "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if re.MatchString(strings.TrimSpace(ln)) {
			changed = true
			continue
		}
		out = append(out, ln)
	}
	if !changed {
		return nil, false, nil
	}
	if err := osWriteFileFn(indexPath, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		return nil, false, err
	}
	return orig, true, nil
}

// archivedIndexEntryRe matches one bullet entry of the archived decisions
// index: `- YYYY-MM-DD — [slug](slug.md) — Status — reason`. Mirrors
// archivedDecisionEntryRe in pkg/lint/decisions_index_rules.go (kept
// independent per this package's no-pkg/lint-dependency layering rule).
var archivedIndexEntryRe = regexp.MustCompile(`^-\s+(\d{4}-\d{2}-\d{2})\s+—\s+\[([^\]]+)\]\(([^)]+)\)\s+—\s+(.+)$`)

// appendArchivedIndexEntry inserts a new chronologically-ordered bullet entry
// into the archived decisions index (spec/decisions/archived/README.md),
// replacing the `_No archived decisions yet._` placeholder on the first
// entry. It returns the pre-edit file bytes for rollback.
//
// The caller must have already ensured the index file exists (via
// EnsureArchivedIndexStub) — a missing file is a genuine error here, not a
// silent no-op, because a disposition transition MUST end with an archived
// entry recorded.
func appendArchivedIndexEntry(indexPath, date, slug, status, reason string) (original []byte, err error) {
	orig, err := osReadFileFn(indexPath)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(orig), "\n")

	oqIdx := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "## Open Questions" {
			oqIdx = i
			break
		}
	}
	prologueEnd := len(lines)
	if oqIdx >= 0 {
		prologueEnd = oqIdx
	}

	type entry struct {
		date, slug, raw string
	}
	var entries []entry
	insertionEnd := prologueEnd
	for i := 0; i < prologueEnd; i++ {
		t := strings.TrimSpace(lines[i])
		if m := archivedIndexEntryRe.FindStringSubmatch(t); m != nil {
			entries = append(entries, entry{date: m[1], slug: m[2], raw: t})
			continue
		}
		if strings.HasPrefix(t, "_No archived decisions yet._") && insertionEnd == prologueEnd {
			insertionEnd = i
		}
	}
	// If we never hit the placeholder but DID find entries, the insertion
	// point is just before the first entry line.
	if insertionEnd == prologueEnd && len(entries) > 0 {
		for i := 0; i < prologueEnd; i++ {
			if archivedIndexEntryRe.MatchString(strings.TrimSpace(lines[i])) {
				insertionEnd = i
				break
			}
		}
	}
	for insertionEnd > 0 && strings.TrimSpace(lines[insertionEnd-1]) == "" {
		insertionEnd--
	}

	newRaw := fmt.Sprintf("- %s — [%s](%s.md) — %s — %s", date, slug, slug, status, reason)
	entries = append(entries, entry{date: date, slug: slug, raw: newRaw})
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].date != entries[j].date {
			return entries[i].date < entries[j].date
		}
		return entries[i].slug < entries[j].slug
	})

	body := make([]string, 0, len(entries))
	for _, e := range entries {
		body = append(body, e.raw)
	}

	out := make([]string, 0, len(lines)+len(body)+4)
	out = append(out, lines[:insertionEnd]...)
	out = append(out, "", "")
	out = append(out, body...)
	if oqIdx >= 0 {
		out = append(out, "", "")
		out = append(out, lines[oqIdx:]...)
	}

	if err := osWriteFileFn(indexPath, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		return nil, err
	}
	return orig, nil
}
