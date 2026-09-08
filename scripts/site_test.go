package scripts_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// deploy is the workflow that publishes the website. It is a separate file from
// ci.yml on purpose: publishing needs `pages: write`, and the gates must not
// have it.
var deploy = filepath.Join("..", ".github", "workflows", "site.yml")

// TestTheSiteWorkflowOnlyCallsMakeTargets is TestMakefileTargetsExist for the
// second workflow. The rule is the same one M0-S3 set: a pipeline says where
// work runs, and the work itself is a make target a contributor can run first.
func TestTheSiteWorkflowOnlyCallsMakeTargets(t *testing.T) {
	targets := makeTargets(t)

	called := makeInvocations(t, deploy)
	require.NotEmpty(t, called, "%s runs no make target, so it builds nothing", deploy)

	for _, target := range called {
		require.Truef(t, targets[target],
			"%s calls `make %s`, which the Makefile does not define", deploy, target)
	}
}

// TestTheDeployJobTriggersOnlyOnTrunk is M8-S3's workflow lint.
//
// A site that publishes from a pull request publishes whatever the pull request
// says, to the address people read — and finds out from them. The build half
// runs everywhere; the deploy half is fenced to trunk twice over, by the
// workflow's own triggers and by a condition on the job.
func TestTheDeployJobTriggersOnlyOnTrunk(t *testing.T) {
	body, err := os.ReadFile(deploy)
	require.NoError(t, err)

	text := string(body)

	require.Regexp(t, regexp.MustCompile(`(?m)^\s*branches: \[main\]$`), text,
		"%s does not restrict its push trigger to trunk", deploy)

	before, after, found := strings.Cut(text, "\n  deploy:")
	require.True(t, found, "%s has no deploy job", deploy)

	require.Contains(t, after, `if: github.ref == 'refs/heads/main'`,
		"%s deploys from a ref other than trunk", deploy)

	// The permission that publishes is on the deploy job and nowhere else, so a
	// pull request build cannot be talked into writing to Pages.
	require.NotContains(t, before, "pages: write",
		"%s grants pages: write outside the deploy job", deploy)
}

// TestTheSiteWorkflowPublishesWhatIsCommitted keeps the deploy honest about
// where the bytes come from: it regenerates the site and refuses to publish if
// the result differs from what the branch carries, which is the same gate
// `make test` applies and the reason the site is reviewable as a diff.
func TestTheSiteWorkflowPublishesWhatIsCommitted(t *testing.T) {
	body, err := os.ReadFile(deploy)
	require.NoError(t, err)

	text := string(body)

	require.Contains(t, text, "git diff --exit-code -- web/site",
		"%s does not check the committed site against the built one", deploy)
	require.Contains(t, text, "path: web/site",
		"%s publishes something other than web/site", deploy)
}
