//go:build e2e

// memberroles_test.go covers custom member roles at the two levels GitLab
// offers them: the instance level, which a self-managed Ultimate instance
// serves, and the group level, which it refuses as deprecated and points at
// the instance level instead. The group half is therefore a refusal on the
// runtime this package targets, and it is asserted as one.

package ee

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/memberroles"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The access level a custom role is based on: Guest, the lowest GitLab
// accepts for one.
const memberRoleBaseAccessLevel = int64(10)

// memberRoleIDs lists the IDs of a role listing.
func memberRoleIDs(roles []memberroles.Output) []int64 {
	ids := make([]int64, 0, len(roles))
	for _, role := range roles {
		ids = append(ids, role.ID)
	}
	return ids
}

// TestMemberRoles_InstanceLevel_CreatesListsAndDeletes creates an instance
// custom role on every surface, finds it in the listing, and deletes it.
//
// Replaces: TestMeta_MemberRoles
func TestMemberRoles_InstanceLevel_CreatesListsAndDeletes(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		before := harness.Do[memberroles.ListOutput](s, actionMemberRoleListInstance, nil)
		e.T.Logf("%d instance member role(s) before the create", len(before.Roles))

		created := harness.Do[memberroles.Output](s, actionMemberRoleCreateInstance, map[string]any{
			"name": e.Name("role"), "base_access_level": memberRoleBaseAccessLevel,
			"description": "e2e instance custom role", "read_code": true,
		})
		if created.ID == 0 {
			e.T.Fatalf("create_instance answered %+v, want a role with an ID", created)
		}
		e.Defer("instance member role", func(ctx context.Context) error {
			_, err := e.Client().GL().MemberRolesService.DeleteInstanceMemberRole(created.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		after := harness.Do[memberroles.ListOutput](s, actionMemberRoleListInstance, nil)
		if !containsID(memberRoleIDs(after.Roles), created.ID) {
			e.T.Errorf("the instance listing does not hold the created role %d: %v", created.ID, memberRoleIDs(after.Roles))
		}

		harness.DoVoid(s, actionMemberRoleDeleteInstance, map[string]any{"member_role_id": created.ID})
		gone := harness.Do[memberroles.ListOutput](s, actionMemberRoleListInstance, nil)
		if containsID(memberRoleIDs(gone.Roles), created.ID) {
			e.T.Errorf("the instance listing still holds role %d after its delete", created.ID)
		}
	})
}

// TestMemberRoles_GroupLevel_IsRefusedAsDeprecatedOnSelfManaged asks for
// the group-level roles of a fresh group on every surface and checks that
// all three group actions are refused with the hint naming the instance
// level, which is where a self-managed instance keeps custom roles.
//
// Replaces: TestMeta_MemberRoles
func TestMemberRoles_GroupLevel_IsRefusedAsDeprecatedOnSelfManaged(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))
	if e.Client().IsGitLabDotCom() {
		e.Skipf("group-level custom roles are served on GitLab.com, and this scenario asserts the self-managed refusal")
	}

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("roles"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		hint := []string{"deprecated", "self-managed", "instance-level"}

		refused := harness.ExpectToolError(s, actionMemberRoleListGroup, map[string]any{"group_id": group.IDParam()}, "deprecated")
		assertMentions(e, "the group role listing", refused, hint...)

		refused = harness.ExpectToolError(s, actionMemberRoleCreateGroup, map[string]any{
			"group_id": group.IDParam(), "name": e.Name("role"), "base_access_level": memberRoleBaseAccessLevel,
			"description": "e2e group custom role", "read_code": true,
		}, "deprecated")
		assertMentions(e, "the group role create", refused, hint...)

		refused = harness.ExpectToolError(s, actionMemberRoleDeleteGroup, map[string]any{"group_id": group.IDParam(), "member_role_id": int64(1)}, "deprecated")
		assertMentions(e, "the group role delete", refused, hint...)
	})
}
