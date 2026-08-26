// Package config reads .isu.yml, the one configuration file isu has.
//
// The schema in PLAN.md section 1 is the whole schema. An unknown key is a
// validation error rather than a silent no-op, because a typo in a key that is
// ignored is a setting somebody believes is on.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"github.com/dgorshkov/isu/internal/issue"
)

// FileName is where the configuration lives, at the repository root.
const FileName = ".isu.yml"

// The configuration keys.
const (
	KeyPrefix             = "prefix"
	KeyAgents             = "agents"
	KeyDirectTriage       = "direct_triage"
	KeyStaleDays          = "stale_days"
	KeyFetchWarnHours     = "fetch_warn_hours"
	KeyAttachmentMaxBytes = "attachment_max_bytes"
)

// The defaults, for every key that has one.
const (
	DefaultStaleDays          = 7
	DefaultFetchWarnHours     = 24
	DefaultAttachmentMaxBytes = 524288
)

// Config is .isu.yml, parsed, with every default applied.
type Config struct {
	// Prefix is the issue id prefix. It is required: an id is
	// <PREFIX>-<token>, and there is no sensible default for the half that
	// says which project this is.
	Prefix string
	// Agents are the commit authors treated as agents by the owner
	// immutability check in M5-S4.
	Agents []string
	// DirectTriage allows `isu triage --push` to write straight to trunk.
	DirectTriage bool
	// StaleDays is how old a claim has to be before it is stale.
	StaleDays int
	// FetchWarnHours is how old the newest remote ref may be before isu warns
	// that what it is showing you may be behind.
	FetchWarnHours int
	// AttachmentMaxBytes is the per-attachment cap enforced by M5-S2.
	AttachmentMaxBytes int
}

// Default is the configuration of a repository whose .isu.yml sets nothing but
// the prefix it must set.
func Default() Config {
	return Config{
		StaleDays:          DefaultStaleDays,
		FetchWarnHours:     DefaultFetchWarnHours,
		AttachmentMaxBytes: DefaultAttachmentMaxBytes,
	}
}

// StaleAfter is StaleDays as a duration.
func (c Config) StaleAfter() time.Duration {
	return time.Duration(c.StaleDays) * 24 * time.Hour
}

// FetchWarnAfter is FetchWarnHours as a duration.
func (c Config) FetchWarnAfter() time.Duration {
	return time.Duration(c.FetchWarnHours) * time.Hour
}

// NewID returns a fresh id under this repository's prefix. It is the spelling
// PLAN.md's M1-S4 names, over the pure generator in the issue package.
func (c Config) NewID(title, owner string, created time.Time) (string, error) {
	return issue.NewID(c.Prefix, title, owner, created)
}

// Load reads .isu.yml from the given repository root.
func Load(root string) (*Config, error) {
	path := filepath.Join(root, FileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", FileName, err)
	}

	cfg, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return cfg, nil
}

// Parse reads a configuration file. Every problem with it is reported at once:
// a file with three bad values should take one round trip to fix, not three.
func Parse(data []byte) (*Config, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("this is not a YAML mapping: %w", err)
	}

	cfg := Default()

	var p problems

	for _, key := range sortedKeys(raw) {
		value := raw[key]

		switch key {
		case KeyPrefix:
			cfg.Prefix = readString(&p, key, value)
		case KeyAgents:
			cfg.Agents = readStrings(&p, key, value)
		case KeyDirectTriage:
			cfg.DirectTriage = readBool(&p, key, value)
		case KeyStaleDays:
			cfg.StaleDays = readInt(&p, key, value, cfg.StaleDays)
		case KeyFetchWarnHours:
			cfg.FetchWarnHours = readInt(&p, key, value, cfg.FetchWarnHours)
		case KeyAttachmentMaxBytes:
			cfg.AttachmentMaxBytes = readInt(&p, key, value, cfg.AttachmentMaxBytes)
		default:
			p.add(key, "unknown key: "+oneOfTheKeys())
		}
	}

	// Only when nothing has already been said about the prefix: a file that
	// wrote a number there should hear that once, not also hear that the key
	// it did write is missing.
	if !p.has(KeyPrefix) {
		switch {
		case cfg.Prefix == "":
			p.add(KeyPrefix, "required: it is the first half of every issue id")
		case !issue.ValidID(cfg.Prefix):
			p.add(KeyPrefix, fmt.Sprintf(
				"%q cannot be a prefix: it becomes part of every issue's folder name, "+
					"so it is letters, digits, -, _ and .", cfg.Prefix))
		}
	}

	if err := p.err(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Problem is one thing wrong with a configuration file.
type Problem struct {
	// Key is the configuration key at fault.
	Key string
	// Message says what is wrong with it.
	Message string
}

func (p Problem) String() string { return p.Key + ": " + p.Message }

// Error is every problem with one configuration file, sorted by key.
type Error struct {
	Problems []Problem
}

func (e *Error) Error() string {
	if len(e.Problems) == 1 {
		return fmt.Sprintf("1 problem: %s", e.Problems[0])
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d problems:", len(e.Problems))
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  %s", p)
	}

	return b.String()
}

type problems []Problem

func (p *problems) add(key, message string) {
	*p = append(*p, Problem{Key: key, Message: message})
}

// has reports whether something has already been said about this key.
func (p problems) has(key string) bool {
	for _, problem := range p {
		if problem.Key == key {
			return true
		}
	}

	return false
}

func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}

	sort.SliceStable(p, func(a, b int) bool { return p[a].Key < p[b].Key })

	return &Error{Problems: p}
}

// readString reads a string value, naming the key when it is not one.
func readString(p *problems, key string, value any) string {
	s, ok := value.(string)
	if !ok {
		p.add(key, fmt.Sprintf("expected a string, found %s", describe(value)))

		return ""
	}

	return s
}

// readStrings reads a list of strings. A single string is not a list: writing
// one where a list belongs is a mistake worth naming rather than absorbing.
func readStrings(p *problems, key string, value any) []string {
	items, ok := value.([]any)
	if !ok {
		p.add(key, fmt.Sprintf("expected a list of strings, found %s", describe(value)))

		return nil
	}

	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			p.add(key, fmt.Sprintf("entry %d: expected a string, found %s", i+1, describe(item)))
			continue
		}
		out = append(out, s)
	}

	return out
}

func readBool(p *problems, key string, value any) bool {
	b, ok := value.(bool)
	if !ok {
		p.add(key, fmt.Sprintf("expected true or false, found %s", describe(value)))

		return false
	}

	return b
}

// readInt reads a whole number that is not negative. On a bad value the
// default is kept, so that one wrong line does not also produce a cascade of
// nonsense from every check that reads it.
func readInt(p *problems, key string, value any, fallback int) int {
	n, ok := asInt(value)
	if !ok {
		p.add(key, fmt.Sprintf("expected a whole number, found %s", describe(value)))

		return fallback
	}
	if n < 0 {
		p.add(key, fmt.Sprintf("expected a whole number that is not negative, found %d", n))

		return fallback
	}

	return int(n)
}

// asInt normalises the two shapes a YAML integer arrives in.
func asInt(value any) (int64, bool) {
	switch v := value.(type) {
	case uint64:
		return int64(v), true
	case int64:
		return v, true
	default:
		return 0, false
	}
}

// describe names what a value turned out to be, for an error message that
// tells the author what they wrote rather than what a parser calls it.
func describe(value any) string {
	switch v := value.(type) {
	case nil:
		return "nothing"
	case string:
		return fmt.Sprintf("the string %q", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []any:
		return "a list"
	case map[string]any:
		return "a mapping"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// keys is every key the schema defines, in the order PLAN.md documents them.
var keys = []string{
	KeyPrefix, KeyAgents, KeyDirectTriage, KeyStaleDays,
	KeyFetchWarnHours, KeyAttachmentMaxBytes,
}

func oneOfTheKeys() string {
	return "this file's keys are " + strings.Join(keys, ", ")
}

// sortedKeys makes the order problems are found in independent of the order a
// map happened to iterate.
func sortedKeys(raw map[string]any) []string {
	out := make([]string, 0, len(raw))
	for key := range raw {
		out = append(out, key)
	}
	sort.Strings(out)

	return out
}
