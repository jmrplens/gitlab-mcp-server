//go:build e2e

// fine_grained_tokens_test.go covers a session on a fine-grained personal
// access token (issue 952): the binary reads the token's grant and the
// instance's version at startup, and decides from the table it carries which
// actions the token is listed and which it may call.
//
// Every scenario mints its token with the fixture's builder, which creates it
// through the route GitLab offers its own user, so the grant is exactly the
// scopes the test wrote. The startup scopes (User, Namespace and Personal
// Access Token: Read for the user, Metadata: Read for the instance) are what
// lets the server learn what the token is; a scenario adds what it is about to
// them.
//
// The grant is judged only on the release the table was recorded from (phase
// B). On the prerelease of the release right after it, which is what a nightly
// image reports, the grant decides the listing alone and every call phase A
// allows is handed to GitLab. On any other release the server withholds
// exactly what no fine-grained token can reach and leaves the rest to GitLab
// (phase A, the grant not evaluated). So every scenario asserts what the
// instance's release decides, the listing where the grant decides it and the
// calls where it decides them, with the recorded version named where it is
// not that release: the same scenario holds each state, and none skips.
//
// The direct probes at the end bypass the server: each sends GitLab the
// request a cause in the table rests on, with a fine-grained token, and holds
// GitLab's own answer to what the table says, so a release that changes one
// fails here rather than in a model's answer.

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/files"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/settings"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The assignable permissions the scenarios grant, by the names GitLab's token
// creation route takes.
const (
	grantReadProject            = "read_project"
	grantReadWorkItem           = "read_work_item"
	grantCreateWorkItem         = "create_work_item"
	grantReadRepository         = "read_repository"
	grantReadBranch             = "read_branch"
	grantCreateRepository       = "create_repository"
	grantReadApplicationSetting = "read_application_setting"
)

// The permissions a refusal names, in the words GitLab's token creation page
// and its refusals use for them.
const (
	wordsBranchCreate   = "Branch: Create"
	wordsRepositoryRead = "Repository: Read"
	wordsMetadataRead   = "Metadata: Read"
	wordsWorkItemRead   = "Work Item: Read"
	wordsProjectRead    = "Project: Read"
)

// The stable sentences the server opens a fine-grained refusal with, one per
// phase (internal/finegrained), which a scenario asserts the phase by.
const (
	refusalPhaseA = "is not available to a fine-grained personal access token"
	refusalPhaseB = "this fine-grained personal access token was not granted"
)

// expectPhase holds a fine-grained session to what the instance's release
// decides ([harness.Session.FineGrainedPhase]), and reports what its grant
// decides there: the listing, the calls, both (phase B) or neither (phase A).
func expectPhase(e *harness.Env, s *harness.Session) harness.FineGrainedJudgement {
	e.T.Helper()
	judgement, problem := s.FineGrainedPhase()
	if problem != "" {
		e.T.Fatal(problem)
	}
	return judgement
}

// expectRefusedByGitLab asserts that a call the server handed to GitLab came
// back as GitLab's own 403, which is a class the server's withholding never
// takes, and that the refusal names the permission the token lacks.
func expectRefusedByGitLab(e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any, permission string) string {
	e.T.Helper()
	said := harness.Refused(s, id, params, harness.FailureForbidden)
	if !strings.Contains(said, permission) {
		e.T.Errorf("GitLab's refusal of %s does not name %s: %s", id, permission, firstLine(said))
	}
	return said
}

// expectDeniedWithheld asserts that an action no fine-grained token can run at
// the recorded release is withheld, in the phase A words, naming the release
// its verdict comes from and the reason given.
func expectDeniedWithheld(e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any, reason string) {
	e.T.Helper()
	said := harness.Withheld(s, id, params)
	version := actiongrants.Table().DisplayVersion()
	if !strings.Contains(said, refusalPhaseA) || !strings.Contains(said, version) || !strings.Contains(said, reason) {
		e.T.Errorf("%s was withheld without saying %q, naming GitLab %s and %q: %s", id, refusalPhaseA, version, reason, said)
	}
}

// expectNotGranted asserts what a write outside the grant gets: where the
// grant decides the calls (phase B) the server withholds it naming the
// permission the grant lacks, and elsewhere it lets GitLab answer, which names
// the same permission in its refusal.
func expectNotGranted(e *harness.Env, s *harness.Session, callsJudged bool, id harness.ActionID, params map[string]any, permission string) {
	e.T.Helper()
	if !callsJudged {
		expectRefusedByGitLab(e, s, id, params, permission)
		return
	}
	said := harness.Withheld(s, id, params)
	if !strings.Contains(said, refusalPhaseB) || !strings.Contains(said, permission) {
		e.T.Errorf("%s was withheld without saying the grant lacks %s: %s", id, permission, said)
	}
}

// onProjects is a grant of permissions on the named projects alone.
func onProjects(permissions []string, projects ...fixture.Project) fixture.GranularScope {
	ids := make([]int64, 0, len(projects))
	for _, project := range projects {
		ids = append(ids, project.ID)
	}
	return fixture.GranularScope{Access: fixture.AccessSelectedMemberships, Permissions: permissions, ProjectIDs: ids}
}

// withStartup is the startup scopes followed by the ones a scenario is about.
func withStartup(scopes ...fixture.GranularScope) []fixture.GranularScope {
	return append(fixture.StartupScopes(), scopes...)
}

// developerOf creates a user who is a developer of every project named.
func developerOf(e *harness.Env, prefix string, projects ...fixture.Project) fixture.User {
	e.T.Helper()
	user := fixture.NewUser(e, prefix)
	for _, project := range projects {
		fixture.AddProjectMember(e, project, user, gl.DeveloperPermissions)
	}
	return user
}

// TestFineGrained_StartupGrant_StartsOnStdioAndOverHTTP starts a server on a
// token holding the four startup permissions and nothing else, once on stdio
// and once over HTTP, where the grant is read per pool entry: both read the
// grant and the version, answer as the token's user, and withhold a project
// write the grant does not reach where the grant decides the calls (phase B),
// or leave it to GitLab where it does not.
func TestFineGrained_StartupGrant_StartsOnStdioAndOverHTTP(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	user := fixture.NewUser(e, "fgstart")
	token := fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()...)

	expectStartedOn(e, harness.TransportStdio, token)
	expectStartedOn(e, harness.TransportHTTP, token)
}

// expectStartedOn starts a session on the token over one transport and holds
// it to what a start on the startup grant alone gives.
func expectStartedOn(e *harness.Env, transport harness.TransportKind, token fixture.Token) {
	e.T.Helper()
	s := e.Session(harness.ServerConfig{Token: token.Value, Transport: transport})
	judgement := expectPhase(e, s)

	me := harness.Do[users.Output](s, actionUserCurrent, nil)
	if me.ID != token.UserID {
		e.T.Errorf("over %s the session answered as user %d, want the token's owner %d", transport, me.ID, token.UserID)
	}
	if got := s.Serves(actionBranchCreate); got == judgement.Calls {
		e.T.Errorf("over %s the session serves %s = %t, want %t in %s", transport, actionBranchCreate, got, !judgement.Calls, phaseName(judgement))
	}
	if got := s.Tier(); got != edition.Free {
		e.T.Errorf("over %s a token that cannot read the license detected tier %s, want %s", transport, got, edition.Free)
	}
}

// phaseName names what a grant decides, for a failure message.
func phaseName(judgement harness.FineGrainedJudgement) string {
	switch {
	case judgement.Calls:
		return "phase B"
	case judgement.Listing:
		return "the listing-only state"
	default:
		return "phase A"
	}
}

// TestFineGrained_WithoutMetadataRead_StartsWithTheGrantUnjudged starts a
// server on a token granted everything at the user boundary it needs and not
// Metadata: Read: the server cannot read the instance's version, so it does
// not evaluate the grant (phase A, whatever the release), still detects its
// tier from the namespaces the user holds, answers as the token's user, and
// withholds what no fine-grained token can reach, saying it could not read the
// version and what would let it.
func TestFineGrained_WithoutMetadataRead_StartsWithTheGrantUnjudged(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		user := fixture.NewUser(e, "fgnometa")
		return fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()[0])
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value})

		authority := s.Authority()
		if authority == nil || authority.Phase() != finegrained.PhaseUnknown || authority.Fallback() != finegrained.FallbackVersionUnreadable {
			e.T.Fatalf("a token refused Metadata: Read is judged %+v, want phase A for want of a readable version", authority)
		}
		if got := s.Tier(); got != edition.Free {
			e.T.Errorf("the session detected tier %s, want %s from the user's namespaces", got, edition.Free)
		}
		if me := harness.Do[users.Output](s, actionUserCurrent, nil); me.ID != token.UserID {
			e.T.Errorf("the session answered as user %d, want the token's owner %d", me.ID, token.UserID)
		}
		// The arguments never reach GitLab: the refusal comes before any
		// handler runs, so the project is a placeholder.
		expectDeniedWithheld(e, s, actionBranchRuleList, map[string]any{"project_path": "a/b"}, wordsMetadataRead)
		expectDeniedWithheld(e, s, actionWorkItemCreate, map[string]any{
			"full_path": "a/b", "work_item_type_id": "gid://gitlab/WorkItems::Type/1", "title": "must not be created",
		}, "WorkItemType")
	})
}

// projectGrantFixture is two projects a user develops in and a token granted
// project reads and work item writes on the first alone.
type projectGrantFixture struct {
	granted, other fixture.Project
	token          fixture.Token
}

// TestFineGrained_ProjectGrant_ServesWhatItReachesAndWithholdsTheRest grants
// project reads and work item writes on one project: a read and a write there
// are served and run, a write the grant does not hold is withheld naming the
// permission it lacks, a read of the user's other project goes to GitLab,
// whose 403 naming the permission the answer carries, and what no
// fine-grained token can reach is withheld whatever the grant holds.
//
// The fixture waits until the user sees the other project too, although the
// grant does not name it: before the background job that refreshes the user's
// authorizations has run, GitLab answers that read 404 for a project the user
// cannot see, which is not the refusal this scenario is about.
func TestFineGrained_ProjectGrant_ServesWhatItReachesAndWithholdsTheRest(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) projectGrantFixture {
		granted := fixture.NewProject(e, fixture.WithNamePrefix("fggranted"))
		other := fixture.NewProject(e, fixture.WithNamePrefix("fgother"))
		user := developerOf(e, "fgproject", granted, other)
		fixture.AwaitProjectAccess(e, user, other)
		return projectGrantFixture{granted: granted, other: other, token: fixture.NewFineGrainedToken(e, user, withStartup(
			onProjects([]string{grantReadProject, grantReadWorkItem, grantCreateWorkItem}, granted),
		)...)}
	}, func(e *harness.Env, surface harness.Surface, f projectGrantFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		judgement := expectPhase(e, s)
		here := map[string]any{"project_id": f.granted.IDParam()}

		title := "fine-grained " + string(surface)
		created := harness.Do[issues.Output](s, actionIssueCreate, withParams(here, map[string]any{"title": title}))
		if created.IID == 0 || created.Title != title {
			e.T.Fatalf("issue create answered %+v, want the issue %q", created, title)
		}
		listed := harness.Do[issues.ListOutput](s, actionIssueList, here)
		if !containsIssue(listed.Issues, created.IID) {
			e.T.Errorf("the granted project's issues do not hold #%d it just created", created.IID)
		}

		expectNotGranted(e, s, judgement.Calls, actionBranchCreate, withParams(here, map[string]any{
			"branch_name": "fg-" + string(surface), "ref": f.granted.DefaultBranch,
		}), wordsBranchCreate)

		refused := expectRefusedByGitLab(e, s, actionIssueList, map[string]any{"project_id": f.other.IDParam()}, wordsWorkItemRead)
		e.T.Logf("the read of the project outside the grant came back as: %s", firstLine(refused))

		expectDeniedWithheld(e, s, actionBranchRuleList, map[string]any{"project_path": f.granted.Path}, "BranchRule")
	})
}

// containsIssue reports whether a listing holds the issue.
func containsIssue(listed []issues.Output, iid int64) bool {
	for _, issue := range listed {
		if issue.IID == iid {
			return true
		}
	}
	return false
}

// grantedProjectFixture is a project and a token granted one set of
// permissions on it.
type grantedProjectFixture struct {
	project fixture.Project
	token   fixture.Token
}

// TestFineGrained_CreateOnlyGrant_CreatesARepositoryFile grants Repository:
// Create on one project and nothing that reads it: the file create is served
// and the file is in the repository afterwards, read back by the run's own
// token, since this one cannot read it.
func TestFineGrained_CreateOnlyGrant_CreatesARepositoryFile(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) grantedProjectFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("fgcreate"))
		user := developerOf(e, "fgcreate", project)
		return grantedProjectFixture{project: project, token: fixture.NewFineGrainedToken(e, user, withStartup(
			onProjects([]string{grantCreateRepository}, project),
		)...)}
	}, func(e *harness.Env, surface harness.Surface, f grantedProjectFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		expectPhase(e, s)

		path := "fine-grained-" + string(surface) + ".txt"
		created := harness.Do[files.FileInfoOutput](s, actionRepositoryFileCreate, map[string]any{
			"project_id": f.project.IDParam(), "file_path": path, "branch": f.project.DefaultBranch,
			"content": "created with a create-only grant\n", "commit_message": "chore: add " + path,
		})
		if created.FilePath != path {
			e.T.Fatalf("file create answered %+v, want %s", created, path)
		}
		if _, _, err := e.Client().GL().RepositoryFiles.GetFile(f.project.ID, path, &gl.GetFileOptions{Ref: new(f.project.DefaultBranch)}, gl.WithContext(e.Ctx)); err != nil {
			e.T.Errorf("the file the create-only grant wrote is not in the repository: %v", err)
		}
	})
}

// rawMetadataFixture is a project with a token granted its repository and one
// granted only the project.
type rawMetadataFixture struct {
	project            fixture.Project
	reader, projectOne fixture.Token
}

// TestFineGrained_RawFileMetadata_NeedsWhatTheRawReadNeeds holds the one
// request the derivation places by a declaration: the raw file metadata is a
// HEAD Grape answers from the raw read's GET, so it needs Repository: Read.
// A token granted it is listed the action and served the metadata. One
// granted only the project is not listed it where the grant decides the
// listing, and the call still goes to GitLab in every phase, since GitLab
// serves a public project's repository to anyone; GitLab refuses it on this
// private one with a 403, whose HEAD carries an empty body and so no
// permission to quote, and never the server's own refusal.
func TestFineGrained_RawFileMetadata_NeedsWhatTheRawReadNeeds(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, newRawMetadataFixture, func(e *harness.Env, surface harness.Surface, f rawMetadataFixture) {
		params := map[string]any{"project_id": f.project.IDParam(), "file_path": readmePath, "ref": f.project.DefaultBranch}

		reader := e.Session(harness.ServerConfig{Surface: surface, Token: f.reader.Value})
		expectPhase(e, reader)
		got := harness.Do[files.MetaDataOutput](reader, actionRepositoryFileRawMetadata, params)
		if got.FileName != readmePath || got.Size == 0 {
			e.T.Errorf("raw metadata answered %+v, want the README with its size", got)
		}

		other := e.Session(harness.ServerConfig{Surface: surface, Token: f.projectOne.Value})
		if expectPhase(e, other).Listing && slices.Contains(other.Actions(), actionRepositoryFileRawMetadata) {
			e.T.Errorf("%s is listed for a token whose grant does not hold %s", actionRepositoryFileRawMetadata, wordsRepositoryRead)
		}
		harness.Refused(other, actionRepositoryFileRawMetadata, params, harness.FailureForbidden)
	})
}

// readmePath is the file every fixture project is initialized with.
const readmePath = "README.md"

// newRawMetadataFixture builds a project and its two tokens.
func newRawMetadataFixture(e *harness.Env) rawMetadataFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgraw"))
	user := developerOf(e, "fgraw", project)
	return rawMetadataFixture{
		project:    project,
		reader:     fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantReadProject, grantReadRepository}, project))...),
		projectOne: fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantReadProject}, project))...),
	}
}

// TestFineGrained_AdministratorGrant_ReadsTheApplicationSettings grants an
// administrator Application Setting: Read at the instance: the admin read is
// served and answers the instance's settings, so a fine-grained token reaches
// an instance-boundary permission.
//
// It says nothing about Admin Mode: the Docker instance does not enforce it
// (the application setting is off unless an operator turns it on), and with it
// off GitLab treats every administrator as one in admin mode, so this passes
// whatever GitLab does with Admin Mode for a fine-grained token.
// TestFineGrained_AdministratorGrant_IsInAdminModeWhereAdminModeIsEnforced
// turns the setting on and asks.
func TestFineGrained_AdministratorGrant_ReadsTheApplicationSettings(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		admin := fixture.NewUser(e, "fgadmin", func(opts *gl.CreateUserOptions) { opts.Admin = new(true) })
		return fixture.NewFineGrainedToken(e, admin, withStartup(fixture.GranularScope{
			Access: fixture.AccessInstance, Permissions: []string{grantReadApplicationSetting},
		})...)
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value})
		expectPhase(e, s)

		got := harness.Do[settings.GetOutput](s, actionAdminSettingsGet, nil)
		if len(got.Settings) == 0 {
			e.T.Errorf("the admin read answered no settings")
		}
	})
}

// adminModeFixture is an administrator's fine-grained token granted
// Application Setting: Read at the instance, and a classic token of the same
// administrator with the api scope and not admin_mode.
type adminModeFixture struct {
	fineGrained, classic fixture.Token
}

// TestFineGrained_AdministratorGrant_IsInAdminModeWhereAdminModeIsEnforced
// turns Admin Mode on for the instance and holds GitLab to the reading the
// listing of instance-boundary admin actions rests on (ADR-0024): an
// administrator's fine-grained token is in admin mode for API calls, because
// the API guard takes a token that passes the admin_mode scope check out of
// the session, and the check skips the scope comparison for a fine-grained
// token (lib/api/api_guard.rb and app/services/access_token_validation_service.rb
// at 19.4.1). The same administrator's classic token without admin_mode is
// the control: GitLab refuses it the admin read, which is what shows Admin Mode
// was enforced while the fine-grained token's read was served on every
// surface.
//
// Serial, since every administrator's classic token without admin_mode is
// refused admin calls while the setting is on, and the setting reaches every
// GitLab process only after the minute each keeps its settings for, on the way
// on and on the way off.
func TestFineGrained_AdministratorGrant_IsInAdminModeWhereAdminModeIsEnforced(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal), harness.Serial())

	harness.SurfacesWith(e, func(e *harness.Env) adminModeFixture {
		admin := fixture.NewUser(e, "fgadminmode", func(opts *gl.CreateUserOptions) { opts.Admin = new(true) })
		f := adminModeFixture{
			fineGrained: fixture.NewFineGrainedToken(e, admin, withStartup(fixture.GranularScope{
				Access: fixture.AccessInstance, Permissions: []string{grantReadApplicationSetting},
			})...),
			classic: fixture.NewToken(e, admin),
		}
		enforceAdminMode(e)
		return f
	}, func(e *harness.Env, surface harness.Surface, f adminModeFixture) {
		classic, err := e.ClientFor(f.classic.Value)
		if err != nil {
			e.T.Fatalf("building a client for the classic token: %v", err)
		}
		if _, _, readErr := classic.GL().Settings.GetSettings(gl.WithContext(e.Ctx)); !fixture.IsStatus(readErr, http.StatusForbidden) {
			e.T.Fatalf("the classic token without admin_mode was answered %v on the admin read, want 403: Admin Mode is not enforced", readErr)
		}

		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.fineGrained.Value})
		expectPhase(e, s)
		got := harness.Do[settings.GetOutput](s, actionAdminSettingsGet, nil)
		if len(got.Settings) == 0 {
			e.T.Errorf("the admin read under Admin Mode answered no settings")
		}
	})
}

// adminModeSettle is how long after the admin_mode setting changes every
// GitLab process answers by its new value: each keeps the application settings
// in process memory for a minute (app/models/concerns/cacheable_attributes.rb,
// application_settings_cache_seconds, at 19.4.1), so until then a request can
// land on a process that still answers by the value before the change. The
// second on top is margin, as for a feature flag.
const adminModeSettle = time.Minute + time.Second

// enforceAdminMode turns the instance's Admin Mode on for the rest of the test
// and returns once every GitLab process enforces it, and turns it off again
// when the test ends, waiting the same minute before the test gives the
// instance back. An instance that already enforces it is left as it is.
//
// The restore is a cleanup of the test rather than of its ledger: it runs
// before the ledger's undo work and before the serial gate opens, with its own
// deadline, since the wait alone outlasts the budget the ledger's work shares.
func enforceAdminMode(e *harness.Env) {
	e.T.Helper()
	current, _, readErr := e.Client().GL().Settings.GetSettings(gl.WithContext(e.Ctx))
	if readErr != nil {
		e.T.Fatalf("reading the application settings: %v", readErr)
	}
	if current.AdminMode {
		return
	}
	e.T.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*adminModeSettle)
		defer cancel()
		if _, _, offErr := e.Client().GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{AdminMode: new(false)}, gl.WithContext(ctx)); offErr != nil {
			e.T.Errorf("turning Admin Mode off again: %v", offErr)
			return
		}
		if waitErr := awaitSettled(ctx, adminModeSettle); waitErr != nil {
			e.T.Errorf("waiting for Admin Mode to be off everywhere: %v", waitErr)
		}
	})
	if _, _, onErr := e.Client().GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{AdminMode: new(true)}, gl.WithContext(e.Ctx)); onErr != nil {
		e.T.Fatalf("turning Admin Mode on: %v", onErr)
	}
	if waitErr := awaitSettled(e.Ctx, adminModeSettle); waitErr != nil {
		e.T.Fatalf("waiting for Admin Mode to be on everywhere: %v", waitErr)
	}
}

// awaitSettled holds for lifetime, or until ctx ends.
func awaitSettled(ctx context.Context, lifetime time.Duration) error {
	timer := time.NewTimer(lifetime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// publicProjectFixture is a public project holding two linked issues, and a
// token granted nothing on it.
type publicProjectFixture struct {
	project        fixture.Project
	source, target fixture.Issue
	token          fixture.Token
}

// TestFineGrained_PublicProject_ServesAReadTheGrantDoesNotHold reads the issue
// links of a public project with a token granted nothing past the startup
// scopes: where the grant decides the listing the read is not listed for it,
// since the grant does not hold Work Item: Read, and in every phase it is
// served, since GitLab answers it on a public project whoever asks, and
// GitLab answers it with the link.
func TestFineGrained_PublicProject_ServesAReadTheGrantDoesNotHold(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, newPublicProjectFixture, func(e *harness.Env, surface harness.Surface, f publicProjectFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		// Phase A lists every action GitLab may serve, this one included, so
		// only a grant that decides the listing leaves it out.
		if expectPhase(e, s).Listing && slices.Contains(s.Actions(), actionIssueLinkList) {
			e.T.Errorf("%s is listed for a token whose grant does not hold %s", actionIssueLinkList, wordsWorkItemRead)
		}
		listed := harness.Do[issuelinks.ListOutput](s, actionIssueLinkList, map[string]any{
			"project_id": f.project.IDParam(), "issue_iid": f.source.IID,
		})
		if len(listed.Relations) != 1 || int64(listed.Relations[0].IID) != f.target.IID {
			e.T.Errorf("the public issue's relations are %+v, want the one link to #%d", listed.Relations, f.target.IID)
		}
	})
}

// newPublicProjectFixture builds a public project with two linked issues and
// a token holding the startup scopes alone.
func newPublicProjectFixture(e *harness.Env) publicProjectFixture {
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgpublic"), fixture.WithVisibility(gl.PublicVisibility))
	source := fixture.NewIssue(e, project, "public source")
	target := fixture.NewIssue(e, project, "public target")
	_, _, err := e.Client().GL().IssueLinks.CreateIssueLink(project.ID, source.IID, &gl.CreateIssueLinkOptions{
		TargetProjectID: new(project.IDParam()), TargetIssueIID: new(strconv.FormatInt(target.IID, 10)),
	}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("linking the public issues: %v", err)
	}
	user := fixture.NewUser(e, "fgpublic")
	return publicProjectFixture{project: project, source: source, target: target, token: fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()...)}
}

// personalProjectFixture is a project in a user's own namespace, another
// project the user develops in, and a token granted project reads on the
// user's personal projects.
type personalProjectFixture struct {
	personal, member fixture.Project
	token            fixture.Token
}

// TestFineGrained_PersonalProjects_ReachTheUsersOwnNamespaceOnly grants
// project reads at the personal-projects level: GitLab attaches the scope to
// the creating user's own namespace, so the server serves the read, GitLab
// answers it for the project in that namespace, and refuses it with a 403
// naming Project: Read for a project the user only develops in. The fixture
// waits until the user sees that project, since GitLab answers 404 for it
// until the background job refreshing the user's authorizations has run.
func TestFineGrained_PersonalProjects_ReachTheUsersOwnNamespaceOnly(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) personalProjectFixture {
		user := fixture.NewUser(e, "fgpersonal")
		personal := fixture.NewProject(e, fixture.WithNamePrefix("fgpersonal"), fixture.OwnedBy(user))
		member := fixture.NewProject(e, fixture.WithNamePrefix("fgmember"))
		fixture.AddProjectMember(e, member, user, gl.DeveloperPermissions)
		fixture.AwaitProjectAccess(e, user, member)
		return personalProjectFixture{personal: personal, member: member, token: fixture.NewFineGrainedToken(e, user, withStartup(
			fixture.GranularScope{Access: fixture.AccessPersonalProjects, Permissions: []string{grantReadProject}},
		)...)}
	}, func(e *harness.Env, surface harness.Surface, f personalProjectFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		expectPhase(e, s)

		got := harness.Do[map[string]any](s, actionProjectGet, map[string]any{"project_id": f.personal.IDParam()})
		if fmt.Sprint(got["id"]) != f.personal.IDParam() {
			e.T.Errorf("the personal project read answered %v, want project %d", got["id"], f.personal.ID)
		}
		expectRefusedByGitLab(e, s, actionProjectGet, map[string]any{"project_id": f.member.IDParam()}, wordsProjectRead)
	})
}

// rawQuery is the ref a raw file read names.
type rawQuery struct {
	Ref string `url:"ref"`
}

// TestFineGrainedProbes_RawFileHead_AsksWhatTheGetAsks measures what the
// head-inherits-get declaration of cmd/gen_action_grants rests on: GitLab
// mounts the raw file read as a GET only, and answers a HEAD of it from the
// same endpoint, so the HEAD needs Repository: Read exactly as the GET does.
func TestFineGrainedProbes_RawFileHead_AsksWhatTheGetAsks(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	f := newRawMetadataFixture(e)
	path := fmt.Sprintf("projects/%d/repository/files/%s/raw", f.project.ID, readmePath)
	query := rawQuery{Ref: f.project.DefaultBranch}

	// Asked in turn rather than as subtests: a probe fails through the Env's
	// own T, which a subtest must not do.
	expectProbeStatus(e, f.reader, http.MethodGet, path, query, http.StatusOK)
	expectProbeStatus(e, f.reader, http.MethodHead, path, query, http.StatusOK)
	expectProbeStatus(e, f.projectOne, http.MethodGet, path, query, http.StatusForbidden)
	expectProbeStatus(e, f.projectOne, http.MethodHead, path, query, http.StatusForbidden)
}

// expectProbeStatus sends one REST probe and holds GitLab's status to want.
func expectProbeStatus(e *harness.Env, token fixture.Token, method, path string, query any, want int) {
	e.T.Helper()
	if got := fixture.ProbeREST(e, token, method, path, query); got.Status != want {
		e.T.Errorf("%s %s answered %d, want %d: %s", method, path, got.Status, want, got.Body)
	}
}

// TestFineGrainedProbes_RESTRefusal_NamesThePermission measures the answer
// the server's refusal handling reads: a REST route a fine-grained token's
// grant does not reach is refused 403 with insufficient_granular_scope and
// the permission it needs, in the words the token creation page uses.
func TestFineGrainedProbes_RESTRefusal_NamesThePermission(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgrest"))
	user := developerOf(e, "fgrest", project)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantReadProject}, project))...)

	got := fixture.ProbeREST(e, token, http.MethodGet, fmt.Sprintf("projects/%d/repository/branches", project.ID), nil)
	if got.Status != http.StatusForbidden || !strings.Contains(got.Body, "insufficient_granular_scope") || !strings.Contains(got.Body, "Branch: Read") {
		t.Errorf("a branch list outside the grant answered %d: %s; want 403 with insufficient_granular_scope naming Branch: Read", got.Status, got.Body)
	}
}

// TestFineGrainedProbes_UndeclaredGraphQLTypes_AreEmptyWithNoError measures
// the two read effects the table records for a GraphQL type GitLab declares no
// fine-grained permission on, with the same document sent by the run's classic
// token beside it: a field of such a type answers null (namespace, the type
// group.epic_get and issue.work_item_get read), and a list of it loses its
// items (branchRules, which branch.rule_list reads). Neither carries an error,
// which is why the server withholds the actions rather than letting a model
// read an empty answer as an absence.
func TestFineGrainedProbes_UndeclaredGraphQLTypes_AreEmptyWithNoError(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	project := fixture.NewProject(e, fixture.WithNamePrefix("fggraphql"))
	user := developerOf(e, "fggraphql", project)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantReadProject, grantReadBranch, grantReadWorkItem}, project))...)

	variables := map[string]any{"path": project.Path}

	expectEmptyForFineGrained(e, token, "namespace", `query($path: ID!) { namespace(fullPath: $path) { id } }`, variables,
		func(data json.RawMessage) bool { return string(data) == "null" })
	expectEmptyForFineGrained(e, token, "project", `query($path: ID!) { project(fullPath: $path) { branchRules(first: 20) { nodes { name } } } }`, variables,
		func(data json.RawMessage) bool { return !strings.Contains(string(data), `"name"`) })
}

// expectEmptyForFineGrained sends one document as the run's classic token and
// as the fine-grained one, and holds the classic answer at field to something
// and the fine-grained one to what empty says is nothing, both with no error.
func expectEmptyForFineGrained(e *harness.Env, token fixture.Token, field, document string, variables map[string]any, empty func(json.RawMessage) bool) {
	e.T.Helper()
	classic := fixture.ProbeGraphQL(e, fixture.Token{}, document, variables)
	fine := fixture.ProbeGraphQL(e, token, document, variables)
	if len(classic.Errors) > 0 || len(fine.Errors) > 0 {
		e.T.Errorf("%s was answered with errors: classic %v, fine-grained %v", field, classic.Errors, fine.Errors)
		return
	}
	if empty(classic.Data[field]) {
		e.T.Errorf("the classic token was answered %s at %s, so the probe shows nothing", classic.Data[field], field)
	}
	if !empty(fine.Data[field]) {
		e.T.Errorf("the fine-grained token was answered %s at %s, want it empty", fine.Data[field], field)
	}
}

// The assignable permissions the write probes grant, by the names GitLab's
// token creation route takes.
const (
	grantCreateCustomEmoji = "create_custom_emoji"
	grantCreateAchievement = "create_achievement"
)

// graphQLResourceAccessError is GitLab's generic refusal of a GraphQL
// request (Gitlab::Graphql::Authorize::AuthorizeResource::RESOURCE_ACCESS_ERROR),
// which is also what it refuses a fine-grained token a mutation that declares
// no fine-grained permission with.
const graphQLResourceAccessError = "The resource that you are attempting to access does not exist or you don't have permission to perform this action"

// onGroups is a grant of permissions on the named groups and everything below
// them.
func onGroups(permissions []string, groups ...fixture.Group) fixture.GranularScope {
	ids := make([]int64, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return fixture.GranularScope{Access: fixture.AccessSelectedMemberships, Permissions: permissions, GroupIDs: ids}
}

// expectCommittedWithoutObject holds a write's answer to what GitLab gives a
// fine-grained token whose grant holds the mutation and not the object its
// payload returns: the payload itself, with no errors of its own, and the
// object at objectField null. nulledBy is the top-level error that null
// writes where the object sits below a non-null position, or "" where it is
// nullable and no error comes with it.
func expectCommittedWithoutObject(e *harness.Env, written fixture.GraphQLAnswer, field, objectField, nulledBy string) {
	e.T.Helper()
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(written.Data[field], &payload); err != nil || payload == nil {
		e.T.Fatalf("the fine-grained write answered %s at %s, want its payload: %v (errors %v)", written.Data[field], field, err, written.Errors)
	}
	if string(payload["errors"]) != "[]" {
		e.T.Errorf("the fine-grained write's payload carries errors %s, want none", payload["errors"])
	}
	if string(payload[objectField]) != "null" {
		e.T.Errorf("the fine-grained write answered %s at %s.%s, want null", payload[objectField], field, objectField)
	}
	switch {
	case nulledBy == "" && len(written.Errors) > 0:
		e.T.Errorf("the fine-grained write answered errors %v, want none", written.Errors)
	case nulledBy != "" && (len(written.Errors) == 0 || !slices.ContainsFunc(written.Errors, func(m string) bool { return strings.HasPrefix(m, nulledBy) })):
		e.T.Errorf("the fine-grained write answered errors %v, want the one %q writes", written.Errors, nulledBy)
	}
}

// expectReadBack reads what a write made with the run's classic token and
// holds the answer at field to carry want, which is what shows the write
// GitLab answered a fine-grained token without committed.
func expectReadBack(e *harness.Env, document string, variables map[string]any, field, want string) {
	e.T.Helper()
	after := fixture.ProbeGraphQL(e, fixture.Token{}, document, variables)
	if len(after.Errors) > 0 || !strings.Contains(string(after.Data[field]), want) {
		e.T.Errorf("the classic read of %s answered %s %v, want it to carry %s: the write did not commit", field, after.Data[field], after.Errors, want)
	}
}

// TestFineGrainedProbes_CustomEmojiCreate_CommitsAndAnswersNull measures what
// the custom_emoji.create row of the table rests on, and what the handler's
// probably-committed answer describes (issue 1103): a token granted Custom
// Emoji: Create on a group passes createCustomEmoji's own check, so GitLab
// creates the emoji, and then checks the CustomEmoji the payload returns,
// which at 19.4.1 declares no fine-grained permission (19.5 declares Custom
// Emoji: Read, which this grant does not hold either), so the payload answers
// it null with no error. The run's classic token then finds the emoji.
func TestFineGrainedProbes_CustomEmojiCreate_CommitsAndAnswersNull(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedFixtureService))
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("fgemoji"))
	user := fixture.NewUser(e, "fgemoji")
	fixture.AddGroupMember(e, group, user, gl.MaintainerPermissions)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onGroups([]string{grantCreateCustomEmoji}, group))...)
	const name = "fg_committed"

	written := fixture.ProbeGraphQL(e, token, `mutation($group: ID!, $name: String!, $url: String!) {
  createCustomEmoji(input: {groupPath: $group, name: $name, url: $url}) { customEmoji { id name } errors }
}`, map[string]any{"group": group.Path, "name": name, "url": fixture.ServiceURL(e, "/emoji.png")})

	expectCommittedWithoutObject(e, written, "createCustomEmoji", "customEmoji", "")
	expectReadBack(e, `query($group: ID!) { group(fullPath: $group) { customEmoji { nodes { name } } } }`,
		map[string]any{"group": group.Path}, "group", `"`+name+`"`)
}

// TestFineGrainedProbes_AchievementCreate_CommitsAndAnswersNull measures the
// same of achievementsCreate, which the achievement.create row rests on: a
// token granted Achievement: Create on a group creates the achievement, and
// GitLab then answers the payload's Achievement, a type that declares no
// fine-grained permission, null with no error (issue 1103).
func TestFineGrainedProbes_AchievementCreate_CommitsAndAnswersNull(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("fgachieve"))
	user := fixture.NewUser(e, "fgachieve")
	fixture.AddGroupMember(e, group, user, gl.MaintainerPermissions)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onGroups([]string{grantCreateAchievement}, group))...)
	name := e.Name("fg-achievement")

	written := fixture.ProbeGraphQL(e, token, `mutation($namespace: NamespaceID!, $name: String!) {
  achievementsCreate(input: {namespaceId: $namespace, name: $name}) { achievement { id name } errors }
}`, map[string]any{"namespace": "gid://gitlab/Namespace/" + strconv.FormatInt(group.ID, 10), "name": name})

	expectCommittedWithoutObject(e, written, "achievementsCreate", "achievement", "")
	expectReadBack(e, `query($group: ID!) { group(fullPath: $group) { achievements { nodes { name } } } }`,
		map[string]any{"group": group.Path}, "group", `"`+name+`"`)
}

// TestFineGrainedProbes_WorkItemCreate_CommitsAndNullsTheWorkItem measures the
// non-null form of the same answer, which the issue.work_item_create row
// rests on: a token granted Work Item: Create and Read on a project creates
// the work item, and GitLab then checks its workItemType, a non-null field of
// the type WorkItemType, which at 19.4.1 declares no fine-grained permission,
// so the null takes the work item with it and GitLab's GraphQL library writes
// the error that says so (issue 1103). It flips once GitLab declares
// WorkItemType, which its own merge request 251840 proposes.
func TestFineGrainedProbes_WorkItemCreate_CommitsAndNullsTheWorkItem(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgworkitem"))
	user := developerOf(e, "fgworkitem", project)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantCreateWorkItem, grantReadWorkItem}, project))...)
	title := e.Name("fine-grained work item")

	written := fixture.ProbeGraphQL(e, token, `mutation($path: ID!, $title: String!, $type: WorkItemsTypeID!) {
  workItemCreate(input: {namespacePath: $path, title: $title, workItemTypeId: $type}) { workItem { id title workItemType { name } } errors }
}`, map[string]any{"path": project.Path, "title": title, "type": string(gl.WorkItemTypeIssue)})

	expectCommittedWithoutObject(e, written, "workItemCreate", "workItem", "Cannot return null for non-nullable field WorkItem.workItemType")
	expectReadBack(e, `query($path: ID!) { project(fullPath: $path) { workItems(first: 20) { nodes { title } } } }`,
		map[string]any{"path": project.Path}, "project", strconv.Quote(title))
}

// TestFineGrainedProbes_UndeclaredMutation_IsRefusedWithTheGenericError
// measures how GitLab refuses a fine-grained token a mutation that declares
// no fine-grained permission, which the CauseMutationUndeclared rows of the
// table rest on: before anything runs, with its generic "does not exist or you
// don't have permission" sentence, not the "Access denied: This operation
// requires a fine-grained ..." sentence a declared mutation the grant lacks
// is refused with, so nothing in the answer says the token is the reason. The
// mutation is EchoCreate, which changes nothing and is on GitLab's pending
// list at 19.4.1; the run's classic token is answered its echo. The document
// passes an empty errors list, because EchoCreate answers its input's errors
// back in a payload field that cannot be null, and a call that names none is
// answered null whoever makes it.
func TestFineGrainedProbes_UndeclaredMutation_IsRefusedWithTheGenericError(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	user := fixture.NewUser(e, "fgecho")
	token := fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()...)
	const document = `mutation($messages: [String!]) { echoCreate(input: {messages: $messages, errors: []}) { echoes errors } }`
	variables := map[string]any{"messages": []string{"fine-grained"}}

	classic := fixture.ProbeGraphQL(e, fixture.Token{}, document, variables)
	if len(classic.Errors) > 0 || !strings.Contains(string(classic.Data["echoCreate"]), `"fine-grained"`) {
		t.Fatalf("the classic token's echo answered %s %v, want the message echoed", classic.Data["echoCreate"], classic.Errors)
	}
	fine := fixture.ProbeGraphQL(e, token, document, variables)
	if string(fine.Data["echoCreate"]) != "null" || !slices.Contains(fine.Errors, graphQLResourceAccessError) {
		t.Errorf("the fine-grained echo answered %s %v, want null and the generic refusal %q", fine.Data["echoCreate"], fine.Errors, graphQLResourceAccessError)
	}
	for _, message := range fine.Errors {
		if strings.Contains(message, "fine-grained") {
			t.Errorf("the fine-grained echo's refusal names the token: %q", message)
		}
	}
}

// TestFineGrainedProbes_CatalogListing_AnswersEachResourceNull measures the
// list-of-nulls answer of issue 1103, which the ci_catalog.list row rests on:
// CiCatalogResource declares neither an ability nor a fine-grained
// permission, so GitLab redacts nothing from the listing and checks each
// resource as it answers it, and a fine-grained token, granted Project: Read
// on the resource's project, reads one null per resource the classic token
// reads, with no error. Publishing the version needs a runner, as the catalog
// read's own scenario does.
func TestFineGrainedProbes_CatalogListing_AnswersEachResourceNull(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedRunner), harness.Locks(harness.LockRunner))
	if !e.DockerMode() {
		e.Skipf("publishing a catalog version needs the compose-internal GitLab URL and a runner, which only the Docker stack provides")
	}
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgcatalog"))
	if !markCatalogResource(e, project) {
		return
	}
	publishCatalogVersion(e, project)
	user := developerOf(e, "fgcatalog", project)
	token := fixture.NewFineGrainedToken(e, user, withStartup(onProjects([]string{grantReadProject}, project))...)
	const document = `query($search: String) { ciCatalogResources(search: $search) { nodes { id name } } }`
	variables := map[string]any{"search": project.Name}

	// The catalog index the listing reads is written by a background job, so
	// the classic listing is waited for before the fine-grained one is asked.
	var classic fixture.GraphQLAnswer
	if err := harness.Poll(e.Ctx, 2*time.Second, 60*time.Second, func() (bool, string, error) {
		classic = fixture.ProbeGraphQL(e, fixture.Token{}, document, variables)
		listed := string(classic.Data["ciCatalogResources"])
		return len(classic.Errors) == 0 && strings.Contains(listed, `"name":"`+project.Name+`"`), listed, nil
	}); err != nil {
		t.Fatalf("the classic token never listed the published resource: %v", err)
	}
	fine := fixture.ProbeGraphQL(e, token, document, variables)
	if len(fine.Errors) > 0 || !strings.Contains(string(fine.Data["ciCatalogResources"]), `"nodes":[null]`) {
		t.Errorf("the fine-grained listing answered %s %v, want one null resource and no error", fine.Data["ciCatalogResources"], fine.Errors)
	}
}
