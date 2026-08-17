//go:build e2e && enterprise

// admin_license_ee_test.go exercises the instance license mutation actions
// (license_add, license_delete) that the CE suite can never run: the
// endpoints only exist on EE runtimes. The lifecycle is state-preserving by
// construction — it adds a duplicate of the license that is already
// installed and then deletes that duplicate — so the active plan never
// changes while it runs and the cached-license workflow that provisions the
// ephemeral EE instance is untouched. As a safety interlock it only runs
// against the disposable Docker instance.
//
// Build tag: e2e && enterprise.
package suite

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/license"
)

// adminLicenseCachedKey returns the Base64 license string the provisioning
// scripts cached for this run, or skips: E2E_ENTERPRISE_LICENSE_FILE
// overrides the default cache location (paths are tried as given and
// relative to the suite's working directory, which is test/e2e/suite).
func adminLicenseCachedKey(t *testing.T) string {
	t.Helper()
	candidates := []string{"../.enterprise-license"}
	if override := os.Getenv("E2E_ENTERPRISE_LICENSE_FILE"); override != "" {
		candidates = []string{override, "../../../" + override}
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if key := strings.TrimSpace(string(data)); key != "" {
			return key
		}
	}
	t.Skipf("no cached Enterprise license found (tried %v); provision one via setup-gitlab.sh", candidates)
	return ""
}

// TestMeta_AdminLicenseLifecycle adds a duplicate of the installed license
// and deletes that duplicate, covering license_add and license_delete while
// leaving the instance exactly as found (license_get verifies the active
// license ID before and after).
func TestMeta_AdminLicenseLifecycle(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}
	if !isDockerMode() {
		t.Skip("license mutations are only exercised against the ephemeral Docker instance")
	}
	key := adminLicenseCachedKey(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	before, err := callToolOn[license.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
		"action": "license_get",
	})
	requireNoError(t, err, "license_get before lifecycle")
	requireTruef(t, before.License.ID > 0, "expected an installed license, got %+v", before.License)

	added, err := callToolOn[license.AddOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
		"action": "license_add",
		"params": map[string]any{"license": key},
	})
	requireNoError(t, err, "license_add duplicate")
	requireTruef(t, added.License.ID > 0 && added.License.ID != before.License.ID,
		"expected a new license record, got %+v (before %d)", added.License, before.License.ID)
	t.Logf("Added duplicate license %d (plan %s)", added.License.ID, added.License.Plan)

	err = callToolVoidOn(ctx, sess.meta, "gitlab_admin", map[string]any{
		"action": "license_delete",
		"params": map[string]any{"id": added.License.ID},
	})
	requireNoError(t, err, "license_delete duplicate")

	after, err := callToolOn[license.GetOutput](ctx, sess.meta, "gitlab_admin", map[string]any{
		"action": "license_get",
	})
	requireNoError(t, err, "license_get after lifecycle")
	requireTruef(t, after.License.ID == before.License.ID,
		"active license changed: before %d, after %d", before.License.ID, after.License.ID)
	t.Logf("Active license %d unchanged after add/delete cycle", after.License.ID)
}
