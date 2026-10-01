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
// B). On any other release the server withholds exactly what no fine-grained
// token can reach and leaves the rest to GitLab (phase A, the grant not
// evaluated), so every phase B scenario asserts phase B where the instance is
// that release and phase A, with the recorded version named, where it is not:
// the same scenario holds both, and neither skips.
//
// The direct probes at the end bypass the server: each sends GitLab the
// request a cause in the table rests on, with a fine-grained token, and holds
// GitLab's own answer to what the table says, so a release that changes one
// fails here rather than in a model's answer.

package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

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
)

// The stable sentences the server opens a fine-grained refusal with, one per
// phase (internal/finegrained), which a scenario asserts the phase by.
const (
	refusalPhaseA = "is not available to a fine-grained personal access token"
	refusalPhaseB = "this fine-grained personal access token was not granted"
)

// expectPhase holds a fine-grained session to the phase the instance's release
// decides ([harness.Session.FineGrainedPhase]), and reports whether its grant
// was evaluated (phase B).
func expectPhase(e *harness.Env, s *harness.Session) bool {
	e.T.Helper()
	judged, problem := s.FineGrainedPhase()
	if problem != "" {
		e.T.Fatal(problem)
	}
	return judged
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

// expectNotGranted asserts what a write outside the grant gets: in phase B the
// server withholds it naming the permission the grant lacks, and in phase A it
// lets GitLab answer, which names the same permission in its refusal.
func expectNotGranted(e *harness.Env, s *harness.Session, judged bool, id harness.ActionID, params map[string]any, permission string) {
	e.T.Helper()
	if !judged {
		harness.ExpectToolError(s, id, params, permission)
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
// write the grant does not reach (phase B), or leave it to GitLab on a release
// the table does not record (phase A).
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
	judged := expectPhase(e, s)

	me := harness.Do[users.Output](s, actionUserCurrent, nil)
	if me.ID != token.UserID {
		e.T.Errorf("over %s the session answered as user %d, want the token's owner %d", transport, me.ID, token.UserID)
	}
	if got := s.Serves(actionBranchCreate); got == judged {
		e.T.Errorf("over %s the session serves %s = %t, want %t in phase %s", transport, actionBranchCreate, got, !judged, phaseName(judged))
	}
	if got := s.Tier(); got != edition.Free {
		e.T.Errorf("over %s a token that cannot read the license detected tier %s, want %s", transport, got, edition.Free)
	}
}

// phaseName names a phase for a failure message.
func phaseName(judged bool) string {
	if judged {
		return "B"
	}
	return "A"
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
// whose refusal the answer carries, and what no fine-grained token can reach
// is withheld whatever the grant holds.
func TestFineGrained_ProjectGrant_ServesWhatItReachesAndWithholdsTheRest(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) projectGrantFixture {
		granted := fixture.NewProject(e, fixture.WithNamePrefix("fggranted"))
		other := fixture.NewProject(e, fixture.WithNamePrefix("fgother"))
		user := developerOf(e, "fgproject", granted, other)
		return projectGrantFixture{granted: granted, other: other, token: fixture.NewFineGrainedToken(e, user, withStartup(
			onProjects([]string{grantReadProject, grantReadWorkItem, grantCreateWorkItem}, granted),
		)...)}
	}, func(e *harness.Env, surface harness.Surface, f projectGrantFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		judged := expectPhase(e, s)
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

		expectNotGranted(e, s, judged, actionBranchCreate, withParams(here, map[string]any{
			"branch_name": "fg-" + string(surface), "ref": f.granted.DefaultBranch,
		}), wordsBranchCreate)

		refused := harness.ExpectToolError(s, actionIssueList, map[string]any{"project_id": f.other.IDParam()}, "fine-grained")
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
// granted only the project is not listed it, and the call still goes to
// GitLab, since GitLab serves a public project's repository to anyone, and
// GitLab refuses it on this private one, with a HEAD's empty body and so no
// permission to quote.
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
		if expectPhase(e, other) && slices.Contains(other.Actions(), actionRepositoryFileRawMetadata) {
			e.T.Errorf("%s is listed for a token whose grant does not hold %s", actionRepositoryFileRawMetadata, wordsRepositoryRead)
		}
		if _, err := harness.Try[files.MetaDataOutput](other, actionRepositoryFileRawMetadata, params); err == nil {
			e.T.Errorf("a token granted only the project was answered the raw metadata of a private project")
		}
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
// an instance-boundary permission the way GitLab's admin mode lets it.
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

// publicProjectFixture is a public project holding two linked issues, and a
// token granted nothing on it.
type publicProjectFixture struct {
	project        fixture.Project
	source, target fixture.Issue
	token          fixture.Token
}

// TestFineGrained_PublicProject_ServesAReadTheGrantDoesNotHold reads the issue
// links of a public project with a token granted nothing past the startup
// scopes: the read is not listed for it, since the grant does not hold Work
// Item: Read, and is still served, since GitLab answers it on a public project
// whoever asks, and GitLab answers it with the link.
func TestFineGrained_PublicProject_ServesAReadTheGrantDoesNotHold(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, newPublicProjectFixture, func(e *harness.Env, surface harness.Surface, f publicProjectFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value})
		if !expectPhase(e, s) {
			// Phase A lists every action GitLab may serve, this one included.
			return
		}

		if slices.Contains(s.Actions(), actionIssueLinkList) {
			e.T.Errorf("%s is listed for a token whose grant does not hold Work Item: Read", actionIssueLinkList)
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
// answers it for the project in that namespace, and refuses it for a project
// the user only develops in.
func TestFineGrained_PersonalProjects_ReachTheUsersOwnNamespaceOnly(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) personalProjectFixture {
		user := fixture.NewUser(e, "fgpersonal")
		personal := fixture.NewProject(e, fixture.WithNamePrefix("fgpersonal"), fixture.OwnedBy(user))
		member := fixture.NewProject(e, fixture.WithNamePrefix("fgmember"))
		fixture.AddProjectMember(e, member, user, gl.DeveloperPermissions)
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
		// GitLab answers 403 naming the permission once the membership has
		// reached the user's authorizations, and 404 before, which a
		// background job decides the moment of; either is GitLab refusing a
		// project the grant does not reach, and neither is served.
		said := harness.ExpectToolError(s, actionProjectGet, map[string]any{"project_id": f.member.IDParam()}, "")
		if !strings.Contains(said, "Project: Read") && !strings.Contains(strings.ToLower(said), "not found") {
			e.T.Errorf("the read of a project outside the personal namespace came back as neither GitLab's refusal nor its not-found: %s", said)
		}
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
