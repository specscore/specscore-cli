package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// ideaDiscoverFn is the injectable discovery function; tests may replace it.
var ideaDiscoverFn = idea.Discover

// featureParseStatusFn is the injectable feature-status parser; tests may replace it.
var featureParseStatusFn = feature.ParseFeatureStatus

// ideaChecker is a dispatch checker that runs all idea-* rules in one pass
// per file so parsing is shared.
type ideaChecker struct {
	fix bool
}

func newIdeaChecker() *ideaChecker {
	return &ideaChecker{}
}

// Rule names this checker emits. Keep aligned with allRuleNames.
var ideaRuleNames = []string{
	"idea-location",
	"idea-slug-format",
	"idea-single-file",
	"idea-title-format",
	"idea-header-fields",
	"idea-id-is-slug",
	"idea-required-sections",
	"idea-not-doing-non-empty",
	"idea-must-be-true-present",
	"idea-hmw-framing",
	"idea-status-values",
	"idea-specified-requires-promotion",
	"idea-archived-location",
	"idea-archive-reason",
	"idea-supersedes-target-archived",
	"idea-related-ideas-format",
	"idea-related-ideas-target-exists",
	"idea-sync-lint-strict",
	"idea-feature-cross-reference",
	"idea-index-completeness",
	"idea-index-row-sync",
	"idea-archived-index-chronological",
	"idea-type-values",
	"idea-type-title-consistency",
	"idea-targets-required",
	"idea-targets-exists",
	"idea-change-request-location",
	"idea-phase-non-empty",
}

func (c *ideaChecker) name() string     { return "idea-location" }
func (c *ideaChecker) severity() string { return "error" }

func (c *ideaChecker) check(specRoot string) ([]Violation, error) {
	return CheckIdeas(specRoot, c.fix)
}

// CheckIdeas runs all idea lint rules. If fix is true, auto-fixable
// violations are repaired on disk and omitted from the returned slice.
func CheckIdeas(specRoot string, fix bool) ([]Violation, error) {
	var violations []Violation

	ideasDir := idea.ResolveIdeasDir(specRoot)
	if info, err := os.Stat(ideasDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	// Legacy status autofix: rewrite the closed set of legacy idea **Status:**
	// tokens (Archived→Stale) before the checks run, so a single pass converges.
	// The Archived rewrite also sets the orthogonal `**Archived:** true` flag.
	if fix {
		if err := fixLegacyStatusesInTree(ideasDir, legacyIdeaStatusMap, true); err != nil {
			return nil, err
		}
	}

	// 1. Detect directories (REQ: single-file).
	dirs, err := idea.FindIdeaDirectories(specRoot)
	if err != nil {
		return nil, err
	}
	for _, d := range dirs {
		rel, _ := filepath.Rel(specRoot, d)
		violations = append(violations, Violation{
			File:     rel,
			Line:     0,
			Severity: "error",
			Rule:     "idea-single-file",
			Message:  fmt.Sprintf("ideas must be single markdown files; directory found at %s", rel),
		})
	}

	// 2. Check for misplaced idea files (idea-location).
	// We report files inside archived/ subdirs of archived/ (deeper nesting) or
	// in docs/ideas/ but only if inside specRoot (we stay within spec tree).
	misplaced, err := findMisplacedIdeaFiles(specRoot)
	if err != nil {
		return nil, err
	}
	for _, f := range misplaced {
		rel, _ := filepath.Rel(specRoot, f)
		violations = append(violations, Violation{
			File:     rel,
			Line:     0,
			Severity: "error",
			Rule:     "idea-location",
			Message:  fmt.Sprintf("idea files must live at spec/ideas/<slug>.md or spec/ideas/archived/<slug>.md; got %s", rel),
		})
	}

	// 3. Discover and parse idea files.
	discovered, err := ideaDiscoverFn(specRoot)
	if err != nil {
		return nil, err
	}

	parsed := make(map[string]*idea.Idea)
	archivedMap := make(map[string]bool)
	for _, d := range discovered {
		archivedMap[d.Slug] = d.Archived
		p, err := idea.Parse(d.Path)
		if err != nil {
			rel, _ := filepath.Rel(specRoot, d.Path)
			violations = append(violations, Violation{
				File:     rel,
				Severity: "error",
				Rule:     "idea-location",
				Message:  fmt.Sprintf("cannot read idea file: %v", err),
			})
			continue
		}
		parsed[d.Slug] = p
	}

	// 4. Feature -> ideas map.
	featureIdeas, err := idea.FeatureSourceIdeas(specRoot)
	if err != nil {
		return nil, err
	}

	// 5. Per-idea rules.
	for _, d := range discovered {
		p, ok := parsed[d.Slug]
		if !ok {
			continue
		}
		rel, _ := filepath.Rel(specRoot, d.Path)
		violations = append(violations, ideaFileRules(p, rel, d.Archived, d.IsProposal, d.FeatureDir, specRoot, parsed, archivedMap)...)
	}

	// 6. Cross-artifact: sync-lint-strict + feature-cross-reference.
	syncVs, fixedSync := ideaSyncRules(specRoot, parsed, archivedMap, featureIdeas, fix)
	violations = append(violations, syncVs...)

	// 7. Index completeness + chronological order.
	idxVs, _ := ideaIndexRules(specRoot, discovered, parsed, fix)
	violations = append(violations, idxVs...)

	_ = fixedSync
	return violations, nil
}

// findMisplacedIdeaFiles locates .md files under `spec/ideas/` but outside
// the allowed positions (top-level, archived/, or seeds/). The seeds/
// subdirectory holds sidekick-seed documents, which are a separate
// document type validated by the sidekick-seed rule — files there are
// not misplaced Ideas.
func findMisplacedIdeaFiles(specRoot string) ([]string, error) {
	ideasDir := idea.ResolveIdeasDir(specRoot)
	archivedDir := filepath.Join(ideasDir, "archived")
	seedsDir := filepath.Join(ideasDir, "seeds")
	var out []string
	err := filepath.Walk(ideasDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		dir := filepath.Dir(path)
		if dir == ideasDir || dir == archivedDir || dir == seedsDir {
			return nil
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// ideaFileRules returns violations for a single parsed idea file.
func ideaFileRules(p *idea.Idea, relPath string, archived bool, isProposal bool, featureDir string, specRoot string, all map[string]*idea.Idea, archivedMap map[string]bool) []Violation {
	var vs []Violation

	// idea-slug-format
	if err := idea.ValidateSlug(p.Slug); err != nil {
		vs = append(vs, Violation{
			File: relPath, Line: 0, Severity: "error",
			Rule: "idea-slug-format", Message: err.Error(),
		})
	}

	// idea-title-format — accept both "# Idea: <Name>" and "# Proposal: <Name>"
	if !p.HasTitle {
		vs = append(vs, Violation{
			File: relPath, Line: 1, Severity: "error",
			Rule: "idea-title-format", Message: "missing title (expected `# Idea: <Name>` or `# Proposal: <Name>`)",
		})
	} else if !p.TitleOK {
		vs = append(vs, Violation{
			File: relPath, Line: p.TitleLine, Severity: "error",
			Rule: "idea-title-format", Message: "title must use `# Idea: <Name>` or `# Proposal: <Name>` format",
		})
	} else if strings.TrimSpace(p.TitleName) == "" {
		vs = append(vs, Violation{
			File: relPath, Line: p.TitleLine, Severity: "error",
			Rule: "idea-title-format", Message: "title must use `# Idea: <Name>` or `# Proposal: <Name>` format (missing name)",
		})
	}

	// idea-id-is-slug — no **Id:** line.
	if _, ok := p.FieldByName["Id"]; ok {
		vs = append(vs, Violation{
			File: relPath, Line: p.FieldByName["Id"].Line, Severity: "error",
			Rule: "idea-id-is-slug", Message: "ideas must not carry an **Id:** field; filename slug is authoritative",
		})
	}

	// idea-header-fields: required fields present, in order.
	seen := map[string]bool{}
	order := []string{}
	for _, f := range p.Fields {
		if !seen[f.Name] {
			seen[f.Name] = true
			order = append(order, f.Name)
		}
	}
	for _, req := range idea.RequiredHeaderFields {
		if !seen[req] {
			vs = append(vs, Violation{
				File: relPath, Line: 0, Severity: "error",
				Rule: "idea-header-fields", Message: fmt.Sprintf("missing required header field **%s:**", req),
			})
		}
	}
	// Ordering check (ignore unknown fields).
	var filtered []string
	for _, name := range order {
		for _, req := range idea.RequiredHeaderFields {
			if name == req {
				filtered = append(filtered, name)
				break
			}
		}
	}
	// Check that filtered is a prefix of RequiredHeaderFields (subset keeping order).
	j := 0
	orderedOK := true
	for _, name := range filtered {
		for j < len(idea.RequiredHeaderFields) && idea.RequiredHeaderFields[j] != name {
			j++
		}
		if j >= len(idea.RequiredHeaderFields) {
			orderedOK = false
			break
		}
		j++
	}
	if !orderedOK {
		vs = append(vs, Violation{
			File: relPath, Line: 0, Severity: "error",
			Rule: "idea-header-fields", Message: "required header fields are not in the canonical order (Status, Date, Owner, Promotes To, Supersedes, Related Ideas)",
		})
	}

	// Determine effective type early — needed by multiple rules below.
	effectiveType := p.EffectiveType()

	// idea-status-values
	status := p.Status()
	if status != "" && !idea.ValidStatuses[status] {
		vs = append(vs, Violation{
			File: relPath, Line: p.FieldByName["Status"].Line, Severity: "error",
			Rule:    "idea-status-values",
			Message: fmt.Sprintf("Status %q is not one of Draft, In Review, Approved, Specifying, Specified, Implementing, Implemented, Rejected, Stale", status),
		})
	}

	// idea-specified-requires-promotion (covers all derived statuses that
	// require a non-empty Promotes To list). Change-request ideas are
	// author-managed and always have Promotes To = "—", so skip them.
	if effectiveType == "feature-request" {
		if status == "Specifying" || status == "Specified" || status == "Implementing" || status == "Implemented" {
			if len(p.PromotesTo()) == 0 {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Status"].Line, Severity: "error",
					Rule:    "idea-specified-requires-promotion",
					Message: fmt.Sprintf("Status: %s requires a non-empty **Promotes To:** list", status),
				})
			}
		}
	}

	// idea-archived-location
	// Archival is an orthogonal axis: an archived Idea keeps its real
	// terminal **Status:** and is marked by `**Archived:** true` PLUS
	// relocation to spec/ideas/archived/. The flag and the location MUST
	// agree (for feature-request ideas — change-request ideas stay at their
	// feature-scoped path and carry only the flag).
	//
	//   - archivedFlag is the `**Archived:** true` header axis.
	//   - archived is the location axis (file lives under archived/).
	archivedFlag := p.Archived()
	if effectiveType != "change-request" {
		if archivedFlag && !archived {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Archived"].Line, Severity: "error",
				Rule:    "idea-archived-location",
				Message: "**Archived:** true requires the file to live under spec/ideas/archived/",
			})
		}
		if archived && !archivedFlag {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Status"].Line, Severity: "error",
				Rule:    "idea-archived-location",
				Message: "files under spec/ideas/archived/ must carry **Archived:** true",
			})
		}
	}
	// An archived Idea must carry a terminal status — not a mid-lifecycle
	// one. Archival files an Idea away after it reaches a terminal outcome
	// (shipped, turned down, or decayed).
	if archivedFlag && status != "" {
		switch status {
		case "Implemented", "Rejected", "Stale":
		default:
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Status"].Line, Severity: "error",
				Rule:    "idea-archived-location",
				Message: fmt.Sprintf("archived idea must carry a terminal status (Implemented, Rejected, or Stale); got %q", status),
			})
		}
	}

	// idea-archive-note
	// The **Archive Note:** is OPTIONAL (the disposition reason lives on the
	// terminal status transition). Validate format-if-present only: when the
	// line exists it must be non-empty.
	if f, ok := p.FieldByName["Archive Note"]; ok {
		if note := p.ArchiveNote(); note == "" || note == "—" || note == "-" {
			vs = append(vs, Violation{
				File: relPath, Line: f.Line, Severity: "error",
				Rule:    "idea-archive-note",
				Message: "**Archive Note:** is present but empty; omit the line or give it content",
			})
		}
	}

	// idea-supersedes-target-archived
	// A superseded predecessor is filed away along the archived axis: it
	// carries `**Archived:** true` (and lives under archived/) and its
	// terminal status is Stale (Idea has no `Superseded` status).
	for _, sup := range p.Supersedes() {
		tgt, ok := all[sup]
		if !ok {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Supersedes"].Line, Severity: "error",
				Rule:    "idea-supersedes-target-archived",
				Message: fmt.Sprintf("supersedes target %q not found", sup),
			})
			continue
		}
		if !archivedMap[sup] || !tgt.Archived() {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Supersedes"].Line, Severity: "error",
				Rule:    "idea-supersedes-target-archived",
				Message: fmt.Sprintf("supersedes target %q must be archived (**Archived:** true under spec/ideas/archived/)", sup),
			})
		}
	}

	// idea-related-ideas-format + target-exists
	for _, entry := range p.RelatedIdeas() {
		rel, slug, ok := strings.Cut(entry, ":")
		rel = strings.TrimSpace(rel)
		slug = strings.TrimSpace(slug)
		if !ok || rel == "" || slug == "" {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Related Ideas"].Line, Severity: "error",
				Rule:    "idea-related-ideas-format",
				Message: fmt.Sprintf("related-ideas entry %q must be `<relationship>:<slug>`", entry),
			})
			continue
		}
		if !idea.ValidRelationships[rel] {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Related Ideas"].Line, Severity: "error",
				Rule:    "idea-related-ideas-format",
				Message: fmt.Sprintf("unknown relationship %q (valid: depends_on, alternative_to, extends, conflicts_with)", rel),
			})
			continue
		}
		if _, ok := all[slug]; !ok {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Related Ideas"].Line, Severity: "error",
				Rule:    "idea-related-ideas-target-exists",
				Message: fmt.Sprintf("related-ideas target %q does not resolve to an Idea", slug),
			})
		}
	}

	// idea-required-sections (in order)
	presentOrder := []string{}
	for _, s := range p.Sections {
		presentOrder = append(presentOrder, s.Title)
	}
	var missing []string
	for _, req := range idea.RequiredSections {
		if _, ok := p.SectionByTitle[req]; !ok {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		vs = append(vs, Violation{
			File: relPath, Line: 0, Severity: "error",
			Rule:    "idea-required-sections",
			Message: fmt.Sprintf("missing required section(s): %s", strings.Join(missing, ", ")),
		})
	} else {
		// Check order.
		idx := 0
		ordered := true
		for _, name := range presentOrder {
			match := -1
			for k, req := range idea.RequiredSections {
				if req == name {
					match = k
					break
				}
			}
			if match == -1 {
				continue
			}
			if match < idx {
				ordered = false
				break
			}
			idx = match
		}
		if !ordered {
			vs = append(vs, Violation{
				File: relPath, Line: 0, Severity: "error",
				Rule:    "idea-required-sections",
				Message: "required sections present but not in canonical order",
			})
		}
	}

	// idea-not-doing-non-empty
	if s, ok := p.SectionByTitle["Not Doing (and Why)"]; ok {
		if len(s.Items) == 0 {
			vs = append(vs, Violation{
				File: relPath, Line: s.StartLine, Severity: "error",
				Rule:    "idea-not-doing-non-empty",
				Message: "Not Doing (and Why) must contain at least one explicit exclusion",
			})
		}
	}

	// idea-must-be-true-present
	if s, ok := p.SectionByTitle["Key Assumptions to Validate"]; ok {
		tab := idea.ParseTable(s.Body)
		found := false
		if tab != nil {
			for _, row := range tab.Rows {
				if len(row) > 0 && strings.EqualFold(row[0], "Must-be-true") {
					// require the assumption column to be non-empty
					if len(row) > 1 && strings.TrimSpace(row[1]) != "" && strings.TrimSpace(row[1]) != "…" {
						found = true
						break
					}
				}
			}
		}
		if !found {
			vs = append(vs, Violation{
				File: relPath, Line: s.StartLine, Severity: "error",
				Rule:    "idea-must-be-true-present",
				Message: "Key Assumptions to Validate must list at least one Must-be-true assumption with content",
			})
		}
	}

	// idea-hmw-framing (warn)
	if s, ok := p.SectionByTitle["Problem Statement"]; ok {
		if !hmwRe.MatchString(s.Body) {
			vs = append(vs, Violation{
				File: relPath, Line: s.StartLine, Severity: "warning",
				Rule:    "idea-hmw-framing",
				Message: "Problem Statement should contain a 'How Might We' framing",
			})
		}
	}

	// --- Idea type, targets, phase, and change-request location rules ---

	ideaType := p.IdeaType()

	// idea-type-values — validate Type field when present.
	if ideaType != "" && !idea.ValidIdeaTypes[ideaType] {
		vs = append(vs, Violation{
			File: relPath, Line: p.FieldByName["Type"].Line, Severity: "error",
			Rule:    "idea-type-values",
			Message: fmt.Sprintf("Type %q is not one of feature-request, change-request", ideaType),
		})
	}

	// idea-type-title-consistency — title prefix must agree with type.
	if p.TitleOK {
		if p.TitlePrefix == "Proposal" && effectiveType != "change-request" {
			vs = append(vs, Violation{
				File: relPath, Line: p.TitleLine, Severity: "error",
				Rule:    "idea-type-title-consistency",
				Message: "`# Proposal:` title requires **Type:** change-request",
			})
		}
		if p.TitlePrefix == "Idea" && ideaType == "change-request" {
			vs = append(vs, Violation{
				File: relPath, Line: p.TitleLine, Severity: "error",
				Rule:    "idea-type-title-consistency",
				Message: "`# Idea:` title is not compatible with **Type:** change-request (use `# Proposal:` instead)",
			})
		}
	}

	// idea-targets-required — Targets required for change-request, prohibited for feature-request.
	targets := p.Targets()
	if effectiveType == "change-request" {
		if targets == "" || targets == "—" || targets == "-" {
			line := 0
			if f, ok := p.FieldByName["Targets"]; ok {
				line = f.Line
			}
			vs = append(vs, Violation{
				File: relPath, Line: line, Severity: "error",
				Rule:    "idea-targets-required",
				Message: "**Type:** change-request requires a non-empty **Targets:** field referencing a feature slug",
			})
		}
	}
	if effectiveType == "feature-request" && targets != "" && targets != "—" && targets != "-" {
		vs = append(vs, Violation{
			File: relPath, Line: p.FieldByName["Targets"].Line, Severity: "error",
			Rule:    "idea-targets-required",
			Message: "**Targets:** is not allowed when Type is feature-request (or absent)",
		})
	}

	// idea-targets-exists — Targets must reference an existing Feature directory
	// (one that contains a README.md).
	if effectiveType == "change-request" && targets != "" && targets != "—" && targets != "-" {
		featureReadme := filepath.Join(specRoot, "features", targets, "README.md")
		if _, err := os.Stat(featureReadme); err != nil {
			vs = append(vs, Violation{
				File: relPath, Line: p.FieldByName["Targets"].Line, Severity: "error",
				Rule:    "idea-targets-exists",
				Message: fmt.Sprintf("**Targets:** %q does not resolve to an existing feature directory", targets),
			})
		}
	}

	// idea-change-request-location — change-request must live at
	// spec/features/{targets}/proposals/{slug}.md
	if effectiveType == "change-request" && targets != "" && targets != "—" && targets != "-" {
		expectedRel := filepath.Join("features", targets, "proposals", p.Slug+".md")
		if relPath != expectedRel {
			vs = append(vs, Violation{
				File: relPath, Line: 0, Severity: "error",
				Rule:    "idea-change-request-location",
				Message: fmt.Sprintf("change-request idea must be at %s; got %s", expectedRel, relPath),
			})
		}
	}

	// idea-phase-non-empty — Phase, when present, must not be empty.
	if f, ok := p.FieldByName["Phase"]; ok {
		phase := strings.TrimSpace(f.Value)
		if phase == "" {
			vs = append(vs, Violation{
				File: relPath, Line: f.Line, Severity: "error",
				Rule:    "idea-phase-non-empty",
				Message: "**Phase:** field is present but empty; provide a value or remove the field",
			})
		}
	}

	// idea-header-fields ordering: validate Type, Targets, Phase position
	// relative to Status and Date. When present, Type/Targets/Phase must
	// appear after Status and before Date.
	if len(p.Fields) > 0 {
		fieldPositions := make(map[string]int)
		for idx, f := range p.Fields {
			if _, exists := fieldPositions[f.Name]; !exists {
				fieldPositions[f.Name] = idx
			}
		}
		statusPos, hasStatus := fieldPositions["Status"]
		datePos, hasDate := fieldPositions["Date"]
		typePos, hasType := fieldPositions["Type"]
		targetsPos, hasTargets := fieldPositions["Targets"]
		phasePos, hasPhase := fieldPositions["Phase"]

		if hasType {
			if hasStatus && typePos <= statusPos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Type"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Type:** must appear after **Status:**",
				})
			}
			if hasDate && typePos >= datePos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Type"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Type:** must appear before **Date:**",
				})
			}
		}
		if hasTargets {
			if hasType && targetsPos <= typePos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Targets"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Targets:** must appear after **Type:**",
				})
			} else if !hasType && hasStatus && targetsPos <= statusPos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Targets"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Targets:** must appear after **Status:**",
				})
			}
			if hasDate && targetsPos >= datePos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Targets"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Targets:** must appear before **Date:**",
				})
			}
		}
		if hasPhase {
			if hasTargets && phasePos <= targetsPos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Phase"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Phase:** must appear after **Targets:**",
				})
			} else if !hasTargets && hasType && phasePos <= typePos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Phase"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Phase:** must appear after **Type:**",
				})
			}
			if hasDate && phasePos >= datePos {
				vs = append(vs, Violation{
					File: relPath, Line: p.FieldByName["Phase"].Line, Severity: "error",
					Rule:    "idea-header-fields",
					Message: "**Phase:** must appear before **Date:**",
				})
			}
		}
	}

	return vs
}

var hmwRe = regexp.MustCompile(`(?i)how might we`)

// ideaSyncRules enforces that Feature **Source Ideas:** entries agree with
// each idea's **Promotes To:** / **Status:**. When fix is true, rewrites the
// idea header in place and omits the fixed violations.
func ideaSyncRules(specRoot string, parsed map[string]*idea.Idea, archivedMap map[string]bool, featureIdeas map[string][]string, fix bool) ([]Violation, bool) {
	var vs []Violation
	fixed := false

	// Build reverse index: idea slug -> []feature slug. Cross-repo references
	// (URL form) name an Idea in another repo and are excluded — they resolve
	// via the linkage system, not the local spec tree.
	reverse := make(map[string][]string)
	for feature, ideas := range featureIdeas {
		for _, slug := range ideas {
			if idea.IsCrossRepoRef(slug) {
				continue
			}
			reverse[slug] = append(reverse[slug], feature)
		}
	}
	for _, list := range reverse {
		sort.Strings(list)
	}

	// idea-feature-cross-reference: each feature->idea reference must resolve
	// and the idea must be Approved, Specifying, Specified, Implementing, or
	// Implemented (not Draft/In Review/Rejected/Stale).
	validCrossRefStatuses := map[string]bool{
		"Approved":     true,
		"Specifying":   true,
		"Specified":    true,
		"Implementing": true,
		"Implemented":  true,
	}
	for featureSlug, ideas := range featureIdeas {
		for _, slug := range ideas {
			// Cross-repo references (URL form) point at an Idea in another
			// repository; they resolve via the linkage system and are not checked
			// against the local spec tree.
			if idea.IsCrossRepoRef(slug) {
				continue
			}
			p, ok := parsed[slug]
			if !ok {
				rel := filepath.Join("features", featureSlug, "README.md")
				vs = append(vs, Violation{
					File: rel, Line: 0, Severity: "error",
					Rule:    "idea-feature-cross-reference",
					Message: fmt.Sprintf("Source Ideas references %q which does not resolve to an Idea", slug),
				})
				continue
			}
			st := p.Status()
			if !validCrossRefStatuses[st] {
				rel := filepath.Join("features", featureSlug, "README.md")
				vs = append(vs, Violation{
					File: rel, Line: 0, Severity: "error",
					Rule:    "idea-feature-cross-reference",
					Message: fmt.Sprintf("Source Ideas references idea %q with Status %q (must be Approved, Specifying, Specified, Implementing, or Implemented)", slug, st),
				})
			}
		}
	}

	// Feature status cache for derivation. Read each referenced feature's
	// **Status:** value lazily so we only touch what we need.
	featureStatusCache := make(map[string]string)
	getFeatureStatus := func(featureSlug string) string {
		if st, ok := featureStatusCache[featureSlug]; ok {
			return st
		}
		readme := filepath.Join(specRoot, "features", featureSlug, "README.md")
		st, err := featureParseStatusFn(readme)
		if err != nil {
			st = ""
		}
		featureStatusCache[featureSlug] = st
		return st
	}

	// idea-sync-lint-strict per idea.
	//
	// Derivation rules (per spec REQ:implementing-derivation +
	// REQ:specified-derivation):
	//   - no Feature references                                          -> Approved
	//   - 1+ refs, any  referenced Feature at Draft or In Review         -> Specifying
	//   - 1+ refs, every referenced Feature at Approved                  -> Specified
	//   - 1+ refs, any  referenced Feature at Implementing               -> Implementing
	//   - 1+ refs, every referenced Feature at Stable                    -> Implemented
	//
	// Change-request ideas are author-managed — skip all derivation.
	for slug, p := range parsed {
		// Skip change-request ideas: they are author-managed, not derived.
		if p.EffectiveType() == "change-request" {
			continue
		}

		refs := reverse[slug]
		expectedPromotes := append([]string{}, refs...)
		sort.Strings(expectedPromotes)

		// Split actual Promotes To into local slugs and cross-repo references.
		// A cross-repo entry (URL form) promotes to a Feature in ANOTHER repo;
		// it resolves via the linkage system and cannot be derived from — nor
		// validated against — the local spec tree. Such entries are
		// author-managed: excluded from the local promotes comparison, and their
		// presence makes the idea's Status author-managed too, since the
		// cross-repo Feature statuses that would drive derivation aren't visible
		// here.
		var localActual, crossPromotes []string
		for _, e := range p.PromotesTo() {
			if idea.IsCrossRepoRef(e) {
				crossPromotes = append(crossPromotes, e)
			} else {
				localActual = append(localActual, e)
			}
		}
		sort.Strings(localActual)
		hasCrossPromotes := len(crossPromotes) > 0

		var expectedStatus string
		if len(refs) == 0 {
			// No references. The idea may legitimately be Draft/Approved/etc.
			// Drift only if current status is one of the derived states.
			cur := p.Status()
			if cur == "Specifying" || cur == "Specified" || cur == "Implementing" || cur == "Implemented" {
				expectedStatus = "Approved" // drop back
			} else {
				expectedStatus = "" // no change expected
			}
		} else {
			// "Done" = Stable or Deprecated. Deprecated is a post-Stable
			// terminal state, so it must derive like Stable (Implemented) —
			// never drag the Idea backward to Specified.
			allDone := true
			anyImplementing := false
			anyDraftOrInReview := false
			for _, fs := range refs {
				fst := getFeatureStatus(fs)
				if fst != "Stable" && fst != "Deprecated" {
					allDone = false
				}
				if fst == "Implementing" {
					anyImplementing = true
				}
				if fst == string(lifecycle.FeatureDraft) || fst == string(lifecycle.FeatureInReview) {
					anyDraftOrInReview = true
				}
			}
			if allDone {
				expectedStatus = "Implemented"
			} else if anyImplementing {
				expectedStatus = "Implementing"
			} else if anyDraftOrInReview {
				expectedStatus = "Specifying"
			} else {
				// All features at Approved (none Draft/In Review/Implementing/Stable/Deprecated).
				expectedStatus = "Specified"
			}
		}

		// Cross-repo promotions make Status author-managed: skip derivation so a
		// mid-flight idea specified via features in another repo is not dragged
		// back to Approved.
		if hasCrossPromotes {
			expectedStatus = ""
		}

		// Compare the local-derived set against only the LOCAL Promotes To
		// entries; cross-repo entries are always allowed and never count as drift.
		promotesDrift := !stringSliceEq(expectedPromotes, localActual)
		statusDrift := expectedStatus != "" && p.Status() != expectedStatus

		if !promotesDrift && !statusDrift {
			continue
		}

		rel, _ := filepath.Rel(specRoot, p.Path)
		if fix {
			newStatus := p.Status()
			if statusDrift {
				newStatus = expectedStatus
			}
			// Preserve cross-repo promotions verbatim; only the local set is
			// derived from the reverse index.
			newList := append(append([]string{}, expectedPromotes...), crossPromotes...)
			newPromotes := strings.Join(newList, ", ")
			if newPromotes == "" {
				newPromotes = "—"
			}
			if err := rewriteIdeaHeader(p.Path, map[string]string{
				"Status":      newStatus,
				"Promotes To": newPromotes,
			}); err == nil {
				// Reflect the on-disk rewrite into the in-memory `parsed`
				// state so downstream rules in the same lint pass (the
				// ideas-index sync below) see the post-rewrite values.
				// Without this, idea-index-row-sync runs against stale
				// data and the index README sync only happens on a
				// SECOND `--fix` pass, breaking fix-is-idempotent.
				if hf, ok := p.FieldByName["Status"]; ok {
					hf.Value = newStatus
					p.FieldByName["Status"] = hf
				}
				if hf, ok := p.FieldByName["Promotes To"]; ok {
					hf.Value = newPromotes
					p.FieldByName["Promotes To"] = hf
				}
				fixed = true
				continue
			}
		}

		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "idea-sync-lint-strict",
			Message: fmt.Sprintf("idea %q drift: Promotes To / Status disagree with referencing features (run `specscore lint --fix`)", slug),
		})
	}

	return vs, fixed
}

func stringSliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// rewriteIdeaHeader updates named **Field:** lines in place. Only fields
// already present are updated; no new fields are inserted.
func rewriteIdeaHeader(path string, updates map[string]string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		m := fieldRewriteRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[2]
		if v, ok := updates[name]; ok {
			lines[i] = fmt.Sprintf("%s**%s:** %s", m[1], name, v)
		}
	}
	out := []byte(strings.Join(lines, "\n"))
	// Keep the leading frontmatter `status:` mirror in sync with the body
	// **Status:** we just advanced, so a single `lint --fix` pass converges
	// (issue #68). Without this the body/frontmatter disagree and the next
	// lint reports a status-mirror drift that --fix itself introduced — a
	// state only `migrate` could previously repair. setFrontmatterStatus is
	// the same canonical helper statusMirrorChecker.fix uses: body is
	// canonical, and a missing frontmatter block is left for `migrate`.
	if status, ok := updates["Status"]; ok {
		out = setFrontmatterStatus(out, status)
	}
	return os.WriteFile(path, out, 0o644)
}

var fieldRewriteRe = regexp.MustCompile(`^(\s*)\*\*([^*]+?):\*\*\s*(.*)$`)
