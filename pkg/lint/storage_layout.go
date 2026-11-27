package lint

// Features implemented: cli/spec/lint/storage-layout

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// storageLayoutRuleNames are the rule IDs implemented by storageLayoutChecker,
// registered to the single checker with per-violation filtering on
// Violation.Rule (the parked-shape / parked-stale pattern):
//
//   - feature-storage-layout (error) — a Feature README must carry one
//     `## Storage layout` section that either states it stores nothing or
//     describes the layout with an approved `**Approved by:**` line
//     (cli/spec/lint/storage-layout#req:section-required).
//   - feature-storage-layout-pending (warning) — an approval that is still
//     `pending` (or a placeholder) on a Feature not yet Approved, so a draft can
//     be reviewed while the gap stays visible
//     (cli/spec/lint/storage-layout#req:pending).
//   - feature-storage-layout-existing (info, advisory) — a layout that existed
//     before the rule, honestly recorded as not reviewed, admitted only when
//     storage_layout.existing_until is configured
//     (cli/spec/lint/storage-layout#req:existing-layouts).
//
// All are opt-in: they report nothing unless the repository sets
// `storage_layout.required: true` in specscore.yaml, or the caller names the
// rule in --rules or --fix (cli/spec/lint/storage-layout#req:opt-in).
var storageLayoutRuleNames = []string{storageLayoutRule, storageLayoutPendingRule, storageLayoutExistingRule}

const (
	storageLayoutRule         = "feature-storage-layout"
	storageLayoutPendingRule  = "feature-storage-layout-pending"
	storageLayoutExistingRule = "feature-storage-layout-existing"
	storageLayoutHeading      = "Storage layout"
	storageLayoutNone         = "None — this feature stores nothing."
	storageLayoutApprovedKey  = "**approved by:**"
)

// storageLayoutPreApproval are the Feature statuses at which an unapproved
// storage layout is a warning: the pre-approval values of
// feature.ValidStatuses plus the legacy `Under Review` and `Planned`. Every
// other status — Approved, Implementing, Stable, and any unknown, misspelled
// or missing value — is treated as approved, so the rule fails closed
// (cli/spec/lint/storage-layout#req:pending).
var storageLayoutPreApproval = []string{"draft", "in review", "amending", "under review", "planned"}

// storageLayoutKnownApproved are the statuses that are approved by name; any
// other non-pre-approval, non-exempt status is reported as unrecognized.
var storageLayoutKnownApproved = []string{"approved", "implementing", "stable"}

// storageLayoutExempt are the statuses whose Features build nothing, so they
// need no storage layout.
var storageLayoutExempt = []string{"rejected", "deprecated"}

// storageLayoutPlaceholders is THE closed set of `**Approved by:**` values
// that mean "nobody has approved this yet". They are compared lower-cased,
// trimmed and with one trailing period removed; the empty value is included.
// Documented in cli/spec/lint/storage-layout#req:approval-line.
var storageLayoutPlaceholders = []string{"", "—", "–", "-", "none", "n/a", "na", "tbd", "tba", "todo", "self", "?", "unknown", "nobody"}

// storageLayoutNegativeWords are first words that make an `**Approved by:**`
// value unapproved whatever follows (`not approved`, `no one`, `awaiting
// review`, `TBD (owner to review)`, ...). Compared as a whole lower-cased word.
var storageLayoutNegativeWords = []string{"not", "no", "none", "awaiting", "waiting", "pending", "tbd", "tba", "todo"}

// storageLayoutDateRe finds YYYY-MM-DD dates in an approval value; an approval
// needs at least one real calendar date.
var storageLayoutDateRe = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)

// storageLayoutApprovalShape is the one-line hint every not-approved message
// carries.
const storageLayoutApprovalShape = " An approval names who approved it and the date (YYYY-MM-DD), ideally with their words quoted, e.g. `**Approved by:** Ada Lovelace, 2026-11-13: \"yes, build it\"`."

// storageLayoutDashRe normalizes a dash: an em dash, an en dash, or one or
// more ASCII hyphens are all accepted.
var storageLayoutDashRe = regexp.MustCompile(`\s*(?:—|–|-+)\s*`)

// storageLayoutExistingRe matches the lower-cased existing-layout approval
// value `not reviewed — existing layout, recorded YYYY-MM-DD` (em dash, en
// dash or ASCII hyphens; optional trailing period).
var storageLayoutExistingRe = regexp.MustCompile(`^not reviewed\s*(?:—|–|-+)\s*existing layout,\s*recorded\s+(\d{4}-\d{2}-\d{2})\.?$`)

type storageLayoutChecker struct {
	projectRoot string
	// explicit is true when the caller named one of storageLayoutRuleNames in
	// --rules, or feature-storage-layout in --fix: an explicit selection runs
	// the rules without the config opt-in.
	explicit bool
	// fixNamed is true when --fix explicitly names feature-storage-layout.
	// The fix is opt-in (like no-source): a bare --fix / --fix=all never
	// inserts the section (cli/spec/lint/storage-layout#req:autofix-skeleton).
	fixNamed bool
}

func newStorageLayoutChecker(projectRoot string, explicit, fixNamed bool) *storageLayoutChecker {
	return &storageLayoutChecker{projectRoot: projectRoot, explicit: explicit || fixNamed, fixNamed: fixNamed}
}

func (c *storageLayoutChecker) name() string         { return storageLayoutRule }
func (c *storageLayoutChecker) severity() string     { return "error" }
func (c *storageLayoutChecker) fixTargets() []string { return []string{storageLayoutRule} }

// storageLayoutSettings is the rule's view of specscore.yaml.
type storageLayoutSettings struct {
	on            bool
	guide         string
	existingUntil time.Time
	hasExisting   bool
	configErr     string
	// configWarn is true when a malformed block also says `required: false`:
	// the repository is not opted in, so the problem is a warning.
	configWarn bool
}

// storageLayoutToday returns today's UTC date; a seam for tests.
var storageLayoutToday = func() time.Time {
	t, _ := time.Parse(dateLayout, time.Now().UTC().Format(dateLayout))
	return t
}

func (c *storageLayoutChecker) settings(specRoot string) storageLayoutSettings {
	cfg, err := projectdef.ReadSpecConfig(lintProjectRoot(c.projectRoot, specRoot))
	if err != nil {
		// No readable specscore.yaml: lint's repo-config gate reports that.
		// A storage_layout block can no longer make the read fail (shape
		// problems are recorded, not returned), so the rules simply stay off
		// unless explicitly selected.
		return storageLayoutSettings{on: c.explicit}
	}
	until, hasUntil := cfg.StorageLayoutExistingUntil()
	configErr := cfg.StorageLayoutShapeError()
	if configErr == "" && hasUntil && until.After(storageLayoutToday()) {
		configErr = "`storage_layout.existing_until` (" + until.Format(dateLayout) + ") is in the future; it must be on or before today, the day existing layouts were recorded"
	}
	return storageLayoutSettings{
		on:            c.explicit || cfg.StorageLayoutRequired(),
		guide:         cfg.StorageLayoutGuideURL(),
		existingUntil: until,
		hasExisting:   hasUntil,
		configErr:     configErr,
		configWarn:    cfg.StorageLayoutExplicitlyOff(),
	}
}

func (c *storageLayoutChecker) check(specRoot string) ([]Violation, error) {
	s := c.settings(specRoot)
	if s.configErr != "" {
		// A malformed block fails closed and visibly (req:config-shape); it is
		// a warning only when the block itself says `required: false`.
		severity := "error"
		if s.configWarn {
			severity = "warning"
		}
		return []Violation{{File: projectdef.SpecConfigFile, Severity: severity, Rule: storageLayoutRule,
			Message: "invalid `storage_layout` block in specscore.yaml: " + s.configErr + "; fix the block (see `specscore rules`)."}}, nil
	}
	if !s.on {
		return nil, nil
	}
	var out []Violation
	err := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		rel, _ := filepath.Rel(specRoot, readmePath)
		out = append(out, checkStorageLayout(rel, string(content), s)...)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// fix inserts the section skeleton into every in-scope Feature README that
// has no `## Storage layout` section. It runs only when --fix names
// feature-storage-layout. Idempotent.
func (c *storageLayoutChecker) fix(specRoot string) error {
	if !c.fixNamed {
		return nil
	}
	s := c.settings(specRoot)
	if s.configErr != "" || !s.on {
		return nil
	}
	return walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		if out, changed := AddStorageLayoutSkeleton(string(content), s.guide); changed {
			_ = os.WriteFile(readmePath, []byte(out), 0o644)
		}
	})
}

// storageLayoutDoc is what the rules need from one README.
type storageLayoutDoc struct {
	isFeature     bool
	status        string
	statusLine    int    // 1-based; 0 when absent
	date          string // the `**Date:**` header value; "" when absent
	found         bool
	headingLine   int // 1-based
	duplicateLine int // 1-based line of a second `## Storage layout`; 0 when none
	// content holds the section's visible lines (blank lines and HTML
	// comments removed); fenced code lines are kept verbatim.
	content      []string
	approved     bool
	approvedLine int // 1-based
	approval     string
	// unbalancedFence is true when the file ends inside a fenced code block.
	unbalancedFence bool
}

func (d storageLayoutDoc) exempt() bool {
	return slices.Contains(storageLayoutExempt, strings.ToLower(d.status))
}

// mdFence tracks a CommonMark fenced code block: it opens on a line of three
// or more backticks or tildes, and closes only on a line of the same
// character, at least as long, with nothing after it.
type mdFence struct {
	ch byte
	n  int
}

func (f *mdFence) open() bool { return f.n > 0 }

// fenceRun returns the fence character and run length that t starts with, or
// 0, 0 when t does not start with three or more backticks or tildes.
func fenceRun(t string) (byte, int) {
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return 0, 0
	}
	n := len(t) - len(strings.TrimLeft(t, t[:1]))
	if n < 3 {
		return 0, 0
	}
	return t[0], n
}

// feed advances the fence state over one line and reports whether the line
// was a fence line (opening or closing).
func (f *mdFence) feed(line string) bool {
	t := strings.TrimSpace(line)
	ch, n := fenceRun(t)
	if !f.open() {
		if n > 0 {
			f.ch, f.n = ch, n
			return true
		}
		return false
	}
	if ch == f.ch && n >= f.n && strings.TrimSpace(t[n:]) == "" {
		f.ch, f.n = 0, 0
		return true
	}
	return false
}

func parseStorageLayoutDoc(content string) storageLayoutDoc {
	var d storageLayoutDoc
	var fence mdFence
	inSection, inComment := false, false
	for i, line := range strings.Split(content, "\n") {
		if fence.open() {
			fence.feed(line)
			if t := strings.TrimSpace(line); inSection && t != "" {
				d.content = append(d.content, t)
			}
			continue
		}
		var visible string
		visible, inComment = stripHTMLComments(line, inComment)
		t := strings.TrimSpace(visible)
		if t == "" {
			continue
		}
		if fence.feed(t) {
			if inSection {
				d.content = append(d.content, t)
			}
			continue
		}
		if !d.isFeature && strings.HasPrefix(t, "# Feature: ") {
			d.isFeature = true
		}
		if d.statusLine == 0 && strings.HasPrefix(t, "**Status:**") {
			d.statusLine = i + 1
			d.status = strings.TrimSpace(strings.TrimPrefix(t, "**Status:**"))
		}
		if d.date == "" && strings.HasPrefix(t, "**Date:**") {
			d.date = strings.TrimSpace(strings.TrimPrefix(t, "**Date:**"))
		}
		if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "## ") || t == "---" {
			inSection = false
			if strings.HasPrefix(t, "## ") && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(t, "## ")), storageLayoutHeading) {
				if d.found {
					if d.duplicateLine == 0 {
						d.duplicateLine = i + 1
					}
					continue
				}
				d.found, inSection, d.headingLine = true, true, i+1
			}
			continue
		}
		if !inSection {
			continue
		}
		d.content = append(d.content, t)
		// A list-item marker before the approval line is tolerated.
		item := t
		for _, marker := range []string{"- ", "* ", "+ "} {
			item = strings.TrimSpace(strings.TrimPrefix(item, marker))
		}
		if !d.approved && strings.HasPrefix(strings.ToLower(item), storageLayoutApprovedKey) {
			d.approved, d.approvedLine = true, i+1
			d.approval = strings.TrimSpace(item[len(storageLayoutApprovedKey):])
		}
	}
	d.unbalancedFence = fence.open()
	return d
}

// stripHTMLComments removes `<!-- … -->` spans from line, carrying an open
// comment across lines via inComment.
func stripHTMLComments(line string, inComment bool) (string, bool) {
	var b strings.Builder
	for line != "" {
		if inComment {
			end := strings.Index(line, "-->")
			if end < 0 {
				return b.String(), true
			}
			line, inComment = line[end+3:], false
			continue
		}
		start := strings.Index(line, "<!--")
		if start < 0 {
			b.WriteString(line)
			break
		}
		b.WriteString(line[:start])
		line, inComment = line[start+4:], true
	}
	return b.String(), inComment
}

// normalizeStorageLayoutText lower-cases s, trims it, drops one trailing
// period and normalizes every dash to " - ".
func normalizeStorageLayoutText(s string) string {
	s = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	return strings.TrimSpace(storageLayoutDashRe.ReplaceAllString(s, " - "))
}

// isStorageLayoutNone reports whether s is the "stores nothing" statement,
// ignoring case, the dash form and a trailing period.
func isStorageLayoutNone(s string) bool {
	return normalizeStorageLayoutText(s) == "none - this feature stores nothing"
}

// storageLayoutApproved reports whether an approval value is a real
// approval: not a placeholder, not starting with a negative word, and
// carrying at least one real YYYY-MM-DD date
// (cli/spec/lint/storage-layout#req:approval-line).
func storageLayoutApproved(value string) bool {
	v := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if slices.Contains(storageLayoutPlaceholders, v) {
		return false
	}
	words := strings.FieldsFunc(v, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(words) > 0 && slices.Contains(storageLayoutNegativeWords, words[0]) {
		return false
	}
	for _, m := range storageLayoutDateRe.FindAllString(v, -1) {
		if _, err := time.Parse(dateLayout, m); err == nil {
			return true
		}
	}
	return false
}

// storageLayoutExisting classifies an approval value against the
// existing-layout form. isExisting is true for any value beginning with
// `not reviewed` that mentions `existing layout` — an attempt at the form. valid is true only
// for the exact form `not reviewed — existing layout, recorded YYYY-MM-DD`
// (any dash) with a real calendar date, returned as recorded.
func storageLayoutExisting(value string) (recorded time.Time, isExisting, valid bool) {
	v := strings.ToLower(strings.TrimSpace(value))
	if !strings.HasPrefix(v, "not reviewed") || !strings.Contains(v, "existing layout") {
		return time.Time{}, false, false
	}
	m := storageLayoutExistingRe.FindStringSubmatch(v)
	if m == nil {
		return time.Time{}, true, false
	}
	t, err := time.Parse(dateLayout, m[1])
	return t, true, err == nil
}

func checkStorageLayout(rel, content string, s storageLayoutSettings) []Violation {
	d := parseStorageLayoutDoc(content)
	if !d.isFeature || d.exempt() {
		return nil
	}
	see := ""
	if s.guide != "" {
		see = " See " + s.guide
	}
	errAt := func(line int, msg string) []Violation {
		return []Violation{{File: rel, Line: line, Severity: "error", Rule: storageLayoutRule, Message: msg + see}}
	}
	if d.duplicateLine != 0 {
		return errAt(d.duplicateLine, "Feature has more than one `## Storage layout` section; merge them into one.")
	}
	if !d.found {
		line := d.statusLine
		if line == 0 {
			line = 1
		}
		if d.unbalancedFence {
			return errAt(line, "Feature has no `## Storage layout` section, and `--fix=feature-storage-layout` cannot insert one because a fenced code block is never closed; close the fence, then add a section that says `"+storageLayoutNone+"` or lists the stored records with an `**Approved by:**` line.")
		}
		v := errAt(line, "Feature has no `## Storage layout` section; add one that says `"+storageLayoutNone+
			"` or lists the stored records with an `**Approved by:**` line (`specscore spec lint --fix=feature-storage-layout` inserts a skeleton).")
		v[0].FixTarget = storageLayoutRule
		return v
	}
	if len(d.content) == 0 {
		return errAt(d.headingLine, "`## Storage layout` is empty; write `"+storageLayoutNone+
			"` or list the stored records and add an `**Approved by:**` line.")
	}
	if slices.ContainsFunc(d.content, isStorageLayoutNone) {
		if len(d.content) == 1 {
			return nil
		}
		return errAt(d.headingLine, "`## Storage layout` says the Feature stores nothing but also lists content; keep only `"+
			storageLayoutNone+"`, or remove it and list the records with an `**Approved by:**` line.")
	}
	if !d.approved {
		return errAt(d.headingLine, "`## Storage layout` has no `**Approved by:**` line; show the layout to a human and record who approved it and when (write `pending` until then).")
	}
	recorded, isExisting, validExisting := storageLayoutExisting(d.approval)
	featureDate, dateErr := time.Parse(dateLayout, d.date)
	existingOK := validExisting && s.hasExisting && !recorded.After(s.existingUntil) &&
		dateErr == nil && !featureDate.After(s.existingUntil)
	if isExisting && !existingOK {
		// The grandfather line on a Feature it does not cover is never an
		// approval, at any status (req:existing-layouts).
		return errAt(d.approvedLine, "`**Approved by:** "+d.approval+"` is only for layouts that predate this rule: it counts when `storage_layout.existing_until` is set and both the recorded date and this Feature's `**Date:**` are on or before it; get a human's approval instead."+storageLayoutApprovalShape)
	}
	if existingOK || storageLayoutApproved(d.approval) {
		if len(d.content) == 1 {
			return errAt(d.headingLine, "`## Storage layout` records an approval but describes no layout; list the stored records with example paths, or write `"+storageLayoutNone+"`.")
		}
		if existingOK {
			return []Violation{{File: rel, Line: d.approvedLine, Severity: "info", Rule: storageLayoutExistingRule, Advisory: true,
				Message: "existing storage layout recorded, not reviewed by a human; show it to a human and replace the line with their approval." + see}}
		}
		return nil
	}
	shown := "`**Approved by:** " + d.approval + "`"
	if d.approval == "" {
		shown = "`**Approved by:**` is empty"
	}
	status := strings.ToLower(d.status)
	if slices.Contains(storageLayoutPreApproval, status) {
		return []Violation{{File: rel, Line: d.approvedLine, Severity: "warning", Rule: storageLayoutPendingRule,
			Message: "storage layout not yet approved (" + shown + "); show it to a human and record their approval before the Feature is Approved." + storageLayoutApprovalShape + see}}
	}
	label := "Feature is " + d.status
	if !slices.Contains(storageLayoutKnownApproved, status) {
		label = "Feature status `" + d.status + "` is not a recognized pre-approval status (treated as approved)"
	}
	return errAt(d.approvedLine, label+" but its storage layout is not approved ("+shown+"); get a human's approval of the layout and record it."+storageLayoutApprovalShape)
}

// storageLayoutSkeleton is the section the fix and `specscore feature new`
// insert. It never carries the existing-layout form.
func storageLayoutSkeleton(guide string) string {
	var b strings.Builder
	b.WriteString("## Storage layout\n\n<!--\n")
	b.WriteString("Show this layout to a human and record their approval below before building it.\n")
	b.WriteString("If this feature stores nothing, replace this section's body with: " + storageLayoutNone + "\n")
	if guide != "" {
		b.WriteString("Principles: " + guide + "\n")
	}
	b.WriteString("1. Example records with their paths (real-looking data).\n")
	b.WriteString("2. Existing records considered for reuse, and why each was or was not reused.\n")
	b.WriteString("3. Reads and writes per user action, and what happens at a very large scale (e.g. 100,000 rows or participants).\n")
	b.WriteString("-->\n\n**Approved by:** pending\n")
	return b.String()
}

// AddStorageLayoutSkeleton returns content with the `## Storage layout`
// skeleton (`**Approved by:** pending` plus the three prompts) inserted, and
// true, when content is a non-exempt Feature README without the section.
// Otherwise — not a Feature, exempt status, section already present, or a
// fenced code block left open (where an insert would be invisible and
// repeat on every run) — it returns content unchanged and false.
//
// The skeleton is placed before `## Acceptance Criteria`, else before
// `## Open Questions`, else before the adherence footer's `---` separator,
// else at the end of the file. CRLF files get CRLF line endings.
// `specscore feature new` applies it to the README it writes in an opted-in
// repository (cli/spec/lint/storage-layout#req:autofix-skeleton).
func AddStorageLayoutSkeleton(content, guide string) (string, bool) {
	d := parseStorageLayoutDoc(content)
	if !d.isFeature || d.exempt() || d.found || d.unbalancedFence {
		return content, false
	}
	crlf := strings.Contains(content, "\r\n")
	if crlf {
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}
	out := insertStorageLayoutSkeleton(content, guide)
	if crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return out, true
}

func insertStorageLayoutSkeleton(content, guide string) string {
	lines := strings.Split(content, "\n")
	at := -1
	for _, target := range []string{"## Acceptance Criteria", "## Open Questions"} {
		var fence mdFence
		for i, l := range lines {
			if fence.feed(l) || fence.open() {
				continue
			}
			if strings.TrimSpace(l) == target {
				at = i
				break
			}
		}
		if at >= 0 {
			break
		}
	}
	if at < 0 {
		for i := len(lines) - 2; i >= 0; i-- {
			if strings.TrimSpace(lines[i]) == "---" && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "*This document follows") {
				at = i
				break
			}
		}
	}
	skeleton := storageLayoutSkeleton(guide)
	if at < 0 {
		return strings.TrimRight(content, "\n") + "\n\n" + skeleton
	}
	head := strings.Join(lines[:at], "\n")
	tail := strings.Join(lines[at:], "\n")
	return strings.TrimRight(head, "\n") + "\n\n" + skeleton + "\n" + tail
}
