package calc

import "testing"

func TestClassifyNegative(t *testing.T) {
	if got := Classify(-1); got != "negative" {
		t.Fatalf("Classify(-1) = %q", got)
	}
}
