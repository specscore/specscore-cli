// Package plan parses single-file Plan artifacts at spec/plans/<slug>.md per
// the SpecStudio plan-Feature contract
// (https://github.com/specscore/specstudio-skills/blob/main/spec/features/skills/plan/README.md).
//
// The directory-form plans at spec/plans/<slug>/README.md historically used by
// specscore-cli are out of scope for this package — they are parsed by the
// existing plan-hierarchy / plan-roi-metadata lint checkers.
package plan

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// PlaceholderBodyToken is the byte-exact marker the parser recognizes as a
// placeholder task body in `**Mode:** stub` Plans. The MVP working decision
// (see Open Questions in the plan-rules Feature) is an HTML comment so
// the marker is invisible in rendered markdown.
const PlaceholderBodyToken = "<!-- implement: pending -->"

// Mode enumerates valid `**Mode:**` body-metadata values.
type Mode string

const (
	ModeFull Mode = "full"
	ModeStub Mode = "stub"
)

// TaskStatus enumerates valid `**Status:**` task body-field values.
type TaskStatus string

const (
	StatusPlanning   TaskStatus = "planning"
	StatusQueued     TaskStatus = "queued"
	StatusInProgress TaskStatus = "in_progress"
	StatusBlocked    TaskStatus = "blocked"
	StatusComplete   TaskStatus = "complete"
	StatusFailed     TaskStatus = "failed"
	StatusAborted    TaskStatus = "aborted"
)

// Plan is a parsed single-file Plan artifact.
type Plan struct {
	Path string // absolute path on disk
	Slug string // filename without `.md`

	HasPlanTitle bool   // first H1 line was `# Plan: <title>`
	TitleLine    int    // 1-based line number of the title (0 when absent)
	Title        string // the `<title>` portion after `# Plan: `

	SourceFeature     string   // value of `**Source Feature:**` (empty when missing)
	SourceFeatureLine int      // 1-based line of the field; 0 when absent
	SourceIdea        string   // Idea slug from `**Source:** idea:<slug>` (empty when not idea-sourced)
	SourceNone        bool     // true when the source line is `**Source:** none` (source-less plan)
	SourceRaw         string   // raw value of a `**Source:**` line as written (empty when absent)
	SourceLine        int      // 1-based line of the `**Source:**` field; 0 when absent
	Status            string   // value of `**Status:**` (empty when missing)
	StatusLine        int      // 1-based line of the field; 0 when absent
	Date              string   // value of `**Date:**` (empty when missing)
	DateLine          int      // 1-based line of the field; 0 when absent
	Owner             string   // value of `**Owner:**` (empty when missing)
	OwnerLine         int      // 1-based line of the field; 0 when absent
	Parent            string   // value of `**Parent:**` (empty when missing) — master/sub-plan composition
	ParentLine        int      // 1-based line of the field; 0 when absent
	PrerequisitePlans []string // same-repo predecessor plan slugs from `**Prerequisite Plans:**`
	PrerequisiteLine  int      // 1-based line of the field; 0 when absent
	PrerequisiteRaw   string   // raw field value, retained so lint can distinguish malformed input
	Mode              Mode     // `full` (default) or `stub`
	ModeLine          int      // 1-based line of `**Mode:**`; 0 when absent
	ModeRaw           string   // raw value as written (used by P-004 to report invalid tokens)
	ModeRawPresent    bool     // true when the field was present at all
	ModeValueValid    bool     // true when ModeRaw parsed cleanly into Mode

	Tasks []Task // task blocks in source order

	DeferredACs     []DeferredAC // entries under `## Deferred AC Coverage`
	DeferredACsLine int          // 1-based line of the H2 heading; 0 when absent
}

// Task captures a `### Task N: <name>` block.
type Task struct {
	Number      int      // parsed N from `### Task N:`
	Name        string   // text after `Task N: `
	HeadingLine int      // 1-based line of the `### Task N:` heading
	BodyLines   []string // lines after the heading, up to the next task / H2 / EOF (verbatim)
	BodyStart   int      // 1-based line where the body begins (one past the heading)

	Verifies         []string // AC IDs from `**Verifies:**`, in source order
	VerifiesLine     int      // 1-based line of `**Verifies:**`; 0 when absent
	VerifiesPresent  bool     // true when the field was present
	Id               string   // stable task identifier from `**Id:**`; empty when absent
	IdLine           int      // 1-based line of `**Id:**`; 0 when absent
	IdPresent        bool     // true when the field was present
	Status           TaskStatus
	StatusLine       int    // 1-based line of `**Status:**`; 0 when absent
	StatusRaw        string // raw value as written
	StatusPresent    bool   // true when the field was present
	StatusValueValid bool   // true when StatusRaw parsed cleanly into TaskStatus
	DependsOn        []int  // predecessor task numbers, empty when none
	DependsOnLine    int    // 1-based line of `**Depends-On:**`; 0 when absent
	DependsOnRaw     string // raw value as written
	DependsOnPresent bool
	DependsOnValid   bool // true when raw value parsed cleanly (em-dash or list of ints)
	HasPlaceholder   bool // true when the body contains the placeholder token on its own line
	PlaceholderLine  int  // 1-based line of the placeholder; 0 when absent

	// Implementation-commit provenance, written by `task change-status` as an
	// `**Implemented-by:** <ref>` field adjacent to the task's **Status:**.
	ImplementationCommit string // raw value of `**Implemented-by:**`; empty when absent or empty
	ImplementedByLine    int    // 1-based line of `**Implemented-by:**`; 0 when absent
	ImplementedByPresent bool   // true when the field was present (even with an empty value)
}

// DeferredAC is a single `- <feature-slug>#ac:<ac-slug> — <reason>` line.
type DeferredAC struct {
	ACID   string // `<feature-slug>#ac:<ac-slug>`
	Line   int    // 1-based line of the entry
	Reason string // text after the em-dash; opaque to lint
}

// IsSingleFilePlanPath reports whether path looks like a single-file Plan
// candidate location — i.e., directly under spec/plans/, has a `.md`
// extension, and is not named README.md (which is the index file).
//
// It does NOT read the file; callers still must validate the title prefix via
// Parse() before treating it as a Plan.
func IsSingleFilePlanPath(plansDir, filePath string) bool {
	if !strings.HasSuffix(filePath, ".md") {
		return false
	}
	if strings.HasSuffix(filePath, string(os.PathSeparator)+"README.md") || filePath == "README.md" {
		return false
	}
	// File must be a direct child of plansDir.
	rel, err := filepathRel(plansDir, filePath)
	if err != nil {
		return false
	}
	if strings.ContainsRune(rel, os.PathSeparator) {
		return false
	}
	return true
}

// Parse reads a candidate Plan file. It returns a populated Plan even when the
// file is not actually a Plan (HasPlanTitle == false in that case) so callers
// can distinguish "not a Plan" from "malformed Plan".
func Parse(path string) (*Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	p := &Plan{Path: path, Slug: slugFromPath(path), Mode: ModeFull}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	// Pass 1: locate title, header fields, section starts.
	type sectionStart struct {
		title string
		line  int // 0-based line index
	}
	var sections []sectionStart

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if rest, ok := strings.CutPrefix(trimmed, "# "); !p.HasPlanTitle && ok {
			if title, ok := strings.CutPrefix(rest, "Plan:"); ok {
				p.HasPlanTitle = true
				p.TitleLine = i + 1
				p.Title = strings.TrimSpace(title)
			}
			continue
		}
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			sections = append(sections, sectionStart{
				title: strings.TrimSpace(title),
				line:  i,
			})
		}
	}

	if !p.HasPlanTitle {
		return p, nil
	}

	// Header fields live between the title (exclusive) and the first ## heading.
	headerEnd := len(lines)
	if len(sections) > 0 {
		headerEnd = sections[0].line
	}
	for i := p.TitleLine; i < headerEnd; i++ {
		line := strings.TrimSpace(lines[i])
		if name, val, ok := matchBoldField(line); ok {
			switch name {
			case "Source Feature":
				p.SourceFeature = val
				p.SourceFeatureLine = i + 1
			case "Source":
				p.SourceRaw = val
				p.SourceLine = i + 1
				if rest, ok := strings.CutPrefix(val, "idea:"); ok {
					p.SourceIdea = strings.TrimSpace(rest)
				} else if val == "none" {
					p.SourceNone = true
				}
			case "Status":
				p.Status = val
				p.StatusLine = i + 1
			case "Date":
				p.Date = val
				p.DateLine = i + 1
			case "Owner":
				p.Owner = val
				p.OwnerLine = i + 1
			case "Parent":
				p.Parent = val
				p.ParentLine = i + 1
			case "Prerequisite Plans":
				p.PrerequisiteRaw = val
				p.PrerequisiteLine = i + 1
				p.PrerequisitePlans = splitCommaList(val)
			case "Mode":
				p.ModeRaw = val
				p.ModeLine = i + 1
				p.ModeRawPresent = true
				switch val {
				case "full":
					p.Mode = ModeFull
					p.ModeValueValid = true
				case "stub":
					p.Mode = ModeStub
					p.ModeValueValid = true
				default:
					// Leave p.Mode at its default (full) so downstream
					// rules can keep operating; P-004 reports the invalid value.
					p.ModeValueValid = false
				}
			}
		}
	}

	// Locate the `## Tasks` and `## Deferred AC Coverage` sections.
	tasksStart, tasksEnd := -1, len(lines)
	deferredStart, deferredEnd := -1, len(lines)
	for i, s := range sections {
		next := len(lines)
		if i+1 < len(sections) {
			next = sections[i+1].line
		}
		switch s.title {
		case "Tasks":
			tasksStart = s.line
			tasksEnd = next
		case "Deferred AC Coverage":
			deferredStart = s.line
			deferredEnd = next
			p.DeferredACsLine = s.line + 1
		}
	}

	if tasksStart >= 0 {
		p.Tasks = parseTasks(lines, tasksStart+1, tasksEnd)
	}
	if deferredStart >= 0 {
		p.DeferredACs = parseDeferredACs(lines, deferredStart+1, deferredEnd)
	}

	return p, nil
}

// taskHeadingRe matches `### Task N: <name>` where N is one-or-more digits.
var taskHeadingRe = regexp.MustCompile(`^###\s+Task\s+(\d+)\s*:\s*(.*)$`)

// boldFieldRe matches `**Name:** value`. The bold prefix MUST be at column 0
// after trimming so this stays unambiguous.
var boldFieldRe = regexp.MustCompile(`^\*\*([^*]+?):\*\*\s*(.*)$`)

func matchBoldField(line string) (name, val string, ok bool) {
	m := boldFieldRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), true
}

// parseTasks parses `### Task N: …` blocks within lines[start:end].
func parseTasks(lines []string, start, end int) []Task {
	type rawTask struct {
		num      int
		name     string
		headLine int // 0-based
		bodyFrom int
	}
	var raws []rawTask
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if m := taskHeadingRe.FindStringSubmatch(trimmed); m != nil {
			n, _ := strconv.Atoi(m[1])
			raws = append(raws, rawTask{
				num:      n,
				name:     strings.TrimSpace(m[2]),
				headLine: i,
				bodyFrom: i + 1,
			})
		}
	}
	tasks := make([]Task, 0, len(raws))
	for i, r := range raws {
		bodyEnd := end
		if i+1 < len(raws) {
			bodyEnd = raws[i+1].headLine
		}
		body := lines[r.bodyFrom:bodyEnd]
		t := Task{
			Number:      r.num,
			Name:        r.name,
			HeadingLine: r.headLine + 1,
			BodyStart:   r.bodyFrom + 1,
			BodyLines:   append([]string(nil), body...),
			Status:      StatusPlanning,
		}
		parseTaskBody(&t)
		tasks = append(tasks, t)
	}
	return tasks
}

// parseTaskBody walks t.BodyLines and populates the Verifies/Status/Depends-On/placeholder fields.
func parseTaskBody(t *Task) {
	for offset, raw := range t.BodyLines {
		line := strings.TrimSpace(raw)
		absLine := t.BodyStart + offset

		// Placeholder marker matching is byte-exact after trimming whitespace.
		if line == PlaceholderBodyToken {
			t.HasPlaceholder = true
			t.PlaceholderLine = absLine
			continue
		}

		name, val, ok := matchBoldField(line)
		if !ok {
			continue
		}
		switch name {
		case "Verifies":
			t.VerifiesPresent = true
			t.VerifiesLine = absLine
			t.Verifies = append(t.Verifies, splitCommaList(val)...)
		case "Id":
			t.IdPresent = true
			t.IdLine = absLine
			t.Id = val
		case "Status":
			t.StatusPresent = true
			t.StatusRaw = val
			t.StatusLine = absLine
			switch TaskStatus(val) {
			case StatusPlanning, StatusQueued, StatusInProgress, StatusBlocked, StatusComplete, StatusFailed, StatusAborted:
				t.Status = TaskStatus(val)
				t.StatusValueValid = true
			default:
				t.StatusValueValid = false
			}
		case "Depends-On":
			t.DependsOnPresent = true
			t.DependsOnRaw = val
			t.DependsOnLine = absLine
			deps, ok := parseDependsOn(val)
			t.DependsOnValid = ok
			t.DependsOn = deps
		case "Implemented-by":
			t.ImplementedByPresent = true
			t.ImplementedByLine = absLine
			t.ImplementationCommit = val
		}
	}
}

// parseDependsOn parses the value of `**Depends-On:**` into a slice of
// predecessor task numbers. The em-dash sentinel (`—`) or its ASCII
// equivalents (`-`, empty string) means "no predecessors". Returns (deps,
// true) on success or (deps, false) when any token failed to parse — partial
// results are still returned so callers can report specific offenders.
func parseDependsOn(val string) ([]int, bool) {
	v := strings.TrimSpace(val)
	if v == "" || v == "—" || v == "-" {
		return nil, true
	}
	parts := strings.Split(v, ",")
	var out []int
	ok := true
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue // trailing commas, double commas — tolerated
		}
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 {
			ok = false
			continue
		}
		out = append(out, n)
	}
	return out, ok
}

// splitCommaList splits a comma-separated header value, returning the trimmed
// non-empty parts in source order.
func splitCommaList(val string) []string {
	v := strings.TrimSpace(val)
	if v == "" || v == "—" || v == "-" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

// deferredEntryRe matches `- <feature-slug>#ac:<ac-slug> — <reason>` lines.
// The dash separator may be em-dash, en-dash, or ASCII hyphen and MUST be
// preceded by whitespace, so a hyphen inside the AC slug is never mistaken for
// the separator. The AC id is captured greedily as a single whitespace-free
// token. Reason text is opaque to the parser.
var deferredEntryRe = regexp.MustCompile(`^[-*]\s+(\S+#ac:\S+)(?:\s+[—–-]\s*(.*))?\s*$`)

func parseDeferredACs(lines []string, start, end int) []DeferredAC {
	var out []DeferredAC
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if m := deferredEntryRe.FindStringSubmatch(trimmed); m != nil {
			reason := ""
			if len(m) > 2 {
				reason = strings.TrimSpace(m[2])
			}
			out = append(out, DeferredAC{
				ACID:   strings.TrimSpace(m[1]),
				Line:   i + 1,
				Reason: reason,
			})
		}
	}
	return out
}

// slugFromPath returns the filename without `.md`.
func slugFromPath(path string) string {
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == os.PathSeparator || path[i] == '/' {
			base = path[i+1:]
			break
		}
	}
	return strings.TrimSuffix(base, ".md")
}

// filepathRel is a tiny inlined variant of filepath.Rel that returns an error
// when filePath is not under plansDir. We keep it inline to avoid an extra
// import in a tight helper.
func filepathRel(plansDir, filePath string) (string, error) {
	if !strings.HasPrefix(filePath, plansDir) {
		return "", fmt.Errorf("not under plans dir")
	}
	rel := strings.TrimPrefix(filePath, plansDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	if rel == "" {
		return "", fmt.Errorf("path is plans dir itself")
	}
	return rel, nil
}
