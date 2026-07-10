package runner

import (
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

func TestBag_MergePreservesInsertionOrder(t *testing.T) {
	bag := NewBag()
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}})
	bag.Merge([]blocks.Capture{{Name: "token", Value: "t-1"}})

	want := []blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}, {Name: "token", Value: "t-1"}}
	if got := bag.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %v, want %v", got, want)
	}
}

func TestBag_MergeUpdatesInPlaceKeepingOrder(t *testing.T) {
	bag := NewBag()
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}})
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "7"}})

	want := []blocks.Capture{{Name: "uid", Value: "7"}, {Name: "name", Value: "alice"}}
	if got := bag.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %v, want %v", got, want)
	}
}

func TestBag_MapReturnsFinalState(t *testing.T) {
	bag := NewBag()
	if got := bag.Map(); got == nil || len(got) != 0 {
		t.Errorf("empty bag Map() = %v, want empty non-nil map", got)
	}
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "42"}})
	got := bag.Map()
	if !reflect.DeepEqual(got, map[string]string{"uid": "42"}) {
		t.Errorf("Map() = %v", got)
	}
	// The map is a copy: mutating it must not touch the bag.
	got["uid"] = "mutated"
	if bag.Map()["uid"] != "42" {
		t.Error("Map() exposed the bag's internal state")
	}
}

func TestBag_InterpolateReplacesKnownVariables(t *testing.T) {
	bag := NewBag()
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}})

	got, err := bag.Interpolate(`SELECT * FROM users WHERE id = {{uid}} AND name = '{{name}}' -- {{uid}} again`)
	if err != nil {
		t.Fatalf("Interpolate error: %v", err)
	}
	want := `SELECT * FROM users WHERE id = 42 AND name = 'alice' -- 42 again`
	if got != want {
		t.Errorf("Interpolate() = %q, want %q", got, want)
	}
}

func TestBag_InterpolateUnknownVariableFailsNamingIt(t *testing.T) {
	bag := NewBag()
	bag.Merge([]blocks.Capture{{Name: "uid", Value: "42"}})

	_, err := bag.Interpolate("id = {{uid}} but {{missing}} is unknown")
	if err == nil {
		t.Fatal("Interpolate accepted an unknown variable")
	}
	if !strings.Contains(err.Error(), "unknown variable {{missing}}") {
		t.Errorf("error does not name the variable: %v", err)
	}
}

func TestBag_InterpolateLeavesNonPlaceholderBracesAlone(t *testing.T) {
	bag := NewBag()
	for _, text := range []string{
		"awk '{{print $1}} }'", // inner token is not a bare variable name
		"a {{9lives}} b",       // names cannot start with a digit
		"{ single } {{}} {{ spaced }}",
	} {
		got, err := bag.Interpolate(text)
		if err != nil {
			t.Errorf("Interpolate(%q) error: %v", text, err)
		}
		if got != text {
			t.Errorf("Interpolate(%q) = %q, want unchanged", text, got)
		}
	}
}
