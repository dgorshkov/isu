package issue

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"time"
)

// idAlphabet is Crockford base32, lowercased: i, l, o and u are absent, so
// nothing in an id is ambiguous read aloud, typed from a screenshot or written
// on a whiteboard.
const idAlphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// TokenLength is how many alphabet characters an id carries after its prefix.
// Six characters of a 32-symbol alphabet is thirty bits.
const TokenLength = 6

// entropyBytes is how much randomness goes into the hash. It is what makes
// allocation need no coordination: two clones creating an issue at the same
// moment collide on thirty bits of hash, not on a counter they both read as
// the same number.
const entropyBytes = 8

// randRead is crypto/rand.Read, named here so that the one failure path in
// this file has a test rather than a comment saying it cannot happen.
var randRead = rand.Read

// NewToken returns the token half of a new id.
//
// It is pure in the way that matters: it reads no file, opens no socket, takes
// no lock and knows about no other issue. Sequential `max + 1` cannot see the
// issues sitting on unmerged branches, so under any real load it hands out the
// same number twice; hashing the issue's own fields with fresh randomness
// needs nobody's agreement.
//
// It therefore cannot detect a collision either. Regenerating on one is
// `isu new`'s job in M4-S3, because detecting one means loading the repository
// and the git layer does not exist until M2.
func NewToken(title, owner string, created time.Time) (string, error) {
	entropy := make([]byte, entropyBytes)
	if _, err := randRead(entropy); err != nil {
		return "", fmt.Errorf("reading random bytes for a new id: %w", err)
	}

	return token(title, owner, created, entropy), nil
}

// NewID returns prefix, a hyphen, and a fresh token.
//
// PLAN.md names this NewID(title, owner, created). The prefix argument is
// added because <PREFIX>-<token> is what an id is, and the config that carries
// the prefix arrives in this same story; config.Config.NewID is the spelling
// the plan describes.
func NewID(prefix, title, owner string, created time.Time) (string, error) {
	tok, err := NewToken(title, owner, created)
	if err != nil {
		return "", err
	}

	return prefix + "-" + tok, nil
}

// token hashes the issue's identity with the given entropy and renders the
// first thirty bits of the digest in the alphabet above.
//
// The fields are separated by a NUL rather than run together, so that a title
// ending in the owner's name cannot hash to the same digest as a shorter title
// and a longer owner.
func token(title, owner string, created time.Time, entropy []byte) string {
	h := sha256.New()
	for _, field := range []string{title, owner, created.Format(time.DateOnly)} {
		h.Write([]byte(field))
		h.Write([]byte{0})
	}
	h.Write(entropy)

	sum := h.Sum(nil)
	bits := uint32(sum[0])<<24 | uint32(sum[1])<<16 | uint32(sum[2])<<8 | uint32(sum[3])
	bits >>= 32 - TokenLength*5

	out := make([]byte, TokenLength)
	for i := range out {
		out[i] = idAlphabet[(bits>>(5*(TokenLength-1-i)))&31]
	}

	return string(out)
}
