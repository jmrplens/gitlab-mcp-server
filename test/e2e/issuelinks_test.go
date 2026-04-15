//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issuelinks"
)

func TestIndividual_IssueLinks(t *testing.T) {
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)
	issue1 := createIssue(ctx, t, sess.individual, proj, "link-source")
	issue2 := createIssue(ctx, t, sess.individual, proj, "link-target")

	var linkID int

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuelinks.Output](ctx, sess.individual, "gitlab_issue_link_create", issuelinks.CreateInput{
			ProjectID:       proj.pidOf(),
			IssueIID:        int(issue1.IID),
			TargetProjectID: proj.pidStr(),
			TargetIssueIID:  fmt.Sprintf("%d", issue2.IID),
		})
		requireNoError(t, err, "create issue link")
		requireTrue(t, out.ID > 0, "expected link ID")
		linkID = out.ID
		t.Logf("Created issue link %d", linkID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuelinks.ListOutput](ctx, sess.individual, "gitlab_issue_link_list", issuelinks.ListInput{
			ProjectID: proj.pidOf(),
			IssueIID:  int(issue1.IID),
		})
		requireNoError(t, err, "list issue links")
		requireTrue(t, len(out.Relations) >= 1, "expected at least 1 link")
	})

	t.Run("Get", func(t *testing.T) {
		requireTrue(t, linkID > 0, "linkID not set")
		out, err := callToolOn[issuelinks.Output](ctx, sess.individual, "gitlab_issue_link_get", issuelinks.GetInput{
			ProjectID:   proj.pidOf(),
			IssueIID:    int(issue1.IID),
			IssueLinkID: linkID,
		})
		requireNoError(t, err, "get issue link")
		requireTrue(t, out.ID == linkID, "expected link ID %d", linkID)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, linkID > 0, "linkID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_issue_link_delete", issuelinks.DeleteInput{
			ProjectID:   proj.pidOf(),
			IssueIID:    int(issue1.IID),
			IssueLinkID: linkID,
		})
		requireNoError(t, err, "delete issue link")
	})
}

func TestMeta_IssueLinks(t *testing.T) {
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)
	issue1 := createIssueMeta(ctx, t, sess.meta, proj, "meta-link-source")
	issue2 := createIssueMeta(ctx, t, sess.meta, proj, "meta-link-target")

	var linkID int

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[issuelinks.Output](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "link_create",
			"params": map[string]any{
				"project_id":        proj.pidStr(),
				"issue_iid":         issue1.IID,
				"target_project_id": proj.pidStr(),
				"target_issue_iid":  fmt.Sprintf("%d", issue2.IID),
			},
		})
		requireNoError(t, err, "meta create issue link")
		requireTrue(t, out.ID > 0, "expected link ID")
		linkID = out.ID
		t.Logf("Created issue link %d via meta-tool", linkID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[issuelinks.ListOutput](ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "link_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"issue_iid":  issue1.IID,
			},
		})
		requireNoError(t, err, "meta list issue links")
		requireTrue(t, len(out.Relations) >= 1, "expected at least 1 link")
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, linkID > 0, "linkID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_issue", map[string]any{
			"action": "link_delete",
			"params": map[string]any{
				"project_id":    proj.pidStr(),
				"issue_iid":     issue1.IID,
				"issue_link_id": linkID,
			},
		})
		requireNoError(t, err, "meta delete issue link")
	})
}
