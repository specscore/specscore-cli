package feature

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// NewOptions holds the parameters for creating a new feature.
type NewOptions struct {
	Title       string   // Human-readable feature title (required).
	Slug        string   // Feature slug (directory name); auto-generated from Title if empty.
	Parent      string   // Parent feature ID for creating a sub-feature.
	Status      string   // Initial feature status (default "Draft").
	Description string   // Short description for the Summary section.
	DependsOn   []string // Feature IDs this feature depends on.
	// Body, when non-empty, is used verbatim as the README content instead of
	// the generated template (e.g. a template fetched from the gallery by the
	// CLI's runtime-fetch). The caller is responsible for its validity.
	Body string
}

// NewResult describes the outcome of creating a new feature.
type NewResult struct {
	FeatureID    string   // The created feature's full ID.
	FeatureDir   string   // Absolute path to the created feature directory.
	ReadmePath   string   // Absolute path to the created README.md.
	ChangedFiles []string // All files that were created or modified.
	Info         Info     // Metadata for the newly created feature.
}

// New scaffolds a new feature directory with a README template.
// It does NOT perform any git operations — those belong in the CLI layer.
func New(featuresDir string, opts NewOptions) (*NewResult, error) {
	if opts.Title == "" {
		return nil, exitcode.InvalidArgsError("missing required option: Title")
	}

	status := opts.Status
	if status == "" {
		status = "Draft"
	}
	if !IsValidStatus(status) {
		return nil, exitcode.InvalidArgsErrorf("invalid status: %s (supported: Draft, Under Review, Approved, Implementing, Stable, Deprecated)", status)
	}

	// Generate or validate slug.
	slug := opts.Slug
	if slug == "" {
		slug = GenerateSlug(opts.Title)
	} else {
		if err := ValidateSlug(slug); err != nil {
			return nil, exitcode.InvalidArgsErrorf("invalid slug: %v", err)
		}
	}

	// Validate mutual exclusion: --parent vs slash-in-slug.
	if opts.Parent != "" && strings.Contains(slug, "/") {
		return nil, exitcode.InvalidArgsError(
			"cannot use Parent with a slug containing slashes; use one or the other",
		)
	}

	// Validate dependency feature IDs exist.
	for _, dep := range opts.DependsOn {
		if !Exists(featuresDir, dep) {
			return nil, exitcode.InvalidArgsErrorf("dependency feature not found: %s", dep)
		}
	}

	// Resolve full feature path.
	var featureID string
	var parentID string

	switch {
	case opts.Parent != "":
		featureID = opts.Parent + "/" + slug
		parentID = opts.Parent
	case strings.Contains(slug, "/"):
		featureID = slug
		parts := strings.Split(slug, "/")
		parentID = strings.Join(parts[:len(parts)-1], "/")
	default:
		featureID = slug
	}

	featureDir := filepath.Join(featuresDir, filepath.FromSlash(featureID))

	// Validate parent exists (for sub-features).
	if parentID != "" {
		if !Exists(featuresDir, parentID) {
			return nil, exitcode.NotFoundErrorf("parent feature not found: %s", parentID)
		}
	}

	// Verify target doesn't exist.
	if _, err := os.Stat(featureDir); err == nil {
		return nil, exitcode.InvalidStateErrorf("feature already exists at: %s", featureID)
	}

	// Create feature directory and README.
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		return nil, exitcode.UnexpectedErrorf("creating feature directory: %v", err)
	}

	readme := opts.Body
	if readme == "" {
		readme = GenerateReadme(opts.Title, status, opts.Description, opts.DependsOn)
	}
	readmePath := filepath.Join(featureDir, "README.md")
	if err := osWriteFile(readmePath, []byte(readme), 0o644); err != nil {
		return nil, exitcode.UnexpectedErrorf("writing README.md: %v", err)
	}

	changedFiles := []string{readmePath}

	// Update parent's Contents section (sub-features).
	if parentID != "" {
		parentReadme := ReadmePath(featuresDir, parentID)
		changed, err := UpdateParentContents(parentReadme, filepath.Base(featureDir), opts.Description)
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("updating parent contents: %v", err)
		}
		if changed {
			changedFiles = append(changedFiles, parentReadme)
		}
	}

	// Update feature index for top-level features.
	if parentID == "" {
		indexPath := filepath.Join(featuresDir, "README.md")
		changed, err := UpdateFeatureIndex(indexPath, featureID, status, opts.Description)
		if err != nil {
			return nil, exitcode.UnexpectedErrorf("updating feature index: %v", err)
		}
		if changed {
			changedFiles = append(changedFiles, indexPath)
		}
	}

	// Build info for the result.
	sections, err := parseSectionsFn(readmePath)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("parsing sections: %v", err)
	}

	deps := opts.DependsOn
	if deps == nil {
		deps = []string{}
	}

	info := Info{
		Path:     featureID,
		Status:   status,
		Deps:     deps,
		Refs:     []string{},
		Children: nil,
		Plans:    nil,
		Sections: sections,
	}

	return &NewResult{
		FeatureID:    featureID,
		FeatureDir:   featureDir,
		ReadmePath:   readmePath,
		ChangedFiles: changedFiles,
		Info:         info,
	}, nil
}

// UpdateParentContents adds or updates the ## Contents section in the
// parent's README. Returns true if the file was modified.
func UpdateParentContents(parentReadmePath, childSlug, description string) (bool, error) {
	content, err := os.ReadFile(parentReadmePath)
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(content), "\n")
	desc := description
	if desc == "" {
		desc = "TODO: Add description."
	}

	newRow := fmt.Sprintf("| [%s](%s/README.md) | %s |", childSlug, childSlug, desc)

	contentsIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "## Contents" {
			contentsIdx = i
			break
		}
	}

	if contentsIdx >= 0 {
		insertIdx := contentsIdx + 1
		for insertIdx < len(lines) {
			trimmed := strings.TrimSpace(lines[insertIdx])
			if strings.HasPrefix(trimmed, "## ") && trimmed != "## Contents" {
				break
			}
			insertIdx++
		}
		for insertIdx > contentsIdx+1 && strings.TrimSpace(lines[insertIdx-1]) == "" {
			insertIdx--
		}
		lines = append(lines[:insertIdx+1], lines[insertIdx:]...)
		lines[insertIdx] = newRow
	} else {
		summaryIdx := -1
		for i, line := range lines {
			if strings.TrimSpace(line) == "## Summary" {
				summaryIdx = i
				break
			}
		}

		insertAfter := 0
		if summaryIdx >= 0 {
			insertAfter = summaryIdx + 1
			for insertAfter < len(lines) {
				trimmed := strings.TrimSpace(lines[insertAfter])
				if strings.HasPrefix(trimmed, "## ") {
					break
				}
				insertAfter++
			}
		}

		contentsBlock := []string{
			"## Contents",
			"",
			"| Child | Description |",
			"|---|---|",
			newRow,
			"",
		}

		newLines := make([]string, 0, len(lines)+len(contentsBlock))
		newLines = append(newLines, lines[:insertAfter]...)
		newLines = append(newLines, contentsBlock...)
		newLines = append(newLines, lines[insertAfter:]...)
		lines = newLines
	}

	result := strings.Join(lines, "\n")
	if err := os.WriteFile(parentReadmePath, []byte(result), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateFeatureIndex adds a new row to the feature index at
// spec/features/README.md. Returns true if the file was modified.
//
// The emitted row's column count matches the existing table header so
// repos with different schemas (4-column `Feature|Status|Kind|Description`,
// 7-column `Feature|Status|Kind|URL|Consumer Path|Index|Description`, etc.)
// all get well-formed rows. Per-column values are inferred from known
// header names (see indexRowCellFor); unknown columns get a `—` placeholder
// for human curation. When no header can be parsed, the function falls back
// to a 4-cell `Feature | Status | Kind | Description` row — the historical
// default emitted by `specscore feature new`.
func UpdateFeatureIndex(indexPath, featureID, status, description string) (bool, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	lines := strings.Split(string(content), "\n")
	statusCell := status
	if statusCell == "" {
		statusCell = "Draft"
	}
	desc := description
	if desc == "" {
		desc = "TODO: Add description."
	}

	// featureDir is the parent dir of the index — child feature READMEs
	// live at <featureDir>/<slug>/README.md. Used to read the target's
	// adherence-footer URL when the index has a URL column.
	featureDir := filepath.Dir(indexPath)

	headers, headerLine := findIndexTableHeader(lines)
	var newRow string
	if headers != nil {
		cells := make([]string, len(headers))
		for i, h := range headers {
			cells[i] = indexRowCellFor(strings.ToLower(strings.TrimSpace(h)), featureID, statusCell, desc, featureDir)
		}
		newRow = "| " + strings.Join(cells, " | ") + " |"
	} else {
		// No header parseable — fall back to the legacy 4-cell shape.
		newRow = fmt.Sprintf("| [%s](%s/README.md) | %s | — | %s |", featureID, featureID, statusCell, desc)
	}

	tableEnd := findLastTableRow(lines, headerLine)

	if tableEnd >= 0 {
		insertIdx := tableEnd + 1
		lines = append(lines[:insertIdx+1], lines[insertIdx:]...)
		lines[insertIdx] = newRow
	} else {
		lines = append(lines, "", newRow)
	}

	result := strings.Join(lines, "\n")
	if err := os.WriteFile(indexPath, []byte(result), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// findIndexTableHeader scans `lines` for the first table-header row — a
// `|`-bounded line immediately followed by a separator row like
// `|---|---|`. Returns the parsed header cells (trimmed, NOT lowercased)
// and the line index of the header. Returns (nil, -1) when no header is
// found.
func findIndexTableHeader(lines []string) ([]string, int) {
	for i := 0; i < len(lines)-1; i++ {
		header := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(header, "|") || !strings.HasSuffix(header, "|") {
			continue
		}
		next := strings.TrimSpace(lines[i+1])
		if !isTableSeparatorRow(next) {
			continue
		}
		return splitTableRow(header), i
	}
	return nil, -1
}

// isTableSeparatorRow reports whether s is a markdown table separator row
// like `|---|---|` or `| :--- | ---: |`. Every cell must be non-empty and
// composed of `-`, `:`, and whitespace only.
func isTableSeparatorRow(s string) bool {
	if !strings.HasPrefix(s, "|") || !strings.HasSuffix(s, "|") {
		return false
	}
	cells := splitTableRow(s)
	for _, c := range cells {
		c = strings.TrimSpace(c)
		if c == "" {
			return false
		}
		for _, r := range c {
			if r != '-' && r != ':' && r != ' ' && r != '\t' {
				return false
			}
		}
	}
	return true
}

// splitTableRow trims outer `|` and splits on `|`, returning the per-cell
// strings. Cells are NOT trimmed.
func splitTableRow(s string) []string {
	inner := strings.TrimPrefix(strings.TrimSuffix(s, "|"), "|")
	return strings.Split(inner, "|")
}

// findLastTableRow returns the line index of the last `|`-prefixed row in
// the table that starts at headerLine. If headerLine is -1, falls back to
// scanning every line (legacy behavior). Returns -1 when no row is found.
func findLastTableRow(lines []string, headerLine int) int {
	start := 0
	if headerLine >= 0 {
		start = headerLine
	}
	tableEnd := -1
	inTable := false
	for i := start; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "|") {
			inTable = true
			tableEnd = i
		} else if inTable && trimmed == "" {
			break
		} else if inTable && strings.HasPrefix(trimmed, "## ") {
			break
		}
	}
	return tableEnd
}

// indexRowCellFor returns the cell value for a column whose header (already
// lowercased and trimmed) is h. Unknown headers get `—` so the row remains
// structurally valid and a human can fill it in. The slug is used both for
// the feature link and as the basis for the conventional spec URL.
//
// featureDir is the parent directory of the index file; the target feature
// README is read from `<featureDir>/<slug>/README.md` when needed (e.g. to
// extract the adherence-footer URL for the URL column).
func indexRowCellFor(h, slug, status, description, featureDir string) string {
	switch h {
	case "feature", "name", "child":
		return fmt.Sprintf("[%s](%s/README.md)", slug, slug)
	case "status":
		return status
	case "description", "desc":
		return description
	case "kind":
		if strings.HasSuffix(slug, "-index") {
			return "Index"
		}
		return "—"
	case "url":
		if u := readAdherenceFooterURL(filepath.Join(featureDir, slug, "README.md")); u != "" {
			return u
		}
		return "—"
	default:
		return "—"
	}
}

// readAdherenceFooterURL reads the target feature README and returns the
// canonical spec URL declared in its adherence footer
// (`*This document follows the https://specscore.md/<x>-specification*`).
// Returns "" when no such footer is found.
func readAdherenceFooterURL(readmePath string) string {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}
	const marker = "*This document follows the "
	idx := strings.LastIndex(string(data), marker)
	if idx < 0 {
		return ""
	}
	rest := string(data)[idx+len(marker):]
	end := strings.Index(rest, "*")
	if end < 0 {
		return ""
	}
	url := strings.TrimSpace(rest[:end])
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}
	return url
}
