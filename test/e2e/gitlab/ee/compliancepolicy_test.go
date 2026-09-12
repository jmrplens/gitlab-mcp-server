//go:build e2e

// compliancepolicy_test.go covers the instance's compliance policy
// settings: the read, and the two refusals the update makes, one before
// GitLab is asked and one from GitLab. Nothing here changes the setting,
// which is instance-wide and would be visible to every other test.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/compliancepolicy"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestCompliancePolicy_Settings_ReadAndTheUpdateRefusals reads the
// settings on every surface, then asks for an update with no namespace,
// which the handler refuses before any request, and one naming a namespace
// that does not exist, which GitLab refuses.
//
// Replaces: TestMeta_CompliancePolicy
func TestCompliancePolicy_Settings_ReadAndTheUpdateRefusals(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.Tier(edition.Ultimate)))

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		settings := harness.Do[compliancepolicy.Output](s, actionCompliancePolicyGet, nil)
		if settings.CSPNamespaceID == nil {
			e.T.Log("no compliance policy namespace is set")
		} else {
			e.T.Logf("the compliance policy namespace is %d", *settings.CSPNamespaceID)
		}

		refused := harness.ExpectToolError(s, actionCompliancePolicyUpdate, nil, "csp_namespace_id")
		assertMentions(e, "the update with no namespace", refused, "required")

		refused = harness.ExpectToolError(s, actionCompliancePolicyUpdate, map[string]any{"csp_namespace_id": missingID}, "csp_namespace_id")
		assertMentions(e, "the update naming a missing namespace", refused, "top-level group", "lock")
	})
}
