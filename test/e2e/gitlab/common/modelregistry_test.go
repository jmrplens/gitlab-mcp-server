//go:build e2e

// modelregistry_test.go covers the one model registry action the suite can
// drive without a model: the download of a model version file from a
// project that does not exist, which is refused as not found on every
// surface. The Docker instance holds no model, and the tool is what routes
// the refusal and names what to verify.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The project and the model version the download names, neither of which
// a fresh instance has.
const (
	missingModelProjectID = "999999"
	missingModelVersionID = "999999"
)

// TestModelRegistry_Download_MissingProjectRefused asks every surface for a
// model version file of a project that does not exist, and reads the
// refusal, which names the parameters to verify.
//
// Replaces: TestMeta_ModelRegistry
func TestModelRegistry_Download_MissingProjectRefused(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		refused := harness.Refused(s, actionModelRegistryDownload, map[string]any{
			"project_id": missingModelProjectID, "model_version_id": missingModelVersionID, "path": "/", "filename": "model.bin",
		}, harness.FailureNotFound)
		assertMentions(e, "the download of a model version from a project that does not exist", refused, "model_version_id")
	})
}
