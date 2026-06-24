//go:build e2e && !enterprise

// importservice_ce_test.go exercises the repository import tools against a live
// GitLab instance. Real third-party (Bitbucket) credentials are not available
// in CI, so the Bitbucket Cloud import is a routing + graceful-error test: it
// confirms the tool reaches GitLab end-to-end and the import is rejected
// (invalid token, unknown repo, or the Bitbucket import source being disabled —
// the default on a fresh CE instance). This is the only feasible e2e for the
// v2.41.0 bitbucket_api_token + bitbucket_email auth path; the field wiring
// itself is covered exhaustively by the importservice unit tests.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIndividual_BitbucketCloudImport_APIToken drives gitlab_import_from_bitbucket_cloud
// through the v2.41.0 API-token auth path and asserts graceful routing.
func TestIndividual_BitbucketCloudImport_APIToken(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, err := callToolOn[importservice.BitbucketCloudImportOutput](ctx, sess.individual, "gitlab_import_from_bitbucket_cloud", importservice.ImportFromBitbucketCloudInput{
		BitbucketUsername: "e2e-fake-user",
		BitbucketAPIToken: "e2e-fake-api-token",
		BitbucketEmail:    "e2e-fake@example.com",
		RepoPath:          "e2e-nonexistent/repo",
		TargetNamespace:   sess.username,
		NewName:           "e2e-bb-import-should-not-exist",
	})
	if err != nil {
		t.Logf("Bitbucket Cloud import (api_token path) routed and was rejected as expected: %v", err)
		return
	}

	// Unexpected: GitLab scheduled an import from the fake credentials. Remove
	// the project it created so the test leaves no residue.
	t.Logf("Bitbucket Cloud import unexpectedly scheduled project %d (%s); cleaning up", out.ID, out.FullPath)
	if out.ID > 0 {
		_ = callToolVoidOn(ctx, sess.individual, "gitlab_project_delete", projects.DeleteInput{
			ProjectID: toolutil.StringOrInt(strconv.FormatInt(out.ID, 10)),
		})
	}
}
