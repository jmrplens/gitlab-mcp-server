//go:build e2e

// integrations_test.go covers the group-level Datadog integration, the one
// group integration with actions of its own: set it with a placeholder key,
// read it back, delete it, and show that a set naming no field is refused
// before GitLab is asked.
//
// The key is a placeholder of the shape GitLab validates and nothing ever
// sends to Datadog: the integration is created inactive, since GitLab
// cannot verify the key, and it is deleted in the same test. The endpoint is
// licensed although the catalog lists the actions as Free, which is why the
// scenario lives here rather than in the common package.

package ee

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/integrations"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// placeholderDatadogKey has the 32-hex shape GitLab accepts for an API key.
// It is not a credential and reaches no Datadog.
const placeholderDatadogKey = "0123456789abcdef0123456789abcdef"

// TestGroupDatadogIntegration_Lifecycle_SetsReadsAndDeletes walks the
// integration of a fresh group through set, get and delete on every
// surface, and asks for a set with nothing to set.
//
// Replaces: TestGroupDatadogIntegration
func TestGroupDatadogIntegration_Lifecycle_SetsReadsAndDeletes(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("datadog"))
		params := map[string]any{"group_id": group.IDParam()}

		refused := harness.ExpectToolError(s, actionGroupDatadogSet, params, "at least one of")
		e.T.Logf("a set naming no field is refused before any request: %s", firstLine(refused))

		set := harness.Do[integrations.SetGroupDatadogOutput](s, actionGroupDatadogSet, withParams(params, map[string]any{"api_key": placeholderDatadogKey}))
		e.T.Logf("set answered %+v", set)

		got := harness.Do[integrations.GetGroupDatadogOutput](s, actionGroupDatadogGet, params)
		if got.Integration.ID == 0 || !got.Integration.Active {
			e.T.Errorf("get answered %+v after the set, want the active integration record", got.Integration)
		}

		harness.DoVoid(s, actionGroupDatadogDelete, params)
		// A deleted group integration is disabled rather than removed: the
		// licensed run found the read answering the record with active off,
		// so that is what is asserted, and a not-found is accepted as the
		// other way GitLab can say the integration is gone.
		after, err := harness.Try[integrations.GetGroupDatadogOutput](s, actionGroupDatadogGet, params)
		switch {
		case err != nil && strings.Contains(strings.ToLower(err.Error()), "not found"):
			e.T.Logf("the read after the delete is refused: %s", firstLine(err.Error()))
		case err != nil:
			e.T.Errorf("the read after the delete failed for another reason than not found: %v", err)
		case after.Integration.Active:
			e.T.Errorf("the integration is still active after its delete: %+v", after.Integration)
		}
	})
}
