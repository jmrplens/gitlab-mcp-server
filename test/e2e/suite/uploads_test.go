//go:build e2e

// uploads_test.go — E2E tests for project upload domain.
package suite

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/uploads"
)

func TestIndividual_Uploads(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	content := base64.StdEncoding.EncodeToString([]byte("E2E upload content"))

	t.Run("Upload", func(t *testing.T) {
		out, err := callToolOn[uploads.UploadOutput](ctx, sess.individual, "gitlab_project_upload", uploads.UploadInput{
			ProjectID:     proj.pidOf(),
			Filename:      "e2e-test.txt",
			ContentBase64: content,
		})
		requireNoError(t, err, "upload file")
		requireTrue(t, out.URL != "", "expected non-empty upload URL")
		requireTrue(t, out.Markdown != "", "expected non-empty markdown")
		t.Logf("Uploaded: %s", out.URL)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[uploads.ListOutput](ctx, sess.individual, "gitlab_project_upload_list", uploads.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list uploads")
		requireTrue(t, len(out.Uploads) >= 1, "expected >=1 upload, got %d", len(out.Uploads))
	})
}

func TestMeta_Uploads(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	content := base64.StdEncoding.EncodeToString([]byte("E2E meta upload content"))

	t.Run("Upload", func(t *testing.T) {
		out, err := callToolOn[uploads.UploadOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"filename":       "e2e-meta-test.txt",
				"content_base64": content,
			},
		})
		requireNoError(t, err, "upload file meta")
		requireTrue(t, out.URL != "", "expected non-empty upload URL")
		t.Logf("Uploaded (meta): %s", out.URL)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[uploads.ListOutput](ctx, sess.meta, "gitlab_project", map[string]any{
			"action": "upload_list",
			"params": map[string]any{
				"project_id": proj.pidStr(),
			},
		})
		requireNoError(t, err, "list uploads meta")
		requireTrue(t, len(out.Uploads) >= 1, "expected >=1 upload, got %d", len(out.Uploads))
	})
}
