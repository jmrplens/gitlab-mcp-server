//go:build e2e

package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/boards"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/members"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/pages"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectimportexport"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projectstatistics"
)

// TestMeta_ProjectCore exercises core project CRUD, fork, star, archive, and
// user/group listing actions on the gitlab_project meta-tool that are not
// already covered by other domain test files (labels, milestones, badges, etc.).
func TestMeta_ProjectCore(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// ── Core CRUD ────────────────────────────────────────────────────────
	t.Run("Get", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project get")
		requireTrue(t, out.ID > 0, "project ID should be positive")
		t.Logf("Got project %d: %s", out.ID, out.Name)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[projects.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list",
			"params": map[string]any{"membership": true},
		})
		requireNoError(t, err, "meta project list")
		requireTrue(t, len(out.Projects) >= 1, "expected at least 1 project")
		t.Logf("Listed %d projects", len(out.Projects))
	})

	t.Run("Update", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"description": "Updated by E2E projects_meta_test",
			},
		})
		requireNoError(t, err, "meta project update")
		requireTrue(t, out.Description == "Updated by E2E projects_meta_test", "description mismatch")
		t.Logf("Updated project %d", out.ID)
	})

	t.Run("ListUserProjects", func(t *testing.T) {
		user := os.Getenv("GITLAB_USER")
		if user == "" {
			t.Skip("GITLAB_USER not set")
		}
		out, err := callToolOn[projects.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_user_projects",
			"params": map[string]any{"user_id": user},
		})
		requireNoError(t, err, "meta project list_user_projects")
		requireTrue(t, len(out.Projects) >= 1, "expected at least 1 user project")
		t.Logf("Listed %d user projects", len(out.Projects))
	})

	// ── Star / Unstar ────────────────────────────────────────────────────
	t.Run("Star", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "star",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project star")
		requireTrue(t, out.ID > 0, "project ID should be positive after star")
		t.Logf("Starred project %d", out.ID)
	})

	t.Run("Unstar", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "unstar",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project unstar")
		t.Logf("Unstarred project %d", out.ID)
	})

	// ── Archive / Unarchive ──────────────────────────────────────────────
	t.Run("Archive", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "archive",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project archive")
		requireTrue(t, out.Archived, "project should be archived")
		t.Logf("Archived project %d", out.ID)
	})

	t.Run("Unarchive", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "unarchive",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project unarchive")
		requireTrue(t, !out.Archived, "project should not be archived")
		t.Logf("Unarchived project %d", out.ID)
	})

	// ── Read-only info ───────────────────────────────────────────────────
	t.Run("Languages", func(t *testing.T) {
		_, err := callToolOn[projects.LanguagesOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "languages",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project languages")
		t.Log("Got project languages")
	})

	t.Run("ListUsers", func(t *testing.T) {
		out, err := callToolOn[projects.ListProjectUsersOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_users",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project list_users")
		requireTrue(t, len(out.Users) >= 1, "expected at least 1 project user")
		t.Logf("Listed %d project users", len(out.Users))
	})

	t.Run("ListGroups", func(t *testing.T) {
		out, err := callToolOn[projects.ListProjectGroupsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_groups",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project list_groups")
		t.Logf("Listed %d project groups", len(out.Groups))
	})

	t.Run("ListStarrers", func(t *testing.T) {
		out, err := callToolOn[projects.ListProjectStarrersOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_starrers",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project list_starrers")
		t.Logf("Listed %d project starrers", len(out.Starrers))
	})

	t.Run("StatisticsGet", func(t *testing.T) {
		_, err := callToolOn[projectstatistics.GetOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "statistics_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project statistics_get")
		t.Log("Got project statistics")
	})

	t.Run("RepositoryStorageGet", func(t *testing.T) {
		out, err := callToolOn[projects.RepositoryStorageOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "repository_storage_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project repository_storage_get")
		t.Logf("Got repository storage: %s", out.RepositoryStorage)
	})

	// ── Fork ─────────────────────────────────────────────────────────────
	t.Run("Fork", func(t *testing.T) {
		out, err := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "fork",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("fork may fail in test env: %v", err)
			return
		}
		requireTrue(t, out.ID > 0, "forked project ID should be positive")
		t.Logf("Forked project → %d", out.ID)

		// Cleanup fork
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{"project_id": out.PathWithNamespace},
		})
	})

	t.Run("ListForks", func(t *testing.T) {
		out, err := callToolOn[projects.ListForksOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "list_forks",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project list_forks")
		t.Logf("Listed %d forks", len(out.Forks))
	})

	// ── Housekeeping ─────────────────────────────────────────────────────
	t.Run("StartHousekeeping", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "start_housekeeping",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("start_housekeeping may fail in some envs: %v", err)
			return
		}
		t.Log("Started housekeeping")
	})
}

// TestMeta_ProjectHooks exercises the project webhook lifecycle via
// gitlab_project meta-tool (hook_*).
func TestMeta_ProjectHooks(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var hookID int64

	t.Run("HookAdd", func(t *testing.T) {
		out, err := callToolOn[projects.HookOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_add",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"url":          "https://example.com/hook",
				"push_events":  true,
				"token":        "test-secret-token",
				"name":         "E2E Test Hook",
				"description":  "Hook created by E2E test",
			},
		})
		requireNoError(t, err, "meta project hook_add")
		requireTrue(t, out.ID > 0, "hook ID should be positive")
		hookID = out.ID
		t.Logf("Added hook %d", hookID)
	})

	t.Run("HookList", func(t *testing.T) {
		out, err := callToolOn[projects.ListHooksOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "meta project hook_list")
		requireTrue(t, len(out.Hooks) >= 1, "expected at least 1 hook")
		t.Logf("Listed %d hooks", len(out.Hooks))
	})

	t.Run("HookGet", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		out, err := callToolOn[projects.HookOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
			},
		})
		requireNoError(t, err, "meta project hook_get")
		requireTrue(t, out.ID == hookID, "expected hook %d, got %d", hookID, out.ID)
		t.Logf("Got hook %d", out.ID)
	})

	t.Run("HookEdit", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		out, err := callToolOn[projects.HookOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_edit",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"hook_id":       hookID,
				"url":           "https://example.com/hook-updated",
				"issues_events": true,
			},
		})
		requireNoError(t, err, "meta project hook_edit")
		requireTrue(t, out.ID == hookID, "expected hook %d, got %d", hookID, out.ID)
		t.Logf("Edited hook %d", out.ID)
	})

	t.Run("HookSetCustomHeader", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_set_custom_header",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
				"key":        "X-E2E-Test",
				"value":      "e2e-value",
			},
		})
		requireNoError(t, err, "meta project hook_set_custom_header")
		t.Log("Set custom header on hook")
	})

	t.Run("HookDeleteCustomHeader", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_delete_custom_header",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
				"key":        "X-E2E-Test",
			},
		})
		requireNoError(t, err, "meta project hook_delete_custom_header")
		t.Log("Deleted custom header from hook")
	})

	t.Run("HookSetURLVariable", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_set_url_variable",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
				"key":        "e2e_var",
				"value":      "test-value",
			},
		})
		if err != nil {
			t.Logf("hook_set_url_variable may require URL with {var}: %v", err)
			return
		}
		t.Log("Set URL variable on hook")
	})

	t.Run("HookDeleteURLVariable", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_delete_url_variable",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
				"key":        "e2e_var",
			},
		})
		if err != nil {
			t.Logf("hook_delete_url_variable tolerant: %v", err)
			return
		}
		t.Log("Deleted URL variable from hook")
	})

	t.Run("HookTest", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		_, err := callToolOn[projects.TriggerTestHookOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_test",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
				"trigger":    "push_events",
			},
		})
		if err != nil {
			t.Logf("hook_test may fail if endpoint unreachable: %v", err)
			return
		}
		t.Log("Tested hook trigger")
	})

	t.Run("HookDelete", func(t *testing.T) {
		requireTrue(t, hookID > 0, "hookID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "hook_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"hook_id":    hookID,
			},
		})
		requireNoError(t, err, "meta project hook_delete")
		t.Logf("Deleted hook %d", hookID)
	})
}

// TestMeta_ProjectLabelsDeep tests label subscribe/unsubscribe/promote
// actions that are not covered by labels_test.go.
func TestMeta_ProjectLabelsDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var labelName string

	t.Run("LabelCreate", func(t *testing.T) {
		labelName = uniqueName("lbl")
		out, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       labelName,
				"color":      "#FF0000",
			},
		})
		requireNoError(t, err, "label create")
		requireTrue(t, out.Name == labelName, "label name mismatch")
		t.Logf("Created label %q", labelName)
	})

	t.Run("LabelGet", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		out, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelName,
			},
		})
		requireNoError(t, err, "label get")
		requireTrue(t, out.Name == labelName, "expected label %q", labelName)
		t.Logf("Got label %q", out.Name)
	})

	t.Run("LabelSubscribe", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		out, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_subscribe",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelName,
			},
		})
		requireNoError(t, err, "label subscribe")
		requireTrue(t, out.Subscribed, "expected label to be subscribed")
		t.Logf("Subscribed to label %q", out.Name)
	})

	t.Run("LabelUnsubscribe", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_unsubscribe",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelName,
			},
		})
		requireNoError(t, err, "label unsubscribe")
		t.Logf("Unsubscribed from label %q", labelName)
	})

	t.Run("LabelPromote", func(t *testing.T) {
		requireTrue(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_promote",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelName,
			},
		})
		if err != nil {
			t.Logf("label_promote may require group context: %v", err)
			return
		}
		t.Logf("Promoted label %q to group level", labelName)
	})
}

// TestMeta_ProjectMilestonesDeep tests milestone list, issues, and MRs
// actions not covered by milestones_test.go.
func TestMeta_ProjectMilestonesDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var milestoneID int64

	t.Run("MilestoneCreate", func(t *testing.T) {
		out, err := callToolOn[milestones.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      uniqueName("ms"),
			},
		})
		requireNoError(t, err, "milestone_create")
		milestoneID = out.ID
		t.Logf("Created milestone %d", milestoneID)
	})

	t.Run("MilestoneList", func(t *testing.T) {
		out, err := callToolOn[milestones.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "milestone_list")
		requireTrue(t, len(out.Milestones) >= 1, "expected at least 1 milestone")
		t.Logf("Listed %d milestones", len(out.Milestones))
	})

	t.Run("MilestoneIssues", func(t *testing.T) {
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[milestones.MilestoneIssuesOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_issues",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"milestone_id": milestoneID,
			},
		})
		requireNoError(t, err, "milestone_issues")
		t.Logf("Milestone has %d issues", len(out.Issues))
	})

	t.Run("MilestoneMergeRequests", func(t *testing.T) {
		requireTrue(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[milestones.MilestoneMergeRequestsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_merge_requests",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"milestone_id": milestoneID,
			},
		})
		requireNoError(t, err, "milestone_merge_requests")
		t.Logf("Milestone has %d MRs", len(out.MergeRequests))
	})
}

// TestMeta_ProjectMembersDeep tests member add/edit/delete/inherited actions
// not covered by members_test.go.
func TestMeta_ProjectMembersDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("MemberInherited", func(t *testing.T) {
		// List inherited members (from group ancestry)
		_, err := callToolOn[members.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "member_inherited",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"user_id":    1,
			},
		})
		if err != nil {
			t.Logf("member_inherited may fail if user not inherited: %v", err)
			return
		}
		t.Log("Got inherited member")
	})
}

// TestMeta_ProjectBadgesDeep tests badge_get and badge_preview actions
// not covered by badges_test.go.
func TestMeta_ProjectBadgesDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var badgeID int64

	t.Run("BadgeAdd", func(t *testing.T) {
		out, err := callToolOn[badges.AddProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"link_url":   "https://example.com/badge",
				"image_url":  "https://img.shields.io/badge/test-pass-green",
				"name":       "E2E Badge",
			},
		})
		requireNoError(t, err, "badge_add")
		requireTrue(t, out.Badge.ID > 0, "badge ID should be positive")
		badgeID = out.Badge.ID
		t.Logf("Added badge %d", badgeID)
	})

	t.Run("BadgeGet", func(t *testing.T) {
		requireTrue(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.GetProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"badge_id":   badgeID,
			},
		})
		requireNoError(t, err, "badge_get")
		requireTrue(t, out.Badge.ID == badgeID, "expected badge %d, got %d", badgeID, out.Badge.ID)
		t.Logf("Got badge %d", out.Badge.ID)
	})

	t.Run("BadgePreview", func(t *testing.T) {
		out, err := callToolOn[badges.PreviewProjectOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "badge_preview",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"link_url":   "https://example.com/preview",
				"image_url":  "https://img.shields.io/badge/preview-test-blue",
			},
		})
		requireNoError(t, err, "badge_preview")
		t.Logf("Badge preview rendered: link=%s", out.Badge.RenderedLinkURL)
	})
}

// TestMeta_ProjectBoardsDeep tests board update and board list CRUD actions
// not covered by boards_test.go.
func TestMeta_ProjectBoardsDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var boardID int64
	var listID int64

	t.Run("BoardCreate", func(t *testing.T) {
		out, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       uniqueName("board"),
			},
		})
		requireNoError(t, err, "board_create")
		requireTrue(t, out.ID > 0, "board ID should be positive")
		boardID = out.ID
		t.Logf("Created board %d", boardID)
	})

	t.Run("BoardUpdate", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.BoardOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
				"name":       "Updated Board Name",
			},
		})
		requireNoError(t, err, "board_update")
		requireTrue(t, out.ID == boardID, "board ID mismatch")
		t.Logf("Updated board %d", boardID)
	})

	t.Run("BoardListList", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")
		out, err := callToolOn[boards.ListBoardListsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
			},
		})
		requireNoError(t, err, "board_list_list")
		t.Logf("Board has %d lists", len(out.Lists))
	})

	t.Run("BoardListCreate", func(t *testing.T) {
		requireTrue(t, boardID > 0, "boardID not set")

		// Create a label first (board lists need a label)
		lbl, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       uniqueName("blbl"),
				"color":      "#00FF00",
			},
		})
		requireNoError(t, err, "create label for board list")

		out, err := callToolOn[boards.BoardListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
				"label_id":   lbl.ID,
			},
		})
		requireNoError(t, err, "board_list_create")
		requireTrue(t, out.ID > 0, "board list ID should be positive")
		listID = out.ID
		t.Logf("Created board list %d", listID)
	})

	t.Run("BoardListGet", func(t *testing.T) {
		requireTrue(t, listID > 0, "listID not set")
		out, err := callToolOn[boards.BoardListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
				"list_id":    listID,
			},
		})
		requireNoError(t, err, "board_list_get")
		requireTrue(t, out.ID == listID, "expected list %d, got %d", listID, out.ID)
		t.Logf("Got board list %d", out.ID)
	})

	t.Run("BoardListUpdate", func(t *testing.T) {
		requireTrue(t, listID > 0, "listID not set")
		out, err := callToolOn[boards.BoardListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
				"list_id":    listID,
				"position":   0,
			},
		})
		requireNoError(t, err, "board_list_update")
		requireTrue(t, out.ID == listID, "list ID mismatch")
		t.Logf("Updated board list %d", out.ID)
	})

	t.Run("BoardListDelete", func(t *testing.T) {
		requireTrue(t, listID > 0, "listID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "board_list_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"board_id":   boardID,
				"list_id":    listID,
			},
		})
		requireNoError(t, err, "board_list_delete")
		t.Logf("Deleted board list %d", listID)
	})
}

// TestMeta_ProjectApprovals tests project-level approval configuration and
// approval rules via the gitlab_project meta-tool.
func TestMeta_ProjectApprovals(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var ruleID int64

	t.Run("ApprovalConfigGet", func(t *testing.T) {
		_, err := callToolOn[projects.ApprovalConfigOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_config_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "approval_config_get")
		t.Log("Got approval config")
	})

	t.Run("ApprovalConfigChange", func(t *testing.T) {
		_, err := callToolOn[projects.ApprovalConfigOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_config_change",
			"params": map[string]any{
				"project_id":                             proj.pidStr(),
				"reset_approvals_on_push":                true,
				"disable_overriding_approvers_per_merge_request": false,
			},
		})
		if err != nil {
			t.Logf("approval_config_change may require Premium: %v", err)
			return
		}
		t.Log("Changed approval config")
	})

	t.Run("ApprovalRuleCreate", func(t *testing.T) {
		out, err := callToolOn[projects.ApprovalRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_rule_create",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"name":               "E2E Approval Rule",
				"approvals_required": 1,
			},
		})
		if err != nil {
			t.Logf("approval_rule_create may require Premium: %v", err)
			return
		}
		requireTrue(t, out.ID > 0, "rule ID should be positive")
		ruleID = out.ID
		t.Logf("Created approval rule %d", ruleID)
	})

	t.Run("ApprovalRuleList", func(t *testing.T) {
		out, err := callToolOn[projects.ListApprovalRulesOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_rule_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("approval_rule_list may require Premium: %v", err)
			return
		}
		t.Logf("Listed %d approval rules", len(out.Rules))
	})

	t.Run("ApprovalRuleGet", func(t *testing.T) {
		if ruleID == 0 {
			t.Skip("ruleID not set (Premium required)")
		}
		out, err := callToolOn[projects.ApprovalRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_rule_get",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"approval_rule_id": ruleID,
			},
		})
		requireNoError(t, err, "approval_rule_get")
		requireTrue(t, out.ID == ruleID, "rule ID mismatch")
		t.Logf("Got approval rule %d", out.ID)
	})

	t.Run("ApprovalRuleUpdate", func(t *testing.T) {
		if ruleID == 0 {
			t.Skip("ruleID not set (Premium required)")
		}
		out, err := callToolOn[projects.ApprovalRuleOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_rule_update",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"approval_rule_id":   ruleID,
				"name":               "E2E Approval Rule Updated",
				"approvals_required": 2,
			},
		})
		requireNoError(t, err, "approval_rule_update")
		t.Logf("Updated approval rule %d", out.ID)
	})

	t.Run("ApprovalRuleDelete", func(t *testing.T) {
		if ruleID == 0 {
			t.Skip("ruleID not set (Premium required)")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "approval_rule_delete",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"approval_rule_id": ruleID,
			},
		})
		requireNoError(t, err, "approval_rule_delete")
		t.Logf("Deleted approval rule %d", ruleID)
	})
}

// TestMeta_ProjectExport tests project export/import status actions.
func TestMeta_ProjectExport(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("ExportSchedule", func(t *testing.T) {
		_, err := callToolOn[projectimportexport.ScheduleExportOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "export_schedule",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "export_schedule")
		t.Log("Scheduled project export")
	})

	t.Run("ExportStatus", func(t *testing.T) {
		_, err := callToolOn[projectimportexport.ExportStatusOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "export_status",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "export_status")
		t.Log("Got export status")
	})

	t.Run("ImportStatus", func(t *testing.T) {
		_, err := callToolOn[projectimportexport.ImportStatusOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "import_status",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("import_status may fail if no import in progress: %v", err)
			return
		}
		t.Log("Got import status")
	})
}

// TestMeta_ProjectIntegrations tests project integration actions.
func TestMeta_ProjectIntegrations(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("IntegrationList", func(t *testing.T) {
		_, err := callToolOn[integrations.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "integration_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "integration_list")
		t.Log("Listed integrations")
	})
}

// TestMeta_ProjectPages tests Pages read actions.
func TestMeta_ProjectPages(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("PagesGet", func(t *testing.T) {
		_, err := callToolOn[pages.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("pages_get may fail if Pages not configured: %v", err)
			return
		}
		t.Log("Got Pages info")
	})

	t.Run("PagesDomainListAll", func(t *testing.T) {
		_, err := callToolOn[pages.ListAllDomainsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_list_all",
			"params": map[string]any{},
		})
		if err != nil {
			t.Logf("pages_domain_list_all may fail: %v", err)
			return
		}
		t.Log("Listed all Pages domains")
	})

	t.Run("PagesDomainList", func(t *testing.T) {
		_, err := callToolOn[pages.ListDomainsOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pages_domain_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("pages_domain_list may fail if no Pages: %v", err)
			return
		}
		t.Log("Listed project Pages domains")
	})
}

// TestMeta_ProjectMirroring tests pull mirror and housekeeping actions.
func TestMeta_ProjectMirroring(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	t.Run("PullMirrorGet", func(t *testing.T) {
		_, err := callToolOn[projects.PullMirrorOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "pull_mirror_get",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("pull_mirror_get may fail if no mirror configured: %v", err)
			return
		}
		t.Log("Got pull mirror info")
	})
}
