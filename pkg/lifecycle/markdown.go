package lifecycle

import "strings"

// StructuralMarkdownMask classifies the source lines that are eligible to
// carry lifecycle-controlled Markdown structure. The Plan parser calls this
// same scanner, so lifecycle writers and Plan readers agree that frontmatter,
// fenced or indented code samples, and HTML comments are not real headings,
// fields, or footers. retainedComment is the one byte-exact HTML comment that
// a caller intentionally gives structural meaning (the Plan stub marker); it
// is empty for ordinary lifecycle artifacts.
//
// It intentionally implements only the small Markdown subset required for
// structural safety rather than trying to render Markdown.
func StructuralMarkdownMask(lines []string, retainedComment string) []bool {
	mask := make([]bool, len(lines))
	// A UTF-8 BOM is permitted only at the beginning of the physical source
	// stream. Recognise it for the opening fence without normalising the line:
	// every later caller retains its original physical line offsets.
	inFrontmatter := len(lines) > 0 && IsLeadingFrontmatterFence(lines[0])
	inComment := false
	var fence markdownFence

	for i, line := range lines {
		if inFrontmatter {
			if i > 0 && IsFrontmatterFence(line) {
				inFrontmatter = false
			}
			continue
		}
		if inComment {
			if close := strings.Index(line, "-->"); close >= 0 {
				inComment = HTMLCommentContinues(line[close+3:])
			}
			continue
		}
		if IsIndentedCode(line) || fence.containsOrOpens(line) {
			continue
		}
		if retainedComment != "" && strings.TrimSpace(line) == retainedComment {
			mask[i] = true
			continue
		}
		if start := strings.Index(line, "<!--"); start >= 0 {
			inComment = HTMLCommentContinues(line[start:])
			continue
		}
		mask[i] = true
	}
	return mask
}

func IsFrontmatterFence(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "---" || trimmed == "..."
}

func IsLeadingFrontmatterFence(line string) bool {
	return IsFrontmatterFence(strings.TrimPrefix(line, "\ufeff"))
}

func HTMLCommentContinues(fragment string) bool {
	for {
		start := strings.Index(fragment, "<!--")
		if start < 0 {
			return false
		}
		afterStart := fragment[start+len("<!--"):]
		end := strings.Index(afterStart, "-->")
		if end < 0 {
			return true
		}
		fragment = afterStart[end+len("-->"):]
	}
}

func IsIndentedCode(line string) bool {
	spaces := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ':
			spaces++
			if spaces >= 4 {
				return true
			}
		case '\t':
			return true
		default:
			return false
		}
	}
	return spaces >= 4
}

type markdownFence struct {
	marker byte
	length int
}

func (f *markdownFence) containsOrOpens(line string) bool {
	if f.marker != 0 {
		if FenceCloses(line, f.marker, f.length) {
			f.marker, f.length = 0, 0
		}
		return true
	}
	marker, length, ok := fenceOpens(line)
	if !ok {
		return false
	}
	f.marker, f.length = marker, length
	return true
}

func fenceOpens(line string) (byte, int, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' && indent < 3 {
		indent++
	}
	if indent == len(line) || (indent < len(line) && line[indent] == ' ') {
		return 0, 0, false
	}
	marker := line[indent]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}
	length := 0
	for indent+length < len(line) && line[indent+length] == marker {
		length++
	}
	return marker, length, length >= 3
}

func FenceCloses(line string, marker byte, minimumLength int) bool {
	indent := 0
	for indent < len(line) && line[indent] == ' ' && indent < 3 {
		indent++
	}
	if indent == len(line) || (indent < len(line) && line[indent] == ' ') || line[indent] != marker {
		return false
	}
	length := 0
	for indent+length < len(line) && line[indent+length] == marker {
		length++
	}
	return length >= minimumLength && strings.TrimSpace(line[indent+length:]) == ""
}
