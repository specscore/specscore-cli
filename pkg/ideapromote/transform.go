package ideapromote

import (
	"strings"

	"github.com/specscore/specscore-cli/pkg/idea"
)

// SeedContent is the parsed shape of a sidekick seed relevant to the
// promote transform.
type SeedContent struct {
	// Frontmatter holds the YAML frontmatter key→raw-value lines (in
	// encounter order) when the seed opens with a `---` fence. Empty when
	// the seed has no frontmatter.
	Frontmatter map[string]string
	// FrontmatterOrder preserves the key order of Frontmatter.
	FrontmatterOrder []string
	// Title is the text after the first `# ` H1 line (the seed's title).
	// Empty when the seed has no H1.
	Title string
	// Body is the prose between the title and the first `## ` H2 section
	// (or end of file), trimmed of surrounding blank lines. This is what
	// folds into the Idea's ## Context.
	Body string
	// Verdict is the full `## Consilium Verdict` section (heading line
	// included) when present, else empty.
	Verdict string
}

// ParseSeed parses a sidekick-seed file body into its promote-relevant
// parts: optional YAML frontmatter, the H1 title, the prose body up to
// the first H2, and a ## Consilium Verdict section if present.
func ParseSeed(content string) SeedContent {
	var sc SeedContent
	sc.Frontmatter = make(map[string]string)

	lines := strings.Split(content, "\n")
	idx := 0

	// Frontmatter: a leading `---` fence closed by another `---`.
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for j := 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "---" {
				idx = j + 1
				break
			}
			line := lines[j]
			if k := strings.IndexByte(line, ':'); k >= 0 {
				key := strings.TrimSpace(line[:k])
				val := strings.TrimSpace(line[k+1:])
				if key != "" {
					if _, ok := sc.Frontmatter[key]; !ok {
						sc.FrontmatterOrder = append(sc.FrontmatterOrder, key)
					}
					sc.Frontmatter[key] = val
				}
			}
		}
	}

	// Title: first `# ` H1 after frontmatter.
	for idx < len(lines) {
		t := strings.TrimSpace(lines[idx])
		if strings.HasPrefix(t, "# ") {
			sc.Title = strings.TrimSpace(strings.TrimPrefix(t, "# "))
			idx++
			break
		}
		if t == "" {
			idx++
			continue
		}
		// Non-blank, non-title line before any H1: no title; the body
		// starts here.
		break
	}

	// Walk remaining lines: prose body until the first `## ` heading.
	// Capture a `## Consilium Verdict` section in full.
	var bodyLines []string
	var verdictLines []string
	inVerdict := false
	for ; idx < len(lines); idx++ {
		line := lines[idx]
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(t, "## ")), "Consilium Verdict") {
				inVerdict = true
				verdictLines = append(verdictLines, line)
				continue
			}
			// Any other H2 ends both prose and verdict capture.
			inVerdict = false
			continue
		}
		if inVerdict {
			verdictLines = append(verdictLines, line)
			continue
		}
		bodyLines = append(bodyLines, line)
	}

	sc.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	sc.Verdict = strings.TrimRight(strings.Join(verdictLines, "\n"), "\n")
	return sc
}

// TransformOptions controls the seed→Idea transform.
type TransformOptions struct {
	Slug string
	// Owner defaults to the seed's captured_by / unknown when empty.
	Owner string
	// Date defaults to today (UTC) when empty.
	Date string
	// VerdictMode selects how the seed verdict is carried forward.
	VerdictMode VerdictMode
	// SeedRefForPointer is the provenance reference written by the
	// pointer mode (e.g. the seed's repo-relative path). When empty the
	// pointer falls back to the seed filename.
	SeedRefForPointer string
}

// Transform builds the lint-clean Idea body from a parsed seed. The
// seed's H1 becomes `# Idea: <title>`, frontmatter is dropped, the prose
// body folds into ## Context, every other canonical section gets its
// HTML-comment prompt (via idea.Scaffold), and the verdict is carried
// forward per opts.VerdictMode.
func Transform(seed SeedContent, opts TransformOptions) ([]byte, error) {
	title := strings.TrimSpace(seed.Title)
	ctx := strings.TrimSpace(seed.Body)

	// Carry the verdict forward, appending to the Context body so the
	// result stays a lint-clean Idea (no extra non-canonical H2).
	if vd := carryVerdict(seed, opts); vd != "" {
		if ctx == "" {
			ctx = vd
		} else {
			ctx = ctx + "\n\n" + vd
		}
	}

	scaffoldOpts := idea.ScaffoldOptions{
		Slug:    opts.Slug,
		Title:   title,
		Owner:   opts.Owner,
		Date:    opts.Date,
		Context: ctx,
	}
	return ideaScaffoldFn(scaffoldOpts)
}

// carryVerdict produces the verdict text to fold into ## Context per the
// carry-forward mode. Returns "" when the seed carries no verdict or the
// mode is drop.
func carryVerdict(seed SeedContent, opts TransformOptions) string {
	if strings.TrimSpace(seed.Verdict) == "" {
		return ""
	}
	switch opts.VerdictMode {
	case VerdictDrop:
		return ""
	case VerdictFull:
		// Demote the seed's `## Consilium Verdict` heading to `###` so it
		// nests under ## Context without introducing a non-canonical H2.
		return demoteVerdictHeading(seed.Verdict)
	default: // pointer (and unset → default pointer)
		ref := strings.TrimSpace(opts.SeedRefForPointer)
		if ref == "" {
			ref = "the originating seed"
		}
		return "Consilium verdict carried from " + ref + "."
	}
}

// demoteVerdictHeading rewrites the leading `## Consilium Verdict` line of
// a full verdict section to `### Consilium Verdict` so it can nest inside
// the Idea's ## Context section.
func demoteVerdictHeading(verdict string) string {
	lines := strings.Split(verdict, "\n")
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			lines[i] = "### " + strings.TrimSpace(strings.TrimPrefix(t, "## "))
			break
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

// ideaScaffoldFn is a seam for testing the scaffold step.
var ideaScaffoldFn = idea.Scaffold
