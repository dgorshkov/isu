package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// semverPattern is the official SemVer 2.0.0 regular expression, with the named
// capture groups removed so that it compiles under RE2.
//
// See https://semver.org/#is-there-a-suggested-regular-expression-regex-to-check-a-semver-string
const semverPattern = `^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)` +
	`(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?` +
	`(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`

func TestVersionCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"--version"}, &stdout, &stderr)

	require.Equal(t, 0, code, "--version must exit zero; stderr: %s", stderr.String())
	require.Empty(t, stderr.String(), "--version must not write to stderr")

	// Only the first line is pinned: M9-S1 stamps commit metadata into the
	// build, and it must land on a later line rather than decorating this one.
	first, _, _ := strings.Cut(stdout.String(), "\n")
	require.Regexp(t, regexp.MustCompile(semverPattern), first)
}

func TestRunStreams(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string // stream the usage text is expected on: "stdout" or "stderr"
	}{
		{name: "help is not an error", args: []string{"--help"}, wantCode: 0, wantOut: "stdout"},
		{name: "short help is not an error", args: []string{"-h"}, wantCode: 0, wantOut: "stdout"},
		{name: "unknown flag", args: []string{"--nope"}, wantCode: 2, wantOut: "stderr"},
		{name: "no arguments", args: nil, wantCode: 2, wantOut: "stderr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := run(tt.args, &stdout, &stderr)

			require.Equal(t, tt.wantCode, code)

			got, other := stdout.String(), stderr.String()
			if tt.wantOut == "stderr" {
				got, other = other, got
			}
			require.Contains(t, got, "Usage:", "usage belongs on %s", tt.wantOut)
			require.Empty(t, other, "nothing belongs on the other stream")
		})
	}
}
