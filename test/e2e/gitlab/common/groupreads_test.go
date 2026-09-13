//go:build e2e

// groupreads_test.go covers what a group answers about itself beside the
// lifecycle groups_test.go drives: the listing it appears in, its members,
// its subgroups, the groups shared with it and invited to it, and the
// places it could be transferred to; and the two create options of the
// old suite's client-go 2.41 scenario, read back off the group.

package common

import (
	"context"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupIDsOf lists the ids of a group listing.
func groupIDsOf(listed []groups.Output) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, group := range listed {
		ids = append(ids, group.ID)
	}
	return ids
}

// transferLocationIDs lists the ids of a transfer location listing.
func transferLocationIDs(listed []groups.TransferLocationOutput) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, location := range listed {
		ids = append(ids, location.ID)
	}
	return ids
}

// groupReadsFixture is a group with one subgroup under it and one sibling
// group beside it, which is what the reads are held to.
type groupReadsFixture struct {
	group    fixture.Group
	subgroup fixture.Group
	sibling  fixture.Group
}

// TestGroupReads_FixtureGroup_ListsAndDescribes reads one shared group on
// every surface: found in the listing by its name, read with its creator
// as its Owner among the members, its subgroup among the subgroups, no
// group shared with it or invited to it, and its sibling among the groups
// it could be transferred to.
//
// Replaces: TestIndividual_Groups, TestMeta_Groups
func TestGroupReads_FixtureGroup_ListsAndDescribes(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) groupReadsFixture {
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("reads"))
		return groupReadsFixture{
			group:    group,
			subgroup: fixture.NewSubgroup(e, group, fixture.WithGroupNamePrefix("child")),
			sibling:  fixture.NewGroup(e, fixture.WithGroupNamePrefix("sibling")),
		}
	}, func(e *harness.Env, surface harness.Surface, f groupReadsFixture) {
		s := e.On(surface)
		params := map[string]any{"group_id": f.group.IDParam()}
		user := e.Runtime()

		listed := harness.Do[groups.ListOutput](s, actionGroupList, map[string]any{"search": f.group.Name})
		if !containsID(groupIDsOf(listed.Groups), f.group.ID) {
			e.T.Errorf("the group listing searched for %q holds %v, want group %d", f.group.Name, groupIDsOf(listed.Groups), f.group.ID)
		}
		got := harness.Do[groups.DetailOutput](s, actionGroupGet, params)
		if got.ID != f.group.ID || got.FullPath != f.group.Path || got.Archived {
			e.T.Errorf("group get answered %+v, want the active group %d at %q", got, f.group.ID, f.group.Path)
		}

		members := harness.Do[groups.MemberListOutput](s, actionGroupMembers, params)
		owner := false
		for _, member := range members.Members {
			if member.ID == user.UserID && member.AccessLevel == int(gl.OwnerPermissions) {
				owner = true
			}
		}
		if !owner {
			e.T.Errorf("the group's members %+v do not hold its creator %d as an Owner", members.Members, user.UserID)
		}
		subgroups := harness.Do[groups.ListOutput](s, actionGroupSubgroups, params)
		if ids := groupIDsOf(subgroups.Groups); len(ids) != 1 || ids[0] != f.subgroup.ID {
			e.T.Errorf("the group lists the subgroups %v, want exactly %d", ids, f.subgroup.ID)
		}

		sharedWith := harness.Do[groups.ListOutput](s, actionGroupSharedWith, withParams(params, map[string]any{"min_access_level": int64(gl.GuestPermissions)}))
		if len(sharedWith.Groups) != 0 {
			e.T.Errorf("the group is shared with %v, and nothing shared it", groupIDsOf(sharedWith.Groups))
		}
		invited := harness.Do[groups.ListOutput](s, actionGroupInvitedGroups, withParams(params, map[string]any{"relation": []string{"direct"}}))
		if len(invited.Groups) != 0 {
			e.T.Errorf("the group has invited %v, and nothing was invited", groupIDsOf(invited.Groups))
		}
		locations := harness.Do[groups.TransferLocationsListOutput](s, actionGroupTransferLocations, withParams(params, map[string]any{"search": f.sibling.Name}))
		if ids := transferLocationIDs(locations.Locations); !containsID(ids, f.sibling.ID) || containsID(ids, f.group.ID) {
			e.T.Errorf("the group could be transferred to %v, want its sibling %d among them and never itself", ids, f.sibling.ID)
		}
	})
}

// TestGroupCreate_MathRenderingAndSnippets_ReadBack creates a group on
// every surface with the math rendering limits and personal snippets
// turned on, and reads back the one of the two a Free instance publishes:
// allow_personal_snippets is tagged premium on the group detail, so the
// catalog prunes it from the output schema at this tier and there is
// nothing to hold it to. The old test sent both and logged what came
// back without asserting either.
//
// Replaces: TestIndividual_GroupNewV241Fields
func TestGroupCreate_MathRenderingAndSnippets_ReadBack(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("flags")

		created := harness.Do[groups.DetailOutput](s, actionGroupCreate, map[string]any{
			"name": name, "path": name, "visibility": "private", "math_rendering_limits_enabled": true, "allow_personal_snippets": true,
		})
		if created.ID == 0 || created.Path != name {
			e.T.Fatalf("group create answered %+v, want the group %q with an ID", created, name)
		}
		e.Defer("group "+created.FullPath, func(ctx context.Context) error {
			return fixture.DeleteGroup(ctx, e.Client(), created.ID, created.FullPath)
		})

		got := harness.Do[groups.DetailOutput](s, actionGroupGet, map[string]any{"group_id": fixture.Group{ID: created.ID}.IDParam()})
		if got.ID != created.ID || got.Archived || !got.MathRenderingLimitsEnabled {
			e.T.Errorf("group get answered %+v, want the active group %d with math rendering limits on", got, created.ID)
		}
	})
}
