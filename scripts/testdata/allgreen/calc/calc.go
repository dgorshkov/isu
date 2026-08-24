// Package calc is fixture code for the module that meets both floors.
package calc

// Sum adds every element of ns.
func Sum(ns []int) int {
	total := 0
	for _, n := range ns {
		total += n
	}
	return total
}
