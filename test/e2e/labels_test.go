//go:build e2e

package e2e

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/labels"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestIndividual_Labels exercises label CRUD using individual MCP tools.
func TestIndividual_Labels(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	var labelID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[labels.Output](ctx, sess.individual, "gitlab_label_create", labels.CreateInput{
			ProjectID: proj.pidOf(),
			Name:      "e2e-label",
			Color:     "#428BCA",
		})
		requireNoError(t, err, "label create")
		requireTrue(t, out.ID > 0, "label ID should be positive")
		labelID = out.ID
		t.Logf("Created label: %s (ID=%d, color=%s)", out.Name, out.ID, out.Color)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[labels.ListOutput](ctx, sess.individual, "gitlab_label_list", labels.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "label list")
		requireTrue(t, len(out.Labels) >= 1, "expected at least 1 label, got %d", len(out.Labels))
		t.Logf("Listed %d labels", len(out.Labels))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, labelID > 0, "labelID not set")
		out, err := callToolOn[labels.Output](ctx, sess.individual, "gitlab_label_update", labels.UpdateInput{
			ProjectID:   proj.pidOf(),
			LabelID:     toolutil.StringOrInt(strconv.FormatInt(labelID, 10)),
			Description: "Updated by E2E",
		})
		requireNoError(t, err, "label update")
		requireTrue(t, out.Description == "Updated by E2E", "expected updated description")
		t.Logf("Updated label: %s", out.Name)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, labelID > 0, "labelID not set")
		err := callToolVoidOn(ctx, sess.individual, "gitlab_label_delete", labels.DeleteInput{
			ProjectID: proj.pidOf(),
			LabelID:   toolutil.StringOrInt(strconv.FormatInt(labelID, 10)),
		})
		requireNoError(t, err, "label delete")
		t.Log("Deleted label")
	})
}

// TestMeta_Labels exercises label CRUD using the gitlab_project meta-tool.
func TestMeta_Labels(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	var labelID int64

	t.Run("Create", func(t *testing.T) {
		out, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_create",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"name":       "e2e-meta-label",
				"color":      "#FF0000",
			},
		})
		requireNoError(t, err, "meta label create")
		requireTrue(t, out.ID > 0, "expected positive label ID")
		labelID = out.ID
		t.Logf("Created label: %s (ID=%d)", out.Name, out.ID)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[labels.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "meta label list")
		requireTrue(t, len(out.Labels) >= 1, "expected at least 1 label")
		t.Logf("Listed %d labels via meta-tool", len(out.Labels))
	})

	t.Run("Update", func(t *testing.T) {
		requireTrue(t, labelID > 0, "labelID not set")
		out, err := callToolOn[labels.Output](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_update",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelID,
				"color":      "#00FF00",
			},
		})
		requireNoError(t, err, "meta label update")
		requireTrue(t, out.ID == labelID, "label ID mismatch after update")
		t.Logf("Updated label: %s (color=%s)", out.Name, out.Color)
	})

	t.Run("Delete", func(t *testing.T) {
		requireTrue(t, labelID > 0, "labelID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "label_delete",
			"params": map[string]any{
				"project_id": proj.pidStr(),
				"label_id":   labelID,
			},
		})
		requireNoError(t, err, "meta label delete")
		t.Logf("Deleted label ID=%d", labelID)
	})
}
