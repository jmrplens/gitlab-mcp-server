//go:build e2e

// runnercontrollers_test.go covers the runner controller family, an
// administrator's experimental Ultimate API: the controller itself, its
// tokens, and its scopes at the instance and at the runner level.
//
// The runner scopes name the instance runner the Docker stack registers,
// which is the one shared thing here: two tests scoping it at once would
// contend for it, so the test declares the runner lock and the runner need.

package ee

import (
	"context"
	"net/http"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runnercontrollers"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runnercontrollerscopes"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/runnercontrollertokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The controller states the lifecycle moves through.
const (
	controllerStateDisabled = "disabled"
	controllerStateDryRun   = "dry_run"
)

// controllerTokenIDs lists the IDs of a token listing.
func controllerTokenIDs(out runnercontrollertokens.ListOutput) []int64 {
	ids := make([]int64, 0, len(out.Tokens))
	for _, token := range out.Tokens {
		ids = append(ids, token.ID)
	}
	return ids
}

// scopedRunnerIDs lists the runners a controller is scoped to.
func scopedRunnerIDs(out runnercontrollerscopes.ScopesOutput) []int64 {
	ids := make([]int64, 0, len(out.RunnerLevelScopings))
	for _, scoping := range out.RunnerLevelScopings {
		ids = append(ids, scoping.RunnerID)
	}
	return ids
}

// TestRunnerControllers_Lifecycle_TokensScopesAndDeletion creates one
// controller per surface, updates it, walks its token through create,
// list, get, rotate and revoke, walks its scopes at the instance level and
// against the Docker runner, and deletes it. The listing and the read of a
// controller that does not exist are asked alongside.
//
// Replaces: TestMeta_RunnerControllerLifecycle, TestEE_MetaRunnerManagement
func TestRunnerControllers_Lifecycle_TokensScopesAndDeletion(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin, harness.NeedRunner, harness.Tier(edition.Ultimate)), harness.Locks(harness.LockRunner))
	runnerID := fixture.DockerRunnerID(e)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)

		listed := harness.Do[runnercontrollers.ListOutput](s, actionRunnerControllerList, nil)
		e.T.Logf("%d runner controller(s) before the create", len(listed.Controllers))
		refused := harness.Refused(s, actionRunnerControllerGet, map[string]any{"controller_id": missingID}, harness.FailureNotFound)
		e.T.Logf("the read of a missing controller is refused: %s", firstLine(refused))

		created := harness.Do[runnercontrollers.Output](s, actionRunnerControllerCreate, map[string]any{
			"description": e.Name("controller"), "state": controllerStateDisabled,
		})
		if created.ID == 0 {
			e.T.Fatalf("controller_create answered %+v, want a controller with an ID", created)
		}
		e.Defer("runner controller", func(ctx context.Context) error {
			_, err := e.Client().GL().RunnerControllers.DeleteRunnerController(created.ID, gl.WithContext(ctx))
			if err != nil && !fixture.IsStatus(err, http.StatusNotFound) {
				return err
			}
			return nil
		})
		controller := map[string]any{"controller_id": created.ID}

		updated := harness.Do[runnercontrollers.Output](s, actionRunnerControllerUpdate, withParams(controller, map[string]any{
			"description": e.Name("controller-updated"), "state": controllerStateDryRun,
		}))
		if updated.ID != created.ID || updated.State != controllerStateDryRun {
			e.T.Errorf("controller_update answered %+v, want controller %d in state %s", updated, created.ID, controllerStateDryRun)
		}

		walkControllerTokens(e, s, controller)
		walkControllerScopes(e, s, controller, created.ID, runnerID)

		harness.DoVoid(s, actionRunnerControllerDelete, controller)
		gone := harness.Refused(s, actionRunnerControllerGet, controller, harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(gone))
	})
}

// walkControllerTokens takes one token of the controller through create,
// list, get, rotate and revoke.
func walkControllerTokens(e *harness.Env, s *harness.Session, controller map[string]any) {
	e.T.Helper()

	token := harness.Do[runnercontrollertokens.Output](s, actionRunnerControllerTokenCreate, withParams(controller, map[string]any{"description": "e2e controller token"}))
	if token.ID == 0 {
		e.T.Fatalf("controller_token_create answered %+v, want a token with an ID", token)
	}
	tokens := harness.Do[runnercontrollertokens.ListOutput](s, actionRunnerControllerTokenList, controller)
	if !containsID(controllerTokenIDs(tokens), token.ID) {
		e.T.Errorf("the token listing does not hold the created token %d: %v", token.ID, controllerTokenIDs(tokens))
	}
	got := harness.Do[runnercontrollertokens.Output](s, actionRunnerControllerTokenGet, withParams(controller, map[string]any{"token_id": token.ID}))
	if got.ID != token.ID {
		e.T.Errorf("controller_token_get answered token %d, want %d", got.ID, token.ID)
	}
	rotated := harness.Do[runnercontrollertokens.Output](s, actionRunnerControllerTokenRotate, withParams(controller, map[string]any{"token_id": token.ID}))
	if rotated.ID == 0 {
		e.T.Fatalf("controller_token_rotate answered %+v, want a token with an ID", rotated)
	}
	// A rotation may mint a new record, so the revoke names the current one.
	harness.DoVoid(s, actionRunnerControllerTokenRevoke, withParams(controller, map[string]any{"token_id": rotated.ID}))
	afterRevoke := harness.Do[runnercontrollertokens.ListOutput](s, actionRunnerControllerTokenList, controller)
	if containsID(controllerTokenIDs(afterRevoke), rotated.ID) {
		e.T.Errorf("the token listing still holds token %d after its revoke: %v", rotated.ID, controllerTokenIDs(afterRevoke))
	}
}

// walkControllerScopes adds and removes the instance scope and the Docker
// runner's scope, reading the listing back after each change so the effect
// is observed and not only the answer.
func walkControllerScopes(e *harness.Env, s *harness.Session, controller map[string]any, controllerID, runnerID int64) {
	e.T.Helper()

	scopes := harness.Do[runnercontrollerscopes.ScopesOutput](s, actionRunnerControllerScopeList, controller)
	e.T.Logf("controller %d scopes before: %d instance, %d runner", controllerID, len(scopes.InstanceLevelScopings), len(scopes.RunnerLevelScopings))
	harness.DoVoid(s, actionRunnerControllerScopeAddInstance, controller)
	scopes = harness.Do[runnercontrollerscopes.ScopesOutput](s, actionRunnerControllerScopeList, controller)
	if len(scopes.InstanceLevelScopings) == 0 {
		e.T.Errorf("controller %d reports no instance-level scoping right after adding one", controllerID)
	}
	harness.DoVoid(s, actionRunnerControllerScopeRemoveInstance, controller)
	scopes = harness.Do[runnercontrollerscopes.ScopesOutput](s, actionRunnerControllerScopeList, controller)
	if len(scopes.InstanceLevelScopings) != 0 {
		e.T.Errorf("controller %d still reports %d instance-level scoping(s) after removing them", controllerID, len(scopes.InstanceLevelScopings))
	}

	scoped := harness.Do[runnercontrollerscopes.RunnerScopeOutput](s, actionRunnerControllerScopeAddRunner, withParams(controller, map[string]any{"runner_id": runnerID}))
	if scoped.RunnerID != runnerID {
		e.T.Errorf("controller_scope_add_runner answered runner %d, want %d", scoped.RunnerID, runnerID)
	}
	harness.DoVoid(s, actionRunnerControllerScopeRemoveRunner, withParams(controller, map[string]any{"runner_id": runnerID}))
	scopes = harness.Do[runnercontrollerscopes.ScopesOutput](s, actionRunnerControllerScopeList, controller)
	if containsID(scopedRunnerIDs(scopes), runnerID) {
		e.T.Errorf("controller %d is still scoped to runner %d after removing it", controllerID, runnerID)
	}
}
