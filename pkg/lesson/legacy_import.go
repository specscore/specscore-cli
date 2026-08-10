package lesson

// Deterministic, lossless legacy-log inventory and reviewed application.
// Importers record source evidence before creating any compact artefact; they
// never renumber IDs or make a semantic duplicate decision from prose.

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type LegacyEntry struct {
	Key           string   `json:"key"`
	LegacyID      string   `json:"legacy_id"`
	Ordinal       int      `json:"ordinal"`
	Title         string   `json:"title"`
	StartLine     int      `json:"start_line"`
	EndLine       int      `json:"end_line"`
	RawStatus     string   `json:"raw_status,omitempty"`
	BytesSHA256   string   `json:"bytes_sha256"`
	Raw           string   `json:"raw,omitempty"`
	SuggestedSlug string   `json:"suggested_slug"`
	Warnings      []string `json:"warnings,omitempty"`
}
type LegacyInventory struct {
	Source       string        `json:"source"`
	SourceSHA256 string        `json:"source_sha256"`
	Entries      []LegacyEntry `json:"entries"`
}
type LegacyMapping struct {
	SourceSHA256 string               `json:"source_sha256"`
	Entries      []LegacyMappingEntry `json:"entries"`
}
type LegacyMappingEntry struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Slug   string `json:"slug,omitempty"`
	Title  string `json:"title,omitempty"`
}
type LegacyApplyResult struct {
	CreatedLessons     []string `json:"created_lessons"`
	CreatedOccurrences []string `json:"created_occurrences"`
	Manual             []string `json:"manual"`
	Skipped            []string `json:"skipped"`
	Manifest           string   `json:"manifest"`
}

var legacyHeading = regexp.MustCompile(`^(?:##|###)\s+(L[0-9][A-Za-z0-9-]*)(?:\s*[—-]\s*|:\s*)(.*)$`)
var legacyStatus = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*(.+?)\s*$`)

func InventoryLegacy(source string) (LegacyInventory, error) {
	b, err := os.ReadFile(source)
	if err != nil {
		return LegacyInventory{}, err
	}
	sum := sha256.Sum256(b)
	inv := LegacyInventory{Source: source, SourceSHA256: hex.EncodeToString(sum[:])}
	lines := strings.SplitAfter(string(b), "\n")
	type start struct {
		line      int
		offset    int
		id, title string
	}
	var starts []start
	offset := 0
	for i, line := range lines {
		plain := strings.TrimRight(line, "\r\n")
		if m := legacyHeading.FindStringSubmatch(plain); m != nil {
			starts = append(starts, start{i + 1, offset, m[1], strings.TrimSpace(m[2])})
		}
		offset += len(line)
	}
	counts := map[string]int{}
	for i, s := range starts {
		endOffset, endLine := len(b), len(lines)
		if i+1 < len(starts) {
			endOffset, endLine = starts[i+1].offset, starts[i+1].line-1
		}
		raw := string(b[s.offset:endOffset])
		counts[s.id]++
		ordinal := counts[s.id]
		entry := LegacyEntry{Key: fmt.Sprintf("%s#%d", s.id, ordinal), LegacyID: s.id, Ordinal: ordinal, Title: s.title, StartLine: s.line, EndLine: endLine, Raw: raw, BytesSHA256: shaString([]byte(raw)), SuggestedSlug: legacySlug(s.id, s.title)}
		if m := legacyStatus.FindStringSubmatch(raw); m != nil {
			entry.RawStatus = strings.TrimSpace(m[1])
		} else {
			entry.Warnings = append(entry.Warnings, "status-not-found")
		}
		if ordinal > 1 {
			entry.Warnings = append(entry.Warnings, "duplicate-legacy-id")
		}
		if entry.Title == "" {
			entry.Warnings = append(entry.Warnings, "empty-title")
		}
		inv.Entries = append(inv.Entries, entry)
	}
	return inv, nil
}
func shaString(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func legacySlug(id, title string) string {
	raw := strings.ToLower(id + "-" + title)
	var b strings.Builder
	dash := false
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func ApplyLegacy(lessonsDir string, inv LegacyInventory, mapping LegacyMapping) (LegacyApplyResult, error) {
	if inv.SourceSHA256 != mapping.SourceSHA256 {
		return LegacyApplyResult{}, fmt.Errorf("mapping source_sha256 does not match unchanged source")
	}
	byKey := map[string]LegacyEntry{}
	for _, e := range inv.Entries {
		byKey[e.Key] = e
	}
	byMap := map[string]LegacyMappingEntry{}
	for _, m := range mapping.Entries {
		if _, found := byMap[m.Key]; found {
			return LegacyApplyResult{}, fmt.Errorf("mapping repeats key %q", m.Key)
		}
		byMap[m.Key] = m
	}
	for _, e := range inv.Entries {
		if _, ok := byMap[e.Key]; !ok {
			return LegacyApplyResult{}, fmt.Errorf("mapping lacks explicit disposition for %s", e.Key)
		}
	}
	// Validate every disposition and target before creating the manifest or a
	// single artifact. An unreviewed/invalid mapping is therefore write-free,
	// not a partially imported repository that needs manual repair.
	for key, m := range byMap {
		e := byKey[key]
		switch m.Action {
		case "manual":
		case "new":
			if err := ValidateSlug(m.Slug); err != nil {
				return LegacyApplyResult{}, fmt.Errorf("mapping %s: %w", key, err)
			}
			target := filepath.Join(lessonsDir, m.Slug, "README.md")
			if _, err := os.Stat(target); err == nil && !hasLegacyProvenance(target, e.Key, inv.SourceSHA256) {
				return LegacyApplyResult{}, fmt.Errorf("mapping %s target exists without matching provenance: %s", key, target)
			}
		case "occurrence":
			if err := ValidateSlug(m.Slug); err != nil {
				return LegacyApplyResult{}, fmt.Errorf("mapping %s: %w", key, err)
			}
			path, err := ResolveLessonFile(lessonsDir, m.Slug)
			if err != nil {
				return LegacyApplyResult{}, fmt.Errorf("mapping %s occurrence target: %w", key, err)
			}
			l, err := Parse(path)
			if err != nil || !l.Canonical {
				return LegacyApplyResult{}, fmt.Errorf("mapping %s occurrence target must be a canonical Lesson", key)
			}
		default:
			return LegacyApplyResult{}, fmt.Errorf("mapping %s has invalid action %q (new, occurrence, manual)", key, m.Action)
		}
	}
	result := LegacyApplyResult{}
	manifestPath, err := writeImportManifest(lessonsDir, inv, mapping)
	if err != nil {
		return result, err
	}
	result.Manifest = manifestPath
	keys := make([]string, 0, len(byMap))
	for key := range byMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		e, m := byKey[key], byMap[key]
		switch m.Action {
		case "manual":
			result.Manual = append(result.Manual, key)
			continue
		case "new":
			target := filepath.Join(lessonsDir, m.Slug, "README.md")
			if _, err := os.Stat(target); err == nil {
				if hasLegacyProvenance(target, e.Key, inv.SourceSHA256) {
					result.Skipped = append(result.Skipped, key)
					continue
				}
				return result, fmt.Errorf("mapping %s target exists without matching provenance: %s", key, target)
			}
			title := m.Title
			if title == "" {
				title = e.Title
			}
			if title == "" {
				title = m.Slug
			}
			if err := writeImportedLesson(target, m.Slug, title, e, inv); err != nil {
				return result, err
			}
			result.CreatedLessons = append(result.CreatedLessons, m.Slug)
		case "occurrence":
			path, err := ResolveLessonFile(lessonsDir, m.Slug)
			if err != nil {
				return result, err
			}
			// Explicit mapping is the human decision that this historical record
			// is a useful recurrence; generic unknown context avoids fabrication.
			o, err := AddOccurrence(AddOccurrenceOptions{LessonPath: path, Summary: "Imported legacy occurrence " + e.Key, Context: map[string]any{"execution": map[string]any{"kind": "unknown"}}, Evidence: Evidence{Kind: "path", Ref: stringPtr(".legacy-import/" + inv.SourceSHA256 + ".json")}, Redactions: []string{"legacy-unstructured-context"}})
			if err != nil {
				return result, err
			}
			result.CreatedOccurrences = append(result.CreatedOccurrences, o.ID)
		default:
			panic("validated mapping action")
		}
	}
	return result, nil
}
func stringPtr(s string) *string { return &s }
func hasLegacyProvenance(path, key, sourceHash string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), "legacy-import:"+sourceHash+"#"+key)
}
func writeImportedLesson(target, slug, title string, e LegacyEntry, inv LegacyInventory) error {
	if err := os.MkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
		return err
	}
	provenance := "legacy-import:" + inv.SourceSHA256 + "#" + e.Key
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: slug, Title: title, Owner: "legacy-import"}, []string{"process"})
	if err != nil {
		return err
	}
	s := strings.Replace(string(body), "**Legacy Provenance:** —", "**Legacy Provenance:** "+provenance, 1)
	if err := os.WriteFile(target, []byte(s), 0o644); err != nil {
		return err
	}
	legacyDir := filepath.Join(filepath.Dir(target), "legacy")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(legacyDir, e.BytesSHA256+".md"), []byte(e.Raw), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(legacyDir, "README.md"), []byte("# Legacy evidence\n\nLossless source entry: `"+e.Key+"`. Source digest: `"+inv.SourceSHA256+"`.\n"), 0o644)
}
func writeImportManifest(lessonsDir string, inv LegacyInventory, mapping LegacyMapping) (string, error) {
	dir := filepath.Join(lessonsDir, ".legacy-import")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, inv.SourceSHA256+".json")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	source, err := os.ReadFile(inv.Source)
	if err != nil {
		return "", err
	}
	payload := struct {
		LegacyInventory
		Mapping           LegacyMapping `json:"mapping"`
		SourceBytesBase64 string        `json:"source_bytes_base64"`
	}{inv, mapping, base64.StdEncoding.EncodeToString(source)}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
