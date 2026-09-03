package rule

import (
	"reflect"
	"testing"
)

func TestParseSource(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    SourceRef
		wantErr bool
	}{
		{name: "lesson", raw: "lesson:kinder-fake-hides-bug", want: SourceRef{Kind: SourceLesson, Value: "kinder-fake-hides-bug", Raw: "lesson:kinder-fake-hides-bug"}},
		{name: "idea", raw: "idea:rules-entity", want: SourceRef{Kind: SourceIdea, Value: "rules-entity", Raw: "idea:rules-entity"}},
		{name: "decision by number", raw: "decision:0012", want: SourceRef{Kind: SourceDecision, Value: "0012", Raw: "decision:0012"}},
		{name: "decision by stem", raw: "decision:0012-payment-rail", want: SourceRef{Kind: SourceDecision, Value: "0012-payment-rail", Raw: "decision:0012-payment-rail"}},
		{name: "https url", raw: "https://example.com/x", want: SourceRef{Kind: SourceURL, Value: "https://example.com/x", Raw: "https://example.com/x"}},
		{name: "http url", raw: "http://example.com/x", want: SourceRef{Kind: SourceURL, Value: "http://example.com/x", Raw: "http://example.com/x"}},

		{name: "empty", raw: "  ", wantErr: true},
		{name: "bare word is never guessed", raw: "kinder-fake-hides-bug", wantErr: true},
		{name: "unknown kind", raw: "memo:something", wantErr: true},
		{name: "missing value", raw: "lesson:", wantErr: true},
		{name: "lesson slug must be kebab", raw: "lesson:Kinder_Fake", wantErr: true},
		{name: "decision must be NNNN", raw: "decision:twelve", wantErr: true},
		{name: "decision must be four digits", raw: "decision:12", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSource(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSource(%q) = %+v, want error", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSource(%q): %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("ParseSource(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
			if got.String() != tc.raw {
				t.Fatalf("SourceRef.String() = %q, want round-trip %q", got.String(), tc.raw)
			}
		})
	}
}

func TestLessonSources(t *testing.T) {
	refs, err := ParseSources([]string{"lesson:a", "decision:0001", "lesson:b", "https://x/y"})
	if err != nil {
		t.Fatalf("ParseSources: %v", err)
	}
	got := LessonSources(refs)
	if want := []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LessonSources = %v, want %v", got, want)
	}
}

func TestParsePromotesTo(t *testing.T) {
	cases := []struct {
		name     string
		value    string
		wantSlug string
		wantOK   bool
		wantErr  bool
	}{
		{name: "absent", value: "", wantOK: false},
		{name: "sentinel", value: Sentinel, wantOK: false},
		{name: "hyphen sentinel", value: "-", wantOK: false},
		{name: "valid", value: "rule:never-mock-backends", wantSlug: "never-mock-backends", wantOK: true},
		{name: "whitespace tolerated", value: "  rule: never-mock-backends ", wantSlug: "never-mock-backends", wantOK: true},
		{name: "wrong prefix", value: "lesson:x", wantErr: true},
		{name: "bare slug", value: "never-mock-backends", wantErr: true},
		{name: "invalid slug", value: "rule:Never_Mock", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slug, ok, err := ParsePromotesTo(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParsePromotesTo(%q) = (%q, %v, nil), want error", tc.value, slug, ok)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePromotesTo(%q): %v", tc.value, err)
			}
			if ok != tc.wantOK || slug != tc.wantSlug {
				t.Fatalf("ParsePromotesTo(%q) = (%q, %v), want (%q, %v)", tc.value, slug, ok, tc.wantSlug, tc.wantOK)
			}
		})
	}
}

func TestPromotesToRef(t *testing.T) {
	if got := PromotesToRef("x-y"); got != "rule:x-y" {
		t.Fatalf("PromotesToRef = %q", got)
	}
}

func TestMergeSources(t *testing.T) {
	cases := []struct {
		name    string
		current []string
		add     []string
		remove  []string
		want    []string
		wantErr bool
	}{
		{name: "add to empty", current: nil, add: []string{"lesson:a"}, want: []string{"lesson:a"}},
		{name: "add preserves order", current: []string{"lesson:a"}, add: []string{"decision:0001"}, want: []string{"lesson:a", "decision:0001"}},
		{name: "remove", current: []string{"lesson:a", "lesson:b"}, remove: []string{"lesson:a"}, want: []string{"lesson:b"}},
		{name: "remove then add", current: []string{"lesson:a"}, remove: []string{"lesson:a"}, add: []string{"lesson:b"}, want: []string{"lesson:b"}},
		{name: "remove everything", current: []string{"lesson:a"}, remove: []string{"lesson:a"}, want: []string{}},

		{name: "adding a duplicate is refused", current: []string{"lesson:a"}, add: []string{"lesson:a"}, wantErr: true},
		{name: "removing an absent source is refused", current: nil, remove: []string{"lesson:a"}, wantErr: true},
		{name: "invalid add is refused", add: []string{"bogus"}, wantErr: true},
		{name: "invalid remove is refused", remove: []string{"bogus"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MergeSources(tc.current, tc.add, tc.remove)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("MergeSources = %v, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("MergeSources: %v", err)
			}
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("MergeSources = %v, want %v", got, tc.want)
			}
		})
	}
}
