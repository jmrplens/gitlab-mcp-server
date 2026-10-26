// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// accessrequests_test.go covers the one part of an access request approval a
// license changes: Minimal access, which GitLab grants only on a top-level
// group whose license includes the role, and which an approval on an
// unlicensed instance answers with 201 while the request stays pending.

package ce

import (
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access request actions the scenario drives, at group scope.
const (
	actionAccessRequestGroup     harness.ActionID = "access.request_group"
	actionAccessRequestListGroup harness.ActionID = "access.request_list_group"
	actionAccessApproveGroup     harness.ActionID = "access.approve_group"
)

// TestAccessRequests_MinimalAccessApproval_OnTheFreeTier_IsRefusedAndTheRequestStaysPending
// has one fixture user per surface ask for access to a public group and the
// owner approve it at Minimal access (issue 1280). The session's tier is
// Free, where GitLab has no Minimal Access role, so the approval is refused
// before it is sent on every surface (the group's input schema lists 5, for
// the licensed top-level groups that grant it, so the refusal is the
// handler's own), and the request is still listed afterwards.
func TestAccessRequests_MinimalAccessApproval_OnTheFreeTier_IsRefusedAndTheRequestStaysPending(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildRequestableGroup, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		requester := fixture.NewUser(e, "requester")
		token := fixture.NewToken(e, requester, "api")
		owner := e.On(surface)
		asking := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Private: true})

		scope := map[string]any{"group_id": group.IDParam()}
		if requested := harness.Do[accessrequests.Output](asking, actionAccessRequestGroup, scope); requested.ID != requester.ID {
			e.T.Errorf("the request for access answered user %d, want the requester %d", requested.ID, requester.ID)
		}
		refusal := harness.ExpectToolError(owner, actionAccessApproveGroup,
			map[string]any{"group_id": group.IDParam(), "user_id": requester.ID, "access_level": int(gl.MinimalAccessPermissions)}, "Free tier")
		if !strings.Contains(refusal, "top-level group") {
			e.T.Errorf("the refusal does not say where Minimal access is granted: %q", refusal)
		}

		pending := harness.Do[accessrequests.ListOutput](owner, actionAccessRequestListGroup, scope)
		still := false
		for _, request := range pending.AccessRequests {
			still = still || request.ID == requester.ID
		}
		if !still {
			e.T.Errorf("the request of user %d is no longer pending after the refused approval: %+v", requester.ID, pending.AccessRequests)
		}
	})
}

// buildRequestableGroup creates a public group once on the parent Env and
// opens it to access requests, since a non-member can only ask for access to
// a group it can see, and only where the setting allows it.
func buildRequestableGroup(e *harness.Env) fixture.Group {
	e.T.Helper()

	group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("areq"), fixture.WithGroupVisibility(gl.PublicVisibility))
	if _, _, err := e.Client().GL().Groups.UpdateGroup(group.ID, &gl.UpdateGroupOptions{RequestAccessEnabled: new(true)}, gl.WithContext(e.Ctx)); err != nil {
		e.T.Fatalf("enabling access requests on group %d: %v", group.ID, err)
	}
	return group
}
