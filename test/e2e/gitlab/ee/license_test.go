//go:build e2e

// license_test.go covers the license API on an instance that has one, which
// is the twin of the ce package's test and the first scenario of this
// package: it needs nothing but the license the package requires, and it is
// what a run pointed at the wrong runtime is refused on.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionLicenseGet reads the installed license through the admin group.
const actionLicenseGet harness.ActionID = "admin.license_get"

// TestLicense_InstalledLicenseMatchesTier runs admin.license_get on every
// surface and holds the answer to what the harness's probe resolved from the
// same endpoint: the plan maps to the tier the run is on, and the license is
// the current one, since an expired license would leave the instance Free
// and this package refused.
func TestLicense_InstalledLicenseMatchesTier(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))
	rt := e.Runtime()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		out := harness.Do[license.GetOutput](s, actionLicenseGet, nil)

		if out.License.ID == 0 {
			e.T.Errorf("the license answer carries no ID: %+v", out.License)
		}
		if got := edition.TierFromPlan(out.License.Plan); got != rt.Tier {
			e.T.Errorf("the license plan %q maps to %s, and the probe resolved %s from the same instance", out.License.Plan, got, rt.Tier)
		}
		if out.License.Expired {
			e.T.Errorf("the license %d is expired, which would leave the instance unlicensed", out.License.ID)
		}
	})
}
