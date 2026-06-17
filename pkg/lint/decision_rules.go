package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Decision lint rule IDs. Each maps to a REQ in the decision Feature spec.
var decisionRuleIDs = []string{
	"D-title-format",
	"D-header-fields",
	"D-status-values",
	"D-filename-format",
	"D-number-assignment",
	"D-single-file",
	"D-required-sections",
	"D-declined-alternatives-non-empty",
	"D-observed-consequences-placeholder",
	"D-source-idea-optional",
	"D-supersedes-target-exists",
	"D-supersedes-bidirectional",
	"D-archived-location",
	"D-superseded-requires-successor",
	"D-affected-features-target-exists",
}

type decisionRulesChecker struct{}

func newDecisionRulesChecker() *decisionRulesChecker {
	return &decisionRulesChecker{}
}

func (c *decisionRulesChecker) name() string     { return "D-title-format" }
func (c *decisionRulesChecker) severity() string { return "error" }

func (c *decisionRulesChecker) check(specRoot string) ([]Violation, error) {
	return checkDecisions(specRoot)
}

// parsedDecision holds the parsed metadata and structure of a single decision file.
type parsedDecision struct {
	path     string // absolute path
	relPath  string // relative to specRoot
	archived bool   // lives under decisions/archived/

	number int    // parsed from filename NNNN prefix
	slug   string // full NNNN-slug

	title       string // text after "# Decision: "
	titleLine   int
	titleOK     bool
	hasTitleTag bool // has "# Decision:" prefix

	// Header fields in order of appearance
	fields      []decisionField
	fieldByName map[string]decisionField

	// Sections (## headings) in order
	sections      []decisionSection
	sectionByName map[string]decisionSection

	lines []string // raw file lines
}

type decisionField struct {
	Name  string
	Value string
	Line  int
}

type decisionSection struct {
	Title     string
	StartLine int
	Body      string
	SubH3s    []string // ### headings within this section
}

var decisionTitleRe = regexp.MustCompile(`^#\s+Decision:\s+(.+)$`)
var decisionFieldRe = regexp.MustCompile(`^\*\*([^*]+?):\*\*\s*(.*)$`)
var decisionFilenameRe = regexp.MustCompile(`^(\d{4})-([a-z0-9]+(?:-[a-z0-9]+)*)\.md$`)
var affectedFeatureSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// decisionRequiredFields in canonical order.
var decisionRequiredFields = []string{
	"Status", "Date", "Owner", "Tags", "Source Idea", "Supersedes", "Superseded By",
}

var decisionValidStatuses = map[string]bool{
	"Draft": true, "In Review": true, "Approved": true,
	"Rejected": true, "Superseded": true, "Deprecated": true,
}

var decisionRequiredSections = []string{
	"Context", "Decision", "Rationale", "Declined Alternatives",
	"Consequences at Decision Time", "Observed Consequences", "Affected Features",
}

func parseDecisionFile(path, relPath string, archived bool) (*parsedDecision, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Single source of truth for decision title/header/section parsing —
	// shared with the D-immutability committed-version comparison so a fix to
	// the parser (e.g. leading-frontmatter tolerance) covers both call sites.
	d, err := parseDecisionFromContentFn(string(data), relPath, archived)
	if err != nil {
		return nil, err
	}
	// Filename-derived fields are only meaningful for an on-disk file (not the
	// git-show'd content the immutability check parses).
	d.path = path
	base := filepath.Base(path)
	if m := decisionFilenameRe.FindStringSubmatch(base); m != nil {
		d.number, _ = strconv.Atoi(m[1])
		d.slug = strings.TrimSuffix(base, ".md")
	}
	return d, nil
}

func discoverDecisionFiles(specRoot string) ([]*parsedDecision, error) {
	decisionsDir := filepath.Join(specRoot, "decisions")
	if info, err := os.Stat(decisionsDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	var decisions []*parsedDecision

	// Walk top-level decisions (active)
	entries, err := osReadDirDecision(decisionsDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
			continue
		}
		path := filepath.Join(decisionsDir, e.Name())
		rel, _ := filepath.Rel(specRoot, path)
		d, err := parseDecisionFile(path, rel, false)
		if err != nil {
			continue
		}
		decisions = append(decisions, d)
	}

	// Walk archived decisions
	archivedDir := filepath.Join(decisionsDir, "archived")
	if info, err := os.Stat(archivedDir); err == nil && info.IsDir() {
		entries, err := osReadDirDecision(archivedDir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
				continue
			}
			path := filepath.Join(archivedDir, e.Name())
			rel, _ := filepath.Rel(specRoot, path)
			d, err := parseDecisionFile(path, rel, true)
			if err != nil {
				continue
			}
			decisions = append(decisions, d)
		}
	}

	return decisions, nil
}

func checkDecisions(specRoot string) ([]Violation, error) {
	decisionsDir := filepath.Join(specRoot, "decisions")
	if info, err := os.Stat(decisionsDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	var vs []Violation

	// D-single-file: detect directories under decisions/ that look like decision artifacts
	vs = append(vs, checkDecisionDirectories(specRoot)...)

	decisions, err := discoverDecisionFiles(specRoot)
	if err != nil {
		return nil, err
	}
	if len(decisions) == 0 {
		return vs, nil
	}

	// Build lookup by slug for cross-references
	bySlug := make(map[string]*parsedDecision)
	for _, d := range decisions {
		if d.slug != "" {
			bySlug[d.slug] = d
		}
	}

	// Collect all numbers for sequential check
	var numbers []int
	for _, d := range decisions {
		if d.number > 0 {
			numbers = append(numbers, d.number)
		}
	}
	sort.Ints(numbers)

	for _, d := range decisions {
		vs = append(vs, checkDecisionFile(d, specRoot, bySlug, numbers)...)
	}

	// D-supersedes-bidirectional (reverse): every "Superseded By: X" must point
	// to an existing Decision whose Supersedes points back. The forward check
	// (driven by Supersedes) never examines decisions reached only via
	// Superseded By, so orphan or one-sided back-references go undetected.
	for _, d := range decisions {
		f, ok := d.fieldByName["Superseded By"]
		if !ok {
			continue
		}
		val := strings.TrimSpace(f.Value)
		if val == "" || val == "—" || val == "-" {
			continue
		}
		successor, exists := bySlug[val]
		if !exists {
			vs = append(vs, Violation{
				File: d.relPath, Line: f.Line, Severity: "error",
				Rule:    "D-supersedes-bidirectional",
				Message: fmt.Sprintf("Superseded By references %q but that Decision does not exist", val),
			})
			continue
		}
		successorSupersedes := ""
		if sf, ok := successor.fieldByName["Supersedes"]; ok {
			successorSupersedes = strings.TrimSpace(sf.Value)
		}
		if successorSupersedes != d.slug {
			vs = append(vs, Violation{
				File: d.relPath, Line: f.Line, Severity: "error",
				Rule:    "D-supersedes-bidirectional",
				Message: fmt.Sprintf("Superseded By references %q but that Decision's Supersedes is %q (expected %q)", val, successorSupersedes, d.slug),
			})
		}
	}

	return vs, nil
}

func checkDecisionDirectories(specRoot string) []Violation {
	var vs []Violation
	decisionsDir := filepath.Join(specRoot, "decisions")

	entries, err := osReadDirDecision(decisionsDir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "archived" {
			continue
		}
		rel := filepath.Join("decisions", e.Name())
		vs = append(vs, Violation{
			File: rel, Severity: "error",
			Rule:    "D-single-file",
			Message: fmt.Sprintf("decisions must be single markdown files; directory found at %s", rel),
		})
	}

	// Also check archived/
	archivedDir := filepath.Join(decisionsDir, "archived")
	entries, err = osReadDirDecision(archivedDir)
	if err != nil {
		return vs
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rel := filepath.Join("decisions", "archived", e.Name())
		vs = append(vs, Violation{
			File: rel, Severity: "error",
			Rule:    "D-single-file",
			Message: fmt.Sprintf("decisions must be single markdown files; directory found at %s", rel),
		})
	}

	return vs
}

func checkDecisionFile(d *parsedDecision, specRoot string, bySlug map[string]*parsedDecision, allNumbers []int) []Violation {
	var vs []Violation

	// D-filename-format
	base := filepath.Base(d.path)
	if !decisionFilenameRe.MatchString(base) {
		vs = append(vs, Violation{
			File: d.relPath, Line: 0, Severity: "error",
			Rule:    "D-filename-format",
			Message: fmt.Sprintf("decision filename %q must match NNNN-<slug>.md", base),
		})
	}

	// D-title-format
	if !d.titleOK {
		msg := "decision title must use `# Decision: <Title>` format"
		if d.titleLine > 0 && !d.hasTitleTag {
			msg = "decision title must use `# Decision: <Title>` format (missing `Decision:` prefix)"
		}
		vs = append(vs, Violation{
			File: d.relPath, Line: max(d.titleLine, 1), Severity: "error",
			Rule: "D-title-format", Message: msg,
		})
	}

	// D-header-fields: required fields present and in order
	vs = append(vs, checkDecisionHeaderFields(d)...)

	// D-status-values
	if f, ok := d.fieldByName["Status"]; ok {
		if !decisionValidStatuses[f.Value] {
			vs = append(vs, Violation{
				File: d.relPath, Line: f.Line, Severity: "error",
				Rule:    "D-status-values",
				Message: fmt.Sprintf("Status %q must be one of Draft, In Review, Approved, Rejected, Superseded, Deprecated", f.Value),
			})
		}
	}

	// D-number-assignment: check for backfill
	if d.number > 0 && len(allNumbers) > 0 {
		highest := allNumbers[len(allNumbers)-1]
		if d.number < highest {
			isBackfill := true
			for _, n := range allNumbers {
				if n == d.number {
					isBackfill = false
					break
				}
			}
			if isBackfill {
				vs = append(vs, Violation{
					File: d.relPath, Line: 0, Severity: "error",
					Rule:    "D-number-assignment",
					Message: fmt.Sprintf("decision number %04d appears to backfill a gap (highest existing: %04d)", d.number, highest),
				})
			}
		}
	}

	// D-required-sections
	vs = append(vs, checkDecisionRequiredSections(d)...)

	// D-declined-alternatives-non-empty
	if s, ok := d.sectionByName["Declined Alternatives"]; ok {
		if len(s.SubH3s) == 0 {
			vs = append(vs, Violation{
				File: d.relPath, Line: s.StartLine, Severity: "error",
				Rule:    "D-declined-alternatives-non-empty",
				Message: "Declined Alternatives must contain at least one ### entry",
			})
		}
	}

	// D-observed-consequences-placeholder
	status := ""
	if f, ok := d.fieldByName["Status"]; ok {
		status = f.Value
	}
	if status == "Draft" {
		if s, ok := d.sectionByName["Observed Consequences"]; ok {
			body := strings.TrimSpace(s.Body)
			if !strings.Contains(body, "None observed yet.") {
				vs = append(vs, Violation{
					File: d.relPath, Line: s.StartLine, Severity: "error",
					Rule:    "D-observed-consequences-placeholder",
					Message: "Draft decisions must contain 'None observed yet.' in Observed Consequences",
				})
			}
		}
	}

	// D-adherence-footer (handled by adherence_footer.go via docTypeTargets)
	// No need to duplicate — we register walk functions there.

	// D-source-idea-optional: when non-empty, target must exist
	if f, ok := d.fieldByName["Source Idea"]; ok {
		val := strings.TrimSpace(f.Value)
		if val != "" && val != "—" && val != "-" {
			ideaPath := filepath.Join(specRoot, "ideas", val+".md")
			archivedIdeaPath := filepath.Join(specRoot, "ideas", "archived", val+".md")
			if _, err := os.Stat(ideaPath); err != nil {
				if _, err2 := os.Stat(archivedIdeaPath); err2 != nil {
					vs = append(vs, Violation{
						File: d.relPath, Line: f.Line, Severity: "error",
						Rule:    "D-source-idea-optional",
						Message: fmt.Sprintf("Source Idea %q does not resolve to an existing Idea under spec/ideas/", val),
					})
				}
			}
		}
	}

	// D-archived-location: Rejected/Superseded/Deprecated must be in archived/
	if status == "Rejected" || status == "Superseded" || status == "Deprecated" {
		if !d.archived {
			vs = append(vs, Violation{
				File: d.relPath, Line: d.fieldByName["Status"].Line, Severity: "error",
				Rule:    "D-archived-location",
				Message: fmt.Sprintf("Status: %s requires file to be in spec/decisions/archived/", status),
			})
		}
	}
	// Active decisions must NOT be in archived/
	if d.archived && (status == "Draft" || status == "In Review" || status == "Approved") {
		vs = append(vs, Violation{
			File: d.relPath, Line: d.fieldByName["Status"].Line, Severity: "error",
			Rule:    "D-archived-location",
			Message: fmt.Sprintf("files under spec/decisions/archived/ must have Status Rejected, Superseded, or Deprecated; got %q", status),
		})
	}

	// D-superseded-requires-successor
	if status == "Superseded" {
		supersededBy := ""
		if f, ok := d.fieldByName["Superseded By"]; ok {
			supersededBy = strings.TrimSpace(f.Value)
		}
		if supersededBy == "" || supersededBy == "—" || supersededBy == "-" {
			line := 0
			if f, ok := d.fieldByName["Superseded By"]; ok {
				line = f.Line
			}
			vs = append(vs, Violation{
				File: d.relPath, Line: line, Severity: "error",
				Rule:    "D-superseded-requires-successor",
				Message: "Status: Superseded requires a non-empty **Superseded By:** field",
			})
		}
	}
	// Deprecated and Rejected must have Superseded By = —
	if status == "Deprecated" || status == "Rejected" {
		if f, ok := d.fieldByName["Superseded By"]; ok {
			val := strings.TrimSpace(f.Value)
			if val != "—" && val != "-" && val != "" {
				vs = append(vs, Violation{
					File: d.relPath, Line: f.Line, Severity: "error",
					Rule:    "D-superseded-requires-successor",
					Message: fmt.Sprintf("Status: %s requires **Superseded By:** to be `—`", status),
				})
			}
		}
	}

	// D-supersedes-target-exists + D-supersedes-bidirectional
	if f, ok := d.fieldByName["Supersedes"]; ok {
		val := strings.TrimSpace(f.Value)
		if val != "" && val != "—" && val != "-" {
			target, exists := bySlug[val]
			if !exists {
				vs = append(vs, Violation{
					File: d.relPath, Line: f.Line, Severity: "error",
					Rule:    "D-supersedes-target-exists",
					Message: fmt.Sprintf("Supersedes target %q does not exist", val),
				})
			} else {
				// Bidirectional check
				targetSupersededBy := ""
				if tf, ok := target.fieldByName["Superseded By"]; ok {
					targetSupersededBy = strings.TrimSpace(tf.Value)
				}
				targetStatus := ""
				if tf, ok := target.fieldByName["Status"]; ok {
					targetStatus = tf.Value
				}

				if targetSupersededBy != d.slug {
					vs = append(vs, Violation{
						File: d.relPath, Line: f.Line, Severity: "error",
						Rule:    "D-supersedes-bidirectional",
						Message: fmt.Sprintf("this Decision supersedes %q but that Decision's Superseded By is %q (expected %q)", val, targetSupersededBy, d.slug),
					})
				}
				if targetStatus != "Superseded" {
					vs = append(vs, Violation{
						File: d.relPath, Line: f.Line, Severity: "error",
						Rule:    "D-supersedes-bidirectional",
						Message: fmt.Sprintf("this Decision supersedes %q but that Decision has Status %q (expected Superseded)", val, targetStatus),
					})
				}
				if !target.archived {
					vs = append(vs, Violation{
						File: d.relPath, Line: f.Line, Severity: "error",
						Rule:    "D-supersedes-bidirectional",
						Message: fmt.Sprintf("this Decision supersedes %q but that Decision is not in spec/decisions/archived/", val),
					})
				}
			}
		}
	}

	// D-affected-features-target-exists
	if s, ok := d.sectionByName["Affected Features"]; ok {
		vs = append(vs, checkAffectedFeatures(d.relPath, s, specRoot)...)
	}

	return vs
}

func checkDecisionHeaderFields(d *parsedDecision) []Violation {
	var vs []Violation

	seen := make(map[string]bool)
	for _, f := range d.fields {
		seen[f.Name] = true
	}

	for _, req := range decisionRequiredFields {
		if !seen[req] {
			vs = append(vs, Violation{
				File: d.relPath, Line: 0, Severity: "error",
				Rule:    "D-header-fields",
				Message: fmt.Sprintf("missing required header field **%s:**", req),
			})
		}
	}

	// Check ordering of required fields
	var present []string
	for _, f := range d.fields {
		for _, req := range decisionRequiredFields {
			if f.Name == req {
				present = append(present, f.Name)
				break
			}
		}
	}
	j := 0
	ordered := true
	for _, name := range present {
		for j < len(decisionRequiredFields) && decisionRequiredFields[j] != name {
			j++
		}
		if j >= len(decisionRequiredFields) {
			ordered = false
			break
		}
		j++
	}
	if !ordered {
		vs = append(vs, Violation{
			File: d.relPath, Line: 0, Severity: "error",
			Rule:    "D-header-fields",
			Message: "required header fields are not in canonical order (Status, Date, Owner, Tags, Source Idea, Supersedes, Superseded By)",
		})
	}

	return vs
}

func checkDecisionRequiredSections(d *parsedDecision) []Violation {
	var vs []Violation

	present := make(map[string]bool)
	for _, s := range d.sections {
		present[s.Title] = true
	}

	var missing []string
	for _, req := range decisionRequiredSections {
		if !present[req] {
			missing = append(missing, req)
		}
	}
	if len(missing) > 0 {
		vs = append(vs, Violation{
			File: d.relPath, Line: 0, Severity: "error",
			Rule:    "D-required-sections",
			Message: fmt.Sprintf("missing required section(s): %s", strings.Join(missing, ", ")),
		})
		return vs
	}

	// Check ordering
	var sectionOrder []string
	for _, s := range d.sections {
		sectionOrder = append(sectionOrder, s.Title)
	}

	idx := 0
	ordered := true
	for _, name := range sectionOrder {
		match := -1
		for k, req := range decisionRequiredSections {
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
			File: d.relPath, Line: 0, Severity: "error",
			Rule:    "D-required-sections",
			Message: "required sections present but not in canonical order",
		})
	}

	return vs
}

// checkAffectedFeatures validates that feature slugs in the Affected Features
// section resolve to existing directories under spec/features/.
func checkAffectedFeatures(relPath string, s decisionSection, specRoot string) []Violation {
	var vs []Violation
	body := strings.TrimSpace(s.Body)

	if body == "None at this time." || body == "" {
		return nil
	}

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		entry := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if entry == "None at this time." || entry == "" {
			continue
		}
		// Extract the slug: take text before " — " separator or first space.
		slug := entry
		if idx := strings.Index(slug, " — "); idx >= 0 {
			slug = slug[:idx]
		} else if idx := strings.Index(slug, " "); idx >= 0 {
			slug = slug[:idx]
		}
		slug = strings.TrimSpace(slug)
		// Skip entries that contain backticks, paths, or other non-slug chars.
		// Only validate entries that look like bare feature slugs.
		if strings.ContainsAny(slug, "`/") {
			continue
		}
		if slug == "—" || slug == "-" || slug == "" {
			continue
		}
		if !affectedFeatureSlugRe.MatchString(slug) {
			continue
		}
		featureDir := filepath.Join(specRoot, "features", slug)
		if _, err := os.Stat(featureDir); err != nil {
			vs = append(vs, Violation{
				File: relPath, Line: s.StartLine, Severity: "error",
				Rule:    "D-affected-features-target-exists",
				Message: fmt.Sprintf("Affected Feature %q does not resolve to a directory under spec/features/", slug),
			})
		}
	}

	return vs
}

// walkDecisionFiles invokes fn for every Decision file under specRoot/decisions/*.md
// (excluding README.md) and specRoot/decisions/archived/*.md (excluding README.md).
func walkDecisionFiles(specRoot string, fn func(path string, content []byte)) error {
	decisionsDir := filepath.Join(specRoot, "decisions")
	if info, err := os.Stat(decisionsDir); err != nil || !info.IsDir() {
		return nil
	}

	// Active decisions
	entries, err := osReadDirDecision(decisionsDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
			continue
		}
		path := filepath.Join(decisionsDir, e.Name())
		content, err := osReadFileDecision(path)
		if err != nil {
			continue
		}
		fn(path, content)
	}

	// Archived decisions
	archivedDir := filepath.Join(decisionsDir, "archived")
	if info, err := os.Stat(archivedDir); err == nil && info.IsDir() {
		entries, err := osReadDirDecision(archivedDir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "README.md" {
				continue
			}
			path := filepath.Join(archivedDir, e.Name())
			content, err := osReadFileDecision(path)
			if err != nil {
				continue
			}
			fn(path, content)
		}
	}

	return nil
}

// walkDecisionsIndex invokes fn for specRoot/decisions/README.md if present.
func walkDecisionsIndex(specRoot string, fn func(path string, content []byte)) error {
	path := filepath.Join(specRoot, "decisions", "README.md")
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	fn(path, content)
	return nil
}
