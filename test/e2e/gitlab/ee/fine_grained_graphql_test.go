//go:build e2e

// fine_grained_graphql_test.go covers what a fine-grained personal access
// token (issue 952) meets in the licensed GraphQL surface: the reads and
// writes no such token can reach at the recorded release, which the server
// withholds with the reason; the vulnerability reads it can, which are served
// with a note naming what GitLab leaves empty for the credential; a public
// project's attestations, served to a grant that does not hold them; and, by
// direct probes that bypass the server, what GitLab answers a group's work
// item, which is what the table's boundary cause rests on.
//
// Every session pins Ultimate: a fine-grained token cannot read the license,
// so a session left to detect would be served the Free catalog and none of
// the actions asked about here.

package ee

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/attestations"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/vulnerabilities"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The assignable permissions the scenarios grant, by the names GitLab's token
// creation route takes.
const (
	fineGrantReadProject       = "read_project"
	fineGrantReadVulnerability = "read_vulnerability"
	fineGrantReadWorkItem      = "read_work_item"
	fineGrantUpdateWorkItem    = "update_work_item"
)

// The words the server's answers carry, which the scenarios assert.
const (
	// fineGrainedPhaseA is the sentence a withheld answer opens with when no
	// fine-grained token can reach the action.
	fineGrainedPhaseA = "is not available to a fine-grained personal access token"
	// fineGrainedDegraded opens the note a served answer gets for the parts
	// GitLab leaves empty for the credential.
	fineGrainedDegraded = "leaves part of this answer empty for a fine-grained personal access token"
	// fineGrainedNull is the note a not-found GraphQL answer gets.
	fineGrainedNull = "not found may mean this token cannot see it"
)

// fineGrainedPhaseOf holds a fine-grained session to the phase the instance's
// release decides, and reports whether its grant was evaluated.
func fineGrainedPhaseOf(e *harness.Env, s *harness.Session) bool {
	e.T.Helper()
	judged, problem := s.FineGrainedPhase()
	if problem != "" {
		e.T.Fatal(problem)
	}
	return judged
}

// expectWithheldFor asserts that an action no fine-grained token can run is
// withheld in the phase A words, with the reason given.
func expectWithheldFor(e *harness.Env, s *harness.Session, id harness.ActionID, params map[string]any, reason string) {
	e.T.Helper()
	said := harness.Withheld(s, id, params)
	if !strings.Contains(said, fineGrainedPhaseA) || !strings.Contains(said, reason) {
		e.T.Errorf("%s was withheld without saying %q and %q: %s", id, fineGrainedPhaseA, reason, said)
	}
}

// TestFineGrained_LicensedActionsNoTokenReaches_AreWithheldWithTheReason
// starts a session on a token that cannot read the instance's version, so its
// grant is not evaluated (phase A), and asks for three licensed actions no
// fine-grained token reaches at the recorded release: an epic read GitLab
// answers null, a security attribute write it commits and answers null, and a
// vulnerability count it answers null. Each is withheld on every surface,
// naming the GraphQL type and what would let the grant be judged. The
// arguments never reach GitLab, so they are placeholders.
func TestFineGrained_LicensedActionsNoTokenReaches_AreWithheldWithTheReason(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		user := fixture.NewUser(e, "fglicensed")
		return fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()[0])
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Tier: harness.TierUltimate})
		authority := s.Authority()
		if authority == nil || authority.Phase() != finegrained.PhaseUnknown || authority.Fallback() != finegrained.FallbackVersionUnreadable {
			e.T.Fatalf("a token refused Metadata: Read is judged %+v, want phase A for want of a readable version", authority)
		}

		expectWithheldFor(e, s, actionEpicGet, map[string]any{"full_path": "a/b", "epic_iid": 1}, "Namespace")
		expectWithheldFor(e, s, actionSecurityAttributeCreate, map[string]any{
			"namespace_id": 1, "category_id": 1,
			"attributes": []map[string]any{{"name": "withheld", "description": "never created", "color": attributeColor}},
		}, "SecurityAttribute")
		expectWithheldFor(e, s, actionVulnerabilitySeverityCount, map[string]any{"project_path": "a/b"}, "VulnerabilitySeveritiesCount")
	})
}

// grantedVulnerabilityFixture is a project a user develops in and a token
// granted the project and its vulnerabilities.
type grantedVulnerabilityFixture struct {
	project fixture.Project
	token   fixture.Token
}

// TestFineGrained_VulnerabilityReads_AreServedWithWhatGitLabLeavesEmpty
// grants a project's vulnerabilities: the listing is served, and its answer
// carries the note naming the parts GitLab leaves empty for every
// fine-grained token (a finding's token status among them) and that an empty
// list may be the credential's; a read of a vulnerability nothing has is
// served too, and its not-found answer says that may be the credential.
func TestFineGrained_VulnerabilityReads_AreServedWithWhatGitLabLeavesEmpty(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) grantedVulnerabilityFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("fgvuln"))
		user := fixture.NewUser(e, "fgvuln")
		fixture.AddProjectMember(e, project, user, gl.DeveloperPermissions)
		return grantedVulnerabilityFixture{project: project, token: fixture.NewFineGrainedToken(e, user, append(fixture.StartupScopes(),
			fixture.GranularScope{
				Access: fixture.AccessSelectedMemberships, ProjectIDs: []int64{project.ID},
				Permissions: []string{fineGrantReadProject, fineGrantReadVulnerability},
			})...)}
	}, func(e *harness.Env, surface harness.Surface, f grantedVulnerabilityFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value, Tier: harness.TierUltimate})
		fineGrainedPhaseOf(e, s)

		listed, said := harness.DoAnswer[vulnerabilities.ListOutput](s, actionVulnerabilityList, map[string]any{"project_path": f.project.Path})
		if len(listed.Vulnerabilities) != 0 {
			e.T.Errorf("a project nothing scanned lists %d vulnerabilities", len(listed.Vulnerabilities))
		}
		if !strings.Contains(said, fineGrainedDegraded) || !strings.Contains(said, "findingTokenStatus") {
			e.T.Errorf("the listing does not say what GitLab leaves empty for the credential: %s", said)
		}

		harness.ExpectToolError(s, actionVulnerabilityGet, map[string]any{"id": "gid://gitlab/Vulnerability/999999999"}, fineGrainedNull)
	})
}

// TestFineGrained_PublicProjectAttestations_AreServedToAGrantWithoutThem lists
// a public project's attestations with a token granted nothing past the
// startup scopes: the read is not listed for it, since the grant does not hold
// Attestation: Read, and is still served, since GitLab answers it on a public
// project whoever asks; what GitLab answers is its own, and never the refusal
// of a permission.
func TestFineGrained_PublicProjectAttestations_AreServedToAGrantWithoutThem(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	harness.SurfacesWith(e, func(e *harness.Env) grantedVulnerabilityFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("fgattest"), fixture.WithVisibility(gl.PublicVisibility))
		user := fixture.NewUser(e, "fgattest")
		return grantedVulnerabilityFixture{project: project, token: fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()...)}
	}, func(e *harness.Env, surface harness.Surface, f grantedVulnerabilityFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value, Tier: harness.TierUltimate})
		if !fineGrainedPhaseOf(e, s) {
			// Phase A lists every action GitLab may serve, this one included.
			return
		}

		_, err := harness.Try[attestations.ListOutput](s, actionAttestationList, map[string]any{
			"project_id": f.project.IDParam(), "subject_digest": emptySubjectDigest,
		})
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "access denied") {
			e.T.Errorf("the public project's attestations were refused for a permission: %v", err)
		}
	})
}

// workItemQuery reads one work item by its global id.
const workItemQuery = `query($id: WorkItemID!) { workItem(id: $id) { id title } }`

// workItemRename renames one work item and answers with it.
const workItemRename = `mutation($id: WorkItemID!, $title: String!) {
  workItemUpdate(input: { id: $id, title: $title }) { workItem { id title } errors }
}`

// TestFineGrainedProbes_GroupWorkItem_ResolvesNoBoundary measures what the
// group-work-item declarations of cmd/gen_action_grants rest on: GitLab
// declares WorkItem at the project boundary only, so an epic, a group's work
// item, resolves no boundary for a fine-grained token granted work items on
// its group. A read of it answers null with no error where the classic token
// reads it, and a write commits, answering null with no error, which is why
// the server withholds the epic actions rather than letting a model take
// either answer for the truth.
func TestFineGrainedProbes_GroupWorkItem_ResolvesNoBoundary(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))
	group := fixture.NewGroup(e)
	epic := fixture.NewEpic(e, group, "fine-grained boundary")
	user := fixture.NewUser(e, "fgepic")
	fixture.AddGroupMember(e, group, user, gl.MaintainerPermissions)
	token := fixture.NewFineGrainedToken(e, user, append(fixture.StartupScopes(), fixture.GranularScope{
		Access: fixture.AccessSelectedMemberships, GroupIDs: []int64{group.ID},
		Permissions: []string{fineGrantReadWorkItem, fineGrantUpdateWorkItem},
	})...)
	id := map[string]any{"id": "gid://gitlab/WorkItem/" + strconv.FormatInt(epic.ID, 10)}

	classic := fixture.ProbeGraphQL(e, fixture.Token{}, workItemQuery, id)
	fine := fixture.ProbeGraphQL(e, token, workItemQuery, id)
	if len(classic.Errors) > 0 || string(classic.Data["workItem"]) == "null" {
		t.Fatalf("the classic token could not read the epic: %s %v", classic.Data["workItem"], classic.Errors)
	}
	if len(fine.Errors) > 0 || string(fine.Data["workItem"]) != "null" {
		t.Errorf("the fine-grained read of the epic answered %s %v, want null with no error", fine.Data["workItem"], fine.Errors)
	}

	renamed := "renamed by a fine-grained token"
	written := fixture.ProbeGraphQL(e, token, workItemRename, map[string]any{"id": id["id"], "title": renamed})
	var payload struct {
		WorkItem json.RawMessage `json:"workItem"`
		Errors   []string        `json:"errors"`
	}
	if err := json.Unmarshal(written.Data["workItemUpdate"], &payload); err != nil || len(written.Errors) > 0 {
		t.Fatalf("the fine-grained write answered %s %v: %v", written.Data["workItemUpdate"], written.Errors, err)
	}
	if string(payload.WorkItem) != "null" || len(payload.Errors) > 0 {
		t.Errorf("the fine-grained write answered %s with errors %v, want null and none", payload.WorkItem, payload.Errors)
	}
	after := fixture.ProbeGraphQL(e, fixture.Token{}, workItemQuery, id)
	if !strings.Contains(string(after.Data["workItem"]), renamed) {
		t.Errorf("the write did not commit: the epic reads %s", after.Data["workItem"])
	}
}

// TestFineGrainedProbes_SeverityCount_IsNullWithNoError measures the read the
// vulnerability count rests on: GitLab declares no fine-grained permission on
// VulnerabilitySeveritiesCount, so a token granted the project's
// vulnerabilities reads null with no error where the classic token reads the
// counts.
func TestFineGrainedProbes_SeverityCount_IsNullWithNoError(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))
	project := fixture.NewProject(e, fixture.WithNamePrefix("fgcount"))
	user := fixture.NewUser(e, "fgcount")
	fixture.AddProjectMember(e, project, user, gl.DeveloperPermissions)
	token := fixture.NewFineGrainedToken(e, user, append(fixture.StartupScopes(), fixture.GranularScope{
		Access: fixture.AccessSelectedMemberships, ProjectIDs: []int64{project.ID},
		Permissions: []string{fineGrantReadProject, fineGrantReadVulnerability},
	})...)
	const document = `query($path: ID!) { project(fullPath: $path) { vulnerabilitySeveritiesCount { critical } } }`
	path := map[string]any{"path": project.Path}

	classic := fixture.ProbeGraphQL(e, fixture.Token{}, document, path)
	fine := fixture.ProbeGraphQL(e, token, document, path)
	if len(classic.Errors) > 0 || !strings.Contains(string(classic.Data["project"]), `"critical"`) {
		t.Fatalf("the classic token could not read the counts: %s %v", classic.Data["project"], classic.Errors)
	}
	if len(fine.Errors) > 0 || !strings.Contains(string(fine.Data["project"]), `"vulnerabilitySeveritiesCount":null`) {
		t.Errorf("the fine-grained count answered %s %v, want null with no error", fine.Data["project"], fine.Errors)
	}
}
