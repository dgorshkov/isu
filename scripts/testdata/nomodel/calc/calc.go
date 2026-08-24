// Package calc is fixture code for a module that has no internal/model at all,
// which is the shape of this repository until M1-S2 lands.
package calc

// Sum adds every element of ns.
func Sum(ns []int) int {
	total := 0
	for _, n := range ns {
		total += n
	}
	return total
}
