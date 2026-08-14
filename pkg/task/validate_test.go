package task

import "testing"

func TestValidateSlug(t *testing.T) {
	for _, slug := range []string{"a", "task-1", "one-two-three"} {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", slug, err)
		}
	}
	for _, slug := range []string{"", "Task", "two_words", "two--words", "../task", "task/child", "-task", "task-"} {
		if err := ValidateSlug(slug); err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", slug)
		}
	}
}
