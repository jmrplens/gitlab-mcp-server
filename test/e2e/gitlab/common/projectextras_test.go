//go:build e2e

// projectextras_test.go covers the project actions that reach beyond the
// project itself: sharing it with a group, its avatar, its restore after
// a delete, its transfer to another namespace and back, its creation on
// another user's behalf, and the fork relation between two projects that
// were never forked.

package common

import (
	"context"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestProjectSharing_WithGroup_ListsAndRemoves shares a project of each
// surface's own with a group of its own as a Developer, finds the group in
// the invited listing and removes the share.
//
// Replaces: TestMeta_ProjectGroupSharing
func TestProjectSharing_WithGroup_ListsAndRemoves(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("shared"))
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("invitee"))
		params := map[string]any{"project_id": project.IDParam()}

		shared := harness.Do[projects.ShareProjectOutput](s, actionProjectShareWithGroup, withParams(params, map[string]any{
			"group_id": group.ID, "group_access": int64(gl.DeveloperPermissions),
		}))
		if shared.GroupID != group.ID {
			e.T.Errorf("share_with_group answered %+v, want the share with group %d", shared, group.ID)
		}
		invited := harness.Do[projects.ListProjectGroupsOutput](s, actionProjectListInvitedGroups, params)
		if !containsID(projectGroupIDs(invited.Groups), group.ID) {
			e.T.Errorf("the invited groups %v do not hold group %d", projectGroupIDs(invited.Groups), group.ID)
		}

		harness.DoVoid(s, actionProjectDeleteSharedGroup, withParams(params, map[string]any{"group_id": group.ID}))
		remaining := harness.Do[projects.ListProjectGroupsOutput](s, actionProjectListInvitedGroups, params)
		if containsID(projectGroupIDs(remaining.Groups), group.ID) {
			e.T.Errorf("the invited groups still hold group %d after the share was removed", group.ID)
		}
	})
}

// TestProjectAvatar_UploadAndDownload_RoundTrips uploads a generated PNG
// as the avatar of a project of each surface's own and downloads it back.
//
// Replaces: TestMeta_ProjectAvatar
func TestProjectAvatar_UploadAndDownload_RoundTrips(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("avatar"))
		params := map[string]any{"project_id": project.IDParam()}

		uploaded := harness.Do[projects.Output](s, actionProjectUploadAvatar, withParams(params, map[string]any{
			"filename": "avatar.png", "content_base64": fixture.PNGBase64(e),
		}))
		if uploaded.ID != project.ID || uploaded.AvatarURL == "" {
			e.T.Errorf("upload_avatar answered %+v, want project %d with an avatar URL", uploaded, project.ID)
		}

		downloaded := harness.Do[projects.DownloadAvatarOutput](s, actionProjectDownloadAvatar, params)
		if downloaded.SizeBytes == 0 || downloaded.ContentBase64 == "" {
			e.T.Errorf("download_avatar answered %d bytes and %d characters of content, want the image back", downloaded.SizeBytes, len(downloaded.ContentBase64))
		}
	})
}

// TestProjectRestore_AfterDelete_ReturnsTheProject deletes a project of
// each surface's own, which an instance with delayed deletion only marks,
// and restores it. An instance that removed it at once has nothing to
// restore, and says so by a delete that was not scheduled.
//
// Replaces: TestMeta_ProjectRestore
func TestProjectRestore_AfterDelete_ReturnsTheProject(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("restore"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("restore"), fixture.InGroup(group))
		params := map[string]any{"project_id": project.IDParam()}

		deleted := harness.Do[projects.DeleteOutput](s, actionProjectDelete, params)
		if deleted.Status != "scheduled" {
			e.Skipf("the delete answered %q rather than a scheduled deletion, so this instance removes projects at once and there is nothing to restore", deleted.Status)
		}

		restored := harness.Do[projects.Output](s, actionProjectRestore, params)
		if restored.ID != project.ID || restored.MarkedForDeletionOn != "" {
			e.T.Errorf("restore answered %+v, want project %d no longer marked for deletion", restored, project.ID)
		}
	})
}

// TestProjectTransfer_ToAGroupAndBack_MovesTheNamespace transfers a
// personal project of each surface's own into a group of its own and back
// into the run user's namespace, reading the path off each answer.
//
// Replaces: TestMeta_ProjectTransfer
func TestProjectTransfer_ToAGroupAndBack_MovesTheNamespace(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("moving"))
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("destination"))
		params := map[string]any{"project_id": project.IDParam()}
		username := e.Runtime().Username

		moved := harness.Do[projects.Output](s, actionProjectTransfer, withParams(params, map[string]any{"namespace": group.Path}))
		if moved.ID != project.ID || !strings.HasPrefix(moved.PathWithNamespace, group.Path+"/") {
			e.T.Errorf("transfer answered %+v, want project %d under %q", moved, project.ID, group.Path)
		}

		back := harness.Do[projects.Output](s, actionProjectTransfer, withParams(params, map[string]any{"namespace": username}))
		if back.ID != project.ID || !strings.HasPrefix(back.PathWithNamespace, username+"/") {
			e.T.Errorf("the transfer back answered %+v, want project %d under %q", back, project.ID, username)
		}
	})
}

// TestProjectCreateForUser_Admin_PlacesItInTheUsersNamespace creates a
// project on behalf of a disposable user, on every surface, and reads the
// namespace off the answer.
//
// Replaces: TestMeta_ProjectCreateForUser
func TestProjectCreateForUser_Admin_PlacesItInTheUsersNamespace(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		user := fixture.NewUser(e, "owner")
		name := e.Name("foruser")

		created := harness.Do[projects.Output](s, actionProjectCreateForUser, map[string]any{
			"user_id": user.ID, "name": name, "visibility": "private",
		})
		if created.ID == 0 || !strings.HasPrefix(created.PathWithNamespace, user.Username+"/") {
			e.T.Fatalf("create_for_user answered %+v, want a project under %q with an ID", created, user.Username)
		}
		// The user's deletion, registered before this, takes the project
		// with it; a deletion of the project's own runs first and says so on
		// its own line when it fails.
		e.Defer("project "+created.PathWithNamespace, func(ctx context.Context) error {
			return fixture.DeleteProject(ctx, e.Client(), created.ID, created.PathWithNamespace)
		})
	})
}

// TestProjectForkRelation_CreateAndDelete_ReadsBack makes one project of
// each surface's own a fork of another, reads the upstream off the answer
// and off GitLab itself, and removes the relation.
//
// Replaces: TestMeta_ProjectForkRelations
func TestProjectForkRelation_CreateAndDelete_ReadsBack(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		source := fixture.NewProject(e, fixture.WithNamePrefix("relsource"))
		fork := fixture.NewProject(e, fixture.WithNamePrefix("reldown"))
		params := map[string]any{"project_id": fork.IDParam()}

		related := harness.Do[projects.Output](s, actionProjectCreateForkRelation, withParams(params, map[string]any{"forked_from_id": source.ID}))
		if related.ID != fork.ID || related.ForkedFromProject == nil || related.ForkedFromProject.ID != source.ID {
			e.T.Errorf("create_fork_relation answered %+v, want project %d forked from %d", related, fork.ID, source.ID)
		}
		// The relation is also read back from GitLab, so the assertion is
		// about what it stored rather than about what the action echoed.
		if stored := readProject(e, fork); stored.ForkedFromProject == nil || stored.ForkedFromProject.ID != source.ID {
			e.T.Errorf("GitLab holds %+v as the upstream of project %d, want project %d", stored.ForkedFromProject, fork.ID, source.ID)
		}

		removed := harness.Do[toolutil.VoidOutput](s, actionProjectDeleteForkRelation, params)
		if removed.Status != voidStatusSuccess {
			e.T.Errorf("delete_fork_relation answered %+v, want a %s status", removed, voidStatusSuccess)
		}
		if stored := readProject(e, fork); stored.ForkedFromProject != nil {
			e.T.Errorf("GitLab still holds %+v as the upstream of project %d after the relation was deleted", stored.ForkedFromProject, fork.ID)
		}
	})
}

// readProject reads a project through client-go, for an assertion about
// what GitLab stored rather than what an action answered.
func readProject(e *harness.Env, project fixture.Project) *gl.Project {
	e.T.Helper()
	stored, _, err := e.Client().GL().Projects.GetProject(project.ID, nil, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("reading project %d through client-go: %v", project.ID, err)
	}
	return stored
}
