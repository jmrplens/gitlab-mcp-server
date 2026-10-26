// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// accessrequests_test.go covers the access request approval a license
// decides: Minimal access, which GitLab grants only on a top-level group whose
// license includes the role, and which an approval anywhere else answers with
// 201 while the request stays pending.

package ee

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/accessrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// minimalAccessGroups is a public top-level group and a public subgroup of
// it, both open to access requests.
type minimalAccessGroups struct {
	top fixture.Group
	sub fixture.Group
}

// TestAccessRequests_MinimalAccessApproval_IsRefusedOnASubgroupAndGrantedOnATopLevelGroup
// has one fixture user per surface ask for access to a subgroup and to its
// top-level group, and the owner approve both at Minimal access (issue
// 1280). On the subgroup the approval is refused before it is sent, since
// GitLab would answer it with 201 and leave the request pending, and the
// request is still listed; on the top-level group of this licensed instance
// it is granted, and answered as the membership at Minimal access only once
// the requester is read back among the group's members.
func TestAccessRequests_MinimalAccessApproval_IsRefusedOnASubgroupAndGrantedOnATopLevelGroup(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, buildMinimalAccessGroups, func(e *harness.Env, surface harness.Surface, groups minimalAccessGroups) {
		requester := fixture.NewUser(e, "requester")
		token := fixture.NewToken(e, requester, "api")
		owner := e.On(surface)
		asking := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Private: true})
		minimal := int(gl.MinimalAccessPermissions)

		sub := map[string]any{"group_id": groups.sub.IDParam()}
		if requested := harness.Do[accessrequests.Output](asking, actionAccessRequestGroup, sub); requested.ID != requester.ID {
			e.T.Errorf("the request for the subgroup answered user %d, want the requester %d", requested.ID, requester.ID)
		}
		refusal := harness.ExpectToolError(owner, actionAccessApproveGroup,
			map[string]any{"group_id": groups.sub.IDParam(), "user_id": requester.ID, "access_level": minimal}, "subgroup")
		assertMentions(e, "the subgroup refusal", refusal, "top-level group", "access_level")
		pending := harness.Do[accessrequests.ListOutput](owner, actionAccessRequestListGroup, sub)
		if !containsID(requesterIDs(pending.AccessRequests), requester.ID) {
			e.T.Errorf("the request of user %d for the subgroup is no longer pending after the refused approval: %+v", requester.ID, pending.AccessRequests)
		}

		top := map[string]any{"group_id": groups.top.IDParam()}
		if requested := harness.Do[accessrequests.Output](asking, actionAccessRequestGroup, top); requested.ID != requester.ID {
			e.T.Errorf("the request for the top-level group answered user %d, want the requester %d", requested.ID, requester.ID)
		}
		approved := harness.Do[accessrequests.MemberOutput](owner, actionAccessApproveGroup,
			map[string]any{"group_id": groups.top.IDParam(), "user_id": requester.ID, "access_level": minimal})
		if approved.ID != requester.ID || approved.AccessLevel != minimal {
			e.T.Errorf("the top-level approval answered user %d at level %d, want %d at Minimal access (%d)", approved.ID, approved.AccessLevel, requester.ID, minimal)
		}
	})
}

// buildMinimalAccessGroups creates the two groups once on the parent Env and
// opens both to requests: a non-member can only ask for access to a group it
// can see, and only where the setting allows it.
func buildMinimalAccessGroups(e *harness.Env) minimalAccessGroups {
	e.T.Helper()

	top := fixture.NewGroup(e, fixture.WithGroupNamePrefix("areq"), fixture.WithGroupVisibility(gl.PublicVisibility))
	sub := fixture.NewSubgroup(e, top, fixture.WithGroupNamePrefix("areq-sub"), fixture.WithGroupVisibility(gl.PublicVisibility))
	for _, group := range []fixture.Group{top, sub} {
		if _, _, err := e.Client().GL().Groups.UpdateGroup(group.ID, &gl.UpdateGroupOptions{RequestAccessEnabled: new(true)}, gl.WithContext(e.Ctx)); err != nil {
			e.T.Fatalf("enabling access requests on group %d: %v", group.ID, err)
		}
	}
	return minimalAccessGroups{top: top, sub: sub}
}

// requesterIDs collects the user IDs of listed access requests.
func requesterIDs(listed []accessrequests.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, request := range listed {
		ids = append(ids, request.ID)
	}
	return ids
}
