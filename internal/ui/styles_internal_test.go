package ui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/model"
)

// Colour is asserted here rather than in a frame, because a frame written into
// a buffer has no colour in it by design: lipgloss reads the profile of what it
// is writing to, and a test's buffer is not a terminal. What a frame can be
// held to is that it is colourless; what the palette can be held to is that the
// two statuses carrying a decision are not drawn as the four that do not.
func TestTheStatusesThatCarryADecisionAreColouredApart(t *testing.T) {
	t.Parallel()

	s := newStyles(nil)

	inProgress := s.forStatus(model.StatusInProgress).GetForeground()
	triage := s.forStatus(model.StatusAwaitingTriage).GetForeground()
	open := s.forStatus(model.StatusOpen).GetForeground()

	require.NotEqual(t, open, inProgress, "somebody is on it, and the row should say so")
	require.NotEqual(t, open, triage, "nobody has accepted it, and the row should say so")
	require.NotEqual(t, inProgress, triage)

	for _, status := range model.Statuses {
		require.Contains(t, s.status, status, "every status in the table has a style")
	}
}

// A status the table does not name is drawn plain rather than not drawn. The
// board has to render the row either way, and `isu check` is where a status
// nobody recognises gets reported.
func TestAStatusThePaletteDoesNotKnowIsDrawnPlain(t *testing.T) {
	t.Parallel()

	s := newStyles(nil)

	require.Equal(t, s.plain, s.forStatus(model.Status("invented")))
}

// A styled string occupies the columns its characters take and not the bytes
// its escape sequences take. Counting the sequences would push every right-hand
// column off the screen the moment colour was switched on — which is the moment
// nobody is running the tests, because the tests run into a buffer.
func TestAnEscapeSequenceOccupiesNoColumns(t *testing.T) {
	t.Parallel()

	const bold = "\x1b[1m"

	require.Equal(t, len("main"), visible(bold+"main"+"\x1b[0m"))
	require.Equal(t, len("isu  main"), visible("isu  "+bold+"main"+"\x1b[0m"))
	require.Equal(t, 0, visible(bold+"\x1b[0m"))
	require.Equal(t, len("plain"), visible("plain"))

	// And a line built out of styled parts still folds at the right column.
	folded := wrap(strings.Repeat(bold+"word"+"\x1b[0m"+" ", 6), 14)
	require.Len(t, folded, 3, "fourteen columns holds two four-letter words")
}
