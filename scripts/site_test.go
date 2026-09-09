package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// verify is the workflow that checks the website. It is a separate file from
// ci.yml because it is about a different artifact, and it publishes nothing:
// Netlify does that, from netlify.toml.
var verify = filepath.Join("..", ".github", "workflows", "site.yml")

// netlifyConfig is how the site is published. It is a file in this repository
// rather than a form in a dashboard, so that what publishes the site is
// reviewable in the same diff as the site.
var netlifyConfig = filepath.Join("..", "netlify.toml")

// TestTheSiteWorkflowOnlyCallsMakeTargets is TestMakefileTargetsExist for the
// second workflow. The rule is the same one M0-S3 set: a pipeline says where
// work runs, and the work itself is a make target a contributor can run first.
func TestTheSiteWorkflowOnlyCallsMakeTargets(t *testing.T) {
	targets := makeTargets(t)

	called := makeInvocations(t, verify)
	require.NotEmpty(t, called, "%s runs no make target, so it builds nothing", verify)

	for _, target := range called {
		require.Truef(t, targets[target],
			"%s calls `make %s`, which the Makefile does not define", verify, target)
	}
}

// TestTheSiteWorkflowChecksWhatIsCommitted is the reason this workflow exists:
// it regenerates the site from web/CONTENT.md, docs/ and the binary, and fails
// when the result differs from what the branch carries.
//
// That is the same gate `make test` applies, run where a pull request cannot
// skip it, and it is what lets the site be reviewed as a diff.
func TestTheSiteWorkflowChecksWhatIsCommitted(t *testing.T) {
	body, err := os.ReadFile(verify)
	require.NoError(t, err)

	require.Contains(t, string(body), "git diff --exit-code -- web/site",
		"%s does not check the committed site against the built one", verify)
}

// TestTheSiteWorkflowCannotPublishAnything is M8-S3's workflow lint, in the
// form the answer took.
//
// The story asks for a test that the deploy job triggers only on trunk. Netlify
// publishes this site, so there is no deploy job to fence — and the assertion
// that replaces it is stronger rather than weaker: this workflow holds no write
// permission at all, so nothing it runs on a pull request from anywhere can
// reach the address people read.
//
// **What moved out of this repository with the deploy job is the guarantee that
// production comes from trunk.** That is now Netlify's production-branch
// setting, and no test here can see it. netlify.toml pins everything that can
// be pinned in a file; the branch is not one of them.
func TestTheSiteWorkflowCannotPublishAnything(t *testing.T) {
	body, err := os.ReadFile(verify)
	require.NoError(t, err)

	text := string(body)

	require.Regexp(t, regexp.MustCompile(`(?m)^permissions:\n  contents: read\n`), text,
		"%s does not declare read-only permissions", verify)

	for _, line := range strings.Split(text, "\n") {
		require.NotContains(t, strings.TrimSpace(line), ": write",
			"%s grants a write permission; it verifies the site and never publishes it", verify)
	}

	require.NotContains(t, text, "deploy-pages",
		"%s deploys the site; Netlify publishes it, from netlify.toml", verify)
}

// TestNetlifyPublishesTheDirectoryTheBuildProduces keeps the publisher and the
// build in step.
//
// `make site` writes web/site and nothing else is the site, so a publish
// directory that named anything else would serve the repository — the source
// markdown, the Go, this test — at the address the landing page is meant to be
// at. It is a form in a dashboard on most projects, which is exactly why it is
// a file here.
func TestNetlifyPublishesTheDirectoryTheBuildProduces(t *testing.T) {
	body, err := os.ReadFile(netlifyConfig)
	require.NoError(t, err, "%s is how the site is published", netlifyConfig)

	text := string(body)

	require.Regexp(t, regexp.MustCompile(`(?m)^\s*publish = "web/site"$`), text,
		"%s publishes something other than the directory `make site` writes", netlifyConfig)

	// The site is committed, so there is nothing for the publisher to build —
	// and a build command would put this deploy behind a Go toolchain in
	// somebody else's build image. The workflow above is what proves the
	// committed bytes are the built ones.
	require.NotRegexp(t, regexp.MustCompile(`(?m)^\s*command = `), text,
		"%s runs a build; the site is committed and CI is what verifies it", netlifyConfig)

	// And nothing may rewrite the bytes on the way out. Netlify's Pretty URLs
	// post-processing is on by default and serves every page with its internal
	// links rewritten — `docs/json.html` to `/docs/json`, double quotes to
	// single — so the gates that run over web/site had never seen a byte a
	// reader was served.
	//
	// Both keys, because `skip_processing` alone did not stop it — 13,233 bytes
	// served against 13,283 committed on the preview for 32cbc65 — and
	// `pretty_urls = false` did: every page on the preview for d9a43e6 is byte
	// for byte what `make site` wrote. This asserts the file says what it means
	// to say, and no more than that: a test here cannot fetch a deploy, so
	// nothing in this repository holds the delivery to it.
	for _, want := range []string{
		`(?m)^\[build\.processing\]\n\s*skip_processing = true$`,
		`(?m)^\[build\.processing\.html\]\n\s*pretty_urls = false$`,
	} {
		require.Regexp(t, regexp.MustCompile(want), text,
			"%s lets the publisher rewrite the pages CI just verified", netlifyConfig)
	}
}
