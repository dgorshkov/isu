// Package calc is fixture code for the coverage gate's own tests. Most of it is
// deliberately left untested so that the gate has something to reject.
package calc

// Classify names the sign and parity of n.
func Classify(n int) string {
	if n < 0 {
		return "negative"
	}
	if n == 0 {
		return "zero"
	}
	if n%2 == 0 {
		return "even"
	}
	return "odd"
}

// Sum adds the first n natural numbers. Nothing tests it, which is the point.
func Sum(n int) int {
	total := 0
	for i := 1; i <= n; i++ {
		total += i
	}
	return total
}
