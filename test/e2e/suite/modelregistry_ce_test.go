//go:build e2e && !enterprise

// modelregistry_ce_test.go tests the GitLab Model Registry MCP tools via the
// gitlab_model_registry meta-tool against a live GitLab instance. Covers
// download with invalid IDs (graceful error path). No ML model data exists
// in Docker CE so the list action returns empty and download gets 404.
//
// Model Registry requires GitLab Premium/Ultimate; the server enforces this
// with 403. The tool itself is always registered but the data path is
// gated by the license tier.
package suite

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/modelregistry"
)

// TestMeta_ModelRegistry exercises gitlab_model_registry download with
// invalid IDs. The action routes successfully even when the target instance
// has no model data, confirming the meta-tool router handles the domain.
func TestMeta_ModelRegistry(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	RunWithCapabilities(t, []Capability{}, func(_ *E2EContext) {
		ctx := context.Background()

		t.Run("DownloadInvalidID_GracefulError", func(t *testing.T) {
			// Use a clearly non-existent model version ID. 404 is expected
			// from the GitLab API; the tool surfaces it as an error.
			out, err := callToolOn[modelregistry.DownloadOutput](ctx, sess.meta, "gitlab_model_registry", map[string]any{
				"action": "download",
				"params": map[string]any{
					"project_id":       "999999",
					"model_version_id": "999999",
					"path":             "/",
					"filename":         "model.bin",
				},
			})
			// The download of a non-existent model must fail. 404 (not found)
			// or 403 (forbidden on CE) are the expected outcomes. Success
			// here would indicate a broken error path that needs fixing.
			if err == nil {
				t.Fatalf("download of invalid model returned no error: out=%+v; expected 404 or 403", out)
			}
			if !isHTTPStatus(err, 404) && !isHTTPStatus(err, 403) {
				t.Fatalf("download error did not match expected 404/403: %v", err)
			}
			t.Logf("download error (expected 404/403): %v", err)
		})
	})
}
