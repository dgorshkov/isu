package gittest

import "testing"

func TestKind(t *testing.T) {
	if Kind("issue") != "an issue" {
		t.Fatal(`Kind("issue")`)
	}
	if Kind("comment") != "a comment" {
		t.Fatal(`Kind("comment")`)
	}
	if Kind("nonsense") != "something else" {
		t.Fatal(`Kind("nonsense")`)
	}
	// "attachment", "readme" and "" are deliberately never exercised: this
	// fixture exists to be rejected by the harness floor.
}
