//go:build e2e

// issues_test.go tests the core issue CRUD MCP tools against a live GitLab
// instance. Covers create, get, list, update, note create/list, and delete
// for both individual and meta-tool modes.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuenotes"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
)

// TestIndividual_Issues exercises the issue lifecycle using individual MCP tools.
func TestIndividual_Issues(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var issueIID int64
	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issues.Output](ctx, sess.individual, "gitlab_issue_create", issues.CreateInput{
			ProjectID:   proj.pidOf(),
			Title:       "E2E issue lifecycle test",
			Description: "Issue created by self-contained E2E test.",
		})
		requireNoError(t, err, "create issue")
		requireTrue(t, out.IID > 0, "issue IID should be positive, got %d", out.IID)
		requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)
		issueIID = out.IID
		t.Logf("Created issue #%d: %s", out.IID, out.Title)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.individual, "gitlab_issue_get", issues.GetInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issueIID,
		})
		requireNoError(t, err, "get issue")
		requireTrue(t, out.IID == issueIID, "expected issue IID %d, got %d", issueIID, out.IID)
		requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)
		t.Logf("Got issue #%d: %s", out.IID, out.Title)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issues.ListOutput](ctx, sess.individual, "gitlab_issue_list", issues.ListInput{
			ProjectID: proj.pidOf(),
			State:     "opened",
		})
		requireNoError(t, err, "list issues")
		requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue, got %d", len(out.Issues))

		found := false
		for _, i := range out.Issues {
			if i.IID == issueIID {
				found = true
				break
			}
		}
		requireTrue(t, found, "issue #%d not found in list", issueIID)
		t.Logf("Listed %d open issues", len(out.Issues))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.individual, "gitlab_issue_update", issues.UpdateInput{
			ProjectID:   proj.pidOf(),
			IssueIID:    issueIID,
			Title:       "E2E issue — updated title",
			Description: "Updated description via E2E test.",
		})
		requireNoError(t, err, "update issue")
		requireTrue(t, out.IID == issueIID, "expected issue IID %d, got %d", issueIID, out.IID)
		requireTrue(t, out.Title == "E2E issue — updated title", "expected updated title, got %q", out.Title)
		t.Logf("Updated issue #%d", out.IID)
	})

	t.Run("NoteCreate", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issuenotes.Output](ctx, sess.individual, "gitlab_issue_note_create", issuenotes.CreateInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issueIID,
			Body:      "**E2E Bot**: Automated comment on issue.",
		})
		requireNoError(t, err, "create issue note")
		requireTrue(t, out.ID > 0, "issue note ID should be positive, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created note ID=%d on issue #%d", out.ID, issueIID)
	})

	t.Run("NoteList", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issuenotes.ListOutput](ctx, sess.individual, "gitlab_issue_note_list", issuenotes.ListInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issueIID,
		})
		requireNoError(t, err, "list issue notes")
		requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note, got %d", len(out.Notes))

		found := false
		for _, n := range out.Notes {
			if n.ID == noteID {
				found = true
				break
			}
		}
		requireTrue(t, found, "note ID=%d not found in list", noteID)
		t.Logf("Listed %d notes on issue #%d", len(out.Notes), issueIID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_issue_delete", issues.DeleteInput{
			ProjectID: proj.pidOf(),
			IssueIID:  issueIID,
		})
		requireNoError(t, err, "delete issue")
		t.Logf("Deleted issue #%d", issueIID)
	})
}

// TestMeta_Issues exercises the issue lifecycle using the gitlab_issue meta-tool.
func TestMeta_Issues(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var issueIID int64
	var noteID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "create",
			"params": map[string]any{
				"project_id":  proj.pidStr(),
				"title":       "Meta E2E issue lifecycle test",
				"description": "Issue created via meta-tool E2E.",
			},
		})
		requireNoError(t, err, "meta issue create")
		requireTrue(t, out.IID > 0, "issue IID should be positive, got %d", out.IID)
		requireTrue(t, out.State == "opened", "expected state 'opened', got %q", out.State)
		issueIID = out.IID
		t.Logf("Created issue #%d via meta-tool", out.IID)
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "get",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireNoError(t, err, "meta issue get")
		requireTrue(t, out.IID == issueIID, "expected issue IID %d, got %d", issueIID, out.IID)
		t.Logf("Got issue #%d via meta-tool", out.IID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issues.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"state":      "opened",
			},
		})
		requireNoError(t, err, "meta issue list")
		requireTrue(t, len(out.Issues) >= 1, "expected at least 1 issue")

		found := false
		for _, i := range out.Issues {
			if i.IID == issueIID {
				found = true
				break
			}
		}
		requireTrue(t, found, "issue #%d not in meta list", issueIID)
		t.Logf("Listed %d issues via meta-tool", len(out.Issues))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issues.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"title":      "Meta E2E issue — updated title",
			},
		})
		requireNoError(t, err, "meta issue update")
		requireTrue(t, out.IID == issueIID, "expected issue IID %d, got %d", issueIID, out.IID)
		t.Logf("Updated issue #%d via meta-tool", out.IID)
	})

	t.Run("NoteCreate", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issuenotes.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
				"body":       "**Meta E2E Bot**: Automated comment via meta-tool.",
			},
		})
		requireNoError(t, err, "meta issue note create")
		requireTrue(t, out.ID > 0, "note ID should be positive, got %d", out.ID)
		noteID = out.ID
		t.Logf("Created note %d on issue #%d via meta-tool", out.ID, issueIID)
	})

	t.Run("NoteList", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		out, err := callToolOn[issuenotes.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "note_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireNoError(t, err, "meta issue note list")
		requireTrue(t, len(out.Notes) >= 1, "expected at least 1 note")

		found := false
		for _, n := range out.Notes {
			if n.ID == noteID {
				found = true
				break
			}
		}
		requireTrue(t, found, "note %d not found in list", noteID)
		t.Logf("Listed %d notes via meta-tool", len(out.Notes))
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, issueIID > 0, "issueIID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issueIID,
			},
		})
		requireNoError(t, err, "meta issue delete")
		t.Logf("Deleted issue #%d via meta-tool", issueIID)
	})
}
