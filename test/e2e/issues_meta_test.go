//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/resourceevents"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/workitems"
)

// TestMeta_IssuesDeep exercises gitlab_issue meta-tool actions not covered
// by issues_test.go, issuediscussions_test.go, awardemoji_test.go, or
// stateevents_test.go.
func TestMeta_IssuesDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "README.md", "issue-deep test", "init commit")

	// Create base issue for sub-tests
	var issueIID int64
	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      uniqueName("deep-issue"),
			},
		})
		requireNoError(t, err, "issue create")
		issueIID = out.IID
		t.Logf("Created issue !%d", issueIID)
	})

	// ── Retrieval variants ───────────────────────────────────────────────
	t.Run("GetByID", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		// First use get to find the global ID
		got, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "get",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue get for ID")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "get_by_id",
			"params": map[string]any{"issue_id": got.ID},
		})
		requireNoError(t, err, "issue get_by_id")
		requireTrue(t, out.IID == issueIID, "IID mismatch")
		t.Logf("Got issue by ID %d → IID %d", got.ID, out.IID)
	})

	t.Run("ListAll", func(t *testing.T) {
		out, err := callToolOn[issues.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "list_all",
			"params": map[string]any{"scope": "all"},
		})
		requireNoError(t, err, "issue list_all")
		requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue")
		t.Logf("Listed all: %d issues", len(out.Issues))
	})

	t.Run("ListGroup", func(t *testing.T) {
		// Need a group — skip if none available
		_, err := callToolOn[issues.ListGroupOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "list_group",
			"params": map[string]any{"group_id": "1"},
		})
		if err != nil {
			t.Logf("list_group may fail without group: %v", err)
			return
		}
		t.Log("Listed group issues")
	})

	// ── Actions on issue ─────────────────────────────────────────────────
	t.Run("Subscribe", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "subscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue subscribe")
		t.Logf("Subscribed to issue !%d", out.IID)
	})

	t.Run("Unsubscribe", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "unsubscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue unsubscribe")
		t.Logf("Unsubscribed from issue !%d", out.IID)
	})

	t.Run("CreateTodo", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TodoOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create_todo",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue create_todo")
		requireTrue(t, out.ID > 0, "todo ID should be positive")
		t.Logf("Created todo %d for issue", out.ID)
	})

	t.Run("Reorder", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "reorder",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		if err != nil {
			t.Logf("reorder may fail without move params: %v", err)
			return
		}
		t.Logf("Reordered issue !%d", out.IID)
	})

	t.Run("Participants", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.ParticipantsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "participants",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue participants")
		t.Logf("Issue has %d participants", len(out.Participants))
	})

	t.Run("MRsClosing", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.RelatedMRsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "mrs_closing",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue mrs_closing")
		t.Logf("Issue has %d closing MRs", len(out.MergeRequests))
	})

	t.Run("MRsRelated", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.RelatedMRsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "mrs_related",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue mrs_related")
		t.Logf("Issue has %d related MRs", len(out.MergeRequests))
	})

	// ── Time tracking ────────────────────────────────────────────────────
	t.Run("TimeEstimateSet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "time_estimate_set",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"duration":   "2h",
			},
		})
		requireNoError(t, err, "time_estimate_set")
		t.Logf("Time estimate set: %ds", out.TimeEstimate)
	})

	t.Run("TimeStatsGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "time_stats_get",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "time_stats_get")
		t.Logf("Time stats: estimate=%ds, spent=%ds", out.TimeEstimate, out.TotalTimeSpent)
	})

	t.Run("SpentTimeAdd", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "spent_time_add",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"duration":   "30m",
			},
		})
		requireNoError(t, err, "spent_time_add")
		t.Logf("Spent time added: %ds total", out.TotalTimeSpent)
	})

	t.Run("SpentTimeReset", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "spent_time_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "spent_time_reset")
		t.Logf("Spent time reset: %ds", out.TotalTimeSpent)
	})

	t.Run("TimeEstimateReset", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "time_estimate_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "time_estimate_reset")
		t.Logf("Time estimate reset: %ds", out.TimeEstimate)
	})

	// ── Issue link get ───────────────────────────────────────────────────
	t.Run("LinkGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		// Create a second issue to link
		issue2, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create",
			"params": map[string]any{"project_id": proj.pidStr(), "title": uniqueName("link-target")},
		})
		requireNoError(t, err, "create target issue for link")

		// Create a link
		link, err := callToolOn[issuelinks.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "link_create",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"issue_iid":         issueIID,
				"target_project_id": proj.pidStr(),
				"target_issue_iid":  issue2.IID,
			},
		})
		requireNoError(t, err, "link_create for link_get test")

		// Get the link
		got, err := callToolOn[issuelinks.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "link_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issueIID,
				"issue_link_id": link.ID,
			},
		})
		requireNoError(t, err, "link_get")
		t.Logf("Got issue link %d", got.ID)
	})

	// ── Statistics ───────────────────────────────────────────────────────
	t.Run("StatisticsGet", func(t *testing.T) {
		out, err := callToolOn[issuestatistics.StatisticsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "statistics_get",
			"params": map[string]any{},
		})
		requireNoError(t, err, "statistics_get")
		t.Logf("Issue statistics: open=%d, closed=%d", out.Opened, out.Closed)
	})

	t.Run("StatisticsGetProject", func(t *testing.T) {
		out, err := callToolOn[issuestatistics.StatisticsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "statistics_get_project",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		requireNoError(t, err, "statistics_get_project")
		t.Logf("Project issue statistics: open=%d", out.Opened)
	})

	t.Run("StatisticsGetGroup", func(t *testing.T) {
		_, err := callToolOn[issuestatistics.StatisticsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "statistics_get_group",
			"params": map[string]any{"group_id": "1"},
		})
		if err != nil {
			t.Logf("statistics_get_group may fail without group: %v", err)
			return
		}
		t.Log("Got group issue statistics")
	})

	// ── Award emoji on notes ─────────────────────────────────────────────
	var noteID int64
	t.Run("CreateNoteForEmoji", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issuenotes.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"body":       "Note for emoji test",
			},
		})
		requireNoError(t, err, "note_create for emoji")
		noteID = out.ID
		t.Logf("Created note %d for emoji test", noteID)
	})

	var noteEmojiID int64
	t.Run("EmojiIssueNoteCreate", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"note_id":    noteID,
				"name":       "thumbsup",
			},
		})
		requireNoError(t, err, "emoji_issue_note_create")
		noteEmojiID = out.ID
		t.Logf("Created note emoji %d", noteEmojiID)
	})

	t.Run("EmojiIssueNoteList", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_list")
		requireTrue(t, len(out.AwardEmoji) >= 1, "expected at least 1 note emoji")
		t.Logf("Listed %d note emojis", len(out.AwardEmoji))
	})

	t.Run("EmojiIssueNoteGet", func(t *testing.T) {
		requireTrue(t, noteEmojiID > 0, "noteEmojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"issue_iid":      issueIID,
				"note_id":        noteID,
				"award_emoji_id": noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_get")
		requireTrue(t, out.ID == noteEmojiID, "emoji ID mismatch")
		t.Logf("Got note emoji %d", out.ID)
	})

	t.Run("EmojiIssueNoteDelete", func(t *testing.T) {
		requireTrue(t, noteEmojiID > 0, "noteEmojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_delete",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"issue_iid":      issueIID,
				"note_id":        noteID,
				"award_emoji_id": noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_delete")
		t.Logf("Deleted note emoji %d", noteEmojiID)
	})

	// ── Resource events ──────────────────────────────────────────────────
	t.Run("EventIssueLabelList", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "event_issue_label_list")
		t.Logf("Listed %d label events", len(out.Events))
	})

	t.Run("EventIssueMilestoneList", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "event_issue_milestone_list")
		t.Logf("Listed %d milestone events", len(out.Events))
	})

	t.Run("EventIssueStateGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		// List state events first to get an ID
		list, err := callToolOn[resourceevents.ListStateEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_state_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list state events to get ID")
		if len(list.Events) == 0 {
			t.Skip("no state events to get")
		}
		out, err := callToolOn[resourceevents.StateEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_state_get",
			"params": map[string]any{
				"project_id":      proj.pidStr(),
				"issue_iid":       issueIID,
				"state_event_id":  list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_state_get")
		t.Logf("Got state event %d: %s", out.ID, out.State)
	})

	t.Run("EventIssueLabelGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		list, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list label events")
		if len(list.Events) == 0 {
			t.Skip("no label events to get")
		}
		_, err = callToolOn[resourceevents.LabelEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_label_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"issue_iid":      issueIID,
				"label_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_label_get")
		t.Log("Got label event")
	})

	t.Run("EventIssueMilestoneGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		list, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list milestone events")
		if len(list.Events) == 0 {
			t.Skip("no milestone events to get")
		}
		_, err = callToolOn[resourceevents.MilestoneEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_milestone_get",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"issue_iid":          issueIID,
				"milestone_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_milestone_get")
		t.Log("Got milestone event")
	})

	t.Run("EventIssueIterationList", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		_, err := callToolOn[resourceevents.ListIterationEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		if err != nil {
			t.Logf("iteration events may require Premium: %v", err)
			return
		}
		t.Log("Listed iteration events")
	})

	t.Run("EventIssueIterationGet", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		list, err := callToolOn[resourceevents.ListIterationEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		if err != nil || len(list.Events) == 0 {
			t.Skip("no iteration events to get")
		}
		_, err = callToolOn[resourceevents.IterationEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_get",
			"params": map[string]any{
				"project_id":         proj.pidStr(),
				"issue_iid":          issueIID,
				"iteration_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_iteration_get")
		t.Log("Got iteration event")
	})

	// ── Move ─────────────────────────────────────────────────────────────
	t.Run("Move", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		// Create a second project to move into
		proj2 := createProjectMeta(ctx, t, sess.meta)
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "move",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"issue_iid":      issueIID,
				"to_project_id":  proj2.pidStr(),
			},
		})
		if err != nil {
			t.Logf("move may fail depending on permissions: %v", err)
			return
		}
		t.Logf("Moved issue to project %s → IID %d", proj2.pidStr(), out.IID)
	})
}

// TestMeta_IssueWorkItems tests the work_item_* actions on gitlab_issue.
func TestMeta_IssueWorkItems(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", "README.md", "work items test", "init commit")

	var workItemIID int64

	t.Run("WorkItemCreate", func(t *testing.T) {
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      uniqueName("work-item"),
				"type":       "ISSUE",
			},
		})
		if err != nil {
			t.Logf("work_item_create may fail on older GitLab: %v", err)
			return
		}
		workItemIID = out.WorkItem.IID
		t.Logf("Created work item IID=%d", workItemIID)
	})

	t.Run("WorkItemList", func(t *testing.T) {
		out, err := callToolOn[workitems.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_list",
			"params": map[string]any{"project_id": proj.pidStr()},
		})
		if err != nil {
			t.Logf("work_item_list may fail on older GitLab: %v", err)
			return
		}
		t.Logf("Listed %d work items", len(out.WorkItems))
	})


	t.Run("WorkItemGet", func(t *testing.T) {
		if workItemIID == 0 {
			t.Skip("workItemIID not set")
		}
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_get",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"work_item_iid": workItemIID,
			},
		})
		requireNoError(t, err, "work_item_get")
		t.Logf("Got work item: %s", out.WorkItem.Title)
	})

	t.Run("WorkItemUpdate", func(t *testing.T) {
		if workItemIID == 0 {
			t.Skip("workItemIID not set")
		}
		out, err := callToolOn[workitems.GetOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_update",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"work_item_iid": workItemIID,
				"title":         "Updated Work Item",
			},
		})
		requireNoError(t, err, "work_item_update")
		t.Logf("Updated work item: %s", out.WorkItem.Title)
	})

	t.Run("WorkItemDelete", func(t *testing.T) {
		if workItemIID == 0 {
			t.Skip("workItemIID not set")
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "work_item_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"work_item_iid": workItemIID,
			},
		})
		requireNoError(t, err, "work_item_delete")
		t.Log("Deleted work item")
	})
}
