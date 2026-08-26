package issue

import (
	"fmt"
	"strconv"
)

// CurrentSchema is the on-disk format version this build of isu reads and
// writes. Adding an optional key does not move it; changing what an existing
// key means does.
const CurrentSchema = 1

// decodeSchema reads the schema field and refuses a version this build does
// not understand. Refusing beats misparsing: a reader that guesses at a format
// it has never seen writes the guess back.
func decodeSchema(doc *Document) (int, error) {
	raw, ok := doc.Get(KeySchema)
	if !ok {
		return 0, fmt.Errorf("%s: required", KeySchema)
	}

	version, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a version number", KeySchema, raw)
	}

	if version != CurrentSchema {
		return 0, fmt.Errorf("%s: this build of isu reads version %d, found %d",
			KeySchema, CurrentSchema, version)
	}

	return version, nil
}
