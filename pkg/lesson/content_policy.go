package lesson

import (
	"fmt"
	"regexp"
	"strings"
)

var unsafeContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:authorization|bearer|token|password|api[_-]?key)\b\s*[:=]`),
	regexp.MustCompile(`(?i)gh[pousr]_[a-z0-9_]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`-----BEGIN (?:[A-Z ]+ )?PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`),
	regexp.MustCompile(`(?:^|[\s(])\+[0-9][0-9 ()-]{7,}[0-9](?:$|[\s).,])`),
	regexp.MustCompile(`(?m)(?:^|[\s"'])/(?:Users|home)/[^/\s"']+(?:/|$)`),
	regexp.MustCompile(`(?i)\b(?:original|raw|user|agent)\s+(?:prompt|log|diff|transcript)\s*:`),
}

// ValidateSafeContent enforces the occurrence schema's content policy for all
// committed Lesson strings. Errors deliberately identify only the field and
// policy class; unsafe values are never echoed into stderr or logs.
func ValidateSafeContent(field, value string) error {
	for _, pattern := range unsafeContentPatterns {
		if pattern.MatchString(value) {
			return fmt.Errorf("%s rejected by committed-content policy", field)
		}
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s contains a NUL byte", field)
	}
	return nil
}
