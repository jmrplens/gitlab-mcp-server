//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrapprovals"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrcontextcommits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mrnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
)

// TestMeta_MRDeep exercises gitlab_merge_request meta-tool actions not covered
// by mergerequests_test.go, mrapproval_test.go, or stateevents_test.go.
func TestMeta_MRDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "README.md", "MR deep test", "init commit")

	// Create a branch + commit so we can open an MR
	commitFileMeta(ctx, t, sess.meta, proj, "feature-deep", "deep.txt", "feat content", "feat commit")

	var mrIID int64
	t.Run("CreateMR", func(t *testing.T) {
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"source_branch": "feature-deep",
				"target_branch": "main",
				"title":         uniqueName("deep-mr"),
			},
		})
		requireNoError(t, err, "MR create")
		mrIID = out.IID
		t.Logf("Created MR !%d", mrIID)
	})

	// ── List variants ────────────────────────────────────────────────────
	t.Run("ListGlobal", func(t *testing.T) {
		out, err := callToolOn[mergerequests.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "list_global",
			"params": map[string]any{"scope": "all"},
		})
		requireNoError(t, err, "list_global")
		requireTrue(t, len(out.MergeRequests) >= 1, "expected at least 1 MR")
		t.Logf("Listed global: %d MRs", len(out.MergeRequests))
	})

	t.Run("ListGroup", func(t *testing.T) {
		_, err := callToolOn[mergerequests.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "list_group",
			"params": map[string]any{"group_id": "1"},
		})
		if err != nil {
			t.Logf("list_group may fail without group: %v", err)
			return
		}
		t.Log("Listed group MRs")
	})

	t.Run("Reviewers", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.ReviewersOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "reviewers",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "reviewers")
		t.Logf("MR has %d reviewers", len(out.Reviewers))
	})

	t.Run("IssuesClosed", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.IssuesClosedOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "issues_closed",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "issues_closed")
		t.Logf("MR would close %d issues", len(out.Issues))
	})

	// ── Subscribe / Unsubscribe ──────────────────────────────────────────
	t.Run("Subscribe", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "subscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "subscribe")
		t.Logf("Subscribed to MR !%d", out.IID)
	})

	t.Run("Unsubscribe", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "unsubscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "unsubscribe")
		t.Logf("Unsubscribed from MR !%d", out.IID)
	})

	// ── Time tracking ────────────────────────────────────────────────────
	t.Run("TimeEstimateSet", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.TimeStatsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "time_estimate_set",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"duration":   "3h",
			},
		})
		requireNoError(t, err, "time_estimate_set")
		t.Logf("Time estimate set: %ds", out.TimeEstimate)
	})

	t.Run("TimeStats", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.TimeStatsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "time_stats",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "time_stats")
		t.Logf("Time stats: estimate=%ds, spent=%ds", out.TimeEstimate, out.TotalTimeSpent)
	})

	t.Run("SpentTimeAdd", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.TimeStatsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "spent_time_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"duration":   "1h",
			},
		})
		requireNoError(t, err, "spent_time_add")
		t.Logf("Spent time added: %ds total", out.TotalTimeSpent)
	})

	t.Run("SpentTimeReset", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.TimeStatsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "spent_time_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "spent_time_reset")
		t.Logf("Spent time reset: %ds", out.TotalTimeSpent)
	})

	t.Run("TimeEstimateReset", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mergerequests.TimeStatsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "time_estimate_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "time_estimate_reset")
		t.Logf("Time estimate reset: %ds", out.TimeEstimate)
	})

	// ── Approval state / rules / config ──────────────────────────────────
	t.Run("ApprovalState", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrapprovals.StateOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_state",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "approval_state")
		t.Logf("Approval state: overwritten=%v, rules=%d", out.ApprovalRulesOverwritten, len(out.Rules))
	})

	t.Run("ApprovalRules", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrapprovals.RulesOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_rules",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "approval_rules")
		t.Logf("MR has %d approval rules", len(out.Rules))
	})

	t.Run("ApprovalConfig", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrapprovals.ConfigOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_config",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "approval_config")
		t.Logf("Approval config for MR %d (project_id=%d)", out.IID, out.ProjectID)
	})

	var approvalRuleID int64
	t.Run("ApprovalRuleCreate", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrapprovals.RuleOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_rule_create",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"mr_iid":             mrIID,
				"name":               "test-rule",
				"approvals_required": 1,
			},
		})
		if err != nil {
			t.Logf("approval_rule_create may fail on CE: %v", err)
			return
		}
		approvalRuleID = out.ID
		t.Logf("Created approval rule %d", approvalRuleID)
	})

	t.Run("ApprovalRuleUpdate", func(t *testing.T) {
		if approvalRuleID == 0 {
			t.Skip("approvalRuleID not set")
		}
		out, err := callToolOn[mrapprovals.RuleOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_rule_update",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"mr_iid":             mrIID,
				"approval_rule_id":   approvalRuleID,
				"name":               "updated-rule",
				"approvals_required": 2,
			},
		})
		requireNoError(t, err, "approval_rule_update")
		t.Logf("Updated approval rule %d: %s", out.ID, out.Name)
	})

	t.Run("ApprovalRuleDelete", func(t *testing.T) {
		if approvalRuleID == 0 {
			t.Skip("approvalRuleID not set")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_rule_delete",
			"params": map[string]any{
				"project_id":       proj.pidStr(),
				"mr_iid":           mrIID,
				"approval_rule_id": approvalRuleID,
			},
		})
		requireNoError(t, err, "approval_rule_delete")
		t.Logf("Deleted approval rule %d", approvalRuleID)
	})

	t.Run("ApprovalReset", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "approval_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		if err != nil {
			t.Logf("approval_reset may need Premium: %v", err)
			return
		}
		t.Log("Reset approvals")
	})

	// ── Context commits ──────────────────────────────────────────────────
	t.Run("ContextCommitsList", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrcontextcommits.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "context_commits_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "context_commits_list")
		t.Logf("MR has %d context commits", len(out.Commits))
	})

	// ── Award emoji on MR ────────────────────────────────────────────────
	var mrEmojiID int64
	t.Run("EmojiMRCreate", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"name":       "rocket",
			},
		})
		requireNoError(t, err, "emoji_mr_create")
		mrEmojiID = out.ID
		t.Logf("Created MR emoji %d", mrEmojiID)
	})

	t.Run("EmojiMRList", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "emoji_mr_list")
		requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 MR emoji")
		t.Logf("Listed %d MR emojis", len(out.AwardEmoji))
	})

	t.Run("EmojiMRGet", func(t *testing.T) {
		requireTrue(t, mrEmojiID > 0, "mrEmojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"award_emoji_id": mrEmojiID,
			},
		})
		requireNoError(t, err, "emoji_mr_get")
		requireTrue(t, out.ID == mrEmojiID, "emoji ID mismatch")
		t.Logf("Got MR emoji %d", out.ID)
	})

	t.Run("EmojiMRDelete", func(t *testing.T) {
		requireTrue(t, mrEmojiID > 0, "mrEmojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_delete",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"award_emoji_id": mrEmojiID,
			},
		})
		requireNoError(t, err, "emoji_mr_delete")
		t.Logf("Deleted MR emoji %d", mrEmojiID)
	})

	// ── Award emoji on MR note ───────────────────────────────────────────
	var mrNoteID int64
	t.Run("CreateMRNoteForEmoji", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[mrnotes.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"body":       "Note for emoji test",
			},
		})
		requireNoError(t, err, "note_create for emoji")
		mrNoteID = out.ID
		t.Logf("Created MR note %d for emoji", mrNoteID)
	})

	var mrNoteEmojiID int64
	t.Run("EmojiMRNoteCreate", func(t *testing.T) {
		requireTrue(t, mrNoteID > 0, "mrNoteID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"note_id":    mrNoteID,
				"name":       "thumbsup",
			},
		})
		requireNoError(t, err, "emoji_mr_note_create")
		mrNoteEmojiID = out.ID
		t.Logf("Created MR note emoji %d", mrNoteEmojiID)
	})

	t.Run("EmojiMRNoteList", func(t *testing.T) {
		requireTrue(t, mrNoteID > 0, "mrNoteID not set")
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"mr_iid":     mrIID,
				"note_id":    mrNoteID,
			},
		})
		requireNoError(t, err, "emoji_mr_note_list")
		requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 note emoji")
		t.Logf("Listed %d MR note emojis", len(out.AwardEmoji))
	})

	t.Run("EmojiMRNoteGet", func(t *testing.T) {
		requireTrue(t, mrNoteEmojiID > 0, "mrNoteEmojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_note_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"note_id":        mrNoteID,
				"award_emoji_id": mrNoteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_mr_note_get")
		requireTrue(t, out.ID == mrNoteEmojiID, "note emoji ID mismatch")
		t.Logf("Got MR note emoji %d", out.ID)
	})

	t.Run("EmojiMRNoteDelete", func(t *testing.T) {
		requireTrue(t, mrNoteEmojiID > 0, "mrNoteEmojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "emoji_mr_note_delete",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"note_id":        mrNoteID,
				"award_emoji_id": mrNoteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_mr_note_delete")
		t.Logf("Deleted MR note emoji %d", mrNoteEmojiID)
	})

	// ── Resource events ──────────────────────────────────────────────────
	t.Run("EventMRLabelList", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "event_mr_label_list")
		t.Logf("Listed %d MR label events", len(out.Events))
	})

	t.Run("EventMRLabelGet", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		list, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "list label events for get")
		if len(list.Events) == 0 {
			t.Skip("no label events to get")
		}
		_, err = callToolOn[resourceevents.LabelEventOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_label_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"label_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_mr_label_get")
		t.Log("Got MR label event")
	})

	t.Run("EventMRMilestoneList", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		out, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "event_mr_milestone_list")
		t.Logf("Listed %d MR milestone events", len(out.Events))
	})

	t.Run("EventMRMilestoneGet", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		list, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "list milestone events for get")
		if len(list.Events) == 0 {
			t.Skip("no milestone events to get")
		}
		_, err = callToolOn[resourceevents.MilestoneEventOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_milestone_get",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"mr_iid":             mrIID,
				"milestone_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_mr_milestone_get")
		t.Log("Got MR milestone event")
	})

	t.Run("EventMRStateGet", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		list, err := callToolOn[resourceevents.ListStateEventsOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_state_list",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		requireNoError(t, err, "list state events for get")
		if len(list.Events) == 0 {
			t.Skip("no state events to get")
		}
		out, err := callToolOn[resourceevents.StateEventOutput](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "event_mr_state_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"mr_iid":         mrIID,
				"state_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_mr_state_get")
		t.Logf("Got MR state event %d: %s", out.ID, out.State)
	})

	// ── Cancel auto merge (may fail if not set) ──────────────────────────
	t.Run("CancelAutoMerge", func(t *testing.T) {
		requireTrue(t, mrIID > 0, "mrIID not set")
		_, err := callToolOn[mergerequests.Output](ctx, sess.meta, "gitlab_merge_request", map[string]any{
			"action": "cancel_auto_merge",
			"params": map[string]any{"project_id": proj.pidStr(), "mr_iid": mrIID},
		})
		if err != nil {
			t.Logf("cancel_auto_merge expected to fail (no auto_merge set): %v", err)
			return
		}
		t.Log("Cancelled auto merge")
	})
}
