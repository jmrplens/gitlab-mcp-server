//go:build e2e && !enterprise

// groups_meta_helpers_ce_test.go contains shared helpers for advanced gitlab_group
// meta-tool E2E workflows.
//
// The helpers are split by domain (core, hooks, badges, members, labels,
// milestones, boards, and the Enterprise-gated analytics, security settings,
// SSH certs, credentials, LDAP, SAML, wikis, protected branches, and
// protected environments subtrees). The orchestrating run* functions build
// [testing.T.Run] subtrees so each domain reports independently.
//
// The Enterprise-gated subtrees are no-ops on Community Edition sessions
// when called via runMetaGroup*Operations; the underlying CE fallback
// keeps the helpers safe to call from shared test code without build-tag
// gymnastics.
package suite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/badges"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/grouplabels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmembers"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupmilestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitysettings"
)

// runMetaGroupCoreOperations exercises the cross-cutting group meta-tool
// actions (update, search, projects) that apply on both CE and EE.
// Each subtest is independent — only the update step needs the test
// group's ID.
func runMetaGroupCoreOperations(t *testing.T, ctx context.Context, grpName string, groupID int64, groupIDStr string) {
	t.Helper()
	t.Run("Update", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "update",
			"params": map[string]any{
				"group_id":    groupIDStr,
				"description": "Deep test group",
			},
		})
		requireNoError(t, err, "group update")
		t.Logf("Updated group %d", out.ID)
	})

	t.Run("Search", func(t *testing.T) {
		out, err := callToolOn[groups.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "search",
			"params": map[string]any{"query": grpName},
		})
		requireNoError(t, err, "group search")
		requireTruef(t, len(out.Groups) >= 1, "expected at least 1 group in search")
		t.Logf("Search found %d groups", len(out.Groups))
	})

	t.Run("Projects", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groups.ListProjectsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "projects",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "group projects")
		t.Logf("Group has %d projects", len(out.Projects))
	})
}

// runMetaGroupHookOperations runs the full group webhook lifecycle via
// the gitlab_group meta-tool: add → list → get → edit → delete, plus an
// additional subtest that exercises the newer hook event-fields surface.
// The shared hookID is threaded through each subtest so the orchestrator
// can hand off state cleanly.
func runMetaGroupHookOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var hookID int64
	t.Run("HookAdd", func(t *testing.T) { metaGroupHookAdd(t, ctx, groupID, groupIDStr, &hookID) })
	t.Run("HookList", func(t *testing.T) { metaGroupHookList(t, ctx, groupID, groupIDStr) })
	t.Run("HookGet", func(t *testing.T) { metaGroupHookGet(t, ctx, groupID, groupIDStr, &hookID) })
	t.Run("HookEdit", func(t *testing.T) { metaGroupHookEdit(t, ctx, groupID, groupIDStr, &hookID) })
	t.Run("HookDelete", func(t *testing.T) { metaGroupHookDelete(t, ctx, groupID, groupIDStr, &hookID) })
	t.Run("HookNewEventFields", func(t *testing.T) { metaGroupHookNewEventFields(t, ctx, groupID, groupIDStr, &hookID) })
}

// The following metaGroupHook* helpers are the bodies of each subtest
// in runMetaGroupHookOperations. They live in their own functions so
// the orchestrator stays well under the gocognit budget; each helper
// also gets a clean complexity score on its own. The shared hookID is
// passed by pointer so successive subtests can observe writes.

// metaGroupHookAdd creates a new group webhook and records its ID in
// the shared hookID slot for downstream subtests.
func metaGroupHookAdd(t *testing.T, ctx context.Context, groupID int64, groupIDStr string, hookID *int64) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	requireTruef(t, groupID > 0, "groupID not set")
	out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_add",
		"params": map[string]any{
			"group_id":    groupIDStr,
			"url":         "https://example.com/hook",
			"push_events": true,
		},
	})
	requireNoError(t, err, "hook_add")
	*hookID = out.ID
	t.Logf("Added hook %d", *hookID)
}

// metaGroupHookList verifies the hook added by metaGroupHookAdd is
// visible in the group-hook listing.
func metaGroupHookList(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	requireTruef(t, groupID > 0, "groupID not set")
	out, err := callToolOn[groups.HookListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_list",
		"params": map[string]any{"group_id": groupIDStr},
	})
	requireNoError(t, err, "hook_list")
	requireTruef(t, len(out.Hooks) >= 1, "expected at least 1 hook")
	t.Logf("Listed %d hooks", len(out.Hooks))
}

// metaGroupHookGet fetches the hook by ID and asserts the response
// matches the shared hookID.
func metaGroupHookGet(t *testing.T, ctx context.Context, groupID int64, groupIDStr string, hookID *int64) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	requireTruef(t, *hookID > 0, "hookID not set")
	out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_get",
		"params": map[string]any{"group_id": groupIDStr, "hook_id": *hookID},
	})
	requireNoError(t, err, "hook_get")
	requireTruef(t, out.ID == *hookID, "hook ID mismatch")
	t.Logf("Got hook %d", out.ID)
}

// metaGroupHookEdit updates the hook URL and toggles issues_events.
func metaGroupHookEdit(t *testing.T, ctx context.Context, groupID int64, groupIDStr string, hookID *int64) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	requireTruef(t, *hookID > 0, "hookID not set")
	out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_edit",
		"params": map[string]any{
			"group_id":      groupIDStr,
			"hook_id":       *hookID,
			"url":           "https://example.com/hook-updated",
			"issues_events": true,
		},
	})
	requireNoError(t, err, "hook_edit")
	t.Logf("Edited hook %d", out.ID)
}

// metaGroupHookDelete removes the hook created by metaGroupHookAdd.
func metaGroupHookDelete(t *testing.T, ctx context.Context, groupID int64, groupIDStr string, hookID *int64) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	requireTruef(t, *hookID > 0, "hookID not set")
	err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_delete",
		"params": map[string]any{"group_id": groupIDStr, "hook_id": *hookID},
	})
	requireNoError(t, err, "hook_delete")
	t.Logf("Deleted hook %d", *hookID)
}

// metaGroupHookNewEventFields verifies the five event-flag fields added
// in client-go v2.36.0 / exposed in the MCP group-hook surface by
// commit 9579447c:
//   - emoji_events                  (CE-available)
//   - resource_access_token_events  (EE-only, Premium/Ultimate)
//   - project_events                (EE-only, Premium/Ultimate)
//   - push_events_branch_filter     (CE-available)
//   - branch_filter_strategy        (CE-available)
//
// The subtest runs only when sess.enterprise is true so the three EE
// fields are accepted by the GitLab API. On CE, the API would silently
// ignore (or 400 on) the EE flags and the assertion would fail.
func metaGroupHookNewEventFields(t *testing.T, ctx context.Context, groupID int64, groupIDStr string, hookID *int64) {
	t.Helper()
	if !sess.enterprise {
		t.Skipf("skipping: resource_access_token_events / project_events are EE-only")
		return
	}
	requireTruef(t, *hookID > 0, "hookID not set")
	out, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_edit",
		"params": map[string]any{
			"group_id":                     groupIDStr,
			"hook_id":                      *hookID,
			"emoji_events":                 true,
			"resource_access_token_events": true,
			"project_events":               true,
			"push_events_branch_filter":    "main",
			"branch_filter_strategy":       "wildcard",
		},
	})
	requireNoError(t, err, "hook_edit new event fields")
	// Verify the read model reflects every newly-set field.
	if !out.EmojiEvents {
		t.Error("out.EmojiEvents = false, want true after edit")
	}
	if !out.ResourceAccessTokenEvents {
		t.Error("out.ResourceAccessTokenEvents = false, want true after edit")
	}
	if !out.ProjectEvents {
		t.Error("out.ProjectEvents = false, want true after edit")
	}
	if out.PushEventsBranchFilter != "main" {
		t.Errorf("out.PushEventsBranchFilter = %q, want %q", out.PushEventsBranchFilter, "main")
	}
	if out.BranchFilterStrategy != "wildcard" {
		t.Errorf("out.BranchFilterStrategy = %q, want %q", out.BranchFilterStrategy, "wildcard")
	}
	// GET and re-assert to confirm the values round-trip through the
	// GitLab API, not just the edit response echo.
	getOut, err := callToolOn[groups.HookOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "hook_get",
		"params": map[string]any{"group_id": groupIDStr, "hook_id": *hookID},
	})
	requireNoError(t, err, "hook_get after edit")
	if !getOut.EmojiEvents || !getOut.ResourceAccessTokenEvents || !getOut.ProjectEvents {
		t.Errorf("round-trip mismatch: emoji=%v resource_access=%v project=%v",
			getOut.EmojiEvents, getOut.ResourceAccessTokenEvents, getOut.ProjectEvents)
	}
	if getOut.PushEventsBranchFilter != "main" || getOut.BranchFilterStrategy != "wildcard" {
		t.Errorf("round-trip mismatch: branch_filter=%q strategy=%q",
			getOut.PushEventsBranchFilter, getOut.BranchFilterStrategy)
	}
}

// runMetaGroupBadgeOperations drives the group badge CRUD lifecycle via
// the gitlab_group meta-tool (add → list → get → edit → preview → delete).
// Preview is exercised between edit and delete so a regression in the
// render pipeline is caught independently of the storage path.
func runMetaGroupBadgeOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var badgeID int64
	t.Run("BadgeAdd", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.AddGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_add",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"link_url":  "https://example.com/badge",
				"image_url": "https://example.com/badge.svg",
			},
		})
		requireNoError(t, err, "badge_add")
		badgeID = out.Badge.ID
		t.Logf("Added group badge %d", badgeID)
	})

	t.Run("BadgeList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.ListGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "badge_list")
		requireTruef(t, len(out.Badges) >= 1, "expected at least 1 badge")
		t.Logf("Listed %d badges", len(out.Badges))
	})

	t.Run("BadgeGet", func(t *testing.T) {
		requireTruef(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.GetGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_get",
			"params": map[string]any{"group_id": groupIDStr, "badge_id": badgeID},
		})
		requireNoError(t, err, "badge_get")
		requireTruef(t, out.Badge.ID == badgeID, "badge ID mismatch")
		t.Logf("Got badge %d", out.Badge.ID)
	})

	t.Run("BadgeEdit", func(t *testing.T) {
		requireTruef(t, badgeID > 0, "badgeID not set")
		out, err := callToolOn[badges.EditGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_edit",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"badge_id":  badgeID,
				"link_url":  "https://example.com/badge-updated",
				"image_url": "https://example.com/badge-updated.svg",
			},
		})
		requireNoError(t, err, "badge_edit")
		t.Logf("Edited badge %d", out.Badge.ID)
	})

	t.Run("BadgePreview", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[badges.PreviewGroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_preview",
			"params": map[string]any{
				"group_id":  groupIDStr,
				"link_url":  "https://example.com/badge",
				"image_url": "https://example.com/badge.svg",
			},
		})
		requireNoError(t, err, "badge_preview")
		t.Logf("Preview rendered: %s", out.Badge.RenderedLinkURL)
	})

	t.Run("BadgeDelete", func(t *testing.T) {
		requireTruef(t, badgeID > 0, "badgeID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "badge_delete",
			"params": map[string]any{"group_id": groupIDStr, "badge_id": badgeID},
		})
		requireNoError(t, err, "badge_delete")
		t.Logf("Deleted badge %d", badgeID)
	})
}

// runMetaGroupMemberChecks exercises the "member not found" error path of
// the group-member meta-tool actions. Both direct and inherited lookups
// for a user that is not part of a freshly created standalone group must
// return an error so a regression that silently returns an empty struct
// would be caught.
func runMetaGroupMemberChecks(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	t.Run("GroupMemberGet", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		// User ID 1 (root) is NOT a member of a freshly created group by e2e-tester
		_, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_get",
			"params": map[string]any{
				"group_id": groupIDStr,
				"user_id":  "1",
			},
		})
		requireTruef(t, err != nil, "expected error: user 1 is not a member of the group")
		t.Logf("Expected error for non-member user: %v", err)
	})

	t.Run("GroupMemberGetInherited", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		// Standalone group has no inherited members
		_, err := callToolOn[groupmembers.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_member_get_inherited",
			"params": map[string]any{
				"group_id": groupIDStr,
				"user_id":  "1",
			},
		})
		requireTruef(t, err != nil, "expected error: user 1 is not an inherited member")
		t.Logf("Expected error for non-inherited member: %v", err)
	})
}

// runMetaGroupLabelOperations drives the group label CRUD lifecycle via
// the gitlab_group meta-tool. labelName is threaded through the subtests
// so list/delete lookups target the value created in this run.
func runMetaGroupLabelOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var labelName string
	t.Run("LabelCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		labelName = uniqueName("lbl")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     labelName,
				"color":    "#FF0000",
			},
		})
		requireNoError(t, err, "label create")
		t.Logf("Created label: %s (ID=%d)", out.Name, out.ID)
	})

	t.Run("LabelList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[grouplabels.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "label list")
		requireTruef(t, len(out.Labels) >= 1, "expected at least 1 label")
		t.Logf("Listed %d labels", len(out.Labels))
	})

	t.Run("LabelGet", func(t *testing.T) {
		requireTruef(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_get",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label get")
		t.Logf("Got label: %s", out.Name)
	})

	t.Run("LabelUpdate", func(t *testing.T) {
		requireTruef(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_update",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
				"new_name": labelName + "-upd",
				"color":    "#00FF00",
			},
		})
		requireNoError(t, err, "label update")
		labelName = out.Name
		t.Logf("Updated label: %s", labelName)
	})

	t.Run("LabelSubscribe", func(t *testing.T) {
		requireTruef(t, labelName != "", "labelName not set")
		out, err := callToolOn[grouplabels.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_subscribe",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label subscribe")
		t.Logf("Subscribed to label: %s", out.Name)
	})

	t.Run("LabelUnsubscribe", func(t *testing.T) {
		requireTruef(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_unsubscribe",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label unsubscribe")
		t.Log("Unsubscribed from label")
	})

	t.Run("LabelDelete", func(t *testing.T) {
		requireTruef(t, labelName != "", "labelName not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_label_delete",
			"params": map[string]any{
				"group_id": groupIDStr,
				"label_id": labelName,
			},
		})
		requireNoError(t, err, "label delete")
		t.Log("Deleted label")
	})
}

// runMetaGroupMilestoneOperations drives the group milestone lifecycle
// (create → get → update → issues → merge_requests) via the gitlab_group
// meta-tool. The burndown subtest only runs on Enterprise sessions because
// the underlying endpoint is Premium/Ultimate-only.
func runMetaGroupMilestoneOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var milestoneID int64
	t.Run("MilestoneGet", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		// Create a milestone to get
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"title":    uniqueName("ms-deep"),
			},
		})
		requireNoError(t, err, "milestone create")
		milestoneID = out.IID

		got, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_get",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone get")
		requireTruef(t, got.IID == milestoneID, "milestone IID mismatch")
		t.Logf("Got milestone IID %d: %s", got.IID, got.Title)
	})

	t.Run("MilestoneUpdate", func(t *testing.T) {
		requireTruef(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_update",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
				"description":   "Updated milestone",
			},
		})
		requireNoError(t, err, "milestone update")
		t.Logf("Updated milestone %d", out.ID)
	})

	t.Run("MilestoneIssues", func(t *testing.T) {
		requireTruef(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.IssuesOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_issues",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone issues")
		t.Logf("Milestone has %d issues", len(out.Issues))
	})

	t.Run("MilestoneMergeRequests", func(t *testing.T) {
		requireTruef(t, milestoneID > 0, "milestoneID not set")
		out, err := callToolOn[groupmilestones.MergeRequestsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_merge_requests",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "milestone merge_requests")
		t.Logf("Milestone has %d MRs", len(out.MergeRequests))
	})

	t.Run("MilestoneBurndown", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		requireTruef(t, milestoneID > 0, "milestoneID not set")
		_, err := callToolOn[groupmilestones.BurndownChartEventsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_milestone_burndown",
			"params": map[string]any{
				"group_id":      groupIDStr,
				"milestone_iid": milestoneID,
			},
		})
		requireNoError(t, err, "group_milestone_burndown")
		t.Log("Got burndown chart events")
	})
}

// runMetaGroupBoardOperations dispatches to the Enterprise group-board
// subtree. On Community Edition sessions it returns without running so
// the CE and EE suites share the same call site.
func runMetaGroupBoardOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	runEnterpriseMetaGroupBoardOperations(t, ctx, groupID, groupIDStr)
}

// runMetaGroupEnterpriseOperations runs every Enterprise-only gitlab_group
// meta-tool subtree. No-op on Community Edition sessions.
func runMetaGroupEnterpriseOperations(t *testing.T, ctx context.Context, groupPath string, groupID int64, groupIDStr string) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	runEnterpriseMetaGroupAnalyticsOperations(t, ctx, groupPath)
	runEnterpriseMetaGroupSecuritySettingsOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupSSHCertOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupCredentialOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupLDAPOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupSAMLOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupWikiOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupProtectedBranchOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupProtectedEnvOperations(t, ctx, groupID, groupIDStr)
}

// runEnterpriseMetaGroupAnalyticsOperations exercises the value-stream
// analytics aggregation actions on the gitlab_group meta-tool. Each subtest
// asserts the corresponding tool returns the expected shape so future
// catalog changes that drop a field are caught.
func runEnterpriseMetaGroupAnalyticsOperations(t *testing.T, ctx context.Context, groupPath string) {
	t.Helper()

	t.Run("AnalyticsIssuesCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.IssuesCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_issues_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_issues_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})

	t.Run("AnalyticsMRCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.MRCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_mr_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_mr_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})

	t.Run("AnalyticsMembersCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.MembersCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_members_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_members_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})
}

// runEnterpriseMetaGroupSecuritySettingsOperations verifies the
// security_settings_update meta-tool action toggles group secret push
// protection on the freshly created group and reports the updated flag back.
func runEnterpriseMetaGroupSecuritySettingsOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	t.Run("SecuritySettingsUpdate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[securitysettings.GroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "security_settings_update",
			"params": map[string]any{
				"group_id":                       groupIDStr,
				"secret_push_protection_enabled": true,
			},
		})
		requireNoError(t, err, "security_settings_update")
		requireTruef(t, out.SecretPushProtectionEnabled, "expected group secret push protection enabled")
	})
}

// runEnterpriseMetaGroupSSHCertOperations exercises the group-level SSH
// certificate lifecycle: list (empty), create with a freshly minted
// ed25519 key, list (one), and delete. The created certificate ID is
// scoped to the subtest so each call site can run independently.
func runEnterpriseMetaGroupSSHCertOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var certificateID int64

	t.Run("SSHCertListEmpty", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ssh_cert_list empty")
		requireTruef(t, len(out.Certificates) == 0, "expected 0 SSH certificates, got %d", len(out.Certificates))
	})

	t.Run("SSHCertCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupsshcerts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key":      generateED25519AuthorizedKey(t, "e2e-group-ssh-cert"),
				"title":    uniqueName("group-ssh-cert"),
			},
		})
		requireNoError(t, err, "ssh_cert_create")
		requireTruef(t, out.ID > 0, "expected SSH certificate ID > 0")
		certificateID = out.ID
	})

	t.Run("SSHCertListOne", func(t *testing.T) {
		requireTruef(t, certificateID > 0, "certificateID not set")
		out, err := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ssh_cert_list one")
		requireTruef(t, len(out.Certificates) >= 1, "expected at least 1 SSH certificate, got %d", len(out.Certificates))
	})

	t.Run("SSHCertDelete", func(t *testing.T) {
		requireTruef(t, certificateID > 0, "certificateID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_delete",
			"params": map[string]any{
				"group_id":       groupIDStr,
				"certificate_id": certificateID,
			},
		})
		requireNoError(t, err, "ssh_cert_delete")
		certificateID = 0
	})
}

// runEnterpriseMetaGroupCredentialOperations asserts the group credential
// inventory meta-tool actions surface the documented hint when the GitLab
// instance does not expose the credential inventory API (Ultimate +
// Owner/admin required). Without this check a silent success on a misrouted
// credential action would not be caught.
func runEnterpriseMetaGroupCredentialOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	t.Run("CredentialListPATsUnavailableHint", func(t *testing.T) {
		_, err := callToolOn[groupcredentials.PATListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_list_pats",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialListSSHKeysUnavailableHint", func(t *testing.T) {
		_, err := callToolOn[groupcredentials.SSHKeyListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_list_ssh_keys",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialRevokePATUnavailableHint", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_revoke_pat",
			"params": map[string]any{
				"group_id": groupIDStr,
				"token_id": int64(999999),
			},
		})
		requireErrorContainsAll(t, err, "credential_list_pats", "group credential inventory", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialDeleteSSHKeyUnavailableHint", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_delete_ssh_key",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key_id":   int64(999999),
			},
		})
		requireErrorContainsAll(t, err, "credential_list_ssh_keys", "group credential inventory", "Ultimate", "Owner or admin")
	})
}

// requireErrorContainsAll fails the test unless err is non-nil and its
// message contains every fragment. Used to assert that error responses
// carry the action-specific guidance the meta-tool surface is contracted
// to surface (license tier hints, next-step URLs, etc.).
func requireErrorContainsAll(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errText := err.Error()
	for _, fragment := range fragments {
		if !strings.Contains(errText, fragment) {
			t.Fatalf("error %q does not contain %q", errText, fragment)
		}
	}
}

// runEnterpriseMetaGroupLDAPOperations exercises the group LDAP link
// lifecycle (list/add/list/delete and delete-for-provider). Each subtest
// asserts the expected side effect so a regression that drops the link
// silently from the response is caught.
func runEnterpriseMetaGroupLDAPOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	provider := "ldapmain"
	cn := uniqueName("ldap-cn")
	providerCN := uniqueName("ldap-provider-cn")

	t.Run("LDAPLinkListEmpty", func(t *testing.T) {
		out, err := callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ldap_link_list empty")
		requireTruef(t, len(out.Links) == 0, "expected no LDAP links, got %d", len(out.Links))
	})

	t.Run("LDAPLinkAdd", func(t *testing.T) {
		out, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"cn":           cn,
				"group_access": 30,
				"provider":     provider,
			},
		})
		requireNoError(t, err, "ldap_link_add")
		requireTruef(t, out.CN == cn, "LDAP link CN mismatch: got %q want %q", out.CN, cn)
	})

	t.Run("LDAPLinkListOne", func(t *testing.T) {
		out, err := callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ldap_link_list one")
		requireTruef(t, len(out.Links) >= 1, "expected at least 1 LDAP link, got %d", len(out.Links))
	})

	t.Run("LDAPLinkDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_delete",
			"params": map[string]any{
				"group_id": groupIDStr,
				"cn":       cn,
				"provider": provider,
			},
		})
		requireNoError(t, err, "ldap_link_delete")
	})

	t.Run("LDAPLinkDeleteForProvider", func(t *testing.T) {
		_, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"cn":           providerCN,
				"group_access": 30,
				"provider":     provider,
			},
		})
		requireNoError(t, err, "ldap_link_add for provider delete")

		err = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_delete_for_provider",
			"params": map[string]any{
				"group_id": groupIDStr,
				"provider": provider,
				"cn":       providerCN,
			},
		})
		requireNoError(t, err, "ldap_link_delete_for_provider")
	})
}

// runEnterpriseMetaGroupSAMLOperations verifies the SAML link meta-tool
// actions fail with the documented license/SSO hint when the GitLab
// instance does not have SSO configured. The error path is the only
// deterministic behavior on a stock EE Ultimate sandbox without an SSO
// provider wired up, so we assert it instead of expecting success.
func runEnterpriseMetaGroupSAMLOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	samlGroup := uniqueName("saml-group")

	t.Run("SAMLLinkListRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkAddRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_add",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
				"access_level":    30,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkGetRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_get",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkDeleteRequiresConfiguredSSO", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_delete",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})
}

// runEnterpriseMetaGroupBoardOperations drives the group board CRUD
// lifecycle via the gitlab_group meta-tool: create → list → get → update →
// list-lists → delete. The list subtest polls with backoff because GitLab
// is eventually consistent and the created board may not appear in the
// list response for a few seconds after the create call returns.
func runEnterpriseMetaGroupBoardOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var boardID int64

	t.Run("BoardCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     "Test Board",
			},
		})
		requireNoError(t, err, "group_board_create")
		boardID = out.ID
		t.Logf("Created group board %d", boardID)
	})

	t.Run("BoardList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		// Poll the list endpoint until the board created in BoardCreate is
		// visible. GitLab is eventually consistent — the list may not reflect
		// the create for a few seconds after it returns.
		requireTruef(t, boardID > 0, "boardID not set by BoardCreate")
		var lastBoards []groupboards.GroupBoardOutput
		_, listErr := retryWithBackoff(ctx, t, "group_board_list find created", 5, func(int) (struct{}, bool, string, error) {
			out, err := callToolOn[groupboards.ListGroupBoardsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "group_board_list",
				"params": map[string]any{"group_id": groupIDStr},
			})
			if err != nil {
				return struct{}{}, true, "transient list error", err
			}
			lastBoards = out.Boards
			for _, b := range out.Boards {
				if b.ID == boardID {
					return struct{}{}, false, "", nil
				}
			}
			return struct{}{}, true, "newly created board not yet visible in list", nil
		})
		requireNoError(t, listErr, "group_board_list")
		// At minimum the created board must be present.
		found := false
		for _, b := range lastBoards {
			if b.ID == boardID {
				found = true
				break
			}
		}
		requireTruef(t, found, "created board ID=%d not visible in group_board_list after retries", boardID)
		t.Logf("Listed %d group boards (created board present)", len(lastBoards))
	})

	t.Run("BoardGet", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_get",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "board_get")
		requireTruef(t, out.ID == boardID, "board ID mismatch")
		t.Logf("Got board %d: %s", out.ID, out.Name)
	})

	t.Run("BoardUpdate", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_update",
			"params": map[string]any{
				"group_id": groupIDStr,
				"board_id": boardID,
				"name":     "Updated Board",
			},
		})
		requireNoError(t, err, "group_board_update")
		t.Logf("Updated board %d: %s", out.ID, out.Name)
	})

	t.Run("BoardListLists", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.ListBoardListsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_list_lists",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "board_list_lists")
		t.Logf("Board has %d lists", len(out.Lists))
	})

	t.Run("BoardDelete", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_delete",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "group_board_delete")
		t.Logf("Deleted board %d", boardID)
	})
}

// generateED25519AuthorizedKey returns a freshly minted ed25519 public key
// formatted as an OpenSSH authorized-keys line with the supplied comment.
// Used to seed the SSH certificate create action without relying on disk
// fixtures.
func generateED25519AuthorizedKey(t *testing.T, comment string) string {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	requireNoError(t, err, "generate ed25519 public key")

	const keyType = "ssh-ed25519"
	payload := make([]byte, 0, 4+len(keyType)+4+len(publicKey))
	payload = appendLengthPrefixed(payload, []byte(keyType))
	payload = appendLengthPrefixed(payload, publicKey)

	return keyType + " " + base64.StdEncoding.EncodeToString(payload) + " " + comment
}

// maxSSHWireFieldLength is the maximum value an OpenSSH wire-format length
// prefix (uint32) can encode. We panic before generating keys with a field
// that would exceed it.
const maxSSHWireFieldLength = 1<<32 - 1

// appendLengthPrefixed appends value to dst prefixed with its big-endian
// uint32 length, matching the SSH wire format used by OpenSSH authorized
// keys.
func appendLengthPrefixed(dst, value []byte) []byte {
	lengthValue := uint64(len(value))
	if lengthValue > maxSSHWireFieldLength {
		panic("ssh key field is too large")
	}

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(lengthValue))
	dst = append(dst, length[:]...)
	dst = append(dst, value...)
	return dst
}

// runEnterpriseMetaGroupWikiOperations drives the group wiki page CRUD
// lifecycle (create → list → get → update → delete).
func runEnterpriseMetaGroupWikiOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var wikiSlug string

	t.Run("WikiCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"title":    uniqueName("group-wiki"),
				"content":  "# E2E group wiki\n\nInitial content.",
				"format":   "markdown",
			},
		})
		requireNoError(t, err, "wiki_create")
		requireTruef(t, out.Slug != "", "expected wiki slug")
		wikiSlug = out.Slug
		t.Logf("Created group wiki page %s", wikiSlug)
	})

	t.Run("WikiList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupwikis.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_list",
			"params": map[string]any{"group_id": groupIDStr, "with_content": true},
		})
		requireNoError(t, err, "wiki_list")
		requireTruef(t, len(out.WikiPages) >= 1, "expected at least 1 group wiki page")
		t.Logf("Listed %d group wiki pages", len(out.WikiPages))
	})

	t.Run("WikiGet", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_get",
			"params": map[string]any{"group_id": groupIDStr, "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_get")
		requireTruef(t, out.Slug == wikiSlug, "wiki slug mismatch: got %q want %q", out.Slug, wikiSlug)
		t.Logf("Got group wiki page %s", out.Slug)
	})

	t.Run("WikiEdit", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_edit",
			"params": map[string]any{
				"group_id": groupIDStr,
				"slug":     wikiSlug,
				"content":  "# E2E group wiki\n\nUpdated content.",
			},
		})
		requireNoError(t, err, "wiki_edit")
		t.Logf("Updated group wiki page %s", out.Slug)
	})

	t.Run("WikiDelete", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_delete",
			"params": map[string]any{"group_id": groupIDStr, "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_delete")
		t.Logf("Deleted group wiki page %s", wikiSlug)
	})
}

// runEnterpriseMetaGroupProtectedBranchOperations exercises the group
// protected branch lifecycle (protect → list → get → update → unprotect)
// via the gitlab_group meta-tool. The created rule is wildcarded so it
// does not collide with real release branches on the test group.
func runEnterpriseMetaGroupProtectedBranchOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	branchName := uniqueName("release-") + "/*"

	t.Run("ProtectedBranchProtect", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_protect",
			"params": map[string]any{
				"group_id":               groupIDStr,
				"name":                   branchName,
				"push_access_level":      40,
				"merge_access_level":     40,
				"unprotect_access_level": 40,
				"allow_force_push":       false,
			},
		})
		requireNoError(t, err, "protected_branch_protect")
		requireTruef(t, out.Name == branchName, "protected branch name mismatch: got %q want %q", out.Name, branchName)
		t.Logf("Protected group branch %s", out.Name)
	})

	t.Run("ProtectedBranchList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedbranches.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_list",
			"params": map[string]any{"group_id": groupIDStr, "search": branchName},
		})
		requireNoError(t, err, "protected_branch_list")
		requireTruef(t, len(out.Branches) >= 1, "expected at least 1 group protected branch")
		t.Logf("Listed %d group protected branches", len(out.Branches))
	})

	t.Run("ProtectedBranchGet", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_get",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_get")
		requireTruef(t, out.Name == branchName, "protected branch name mismatch")
		t.Logf("Got group protected branch %s", out.Name)
	})

	t.Run("ProtectedBranchUpdate", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_update",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName, "allow_force_push": true},
		})
		requireNoError(t, err, "protected_branch_update")
		requireTruef(t, out.AllowForcePush, "expected allow_force_push=true after update")
		t.Logf("Updated group protected branch %s", out.Name)
	})

	t.Run("ProtectedBranchUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_unprotect",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_unprotect")
		t.Logf("Unprotected group branch %s", branchName)
	})
}

// runEnterpriseMetaGroupProtectedEnvOperations exercises the group
// protected environment lifecycle (invalid-tier hint → protect → list →
// get → update → unprotect). The update step targets an existing deploy
// access level by ID and switches its access tier, proving the update path
// keeps the resource identifier stable across edits.
func runEnterpriseMetaGroupProtectedEnvOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	const envName = "production"
	const developerAccessLevel = 30
	var protectedEnvDeployLevelID int64

	t.Run("ProtectedEnvInvalidTierHint", func(t *testing.T) {
		_, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     uniqueName("production-"),
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireTruef(t, err != nil, "expected protected_env_protect invalid tier error")
		requireTruef(t, strings.Contains(err.Error(), "valid group protected environment tiers"), "expected valid tier hint, got %v", err)
		requireTruef(t, strings.Contains(err.Error(), "production"), "expected tier values in error, got %v", err)
	})

	t.Run("ProtectedEnvProtect", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     envName,
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireNoError(t, err, "protected_env_protect")
		requireTruef(t, out.Name == envName, "protected environment name mismatch: got %q want %q", out.Name, envName)
		requireTruef(t, len(out.DeployAccessLevels) >= 1, "expected at least 1 deploy access level")
		protectedEnvDeployLevelID = out.DeployAccessLevels[0].ID
		requireTruef(t, protectedEnvDeployLevelID > 0, "expected deploy access level ID")
		t.Logf("Protected group environment %s", out.Name)
	})

	t.Run("ProtectedEnvList", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "protected_env_list")
		requireTruef(t, len(out.Environments) >= 1, "expected at least 1 group protected environment")
		t.Logf("Listed %d group protected environments", len(out.Environments))
	})

	t.Run("ProtectedEnvGet", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_get",
			"params": map[string]any{"group_id": groupIDStr, "environment": envName},
		})
		requireNoError(t, err, "protected_env_get")
		requireTruef(t, out.Name == envName, "protected environment name mismatch")
		t.Logf("Got group protected environment %s", out.Name)
	})

	t.Run("ProtectedEnvUpdate", func(t *testing.T) {
		requireTruef(t, protectedEnvDeployLevelID > 0, "protectedEnvDeployLevelID not set")
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_update",
			"params": map[string]any{
				"group_id":    groupIDStr,
				"environment": envName,
				"deploy_access_levels": []map[string]any{
					{"id": protectedEnvDeployLevelID, "access_level": developerAccessLevel},
				},
			},
		})
		requireNoError(t, err, "protected_env_update")
		requireTruef(t, out.Name == envName, "protected environment name mismatch after update: got %q want %q", out.Name, envName)
		foundDeveloperAccess := false
		for _, deployAccessLevel := range out.DeployAccessLevels {
			if deployAccessLevel.AccessLevel == developerAccessLevel {
				foundDeveloperAccess = true
				break
			}
		}
		requireTruef(t, foundDeveloperAccess, "expected deploy access level %d after update", developerAccessLevel)
		t.Logf("Updated group protected environment %s", out.Name)
	})

	t.Run("ProtectedEnvUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_unprotect",
			"params": map[string]any{"group_id": groupIDStr, "environment": envName},
		})
		requireNoError(t, err, "protected_env_unprotect")
		t.Logf("Unprotected group environment %s", envName)
	})
}
