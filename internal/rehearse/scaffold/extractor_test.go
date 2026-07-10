package scaffold

import (
	"reflect"
	"testing"
)

func TestExtract_GWT(t *testing.T) {
	body := []string{
		"",
		"Given a workspace with two repos",
		"When the user runs the index command",
		"Then both repos are listed",
		"",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	want := []string{
		"Given a workspace with two repos",
		"When the user runs the index command",
		"Then both repos are listed",
	}
	if !reflect.DeepEqual(got.Text, want) {
		t.Errorf("Text = %v, want %v", got.Text, want)
	}
}

func TestExtract_GWTBold(t *testing.T) {
	body := []string{
		"**Given** a workspace with two repos",
		"**When** the user runs the index command",
		"**Then** both repos are listed",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	if len(got.Text) != 3 {
		t.Errorf("Text length = %d, want 3: %v", len(got.Text), got.Text)
	}
}

func TestExtract_Scenario(t *testing.T) {
	body := []string{
		"Scenario: Happy path",
		"Given a workspace",
		"When something",
		"Then success",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	if len(got.Text) == 0 || got.Text[0] != "Scenario: Happy path" {
		t.Errorf("Text[0] = %q, want Scenario line first: %v", got.Text[0], got.Text)
	}
}

func TestExtract_Fallback(t *testing.T) {
	body := []string{
		"Some free-form description",
		"without any GWT lines",
	}
	got := Extract(body)
	if got.HasGWT {
		t.Error("HasGWT = true, want false")
	}
	if !reflect.DeepEqual(got.Text, body) {
		t.Errorf("Text = %v, want raw body %v", got.Text, body)
	}
}

func TestExtract_ScenarioWithBoldGWT(t *testing.T) {
	body := []string{
		"Scenario: full path",
		"**Given** a condition",
		"**When** an action",
		"**Then** a result",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	if len(got.Text) != 4 {
		t.Errorf("Text length = %d, want 4", len(got.Text))
	}
	if got.Text[0] != "Scenario: full path" {
		t.Errorf("Text[0] = %q, want 'Scenario: full path'", got.Text[0])
	}
}

func TestExtract_ScenarioNoGWT(t *testing.T) {
	body := []string{
		"Scenario: no implementation",
		"This is just a description",
		"without any Given/When/Then",
	}
	got := Extract(body)
	if got.HasGWT {
		t.Error("HasGWT = true, want false")
	}
	if !reflect.DeepEqual(got.Text, body) {
		t.Errorf("Text = %v, want raw body %v", got.Text, body)
	}
}

func TestExtract_SingleWordGWT(t *testing.T) {
	body := []string{
		"Given",
		"When",
		"Then",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	if len(got.Text) != 3 {
		t.Errorf("Text length = %d, want 3", len(got.Text))
	}
}

func TestExtract_StopsAtHeading(t *testing.T) {
	body := []string{
		"Given a precondition",
		"When an action",
		"## Next Section",
		"This line should not be included",
	}
	got := Extract(body)
	if !got.HasGWT {
		t.Error("HasGWT = false, want true")
	}
	if len(got.Text) != 2 {
		t.Errorf("Text length = %d, want 2 (stops at heading): %v", len(got.Text), got.Text)
	}
}
