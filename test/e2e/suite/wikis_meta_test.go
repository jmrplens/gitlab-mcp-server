//go:build e2e

// wikis_meta_test.go tests the wiki attachment upload action via the
// gitlab_wiki meta-tool against a live GitLab instance, complementing
// the CRUD coverage in wikis_test.go.
package suite

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/wikis"
)

// TestMeta_WikiUploadAttachment exercises the upload_attachment action not covered by wikis_test.go.
func TestMeta_WikiUploadAttachment(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	proj := createProjectMeta(ctx, t, sess.meta)

	// Create a wiki page first (required for uploads)
	_, pageErr := callToolOn[wikis.Output](ctx, sess.meta, "gitlab_wiki", map[string]any{
		"action": "create",
		"params": map[string]any{
			"project_id": proj.pidStr(),
			"title":      "Upload Test Page",
			"content":    "Page for attachment test",
		},
	})
	requireNoError(t, pageErr, "create wiki page")

	t.Run("UploadAttachment", func(t *testing.T) {
		content := base64.StdEncoding.EncodeToString([]byte("E2E test file content"))
		out, err := callToolOn[wikis.AttachmentOutput](ctx, sess.meta, "gitlab_wiki", map[string]any{
			"action": "upload_attachment",
			"params": map[string]any{
				"project_id":     proj.pidStr(),
				"filename":       "test-upload.txt",
				"content_base64": content,
			},
		})
		requireNoError(t, err, "upload_attachment")
		requireTrue(t, out.FileName != "", "upload_attachment: expected filename in output")
		t.Logf("Uploaded attachment: %s (path=%s)", out.FileName, out.FilePath)
	})
}
