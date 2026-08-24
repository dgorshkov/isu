// Package model is fixture code standing in for the real internal/model, which
// carries a 100% floor of its own. One branch here is left uncovered.
package model

// Valid reports whether s is a state the fixture model knows.
func Valid(s string) bool {
	switch s {
	case "open":
		return true
	case "resolved":
		return true
	}
	return false
}
