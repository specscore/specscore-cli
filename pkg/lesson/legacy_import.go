package lesson

// Deterministic, lossless legacy-log inventory and reviewed application.
// Importers record source evidence before creating any compact artefact; they
// never renumber IDs or make a semantic duplicate decision from prose.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/specscore/specscore-cli/pkg/gitremote"
)

// LegacySourceRef identifies the exact immutable Git blob from which an
// inventory was derived. Committed migration artifacts retain this reference,
// byte ranges, and hashes; they never copy the historical prose itself.
type LegacySourceRef struct {
	Repository  string `json:"repository"`
	Path        string `json:"path"`
	Revision    string `json:"revision"`
	CommittedAt string `json:"committed_at"`
	SHA256      string `json:"sha256"`
	ByteCount   int    `json:"byte_count"`
}

type LegacyEntry struct {
	Key           string   `json:"key"`
	Kind          string   `json:"kind"`
	ParentKey     string   `json:"parent_key,omitempty"`
	LegacyID      string   `json:"legacy_id"`
	Ordinal       int      `json:"ordinal"`
	Title         string   `json:"title"`
	StartLine     int      `json:"start_line"`
	EndLine       int      `json:"end_line"`
	StartByte     int      `json:"start_byte"`
	EndByte       int      `json:"end_byte"`
	RawStatus     string   `json:"raw_status,omitempty"`
	BytesSHA256   string   `json:"bytes_sha256"`
	Raw           string   `json:"-"`
	SuggestedSlug string   `json:"suggested_slug"`
	Warnings      []string `json:"warnings,omitempty"`
}
type LegacyInventory struct {
	Source                LegacySourceRef   `json:"source"`
	LessonCount           int               `json:"lesson_count"`
	RecurrenceMarkerCount int               `json:"recurrence_marker_count"`
	EntryProjectionSHA256 string            `json:"entry_projection_sha256"`
	Collisions            []LegacyCollision `json:"collisions"`
	UnmatchedCandidates   []LegacyCandidate `json:"unmatched_candidates"`
	Warnings              []string          `json:"warnings,omitempty"`
	Entries               []LegacyEntry     `json:"entries"`
	localSource           string
}
type LegacyMapping struct {
	Source  LegacySourceRef      `json:"source"`
	Entries []LegacyMappingEntry `json:"entries"`
}

type LegacyCollision struct {
	LegacyID string   `json:"legacy_id"`
	Count    int      `json:"count"`
	Keys     []string `json:"keys"`
}

type LegacyCandidate struct {
	Kind        string `json:"kind"`
	Line        int    `json:"line"`
	BytesSHA256 string `json:"bytes_sha256"`
}
type LegacyMappingEntry struct {
	Key             string   `json:"key"`
	Action          string   `json:"action"`
	Slug            string   `json:"slug,omitempty"`
	Title           string   `json:"title,omitempty"`
	Lesson          string   `json:"lesson,omitempty"`
	ProcessGap      string   `json:"process_gap,omitempty"`
	Status          string   `json:"status,omitempty"`
	Classifications []string `json:"classifications,omitempty"`
}
type LegacyApplyResult struct {
	CreatedLessons     []string               `json:"created_lessons"`
	CreatedOccurrences []string               `json:"created_occurrences"`
	StatusDecisions    []LegacyStatusDecision `json:"status_decisions"`
	Manual             []string               `json:"manual"`
	Skipped            []string               `json:"skipped"`
	Manifest           string                 `json:"manifest"`
}

type LegacyStatusDecision struct {
	Key            string `json:"key"`
	SourceStatus   string `json:"source_status,omitempty"`
	ImportedStatus string `json:"imported_status"`
	Reason         string `json:"reason"`
}

var legacyHeading = regexp.MustCompile(`^(?:##|###)\s+(L[0-9][A-Za-z0-9-]*)(?:\s+\([^)]*\))?\s*(?:[—-]|:)\s*(.*)$`)
var legacyHeadingCandidate = regexp.MustCompile(`^(?:##|###)\s+L[0-9]`)
var legacyStatus = regexp.MustCompile(`(?m)^\*\*Status:\*\*\s*(.+?)\s*$`)
var legacyRecurrenceMarker = regexp.MustCompile(`(?i)^\*\*(?:recurred|recurrence|occurrence)\b[^*]*\*\*`)
var legacyRecurrenceCandidate = regexp.MustCompile(`(?i)^\*\*(?:recurred|recurrence|occurrence)\b`)

// legacyImportDeps is private and scoped to one inventory or application. It
// deliberately replaces the old mutable package hooks so parallel callers
// cannot alter one another's durability or provenance boundary.
type legacyImportDeps struct {
	fs             lessonFS
	abs            func(string) (string, error)
	sourceIdentity func(string, []byte) (LegacySourceRef, error)
	manifest       func(LegacyInventory, LegacyMapping) ([]byte, error)
	findOccurrence func(string, string, lessonFS) (Occurrence, error)
	afterPreflight func()
}

func defaultLegacyImportDeps() legacyImportDeps {
	return legacyImportDeps{
		fs:             osLessonFS{},
		abs:            filepath.Abs,
		sourceIdentity: resolveLegacySourceIdentity,
		manifest:       legacyManifestBytes,
		findOccurrence: findOccurrenceWithFS,
	}
}

func InventoryLegacy(source string) (LegacyInventory, error) {
	return inventoryLegacyWithDeps(source, defaultLegacyImportDeps())
}

func inventoryLegacyWithDeps(source string, deps legacyImportDeps) (LegacyInventory, error) {
	absSource, err := deps.abs(source)
	if err != nil {
		return LegacyInventory{}, err
	}
	b, err := deps.fs.ReadFile(absSource)
	if err != nil {
		return LegacyInventory{}, err
	}
	sum := sha256.Sum256(b)
	ref := LegacySourceRef{SHA256: hex.EncodeToString(sum[:]), ByteCount: len(b)}
	inv := LegacyInventory{Source: ref, localSource: absSource}
	if resolved, identityErr := deps.sourceIdentity(absSource, b); identityErr == nil {
		inv.Source = resolved
	} else {
		inv.Warnings = append(inv.Warnings, "immutable-source-identity-unavailable")
	}
	lines := strings.SplitAfter(string(b), "\n")
	type start struct {
		line      int
		offset    int
		id, title string
	}
	var starts []start
	offset := 0
	hasLessonsSection := false
	for _, line := range lines {
		if strings.TrimSpace(strings.TrimRight(line, "\r\n")) == "## Lessons" {
			hasLessonsSection = true
			break
		}
	}
	inLessons := !hasLessonsSection
	for i, line := range lines {
		plain := strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(plain) == "## Lessons" {
			inLessons = true
			offset += len(line)
			continue
		}
		if inLessons {
			if m := legacyHeading.FindStringSubmatch(plain); m != nil {
				starts = append(starts, start{i + 1, offset, m[1], strings.TrimSpace(m[2])})
			} else if legacyHeadingCandidate.MatchString(strings.TrimSpace(plain)) {
				inv.UnmatchedCandidates = append(inv.UnmatchedCandidates, LegacyCandidate{Kind: "lesson-heading", Line: i + 1, BytesSHA256: shaString([]byte(line))})
			}
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
		entry := LegacyEntry{Key: fmt.Sprintf("%s#%d", s.id, ordinal), Kind: "lesson", LegacyID: s.id, Ordinal: ordinal, Title: s.title, StartLine: s.line, EndLine: endLine, StartByte: s.offset, EndByte: endOffset, Raw: raw, BytesSHA256: shaString([]byte(raw)), SuggestedSlug: legacySlug(s.id, s.title)}
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
		inv.LessonCount++
		markers := inventoryRecurrenceMarkers(entry)
		inv.Entries = append(inv.Entries, markers...)
		inv.RecurrenceMarkerCount += len(markers)
	}
	for id, count := range counts {
		if count < 2 {
			continue
		}
		collision := LegacyCollision{LegacyID: id, Count: count}
		for ordinal := 1; ordinal <= count; ordinal++ {
			collision.Keys = append(collision.Keys, fmt.Sprintf("%s#%d", id, ordinal))
		}
		inv.Collisions = append(inv.Collisions, collision)
	}
	sort.Slice(inv.Collisions, func(i, j int) bool { return inv.Collisions[i].LegacyID < inv.Collisions[j].LegacyID })
	for i, line := range lines {
		plain := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if legacyRecurrenceCandidate.MatchString(plain) && !legacyRecurrenceMarker.MatchString(plain) {
			inv.UnmatchedCandidates = append(inv.UnmatchedCandidates, LegacyCandidate{Kind: "recurrence-marker", Line: i + 1, BytesSHA256: shaString([]byte(line))})
		}
	}
	inv.EntryProjectionSHA256 = legacyEntryProjectionSHA256(inv.Entries)
	return inv, nil
}

// legacyEntryProjectionSHA256 binds the ordered inventory identity without
// copying any legacy prose. It changes when a parser drops/reorders an entry,
// changes a key or parent, or shifts any exact source range/block hash.
func legacyEntryProjectionSHA256(entries []LegacyEntry) string {
	type projectionEntry struct {
		Key           string `json:"key"`
		Kind          string `json:"kind"`
		ParentKey     string `json:"parent_key,omitempty"`
		LegacyID      string `json:"legacy_id"`
		Ordinal       int    `json:"ordinal"`
		StartLine     int    `json:"start_line"`
		EndLine       int    `json:"end_line"`
		StartByte     int    `json:"start_byte"`
		EndByte       int    `json:"end_byte"`
		BytesSHA256   string `json:"bytes_sha256"`
		SuggestedSlug string `json:"suggested_slug,omitempty"`
	}
	projection := make([]projectionEntry, 0, len(entries))
	for _, entry := range entries {
		projection = append(projection, projectionEntry{
			Key: entry.Key, Kind: entry.Kind, ParentKey: entry.ParentKey,
			LegacyID: entry.LegacyID, Ordinal: entry.Ordinal,
			StartLine: entry.StartLine, EndLine: entry.EndLine,
			StartByte: entry.StartByte, EndByte: entry.EndByte,
			BytesSHA256: entry.BytesSHA256, SuggestedSlug: entry.SuggestedSlug,
		})
	}
	b := mustMarshalLegacyProjection(projection)
	return shaString(b)
}

func mustMarshalLegacyProjection(value any) []byte {
	b, err := json.Marshal(value)
	if err != nil {
		panic("fixed legacy inventory projection cannot fail JSON encoding: " + err.Error())
	}
	return b
}

func resolveLegacySourceIdentity(source string, workingBytes []byte) (LegacySourceRef, error) {
	return resolveLegacySourceIdentityWithDeps(source, workingBytes, defaultLegacySourceIdentityDeps())
}

type legacySourceIdentityDeps struct {
	evalSymlinks   func(string) (string, error)
	root           func(string) (string, error)
	rel            func(string, string) (string, error)
	repository     func(string) (string, error)
	revision       func(string) (string, error)
	committedBytes func(string, string, string) ([]byte, error)
	committedAt    func(string, string) (string, error)
}

func defaultLegacySourceIdentityDeps() legacySourceIdentityDeps {
	return legacySourceIdentityDeps{
		evalSymlinks: filepath.EvalSymlinks,
		root: func(dir string) (string, error) {
			b, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
			return strings.TrimSpace(string(b)), err
		},
		rel:        filepath.Rel,
		repository: legacyRepositoryIdentity,
		revision:   gitremote.HeadSHA,
		committedBytes: func(root, revision, rel string) ([]byte, error) {
			return exec.Command("git", "-C", root, "show", revision+":"+rel).Output()
		},
		committedAt: func(root, revision string) (string, error) {
			b, err := exec.Command("git", "-C", root, "show", "-s", "--format=%cI", revision).Output()
			return strings.TrimSpace(string(b)), err
		},
	}
}

func legacyRepositoryIdentity(root string) (string, error) {
	origin, err := gitremote.OriginURL(root)
	if err != nil {
		return "", fmt.Errorf("source repository has no origin")
	}
	remote, ok := gitremote.Parse(origin)
	if !ok {
		return "", fmt.Errorf("source repository origin is not a supported immutable identifier")
	}
	return remote.Host + "/" + remote.Owner + "/" + remote.Repo, nil
}

func resolveLegacySourceIdentityWithDeps(source string, workingBytes []byte, deps legacySourceIdentityDeps) (LegacySourceRef, error) {
	// Git reports the physical worktree root. Resolve a caller path first so
	// macOS's /var -> /private/var alias does not manufacture a false
	// repository-relative traversal.
	physicalSource, err := deps.evalSymlinks(source)
	if err != nil {
		return LegacySourceRef{}, fmt.Errorf("resolving physical source path: %w", err)
	}
	source = physicalSource
	dir := filepath.Dir(source)
	root, err := deps.root(dir)
	if err != nil {
		return LegacySourceRef{}, fmt.Errorf("source is not in a Git worktree")
	}
	rel, err := deps.rel(root, source)
	if err != nil {
		return LegacySourceRef{}, fmt.Errorf("resolving source path")
	}
	rel = filepath.ToSlash(rel)
	if err := validateRepoRelativePath(rel); err != nil {
		return LegacySourceRef{}, fmt.Errorf("source path is not portable")
	}
	repository, err := deps.repository(root)
	if err != nil {
		return LegacySourceRef{}, err
	}
	revision, err := deps.revision(root)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(revision) {
		return LegacySourceRef{}, fmt.Errorf("source revision is unavailable")
	}
	committedBytes, err := deps.committedBytes(root, revision, rel)
	if err != nil || shaString(committedBytes) != shaString(workingBytes) {
		return LegacySourceRef{}, fmt.Errorf("source bytes do not match the recorded revision")
	}
	committedAtText, err := deps.committedAt(root, revision)
	if err != nil {
		return LegacySourceRef{}, fmt.Errorf("source commit timestamp is unavailable")
	}
	committedAt, err := time.Parse(time.RFC3339, committedAtText)
	if err != nil {
		return LegacySourceRef{}, fmt.Errorf("source commit timestamp is invalid")
	}
	return LegacySourceRef{
		Repository:  repository,
		Path:        rel,
		Revision:    revision,
		CommittedAt: committedAt.UTC().Format(time.RFC3339),
		SHA256:      shaString(workingBytes),
		ByteCount:   len(workingBytes),
	}, nil
}

func validateLegacySourceRef(ref LegacySourceRef) error {
	if !repositoryID.MatchString(ref.Repository) {
		return fmt.Errorf("legacy source repository must be host/org/repo")
	}
	if err := validateRepoRelativePath(ref.Path); err != nil {
		return fmt.Errorf("legacy source path is invalid")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(ref.Revision) {
		return fmt.Errorf("legacy source revision must be a full Git commit SHA")
	}
	at, err := time.Parse(time.RFC3339, ref.CommittedAt)
	if err != nil || at.Location() != time.UTC || !strings.HasSuffix(ref.CommittedAt, "Z") {
		return fmt.Errorf("legacy source committed_at must be UTC RFC3339")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(ref.SHA256) || ref.ByteCount < 1 {
		return fmt.Errorf("legacy source byte identity is invalid")
	}
	for field, value := range map[string]string{"source repository": ref.Repository, "source path": ref.Path, "source revision": ref.Revision, "source committed_at": ref.CommittedAt} {
		if err := ValidateSafeContent(field, value); err != nil {
			return err
		}
	}
	return nil
}

func inventoryRecurrenceMarkers(parent LegacyEntry) []LegacyEntry {
	lines := strings.SplitAfter(parent.Raw, "\n")
	offsets := make([]int, len(lines)+1)
	for i := range lines {
		offsets[i+1] = offsets[i] + len(lines[i])
	}
	var out []LegacyEntry
	for i := 0; i < len(lines); i++ {
		plain := strings.TrimRight(lines[i], "\r\n")
		if !legacyRecurrenceMarker.MatchString(strings.TrimSpace(plain)) {
			continue
		}
		j := i + 1
		for ; j < len(lines); j++ {
			if strings.TrimSpace(strings.TrimRight(lines[j], "\r\n")) == "" {
				break
			}
		}
		start, end := offsets[i], offsets[j]
		endLine := parent.StartLine + j - 1
		raw := parent.Raw[start:end]
		ordinal := len(out) + 1
		warnings := []string{"requires-human-occurrence-decision"}
		lower := strings.ToLower(raw)
		if strings.Contains(lower, "twice") || strings.Contains(lower, "three times") || strings.Contains(lower, "four times") || strings.Contains(lower, "at least") {
			warnings = append(warnings, "aggregate-count-ambiguous")
		}
		out = append(out, LegacyEntry{
			Key:         fmt.Sprintf("%s/recurrence#%d", parent.Key, ordinal),
			Kind:        "recurrence-marker",
			ParentKey:   parent.Key,
			LegacyID:    parent.LegacyID,
			Ordinal:     ordinal,
			Title:       strings.TrimSpace(plain),
			StartLine:   parent.StartLine + i,
			EndLine:     endLine,
			StartByte:   parent.StartByte + start,
			EndByte:     parent.StartByte + end,
			Raw:         raw,
			BytesSHA256: shaString([]byte(raw)),
			Warnings:    warnings,
		})
		i = j - 1
	}
	return out
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

type legacyApplyPlan struct {
	byKey          map[string]LegacyEntry
	byMap          map[string]LegacyMappingEntry
	keys           []string
	newTargets     map[string]string
	manifestPath   string
	manifestBytes  []byte
	manifestExists bool
}

// PreflightLegacyApply validates the complete reviewed mapping, configured
// classification vocabulary, immutable source bytes, targets, and manifest
// collisions without writing. CLI adapters call it before preparing an event;
// ApplyLegacy calls the same function again immediately before publication.
func PreflightLegacyApply(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping) error {
	_, err := preflightLegacyApply(lessonsDir, allowedClassifications, inv, mapping)
	return err
}

func preflightLegacyApply(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping) (legacyApplyPlan, error) {
	return preflightLegacyApplyWithFS(lessonsDir, allowedClassifications, inv, mapping, osLessonFS{})
}

func preflightLegacyApplyWithFS(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping, fs lessonFS) (legacyApplyPlan, error) {
	return preflightLegacyApplyWithRuntime(lessonsDir, allowedClassifications, inv, mapping, fs, legacyManifestBytes)
}

func preflightLegacyApplyWithRuntime(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping, fs lessonFS, manifest func(LegacyInventory, LegacyMapping) ([]byte, error)) (legacyApplyPlan, error) {
	plan := legacyApplyPlan{}
	if err := validateLegacySourceRef(inv.Source); err != nil {
		return plan, fmt.Errorf("immutable legacy source identity required before apply: %w", err)
	}
	if inv.Source != mapping.Source {
		return plan, fmt.Errorf("mapping source identity does not match the inventoried immutable source")
	}
	if len(allowedClassifications) == 0 {
		return plan, fmt.Errorf("legacy apply requires a non-empty target lessons.classifications vocabulary")
	}
	allowed := map[string]bool{}
	for _, classification := range allowedClassifications {
		if err := ValidateSlug(classification); err != nil || allowed[classification] {
			return plan, fmt.Errorf("target lessons.classifications contains an invalid or duplicate value")
		}
		allowed[classification] = true
	}
	plan.byKey = map[string]LegacyEntry{}
	for _, entry := range inv.Entries {
		if _, duplicate := plan.byKey[entry.Key]; duplicate {
			return plan, fmt.Errorf("inventory repeats key %q", entry.Key)
		}
		plan.byKey[entry.Key] = entry
	}
	plan.byMap = map[string]LegacyMappingEntry{}
	for _, mappingEntry := range mapping.Entries {
		if _, exists := plan.byKey[mappingEntry.Key]; !exists {
			return plan, fmt.Errorf("mapping references unknown key %q", mappingEntry.Key)
		}
		if _, duplicate := plan.byMap[mappingEntry.Key]; duplicate {
			return plan, fmt.Errorf("mapping repeats key %q", mappingEntry.Key)
		}
		plan.byMap[mappingEntry.Key] = mappingEntry
		plan.keys = append(plan.keys, mappingEntry.Key)
	}
	sort.Strings(plan.keys)
	plan.newTargets = map[string]string{}
	for _, key := range plan.keys {
		mappingEntry := plan.byMap[key]
		if mappingEntry.Action == "new" {
			if previous, exists := plan.newTargets[mappingEntry.Slug]; exists && previous != key {
				return plan, fmt.Errorf("mappings %s and %s target the same new Lesson %q", previous, key, mappingEntry.Slug)
			}
			plan.newTargets[mappingEntry.Slug] = key
		}
	}
	for _, entry := range inv.Entries {
		if _, ok := plan.byMap[entry.Key]; !ok {
			return plan, fmt.Errorf("mapping lacks explicit disposition for %s", entry.Key)
		}
	}
	var unresolvedManual []string
	for _, key := range plan.keys {
		mappingEntry := plan.byMap[key]
		entry := plan.byKey[key]
		for field, value := range map[string]string{
			"mapping key": mappingEntry.Key, "mapping action": mappingEntry.Action,
			"mapping slug": mappingEntry.Slug, "mapping title": mappingEntry.Title,
			"mapping Lesson": mappingEntry.Lesson, "mapping Process Gap": mappingEntry.ProcessGap,
			"mapping status": mappingEntry.Status,
		} {
			if value != "" {
				if err := ValidateSafeContent(field, value); err != nil {
					return plan, err
				}
			}
		}
		switch mappingEntry.Action {
		case "manual":
			if mappingEntry.Slug != "" || mappingEntry.Title != "" || mappingEntry.Lesson != "" || mappingEntry.ProcessGap != "" || mappingEntry.Status != "" || len(mappingEntry.Classifications) != 0 {
				return plan, fmt.Errorf("mapping %s manual decision contains ignored disposition fields", key)
			}
			unresolvedManual = append(unresolvedManual, key)
		case "new":
			if entry.Kind != "lesson" {
				return plan, fmt.Errorf("mapping %s: recurrence markers cannot create Lessons; choose occurrence or manual", key)
			}
			if err := ValidateSlug(mappingEntry.Slug); err != nil {
				return plan, fmt.Errorf("mapping %s: %w", key, err)
			}
			effectiveTitle := strings.TrimSpace(mappingEntry.Title)
			if effectiveTitle == "" {
				effectiveTitle = strings.TrimSpace(entry.Title)
			}
			if effectiveTitle == "" {
				return plan, fmt.Errorf("mapping %s requires a reviewed title because the source heading title is empty", key)
			}
			if err := ValidateSafeContent("reviewed legacy Lesson title", effectiveTitle); err != nil {
				return plan, fmt.Errorf("mapping %s requires an explicit safe title: %w", key, err)
			}
			if err := validateReviewedCompactText("Lesson", mappingEntry.Lesson); err != nil {
				return plan, fmt.Errorf("mapping %s: %w", key, err)
			}
			if err := validateReviewedCompactText("Process Gap", mappingEntry.ProcessGap); err != nil {
				return plan, fmt.Errorf("mapping %s: %w", key, err)
			}
			if mappingEntry.Status != "Recorded" {
				return plan, fmt.Errorf("mapping %s must explicitly import status Recorded; Enforced requires separately reviewed canonical enforcement evidence", key)
			}
			if len(mappingEntry.Classifications) == 0 {
				return plan, fmt.Errorf("mapping %s requires at least one configured classification", key)
			}
			seenClassifications := map[string]bool{}
			for _, classification := range mappingEntry.Classifications {
				if err := ValidateSlug(classification); err != nil || seenClassifications[classification] || !allowed[classification] {
					return plan, fmt.Errorf("mapping %s has an invalid, duplicate, or unconfigured classification", key)
				}
				seenClassifications[classification] = true
			}
			if _, err := fs.Lstat(filepath.Join(lessonsDir, mappingEntry.Slug+".md")); err == nil {
				return plan, fmt.Errorf("mapping %s collides with legacy flat Lesson %q; migrate it first", key, mappingEntry.Slug)
			} else if !os.IsNotExist(err) {
				return plan, err
			}
			targetDir := filepath.Join(lessonsDir, mappingEntry.Slug)
			target := filepath.Join(targetDir, "README.md")
			if _, err := fs.Stat(target); err == nil {
				if err := validateImportedLessonWithFS(target, entry, inv, fs); err != nil {
					return plan, fmt.Errorf("mapping %s target collision: %w", key, err)
				}
			} else if !os.IsNotExist(err) {
				return plan, fmt.Errorf("mapping %s target stat: %w", key, err)
			} else if _, dirErr := fs.Stat(targetDir); dirErr == nil {
				return plan, fmt.Errorf("mapping %s target directory already exists without matching imported provenance", key)
			} else if !os.IsNotExist(dirErr) {
				return plan, fmt.Errorf("mapping %s target directory stat: %w", key, dirErr)
			}
		case "occurrence":
			if mappingEntry.Title != "" || mappingEntry.Lesson != "" || mappingEntry.ProcessGap != "" || mappingEntry.Status != "" || len(mappingEntry.Classifications) != 0 {
				return plan, fmt.Errorf("mapping %s occurrence contains ignored new-Lesson fields", key)
			}
			if err := ValidateSlug(mappingEntry.Slug); err != nil {
				return plan, fmt.Errorf("mapping %s: %w", key, err)
			}
			if _, createdThisRun := plan.newTargets[mappingEntry.Slug]; !createdThisRun {
				path, err := ResolveLessonFile(lessonsDir, mappingEntry.Slug)
				if err != nil {
					return plan, fmt.Errorf("mapping %s occurrence target: %w", key, err)
				}
				parsed, err := Parse(path)
				if err != nil || !parsed.Canonical {
					return plan, fmt.Errorf("mapping %s occurrence target must be a canonical Lesson", key)
				}
			}
		default:
			return plan, fmt.Errorf("mapping %s has invalid action %q (new, occurrence, manual)", key, mappingEntry.Action)
		}
	}
	if len(unresolvedManual) > 0 {
		return plan, fmt.Errorf("mapping retains %d manual decision(s); resolve every row before --apply", len(unresolvedManual))
	}
	if inv.localSource == "" {
		return plan, fmt.Errorf("legacy inventory must be rebuilt from the local immutable source before apply")
	}
	source, err := fs.ReadFile(inv.localSource)
	if err != nil {
		return plan, err
	}
	if shaString(source) != inv.Source.SHA256 || len(source) != inv.Source.ByteCount {
		return plan, fmt.Errorf("legacy source changed after inventory")
	}
	plan.manifestPath = filepath.Join(lessonsDir, ".legacy-import", inv.Source.SHA256+".json")
	plan.manifestBytes, err = manifest(inv, mapping)
	if err != nil {
		return plan, err
	}
	if existing, readErr := fs.ReadFile(plan.manifestPath); readErr == nil {
		plan.manifestExists = true
		if !bytes.Equal(existing, plan.manifestBytes) {
			return plan, fmt.Errorf("import manifest collision: existing bytes differ")
		}
	} else if !os.IsNotExist(readErr) {
		return plan, readErr
	}
	if info, statErr := fs.Stat(filepath.Dir(plan.manifestPath)); statErr == nil && !info.IsDir() {
		return plan, fmt.Errorf("legacy manifest parent is not a directory")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return plan, statErr
	}
	return plan, nil
}

func ApplyLegacy(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping) (LegacyApplyResult, error) {
	return applyLegacyWithDeps(lessonsDir, allowedClassifications, inv, mapping, defaultLegacyImportDeps())
}

type legacyPublishedArtifact struct {
	path      string
	directory bool
	content   []byte
}

// legacyPublishedSet is the importer's exact rollback ownership claim. Files
// carry their published bytes and owned directories carry their complete
// immediate child set. Rollback verifies the entire set before deleting a
// single path, then uses non-recursive Remove calls so an unobserved foreign
// child can never be recursively erased.
type legacyPublishedSet struct {
	artifacts []legacyPublishedArtifact
}

func (p *legacyPublishedSet) addDirectory(path string) {
	p.artifacts = append(p.artifacts, legacyPublishedArtifact{path: path, directory: true})
}

func (p *legacyPublishedSet) addFile(path string, content []byte) {
	p.artifacts = append(p.artifacts, legacyPublishedArtifact{path: path, content: append([]byte(nil), content...)})
}

func (p *legacyPublishedSet) verify(fs lessonFS) error {
	expectedChildren := make(map[string]map[string]bool)
	for _, artifact := range p.artifacts {
		if artifact.directory {
			expectedChildren[artifact.path] = map[string]bool{}
		}
	}
	for _, artifact := range p.artifacts {
		if children, ownedParent := expectedChildren[filepath.Dir(artifact.path)]; ownedParent {
			children[filepath.Base(artifact.path)] = artifact.directory
		}
	}
	for _, artifact := range p.artifacts {
		info, err := fs.Lstat(artifact.path)
		if err != nil {
			return fmt.Errorf("verifying rollback ownership of %s: %w", artifact.path, err)
		}
		if artifact.directory {
			if !info.IsDir() {
				return fmt.Errorf("rollback-owned directory changed type: %s", artifact.path)
			}
			entries, err := fs.ReadDir(artifact.path)
			if err != nil {
				return fmt.Errorf("reading rollback-owned directory %s: %w", artifact.path, err)
			}
			want := expectedChildren[artifact.path]
			if len(entries) != len(want) {
				return fmt.Errorf("rollback-owned directory changed after publication: %s", artifact.path)
			}
			for _, entry := range entries {
				wantDirectory, ok := want[entry.Name()]
				if !ok || entry.IsDir() != wantDirectory {
					return fmt.Errorf("rollback-owned directory changed after publication: %s", artifact.path)
				}
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("rollback-owned file changed type: %s", artifact.path)
		}
		got, err := fs.ReadFile(artifact.path)
		if err != nil {
			return fmt.Errorf("reading rollback-owned file %s: %w", artifact.path, err)
		}
		if !bytes.Equal(got, artifact.content) {
			return fmt.Errorf("rollback-owned file changed after publication: %s", artifact.path)
		}
	}
	return nil
}

func (p *legacyPublishedSet) rollback(fs lessonFS) error {
	if err := p.verify(fs); err != nil {
		return fmt.Errorf("refusing rollback because publication ownership is uncertain: %w", err)
	}
	for i := len(p.artifacts) - 1; i >= 0; i-- {
		path := p.artifacts[i].path
		if err := fs.Remove(path); err != nil {
			return fmt.Errorf("removing rollback-owned path %s: %w", path, err)
		}
		if err := syncDirectoryWithFS(filepath.Dir(path), fs); err != nil {
			return fmt.Errorf("durably removing rollback-owned path %s: %w", path, err)
		}
	}
	return nil
}

func applyLegacyWithDeps(lessonsDir string, allowedClassifications []string, inv LegacyInventory, mapping LegacyMapping, deps legacyImportDeps) (LegacyApplyResult, error) {
	plan, err := preflightLegacyApplyWithRuntime(lessonsDir, allowedClassifications, inv, mapping, deps.fs, deps.manifest)
	if err != nil {
		return LegacyApplyResult{}, err
	}
	if deps.afterPreflight != nil {
		deps.afterPreflight()
	}
	byKey, byMap, keys, newTargets := plan.byKey, plan.byMap, plan.keys, plan.newTargets
	result := LegacyApplyResult{}
	manifestPath, manifestBytes, manifestExists := plan.manifestPath, plan.manifestBytes, plan.manifestExists
	stageDir, err := deps.fs.MkdirTemp(lessonsDir, ".legacy-import-stage-")
	if err != nil {
		return LegacyApplyResult{}, err
	}
	defer func() { _ = deps.fs.RemoveAll(stageDir) }()
	if !manifestExists {
		if err := writeDurableStageFileWithFS(filepath.Join(stageDir, "manifest.json"), manifestBytes, deps.fs); err != nil {
			return result, err
		}
	}
	result.Manifest = manifestPath
	lessonAlready := map[string]bool{}
	occurrenceAlready := map[string]bool{}
	for _, key := range keys {
		e, m := byKey[key], byMap[key]
		switch m.Action {
		case "new":
			target := filepath.Join(lessonsDir, m.Slug, "README.md")
			if _, err := deps.fs.Stat(target); err == nil {
				lessonAlready[key] = true
			} else if os.IsNotExist(err) {
				title := strings.TrimSpace(m.Title)
				if title == "" {
					title = strings.TrimSpace(e.Title)
				}
				if err := writeImportedLessonWithFS(filepath.Join(stageDir, m.Slug, "README.md"), m.Slug, title, m, e, inv, deps.fs); err != nil {
					return result, err
				}
			} else {
				return result, err
			}
			if lessonAlready[key] {
				id := legacyOccurrenceID(inv.Source.SHA256, e.Key, m.Slug)
				if existing, err := deps.findOccurrence(target, id, deps.fs); err == nil {
					if err := validateLegacyOccurrence(existing, inv, e, m.Slug); err != nil {
						return result, fmt.Errorf("mapping %s provider occurrence collision: %w", key, err)
					}
					occurrenceAlready[key] = true
				} else if !os.IsNotExist(err) {
					return result, err
				}
			}
		case "occurrence":
			id := legacyOccurrenceID(inv.Source.SHA256, e.Key, m.Slug)
			path, resolveErr := ResolveLessonFile(lessonsDir, m.Slug)
			if resolveErr != nil {
				if _, createdThisRun := newTargets[m.Slug]; createdThisRun {
					continue
				}
				return result, resolveErr
			}
			if existing, err := deps.findOccurrence(path, id, deps.fs); err == nil {
				if err := validateLegacyOccurrence(existing, inv, e, m.Slug); err != nil {
					return result, fmt.Errorf("mapping %s occurrence collision: %w", key, err)
				}
				occurrenceAlready[key] = true
				continue
			} else if !os.IsNotExist(err) {
				return result, err
			}
		}
	}

	var published legacyPublishedSet
	manifestDirExisted, err := statDirectoryWithFS(filepath.Dir(manifestPath), deps.fs)
	if err != nil {
		return result, err
	}
	rollback := func() error {
		return published.rollback(deps.fs)
	}
	failAfterPublication := func(cause error) (LegacyApplyResult, error) {
		rollbackErr := rollback()
		// A nested occurrence writer may already have crossed a publication or
		// fsync boundary. Rolling back sibling files cannot prove that child is
		// gone, so its prepared event must remain recoverable even when the
		// outer rollback itself succeeds.
		if MutationOutcomeOf(cause) == MutationUncertain {
			if rollbackErr != nil {
				return LegacyApplyResult{}, mutationFailure(MutationUncertain, fmt.Errorf("%v; outer rollback also failed: %w", cause, rollbackErr))
			}
			return LegacyApplyResult{}, mutationFailure(MutationUncertain, cause)
		}
		if rollbackErr != nil {
			return LegacyApplyResult{}, mutationFailure(MutationUncertain, fmt.Errorf("%v; rollback failed: %w", cause, rollbackErr))
		}
		return LegacyApplyResult{}, mutationFailure(MutationCompensated, cause)
	}
	for _, key := range keys {
		m := byMap[key]
		if m.Action != "new" {
			continue
		}
		if lessonAlready[key] {
			continue
		}
		finalDir := filepath.Join(lessonsDir, m.Slug)
		readmeBytes, err := deps.fs.ReadFile(filepath.Join(stageDir, m.Slug, "README.md"))
		if err != nil {
			return failAfterPublication(err)
		}
		if err := publishStagedLessonWithFS(filepath.Join(stageDir, m.Slug), finalDir, deps.fs); err != nil {
			return failAfterPublication(err)
		}
		published.addDirectory(finalDir)
		published.addDirectory(filepath.Join(finalDir, "occurrences"))
		published.addFile(filepath.Join(finalDir, "README.md"), readmeBytes)
		result.CreatedLessons = append(result.CreatedLessons, m.Slug)
	}
	if !manifestExists {
		if !manifestDirExisted {
			if err := deps.fs.Mkdir(filepath.Dir(manifestPath), 0o755); err != nil {
				return failAfterPublication(err)
			}
			published.addDirectory(filepath.Dir(manifestPath))
		}
		if err := deps.fs.Link(filepath.Join(stageDir, "manifest.json"), manifestPath); err != nil {
			return failAfterPublication(err)
		}
		// Ownership begins when the exclusive link succeeds, not after fsync.
		// Recording it first guarantees a post-link durability failure removes it.
		published.addFile(manifestPath, manifestBytes)
		if err := syncDirectoryWithFS(filepath.Dir(manifestPath), deps.fs); err != nil {
			return failAfterPublication(err)
		}
	}
	for _, key := range keys {
		e, m := byKey[key], byMap[key]
		if (m.Action != "occurrence" && m.Action != "new") || occurrenceAlready[key] {
			continue
		}
		path, _ := ResolveLessonFile(lessonsDir, m.Slug)
		id := legacyOccurrenceID(inv.Source.SHA256, e.Key, m.Slug)
		o, err := addOccurrenceWithFS(legacyOccurrenceOptions(path, id, inv, e), deps.fs)
		if err != nil {
			return failAfterPublication(err)
		}
		occurrenceBytes, err := deps.fs.ReadFile(o.Path)
		if err != nil {
			// The immutable child may already be visible, but without its exact
			// bytes this importer cannot claim rollback ownership. Preserve it
			// and classify the transaction for durable retry.
			return failAfterPublication(mutationFailure(MutationUncertain, fmt.Errorf("snapshotting published occurrence: %w", err)))
		}
		published.addFile(o.Path, occurrenceBytes)
		result.CreatedOccurrences = append(result.CreatedOccurrences, o.ID)
	}
	for _, key := range keys {
		m := byMap[key]
		if (m.Action == "occurrence" && occurrenceAlready[key]) || (m.Action == "new" && lessonAlready[key] && occurrenceAlready[key]) {
			result.Skipped = append(result.Skipped, key)
		}
	}
	for _, key := range keys {
		m := byMap[key]
		if m.Action != "new" {
			continue
		}
		result.StatusDecisions = append(result.StatusDecisions, LegacyStatusDecision{
			Key: key, SourceStatus: byKey[key].RawStatus, ImportedStatus: "Recorded",
			Reason: "canonical enforcement was not explicitly reviewed as control, verification, and evidence",
		})
	}
	return result, nil
}

func validateReviewedCompactText(section, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("reviewed %s content is required", section)
	}
	if utf8.RuneCountInString(value) > 2000 {
		return fmt.Errorf("reviewed %s content exceeds 2000 Unicode code points", section)
	}
	lower := strings.ToLower(value)
	if strings.Contains(lower, "<!--") || strings.Contains(lower, "todo:") {
		return fmt.Errorf("reviewed %s content must not contain a placeholder", section)
	}
	for _, line := range strings.Split(value, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			return fmt.Errorf("reviewed %s content must not inject a Markdown heading", section)
		}
	}
	if err := ValidateSafeContent("reviewed "+section, value); err != nil {
		return err
	}
	return nil
}

func stringPtr(s string) *string { return &s }
func pathIsDirectory(path string) bool {
	isDir, _ := statDirectoryWithFS(path, osLessonFS{})
	return isDir
}

func statDirectoryWithFS(path string, fs lessonFS) (bool, error) {
	info, err := fs.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// publishStagedLesson reserves the target directory with mkdir's exclusive
// create, then atomically links the fully-written staged README into it. A
// racing importer cannot replace another writer's Lesson directory.
func publishStagedLesson(stageDir, finalDir string) error {
	return publishStagedLessonWithFS(stageDir, finalDir, osLessonFS{})
}

func publishStagedLessonWithFS(stageDir, finalDir string, fs lessonFS) error {
	readmeBytes, err := fs.ReadFile(filepath.Join(stageDir, "README.md"))
	if err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	var published legacyPublishedSet
	if err := fs.Mkdir(finalDir, 0o755); err != nil {
		return mutationFailure(MutationPrePublication, err)
	}
	published.addDirectory(finalDir)
	rollback := func(cause error) error {
		if rollbackErr := published.rollback(fs); rollbackErr != nil {
			return mutationFailure(MutationUncertain, fmt.Errorf("%v; staged Lesson rollback failed: %w", cause, rollbackErr))
		}
		return mutationFailure(MutationCompensated, cause)
	}
	if err := fs.Mkdir(filepath.Join(finalDir, "occurrences"), 0o755); err != nil {
		return rollback(err)
	}
	published.addDirectory(filepath.Join(finalDir, "occurrences"))
	if err := fs.Link(filepath.Join(stageDir, "README.md"), filepath.Join(finalDir, "README.md")); err != nil {
		return rollback(err)
	}
	published.addFile(filepath.Join(finalDir, "README.md"), readmeBytes)
	if err := syncDirectoryWithFS(finalDir, fs); err != nil {
		return rollback(err)
	}
	if err := syncDirectoryWithFS(filepath.Dir(finalDir), fs); err != nil {
		return rollback(err)
	}
	return nil
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return f.Sync()
}

func legacyOccurrenceOptions(path, id string, inv LegacyInventory, entry LegacyEntry) AddOccurrenceOptions {
	base, _ := time.Parse(time.RFC3339, inv.Source.CommittedAt)
	occurredAt := base.UTC().Add(time.Duration(entry.StartByte) * time.Nanosecond)
	redactions := []string{"legacy-unstructured-context"}
	if containsString(entry.Warnings, "aggregate-count-ambiguous") {
		redactions = append(redactions, "aggregate-count-ambiguous")
	}
	return AddOccurrenceOptions{
		LessonPath: path,
		ID:         id,
		Summary:    "Imported legacy occurrence " + entry.Key,
		Context:    map[string]any{"execution": map[string]any{"kind": "unknown", "id": "legacy-import:" + inv.Source.SHA256 + "#" + entry.Key}},
		Evidence:   Evidence{Kind: "path", Ref: stringPtr("spec/lessons/.legacy-import/" + inv.Source.SHA256 + ".json")},
		Redactions: redactions,
		Now:        occurredAt,
	}
}

func validateLegacyOccurrence(o Occurrence, inv LegacyInventory, entry LegacyEntry, _ string) error {
	want := legacyOccurrenceOptions("", o.ID, inv, entry)
	if !o.OccurredAt.Equal(want.Now) || o.Summary != want.Summary || o.Evidence.Kind != want.Evidence.Kind || o.Evidence.Ref == nil || *o.Evidence.Ref != *want.Evidence.Ref || strings.Join(o.Redactions, "\x00") != strings.Join(want.Redactions, "\x00") {
		return fmt.Errorf("existing deterministic occurrence has different provenance")
	}
	execution, ok := o.Context["execution"].(map[string]any)
	if !ok || execution["kind"] != "unknown" || execution["id"] != "legacy-import:"+inv.Source.SHA256+"#"+entry.Key {
		return fmt.Errorf("existing deterministic occurrence has different provenance")
	}
	return nil
}
func hasLegacyProvenance(path, provenance string) bool {
	b, err := os.ReadFile(path)
	return err == nil && strings.Contains(string(b), "**Legacy Provenance:** "+provenance)
}
func validateImportedLesson(path string, e LegacyEntry, inv LegacyInventory) error {
	return validateImportedLessonWithFS(path, e, inv, osLessonFS{})
}

func validateImportedLessonWithFS(path string, e LegacyEntry, inv LegacyInventory, fs lessonFS) error {
	lessonBytes, err := fs.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Lesson unreadable")
	}
	if err := ValidateSafeContent("imported Lesson", string(lessonBytes)); err != nil {
		return err
	}
	if !strings.Contains(string(lessonBytes), "**Legacy Provenance:** "+legacyProvenance(inv, e)) {
		return fmt.Errorf("provenance differs")
	}
	if info, err := fs.Stat(filepath.Join(filepath.Dir(path), "occurrences")); err != nil || !info.IsDir() {
		return fmt.Errorf("occurrence store missing")
	}
	return nil
}
func legacyOccurrenceID(sourceHash, key, slug string) string {
	sum := sha256.Sum256([]byte(sourceHash + "\x00" + key + "\x00" + slug))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func writeImportedLesson(target, slug, title string, mapping LegacyMappingEntry, e LegacyEntry, inv LegacyInventory) error {
	return writeImportedLessonWithFS(target, slug, title, mapping, e, inv, osLessonFS{})
}

func writeImportedLessonWithFS(target, slug, title string, mapping LegacyMappingEntry, e LegacyEntry, inv LegacyInventory, fs lessonFS) error {
	if err := fs.MkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
		return err
	}
	provenance := legacyProvenance(inv, e)
	committedAt, _ := time.Parse(time.RFC3339, inv.Source.CommittedAt)
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: slug, Title: title, Owner: "legacy-import", Date: committedAt.UTC().Format("2006-01-02")}, mapping.Classifications)
	if err != nil {
		return err
	}
	s := strings.Replace(string(body), "**Legacy Provenance:** —", "**Legacy Provenance:** "+provenance, 1)
	s = replaceCanonicalSection(s, "Lesson", strings.TrimSpace(mapping.Lesson))
	s = replaceCanonicalSection(s, "Process Gap", strings.TrimSpace(mapping.ProcessGap))
	if strings.Contains(s, "<!-- TODO:") {
		return fmt.Errorf("reviewed legacy mapping left a placeholder in the compact Lesson")
	}
	if err := ValidateSafeContent("imported Lesson", s); err != nil {
		return err
	}
	return writeDurableStageFileWithFS(target, []byte(s), fs)
}

func legacyProvenance(inv LegacyInventory, e LegacyEntry) string {
	return fmt.Sprintf("%s@%s:%s#bytes=%d-%d;sha256=%s", inv.Source.Repository, inv.Source.Revision, inv.Source.Path, e.StartByte, e.EndByte, e.BytesSHA256)
}

func writeDurableStageFile(path string, data []byte) error {
	return writeDurableStageFileWithFS(path, data, osLessonFS{})
}

func writeDurableStageFileWithFS(path string, data []byte, fs lessonFS) error {
	f, err := fs.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

type persistedLegacyEntry struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	ParentKey   string `json:"parent_key,omitempty"`
	LegacyID    string `json:"legacy_id"`
	Ordinal     int    `json:"ordinal"`
	StartLine   int    `json:"start_line"`
	EndLine     int    `json:"end_line"`
	StartByte   int    `json:"start_byte"`
	EndByte     int    `json:"end_byte"`
	RawStatus   string `json:"raw_status,omitempty"`
	BytesSHA256 string `json:"bytes_sha256"`
}

func legacyManifestBytes(inv LegacyInventory, mapping LegacyMapping) ([]byte, error) {
	entries := make([]persistedLegacyEntry, 0, len(inv.Entries))
	for _, e := range inv.Entries {
		entries = append(entries, persistedLegacyEntry{Key: e.Key, Kind: e.Kind, ParentKey: e.ParentKey, LegacyID: e.LegacyID, Ordinal: e.Ordinal, StartLine: e.StartLine, EndLine: e.EndLine, StartByte: e.StartByte, EndByte: e.EndByte, RawStatus: e.RawStatus, BytesSHA256: e.BytesSHA256})
	}
	payload := struct {
		SchemaVersion         int                    `json:"schema_version"`
		Source                LegacySourceRef        `json:"source"`
		LessonCount           int                    `json:"lesson_count"`
		RecurrenceMarkerCount int                    `json:"recurrence_marker_count"`
		EntryProjectionSHA256 string                 `json:"entry_projection_sha256"`
		Entries               []persistedLegacyEntry `json:"entries"`
		Mapping               LegacyMapping          `json:"mapping"`
		EntryCount            int                    `json:"entry_count"`
		MappingCount          int                    `json:"mapping_count"`
	}{1, inv.Source, inv.LessonCount, inv.RecurrenceMarkerCount, inv.EntryProjectionSHA256, entries, mapping, len(entries), len(mapping.Entries)}
	return marshalLegacyManifest(payload)
}

func marshalLegacyManifest(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	b = append(b, '\n')
	if err := ValidateSafeContent("legacy import manifest", string(b)); err != nil {
		return nil, err
	}
	return b, nil
}
