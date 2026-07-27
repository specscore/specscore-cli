package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const plansIndexHeader = "| Plan | Status | Source | Date | Owner |"

// IndexContent returns the canonical plans-index content formed by replacing
// the Plan rows in content with rows derived from the flat Plan files in
// plansDir. It preserves every non-table byte, including Recently Closed and
// Open Questions sections. The returned changed flag reports whether writing
// the result would mutate the index.
func IndexContent(plansDir string, content []byte) ([]byte, bool, error) {
	plans, err := Discover(plansDir)
	if err != nil {
		return nil, false, err
	}

	lines := strings.Split(string(content), "\n")
	header := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == plansIndexHeader {
			header = i
			break
		}
	}
	// Historical repositories use a different plans-index schema. Leave that
	// format untouched; the canonical Plan-table projection applies only once
	// the current header has been adopted.
	if header < 0 {
		return content, false, nil
	}
	if header+1 >= len(lines) || !isPlansIndexSeparator(lines[header+1]) {
		return nil, false, fmt.Errorf("plans index must contain the canonical table headed %q", plansIndexHeader)
	}

	end := header + 2
	for end < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[end]), "|") {
		end++
	}

	rows := make([]string, 0, len(plans))
	for _, p := range plans {
		status := strings.TrimSpace(p.Status)
		if status == "" {
			status = "—"
		}
		source := strings.TrimSpace(p.SourceFeature)
		if source == "" {
			source = strings.TrimSpace(p.SourceRaw)
		}
		if source == "" {
			source = "—"
		}
		date := strings.TrimSpace(p.Date)
		if date == "" {
			date = "—"
		}
		owner := strings.TrimSpace(p.Owner)
		if owner == "" {
			owner = "—"
		}
		rows = append(rows, fmt.Sprintf("| [%s](%s.md) | %s | %s | %s | %s |", p.Slug, p.Slug, status, source, date, owner))
	}

	updated := make([]string, 0, len(lines)+len(rows))
	updated = append(updated, lines[:header+2]...)
	updated = append(updated, rows...)
	updated = append(updated, lines[end:]...)
	result := []byte(strings.Join(updated, "\n"))
	return result, string(result) != string(content), nil
}

// SyncIndex rewrites plansDir/README.md only when its derived Plan rows have
// drifted. It is safe to call after every Plan mutation and idempotent.
func SyncIndex(plansDir string) (bool, error) {
	indexPath := filepath.Join(plansDir, "README.md")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return false, err
	}
	updated, changed, err := IndexContent(plansDir, content)
	if err != nil || !changed {
		return changed, err
	}
	if err := os.WriteFile(indexPath, updated, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func isPlansIndexSeparator(line string) bool {
	fields := strings.Split(strings.TrimSpace(line), "|")
	if len(fields) != 7 {
		return false
	}
	for _, field := range fields[1 : len(fields)-1] {
		if strings.Trim(strings.TrimSpace(field), "-") != "" {
			return false
		}
	}
	return true
}
