// Package trace exposes the versioned, JSON-friendly SpecScore records used by
// code-intelligence providers. It owns document meaning; symbol discovery and
// attachment remain provider responsibilities.
package trace

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// ContractVersion identifies the JSON contract. It belongs to the SpecScore
// trace provider contract and is intentionally explicit in every snapshot.
const ContractVersion = "specscore.trace/v1"

// SourceRange is a 1-based, inclusive line and column range in one source
// file. EndColumn is exclusive on EndLine, matching common parser ranges.
type SourceRange struct {
	Path        string `json:"path"`
	StartLine   int    `json:"start_line"`
	StartColumn int    `json:"start_column,omitempty"`
	EndLine     int    `json:"end_line"`
	EndColumn   int    `json:"end_column,omitempty"`
}

// FeatureRecord is a normalized Feature and its inline requirements and ACs.
type FeatureRecord struct {
	ID                 string                      `json:"id"`
	Path               string                      `json:"path"`
	Title              string                      `json:"title,omitempty"`
	Status             string                      `json:"status,omitempty"`
	Source             SourceRange                 `json:"source"`
	Requirements       []RequirementRecord         `json:"requirements,omitempty"`
	AcceptanceCriteria []AcceptanceCriterionRecord `json:"acceptance_criteria,omitempty"`
	Scenarios          []ScenarioRecord            `json:"scenarios,omitempty"`
}

// Short aliases keep the public contract ergonomic for callers that already
// use the record names as domain types.
type Feature = FeatureRecord

// RequirementRecord is a stable, addressable REQ inside a Feature.
type RequirementRecord struct {
	ID        string      `json:"id"`
	FeatureID string      `json:"feature_id"`
	Slug      string      `json:"slug"`
	Title     string      `json:"title,omitempty"`
	Source    SourceRange `json:"source"`
}

type Requirement = RequirementRecord

// AcceptanceCriterionRecord is a stable, addressable AC inside a Feature.
type AcceptanceCriterionRecord struct {
	ID           string      `json:"id"`
	FeatureID    string      `json:"feature_id"`
	Slug         string      `json:"slug"`
	Title        string      `json:"title,omitempty"`
	Requirements []string    `json:"requirements,omitempty"`
	Source       SourceRange `json:"source"`
}

type AcceptanceCriterion = AcceptanceCriterionRecord

// ScenarioRecord is a standalone executable scenario under a Feature's
// _tests directory. Whether its file contains an executable Rehearse block is
// intentionally left for the scenario runner/provider to determine.
type ScenarioRecord struct {
	ID        string      `json:"id"`
	FeatureID string      `json:"feature_id"`
	Slug      string      `json:"slug"`
	Title     string      `json:"title,omitempty"`
	Validates []string    `json:"validates,omitempty"`
	Source    SourceRange `json:"source"`
}

type Scenario = ScenarioRecord

// Snapshot is the top-level JSON exchange shape for a repository or feature.
type Snapshot struct {
	Version  string          `json:"version"`
	Features []FeatureRecord `json:"features"`
}

type Document = Snapshot

var (
	reqHeadingRe    = regexp.MustCompile(`^(#{4,})\s+REQ:\s*([a-z0-9]+(?:-[a-z0-9]+)*)\s*$`)
	acHeadingRe     = regexp.MustCompile(`^(###)\s+AC:\s*([a-z0-9]+(?:-[a-z0-9]+)*)\s*$`)
	scenarioTitleRe = regexp.MustCompile(`^#\s+Scenario:\s*(.+?)\s*$`)
	targetIDRe      = regexp.MustCompile(`(?i)([a-z0-9][a-z0-9_./-]*#(?:req|ac):[a-z0-9][a-z0-9-]*)`)
)

// ParseFeature parses one Feature README. Metadata and section handling are
// delegated to pkg/feature; this package only adds the normalized child
// records that provider integrations need.
func ParseFeature(path string) (*FeatureRecord, error) {
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "README.md")
	}
	parsedStatus, err := feature.ParseFeatureStatus(path)
	if err != nil {
		return nil, err
	}
	title, err := feature.ParseFeatureTitle(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := splitLines(data)
	featureID := featureIDFromPath(path)
	startLine := firstMatchingLine(lines, func(line string) bool { return strings.HasPrefix(line, "# Feature:") })
	if startLine == 0 {
		startLine = 1
	}
	record := &FeatureRecord{
		ID: featureID, Path: path, Title: title, Status: parsedStatus,
		Source: wholeRange(path, lines, startLine),
	}
	parseFeatureChildren(record, lines)
	record.Scenarios, err = parseScenarios(filepath.Dir(path), featureID)
	if err != nil {
		return nil, err
	}
	return record, nil
}

// Discover parses all Feature READMEs below specRoot/spec/features and
// returns deterministic records. It is useful for provider snapshots.
func Discover(specRoot string) ([]FeatureRecord, error) {
	featuresDir := filepath.Join(specRoot, "spec", "features")
	if info, err := os.Stat(featuresDir); err != nil || !info.IsDir() {
		return []FeatureRecord{}, nil
	}
	discovered, err := feature.Discover(featuresDir)
	if err != nil {
		return nil, err
	}
	result := make([]FeatureRecord, 0, len(discovered))
	for _, item := range discovered {
		r, err := ParseFeature(feature.ReadmePath(featuresDir, item.ID))
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

// SnapshotForRoot builds the versioned provider exchange shape for one
// repository root.
func SnapshotForRoot(specRoot string) (*Snapshot, error) {
	features, err := Discover(specRoot)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Version: ContractVersion, Features: features}, nil
}

// ParseScenario parses one standalone scenario file.
func ParseScenario(path string) (*ScenarioRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty scenario %q", path)
	}
	featureID := featureIDFromScenarioPath(path)
	slug := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	title := slug
	startLine := firstMatchingLine(lines, func(line string) bool { return scenarioTitleRe.MatchString(line) })
	if startLine > 0 {
		title = scenarioTitleRe.FindStringSubmatch(lines[startLine-1])[1]
	} else {
		startLine = 1
	}
	validates := parseValidates(lines)
	return &ScenarioRecord{
		ID: featureID + "#scenario:" + slug, FeatureID: featureID, Slug: slug,
		Title: title, Validates: validates, Source: wholeRange(path, lines, startLine),
	}, nil
}

func parseFeatureChildren(record *FeatureRecord, lines []string) {
	mask := lifecycle.StructuralMarkdownMask(lines, "")
	type heading struct {
		level, line int
		kind, slug  string
	}
	var headings []heading
	for i, line := range lines {
		if !mask[i] {
			continue
		}
		if m := reqHeadingRe.FindStringSubmatch(line); m != nil {
			headings = append(headings, heading{len(m[1]), i + 1, "req", strings.ToLower(m[2])})
		}
		if m := acHeadingRe.FindStringSubmatch(line); m != nil && inSection(lines, mask, i, "Acceptance Criteria") {
			headings = append(headings, heading{3, i + 1, "ac", strings.ToLower(m[2])})
		}
	}
	for _, h := range headings {
		end := nextHeadingEnd(lines, mask, h.line, h.level)
		rng := SourceRange{Path: record.Path, StartLine: h.line, StartColumn: 1, EndLine: end, EndColumn: lineEndColumn(lines, end)}
		if h.kind == "req" {
			record.Requirements = append(record.Requirements, RequirementRecord{ID: record.ID + "#req:" + h.slug, FeatureID: record.ID, Slug: h.slug, Title: h.slug, Source: rng})
		} else {
			record.AcceptanceCriteria = append(record.AcceptanceCriteria, AcceptanceCriterionRecord{ID: record.ID + "#ac:" + h.slug, FeatureID: record.ID, Slug: h.slug, Title: h.slug, Requirements: parseRequirementsField(lines, mask, h.line, end), Source: rng})
		}
	}
}

func nextHeadingEnd(lines []string, mask []bool, start, level int) int {
	for i := start; i < len(lines); i++ {
		if !mask[i] {
			continue
		}
		line := lines[i]
		count := 0
		for count < len(line) && line[count] == '#' {
			count++
		}
		if count > 0 && count <= level && count < len(line) && line[count] == ' ' {
			return i
		}
	}
	return len(lines)
}

func parseScenarios(featureDir, featureID string) ([]ScenarioRecord, error) {
	testsDir := filepath.Join(featureDir, "_tests")
	entries, err := os.ReadDir(testsDir)
	if os.IsNotExist(err) {
		return []ScenarioRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var result []ScenarioRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() == "README.md" {
			continue
		}
		r, err := ParseScenario(filepath.Join(testsDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		result = append(result, *r)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func parseRequirementsField(lines []string, mask []bool, start, end int) []string {
	for i := start; i < end && i <= len(lines); i++ {
		if !mask[i-1] {
			continue
		}
		trimmed := strings.TrimSpace(lines[i-1])
		if strings.HasPrefix(trimmed, "**Requirements:**") {
			return normalizeTargets(strings.TrimSpace(strings.TrimPrefix(trimmed, "**Requirements:**")))
		}
	}
	return nil
}

func parseValidates(lines []string) []string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "**Validates:**") {
			return normalizeTargets(strings.TrimSpace(strings.TrimPrefix(trimmed, "**Validates:")))
		}
	}
	return nil
}

func normalizeTargets(value string) []string {
	value = strings.ReplaceAll(value, "]", "")
	var result []string
	for _, match := range targetIDRe.FindAllStringSubmatch(value, -1) {
		id := match[1]
		idx := strings.Index(strings.ToLower(id), "#req:")
		prefix := "req:"
		if idx < 0 {
			idx = strings.Index(strings.ToLower(id), "#ac:")
			prefix = "ac:"
		}
		if idx >= 0 {
			colon := strings.IndexByte(id[idx:], ':')
			id = id[:idx] + "#" + prefix + id[idx+colon+1:]
		}
		result = append(result, id)
	}
	return result
}

func inSection(lines []string, mask []bool, index int, wanted string) bool {
	for i := index; i >= 0; i-- {
		if !mask[i] {
			continue
		}
		if strings.HasPrefix(lines[i], "## ") {
			return strings.TrimSpace(strings.TrimPrefix(lines[i], "## ")) == wanted
		}
		if strings.HasPrefix(lines[i], "# ") {
			return false
		}
	}
	return false
}

func featureIDFromPath(path string) string {
	p := filepath.ToSlash(path)
	marker := "/spec/features/"
	if idx := strings.Index(p, marker); idx >= 0 {
		rest := strings.TrimSuffix(p[idx+len(marker):], "/README.md")
		return rest
	}
	return filepath.Base(filepath.Dir(path))
}

func featureIDFromScenarioPath(path string) string {
	p := filepath.ToSlash(path)
	marker := "/spec/features/"
	if idx := strings.Index(p, marker); idx >= 0 {
		rest := p[idx+len(marker):]
		if cut := strings.Index(rest, "/_tests/"); cut >= 0 {
			return rest[:cut]
		}
	}
	return filepath.Base(filepath.Dir(filepath.Dir(path)))
}

func splitLines(data []byte) []string {
	return strings.Split(strings.TrimSuffix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n"), "\n")
}

func wholeRange(path string, lines []string, start int) SourceRange {
	return SourceRange{Path: path, StartLine: start, StartColumn: 1, EndLine: len(lines), EndColumn: lineEndColumn(lines, len(lines))}
}

func lineEndColumn(lines []string, line int) int {
	if line <= 0 || line > len(lines) {
		return 1
	}
	return len(lines[line-1]) + 1
}

func firstMatchingLine(lines []string, match func(string) bool) int {
	for i, line := range lines {
		if match(line) {
			return i + 1
		}
	}
	return 0
}
