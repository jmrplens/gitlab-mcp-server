// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// accessrequests_test.go covers access requests from both sides. A
// requester is a fixture user with a token of its own, since a request has
// to come from an authenticated non-member, and it asks from a session the
// harness starts on that token. The owner, the run user, lists the request,
// denies it, and when the same user asks again, approves it; the same
// sequence runs on a project and on a group, both made public with access
// requests enabled so a non-member can see them and ask.

package common

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// accessRequestFixture is what the requests are made against: a public
// project and a public group, both accepting requests.
type accessRequestFixture struct {
	project fixture.Project
	group   fixture.Group
}

// accessRequestParties is who takes part in one scope's sequence: the
// owner's session, the requester's session and the requester's user ID.
type accessRequestParties struct {
	owner       *harness.Session
	asking      *harness.Session
	requesterID int64
}

// TestAccessRequests_Lifecycle_RequestDenyAskAgainApprove has one fixture
// user per surface request access to the project and to the group from a
// session on its own token, is listed as pending by the owner, is denied,
// asks again and is approved as a developer, on each of the two scopes.
//
// Replaces: TestIndividual_AccessRequests, TestMeta_AccessRequests
func TestAccessRequests_Lifecycle_RequestDenyAskAgainApprove(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildAccessRequestFixture, func(e *harness.Env, surface harness.Surface, f accessRequestFixture) {
		requester := fixture.NewUser(e, "requester")
		token := fixture.NewToken(e, requester, accessTokenScope)
		parties := accessRequestParties{
			owner:       e.On(surface),
			asking:      e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Private: true}),
			requesterID: requester.ID,
		}

		assertAccessRequestFlow(e, parties, "project "+f.project.IDParam(), map[string]any{"project_id": f.project.IDParam()},
			actionAccessRequestProject, actionAccessRequestListProject, actionAccessDenyProject, actionAccessApproveProject)
		assertAccessRequestFlow(e, parties, "group "+f.group.IDParam(), map[string]any{"group_id": f.group.IDParam()},
			actionAccessRequestGroup, actionAccessRequestListGroup, actionAccessDenyGroup, actionAccessApproveGroup)
	})
}

// TestAccessRequests_Approval_LevelNotGranted_IsRefusedAndTheRequestStaysPending
// has one fixture user per surface ask for access to the project and to the
// group, and the owner approve each at a level the scope does not grant: 0
// and Minimal access on the project, 0 on the group (issue 1280). GitLab
// answers such an approval with 201 and the requester while the request
// stays pending, and this server used to drop a 0 and approve the requester
// as a Developer, so each is refused before the approval is sent and the
// request is still listed after it. An approval naming no level then grants
// Developer, GitLab's default.
//
// The refusal comes from two places: the individual surface's input schema
// lists the levels, so the SDK refuses the call as invalid params, and on the
// other two the handler refuses it as a tool error.
func TestAccessRequests_Approval_LevelNotGranted_IsRefusedAndTheRequestStaysPending(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildAccessRequestFixture, func(e *harness.Env, surface harness.Surface, f accessRequestFixture) {
		requester := fixture.NewUser(e, "requester")
		token := fixture.NewToken(e, requester, accessTokenScope)
		owner := e.On(surface)
		asking := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Private: true})

		project := map[string]any{"project_id": f.project.IDParam()}
		assertRequested(e, harness.Do[accessrequests.Output](asking, actionAccessRequestProject, project), requester.ID)
		for _, level := range []int{0, int(gl.MinimalAccessPermissions)} {
			refuseApproval(e, owner, surface, actionAccessApproveProject, withParams(project, map[string]any{"user_id": requester.ID, "access_level": level}))
		}
		assertStillPending(e, owner, actionAccessRequestListProject, project, requester.ID)
		approved := harness.Do[accessrequests.MemberOutput](owner, actionAccessApproveProject, withParams(project, map[string]any{"user_id": requester.ID}))
		if approved.ID != requester.ID || approved.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("the approval with no level answered user %d at level %d, want %d as a developer (%d)", approved.ID, approved.AccessLevel, requester.ID, gl.DeveloperPermissions)
		}

		group := map[string]any{"group_id": f.group.IDParam()}
		assertRequested(e, harness.Do[accessrequests.Output](asking, actionAccessRequestGroup, group), requester.ID)
		refuseApproval(e, owner, surface, actionAccessApproveGroup, withParams(group, map[string]any{"user_id": requester.ID, "access_level": 0}))
		assertStillPending(e, owner, actionAccessRequestListGroup, group, requester.ID)
	})
}

// assertRequested asserts that a request for access answered the requester.
func assertRequested(e *harness.Env, requested accessrequests.Output, requesterID int64) {
	e.T.Helper()
	if requested.ID != requesterID {
		e.T.Errorf("the request for access answered user %d, want the requester %d", requested.ID, requesterID)
	}
}

// refuseApproval asserts that an approval at a level the scope does not
// grant is refused, in the shape the surface refuses it in.
func refuseApproval(e *harness.Env, s *harness.Session, surface harness.Surface, approve harness.ActionID, params map[string]any) {
	e.T.Helper()
	if surface == harness.SurfaceIndividual {
		harness.Refused(s, approve, params, harness.FailureInvalidParams)
		return
	}
	harness.ExpectToolError(s, approve, params, "omit access_level")
}

// assertStillPending asserts that the requester's request is still listed
// as pending after the refused approvals.
func assertStillPending(e *harness.Env, owner *harness.Session, list harness.ActionID, scope map[string]any, requesterID int64) {
	e.T.Helper()
	pending := harness.Do[accessrequests.ListOutput](owner, list, scope)
	if !containsID(accessRequesterIDs(pending.AccessRequests), requesterID) {
		e.T.Errorf("the request of user %d is no longer pending after the refused approvals: %+v", requesterID, pending.AccessRequests)
	}
}

// buildAccessRequestFixture creates the project and the group once on the
// parent Env and opens both to requests: a non-member can only ask for
// access to something it can see, and only where the setting allows it.
func buildAccessRequestFixture(e *harness.Env) accessRequestFixture {
	e.T.Helper()

	project := fixture.NewProject(e, fixture.WithNamePrefix("areq"), fixture.WithVisibility(gl.PublicVisibility))
	if _, _, err := e.Client().GL().Projects.EditProject(project.ID, &gl.EditProjectOptions{RequestAccessEnabled: new(true)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("enabling access requests on project %d: %v", project.ID, err)
	}
	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("areq"), fixture.WithGroupVisibility(gl.PublicVisibility))
	if _, _, err := e.Client().GL().Groups.UpdateGroup(group.ID, &gl.UpdateGroupOptions{RequestAccessEnabled: new(true)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("enabling access requests on group %d: %v", group.ID, err)
	}
	return accessRequestFixture{project: project, group: group}
}

// assertAccessRequestFlow runs one scope's sequence with the four actions
// that scope has: the requester asks, the owner finds the request listed,
// denies it and finds it gone, the requester asks again, and the owner
// approves it as a developer.
func assertAccessRequestFlow(e *harness.Env, parties accessRequestParties, target string, scope map[string]any, request, list, deny, approve harness.ActionID) {
	e.T.Helper()

	requested := harness.Do[accessrequests.Output](parties.asking, request, scope)
	if requested.ID != parties.requesterID {
		e.T.Errorf("the request for %s answered user %d, want the requester %d", target, requested.ID, parties.requesterID)
	}

	pending := harness.Do[accessrequests.ListOutput](parties.owner, list, scope)
	if !containsID(accessRequesterIDs(pending.AccessRequests), parties.requesterID) {
		e.T.Errorf("the pending requests for %s do not hold user %d: %+v", target, parties.requesterID, pending.AccessRequests)
	}

	requester := withParams(scope, map[string]any{"user_id": parties.requesterID})
	harness.DoVoid(parties.owner, deny, requester)
	afterDeny := harness.Do[accessrequests.ListOutput](parties.owner, list, scope)
	if containsID(accessRequesterIDs(afterDeny.AccessRequests), parties.requesterID) {
		e.T.Errorf("the request of user %d for %s is still pending after its denial: %+v", parties.requesterID, target, afterDeny.AccessRequests)
	}

	again := harness.Do[accessrequests.Output](parties.asking, request, scope)
	if again.ID != parties.requesterID {
		e.T.Errorf("the second request for %s answered user %d, want the requester %d", target, again.ID, parties.requesterID)
	}
	// Approving is the one route of this family GitLab answers with a
	// membership rather than a pending request, so it is the one read back as
	// the member shape and the only one with a level on it.
	approved := harness.Do[accessrequests.MemberOutput](parties.owner, approve, withParams(requester, map[string]any{"access_level": int(gl.DeveloperPermissions)}))
	if approved.ID != parties.requesterID || approved.AccessLevel != int(gl.DeveloperPermissions) {
		e.T.Errorf("the approval for %s answered user %d at level %d, want %d as a developer (%d)", target, approved.ID, approved.AccessLevel, parties.requesterID, gl.DeveloperPermissions)
	}
}

// accessRequesterIDs collects the user IDs of listed access requests.
func accessRequesterIDs(listed []accessrequests.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, request := range listed {
		ids = append(ids, request.ID)
	}
	return ids
}
