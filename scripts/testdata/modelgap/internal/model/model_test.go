package model

import "testing"

func TestValid(t *testing.T) {
	if !Valid("open") {
		t.Fatal(`Valid("open") = false`)
	}
	if Valid("nonsense") {
		t.Fatal(`Valid("nonsense") = true`)
	}
	// "resolved" is deliberately never exercised: this fixture exists to be
	// rejected by the per-package floor.
}
