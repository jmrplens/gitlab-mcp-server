//go:build e2e

// groups_test.go covers a group's own lifecycle through the server, and the
// one membership write the old suite drove: create a group, read it, rename
// it, delete it and check the read is then refused; and add a user to one
// as a Developer. The old suite made these calls before nearly every
// Enterprise scenario, to build the group that scenario stood on, and never
// as a scenario of their own.

package common

import (
	"context"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestGroup_Lifecycle_CreateGetUpdateDelete creates a private group on
// every surface, reads it back, renames it, deletes it and checks the read
// is then refused. The fixture library's deletion is registered behind the
// server's, for a delete that only marked the group.
//
// Replaces: TestEE_MetaGroupEnterpriseOperations, TestMeta_Epics, TestIndividual_GroupCreateUltimateFields
func TestGroup_Lifecycle_CreateGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		name := e.Name("grp")

		created := harness.Do[groups.DetailOutput](s, actionGroupCreate, map[string]any{"name": name, "path": name, "visibility": "private"})
		if created.ID == 0 || created.Path != name || created.Visibility != "private" {
			e.T.Fatalf("group create answered %+v, want a private group at %q with an ID", created, name)
		}
		e.Defer("group "+created.FullPath, func(ctx context.Context) error {
			return fixture.DeleteGroup(ctx, e.Client(), created.ID, created.FullPath)
		})
		group := fixture.Group{ID: created.ID, Path: created.FullPath, Name: created.Name}
		params := map[string]any{"group_id": group.IDParam()}

		got := harness.Do[groups.DetailOutput](s, actionGroupGet, params)
		if got.ID != created.ID || got.FullPath != created.FullPath {
			e.T.Errorf("group get answered %d at %q, want %d at %q", got.ID, got.FullPath, created.ID, created.FullPath)
		}

		updated := harness.Do[groups.DetailOutput](s, actionGroupUpdate, withParams(params, map[string]any{"name": "Renamed " + name, "description": "renamed by the e2e suite"}))
		if updated.ID != created.ID || updated.Name != "Renamed "+name || updated.Description != "renamed by the e2e suite" {
			e.T.Errorf("group update answered %+v, want group %d renamed with the new description", updated, created.ID)
		}

		harness.DoVoid(s, actionGroupDelete, params)
		// A delete on an instance with delayed deletion marks the group and
		// renames its path, so the read by id is what tells the two apart:
		// gone, or marked and still readable under the new path.
		if remaining, err := harness.Try[groups.DetailOutput](s, actionGroupGet, params); err == nil {
			if remaining.MarkedForDeletion == "" {
				e.T.Errorf("the group still reads as %+v after its delete, and is not marked for deletion", remaining)
			}
			e.T.Logf("the delete marked the group for deletion on %s", remaining.MarkedForDeletion)
		}
	})
}

// TestGroupMembers_AddDeveloper_AnswersTheMembership adds a disposable user
// to a fresh group on every surface as a Developer and reads the access
// level off the answer.
//
// Replaces: TestMeta_GroupBillableMembers
func TestGroupMembers_AddDeveloper_AnswersTheMembership(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("members"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		user := fixture.NewUser(e, "member")

		added := harness.Do[groupmembers.Output](s, actionGroupMemberAdd, map[string]any{
			"group_id": group.IDParam(), "user_id": user.ID, "access_level": int64(gl.DeveloperPermissions),
		})
		if added.ID != user.ID || added.AccessLevel != int(gl.DeveloperPermissions) {
			e.T.Errorf("group_member_add answered user %d at level %d, want %d as a Developer", added.ID, added.AccessLevel, user.ID)
		}
	})
}
