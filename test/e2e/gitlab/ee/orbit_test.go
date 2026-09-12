//go:build e2e

// orbit_test.go pins the one licensed thing a self-managed instance must not
// serve: the Orbit knowledge graph group, which exists on GitLab.com alone.
// It is named by prefix rather than by a typed action ID on purpose, since
// the actions are absent from the catalog a self-managed instance builds,
// and a constant naming one would be refused by the static gate as not a
// catalog action, which is the very fact under test.

package ee

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The two spellings the Orbit group has: the tool names on the meta and
// individual surfaces, and the action IDs on the dynamic one.
const (
	orbitToolPrefix   = "gitlab_orbit"
	orbitActionPrefix = "orbit."
)

// TestOrbit_SelfManaged_ServesNoOrbitToolOrAction lists what every surface
// serves and checks that nothing of Orbit is among the tools or the
// actions on a self-managed instance, whatever its tier.
//
// Replaces: TestEnterpriseOrbit_NotRegisteredOnSelfManaged
func TestOrbit_SelfManaged_ServesNoOrbitToolOrAction(t *testing.T) {
	e := harness.New(t)
	if e.Client().IsGitLabDotCom() {
		e.Skipf("Orbit is served on GitLab.com, and this scenario asserts its absence elsewhere")
	}

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		for _, tool := range s.Tools() {
			if strings.HasPrefix(tool, orbitToolPrefix) {
				e.T.Errorf("the %s session serves the tool %s on a self-managed instance", surface, tool)
			}
		}
		for _, action := range s.Actions() {
			if strings.HasPrefix(string(action), orbitActionPrefix) {
				e.T.Errorf("the %s session serves the action %s on a self-managed instance", surface, action)
			}
		}
	})
}
