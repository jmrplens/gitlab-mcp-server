//go:build e2e

// groupextras_test.go covers the group actions beyond the lifecycle: the
// archive flag turned on and off, the issues of the group's projects, the
// avatar, the projects shared into it, the two transfers that move a
// project and a subgroup under a group, the restore of a deleted group,
// and the two ways one group is shared with another.

package common

import (
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupIssueIIDs lists the iids of a group issue listing.
func groupIssueIIDs(listed []issues.Output) []int64 {
	iids := make([]int64, 0, len(listed))
	for _, issue := range listed {
		iids = append(iids, issue.IID)
	}
	return iids
}

// groupProjectIDs lists the ids of a group's project listing.
func groupProjectIDs(listed []groups.ProjectItem) []int64 {
	ids := make([]int64, 0, len(listed))
	for _, project := range listed {
		ids = append(ids, project.ID)
	}
	return ids
}

// sharedWithGroupIDs lists the groups a group is shared with, off its
// detail.
func sharedWithGroupIDs(detail groups.DetailOutput) []int64 {
	ids := make([]int64, 0, len(detail.SharedWithGroups))
	for _, shared := range detail.SharedWithGroups {
		ids = append(ids, shared.GroupID)
	}
	return ids
}

// TestGroupArchive_ArchiveAndUnarchive_ReadBack archives a group of each
// surface's own, reads the flag, unarchives it and reads the flag again.
//
// Replaces: TestMeta_GroupExtrasLifecycle
func TestGroupArchive_ArchiveAndUnarchive_ReadBack(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("archive"))
		params := map[string]any{"group_id": group.IDParam()}

		archived := harness.Do[toolutil.DeleteOutput](s, actionGroupArchive, params)
		if archived.Status != voidStatusSuccess {
			e.T.Errorf("archive answered %+v, want a %s status", archived, voidStatusSuccess)
		}
		if got := harness.Do[groups.DetailOutput](s, actionGroupGet, params); !got.Archived {
			e.T.Errorf("the group reads as %+v after its archive, want archived", got)
		}

		unarchived := harness.Do[toolutil.DeleteOutput](s, actionGroupUnarchive, params)
		if unarchived.Status != voidStatusSuccess {
			e.T.Errorf("unarchive answered %+v, want a %s status", unarchived, voidStatusSuccess)
		}
		if got := harness.Do[groups.DetailOutput](s, actionGroupGet, params); got.Archived {
			e.T.Errorf("the group still reads as archived after its unarchive: %+v", got)
		}
	})
}

// TestGroupReads_IssuesAndAvatar_AnswerTheGroup lists the issues of a
// group of each surface's own, which holds one project with one issue, and
// uploads a generated PNG as the group's avatar.
//
// Replaces: TestMeta_GroupExtrasLifecycle
func TestGroupReads_IssuesAndAvatar_AnswerTheGroup(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("groupreads"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("groupissues"), fixture.InGroup(group))
		issue := fixture.NewIssue(e, project, e.Name("issue"))
		params := map[string]any{"group_id": group.IDParam()}

		listed := harness.Do[issues.ListGroupOutput](s, actionGroupIssues, params)
		if iids := groupIssueIIDs(listed.Issues); len(iids) != 1 || iids[0] != issue.IID {
			e.T.Errorf("the group lists the issues %v, want exactly #%d", iids, issue.IID)
		}

		uploaded := harness.Do[groups.DetailOutput](s, actionGroupUploadAvatar, withParams(params, map[string]any{
			"filename": "avatar.png", "content_base64": fixture.PNGBase64(e),
		}))
		if uploaded.ID != group.ID || uploaded.AvatarURL == "" {
			e.T.Errorf("upload_avatar answered %+v, want group %d with an avatar URL", uploaded, group.ID)
		}
	})
}

// TestGroupTransfers_ProjectAndSubgroup_MoveUnderTheParent lists the
// projects shared into a child group, moves a personal project into a
// parent group, and moves the child group under the parent, once per
// surface with groups of its own.
//
// Replaces: TestMeta_GroupExtrasLifecycle
func TestGroupTransfers_ProjectAndSubgroup_MoveUnderTheParent(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		parent := fixture.NewGroup(e, fixture.WithGroupNamePrefix("parent"))
		child := fixture.NewGroup(e, fixture.WithGroupNamePrefix("child"))
		project := fixture.NewProject(e, fixture.WithNamePrefix("transferred"))

		// The share is fixture plumbing; the group-side listing is the
		// subject.
		shareProjectWithGroup(e, project, child)
		shared := harness.Do[groups.SharedProjectsListOutput](s, actionGroupSharedProjects, map[string]any{"group_id": child.IDParam()})
		if !containsID(groupProjectIDs(shared.Projects), project.ID) {
			e.T.Errorf("the child group lists the shared projects %v, want project %d among them", groupProjectIDs(shared.Projects), project.ID)
		}

		moved := harness.Do[groups.DetailOutput](s, actionGroupTransferProject, map[string]any{"group_id": parent.IDParam(), "project_id": project.IDParam()})
		if moved.ID != parent.ID {
			e.T.Errorf("transfer_project answered group %d, want the parent %d", moved.ID, parent.ID)
		}
		if stored := readProject(e, project); !strings.HasPrefix(stored.PathWithNamespace, parent.Path+"/") {
			e.T.Errorf("GitLab holds project %d at %q after the transfer, want it under %q", project.ID, stored.PathWithNamespace, parent.Path)
		}

		nested := harness.Do[groups.DetailOutput](s, actionGroupTransfer, map[string]any{"group_id": child.IDParam(), "parent_id": parent.ID})
		if nested.ID != child.ID || !strings.HasPrefix(nested.FullPath, parent.Path+"/") {
			e.T.Errorf("transfer answered %+v, want group %d under %q", nested, child.ID, parent.Path)
		}
	})
}

// TestGroupRestore_AfterDelete_ReturnsTheGroup deletes a group of each
// surface's own, which an instance with delayed deletion only marks, and
// restores it. An instance that removed it at once has nothing to restore,
// and the read after the delete says so.
//
// Replaces: TestMeta_GroupRestore
func TestGroupRestore_AfterDelete_ReturnsTheGroup(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("restore"))
		params := map[string]any{"group_id": group.IDParam()}

		harness.DoVoid(s, actionGroupDelete, params)
		// An instance that removes a group at once answers the read with the
		// documented not found. Any other refusal is a failure of the read
		// rather than evidence about how the instance deletes.
		marked, err := harness.Try[groups.DetailOutput](s, actionGroupGet, params)
		if err != nil {
			assertMentions(e, "group_get after the delete", err.Error(), "404")
			e.Skipf("the group is gone after its delete, so this instance removes groups at once and there is nothing to restore: %s", firstLine(err.Error()))
		}
		if marked.MarkedForDeletion == "" {
			e.Skipf("the group reads unmarked after its delete (%+v), so this instance removes groups at once and there is nothing to restore", marked)
		}

		restored := harness.Do[groups.DetailOutput](s, actionGroupRestore, params)
		if restored.ID != group.ID || restored.MarkedForDeletion != "" {
			e.T.Errorf("restore answered %+v, want group %d no longer marked for deletion", restored, group.ID)
		}
	})
}

// TestGroupSharing_TwoSurfaces_ShareAndUnshare shares a host group of each
// surface's own with a guest group of its own through the group's own
// share and unshare, then through the group members' pair, reading the
// share off the host's detail each time.
//
// Replaces: TestMeta_GroupToGroupSharing
func TestGroupSharing_TwoSurfaces_ShareAndUnshare(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		host := fixture.NewGroup(e, fixture.WithGroupNamePrefix("host"))
		guest := fixture.NewGroup(e, fixture.WithGroupNamePrefix("guest"))
		params := map[string]any{"group_id": host.IDParam()}

		shared := harness.Do[groups.ShareGroupOutput](s, actionGroupShareWithGroup, withParams(params, map[string]any{
			"shared_group_id": guest.ID, "group_access": int64(gl.DeveloperPermissions),
		}))
		if shared.SharedGroupID != guest.ID {
			e.T.Errorf("share_with_group answered %+v, want the share with group %d", shared, guest.ID)
		}
		if detail := harness.Do[groups.DetailOutput](s, actionGroupGet, params); !containsID(sharedWithGroupIDs(detail), guest.ID) {
			e.T.Errorf("the host is shared with %v after the share, want group %d among them", sharedWithGroupIDs(detail), guest.ID)
		}
		harness.DoVoid(s, actionGroupUnshareFromGroup, withParams(params, map[string]any{"shared_group_id": guest.ID}))
		if detail := harness.Do[groups.DetailOutput](s, actionGroupGet, params); containsID(sharedWithGroupIDs(detail), guest.ID) {
			e.T.Errorf("the host is still shared with group %d after the unshare", guest.ID)
		}

		memberShared := harness.Do[groupmembers.ShareOutput](s, actionGroupMemberShare, withParams(params, map[string]any{
			"share_group_id": guest.ID, "group_access": int64(gl.DeveloperPermissions),
		}))
		if memberShared.ID != host.ID {
			e.T.Errorf("group_member_share answered group %d, want the host %d", memberShared.ID, host.ID)
		}
		if detail := harness.Do[groups.DetailOutput](s, actionGroupGet, params); !containsID(sharedWithGroupIDs(detail), guest.ID) {
			e.T.Errorf("the host is shared with %v after the members' share, want group %d among them", sharedWithGroupIDs(detail), guest.ID)
		}
		harness.DoVoid(s, actionGroupMemberUnshare, withParams(params, map[string]any{"share_group_id": guest.ID}))
		if detail := harness.Do[groups.DetailOutput](s, actionGroupGet, params); containsID(sharedWithGroupIDs(detail), guest.ID) {
			e.T.Errorf("the host is still shared with group %d after the members' unshare", guest.ID)
		}
	})
}

// shareProjectWithGroup shares a project into a group as a Developer
// through client-go, which is fixture plumbing rather than the subject.
func shareProjectWithGroup(e *harness.Env, project fixture.Project, group fixture.Group) {
	e.T.Helper()
	_, err := e.Client().GL().Projects.ShareProjectWithGroup(project.ID, &gl.ShareWithGroupOptions{
		GroupID: new(group.ID), GroupAccess: new(gl.DeveloperPermissions),
	}, gl.WithContext(e.Ctx))
	if err != nil {
		e.T.Fatalf("sharing project %d with group %d through client-go: %v", project.ID, group.ID, err)
	}
}
