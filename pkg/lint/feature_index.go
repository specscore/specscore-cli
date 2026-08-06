package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/feature"
)

// featureIndexChecker dispatches the feature-index-row-sync rule. The
// rule logic lives in `featureIndexRules` and is invoked twice — once
// in report mode from `check()`, once in mutation mode from `fix()` —
// matching the `checker` / `fixer` split used by every other rule in
// this package.
type featureIndexChecker struct{}

func newFeatureIndexChecker() *featureIndexChecker {
	return &featureIndexChecker{}
}

func (c *featureIndexChecker) name() string     { return "feature-index-row-sync" }
func (c *featureIndexChecker) severity() string { return "error" }

func (c *featureIndexChecker) check(specRoot string) ([]Violation, error) {
	vs, _ := featureIndexRules(specRoot, false)
	return vs, nil
}

// fix implements the fixer interface: rewrites drifted derived cells in
// the features-index to match each feature README. The check
// pass that follows reports zero violations because the rewrite is
// complete; idempotency is satisfied because the second pass finds no
// drift to rewrite.
func (c *featureIndexChecker) fix(specRoot string) error {
	_, _ = featureIndexRules(specRoot, true)
	return nil
}

// featureIndexRules enforces:
//   - feature-index-row-sync: each top-level row's title, `Status`, and
//     `Description` (when that column exists) in
//     spec/features/README.md mirrors the corresponding feature's
//     `**Status:**` value at spec/features/<feature_id>/README.md.
//     Drift typically arises after a Status line is rewritten by hand
//     (or by a future `change-status` verb); the index row, being
//     derived state, must follow.
//
// Scope: top-level features only (entries whose slug contains a "/"
// are sub-features and are NOT listed in the features-index).
//
// What --fix does: rewrites the derived cells in the index row to match the
// feature README. Other schema-specific cells remain untouched.
func featureIndexRules(specRoot string, fix bool) ([]Violation, bool) {
	var vs []Violation
	fixed := false

	featuresDir := filepath.Join(specRoot, "features")
	if info, err := os.Stat(featuresDir); err != nil || !info.IsDir() {
		return nil, false
	}

	indexPath := filepath.Join(featuresDir, "README.md")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, false
	}

	rows, err := readFeatureIndexRows(indexPath)
	if err != nil {
		return nil, false
	}

	type drift struct {
		slug         string
		actual, want featureIndexValue
		lineNum      int
	}
	var drifts []drift
	for _, r := range rows {
		// Top-level features only — sub-features (slug contains "/")
		// are not listed in the features-index.
		if strings.Contains(r.slug, "/") {
			continue
		}
		featureReadme := filepath.Join(featuresDir, r.slug, "README.md")
		if _, err := os.Stat(featureReadme); err != nil {
			// No matching feature directory — that is an
			// orphaned row, which is a different concern from
			// row-sync. Skip silently here; other rules
			// (or future feature-index-completeness) would
			// cover it.
			continue
		}
		status, err := feature.ParseFeatureStatus(featureReadme)
		if err != nil {
			continue
		}
		title, err := feature.ParseFeatureTitle(featureReadme)
		if err != nil {
			continue
		}
		summary, err := parseFeatureIndexSummary(featureReadme)
		if err != nil {
			continue
		}
		// Older feature files may not have a Summary section. Do not invent an
		// empty description for them; once a summary exists it is canonical.
		if summary == "" {
			summary = r.summary
		}
		want := featureIndexValue{title: title, status: status, summary: summary}
		if r.title != want.title || r.status != want.status || (r.hasDescription && r.summary != want.summary) {
			drifts = append(drifts, drift{
				slug: r.slug, actual: featureIndexValue{title: r.title, status: r.status, summary: r.summary}, want: want, lineNum: r.lineNum,
			})
		}
	}

	if len(drifts) == 0 {
		return nil, false
	}

	rel, _ := filepath.Rel(specRoot, indexPath)

	if fix {
		updates := make(map[string]featureIndexValue, len(drifts))
		for _, d := range drifts {
			updates[d.slug] = d.want
		}
		if err := rewriteFeatureIndexRows(indexPath, updates); err == nil {
			return nil, true
		}
		// Fall through to reporting if the rewrite failed.
		slugs := make([]string, 0, len(drifts))
		for _, d := range drifts {
			slugs = append(slugs, d.slug)
		}
		sort.Strings(slugs)
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "feature-index-row-sync",
			Message: fmt.Sprintf("features-index rows drifted from feature READMEs: %s (fix failed)", strings.Join(slugs, ", ")),
		})
		return vs, false
	}

	sort.Slice(drifts, func(i, j int) bool { return drifts[i].slug < drifts[j].slug })
	for _, d := range drifts {
		vs = append(vs, Violation{
			File: rel, Line: d.lineNum, Severity: "error",
			Rule:    "feature-index-row-sync",
			Message: fmt.Sprintf("features-index row for %q is stale (title/status/summary) (run `specscore spec lint --fix`)", d.slug),
		})
	}
	return vs, fixed
}

// featureIndexRow captures one parsed top-level row of the features
type featureIndexRow struct {
	slug, title, status, summary string
	hasDescription               bool
	lineNum                      int
}

type featureIndexValue struct{ title, status, summary string }

// featureIndexRowRe matches one row of the features-index table whose
// first cell is a `[<slug>](<slug>/README.md)` link and whose second
// cell is the row's Status. Trailing cells (Kind, URL, Consumer Path,
// Index, Description, ... — schemas vary across repos) are matched but
// not captured here; rewriteFeatureIndexStatuses preserves them verbatim
// by splitting the row on `|` rather than re-emitting from captures.
//
// At least three cells (link, status, one more) are required so that
// the match cannot accidentally fire on an isolated link line.
var featureIndexRowRe = regexp.MustCompile(`^\|\s*\[[^\]]+\]\(([^)]+)/README\.md\)\s*\|\s*([^|]*?)\s*\|.+\|\s*$`)

// readFeatureIndexRows scans the features-index README and returns one
// featureIndexRow per row of the top-level table. Header and separator
// lines are skipped. Rows whose slug contains "/" (deeper links) are
// retained so the caller can filter; the caller is responsible for
// excluding sub-features from the row-sync check.
func readFeatureIndexRows(path string) ([]featureIndexRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var rows []featureIndexRow
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		m := featureIndexRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|"), "|")
		row := featureIndexRow{slug: m[1], title: strings.TrimSpace(strings.TrimPrefix(strings.Split(strings.TrimSpace(parts[0]), "](")[0], "[")), status: strings.TrimSpace(m[2]), lineNum: lineNum}
		// The current canonical root index names this column Description. Keep
		// legacy schemas without it status/title-only rather than guessing.
		if len(parts) >= 4 {
			row.hasDescription, row.summary = true, strings.TrimSpace(parts[len(parts)-1])
		}
		rows = append(rows, row)
	}
	return rows, scanner.Err()
}

func rewriteFeatureIndexRows(path string, updates map[string]featureIndexValue) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		m := featureIndexRowRe.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		slug := m[1]
		update, ok := updates[slug]
		if !ok {
			continue
		}
		// Cells: trailing `|` then leading `|` produces empty first/last
		// strings after Split — splice the Status cell (index 2) and
		// rejoin without re-emitting the link cell, so any extra cells
		// (kind/url/consumer-path/index/description) round-trip exactly.
		// Preserve the original surrounding whitespace by re-padding the
		// Status cell with a single space on each side.
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|"), "|")
		if len(parts) < 3 {
			continue
		}
		parts[0] = " [" + update.title + "](" + slug + "/README.md) "
		parts[1] = " " + update.status + " "
		if len(parts) >= 4 {
			parts[len(parts)-1] = " " + update.summary + " "
		}
		lines[i] = "|" + strings.Join(parts, "|") + "|"
		changed = true
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// rewriteFeatureIndexStatuses remains as a narrow compatibility helper for
// existing callers/tests. The row-sync fixer uses rewriteFeatureIndexRows so
// title and Description stay canonical too.
func rewriteFeatureIndexStatuses(path string, updates map[string]string) error {
	rows, err := readFeatureIndexRows(path)
	if err != nil {
		return err
	}
	values := make(map[string]featureIndexValue, len(updates))
	for _, row := range rows {
		if status, ok := updates[row.slug]; ok {
			values[row.slug] = featureIndexValue{title: row.title, status: status, summary: row.summary}
		}
	}
	return rewriteFeatureIndexRows(path, values)
}

func parseFeatureIndexSummary(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	in := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Summary" {
			in = true
			continue
		}
		if in && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if in && trimmed != "" {
			return trimmed, nil
		}
	}
	return "", nil
}
