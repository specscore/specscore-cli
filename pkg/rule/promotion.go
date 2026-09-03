package rule

import (
	"fmt"
	"strings"
)

// The Lesson side of the strict lesson<->rule pair.
//
// A promoted Lesson carries `**Promotes To:** rule:<slug>` and the Rule it
// promoted into carries `lesson:<lesson-slug>` in its **Sources:** list. Lint
// checks the pair in both directions, so neither half can be edited away
// without the other being reported — the property that makes "which Lesson
// produced this Rule?" and "did this Lesson ever become a Rule?" both
// answerable from the artifacts alone rather than from an agent's memory.

// LessonPromotesToField is the bold field name written on the Lesson.
const LessonPromotesToField = "Promotes To"

// lessonAnchorFields are the canonical Lesson metadata fields, in order. A
// `**Promotes To:**` line is inserted directly after the last of these that is
// present, which puts it at the end of the Lesson's relation block without
// disturbing the ordered set L-005 validates.
var lessonAnchorFields = []string{
	"Superseded By", "Supersedes", "Duplicate Of", "Legacy Provenance",
	"Classifications", "Owner", "Date", "Status",
}

// SetLessonPromotesTo writes (or rewrites) the Lesson's `**Promotes To:**`
// field to point at ruleSlug. It preserves every other byte of the Lesson.
func SetLessonPromotesTo(lessonPath, ruleSlug string) error {
	return rewriteLessonPromotesTo(lessonPath, PromotesToRef(ruleSlug))
}

// ClearLessonPromotesTo removes the Lesson's `**Promotes To:**` field
// entirely. Writing the em-dash sentinel instead would leave a dangling
// "promoted to nothing" line that reads as a half-finished promotion.
func ClearLessonPromotesTo(lessonPath string) error {
	return rewriteLessonPromotesTo(lessonPath, "")
}

func rewriteLessonPromotesTo(lessonPath, value string) error {
	data, err := osReadFile(lessonPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	existing := -1
	present := map[string]int{}
	for i, raw := range lines {
		name, _, ok := matchBoldField(strings.TrimSpace(raw))
		if !ok {
			continue
		}
		if name == LessonPromotesToField && existing < 0 {
			existing = i
		}
		if _, seen := present[name]; !seen {
			present[name] = i
		}
	}

	switch {
	case value == "" && existing < 0:
		return nil
	case value == "":
		lines = append(lines[:existing], lines[existing+1:]...)
	case existing >= 0:
		lines[existing] = fmt.Sprintf("**%s:** %s", LessonPromotesToField, value)
	default:
		at := -1
		for _, anchor := range lessonAnchorFields {
			if idx, ok := present[anchor]; ok {
				at = idx + 1
				break
			}
		}
		if at < 0 {
			return fmt.Errorf("cannot place **%s:** in %s: no canonical Lesson metadata field found", LessonPromotesToField, lessonPath)
		}
		lines = append(lines, "")
		copy(lines[at+1:], lines[at:])
		lines[at] = fmt.Sprintf("**%s:** %s", LessonPromotesToField, value)
	}
	return WriteFileAtomic(lessonPath, []byte(strings.Join(lines, "\n")))
}
