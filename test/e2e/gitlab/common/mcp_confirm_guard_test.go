//go:build e2e

// mcp_confirm_guard_test.go covers the confirmation guard a destructive action
// is stopped by when the client cannot prompt for approval.
//
// The harness client advertises no elicitation capability by default, which is
// exactly the client the guard exists for: without a way to ask, the server
// fails closed on a destructive call and tells the caller to re-send with
// confirm=true, and only an explicit confirm=true runs it. It is asserted on
// the individual and meta surfaces, which reach the guard through
// toolutil.ConfirmDestructiveAction; the dynamic surface's own guard is
// covered by the dynamic tests.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestConfirmGuard_DestructiveActionFailsClosed checks that a destructive
// action from a client that cannot prompt is refused for want of confirmation,
// leaves the object intact, and runs only when confirm=true is sent.
//
// Replaces: TestDestructiveConfirmGuard_NoElicitationClient
func TestConfirmGuard_DestructiveActionFailsClosed(t *testing.T) {
	e := harness.New(t)

	harness.OnSurfaces(e, "the dynamic surface's confirmation guard is covered by TestDynamic_FindExecuteReadWorkflow",
		[]harness.Surface{harness.SurfaceIndividual, harness.SurfaceMeta},
		func(e *harness.Env, surface harness.Surface) {
			s := e.On(surface)
			project := fixture.NewProject(e, fixture.WithNamePrefix("confirm"))

			t.Run("BlockedWithoutConfirm", func(t *testing.T) {
				refused := harness.Refused(s, actionProjectDelete, map[string]any{"project_id": project.IDParam()},
					harness.FailureNeedsConfirmation, harness.WithoutConfirmation())
				if !strings.Contains(strings.ToLower(refused), "confirm=true") {
					t.Errorf("the guard refusal does not tell the caller to re-send with confirm=true: %s", firstLine(refused))
				}
			})

			t.Run("StillExists", func(t *testing.T) {
				got := harness.Do[projects.Output](s, actionProjectGet, map[string]any{"project_id": project.IDParam()})
				if got.ID != project.ID {
					t.Errorf("project.get after the blocked delete answered %d, want %d", got.ID, project.ID)
				}
			})

			t.Run("ConfirmExecutes", func(t *testing.T) {
				harness.DoVoid(s, actionProjectDelete, map[string]any{"project_id": project.IDParam()})
			})
		})
}
