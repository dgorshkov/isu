package model

import "testing"

func TestValid(t *testing.T) {
	for _, s := range []string{"open", "resolved"} {
		if !Valid(s) {
			t.Fatalf("Valid(%q) = false", s)
		}
	}
	if Valid("nonsense") {
		t.Fatal(`Valid("nonsense") = true`)
	}
}
