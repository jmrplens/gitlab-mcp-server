//go:build e2e && !enterprise

// project_extras_ce_test.go covers gitlab_project meta-tool actions that were
// not exercised by the existing project test files: member CRUD, group
// sharing, avatar upload/download, restore, transfer, create-for-user, fork
// relations, markdown upload deletion, and forced push-mirror sync.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectmirrors"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/uploads"
)

// projectExtrasPNGBase64 is a valid 1x1 transparent PNG. GitLab validates
// avatar uploads as real images, so a text placeholder would be rejected.
const projectExtrasPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

// projectExtrasCreateUser creates a disposable second user through the admin
// API and registers its deletion in the per-test ledger. Member and
// create-for-user tests need an identity other than the suite token's owner,
// which only the disposable Docker instance can safely provide.
func projectExtrasCreateUser(ctx context.Context, e2e *E2EContext) (int64, string) {
	t := e2e.T
	t.Helper()
	if !isDockerMode() {
		t.Skip("second-user fixture requires the disposable Docker GitLab instance (E2E_MODE=docker)")
	}
	//nolint:contextcheck // The admin capability probe is a cached, ctx-free helper shared by the whole suite.
	RequireCapabilities(t, CapabilityAdmin)

	username := uniqueName("px-usr")
	user, _, err := sess.glClient.GL().Users.CreateUser(&gl.CreateUserOptions{
		Email:            new(username + "@e2e-test.local"),
		Name:             new("E2E ProjectExtras " + username),
		Username:         new(username),
		Password:         new("E2eT!Gx9K#p2mNq$8BcZ"),
		SkipConfirmation: new(true),
	}, gl.WithContext(ctx))
	requireNoError(t, err, "create fixture user via admin API")

	requireNoError(t, e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindUser,
		ID:        strconv.FormatInt(user.ID, 10),
		Name:      username,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			_, delErr := sess.glClient.GL().Users.DeleteUser(user.ID, gl.WithContext(cleanupCtx))
			return delErr
		},
	}), "register fixture user cleanup")

	return user.ID, username
}

// projectExtrasRegisterProjectCleanup registers permanent deletion for a
// project created directly through meta calls (outside CreateProjectMeta),
// so restore/import/create-for-user leftovers never outlive the test.
func projectExtrasRegisterProjectCleanup(e2e *E2EContext, id int64, path, name string) {
	e2e.T.Helper()
	idStr := strconv.FormatInt(id, 10)
	requireNoError(e2e.T, e2e.Ledger.Register(ResourceRecord{
		Kind:      ResourceKindProject,
		ID:        idStr,
		Path:      path,
		Name:      name,
		OwnerTest: e2e.Name,
		RunID:     e2e.RunID,
		CreatedAt: time.Now(),
		Cleanup: func(cleanupCtx context.Context) error {
			return callToolVoidOn(cleanupCtx, sess.meta, "gitlab_project", map[string]any{
				"action": "delete",
				"params": map[string]any{
					"project_id":         idStr,
					"permanently_remove": true,
					"full_path":          path,
				},
			})
		},
	}), "register project cleanup")
}

// projectExtrasUploadRef extracts the secret and filename from an upload's
// /uploads/<secret>/<filename> URL, which is the only place GitLab exposes
// the secret needed by upload_delete_by_secret.
func projectExtrasUploadRef(t *testing.T, uploadURL string) (string, string) {
	t.Helper()
	parts := strings.Split(strings.Trim(uploadURL, "/"), "/")
	requireTruef(t, len(parts) >= 3 && parts[len(parts)-3] == "uploads",
		"unexpected upload URL format %q (want .../uploads/<secret>/<filename>)", uploadURL)
	return parts[len(parts)-2], parts[len(parts)-1]
}

// TestMeta_ProjectMemberLifecycle exercises project member add, edit, and
// delete actions through the gitlab_project meta-tool.
//
// The test creates a project fixture and a disposable second user via the
// admin API (Docker mode only), then walks member_add (Developer),
// member_edit (raise to Maintainer), and member_delete. Each subtest asserts
// the returned membership carries the expected user ID and access level.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta.
func TestMeta_ProjectMemberLifecycle(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	userID, username := projectExtrasCreateUser(ctx, e2e)
	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("MemberAdd", func(t *testing.T) {
		out, err := callToolOn[members.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "member_add",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"user_id":      userID,
				"access_level": 30,
			},
		})
		requireNoError(t, err, "member_add")
		requireTruef(t, out.ID == userID, "expected member ID %d, got %d", userID, out.ID)
		requireTruef(t, out.AccessLevel == 30, "expected access level 30, got %d", out.AccessLevel)
		t.Logf("Added member %s (%d) as Developer", username, out.ID)
	})

	t.Run("MemberEdit", func(t *testing.T) {
		out, err := callToolOn[members.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "member_edit",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"user_id":      userID,
				"access_level": 40,
			},
		})
		requireNoError(t, err, "member_edit")
		requireTruef(t, out.AccessLevel == 40, "expected access level 40, got %d", out.AccessLevel)
		t.Logf("Raised member %d to Maintainer", out.ID)
	})

	t.Run("MemberDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "member_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"user_id":    userID,
			},
		})
		requireNoError(t, err, "member_delete")
		requireNotListedOn(ctx, t, sess.meta, "project members after delete", "gitlab_project", map[string]any{
			"action": "members",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(out members.ListOutput) []int64 {
			ids := make([]int64, 0, len(out.Members))
			for _, m := range out.Members {
				ids = append(ids, m.ID)
			}
			return ids
		}, userID)
		t.Logf("Removed member %d", userID)
	})
}

// TestMeta_ProjectGroupSharing exercises project group-sharing actions
// through the gitlab_project meta-tool.
//
// The test creates a project fixture and a helper group, shares the project
// with the group at Developer access via share_with_group, verifies the
// group appears in list_invited_groups, and finally removes the share via
// delete_shared_group. Assertions check the share echoes the group ID and
// that the invited-groups list contains the helper group.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectGroupSharing(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)
	group := CreateGroupMeta(ctx, e2e, sess.meta, "px-share")

	t.Run("ShareWithGroup", func(t *testing.T) {
		out, err := callToolOn[projects.ShareProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "share_with_group",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"group_id":     group.ID,
				"group_access": 30,
			},
		})
		requireNoError(t, err, "share_with_group")
		requireTruef(t, out.GroupID == group.ID, "expected shared group %d, got %d", group.ID, out.GroupID)
		t.Logf("Shared project with group %d: %s", out.GroupID, out.Message)
	})

	t.Run("ListInvitedGroups", func(t *testing.T) {
		out, err := callToolOn[projects.ListProjectGroupsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_invited_groups",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "list_invited_groups")
		found := false
		for _, g := range out.Groups {
			if g.ID == group.ID {
				found = true
				break
			}
		}
		requireTruef(t, found, "expected invited groups to contain group %d, got %d entries", group.ID, len(out.Groups))
		t.Logf("Listed %d invited groups", len(out.Groups))
	})

	t.Run("DeleteSharedGroup", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete_shared_group",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"group_id":   group.ID,
			},
		})
		requireNoError(t, err, "delete_shared_group")
		requireNotListedOn(ctx, t, sess.meta, "project invited groups after unshare", "gitlab_project", map[string]any{
			"action": "list_invited_groups",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(out projects.ListProjectGroupsOutput) []int64 {
			ids := make([]int64, 0, len(out.Groups))
			for _, g := range out.Groups {
				ids = append(ids, g.ID)
			}
			return ids
		}, group.ID)
		t.Logf("Removed group share %d", group.ID)
	})
}

// TestMeta_ProjectAvatar exercises project avatar upload and download
// actions through the gitlab_project meta-tool.
//
// The test creates a project fixture, uploads a minimal in-test 1x1 PNG via
// upload_avatar, and downloads it back via download_avatar. Assertions check
// the upload round-trips to the same project and the downloaded image is
// non-empty base64 content.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectAvatar(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("UploadAvatar", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload_avatar",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"filename":       "e2e-avatar.png",
				"content_base64": projectExtrasPNGBase64,
			},
		})
		requireNoError(t, err, "upload_avatar")
		requireTruef(t, out.ID == proj.ID, "expected project %d, got %d", proj.ID, out.ID)
		t.Logf("Uploaded avatar, avatar_url=%s", out.AvatarURL)
	})

	t.Run("DownloadAvatar", func(t *testing.T) {
		out, err := callToolOn[projects.DownloadAvatarOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "download_avatar",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "download_avatar")
		requireTruef(t, out.SizeBytes > 0, "expected non-empty avatar, got %d bytes", out.SizeBytes)
		requireTruef(t, out.ContentBase64 != "", "expected base64 avatar content")
		t.Logf("Downloaded avatar: %d bytes", out.SizeBytes)
	})
}

// TestMeta_ProjectRestore exercises the project restore action through the
// gitlab_project meta-tool.
//
// The test creates a helper group and a project inside it (delayed deletion
// applies to group-namespace projects, while personal projects are deleted
// immediately), marks the project for deletion, then restores it via the
// restore action. When the instance deletes the project immediately (no
// delayed-deletion window), the restore call returns 404 and the test skips
// with a documented reason instead of failing.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectRestore(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	group := CreateGroupMeta(ctx, e2e, sess.meta, "px-restore")

	name := uniqueName("px-restore")
	created, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":         name,
			"namespace_id": group.ID,
			"visibility":   "private",
		},
	})
	requireNoError(t, err, "create group-namespace project for restore")
	projectExtrasRegisterProjectCleanup(e2e, created.ID, created.PathWithNamespace, created.Name)
	projectID := strconv.FormatInt(created.ID, 10)

	t.Run("MarkForDeletion", func(t *testing.T) {
		deleteErr := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": projectID},
		})
		requireNoError(t, deleteErr, "soft delete project")
		t.Logf("Marked project %s for deletion", projectID)
	})

	t.Run("Restore", func(t *testing.T) {
		out, restoreErr := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "restore",
			"params": map[string]any{"project_id": projectID},
		})
		if restoreErr != nil && isHTTPStatus(restoreErr, 404) {
			t.Skipf("project.restore: instance deleted the project immediately (no delayed-deletion window on this GitLab configuration): %v", restoreErr)
		}
		requireNoError(t, restoreErr, "restore project")
		requireTruef(t, out.ID == created.ID, "expected restored project %d, got %d", created.ID, out.ID)
		t.Logf("Restored project %d (marked_for_deletion_on=%q)", out.ID, out.MarkedForDeletionOn)
	})
}

// TestMeta_ProjectTransfer exercises the project transfer action through the
// gitlab_project meta-tool.
//
// The test creates a personal project fixture and a helper group, transfers
// the project into the group namespace, asserts the new path is under the
// group, then transfers it back to the personal namespace so the fixture
// cleanup path stays valid.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectTransfer(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)
	group := CreateGroupMeta(ctx, e2e, sess.meta, "px-transfer")

	t.Run("TransferToGroup", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "transfer",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"namespace":  group.Path,
			},
		})
		requireNoError(t, err, "transfer project to group")
		requireTruef(t, strings.HasPrefix(out.PathWithNamespace, group.Path+"/"),
			"expected path under group %q, got %q", group.Path, out.PathWithNamespace)
		t.Logf("Transferred project to %s", out.PathWithNamespace)
	})

	t.Run("TransferBack", func(t *testing.T) {
		// Returning the project to the personal namespace keeps the fixture
		// cleanup full_path valid for permanent deletion.
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "transfer",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"namespace":  sess.username,
			},
		})
		requireNoError(t, err, "transfer project back to personal namespace")
		requireTruef(t, strings.HasPrefix(out.PathWithNamespace, sess.username+"/"),
			"expected path under %q, got %q", sess.username, out.PathWithNamespace)
		t.Logf("Transferred project back to %s", out.PathWithNamespace)
	})
}

// TestMeta_ProjectCreateForUser exercises the admin-only create_for_user
// action through the gitlab_project meta-tool.
//
// The test creates a disposable user through the admin API (Docker mode
// only), then creates a project on that user's behalf via create_for_user
// and asserts the resulting project lives in the target user's namespace.
// The project and user are cleaned up through the per-test ledger.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta. Admin token required.
func TestMeta_ProjectCreateForUser(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	userID, username := projectExtrasCreateUser(ctx, e2e)

	name := uniqueName("px-foruser")
	out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create_for_user",
		"params": map[string]any{
			"user_id":    userID,
			"name":       name,
			"visibility": "private",
		},
	})
	requireNoError(t, err, "create_for_user")
	projectExtrasRegisterProjectCleanup(e2e, out.ID, out.PathWithNamespace, out.Name)

	requireTruef(t, out.ID > 0, "expected created project ID")
	requireTruef(t, strings.HasPrefix(out.PathWithNamespace, username+"/"),
		"expected project under %q namespace, got %q", username, out.PathWithNamespace)
	t.Logf("Created project %d for user %s: %s", out.ID, username, out.PathWithNamespace)
}

// TestMeta_ProjectForkRelations exercises fork relation create and delete
// actions through the gitlab_project meta-tool.
//
// The test creates two unrelated project fixtures, establishes a fork
// relationship between them via create_fork_relation, asserts the returned
// relation links the expected source and fork IDs, then removes the
// relationship via delete_fork_relation.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectForkRelations(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	source := CreateProjectMeta(ctx, e2e, sess.meta)
	fork := CreateProjectMeta(ctx, e2e, sess.meta)

	t.Run("CreateForkRelation", func(t *testing.T) {
		out, err := callToolOn[projects.ForkRelationOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "create_fork_relation",
			"params": map[string]any{
				"project_id":     fork.pidStr(),
				"forked_from_id": source.ID,
			},
		})
		requireNoError(t, err, "create_fork_relation")
		// GitLab 19 answers with the full project body instead of a relation
		// object, so verify the established relation through a project read.
		forked, _, getErr := sess.glClient.GL().Projects.GetProject(fork.ID, nil, gl.WithContext(ctx))
		requireNoError(t, getErr, "read fork project after create_fork_relation")
		requireTruef(t, forked.ForkedFromProject != nil && forked.ForkedFromProject.ID == source.ID,
			"expected project %d forked from %d, got %+v", fork.ID, source.ID, forked.ForkedFromProject)
		t.Logf("Created fork relation %d → %d (tool reported %d → %d)",
			source.ID, fork.ID, out.ForkedFromProjectID, out.ForkedToProjectID)
	})

	t.Run("DeleteForkRelation", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete_fork_relation",
			"params": map[string]any{"project_id": fork.pidStr()},
		})
		requireNoError(t, err, "delete_fork_relation")
		// The tool answers with the project body rather than a relation object,
		// so the relation is read back the way create_fork_relation verifies it.
		unforked, _, getErr := sess.glClient.GL().Projects.GetProject(fork.ID, nil, gl.WithContext(ctx))
		requireNoError(t, getErr, "read fork project after delete_fork_relation")
		requireTruef(t, unforked.ForkedFromProject == nil,
			"project %d still names a source project after the relation was deleted: %+v", fork.ID, unforked.ForkedFromProject)
		t.Log("Deleted fork relation")
	})
}

// TestMeta_ProjectUploadDeletes exercises markdown upload deletion actions
// through the gitlab_project meta-tool.
//
// The test creates a project fixture and uploads two small text files via
// the covered upload action, then deletes the first by numeric upload ID
// (upload_delete) and the second by the secret/filename pair parsed from its
// /uploads/<secret>/<filename> URL (upload_delete_by_secret). The by-ID
// subtest skips on instances older than GitLab 17.3, which do not return
// upload IDs.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
func TestMeta_ProjectUploadDeletes(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)

	uploadFile := func(t *testing.T, filename, content string) uploads.UploadOutput {
		t.Helper()
		out, err := callToolOn[uploads.UploadOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"filename":       filename,
				"content_base64": base64.StdEncoding.EncodeToString([]byte(content)),
			},
		})
		requireNoError(t, err, "upload file "+filename)
		return out
	}

	t.Run("UploadDeleteByID", func(t *testing.T) {
		up := uploadFile(t, "px-upload-by-id.txt", "delete me by id")
		if up.ID == 0 {
			t.Skip("GitLab pre-17.3 does not return upload IDs; upload_delete needs the numeric ID")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"upload_id":  up.ID,
			},
		})
		requireNoError(t, err, "upload_delete")
		requireNotListedOn(ctx, t, sess.meta, "project uploads after delete by ID", "gitlab_project", map[string]any{
			"action": "upload_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(out uploads.ListOutput) []int64 {
			ids := make([]int64, 0, len(out.Uploads))
			for _, u := range out.Uploads {
				ids = append(ids, u.ID)
			}
			return ids
		}, up.ID)
		t.Logf("Deleted upload %d by ID", up.ID)
	})

	t.Run("UploadDeleteBySecret", func(t *testing.T) {
		up := uploadFile(t, "px-upload-by-secret.txt", "delete me by secret")
		secret, filename := projectExtrasUploadRef(t, up.URL)
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload_delete_by_secret",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"secret":     secret,
				"filename":   filename,
			},
		})
		requireNoError(t, err, "upload_delete_by_secret")
		requireNotListedOn(ctx, t, sess.meta, "project uploads after delete by secret", "gitlab_project", map[string]any{
			"action": "upload_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		}, func(out uploads.ListOutput) []string {
			names := make([]string, 0, len(out.Uploads))
			for _, u := range out.Uploads {
				names = append(names, u.Filename)
			}
			return names
		}, filename)
		t.Logf("Deleted upload %s/%s by secret", secret, filename)
	})
}

// TestMeta_ProjectMirrorForcePush exercises the mirror_force_push action
// through the gitlab_project meta-tool.
//
// The test creates a source project and a second project acting as the push
// mirror target (reachable via the Docker-internal GitLab URL, mirroring the
// pattern in TestMeta_ProjectRemoteMirrors), adds a password-authenticated
// push mirror, then triggers a forced sync via mirror_force_push. Docker
// mode only: the mirror target URL is a Docker network address that GitLab
// cannot resolve in self-hosted runs.
//
// Build tag: e2e && !enterprise. Mode: CE (Docker only). Surface: meta.
func TestMeta_ProjectMirrorForcePush(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("mirror force push requires the Docker-internal GitLab mirror target (E2E_MODE=docker)")
	}

	e2e := NewE2EContext(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := CreateProjectMeta(ctx, e2e, sess.meta)
	target := CreateProjectMeta(ctx, e2e, sess.meta)
	mirrorURL := remoteMirrorTargetURL(t, target)

	var mirrorID int64

	t.Run("MirrorAdd", func(t *testing.T) {
		out, err := callToolOn[projectmirrors.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_add",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"url":         mirrorURL,
				"enabled":     true,
				"auth_method": "password",
			},
		})
		requireNoError(t, err, "mirror_add")
		requireTruef(t, out.ID > 0, "expected mirror ID")
		mirrorID = out.ID
		t.Logf("Added mirror %d", mirrorID)
	})

	t.Run("MirrorForcePush", func(t *testing.T) {
		requireTruef(t, mirrorID > 0, "mirrorID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "mirror_force_push",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mirror_id":  mirrorID,
			},
		})
		// GitLab rate-limits mirror syncs to one per interval; a 429/403 from
		// an earlier scheduled sync is an environment artifact, not a failure.
		if err != nil && (strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "Too Many Requests")) {
			t.Skipf("mirror sync rate-limited by GitLab: %v", err)
		}
		requireNoError(t, err, "mirror_force_push")
		t.Logf("Forced push sync for mirror %d", mirrorID)
	})
}
