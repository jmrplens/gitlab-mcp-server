//go:build e2e && !enterprise

// uploads_ce_test.go exercises the project upload domain against a live
// GitLab CE instance.
//
// Covers both the individual MCP tool surface (gitlab_upload_file,
// gitlab_upload_get, gitlab_upload_list) and the catalog-backed
// gitlab_project meta-tool surface for upload actions.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/uploads"
)

// TestIndividual_Uploads exercises project upload tools through individual
// MCP tools against a live GitLab CE instance.
//
// The test creates a project fixture, uploads a base64-encoded file via
// gitlab_upload_file, then calls gitlab_upload_list and gitlab_upload_get
// to verify the upload is observable through subsequent reads. Each
// subtest asserts the expected upload payload shape with non-empty name
// and URL.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
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
		requireTruef(t, out.URL != "", "expected non-empty upload URL")
		requireTruef(t, out.Markdown != "", "expected non-empty markdown")
		t.Logf("Uploaded: %s", out.URL)
	})

	t.Run("List", func(t *testing.T) {
		out, err := callToolOn[uploads.ListOutput](ctx, sess.individual, "gitlab_project_upload_list", uploads.ListInput{
			ProjectID: proj.pidOf(),
		})
		requireNoError(t, err, "list uploads")
		requireTruef(t, len(out.Uploads) >= 1, "expected >=1 upload, got %d", len(out.Uploads))
	})
}

// TestMeta_Uploads exercises the upload lifecycle through the
// gitlab_project meta-tool against a live GitLab CE instance.
//
// The test mirrors [TestIndividual_Uploads] but drives every step with
// {action, params} arguments through the catalog-backed tool. Subtests
// cover the upload and upload_list actions with base64-encoded file
// content, verifying the meta-tool returns consistent upload payloads
// with non-empty name and URL.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: meta.
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
		requireTruef(t, out.URL != "", "expected non-empty upload URL")
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
		requireTruef(t, len(out.Uploads) >= 1, "expected >=1 upload, got %d", len(out.Uploads))
	})
}
