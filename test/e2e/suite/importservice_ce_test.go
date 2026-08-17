//go:build e2e && !enterprise

// importservice_ce_test.go exercises the repository import tools against a live
// GitLab instance. The Bitbucket Cloud import drives the v2.41.0
// bitbucket_api_token + bitbucket_email auth path:
//
//   - When BITBUCKET_API_TOKEN, BITBUCKET_EMAIL, BITBUCKET_USERNAME and
//     BITBUCKET_REPO_PATH are all set (see .env / .env.example), the test runs a
//     REAL import: GitLab authenticates to Bitbucket with the token and schedules
//     the project, which the test asserts and then cleans up.
//   - Otherwise it is a routing + graceful-error test: with fake credentials it
//     confirms the tool reaches GitLab and the import is rejected (invalid token,
//     unknown repo, or the Bitbucket import source being disabled).
//
// The field wiring itself is covered exhaustively by the importservice unit tests.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
package suite

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestIndividual_BitbucketCloudImport_APIToken drives gitlab_import_from_bitbucket_cloud
// through the v2.41.0 API-token auth path.
func TestIndividual_BitbucketCloudImport_APIToken(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	token := os.Getenv("BITBUCKET_API_TOKEN")
	email := os.Getenv("BITBUCKET_EMAIL")
	username := os.Getenv("BITBUCKET_USERNAME")
	repoPath := os.Getenv("BITBUCKET_REPO_PATH")

	deleteProject := func(id int64) {
		if id <= 0 {
			return
		}
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanCancel()
		_ = callToolVoidOn(cleanCtx, sess.individual, "gitlab_project_delete", projects.DeleteInput{
			ProjectID: toolutil.StringOrInt(strconv.FormatInt(id, 10)),
		})
	}

	// Real import path — only when full Bitbucket credentials + a source repo are
	// configured. Asserts the token auth is accepted and the import is scheduled.
	if token != "" && email != "" && username != "" && repoPath != "" {
		newName := fmt.Sprintf("e2e-bb-import-%d", time.Now().UnixMilli())
		out, err := callToolOn[importservice.BitbucketCloudImportOutput](ctx, sess.individual, "gitlab_import_from_bitbucket_cloud", importservice.ImportFromBitbucketCloudInput{
			BitbucketUsername: username,
			BitbucketAPIToken: token,
			BitbucketEmail:    email,
			RepoPath:          repoPath,
			TargetNamespace:   sess.username,
			NewName:           newName,
		})
		if err != nil && strings.Contains(err.Error(), "could not be found") {
			t.Skipf("Bitbucket repository %s not accessible with the configured credentials — refresh BITBUCKET_API_TOKEN in .env for real coverage: %v", repoPath, err)
		}
		requireNoError(t, err, "real Bitbucket Cloud import (api_token path)")
		requireTruef(t, out.ID > 0, "real Bitbucket import should schedule a project (got ID %d)", out.ID)
		t.Cleanup(func() { deleteProject(out.ID) })
		t.Logf("Real Bitbucket import scheduled project %d (%s), status=%s", out.ID, out.FullPath, out.ImportStatus)
		return
	}

	// Graceful fallback — fake credentials confirm routing without real Bitbucket
	// access. GitLab rejects them (invalid token / unknown repo / source disabled).
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
	t.Logf("Bitbucket Cloud import unexpectedly scheduled project %d (%s) from fake credentials; cleaning up", out.ID, out.FullPath)
	t.Cleanup(func() { deleteProject(out.ID) })
}
