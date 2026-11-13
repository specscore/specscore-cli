package lint

// Features implemented: cli/spec/lint/storage-layout

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// storageLayoutRuleNames are the rule IDs implemented by storageLayoutChecker,
// registered to the single checker with per-violation filtering on
// Violation.Rule (the parked-shape / parked-stale pattern):
//
//   - feature-storage-layout (error) — a Feature README must carry a
//     `## Storage layout` section that either states it stores nothing or
//     describes the layout with an `**Approved by:**` line
//     (cli/spec/lint/storage-layout#req:section-required).
//   - feature-storage-layout-pending (warning) — `**Approved by:** pending`
//     on a Feature not yet Approved, so a draft can be written and reviewed
//     before approval while the gap stays visible
//     (cli/spec/lint/storage-layout#req:pending).
//
// Both are opt-in: they report nothing unless the repository sets
// `storage_layout.required: true` in specscore.yaml, or the caller names one
// of them in --rules (cli/spec/lint/storage-layout#req:opt-in).
var storageLayoutRuleNames = []string{"feature-storage-layout", "feature-storage-layout-pending"}

const (
	storageLayoutRule        = "feature-storage-layout"
	storageLayoutPendingRule = "feature-storage-layout-pending"
	storageLayoutHeading     = "Storage layout"
	storageLayoutNone        = "None — this feature stores nothing."
	storageLayoutApprovedKey = "**approved by:**"
)

// storageLayoutPostApproval are the statuses at which a pending storage
// approval is an error rather than a warning: the Feature claims approval
// (or is being built, or is built) while its storage layout has none.
var storageLayoutPostApproval = []string{"approved", "implementing", "stable"}

// storageLayoutExempt are the statuses whose Features build nothing, so they
// need no storage layout.
var storageLayoutExempt = []string{"rejected", "deprecated"}

// storageLayoutDashRe normalizes the dash in the None statement: an em dash,
// an en dash, or one or more ASCII hyphens are all accepted.
var storageLayoutDashRe = regexp.MustCompile(`\s*(?:—|–|-+)\s*`)

type storageLayoutChecker struct {
	projectRoot string
	// explicit is true when the caller named one of storageLayoutRuleNames in
	// --rules: an explicit selection runs the rules without the config opt-in.
	explicit bool
}

func newStorageLayoutChecker(projectRoot string, explicit bool) *storageLayoutChecker {
	return &storageLayoutChecker{projectRoot: projectRoot, explicit: explicit}
}

func (c *storageLayoutChecker) name() string         { return storageLayoutRule }
func (c *storageLayoutChecker) severity() string     { return "error" }
func (c *storageLayoutChecker) fixTargets() []string { return []string{storageLayoutRule} }

// settings reports whether the rules are on for this spec tree, and the
// configured guide URL (empty when none).
func (c *storageLayoutChecker) settings(specRoot string) (bool, string) {
	cfg, err := projectdef.ReadSpecConfig(lintProjectRoot(c.projectRoot, specRoot))
	if err != nil {
		// Missing / unreadable specscore.yaml is reported by other rules;
		// without a readable opt-in the rules stay off unless selected.
		return c.explicit, ""
	}
	return c.explicit || cfg.StorageLayoutRequired(), cfg.StorageLayoutGuideURL()
}

func (c *storageLayoutChecker) check(specRoot string) ([]Violation, error) {
	on, guide := c.settings(specRoot)
	if !on {
		return nil, nil
	}
	var out []Violation
	err := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		rel, _ := filepath.Rel(specRoot, readmePath)
		out = append(out, checkStorageLayout(rel, string(content), guide)...)
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// fix inserts the section skeleton (`**Approved by:** pending` plus the three
// prompts) into every in-scope Feature README that has no `## Storage layout`
// section. It never touches a Feature that has one. Idempotent.
func (c *storageLayoutChecker) fix(specRoot string) error {
	on, guide := c.settings(specRoot)
	if !on {
		return nil
	}
	return walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		doc := parseStorageLayoutDoc(string(content))
		if !doc.isFeature || doc.exempt() || doc.found {
			return
		}
		_ = os.WriteFile(readmePath, []byte(insertStorageLayoutSkeleton(string(content), guide)), 0o644)
	})
}

// storageLayoutDoc is what the rules need from one README.
type storageLayoutDoc struct {
	isFeature    bool
	status       string
	statusLine   int // 1-based; 0 when absent
	found        bool
	headingLine  int      // 1-based
	content      []string // non-blank lines of the section with HTML comments removed
	approved     bool
	approvedLine int // 1-based
	approval     string
}

func (d storageLayoutDoc) exempt() bool {
	return slices.Contains(storageLayoutExempt, strings.ToLower(d.status))
}

// isFenceLine reports whether line opens or closes a fenced code block.
func isFenceLine(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~")
}

func parseStorageLayoutDoc(content string) storageLayoutDoc {
	var d storageLayoutDoc
	inFence, inSection, inComment := false, false, false
	for i, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if isFenceLine(line) {
			inFence = !inFence
		}
		if !inFence {
			if !d.isFeature && strings.HasPrefix(t, "# Feature: ") {
				d.isFeature = true
			}
			if d.statusLine == 0 && strings.HasPrefix(t, "**Status:**") {
				d.statusLine = i + 1
				d.status = strings.TrimSpace(strings.TrimPrefix(t, "**Status:**"))
			}
			if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "## ") || t == "---" {
				inSection = false
				if !d.found && strings.HasPrefix(t, "## ") &&
					strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(t, "## ")), storageLayoutHeading) {
					d.found, inSection, d.headingLine = true, true, i+1
				}
				continue
			}
		}
		if !inSection {
			continue
		}
		var visible string
		visible, inComment = stripHTMLComments(line, inComment)
		visible = strings.TrimSpace(visible)
		if visible == "" {
			continue
		}
		d.content = append(d.content, visible)
		if !d.approved && strings.HasPrefix(strings.ToLower(visible), storageLayoutApprovedKey) {
			d.approved, d.approvedLine = true, i+1
			d.approval = strings.TrimSpace(visible[len(storageLayoutApprovedKey):])
		}
	}
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

// isStorageLayoutNone reports whether s is the "stores nothing" statement,
// ignoring case, the dash form and a trailing period.
func isStorageLayoutNone(s string) bool {
	s = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	return storageLayoutDashRe.ReplaceAllString(s, " - ") == "none - this feature stores nothing"
}

func checkStorageLayout(rel, content, guide string) []Violation {
	d := parseStorageLayoutDoc(content)
	if !d.isFeature || d.exempt() {
		return nil
	}
	see := ""
	if guide != "" {
		see = " See " + guide
	}
	errAt := func(line int, msg string) []Violation {
		return []Violation{{File: rel, Line: line, Severity: "error", Rule: storageLayoutRule, Message: msg + see}}
	}
	switch {
	case !d.found:
		line := d.statusLine
		if line == 0 {
			line = 1
		}
		v := errAt(line, "Feature has no `## Storage layout` section; add one that says `"+storageLayoutNone+
			"` or lists the stored records with an `**Approved by:**` line (`specscore spec lint --fix` inserts a skeleton).")
		v[0].FixTarget = storageLayoutRule
		return v
	case len(d.content) == 0:
		return errAt(d.headingLine, "`## Storage layout` is empty; write `"+storageLayoutNone+
			"` or list the stored records and add an `**Approved by:**` line.")
	case isStorageLayoutNone(d.content[0]):
		return nil
	case !d.approved:
		return errAt(d.headingLine, "`## Storage layout` has no `**Approved by:**` line; show the layout to a human and record who approved it and when (write `pending` until then).")
	case d.approval == "":
		return errAt(d.approvedLine, "`**Approved by:**` is empty; record who approved the storage layout and when, or write `pending`.")
	case strings.HasPrefix(strings.ToLower(d.approval), "pending"):
		if slices.Contains(storageLayoutPostApproval, strings.ToLower(d.status)) {
			return errAt(d.approvedLine, "Feature is "+d.status+" but its storage layout is not approved (`**Approved by:** pending`); get a human's approval of the layout and record it before approving the Feature.")
		}
		return []Violation{{File: rel, Line: d.approvedLine, Severity: "warning", Rule: storageLayoutPendingRule,
			Message: "storage layout not yet approved (`**Approved by:** pending`); show it to a human and record their approval before the Feature is Approved." + see}}
	case len(d.content) == 1:
		return errAt(d.headingLine, "`## Storage layout` records an approval but describes no layout; list the stored records with example paths, or write `"+storageLayoutNone+"`.")
	}
	return nil
}

// storageLayoutSkeleton is the section --fix inserts (and therefore
// `specscore feature new` emits in an opted-in repository, via its
// scaffold → lint --fix pass).
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

// insertStorageLayoutSkeleton places the skeleton before `## Acceptance
// Criteria`, else before `## Open Questions`, else before the adherence
// footer's `---` separator, else at the end of the file.
func insertStorageLayoutSkeleton(content, guide string) string {
	lines := strings.Split(content, "\n")
	at := -1
	for _, target := range []string{"## Acceptance Criteria", "## Open Questions"} {
		inFence := false
		for i, l := range lines {
			if isFenceLine(l) {
				inFence = !inFence
			}
			if !inFence && strings.TrimSpace(l) == target {
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
