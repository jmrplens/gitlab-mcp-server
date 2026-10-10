//go:build e2e

// fine_grained_graphql_test.go covers what a fine-grained personal access
// token (issue 952) meets in the licensed GraphQL surface: the reads and
// writes no such token can reach at the recorded release, which the server
// withholds with the reason; the vulnerability reads it can, which are served
// with a note naming what GitLab leaves empty for the credential; a dismissal
// GitLab commits and answers without the vulnerability, for a grant that may
// change it and not read it; a public project's attestations, served to a
// grant that does not hold them; and, by
// direct probes that bypass the server, what GitLab answers a group's work
// item, which is what the table's boundary cause rests on.
//
// Every session pins Ultimate: a fine-grained token cannot read the license,
// so a session left to detect would be served the Free catalog and none of
// the actions asked about here.

package ee

import (
	"encoding/json"
	"slices"
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
	fineGrantReadProject         = "read_project"
	fineGrantReadVulnerability   = "read_vulnerability"
	fineGrantUpdateVulnerability = "update_vulnerability"
	fineGrantReadWorkItem        = "read_work_item"
	fineGrantUpdateWorkItem      = "update_work_item"
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

// fineGrainedPhaseOf holds a fine-grained session to what the instance's
// release decides, and reports what its grant decides there: the listing, the
// calls, both (phase B) or neither (phase A).
func fineGrainedPhaseOf(e *harness.Env, s *harness.Session) harness.FineGrainedJudgement {
	e.T.Helper()
	judgement, problem := s.FineGrainedPhase()
	if problem != "" {
		e.T.Fatal(problem)
	}
	return judgement
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
// startup scopes: where the grant decides the listing the read is not listed
// for it, since the grant does not hold Attestation: Read, and in every phase
// it is served, since GitLab answers it on a public project whoever asks, with
// the project's attestations, of which there are none.
//
// GitLab answers every attestation route 404 while slsa_provenance_statement
// is off, and the flag ships off (attestations_test.go says why that shapes
// the sibling scenario), so the flag is pinned on here: without it the listing
// is refused for the flag and says nothing about the grant. A release that no
// longer defines the flag has dropped the check with it, which is how GitLab
// retires a flag once its routes are generally available, so the pin is made
// only where the flag exists and the listing is asserted either way.
func TestFineGrained_PublicProjectAttestations_AreServedToAGrantWithoutThem(t *testing.T) {
	e := harness.New(t,
		harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)),
		harness.Locks(harness.LockInstanceGlobal))
	if fixture.FeatureDefined(e, attestations.FeatureFlag) {
		fixture.PinFeature(e, attestations.FeatureFlag, true)
	}

	harness.SurfacesWith(e, func(e *harness.Env) grantedVulnerabilityFixture {
		project := fixture.NewProject(e, fixture.WithNamePrefix("fgattest"), fixture.WithVisibility(gl.PublicVisibility))
		user := fixture.NewUser(e, "fgattest")
		return grantedVulnerabilityFixture{project: project, token: fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()...)}
	}, func(e *harness.Env, surface harness.Surface, f grantedVulnerabilityFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value, Tier: harness.TierUltimate})
		// Phase A lists every action GitLab may serve, this one included, so
		// only a grant that decides the listing leaves it out.
		if fineGrainedPhaseOf(e, s).Listing && slices.Contains(s.Actions(), actionAttestationList) {
			e.T.Errorf("%s is listed for a token whose grant does not hold Attestation: Read", actionAttestationList)
		}

		listed := harness.Do[attestations.ListOutput](s, actionAttestationList, map[string]any{
			"project_id": f.project.IDParam(), "subject_digest": emptySubjectDigest,
		})
		if len(listed.Attestations) != 0 {
			e.T.Errorf("a public project that attested nothing lists %d attestations: %+v", len(listed.Attestations), listed.Attestations)
		}
	})
}

// vulnerabilityStateQuery reads one vulnerability's state by its global id,
// written here rather than borrowed from the tool under test.
const vulnerabilityStateQuery = `query($id: VulnerabilityID!) { vulnerability(id: $id) { id state } }`

// grantedDismissalFixture is a project whose pipeline reported
// vulnerabilities, and a token of one of its maintainers granted Vulnerability:
// Update there and not Vulnerability: Read.
type grantedDismissalFixture struct {
	report vulnerabilityFixture
	token  fixture.Token
}

// TestFineGrained_VulnerabilityDismissWithoutRead_IsAnsweredAsProbablyCommitted
// dismisses a vulnerability with a token granted Vulnerability: Update and not
// Vulnerability: Read, which is a state change the server serves. The token
// cannot read the instance's version, so its grant is not evaluated and the
// call reaches GitLab. GitLab checks the token against the mutation before it
// runs and against the vulnerability it answers with only after, so the
// dismissal commits and the vulnerability comes back null with no error: the
// server says the write was probably committed rather than handing back an
// empty vulnerability as the result, and the run's classic token reads the
// vulnerability dismissed. Each surface dismisses a vulnerability of its own.
func TestFineGrained_VulnerabilityDismissWithoutRead_IsAnsweredAsProbablyCommitted(t *testing.T) {
	e := harness.New(t,
		harness.Needs(harness.NeedAdmin, harness.NeedRunner, harness.Tier(edition.Ultimate)),
		harness.Locks(harness.LockRunner))

	harness.SurfacesWith(e, func(e *harness.Env) grantedDismissalFixture {
		report := buildVulnerabilityFixture(e)
		user := fixture.NewUser(e, "fgdismiss")
		fixture.AddProjectMember(e, report.project, user, gl.MaintainerPermissions)
		return grantedDismissalFixture{report: report, token: fixture.NewFineGrainedToken(e, user, fixture.StartupScopes()[0],
			fixture.GranularScope{
				Access: fixture.AccessSelectedMemberships, ProjectIDs: []int64{report.project.ID},
				Permissions: []string{fineGrantReadProject, fineGrantUpdateVulnerability},
			})}
	}, func(e *harness.Env, surface harness.Surface, f grantedDismissalFixture) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: f.token.Value, Tier: harness.TierUltimate})
		if authority := s.Authority(); authority == nil || authority.Phase() != finegrained.PhaseUnknown {
			e.T.Fatalf("a token refused Metadata: Read is judged %+v, want phase A, which serves the dismissal", authority)
		}
		own := f.report.reported[slices.Index(harness.AllSurfaces(), surface)%len(f.report.reported)]

		harness.ExpectToolError(s, actionVulnerabilityDismiss, map[string]any{"id": own.ID}, "probably committed")

		after := fixture.ProbeGraphQL(e, fixture.Token{}, vulnerabilityStateQuery, map[string]any{"id": own.ID})
		if len(after.Errors) > 0 || !strings.Contains(string(after.Data["vulnerability"]), stateDismissed) {
			e.T.Errorf("the dismissal did not commit: the classic token reads %s %v", after.Data["vulnerability"], after.Errors)
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

// fineGrantCreateSecurityAttribute is the assignable permission that grants
// securityAttributeCreate, by the name GitLab's token creation route takes.
const fineGrantCreateSecurityAttribute = "create_security_attribute"

// TestFineGrainedProbes_SecurityAttributeCreate_CommitsAndNullsTheList
// measures what the security_attribute.create row of the table rests on, and
// what the handler's probably-committed answer describes (issue 1103): a token
// granted Security Attribute: Create on a group passes the mutation's own
// check, so GitLab creates the attribute, and then checks each SecurityAttribute
// the payload returns. That type declares no fine-grained permission at
// 19.4.1, and the payload lists the attributes as non-null items, so one null
// item nulls the whole list and GitLab's GraphQL library writes the error that
// says so. The run's classic token then finds the attribute in its category.
func TestFineGrainedProbes_SecurityAttributeCreate_CommitsAndNullsTheList(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("fgsecattr"))
	category := newSecurityCategory(e, e.On(harness.SurfaceMeta), group, true)
	user := fixture.NewUser(e, "fgsecattr")
	fixture.AddGroupMember(e, group, user, gl.OwnerPermissions)
	token := fixture.NewFineGrainedToken(e, user, append(fixture.StartupScopes(), fixture.GranularScope{
		Access: fixture.AccessSelectedMemberships, GroupIDs: []int64{group.ID},
		Permissions: []string{fineGrantCreateSecurityAttribute},
	})...)
	name := e.Name("fg-attribute")

	written := fixture.ProbeGraphQL(e, token, `mutation($namespace: NamespaceID!, $category: SecurityCategoryID!, $name: String!) {
  securityAttributeCreate(input: {namespaceId: $namespace, categoryId: $category,
    attributes: [{name: $name, description: "created by a fine-grained token", color: "#FF0000"}]}) { securityAttributes { id name } errors }
}`, map[string]any{
		"namespace": "gid://gitlab/Namespace/" + strconv.FormatInt(group.ID, 10),
		"category":  "gid://gitlab/Security::Category/" + strconv.FormatInt(category.ID, 10),
		"name":      name,
	})
	var payload struct {
		SecurityAttributes json.RawMessage `json:"securityAttributes"`
		Errors             []string        `json:"errors"`
	}
	if err := json.Unmarshal(written.Data["securityAttributeCreate"], &payload); err != nil {
		t.Fatalf("the fine-grained write answered %s %v: %v", written.Data["securityAttributeCreate"], written.Errors, err)
	}
	if string(payload.SecurityAttributes) != "null" || len(payload.Errors) > 0 {
		t.Errorf("the fine-grained write answered attributes %s and errors %v, want null and none", payload.SecurityAttributes, payload.Errors)
	}
	// The list is [SecurityAttribute!], itself nullable, so graphql-ruby names
	// the element's type without its "!".
	const nulledElement = "Cannot return null for non-nullable element of type 'SecurityAttribute' for SecurityAttributeCreatePayload.securityAttributes"
	if !slices.ContainsFunc(written.Errors, func(m string) bool { return strings.HasPrefix(m, nulledElement) }) {
		t.Errorf("the fine-grained write answered errors %v, want the one %q writes", written.Errors, nulledElement)
	}

	after := fixture.ProbeGraphQL(e, fixture.Token{}, `query($group: ID!) { group(fullPath: $group) { securityCategories { securityAttributes { name } } } }`,
		map[string]any{"group": group.Path})
	if len(after.Errors) > 0 || !strings.Contains(string(after.Data["group"]), `"`+name+`"`) {
		t.Errorf("the classic read of the group's categories answered %s %v, want the attribute %s: the write did not commit", after.Data["group"], after.Errors, name)
	}
}
