package site

import (
	"fmt"
	"strconv"
	"strings"
)

// Documentation in this project is executable, and this is what executes it.
//
// PLAN.md M8-S1 asks that every command in the content plan run and produce the
// output the document claims, and M8-S3 asks the same of every fenced shell
// block in docs/ — "documentation that does not execute is documentation that
// rots, and these docs will be read by agents". One extractor answers both.
//
// The grammar is deliberately small:
//
//	```console
//	$ isu board
//	main · 7 issues · remote refs 3h ago
//	```
//
// A line starting `$ ` is a command; every line after it, until the next
// command or the end of the block, is the output the document claims that
// command printed. A command followed by no lines is run and its status
// checked, and its output is not asserted — which is how a document quotes a
// command without quoting a screenful.
//
// Two rules keep the grammar from being dodged. A command must begin with
// `isu`, so a document cannot ask this to run something else; and a `$ isu`
// line outside a console fence fails the extraction, so a sample cannot escape
// being run by changing its fence.

// Fence is the info string of a block this package executes.
const Fence = "console"

// Command is one command in a console block, and what the document claims it
// printed.
type Command struct {
	// Args is the command line after `isu`.
	Args []string
	// Want is the output the document claims, or empty when it claims none.
	Want string
	// Asserted says the document claimed an output, so the empty string is a
	// claim like any other rather than an absence of one.
	Asserted bool
	// Exit is the status the document says the command leaves. It is 0 unless
	// the fence says otherwise: ```console exit=1.
	Exit int
	// Line is where the command sits in the document, for a message somebody
	// can act on.
	Line int
}

// String is the command as a reader types it.
func (c Command) String() string { return "isu " + strings.Join(c.Args, " ") }

// Block is one fenced console block.
type Block struct {
	// Line is the fence's own line, 1-based.
	Line int
	// Commands are the commands in it, in order.
	Commands []Command
}

// fenced is one fenced block of any kind, which is what the scanner sees before
// anything decides whether it is a console block.
type fenced struct {
	info  string
	line  int
	lines []string
}

// scanFences splits a document into its fenced blocks and the lines outside
// them. It is the one place that understands a fence, so the console grammar
// above and the markdown renderer below cannot disagree about where one ends.
func scanFences(doc string) (blocks []fenced, outside []int) {
	var (
		open   *fenced
		marker string
	)

	for i, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case open == nil && strings.HasPrefix(trimmed, "```"):
			marker = fenceMarker(trimmed)
			open = &fenced{info: strings.TrimPrefix(trimmed, marker), line: i + 1}
		case open != nil && trimmed == marker:
			blocks = append(blocks, *open)
			open = nil
		case open != nil:
			open.lines = append(open.lines, line)
		default:
			outside = append(outside, i)
		}
	}

	if open != nil {
		blocks = append(blocks, *open)
	}

	return blocks, outside
}

// fenceMarker is the run of backticks that opened a fence, since a block
// containing backticks is closed only by a marker at least as long.
func fenceMarker(line string) string {
	n := 0
	for n < len(line) && line[n] == '`' {
		n++
	}

	return line[:n]
}

// Blocks are the console blocks in a document.
//
// The error is a document this package refuses to read: a `$ isu` line loose in
// the prose, a command that is not isu, or a fence whose info string it does
// not understand. Each of those is a sample that would otherwise never run.
func Blocks(doc string) ([]Block, error) {
	fences, outside := scanFences(doc)

	lines := strings.Split(doc, "\n")
	for _, i := range outside {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "$ isu") {
			return nil, fmt.Errorf(
				"line %d: a `$ isu` line outside a ```%s fence is a sample nothing runs",
				i+1, Fence)
		}
	}

	var blocks []Block

	for _, f := range fences {
		if !isConsole(f.info) {
			// A prompt in any other fence is the same dodge as a prompt in the
			// prose: it looks like a transcript and nothing ever ran it. An
			// illustrative block is written without one.
			for i, line := range f.lines {
				if strings.HasPrefix(line, "$ ") {
					return nil, fmt.Errorf(
						"line %d: a `$ ` prompt in a ```%s fence is a transcript nothing runs",
						f.line+i+1, strings.TrimSpace(f.info))
				}
			}

			continue
		}

		block, err := parseBlock(f)
		if err != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}

	return blocks, nil
}

// isConsole reports whether an info string opens a block this package runs.
func isConsole(info string) bool {
	fields := strings.Fields(info)

	return len(fields) > 0 && fields[0] == Fence
}

// parseBlock reads one console block's commands and the output claimed for each.
func parseBlock(f fenced) (Block, error) {
	exit, err := fenceExit(f)
	if err != nil {
		return Block{}, err
	}

	block := Block{Line: f.line}

	for i, line := range f.lines {
		if !strings.HasPrefix(line, "$ ") {
			if len(block.Commands) == 0 {
				return Block{}, fmt.Errorf(
					"line %d: a console block starts with a command, not with output",
					f.line+i+1)
			}

			last := &block.Commands[len(block.Commands)-1]
			last.Want += line + "\n"
			last.Asserted = true

			continue
		}

		args, err := words(strings.TrimPrefix(line, "$ "))
		if err != nil {
			return Block{}, fmt.Errorf("line %d: %w", f.line+i+1, err)
		}

		if len(args) == 0 || args[0] != "isu" {
			return Block{}, fmt.Errorf(
				"line %d: this runs isu and nothing else, and that line is %q",
				f.line+i+1, line)
		}

		block.Commands = append(block.Commands,
			Command{Args: args[1:], Exit: exit, Line: f.line + i + 1})
	}

	if len(block.Commands) == 0 {
		return Block{}, fmt.Errorf("line %d: an empty console block proves nothing", f.line)
	}

	return block, nil
}

// fenceExit reads the status a fence says its commands leave.
func fenceExit(f fenced) (int, error) {
	for i, field := range strings.Fields(f.info) {
		if i == 0 {
			continue
		}

		value, ok := strings.CutPrefix(field, "exit=")
		if !ok {
			return 0, fmt.Errorf("line %d: %q is not something a console fence says", f.line, field)
		}

		exit, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("line %d: %q is not an exit status", f.line, value)
		}

		return exit, nil
	}

	return 0, nil
}

// words splits a command line, honouring double quotes so that a title with a
// space in it is one argument.
func words(line string) ([]string, error) {
	var (
		out    []string
		token  strings.Builder
		quoted bool
		have   bool
	)

	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
			have = true
		case r == ' ' && !quoted:
			if have {
				out = append(out, token.String())
				token.Reset()

				have = false
			}
		default:
			token.WriteRune(r)

			have = true
		}
	}

	if quoted {
		return nil, fmt.Errorf("%q has an unclosed quote", line)
	}

	if have {
		out = append(out, token.String())
	}

	return out, nil
}

// Verify runs every console block in a document against the repository at dir
// and reports the first command whose behaviour is not what the document says.
func Verify(dir, doc string) error {
	blocks, err := Blocks(doc)
	if err != nil {
		return err
	}

	for _, block := range blocks {
		for _, command := range block.Commands {
			if err := verify(dir, command); err != nil {
				return err
			}
		}
	}

	return nil
}

func verify(dir string, command Command) error {
	got := isu(dir, command.Args)

	if got.Exit != command.Exit {
		return fmt.Errorf("line %d: %w", command.Line,
			&ExitError{Sample: got, Want: command.Exit})
	}

	if command.Asserted && strings.TrimRight(got.Output, "\n") !=
		strings.TrimRight(command.Want, "\n") {
		return fmt.Errorf("line %d: %w", command.Line,
			&OutputError{Sample: got, Want: command.Want})
	}

	return nil
}
