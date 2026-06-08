//go:build e2e && !enterprise

// issues_meta_ce_test.go tests advanced gitlab_issue meta-tool actions against a
// live GitLab instance. Covers retrieval variants (get_by_id, list_all,
// list_group), subscribe/unsubscribe, create_todo, reorder, participants,
// closing/related MRs, time tracking, issue links, statistics, award emoji
// on notes, resource events (label, milestone, state, iteration), move,
// and work items CRUD.
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/awardemoji"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupiterations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuelinks"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issuestatistics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projectiterations"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/resourceevents"
)

// TestMeta_IssuesDeep exercises gitlab_issue meta-tool actions not covered
// by issues_test.go, issuediscussions_test.go, awardemoji_test.go, or
// stateevents_test.go.
//
//nolint:maintidx // Ordered E2E workflow keeps issue lifecycle state visible across related GitLab operations.
func TestMeta_IssuesDeep(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	commitFileMeta(ctx, t, sess.meta, proj, "main", testFileMainGo, "issue-deep test", "init commit")

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
		requireTruef(t, issueIID > 0, "issueIID not set")
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
		requireTruef(t, out.IID == issueIID, "IID mismatch")
		t.Logf("Got issue by ID %d → IID %d", got.ID, out.IID)
	})

	t.Run("ListAll", func(t *testing.T) {
		out, err := callToolOn[issues.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "list_all",
			"params": map[string]any{"scope": "all"},
		})
		requireNoError(t, err, "issue list_all")
		requireTruef(t, len(out.Issues) >= 1, "expected at least 1 issue")
		t.Logf("Listed all: %d issues", len(out.Issues))
	})

	t.Run("ListGroup", func(t *testing.T) {
		// Create a temporary group for this test
		grp, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{"name": uniqueName("grp"), "path": uniqueName("grp")},
		})
		requireNoError(t, err, "create group for list_group")
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete", "params": map[string]any{"group_id": strconv.FormatInt(grp.ID, 10)},
			})
		}()
		out, err := callToolOn[issues.ListGroupOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "list_group",
			"params": map[string]any{"group_id": strconv.FormatInt(grp.ID, 10)},
		})
		requireNoError(t, err, "list_group")
		t.Logf("Listed %d group issues", len(out.Issues))
	})

	// ── Actions on issue ─────────────────────────────────────────────────
	t.Run("Subscribe", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "subscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "subscribe")
		t.Logf("Subscribed to issue !%d", out.IID)
	})

	t.Run("Unsubscribe", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "unsubscribe",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue unsubscribe")
		t.Logf("Unsubscribed from issue !%d", out.IID)
	})

	t.Run("CreateTodo", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TodoOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create_todo",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue create_todo")
		requireTruef(t, out.ID > 0, "todo ID should be positive")
		t.Logf("Created todo %d for issue", out.ID)
	})

	t.Run("Reorder", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		// Reorder without move_after_id/move_before_id is expected to fail
		_, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "reorder",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireTruef(t, err != nil, "expected reorder to fail without move_after_id/move_before_id")
		t.Logf("Expected error for reorder without move params: %v", err)
	})

	t.Run("Participants", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.ParticipantsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "participants",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue participants")
		t.Logf("Issue has %d participants", len(out.Participants))
	})

	t.Run("MRsClosing", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.RelatedMRsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "mrs_closing",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue mrs_closing")
		t.Logf("Issue has %d closing MRs", len(out.MergeRequests))
	})

	t.Run("MRsRelated", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.RelatedMRsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "mrs_related",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "issue mrs_related")
		t.Logf("Issue has %d related MRs", len(out.MergeRequests))
	})

	// ── Time tracking ────────────────────────────────────────────────────
	t.Run("TimeEstimateSet", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
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
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "time_stats_get",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "time_stats_get")
		t.Logf("Time stats: estimate=%ds, spent=%ds", out.TimeEstimate, out.TotalTimeSpent)
	})

	t.Run("SpentTimeAdd", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
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
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "spent_time_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "spent_time_reset")
		t.Logf("Spent time reset: %ds", out.TotalTimeSpent)
	})

	t.Run("TimeEstimateReset", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.TimeStatsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "time_estimate_reset",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "time_estimate_reset")
		t.Logf("Time estimate reset: %ds", out.TimeEstimate)
	})

	// ── Issue link get ───────────────────────────────────────────────────
	t.Run("LinkGet", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
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
				"target_issue_iid":  strconv.FormatInt(issue2.IID, 10),
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
		// Create a temporary group for this test
		grp, err := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "create",
			"params": map[string]any{"name": uniqueName("grp"), "path": uniqueName("grp")},
		})
		requireNoError(t, err, "create group for statistics_get_group")
		defer func() {
			_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
				"action": "delete", "params": map[string]any{"group_id": strconv.FormatInt(grp.ID, 10)},
			})
		}()
		_, err = callToolOn[issuestatistics.StatisticsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "statistics_get_group",
			"params": map[string]any{"group_id": strconv.FormatInt(grp.ID, 10)},
		})
		requireNoError(t, err, "statistics_get_group")
		t.Log("Got group issue statistics")
	})

	// ── Award emoji on notes ─────────────────────────────────────────────
	var noteID int64
	t.Run("CreateNoteForEmoji", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
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
		requireTruef(t, noteID > 0, "noteID not set")
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
		requireTruef(t, noteID > 0, "noteID not set")
		out, err := callToolOn[awardemoji.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"note_id":    noteID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_list")
		requireTruef(t, len(out.AwardEmoji) >= 1, "expected at least 1 note emoji")
		t.Logf("Listed %d note emojis", len(out.AwardEmoji))
	})

	t.Run("EmojiIssueNoteGet", func(t *testing.T) {
		requireTruef(t, noteEmojiID > 0, "noteEmojiID not set")
		out, err := callToolOn[awardemoji.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"note_id":    noteID,
				"award_id":   noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_get")
		requireTruef(t, out.ID == noteEmojiID, "emoji ID mismatch")
		t.Logf("Got note emoji %d", out.ID)
	})

	t.Run("EmojiIssueNoteDelete", func(t *testing.T) {
		requireTruef(t, noteEmojiID > 0, "noteEmojiID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "emoji_issue_note_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"note_id":    noteID,
				"award_id":   noteEmojiID,
			},
		})
		requireNoError(t, err, "emoji_issue_note_delete")
		t.Logf("Deleted note emoji %d", noteEmojiID)
	})

	// ── Resource event setup: generate label, milestone, and state events ──
	t.Run("SetupLabelEvent", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		lbl, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-event-label",
				"color":      "#FF0000",
			},
		})
		requireNoError(t, err, "label_create for event setup")
		t.Logf("Created label %q (ID=%d)", lbl.Name, lbl.ID)

		_, err = callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"add_labels": "e2e-event-label",
			},
		})
		requireNoError(t, err, "add label to issue")
		t.Log("Added label to issue")
	})

	t.Run("SetupMilestoneEvent", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		ms, err := callToolOn[milestones.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "milestone_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"title":      "e2e-event-milestone",
			},
		})
		requireNoError(t, err, "milestone_create for event setup")
		t.Logf("Created milestone %q (ID=%d)", ms.Title, ms.ID)

		_, err = callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id":   proj.pidStr(),
				"issue_iid":    issueIID,
				"milestone_id": ms.ID,
			},
		})
		requireNoError(t, err, "set milestone on issue")
		t.Log("Set milestone on issue")
	})

	t.Run("SetupStateEvent", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		_, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"issue_iid":   issueIID,
				"state_event": "close",
			},
		})
		requireNoError(t, err, "close issue for state event")

		_, err = callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"issue_iid":   issueIID,
				"state_event": "reopen",
			},
		})
		requireNoError(t, err, "reopen issue for state event")
		t.Log("Closed and reopened issue to generate state events")
	})

	// ── Resource events ──────────────────────────────────────────────────
	t.Run("EventIssueLabelList", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "event_issue_label_list")
		t.Logf("Listed %d label events", len(out.Events))
	})

	t.Run("EventIssueMilestoneList", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "event_issue_milestone_list")
		t.Logf("Listed %d milestone events", len(out.Events))
	})

	t.Run("EventIssueStateGet", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		// List state events first to get an ID
		list, err := callToolOn[resourceevents.ListStateEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_state_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list state events to get ID")
		requireTruef(t, len(list.Events) > 0, "expected at least 1 state event after close/reopen")
		out, err := callToolOn[resourceevents.StateEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_state_get",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"issue_iid":      issueIID,
				"state_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_state_get")
		t.Logf("Got state event %d: %s", out.ID, out.State)
	})

	t.Run("EventIssueLabelGet", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		list, err := callToolOn[resourceevents.ListLabelEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_label_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list label events")
		requireTruef(t, len(list.Events) > 0, "expected at least 1 label event after adding label")
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
		requireTruef(t, issueIID > 0, "issueIID not set")
		list, err := callToolOn[resourceevents.ListMilestoneEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_milestone_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueIID},
		})
		requireNoError(t, err, "list milestone events")
		requireTruef(t, len(list.Events) > 0, "expected at least 1 milestone event after setting milestone")
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

	// ── Iteration events ─────────────────────────────────────────────────
	// Iterations require a group-scoped project and an Enterprise license.
	// We build a dedicated fixture here so list/get can verify real events.
	// IMPORTANT: setup runs at parent scope so cleanup (project/group delete)
	// fires when TestMeta_IssuesDeep ends, not when the sub-test completes;
	// otherwise the project disappears before EventIssueIterationList runs.
	var (
		iterGroupID      int64
		iterProj         ProjectFixture
		iterIssueIID     int64
		iterFixtureReady bool
	)
	if sess.enterprise {
		if fx, ok := setupIterationFixture(ctx, t); ok {
			iterGroupID = fx.groupID
			iterProj = fx.proj
			iterIssueIID = fx.issueIID
			iterFixtureReady = true
		}
	}
	t.Run("IterationFixture", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		if !iterFixtureReady {
			t.Fatal("iteration fixture setup failed; see parent log")
		}
		t.Logf("Iteration fixture ready: project %s issue !%d", iterProj.Path, iterIssueIID)
	})

	t.Run("IterationListProject", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		if !iterFixtureReady {
			t.Fatal("iteration fixture setup failed; see IterationFixture log")
		}
		out, err := callToolOn[projectiterations.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "iteration_list_project",
			"params": map[string]any{
				"project_id": iterProj.pidStr(),
				"state":      "all",
			},
		})
		requireNoError(t, err, "iteration_list_project")
		requireTruef(t, len(out.Iterations) > 0, "expected at least 1 project iteration")
		t.Logf("Listed %d project iterations", len(out.Iterations))
	})

	t.Run("IterationListGroup", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		if !iterFixtureReady {
			t.Fatal("iteration fixture setup failed; see IterationFixture log")
		}
		out, err := callToolOn[groupiterations.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "iteration_list_group",
			"params": map[string]any{
				"group_id": strconv.FormatInt(iterGroupID, 10),
				"state":    "all",
			},
		})
		requireNoError(t, err, "iteration_list_group")
		requireTruef(t, len(out.Iterations) > 0, "expected at least 1 group iteration")
		t.Logf("Listed %d group iterations", len(out.Iterations))
	})

	t.Run("EventIssueIterationList", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		if !iterFixtureReady {
			t.Fatal("iteration fixture setup failed; see IterationFixture log")
		}
		out, err := callToolOn[resourceevents.ListIterationEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_list",
			"params": map[string]any{"project_id": iterProj.pidStr(), "issue_iid": iterIssueIID},
		})
		requireNoError(t, err, "event_issue_iteration_list")
		requireTruef(t, len(out.Events) > 0, "expected at least 1 iteration event after assignment")
		t.Logf("Listed %d iteration events", len(out.Events))
	})

	t.Run("EventIssueIterationGet", func(t *testing.T) {
		if !sess.enterprise {
			return
		}
		if !iterFixtureReady {
			t.Fatal("iteration fixture setup failed; see IterationFixture log")
		}
		list, err := callToolOn[resourceevents.ListIterationEventsOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_list",
			"params": map[string]any{"project_id": iterProj.pidStr(), "issue_iid": iterIssueIID},
		})
		requireNoError(t, err, "list iteration events for get")
		requireTruef(t, len(list.Events) > 0, "expected at least 1 iteration event for get")
		_, err = callToolOn[resourceevents.IterationEventOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_get",
			"params": map[string]any{
				"project_id":         iterProj.pidStr(),
				"issue_iid":          iterIssueIID,
				"iteration_event_id": list.Events[0].ID,
			},
		})
		requireNoError(t, err, "event_issue_iteration_get")
		t.Log("Got iteration event")
	})

	// ── Move ─────────────────────────────────────────────────────────────
	t.Run("Move", func(t *testing.T) {
		requireTruef(t, issueIID > 0, "issueIID not set")
		// Create a second project to move into
		proj2 := createProjectMeta(ctx, t, sess.meta)
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "move",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issueIID,
				"to_project_id": proj2.pidStr(),
			},
		})
		requireNoError(t, err, "issue move")
		t.Logf("Moved issue to project %s → IID %d", proj2.pidStr(), out.IID)
	})
}

// iterationFixture holds resources required to exercise iteration resource
// events on an issue: a group with an iteration cadence, an iteration, a
// project inside the group, and an issue assigned to that iteration.
type iterationFixture struct {
	groupID      int64
	proj         ProjectFixture
	issueIID     int64
	iterationGID string
}

// setupIterationFixture provisions the group, iteration cadence + iteration
// (via GraphQL — there is no REST endpoint for iteration creation), a project
// inside the group, and an issue assigned to that iteration. It returns
// (fixture, false) and t.Logf-s when any step fails so callers can fail the
// iteration subtree with the setup context preserved in the parent log.
//
//nolint:maintidx // This E2E fixture is deliberately linear so each GitLab setup step logs its own failure context.
func setupIterationFixture(ctx context.Context, t *testing.T) (iterationFixture, bool) {
	t.Helper()
	if sess.glClient == nil {
		t.Logf("iteration fixture: glClient not available")
		return iterationFixture{}, false
	}
	metaSession := sess.meta

	//nolint:contextcheck // Per-test cleanup ledger is owned by NewE2EContext; the GitLab operation still receives ctx.
	grp := CreateGroupMeta(ctx, NewE2EContext(t), metaSession, "iter-grp")

	// 1. Create iteration cadence (manual; we control iteration lifecycle).
	const cadenceMutation = `mutation($groupPath: ID!, $title: String!) {
		iterationCadenceCreate(input: {
			groupPath: $groupPath,
			title: $title,
			automatic: false,
			active: true,
			durationInWeeks: 1,
			iterationsInAdvance: 1,
			rollOver: false
		}) {
			iterationCadence { id }
			errors
		}
	}`
	var cadenceResp struct {
		Data struct {
			IterationCadenceCreate struct {
				IterationCadence struct {
					ID string `json:"id"`
				} `json:"iterationCadence"`
				Errors []string `json:"errors"`
			} `json:"iterationCadenceCreate"`
		} `json:"data"`
	}
	if _, err := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: cadenceMutation,
		Variables: map[string]any{
			"groupPath": grp.Path,
			"title":     uniqueName("e2e-cadence"),
		},
	}, &cadenceResp, gl.WithContext(ctx)); err != nil {
		t.Logf("iteration fixture: cadence create failed: %v", err)
		return iterationFixture{}, false
	}
	if len(cadenceResp.Data.IterationCadenceCreate.Errors) > 0 {
		t.Logf("iteration fixture: cadence errors: %v", cadenceResp.Data.IterationCadenceCreate.Errors)
		return iterationFixture{}, false
	}
	cadenceGID := cadenceResp.Data.IterationCadenceCreate.IterationCadence.ID
	if cadenceGID == "" {
		t.Logf("iteration fixture: cadence GID empty")
		return iterationFixture{}, false
	}

	// 2. Create iteration under the cadence.
	today := time.Now().UTC().Format("2006-01-02")
	dueDate := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
	const iterationMutation = `mutation($cadenceId: IterationsCadenceID!, $groupPath: ID!, $title: String!, $startDate: String!, $dueDate: String!) {
		iterationCreate(input: {
			iterationsCadenceId: $cadenceId,
			groupPath: $groupPath,
			title: $title,
			startDate: $startDate,
			dueDate: $dueDate
		}) {
			iteration { id iid }
			errors
		}
	}`
	var iterResp struct {
		Data struct {
			IterationCreate struct {
				Iteration struct {
					ID  string `json:"id"`
					IID string `json:"iid"`
				} `json:"iteration"`
				Errors []string `json:"errors"`
			} `json:"iterationCreate"`
		} `json:"data"`
	}
	if _, err := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: iterationMutation,
		Variables: map[string]any{
			"cadenceId": cadenceGID,
			"groupPath": grp.Path,
			"title":     uniqueName("e2e-iter"),
			"startDate": today,
			"dueDate":   dueDate,
		},
	}, &iterResp, gl.WithContext(ctx)); err != nil {
		t.Logf("iteration fixture: iteration create failed: %v", err)
		return iterationFixture{}, false
	}
	if len(iterResp.Data.IterationCreate.Errors) > 0 {
		t.Logf("iteration fixture: iteration errors: %v", iterResp.Data.IterationCreate.Errors)
		return iterationFixture{}, false
	}
	iterationGID := iterResp.Data.IterationCreate.Iteration.ID
	if iterationGID == "" {
		t.Logf("iteration fixture: iteration GID empty")
		return iterationFixture{}, false
	}

	// 3. Create a project inside the group.
	projName := uniqueName(e2eProjectPrefix + "iter-")
	projOut, err := callToolOn[projects.Output](ctx, metaSession, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   projName,
			"namespace_id":           grp.ID,
			"description":            "E2E iteration fixture",
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	if err != nil {
		t.Logf("iteration fixture: group-scoped project create failed: %v", err)
		return iterationFixture{}, false
	}
	proj := ProjectFixture{ID: projOut.ID, Path: projOut.PathWithNamespace}
	//nolint:contextcheck // Cleanup must outlive the test context; uses a fresh background ctx with timeout.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = callToolVoidOn(cleanupCtx, metaSession, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(proj.ID, 10),
				"permanently_remove": true,
				"full_path":          proj.Path,
			},
		})
	})
	waitForBranchOn(ctx, t, sess.glClient, proj.ID, defaultBranch)

	// 4. Create an issue in that project.
	issueOut, err := callToolOn[issues.Output](ctx, metaSession, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      uniqueName("iter-issue"),
		},
	})
	if err != nil {
		t.Logf("iteration fixture: issue create failed: %v", err)
		return iterationFixture{}, false
	}

	// 5. Assign issue to the iteration via GraphQL issueSetIteration. Note the
	// mutation name is `issueSetIteration` (not `issuableSetIteration`) and
	// expects projectPath + iid (string) instead of an Issuable GID.
	const assignMutation = `mutation($projectPath: ID!, $iid: String!, $iterationId: IterationID!) {
		issueSetIteration(input: { projectPath: $projectPath, iid: $iid, iterationId: $iterationId }) {
			issue { iid iteration { id title state } }
			errors
		}
	}`
	var assignResp struct {
		Data struct {
			IssueSetIteration struct {
				Issue *struct {
					IID       string `json:"iid"`
					Iteration *struct {
						ID    string `json:"id"`
						Title string `json:"title"`
						State string `json:"state"`
					} `json:"iteration"`
				} `json:"issue"`
				Errors []string `json:"errors"`
			} `json:"issueSetIteration"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if _, assignErr := sess.glClient.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: assignMutation,
		Variables: map[string]any{
			"projectPath": proj.Path,
			"iid":         strconv.FormatInt(issueOut.IID, 10),
			"iterationId": iterationGID,
		},
	}, &assignResp, gl.WithContext(ctx)); assignErr != nil {
		t.Logf("iteration fixture: issueSetIteration failed: %v", assignErr)
		return iterationFixture{}, false
	}
	if len(assignResp.Data.IssueSetIteration.Errors) > 0 {
		t.Logf("iteration fixture: issueSetIteration errors: %v", assignResp.Data.IssueSetIteration.Errors)
		return iterationFixture{}, false
	}
	if len(assignResp.Errors) > 0 {
		t.Logf("iteration fixture: top-level GraphQL errors: %+v", assignResp.Errors)
		return iterationFixture{}, false
	}
	if assignResp.Data.IssueSetIteration.Issue == nil {
		t.Logf("iteration fixture: mutation returned nil issue payload (likely silent permission drop)")
		return iterationFixture{}, false
	}
	if assignResp.Data.IssueSetIteration.Issue.Iteration == nil {
		t.Logf("iteration fixture: mutation succeeded but issue.iteration is nil — GitLab dropped the assignment without erroring")
		return iterationFixture{}, false
	}
	t.Logf(
		"iteration fixture: mutation set iteration %s (%q, state=%s) on issue !%s",
		assignResp.Data.IssueSetIteration.Issue.Iteration.ID,
		assignResp.Data.IssueSetIteration.Issue.Iteration.Title,
		assignResp.Data.IssueSetIteration.Issue.Iteration.State,
		assignResp.Data.IssueSetIteration.Issue.IID,
	)

	// Resource events are emitted asynchronously by Sidekiq workers; poll up
	// to 30 s for the iteration event to materialize on the REST endpoint
	// before returning to the caller.
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		out, listErr := callToolOn[resourceevents.ListIterationEventsOutput](ctx, metaSession, "gitlab_issue", map[string]any{
			"action": "event_issue_iteration_list",
			"params": map[string]any{"project_id": proj.pidStr(), "issue_iid": issueOut.IID},
		})
		if listErr == nil && len(out.Events) > 0 {
			t.Logf("iteration fixture: %d resource_iteration_events materialized", len(out.Events))
			break
		}
		time.Sleep(1500 * time.Millisecond)
	}

	return iterationFixture{
		groupID:      grp.ID,
		proj:         proj,
		issueIID:     issueOut.IID,
		iterationGID: iterationGID,
	}, true
}
