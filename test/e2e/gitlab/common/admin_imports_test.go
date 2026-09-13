//go:build e2e

// admin_imports_test.go ports the external importer scenarios: the GitHub
// repository and gists imports, the Bitbucket Cloud import through both the
// gitlab_admin meta tool and the standalone individual tool, and the Bitbucket
// Server import against the compose fixture.
//
// Every subtest here reaches a third party or a fixture the hermetic run does
// not provision. The GitHub and Bitbucket Cloud imports call the public
// internet, so they are gated on the external-network opt-in and their own
// credentials; the Bitbucket Server import is gated on the Bitbucket fixture.
// A run that opts into none of those skips every subtest with a reason, and
// the importer coverage is reached only where the operator supplies what it
// needs, which is the same gating the old suite carried.

package common

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/importservice"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The GitHub credential key the importer reads, as the repository .env
// carries it.
const settingGitHubToken = "GH_TOKEN"

// importAlreadyCompleted is what GitLab answers a cancel for an import that
// already reached a terminal state, from
// Import::Github::CancelProjectImportService: "The import cannot be canceled
// because it is <status>".
const importAlreadyCompleted = "cannot be canceled because it is"

// The Bitbucket Cloud credential keys the importer reads, as the repository
// .env carries them.
const (
	settingBitbucketAPIToken = "BITBUCKET_API_TOKEN"
	settingBitbucketEmail    = "BITBUCKET_EMAIL"
	settingBitbucketUsername = "BITBUCKET_USERNAME"
	settingBitbucketRepoPath = "BITBUCKET_REPO_PATH"
)

// The Bitbucket Server credential keys, as setup-bitbucket.sh writes them.
const (
	settingBitbucketServerUsername = "BITBUCKET_SERVER_USERNAME"
	settingBitbucketServerToken    = "BITBUCKET_SERVER_TOKEN"
	settingBitbucketServerProject  = "BITBUCKET_SERVER_PROJECT_KEY"
	settingBitbucketServerRepo     = "BITBUCKET_SERVER_REPO_SLUG"
)

// TestAdmin_ExternalImporters drives the four external importer actions
// through the gitlab_admin meta tool, each gated on the credentials and
// network access it needs.
//
// Every subtest builds an Env of its own rather than sharing the parent's,
// because a skip has to end the test that is being skipped: Env.Skipf skips
// the T its Env was built with, so a parent Env skipped from inside a subtest
// ends the whole function from the wrong goroutine, which the testing package
// reports as a failure rather than a skip. Declaring the needs per subtest is
// also what keeps a run that supplies none of these credentials from starting
// a server it will not use.
//
// Replaces: TestMeta_AdminExternalImporters
func TestAdmin_ExternalImporters(t *testing.T) {
	t.Run("GitHubImportAndCancel", func(t *testing.T) {
		e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedExternalNetwork, harness.NeedGitHubToken))
		s := e.On(harness.SurfaceMeta)
		gh := e.Setting(settingGitHubToken)
		repoID, repoName := adminGitHubRepo(e, gh)
		out := harness.Do[importservice.GitHubImportOutput](s, actionAdminImportGitHub, map[string]any{
			"personal_access_token": gh, "repo_id": repoID,
			"target_namespace": e.Runtime().Username, "new_name": e.Name("gh-import"),
		})
		if out.ID == 0 {
			t.Fatalf("import_github answered %+v, want an imported project with an ID", out)
		}
		adminDeferImportedProject(e, out.ID, out.FullPath)
		t.Logf("started GitHub import of %s as project %d", repoName, out.ID)

		// A tiny repository can finish before the cancel lands, which GitLab
		// then refuses with a 400 naming the state; both are the cancel path.
		cancelled, err := harness.Try[importservice.CancelledImportOutput](s, actionAdminImportCancelGitHub, map[string]any{"project_id": out.ID})
		switch {
		case err == nil:
			if cancelled.ID != out.ID {
				t.Errorf("import_cancel_github answered project %d, want the imported %d", cancelled.ID, out.ID)
			}
		case strings.Contains(err.Error(), importAlreadyCompleted):
			t.Logf("the import finished before the cancel landed: %s", firstLine(err.Error()))
		default:
			t.Fatalf("import_cancel_github answered: %v", err)
		}
	})

	t.Run("GistsImport", func(t *testing.T) {
		e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedExternalNetwork, harness.NeedGitHubToken))
		s := e.On(harness.SurfaceMeta)
		gh := e.Setting(settingGitHubToken)
		if _, err := harness.Try[struct{}](s, actionAdminImportGists, map[string]any{"personal_access_token": gh}, harness.For(harness.PurposeTest)); err != nil {
			t.Logf("import_gists answered (GitHub can be degraded): %v", err)
		}
	})

	t.Run("BitbucketCloudImport", func(t *testing.T) {
		e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedExternalNetwork))
		token, email, username, repoPath := requireBitbucketCloud(e)
		s := e.On(harness.SurfaceMeta)
		out := harness.Do[importservice.BitbucketCloudImportOutput](s, actionAdminImportBitbucket, map[string]any{
			"bitbucket_username": username, "bitbucket_api_token": token, "bitbucket_email": email,
			"repo_path": repoPath, "target_namespace": e.Runtime().Username, "new_name": e.Name("bb-import"),
		})
		if out.ID == 0 {
			t.Fatalf("import_bitbucket answered %+v, want an imported project with an ID", out)
		}
		adminDeferImportedProject(e, out.ID, out.FullPath)
		t.Logf("started Bitbucket Cloud import of %s as project %d", repoPath, out.ID)
	})

	t.Run("BitbucketServerImport", func(t *testing.T) {
		e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedBitbucket))
		serverURL, username, token, projectKey, repoSlug := requireBitbucketServer(e)
		s := e.On(harness.SurfaceMeta)
		out := harness.Do[importservice.BitbucketServerImportOutput](s, actionAdminImportBitbucketServer, map[string]any{
			"bitbucket_server_url": serverURL, "bitbucket_server_username": username,
			"personal_access_token": token, "bitbucket_server_project": projectKey,
			"bitbucket_server_repo": repoSlug, "new_namespace": e.Runtime().Username, "new_name": e.Name("bbs-import"),
		})
		if out.ID == 0 {
			t.Fatalf("import_bitbucket_server answered %+v, want an imported project with an ID", out)
		}
		adminDeferImportedProject(e, out.ID, out.FullPath)
		t.Logf("started Bitbucket Server import as project %d", out.ID)
	})
}

// TestBitbucketCloudImport_Individual drives the Bitbucket Cloud import through
// the standalone individual tool, which is the same action the meta tool
// carries reached by its own name.
//
// Replaces: TestIndividual_BitbucketCloudImport_APIToken
func TestBitbucketCloudImport_Individual(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedExternalNetwork))
	token, email, username, repoPath := requireBitbucketCloud(e)
	s := e.On(harness.SurfaceIndividual)

	out := harness.Do[importservice.BitbucketCloudImportOutput](s, actionAdminImportBitbucket, map[string]any{
		"bitbucket_username": username, "bitbucket_api_token": token, "bitbucket_email": email,
		"repo_path": repoPath, "target_namespace": e.Runtime().Username, "new_name": e.Name("bb-import-ind"),
	})
	if out.ID == 0 {
		e.T.Fatalf("import_bitbucket answered %+v, want an imported project with an ID", out)
	}
	adminDeferImportedProject(e, out.ID, out.FullPath)
	e.T.Logf("started Bitbucket Cloud import of %s as project %d through the individual tool", repoPath, out.ID)
}

// requireBitbucketCloud returns the Bitbucket Cloud credentials, skipping when
// any of them is missing. The external-network opt-in is declared as a need by
// every test that calls this, so it is not re-checked here.
func requireBitbucketCloud(e *harness.Env) (token, email, username, repoPath string) {
	e.T.Helper()
	token = e.Setting(settingBitbucketAPIToken)
	email = e.Setting(settingBitbucketEmail)
	username = e.Setting(settingBitbucketUsername)
	repoPath = e.Setting(settingBitbucketRepoPath)
	if token == "" || email == "" || username == "" || repoPath == "" {
		e.Skipf("the Bitbucket Cloud credentials (%s/%s/%s/%s) are not all set",
			settingBitbucketAPIToken, settingBitbucketEmail, settingBitbucketUsername, settingBitbucketRepoPath)
	}
	return token, email, username, repoPath
}

// requireBitbucketServer returns the Bitbucket Server credentials, skipping
// when the fixture was not provisioned.
func requireBitbucketServer(e *harness.Env) (serverURL, username, token, projectKey, repoSlug string) {
	e.T.Helper()
	serverURL = e.Setting("BITBUCKET_SERVER_URL")
	username = e.Setting(settingBitbucketServerUsername)
	token = e.Setting(settingBitbucketServerToken)
	projectKey = e.Setting(settingBitbucketServerProject)
	repoSlug = e.Setting(settingBitbucketServerRepo)
	if serverURL == "" || username == "" || token == "" || projectKey == "" || repoSlug == "" {
		e.Skipf("the Bitbucket Server fixture is not provisioned; set E2E_BITBUCKET=true to run it under Docker")
	}
	return serverURL, username, token, projectKey, repoSlug
}

// adminDeferImportedProject registers permanent deletion of a project an
// importer created, addressed by its numeric id.
func adminDeferImportedProject(e *harness.Env, id int64, path string) {
	e.Defer("imported project "+path, func(ctx context.Context) error {
		return fixture.DeleteProject(ctx, e.Client(), id, path)
	})
}

// githubRepoLookupTimeout bounds the one GitHub API call the import setup
// makes to resolve a repository owned by the token's user.
const githubRepoLookupTimeout = 30 * time.Second

// adminGitHubRepo resolves one repository owned by the GitHub token's user, so
// the import needs no configuration beyond the token. It skips the subtest
// when GitHub is unreachable or owns no repository.
func adminGitHubRepo(e *harness.Env, token string) (int64, string) {
	e.T.Helper()

	ctx, cancel := context.WithTimeout(e.Ctx, githubRepoLookupTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/user/repos?per_page=1&sort=full_name&affiliation=owner", http.NoBody)
	if err != nil {
		e.T.Fatalf("building the GitHub repos request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.Skipf("GitHub is unreachable from this environment: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		e.Skipf("GitHub rejected GH_TOKEN (status %d)", resp.StatusCode)
	}
	var repos []struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		e.T.Fatalf("decoding the GitHub repos response: %v", err)
	}
	if len(repos) == 0 {
		e.Skipf("the GH_TOKEN user owns no repository to import")
	}
	return repos[0].ID, strings.TrimSpace(repos[0].FullName)
}
