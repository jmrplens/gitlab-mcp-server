//go:build e2e

// featureflagextras_test.go covers the tail of a project feature flag's
// life on every surface: read, changed in description and state, deleted,
// and refused on the read afterwards. The create that makes the flag runs
// through the server too, since that is the one way to make one.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/featureflags"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The flag's name, unique within the project each surface gets, and the
// version every new flag is created as.
const (
	featureFlagName    = "e2e-extras-flag"
	featureFlagVersion = "new_version_flag"
)

// TestFeatureFlags_GetUpdateDelete_ReadBack creates a flag in a project of
// each surface's own, reads it, turns it on with a new description, deletes
// it and shows the read afterwards refused.
//
// Replaces: TestIndividual_FeatureFlagExtras
func TestFeatureFlags_GetUpdateDelete_ReadBack(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		project := fixture.NewProject(e, fixture.WithNamePrefix("flags"))
		flag := map[string]any{"project_id": project.IDParam(), "name": featureFlagName}

		created := harness.Do[featureflags.Output](s, actionFeatureFlagCreate, withParams(flag, map[string]any{
			"description": "created by the e2e suite", "version": featureFlagVersion,
		}))
		if created.Name != featureFlagName || created.Version != featureFlagVersion {
			e.T.Fatalf("feature_flag_create answered %+v, want the flag %q as a %s", created, featureFlagName, featureFlagVersion)
		}

		got := harness.Do[featureflags.Output](s, actionFeatureFlagGet, flag)
		if got.Name != featureFlagName || got.Version != featureFlagVersion {
			e.T.Errorf("feature_flag_get answered %+v, want the flag %q as a %s", got, featureFlagName, featureFlagVersion)
		}
		updated := harness.Do[featureflags.Output](s, actionFeatureFlagUpdate, withParams(flag, map[string]any{
			"description": "updated by the e2e suite", "active": true,
		}))
		if updated.Description != "updated by the e2e suite" || !updated.Active {
			e.T.Errorf("feature_flag_update answered %+v, want the flag active with the new description", updated)
		}

		harness.DoVoid(s, actionFeatureFlagDelete, flag)
		refused := harness.Refused(s, actionFeatureFlagGet, flag, harness.FailureNotFound)
		e.T.Logf("the read of the deleted flag is refused: %s", firstLine(refused))
	})
}
