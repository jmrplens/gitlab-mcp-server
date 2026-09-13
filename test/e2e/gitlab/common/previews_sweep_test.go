//go:build e2e

// previews_sweep_test.go asks safe mode to preview every served mutating
// action whose required parameters bind from the World, on all three surfaces.
//
// Safe mode intercepts a mutating operation and answers with a preview of the
// call it would have made, without running the handler, so the World is never
// written even though the sweep names creates, updates and deletes. A
// destructive action is sent confirmed, so the confirmation guard is not what
// stops it and the preview is what a call an operator approved would get. An
// action whose required parameters the World cannot supply is named rather
// than previewed: a preview only fires once the dispatcher's parameter check
// passes, so an unbindable action would be refused for the wrong reason.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestPreviews_Sweep previews every mutating action whose required parameters
// bind from the World, on the dynamic, meta and individual surfaces.
//
// It widens what TestModes_Safe replaced: the old suite held its safe-mode
// coverage to two hand-picked actions in TestSafeMode and
// TestSafeModeDynamicSurface, and this drives every mutating one.
func TestPreviews_Sweep(t *testing.T) {
	e := harness.New(t)
	world := fixture.SharedWorld(e)

	candidates, unbound := sweepCandidates(e, world, false)
	if len(candidates) == 0 {
		t.Fatal("no mutating action bound from the World; the previews sweep would prove nothing")
	}
	for _, u := range unbound {
		t.Logf("mutating %s not previewed: %s", u.id, u.reason)
	}

	sweepSurfaces(t, harness.ModeSafe, func(e *harness.Env, s *harness.Session) {
		previewed, notServed := 0, 0
		for _, c := range candidates {
			if !s.Serves(c.id) {
				notServed++
				continue
			}
			// Refused with FailureSafeMode fails the test if the answer is
			// anything but a preview, which is the assertion: safe mode must
			// preview every mutating action it serves, not run it and not
			// refuse it for another reason.
			harness.Refused(s, c.id, c.params, harness.FailureSafeMode, harness.For(harness.PurposeSweep))
			previewed++
		}
		e.T.Logf("previewed %d mutating actions on %s (%d not served here)", previewed, s.Surface(), notServed)
	})
}
