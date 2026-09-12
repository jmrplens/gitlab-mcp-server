//go:build e2e

// license_test.go covers the license API on an instance that has one, which
// is the twin of the ce package's test and the first scenario of this
// package: it needs nothing but the license the package requires, and it is
// what a run pointed at the wrong runtime is refused on. Beside it is the
// one test that changes the license, by adding and removing a copy of it.

package ee

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/license"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestLicense_AddAndDeleteADuplicate_LeavesTheInstanceAsFound installs a
// second copy of the license the instance already runs on and removes that
// copy again, on every surface, which is the only state-preserving way to
// drive license_add and license_delete: the active license never changes,
// since GitLab keeps the newest one active and the copy is deleted before
// anything else runs.
//
// It holds the license lock and runs alone in its package, because a
// license that comes and goes underneath every other session would change
// what those sessions are served; and it re-probes the tier afterwards, so
// a copy that failed to delete is reported here rather than as a scattering
// of catalog mismatches in later tests. It runs only against the disposable
// Docker instance, whose provisioning script is what cached the key it
// reads.
//
// Replaces: TestMeta_AdminLicenseLifecycle
func TestLicense_AddAndDeleteADuplicate_LeavesTheInstanceAsFound(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceLicense), harness.Serial())
	if !e.DockerMode() {
		e.Skipf("license mutations are only exercised against the ephemeral Docker instance")
	}
	key := fixture.CachedEnterpriseLicense(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		before := harness.Do[license.GetOutput](s, actionLicenseGet, nil)
		if before.License.ID == 0 {
			e.T.Fatalf("license_get answered %+v before the add, want the installed license", before.License)
		}

		added := harness.Do[license.AddOutput](s, actionLicenseAdd, map[string]any{"license": key})
		if added.License.ID == 0 || added.License.ID == before.License.ID {
			e.T.Fatalf("license_add answered %+v, want a new record beside %d", added.License, before.License.ID)
		}
		// The copy is removed whatever happens below, so a failed assertion
		// cannot leave the instance with two licenses; a copy already gone is
		// not an error.
		e.Defer("duplicate license", func(ctx context.Context) error {
			_, err := e.Client().GL().License.DeleteLicense(added.License.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})

		harness.DoVoid(s, actionLicenseDelete, map[string]any{"id": added.License.ID})

		after := harness.Do[license.GetOutput](s, actionLicenseGet, nil)
		if after.License.ID != before.License.ID {
			e.T.Errorf("the active license is %d after the cycle, want %d as found", after.License.ID, before.License.ID)
		}
		if now, was := e.ReprobeTier(); now != was {
			e.T.Errorf("the instance probes as %s after the cycle, and was %s before it", now, was)
		}
	})
}

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
