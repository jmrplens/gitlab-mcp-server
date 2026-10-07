//go:build e2e

// mcp_scopes_read_api_test.go holds what a token carrying read_api and not
// api is served to GitLab's own answers, where that departs from the
// read-only surface it used to be served (ADR-0026). mcp_scopes_test.go shows
// the narrowing from the listing and the refusals; this file calls the
// actions the narrowing now serves and that GitLab is meant to accept from
// read_api, so a wrong reading of GitLab's scope rules fails here as GitLab's
// own refusal rather than passing as a listing that only looks right.

package common

import (
	"os"
	"path/filepath"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accesstokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/packages"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/pipelinetriggers"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// readAPIRunFixture is what the read_api run scenario works on: a project
// carrying a pipeline configuration, a generic package published into it and
// a trigger token for it, and a token carrying read_api alone for a user who
// reports on the project.
type readAPIRunFixture struct {
	project fixture.Project
	token   fixture.Token
	trigger fixture.PipelineTrigger
	pkg     string
}

// TestScope_ReadAPIToken_RunsWhatGitLabAcceptsFromIt holds three of the five
// actions a read_api token is now served although the catalog classifies them
// as writes to GitLab's own answers, on every surface (ADR-0026): the package
// download, which reads the registry with GETs and writes a local file; and
// the trigger run and the runner deletion by token, which GitLab
// authenticates by the trigger or runner token passed as a parameter and
// never by the token the session holds. It verifies the runner by its token
// before deleting it, a read GitLab authenticates the same way, which a
// read_api token was served already. Each is called from a session on a
// read_api token and answers as it would for any token. The package and the
// trigger are made by the run's own token, and the runner per surface, since
// deleting it by its token ends it. The runner registration, the fifth, is
// not driven: registration tokens are off by default on 19.x.
func TestScope_ReadAPIToken_RunsWhatGitLabAcceptsFromIt(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) readAPIRunFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("scope-run"))
		fixture.CIFile(e, project)
		user := fixture.NewUser(e, "scope-run")
		fixture.AddProjectMember(e, project, user, gl.ReporterPermissions)
		name := e.Name("scope-pkg")
		publishPackage(e, e.On(harness.SurfaceDynamic), project, name)
		return readAPIRunFixture{
			project: project,
			token:   fixture.NewToken(e, user, scopeReadAPI),
			trigger: fixture.NewPipelineTrigger(e, project),
			pkg:     name,
		}
	}, func(e *harness.Env, surface harness.Surface, f readAPIRunFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		params := map[string]any{"project_id": f.project.IDParam()}

		outputPath := filepath.Join(e.T.TempDir(), "downloaded.txt")
		downloaded := harness.Do[packages.DownloadOutput](s, actionPackageDownload, withParams(params, map[string]any{
			"package_name": f.pkg, "package_version": packageVersion, "file_name": packageFileName, "output_path": outputPath,
		}))
		if content, err := os.ReadFile(outputPath); err != nil || downloaded.Size != int64(len(packageFileContent)) || string(content) != packageFileContent {
			e.T.Errorf("package download on a read_api token answered %+v and wrote %q (%v), want %q", downloaded, content, err, packageFileContent)
		}

		ran := harness.Do[pipelinetriggers.RunOutput](s, actionPipelineTriggerRun, withParams(params, map[string]any{
			"ref": f.project.DefaultBranch, "token": f.trigger.Token,
		}))
		if ran.ID == 0 || ran.Ref != f.project.DefaultBranch {
			e.T.Errorf("trigger run on a read_api token answered %+v, want a pipeline on %s", ran, f.project.DefaultBranch)
		}

		runner := fixture.NewProjectRunner(e, f.project)
		harness.DoVoid(s, actionRunnerVerify, map[string]any{"token": runner.Token})
		harness.DoVoid(s, actionRunnerDeleteByToken, map[string]any{"token": runner.Token})
		gone := harness.Refused(e.On(surface), actionRunnerGet, map[string]any{"runner_id": runner.ID}, harness.FailureNotFound)
		e.T.Logf("the runner a read_api session deleted by its token is gone: %s", firstLine(gone))
	})
}

// TestScope_ReadAPIToken_RevokesItself holds a fourth to GitLab: DELETE
// /personal_access_tokens/self accepts every scope a personal access token can
// carry, so a session on a read_api token is served the self-revocation and
// the revocation ends that token. A token is minted per surface, since each
// run ends the one it holds, and the administrator's listing is where the
// revocation shows, the revoked credential reading nothing any more.
func TestScope_ReadAPIToken_RevokesItself(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.User {
		return fixture.NewUser(e, "scope-revoke")
	}, func(e *harness.Env, surface harness.Surface, user fixture.User) {
		token := fixture.NewToken(e, user, scopeReadAPI)
		revoking := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Private: true})
		if !revoking.Serves(actionAccessTokenPersonalRevokeSelf) {
			e.T.Fatalf("a %s session on a read_api token does not serve %s, which GitLab accepts from every scope", surface, actionAccessTokenPersonalRevokeSelf)
		}
		harness.DoVoid(revoking, actionAccessTokenPersonalRevokeSelf, nil)

		active := harness.Do[accesstokens.ListOutput](e.On(surface), actionAccessTokenPersonalList, map[string]any{"user_id": user.ID, "state": tokenStateActive})
		if containsID(accessTokenIDs(active.Tokens), token.ID) {
			e.T.Errorf("token %d is still listed as active after a read_api session revoked it", token.ID)
		}
	})
}
