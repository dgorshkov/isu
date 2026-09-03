package github

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The dump is assembled from raw JSON this package decoded, so in the product
// it cannot fail to re-encode. It is still an error return rather than a
// discarded one, because handing back half a dump is the failure that would
// then be somebody's import.
func TestADumpThatCannotBeWrittenSaysSo(t *testing.T) {
	t.Parallel()

	_, err := encodeDump("acme/widgets", []Issue{{
		Number:   1,
		Comments: json.RawMessage("not json"),
	}})

	require.Error(t, err)
	require.Contains(t, err.Error(), "writing the dump")
}
