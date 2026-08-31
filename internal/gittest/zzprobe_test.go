package gittest_test

import (
	"errors"
	"fmt"
	"testing"
)

// Probe 1: Fatal fidelity — testing.T.Fatal uses Sprintln, the wrapper uses Sprint.
func TestProbeFatalFormatting(t *testing.T) {
	err := errors.New("boom")
	t.Logf("real T.Fatal would log:   %q", fmt.Sprintln("gittest:", err))
	t.Logf("recordingTB.Fatal records: %q", fmt.Sprint("gittest:", err))
	t.Logf("two strings: real=%q wrapper=%q",
		fmt.Sprintln("a", "b"), fmt.Sprint("a", "b"))
}

// Probe 2: does Helper() attribution survive the promoted-interface wrapper?
type probeTB struct{ testing.TB }

func helperCaller(tb testing.TB) { tb.Helper(); tb.Errorf("attributed here?") }

// Probe 3: what a FailNow on the un-intercepted path does inside refusal's recover.
func TestProbeFailNowThroughRecover(t *testing.T) {
	reached := false
	func() {
		defer func() {
			if raised := recover(); raised != nil {
				t.Logf("recovered %v", raised)
			}
			t.Logf("deferred ran; recover() was nil during Goexit")
		}()
		go func() {}()
		// simulate testify require against the counterfeit TB
		func() {
			defer func() { reached = true }()
		}()
	}()
	_ = reached
}
