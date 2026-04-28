//go:build e2e

// epics_test.go exercises epic-related tools via the gitlab_group meta-tool:
// epic CRUD (Work Items API), epic notes (GraphQL), epic discussions (GraphQL),
// and epic issues (GraphQL hierarchy widget). All actions are enterprise-gated
// (GITLAB_ENTERPRISE=true).
package suite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicdiscussions"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicissues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epicnotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/epics"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/groups"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
)

// TestMeta_Epics exercises epic CRUD via the gitlab_group meta-tool.
// Epics use the Work Items GraphQL API (full_path + iid parameters).
func TestMeta_Epics(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test group.
	grpName := uniqueName("grp-epic")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupPath := grpOut.FullPath
	t.Logf("Created group %d: %s (full_path=%s)", grpOut.ID, grpName, groupPath)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
	}()

	// ── Create epic ──────────────────────────────────────────────────────
	var epicIID int64
	t.Run("EpicCreate", func(t *testing.T) {
		out, err := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_create",
			"params": map[string]any{
				"full_path":   groupPath,
				"title":       "E2E Epic Test",
				"description": "Created by E2E test",
			},
		})
		requirePremiumFeature(t, err, "epic_create")
		requireTrue(t, out.IID > 0, "epic IID should be > 0, got %d", out.IID)
		requireTrue(t, out.Title == "E2E Epic Test", "epic title mismatch: %s", out.Title)
		epicIID = out.IID
		t.Logf("Created epic IID=%d (ID=%d)", out.IID, out.ID)
	})

	// ── List epics ───────────────────────────────────────────────────────
	t.Run("EpicList", func(t *testing.T) {
		requireTrue(t, epicIID > 0, "epicIID not set")
		out, err := callToolOn[epics.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_list",
			"params": map[string]any{
				"full_path": groupPath,
			},
		})
		requireNoError(t, err, "epic_list")
		requireTrue(t, len(out.Epics) >= 1, "expected at least 1 epic, got %d", len(out.Epics))
		t.Logf("Listed %d epic(s)", len(out.Epics))
	})

	// ── Get epic ─────────────────────────────────────────────────────────
	t.Run("EpicGet", func(t *testing.T) {
		requireTrue(t, epicIID > 0, "epicIID not set")
		out, err := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_get",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_get")
		requireTrue(t, out.IID == epicIID, "epic IID mismatch: want %d, got %d", epicIID, out.IID)
		requireTrue(t, out.Title == "E2E Epic Test", "epic title mismatch: %s", out.Title)
		t.Logf("Got epic IID=%d: %s", out.IID, out.Title)
	})

	// ── Update epic ──────────────────────────────────────────────────────
	t.Run("EpicUpdate", func(t *testing.T) {
		requireTrue(t, epicIID > 0, "epicIID not set")
		out, err := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_update",
			"params": map[string]any{
				"full_path":   groupPath,
				"epic_iid":    epicIID,
				"description": "Updated by E2E test",
			},
		})
		requireNoError(t, err, "epic_update")
		requireTrue(t, out.Description == "Updated by E2E test", "epic description not updated: %s", out.Description)
		t.Logf("Updated epic IID=%d", out.IID)
	})

	// ── Delete epic ──────────────────────────────────────────────────────
	t.Run("EpicDelete", func(t *testing.T) {
		requireTrue(t, epicIID > 0, "epicIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_delete",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_delete")
		t.Log("Deleted epic successfully")
	})
}

// TestMeta_EpicNotes exercises epic note CRUD via the gitlab_group meta-tool.
// Notes use the Work Items GraphQL API (full_path + iid + note_id).
func TestMeta_EpicNotes(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test group and epic.
	grpName := uniqueName("grp-epicnote")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupPath := grpOut.FullPath
	t.Logf("Created group: %s", groupPath)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
	}()

	epicOut, setupErr := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_create",
		"params": map[string]any{
			"full_path": groupPath,
			"title":     "E2E Epic for Notes",
		},
	})
	requireNoError(t, setupErr, "create epic for notes")
	epicIID := epicOut.IID
	t.Logf("Created epic IID=%d for notes test", epicIID)

	// ── Create note ──────────────────────────────────────────────────────
	var noteID int64
	t.Run("NoteCreate", func(t *testing.T) {
		out, err := callToolOn[epicnotes.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_note_create",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"body":      "E2E test note body",
			},
		})
		requireNoError(t, err, "epic_note_create")
		requireTrue(t, out.ID > 0, "note ID should be > 0, got %d", out.ID)
		requireTrue(t, out.Body == "E2E test note body", "note body mismatch: %s", out.Body)
		noteID = out.ID
		t.Logf("Created note ID=%d", noteID)
	})

	// ── List notes ───────────────────────────────────────────────────────
	t.Run("NoteList", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[epicnotes.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_note_list",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_note_list")
		requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note, got %d", len(out.Notes))
		t.Logf("Listed %d note(s)", len(out.Notes))
	})

	// ── Get note ─────────────────────────────────────────────────────────
	t.Run("NoteGet", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[epicnotes.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_note_get",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"note_id":   noteID,
			},
		})
		requireNoError(t, err, "epic_note_get")
		requireTrue(t, out.ID == noteID, "note ID mismatch: want %d, got %d", noteID, out.ID)
		t.Logf("Got note ID=%d: %s", out.ID, out.Body)
	})

	// ── Update note ──────────────────────────────────────────────────────
	t.Run("NoteUpdate", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		out, err := callToolOn[epicnotes.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_note_update",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"note_id":   noteID,
				"body":      "Updated E2E note body",
			},
		})
		requireNoError(t, err, "epic_note_update")
		requireTrue(t, out.Body == "Updated E2E note body", "note body not updated: %s", out.Body)
		t.Logf("Updated note ID=%d", out.ID)
	})

	// ── Delete note ──────────────────────────────────────────────────────
	t.Run("NoteDelete", func(t *testing.T) {
		requireTrue(t, noteID > 0, "noteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_note_delete",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"note_id":   noteID,
			},
		})
		requireNoError(t, err, "epic_note_delete")
		t.Log("Deleted note successfully")
	})
}

// TestMeta_EpicDiscussions exercises epic discussion CRUD via the gitlab_group
// meta-tool. Discussions use the Work Items GraphQL API.
func TestMeta_EpicDiscussions(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test group and epic.
	grpName := uniqueName("grp-epicdisc")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupPath := grpOut.FullPath
	t.Logf("Created group: %s", groupPath)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
	}()

	epicOut, setupErr := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_create",
		"params": map[string]any{
			"full_path": groupPath,
			"title":     "E2E Epic for Discussions",
		},
	})
	requireNoError(t, setupErr, "create epic for discussions")
	epicIID := epicOut.IID
	t.Logf("Created epic IID=%d for discussions test", epicIID)

	// ── Create discussion ────────────────────────────────────────────────
	var discussionID string
	var firstNoteID int64
	t.Run("DiscussionCreate", func(t *testing.T) {
		out, err := callToolOn[epicdiscussions.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_create",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"body":      "E2E discussion thread",
			},
		})
		requireNoError(t, err, "epic_discussion_create")
		requireTrue(t, out.ID != "", "discussion ID should not be empty")
		requireTrue(t, len(out.Notes) >= 1, "discussion should have at least 1 note")
		discussionID = out.ID
		firstNoteID = out.Notes[0].ID
		t.Logf("Created discussion ID=%s with note ID=%d", discussionID, firstNoteID)
	})

	// ── List discussions ─────────────────────────────────────────────────
	t.Run("DiscussionList", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[epicdiscussions.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_list",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_discussion_list")
		requireTrue(t, len(out.Discussions) >= 1, "expected at least 1 discussion, got %d", len(out.Discussions))
		t.Logf("Listed %d discussion(s)", len(out.Discussions))
	})

	// ── Get discussion ───────────────────────────────────────────────────
	t.Run("DiscussionGet", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[epicdiscussions.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_get",
			"params": map[string]any{
				"full_path":     groupPath,
				"epic_iid":      epicIID,
				"discussion_id": discussionID,
			},
		})
		requireNoError(t, err, "epic_discussion_get")
		requireTrue(t, out.ID == discussionID, "discussion ID mismatch: want %s, got %s", discussionID, out.ID)
		t.Logf("Got discussion ID=%s with %d note(s)", out.ID, len(out.Notes))
	})

	// ── Add note to discussion ───────────────────────────────────────────
	var replyNoteID int64
	t.Run("DiscussionAddNote", func(t *testing.T) {
		requireTrue(t, discussionID != "", "discussionID not set")
		out, err := callToolOn[epicdiscussions.NoteOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_add_note",
			"params": map[string]any{
				"full_path":     groupPath,
				"epic_iid":      epicIID,
				"discussion_id": discussionID,
				"body":          "E2E reply note",
			},
		})
		requireNoError(t, err, "epic_discussion_add_note")
		requireTrue(t, out.ID > 0, "reply note ID should be > 0")
		requireTrue(t, out.Body == "E2E reply note", "reply body mismatch: %s", out.Body)
		replyNoteID = out.ID
		t.Logf("Added reply note ID=%d to discussion %s", replyNoteID, discussionID)
	})

	// ── Update note in discussion ────────────────────────────────────────
	t.Run("DiscussionUpdateNote", func(t *testing.T) {
		requireTrue(t, replyNoteID > 0, "replyNoteID not set")
		out, err := callToolOn[epicdiscussions.NoteOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_update_note",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"note_id":   replyNoteID,
				"body":      "Updated E2E reply",
			},
		})
		requireNoError(t, err, "epic_discussion_update_note")
		requireTrue(t, out.Body == "Updated E2E reply", "note body not updated: %s", out.Body)
		t.Logf("Updated note ID=%d", out.ID)
	})

	// ── Delete note from discussion ──────────────────────────────────────
	t.Run("DiscussionDeleteNote", func(t *testing.T) {
		requireTrue(t, replyNoteID > 0, "replyNoteID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_discussion_delete_note",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
				"note_id":   replyNoteID,
			},
		})
		requireNoError(t, err, "epic_discussion_delete_note")
		t.Log("Deleted discussion note successfully")
	})
}

// TestMeta_EpicIssues exercises epic-issue child management via the
// gitlab_group meta-tool. Uses the Work Items GraphQL hierarchy widget.
func TestMeta_EpicIssues(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	// Create a test group.
	grpName := uniqueName("grp-epiciss")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupPath := grpOut.FullPath
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	t.Logf("Created group: %s (ID=%s)", groupPath, groupIDStr)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	// Create a project inside the group to hold issues.
	projName := uniqueName(e2eProjectPrefix + "epiciss")
	projOut, setupErr := callToolOn[projects.Output](ctx, sess.meta, "gitlab_project", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":                   projName,
			"namespace_id":           grpOut.ID,
			"description":            "E2E project for epic issues",
			"visibility":             "private",
			"initialize_with_readme": true,
			"default_branch":         defaultBranch,
		},
	})
	requireNoError(t, setupErr, "create project in group")
	t.Logf("Created project: %s (ID=%d)", projOut.PathWithNamespace, projOut.ID)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id":         strconv.FormatInt(projOut.ID, 10),
				"permanently_remove": true,
				"full_path":          projOut.PathWithNamespace,
			},
		})
	}()

	// Create an epic.
	epicOut, setupErr := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_create",
		"params": map[string]any{
			"full_path": groupPath,
			"title":     "E2E Epic for Issues",
		},
	})
	requireNoError(t, setupErr, "create epic for issues")
	epicIID := epicOut.IID
	t.Logf("Created epic IID=%d", epicIID)

	// Create an issue in the project.
	issueOut, setupErr := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  strconv.FormatInt(projOut.ID, 10),
			"title":       "E2E Issue for Epic",
			"description": "Test issue to assign to epic",
		},
	})
	requireNoError(t, setupErr, "create issue")
	issueIID := issueOut.IID
	t.Logf("Created issue IID=%d in project %s", issueIID, projOut.PathWithNamespace)

	// ── Assign issue to epic ─────────────────────────────────────────────
	t.Run("EpicIssueAssign", func(t *testing.T) {
		out, err := callToolOn[epicissues.AssignOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_issue_assign",
			"params": map[string]any{
				"full_path":          groupPath,
				"epic_iid":           epicIID,
				"child_project_path": projOut.PathWithNamespace,
				"child_iid":          issueIID,
			},
		})
		requireNoError(t, err, "epic_issue_assign")
		requireTrue(t, out.EpicGID != "", "EpicGID should not be empty")
		requireTrue(t, out.ChildGID != "", "ChildGID should not be empty")
		t.Logf("Assigned issue to epic: EpicGID=%s ChildGID=%s", out.EpicGID, out.ChildGID)
	})

	// ── List epic issues ─────────────────────────────────────────────────
	t.Run("EpicIssueList", func(t *testing.T) {
		out, err := callToolOn[epicissues.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_issue_list",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_issue_list")
		requireTrue(t, len(out.Issues) >= 1, "expected at least 1 child issue, got %d", len(out.Issues))
		found := false
		for _, iss := range out.Issues {
			if iss.IID == issueIID {
				found = true
				break
			}
		}
		requireTrue(t, found, "assigned issue IID=%d not found in epic children", issueIID)
		t.Logf("Listed %d child issue(s)", len(out.Issues))
	})

	// ── Remove issue from epic ───────────────────────────────────────────
	t.Run("EpicIssueRemove", func(t *testing.T) {
		out, err := callToolOn[epicissues.AssignOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_issue_remove",
			"params": map[string]any{
				"full_path":          groupPath,
				"epic_iid":           epicIID,
				"child_project_path": projOut.PathWithNamespace,
				"child_iid":          issueIID,
			},
		})
		requireNoError(t, err, "epic_issue_remove")
		requireTrue(t, out.EpicGID != "", "EpicGID should not be empty after remove")
		t.Logf("Removed issue from epic: EpicGID=%s", out.EpicGID)
	})

	// ── Verify list is empty ─────────────────────────────────────────────
	t.Run("EpicIssueList_Empty", func(t *testing.T) {
		out, err := callToolOn[epicissues.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_issue_list",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_issue_list (empty)")
		requireTrue(t, len(out.Issues) == 0, "expected 0 child issues after remove, got %d", len(out.Issues))
		t.Log("Confirmed epic has no child issues after remove")
	})

	// ── Assign + reorder (UpdateOrder) ───────────────────────────────────
	// Create a second issue and assign both to test reordering.
	issue2Out, setupErr := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id":  strconv.FormatInt(projOut.ID, 10),
			"title":       "E2E Issue 2 for Epic",
			"description": "Second test issue for reorder",
		},
	})
	requireNoError(t, setupErr, "create second issue")
	issue2IID := issue2Out.IID
	t.Logf("Created second issue IID=%d", issue2IID)

	// Assign both issues.
	_, setupErr = callToolOn[epicissues.AssignOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_issue_assign",
		"params": map[string]any{
			"full_path":          groupPath,
			"epic_iid":           epicIID,
			"child_project_path": projOut.PathWithNamespace,
			"child_iid":          issueIID,
		},
	})
	requireNoError(t, setupErr, "re-assign issue 1")

	_, setupErr = callToolOn[epicissues.AssignOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_issue_assign",
		"params": map[string]any{
			"full_path":          groupPath,
			"epic_iid":           epicIID,
			"child_project_path": projOut.PathWithNamespace,
			"child_iid":          issue2IID,
		},
	})
	requireNoError(t, setupErr, "assign issue 2")

	// List to get GIDs for reorder.
	listOut, setupErr := callToolOn[epicissues.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_issue_list",
		"params": map[string]any{
			"full_path": groupPath,
			"epic_iid":  epicIID,
		},
	})
	requireNoError(t, setupErr, "list issues for reorder")
	requireTrue(t, len(listOut.Issues) >= 2, "expected at least 2 issues for reorder, got %d", len(listOut.Issues))

	// Find the GIDs.
	var childGID, adjacentGID string
	for _, iss := range listOut.Issues {
		switch iss.IID {
		case issueIID:
			childGID = iss.ID
		case issue2IID:
			adjacentGID = iss.ID
		}
	}
	requireTrue(t, childGID != "", "could not find GID for issue IID=%d", issueIID)
	requireTrue(t, adjacentGID != "", "could not find GID for issue IID=%d", issue2IID)

	t.Run("EpicIssueUpdate_Reorder", func(t *testing.T) {
		out, err := callToolOn[epicissues.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_issue_update",
			"params": map[string]any{
				"full_path":         groupPath,
				"epic_iid":          epicIID,
				"child_id":          childGID,
				"adjacent_id":       adjacentGID,
				"relative_position": "AFTER",
			},
		})
		requireNoError(t, err, "epic_issue_update")
		requireTrue(t, len(out.Issues) >= 2, "expected at least 2 issues after reorder, got %d", len(out.Issues))
		t.Logf("Reordered issues: %d items", len(out.Issues))
	})
}

// TestMeta_EpicLinks exercises the epic_get_links action (REST-backed)
// via the gitlab_group meta-tool.
func TestMeta_EpicLinks(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a test group.
	grpName := uniqueName("grp-epiclinks")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupPath := grpOut.FullPath
	t.Logf("Created group: %s", groupPath)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": strconv.FormatInt(grpOut.ID, 10)},
		})
	}()

	epicOut, setupErr := callToolOn[epics.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "epic_create",
		"params": map[string]any{
			"full_path": groupPath,
			"title":     "E2E Epic for Links",
		},
	})
	requireNoError(t, setupErr, "create epic for links")
	epicIID := epicOut.IID
	t.Logf("Created epic IID=%d", epicIID)

	t.Run("GetLinks_Empty", func(t *testing.T) {
		out, err := callToolOn[epics.LinksOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_get_links",
			"params": map[string]any{
				"full_path": groupPath,
				"epic_iid":  epicIID,
			},
		})
		requireNoError(t, err, "epic_get_links")
		t.Logf("Got %d linked epic(s) (expected 0)", len(out.ChildEpics))
	})
}

// TestMeta_EpicBoards exercises the epic board tools (REST-backed) via
// the gitlab_group meta-tool.
func TestMeta_EpicBoards(t *testing.T) {
	t.Parallel()
	if !sess.enterprise {
		return
	}
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Create a test group.
	grpName := uniqueName("grp-epicboard")
	grpOut, setupErr := callToolOn[groups.Output](ctx, sess.meta, "gitlab_group", map[string]any{
		"action": "create",
		"params": map[string]any{
			"name":       grpName,
			"path":       grpName,
			"visibility": "private",
		},
	})
	requireNoError(t, setupErr, "create group")
	groupIDStr := strconv.FormatInt(grpOut.ID, 10)
	t.Logf("Created group: %s (ID=%s)", grpOut.FullPath, groupIDStr)

	defer func() {
		_ = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "delete",
			"params": map[string]any{"group_id": groupIDStr},
		})
	}()

	t.Run("BoardList", func(t *testing.T) {
		// Epic boards are auto-created by GitLab Premium.
		_, err := callToolOn[map[string]any](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "epic_board_list",
			"params": map[string]any{
				"group_id": groupIDStr,
			},
		})
		requirePremiumFeature(t, err, "epic boards")
		t.Log("Epic board list OK")
	})
}
