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
	"time"

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
// into the run user's namespace.
//
// GitLab 19.4 applies a transfer in the background and the action waits for
// it, so each answer must show the move landed: a queued answer means the
// handler never saw a move GitLab applied, which is the defect this scenario
// exists to catch, and a test that accepted it would stay green against a
// handler that waits out its whole bound on every call. That holds only when
// the move can land inside the action's 45 seconds. An empty project moves in
// seconds once its worker runs, but a busy instance makes the worker wait its
// turn first (99 seconds for a group on the licensed run of 19.4.1), so
// Sidekiq's queues are drained before each transfer and the failure says
// whether they drained. The project is then read back with project.get,
// independently of the action's own read, until GitLab holds it under the
// destination. On 19.3 and older the answer comes after the move and both hold
// at once.
//
// Replaces: TestMeta_ProjectTransfer
func TestProjectTransfer_ToAGroupAndBack_MovesTheNamespace(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("moving"))
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("destination"))

		transferProjectAndWait(e, s, project, group.Path)
		transferProjectAndWait(e, s, project, e.Runtime().Username)
	})
}

// transferProjectAndWait transfers a project into a namespace, holds the
// answer to a move that landed, and reads the project back until GitLab holds
// it there.
func transferProjectAndWait(e *harness.Env, s *harness.Session, project fixture.Project, namespace string) {
	e.T.Helper()
	params := map[string]any{"project_id": project.IDParam()}
	under := namespace + "/"

	drained := fixture.DrainSidekiqWithin(e.Ctx, e.Client(), transferDrainWait)
	// GitLab moves the project before it closes the transfer on the project's
	// namespace, and until then refuses the next one with "Unable to initiate
	// transfer. The project may already have a transfer in progress." The
	// transfer back is sent right after project.get has seen the first move,
	// so it can land in that window, which it did on 19.4.1-ee. GitLab's own
	// troubleshooting page answers that refusal with a retry, and so does this.
	moved := harness.Eventually[projects.TransferOutput](s, actionProjectTransfer,
		withParams(params, map[string]any{"namespace": namespace}), transferRetryInterval, transferRetryWait,
		func(out projects.TransferOutput) bool { return out.ID == project.ID })
	if moved.ID != project.ID || moved.TransferQueued || !strings.HasPrefix(moved.PathWithNamespace, under) {
		e.T.Errorf("the transfer to %q answered %+v, want project %d applied under %q (Sidekiq drained before it: %t)", namespace, moved, project.ID, under, drained)
	}

	stored := harness.Eventually[projects.Output](s, actionProjectGet, params, 2*time.Second, 90*time.Second,
		func(out projects.Output) bool { return strings.HasPrefix(out.PathWithNamespace, under) })
	if stored.ID != project.ID {
		e.T.Errorf("project.get after the transfer to %q answered project %d, want %d", namespace, stored.ID, project.ID)
	}
}

// transferDrainWait bounds the drain before each transfer of the project and
// group scenarios: long enough for a licensed instance to work off what the
// scenarios before it queued, which took more than a minute and a half on
// 19.4.1, and short enough that a queue that never empties fails the scenario
// rather than holding the package until its timeout.
const transferDrainWait = 3 * time.Minute

// transferRetryInterval and transferRetryWait bound how long a transfer is
// resent while GitLab still holds the previous one open on the namespace.
// The window is the tail of a move already applied, so it is seconds; a
// refusal that outlasts the wait is a transfer that did not close, and the
// failure names GitLab's last answer.
const (
	transferRetryInterval = 5 * time.Second
	transferRetryWait     = 90 * time.Second
)

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
