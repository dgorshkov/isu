// Package calc is fixture code that is fully covered, so that the fixture's
// overall coverage clears the floor while internal/model does not.
package calc

// Sum adds every element of ns.
func Sum(ns []int) int {
	total := 0
	for _, n := range ns {
		total += n
	}
	return total
}

// Double returns n twice over.
func Double(n int) int {
	return n * 2
}
