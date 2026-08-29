// Package gittest is fixture code standing in for the real internal/gittest,
// the test harness, which carries a floor of its own below the product's.
//
// Four of the six branches here are covered, which is 67% — under the harness
// floor and well over nothing, so a fixture that failed for being untested
// rather than under-tested would not prove anything.
package gittest

// Kind names what a fixture path is, standing in for the harness's own
// judgements about what it was handed.
func Kind(s string) string {
	switch s {
	case "issue":
		return "an issue"
	case "comment":
		return "a comment"
	case "attachment":
		return "an attachment"
	case "readme":
		return "an explainer"
	case "":
		return "nothing at all"
	}

	return "something else"
}
