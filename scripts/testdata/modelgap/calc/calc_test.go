package calc

import "testing"

func TestSum(t *testing.T) {
	if got := Sum([]int{1, 2, 3}); got != 6 {
		t.Fatalf("Sum = %d", got)
	}
}

func TestDouble(t *testing.T) {
	if got := Double(21); got != 42 {
		t.Fatalf("Double = %d", got)
	}
}
