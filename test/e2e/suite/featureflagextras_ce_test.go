//go:build e2e && !enterprise

// featureflagextras_ce_test.go tests the project feature flag read, update,
// and delete MCP tools against a live GitLab instance. Flag creation is
// already exercised elsewhere and is reused here only as fixture setup.
//
// Build tag: e2e && !enterprise.
package suite

import (
	"context"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/featureflags"
)

// TestIndividual_FeatureFlagExtras exercises the feature flag lifecycle tail
// using individual MCP tools: get → update → delete.
//
// The test creates a project fixture and a new-version feature flag, fetches
// the flag by name, updates its description and active state, deletes it, and
// verifies a follow-up get fails. A best-effort cleanup removes the flag if
// the delete subtest never ran.
//
// Build tag: e2e && !enterprise. Mode: CE. Surface: individual.
func TestIndividual_FeatureFlagExtras(t *testing.T) {
	if !sess.enterprise {
		t.Parallel()
	}
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	proj := createProject(ctx, t, sess.individual)

	const flagName = "e2e-ffx-flag"
	_, err := callToolOn[featureflags.Output](ctx, sess.individual, "gitlab_feature_flag_create", featureflags.CreateInput{
		ProjectID:   proj.pidOf(),
		Name:        flagName,
		Description: "E2E feature flag extras fixture",
		Version:     "new_version_flag",
	})
	requireNoError(t, err, "create feature flag fixture")
	t.Cleanup(func() {
		// Best-effort: the Delete subtest normally removes the flag; a 404
		// here is expected and ignored.
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelCleanup()
		_ = callToolVoidOn(cleanupCtx, sess.individual, "gitlab_feature_flag_delete", featureflags.DeleteInput{
			ProjectID: proj.pidOf(),
			Name:      flagName,
		})
	})

	t.Run("Get", func(t *testing.T) {
		out, gErr := callToolOn[featureflags.Output](ctx, sess.individual, "gitlab_feature_flag_get", featureflags.GetInput{
			ProjectID: proj.pidOf(),
			Name:      flagName,
		})
		requireNoError(t, gErr, "get feature flag")
		requireTruef(t, out.Name == flagName, "expected flag name %q, got %q", flagName, out.Name)
		t.Logf("Got feature flag %s (version=%s)", out.Name, out.Version)
	})

	t.Run("Update", func(t *testing.T) {
		const updatedDescription = "E2E feature flag extras updated"
		out, uErr := callToolOn[featureflags.Output](ctx, sess.individual, "gitlab_feature_flag_update", featureflags.UpdateInput{
			ProjectID:   proj.pidOf(),
			Name:        flagName,
			Description: updatedDescription,
			Active:      new(true),
		})
		requireNoError(t, uErr, "update feature flag")
		requireTruef(t, out.Description == updatedDescription, "expected updated description %q, got %q", updatedDescription, out.Description)
		requireTruef(t, out.Active, "expected flag to be active after update")
		t.Logf("Updated feature flag %s (active=%t)", out.Name, out.Active)
	})

	t.Run("Delete", func(t *testing.T) {
		dErr := callToolVoidOn(ctx, sess.individual, "gitlab_feature_flag_delete", featureflags.DeleteInput{
			ProjectID: proj.pidOf(),
			Name:      flagName,
		})
		requireNoError(t, dErr, "delete feature flag")

		_, gErr := callToolOn[featureflags.Output](ctx, sess.individual, "gitlab_feature_flag_get", featureflags.GetInput{
			ProjectID: proj.pidOf(),
			Name:      flagName,
		})
		requireTruef(t, gErr != nil, "expected get to fail after delete")
		t.Log("Deleted feature flag; follow-up get failed as expected")
	})
}
