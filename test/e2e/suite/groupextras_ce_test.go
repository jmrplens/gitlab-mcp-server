//go:build e2e && !enterprise

// groupextras_ce_test.go covers gitlab_group meta-tool actions that were not
// exercised elsewhere in the suite: archive/unarchive/restore, group and
// project transfers, shared projects, group-wide issue listing, avatar
// upload, group-to-group sharing (both the Groups API surface and the
// group-members surface), direct member management, and webhook
// sub-operations (custom headers, URL variables, test triggers, and event
// resends).
//
// Build tag: e2e && !enterprise.
package suite

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
)

// groupExtrasDeveloperAccess and groupExtrasMaintainerAccess are the GitLab
// access levels used by the member and sharing lifecycles in this file.
const (
	groupExtrasDeveloperAccess  = 30
	groupExtrasMaintainerAccess = 40
)

// TestMeta_GroupExtrasLifecycle exercises archive, unarchive, issues,
// upload_avatar, shared_projects, transfer_project, and transfer through the
// gitlab_group meta-tool.
//
// The test creates a parent group, a child group, and a project fixture. It
// archives and unarchives the child (skipping when the instance predates the
// group archive API or keeps it feature-flagged off), lists group issues,
// uploads a generated 1x1 PNG avatar, shares the project into the child group
// and lists it via shared_projects, transfers the project into the parent
// group, and finally moves the child group under the parent.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupExtrasLifecycle(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	parent := CreateGroupMeta(ctx, e2e, sess.meta, "grp-xtra-parent")
	child := CreateGroupMeta(ctx, e2e, sess.meta, "grp-xtra-child")

	runGroupExtrasArchiveOps(t, ctx, child)
	runGroupExtrasGroupReads(t, ctx, parent)
	runGroupExtrasTransferOps(t, ctx, e2e, parent, child)
}

// runGroupExtrasArchiveOps drives group.archive and group.unarchive on a
// disposable group. The group archive API shipped feature-flagged in GitLab
// 18.0, so a 404/403 on archive skips both subtests instead of failing.
func runGroupExtrasArchiveOps(t *testing.T, ctx context.Context, grp GroupFixture) {
	t.Helper()
	archived := false

	t.Run("Archive", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "archive",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		if isHTTPStatus(err, http.StatusNotFound) || isHTTPStatus(err, http.StatusForbidden) {
			t.Skipf("group archive API unavailable on this GitLab version/flag state: %v", err)
		}
		requireNoError(t, err, "group archive")
		archived = true
		t.Logf("Archived group %d", grp.ID)
	})

	t.Run("Unarchive", func(t *testing.T) {
		if !archived {
			t.Skip("group was not archived (archive API unavailable)")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "unarchive",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "group unarchive")
		t.Logf("Unarchived group %d", grp.ID)
	})
}

// runGroupExtrasGroupReads drives group.issues and group.upload_avatar on a
// fresh group: the issue list must be empty and the avatar upload must return
// the same group ID.
func runGroupExtrasGroupReads(t *testing.T, ctx context.Context, grp GroupFixture) {
	t.Helper()

	t.Run("Issues", func(t *testing.T) {
		out, err := callToolOn[issues.ListGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "issues",
			"params": map[string]any{"group_id": grp.gidStr()},
		})
		requireNoError(t, err, "group issues")
		requireTruef(t, len(out.Issues) == 0, "expected 0 issues in a fresh group, got %d", len(out.Issues))
		t.Logf("Listed %d issues for group %d", len(out.Issues), grp.ID)
	})

	t.Run("UploadAvatar", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "upload_avatar",
			"params": map[string]any{
				"group_id":       grp.gidStr(),
				"filename":       "avatar.png",
				"content_base64": groupExtrasPNGBase64(t),
			},
		})
		requireNoError(t, err, "group upload_avatar")
		requireTruef(t, out.ID == grp.ID, "avatar upload returned group %d, want %d", out.ID, grp.ID)
		t.Logf("Uploaded avatar for group %d", out.ID)
	})
}

// runGroupExtrasTransferOps drives group.shared_projects,
// group.transfer_project, and group.transfer. The project is shared into the
// child group while both live in their original namespaces, then moved into
// the parent group, and finally the child group itself is transferred under
// the parent.
func runGroupExtrasTransferOps(t *testing.T, ctx context.Context, e2e *E2EContext, parent, child GroupFixture) {
	t.Helper()
	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("SharedProjects", func(t *testing.T) {
		// The share itself is fixture plumbing (project domain); only the
		// group-side listing is the action under test here.
		_, err := sess.glClient.GL().Projects.ShareProjectWithGroup(int(proj.ID), &gl.ShareWithGroupOptions{
			GroupID:     new(child.ID),
			GroupAccess: new(gl.AccessLevelValue(groupExtrasDeveloperAccess)),
		}, gl.WithContext(ctx))
		requireNoError(t, err, "share project with group (fixture)")

		out, err := callToolOn[groups.SharedProjectsListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "shared_projects",
			"params": map[string]any{"group_id": child.gidStr()},
		})
		requireNoError(t, err, "group shared_projects")
		requireTruef(t, len(out.Projects) >= 1, "expected at least 1 shared project, got %d", len(out.Projects))
		t.Logf("Group %d lists %d shared projects", child.ID, len(out.Projects))
	})

	t.Run("TransferProject", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "transfer_project",
			"params": map[string]any{
				"group_id":   parent.gidStr(),
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "group transfer_project")
		requireTruef(t, out.ID == parent.ID, "transfer_project returned group %d, want %d", out.ID, parent.ID)
		t.Logf("Transferred project %d into group %d", proj.ID, parent.ID)
	})

	t.Run("TransferGroup", func(t *testing.T) {
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "transfer",
			"params": map[string]any{
				"group_id":  child.gidStr(),
				"parent_id": parent.ID,
			},
		})
		requireNoError(t, err, "group transfer")
		requireTruef(t, strings.HasPrefix(out.FullPath, parent.Path+"/"),
			"transferred group full path %q should live under %q", out.FullPath, parent.Path)
		t.Logf("Transferred group %d under %q", child.ID, parent.Path)
	})
}

// TestMeta_GroupRestore exercises group.restore right after a soft delete via
// the gitlab_group meta-tool.
//
// The test creates a disposable group, deletes it, and calls restore. On
// GitLab Free/CE deletion is immediate (delayed deletion is Premium), so a
// restore failure that reports the group as gone or not marked for deletion
// documents that behavior with a skip instead of failing. When delayed
// deletion applies, the restore must return the original group ID.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupRestore(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	name := uniqueName("grp-restore")
	created, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{"name": name, "path": name},
	})
	requireNoError(t, err, "create group for restore")
	gid := strconv.FormatInt(created.ID, 10)
	defer groupExtrasDeleteGroupSilently(created.ID)

	t.Run("DeleteThenRestore", func(t *testing.T) {
		delErr := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": gid},
		})
		requireNoError(t, delErr, "soft-delete group")

		restored, restoreErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "restore",
			"params": map[string]any{"group_id": gid},
		})
		if restoreErr != nil {
			lower := strings.ToLower(restoreErr.Error())
			requireTruef(t,
				isHTTPStatus(restoreErr, http.StatusNotFound) || strings.Contains(lower, "marked for deletion"),
				"unexpected group restore failure: %v", restoreErr)
			t.Skipf("group restore not applicable: this instance deletes groups immediately (delayed deletion is Premium): %v", restoreErr)
		}
		requireTruef(t, restored.ID == created.ID, "restored group ID %d, want %d", restored.ID, created.ID)
		t.Logf("Restored group %d", restored.ID)
	})
}

// TestMeta_GroupToGroupSharing exercises share_with_group,
// unshare_from_group, group_member_share, and group_member_unshare through
// the gitlab_group meta-tool.
//
// The test creates two sibling groups and walks the same host/guest pair
// through both sharing surfaces: the Groups API share (share_with_group /
// unshare_from_group) and the group-members share (group_member_share /
// group_member_unshare). Each unshare runs before the next share so the pair
// never holds two overlapping invitations.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_GroupToGroupSharing(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	host := CreateGroupMeta(ctx, e2e, sess.meta, "grp-share-host")
	guest := CreateGroupMeta(ctx, e2e, sess.meta, "grp-share-guest")

	t.Run("ShareWithGroup", func(t *testing.T) {
		out, err := callToolOn[groups.ShareGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "share_with_group",
			"params": map[string]any{
				"group_id":        host.gidStr(),
				"shared_group_id": guest.ID,
				"group_access":    groupExtrasDeveloperAccess,
			},
		})
		requireNoError(t, err, "group share_with_group")
		requireTruef(t, out.SharedGroupID == guest.ID, "shared group ID %d, want %d", out.SharedGroupID, guest.ID)
		t.Logf("Shared group %d with group %d: %s", host.ID, guest.ID, out.AccessRole)
	})

	t.Run("UnshareFromGroup", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "unshare_from_group",
			"params": map[string]any{
				"group_id":        host.gidStr(),
				"shared_group_id": guest.ID,
			},
		})
		requireNoError(t, err, "group unshare_from_group")
		t.Logf("Unshared group %d from group %d", host.ID, guest.ID)
	})

	t.Run("MemberShare", func(t *testing.T) {
		out, err := callToolOn[groupmembers.ShareOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_share",
			"params": map[string]any{
				"group_id":       host.gidStr(),
				"share_group_id": guest.ID,
				"group_access":   groupExtrasDeveloperAccess,
			},
		})
		requireNoError(t, err, "group_member_share")
		requireTruef(t, out.ID == host.ID, "member share returned group %d, want %d", out.ID, host.ID)
		t.Logf("Shared group %d with group %d via members surface", host.ID, guest.ID)
	})

	t.Run("MemberUnshare", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_unshare",
			"params": map[string]any{
				"group_id":       host.gidStr(),
				"share_group_id": guest.ID,
			},
		})
		requireNoError(t, err, "group_member_unshare")
		t.Logf("Unshared group %d from group %d via members surface", host.ID, guest.ID)
	})
}

// TestMeta_GroupMemberLifecycle exercises group_member_add,
// group_member_edit, and group_member_remove through the gitlab_group
// meta-tool.
//
// The test provisions a secondary user through the admin Users API (only
// safe on the disposable Docker GitLab, hence the mode gate), adds it to a
// fresh group as Developer, promotes it to Maintainer, and removes it. Each
// step asserts the member payload round-trips the user ID and access level.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta.
// Admin token required.
func TestMeta_GroupMemberLifecycle(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("secondary user provisioning requires the disposable Docker GitLab (E2E_MODE=docker)")
	}
	RequireCapabilities(t, CapabilityAdmin)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-member-xtra")
	userID := groupExtrasCreateUser(ctx, t)

	t.Run("MemberAdd", func(t *testing.T) {
		out, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_add",
			"params": map[string]any{
				"group_id":     grp.gidStr(),
				"user_id":      userID,
				"access_level": groupExtrasDeveloperAccess,
			},
		})
		requireNoError(t, err, "group_member_add")
		requireTruef(t, out.ID == userID, "added member ID %d, want %d", out.ID, userID)
		requireTruef(t, out.AccessLevel == groupExtrasDeveloperAccess, "added member access %d, want %d", out.AccessLevel, groupExtrasDeveloperAccess)
		t.Logf("Added user %d to group %d as Developer", userID, grp.ID)
	})

	t.Run("MemberEdit", func(t *testing.T) {
		out, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_edit",
			"params": map[string]any{
				"group_id":     grp.gidStr(),
				"user_id":      userID,
				"access_level": groupExtrasMaintainerAccess,
			},
		})
		requireNoError(t, err, "group_member_edit")
		requireTruef(t, out.AccessLevel == groupExtrasMaintainerAccess, "edited member access %d, want %d", out.AccessLevel, groupExtrasMaintainerAccess)
		t.Logf("Promoted user %d to Maintainer in group %d", userID, grp.ID)
	})

	t.Run("MemberRemove", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_remove",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"user_id":  userID,
			},
		})
		requireNoError(t, err, "group_member_remove")
		t.Logf("Removed user %d from group %d", userID, grp.ID)
	})
}

// TestMeta_GroupHookExtras exercises the group webhook sub-operations
// hook_set_custom_header, hook_delete_custom_header, hook_set_url_variable,
// hook_delete_url_variable, hook_test, and hook_resend_event through the
// gitlab_group meta-tool.
//
// The test creates a group with a README-initialized project (so push-event
// test deliveries have a commit payload), registers a webhook pointing at
// the Docker fixture service, and walks each sub-operation. URL-variable
// handling is version-tolerant: GitLab rejects variables for hooks whose URL
// lacks a {placeholder} on some versions and accepts them on others, so the
// delete assertion follows whichever outcome the set produced. Event resend
// prefers a real recorded delivery and falls back to the documented 404
// error path when none is visible within the poll budget.
//
// Build tag: e2e && !enterprise. Mode: CE (fixture service required).
// Surface: meta.
func TestMeta_GroupHookExtras(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !hasE2EFixtureService() {
		t.Skip("E2E fixture service unavailable; set E2E_FIXTURE_URL or run with E2E_MODE=docker")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	e2e := NewE2EContext(t)
	grp := CreateGroupMeta(ctx, e2e, sess.meta, "grp-hook-xtra")
	groupExtrasCreateGroupProject(ctx, t, grp)
	hookID := groupExtrasAddHook(ctx, t, grp)

	runGroupExtrasHookHeaderOps(t, ctx, grp, hookID)
	runGroupExtrasHookURLVariableOps(t, ctx, grp, hookID)
	runGroupExtrasHookDeliveryOps(t, ctx, grp, hookID)
}

// groupExtrasAddHook registers a webhook on the group pointing at the E2E
// fixture service and returns its ID. Hook creation itself (hook_add) is
// fixture plumbing for the sub-operation tests.
func groupExtrasAddHook(ctx context.Context, t *testing.T, grp GroupFixture) int64 {
	t.Helper()
	out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_add",
		"params": map[string]any{
			"group_id":    grp.gidStr(),
			"url":         e2eFixtureServiceURL("/group-hook"),
			"push_events": true,
		},
	})
	// Group webhooks are a Premium feature: on a Free-tier catalog the
	// gitlab_group dispatcher drops the hook_* routes from its action enum,
	// so the call dies in schema validation ("does not equal any of"); older
	// dispatch paths said "unknown action", and an ungated server against
	// live CE gets 404 from the API. Any of those means skip everything.
	if err != nil && (isHTTPStatus(err, 404) ||
		strings.Contains(err.Error(), "unknown action") ||
		strings.Contains(err.Error(), "does not equal any of")) {
		t.Skipf("group webhooks require GitLab Premium (tier-gated or 404 on CE): %v", err)
	}
	requireNoError(t, err, "group hook_add")
	requireTruef(t, out.ID > 0, "expected positive hook ID")
	t.Logf("Added group hook %d", out.ID)
	return out.ID
}

// runGroupExtrasHookHeaderOps drives hook_set_custom_header and
// hook_delete_custom_header; both are expected to succeed on a fresh hook.
func runGroupExtrasHookHeaderOps(t *testing.T, ctx context.Context, grp GroupFixture, hookID int64) {
	t.Helper()

	t.Run("HookSetCustomHeader", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_set_custom_header",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"hook_id":  hookID,
				"key":      "X-E2E-Group",
				"value":    "e2e-group-value",
			},
		})
		requireNoError(t, err, "group hook_set_custom_header")
		t.Log("Set custom header on group hook")
	})

	t.Run("HookDeleteCustomHeader", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_delete_custom_header",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"hook_id":  hookID,
				"key":      "X-E2E-Group",
			},
		})
		requireNoError(t, err, "group hook_delete_custom_header")
		t.Log("Deleted custom header from group hook")
	})
}

// runGroupExtrasHookURLVariableOps drives hook_set_url_variable and
// hook_delete_url_variable. The hook URL carries no {placeholder}, so the
// set call may be rejected (422) or accepted depending on the GitLab
// version; the delete assertion mirrors whichever outcome occurred.
func runGroupExtrasHookURLVariableOps(t *testing.T, ctx context.Context, grp GroupFixture, hookID int64) {
	t.Helper()
	variableSet := false

	t.Run("HookSetURLVariable", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_set_url_variable",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"hook_id":  hookID,
				"key":      "e2e_var",
				"value":    "e2e-value",
			},
		})
		variableSet = err == nil
		t.Logf("hook_set_url_variable accepted=%v (err=%v)", variableSet, err)
	})

	t.Run("HookDeleteURLVariable", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_delete_url_variable",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"hook_id":  hookID,
				"key":      "e2e_var",
			},
		})
		if variableSet {
			requireNoError(t, err, "delete URL variable that was set")
			t.Log("Deleted URL variable from group hook")
			return
		}
		requireTruef(t, err != nil, "expected error deleting a URL variable that was never set")
		t.Logf("Expected error deleting unset URL variable: %v", err)
	})
}

// runGroupExtrasHookDeliveryOps drives hook_test and hook_resend_event. The
// resend prefers a real recorded delivery from the test trigger and falls
// back to asserting the 404 error path with a non-existent event ID when no
// delivery becomes visible within the poll budget.
func runGroupExtrasHookDeliveryOps(t *testing.T, ctx context.Context, grp GroupFixture, hookID int64) {
	t.Helper()
	tested := false

	t.Run("HookTest", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_test",
			"params": map[string]any{
				"group_id": grp.gidStr(),
				"hook_id":  hookID,
				"trigger":  "push_events",
			},
		})
		requireNoError(t, err, "group hook_test")
		tested = true
		t.Logf("Triggered test push event on group hook %d", hookID)
	})

	t.Run("HookResendEvent", func(t *testing.T) {
		requireTruef(t, hookID > 0, "hookID not set")
		var eventID int64
		if tested {
			eventID = groupExtrasFirstHookEventID(ctx, t, grp.ID, hookID)
		}
		if eventID == 0 {
			err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "hook_resend_event",
				"params": map[string]any{
					"group_id":      grp.gidStr(),
					"hook_id":       hookID,
					"hook_event_id": int64(999999999),
				},
			})
			requireTruef(t, err != nil, "expected error resending a non-existent hook event")
			t.Logf("Expected error resending non-existent hook event: %v", err)
			return
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "hook_resend_event",
			"params": map[string]any{
				"group_id":      grp.gidStr(),
				"hook_id":       hookID,
				"hook_event_id": eventID,
			},
		})
		requireNoError(t, err, "group hook_resend_event")
		t.Logf("Resent group hook event %d", eventID)
	})
}

// groupExtrasCreateGroupProject creates a README-initialized project inside
// the group so group hook test triggers have a commit payload to deliver.
// The project is removed by the group deletion cascade.
func groupExtrasCreateGroupProject(ctx context.Context, t *testing.T, grp GroupFixture) {
	t.Helper()
	name := uniqueName(e2eProjectPrefix + "grp-hook")
	out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   name,
			"namespace_id":           grp.ID,
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	requireNoError(t, err, "create project inside group")
	waitForBranchOn(ctx, t, sess.glClient, out.ID, defaultBranch)
}

// groupExtrasFirstHookEventID polls the raw group hook events endpoint until
// a delivery is recorded, returning its ID or 0 when none appears within the
// budget. The events endpoint has no client-go wrapper, so the request is
// built through the SDK's generic NewRequest/Do plumbing.
func groupExtrasFirstHookEventID(ctx context.Context, t *testing.T, groupID, hookID int64) int64 {
	t.Helper()
	var events []struct {
		ID int64 `json:"id"`
	}
	const budget = 30 * time.Second
	pollCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	_ = Poll(pollCtx, 2*time.Second, budget, func() (bool, string, error) {
		req, err := sess.glClient.GL().NewRequest(
			http.MethodGet,
			fmt.Sprintf("groups/%d/hooks/%d/events", groupID, hookID),
			nil,
			[]gl.RequestOptionFunc{gl.WithContext(pollCtx)},
		)
		if err != nil {
			return false, "", fmt.Errorf("build hook events request: %w", err)
		}
		events = events[:0]
		if _, doErr := sess.glClient.GL().Do(req, &events); doErr != nil {
			return false, fmt.Sprintf("hook events endpoint not readable yet: %v", doErr), nil
		}
		if len(events) == 0 {
			return false, "no hook events recorded yet", nil
		}
		return true, "hook event recorded", nil
	})
	if len(events) == 0 {
		return 0
	}
	return events[0].ID
}

// groupExtrasCreateUser provisions a secondary GitLab user through the admin
// Users API and registers its deletion. Only called from Docker-gated tests
// because it mutates instance-level user state.
func groupExtrasCreateUser(ctx context.Context, t *testing.T) int64 {
	t.Helper()
	username := uniqueName("grp-xtra-usr")
	user, _, err := sess.glClient.GL().Users.CreateUser(&gl.CreateUserOptions{
		Email:            new(username + "@e2e-test.local"),
		Name:             new("E2E Group Extras " + username),
		Username:         new(username),
		Password:         new("E2eT!Gx9K#p2mNq$8BcZ"),
		SkipConfirmation: new(true),
	}, gl.WithContext(ctx))
	requireNoError(t, err, "create secondary user")
	//nolint:contextcheck // Cleanup runs after the test context is canceled and needs its own bounded context.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = sess.glClient.GL().Users.DeleteUser(user.ID, gl.WithContext(cleanupCtx))
	})
	return user.ID
}

// groupExtrasDeleteGroupSilently removes a group by ID, ignoring errors. Used
// as a safety net for tests that intentionally delete (or fail to restore)
// their own group mid-test.
func groupExtrasDeleteGroupSilently(groupID int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _ = sess.glClient.GL().Groups.DeleteGroup(groupID, nil, gl.WithContext(ctx))
}

// groupExtrasPNGBase64 returns a freshly encoded 1x1 PNG as base64 for
// avatar upload inputs, avoiding binary fixtures on disk.
func groupExtrasPNGBase64(t *testing.T) string {
	t.Helper()
	var buf bytes.Buffer
	requireNoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))), "encode avatar png")
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}
