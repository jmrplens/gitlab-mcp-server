//go:build e2e

// instance_variables_test.go ports the instance-scoped CI variable CRUD the
// old suite drove through the gitlab_ci_variable meta tool: create a variable,
// list it, read it, update its value, delete it, and read it again to see the
// delete took.
//
// Instance CI variables are instance-global keyed state, so the scenario runs
// through the meta tool on the meta surface the old suite used, under the
// instance-global lock, with a single well-known key.

package common

import (
	"context"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/instancevariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestCIVariables_Instance_Lifecycle creates, lists, reads, updates and
// deletes an instance CI variable, then reads the deleted key to confirm it is
// gone.
//
// Replaces: TestMeta_CIVariablesInstance
func TestCIVariables_Instance_Lifecycle(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceMeta)

	const key = "E2E_INSTANCE_VAR"
	// A run that failed before its own delete leaves this key behind, so the
	// next run starts by removing it best-effort.
	e.Defer("instance variable "+key, func(context.Context) error {
		if _, err := harness.Try[instancevariables.Output](s, actionCIVariableInstanceDelete, map[string]any{"key": key}, harness.For(harness.PurposeCleanup)); err != nil {
			e.T.Logf("best-effort cleanup of instance variable %s answered: %v", key, err)
		}
		return nil
	})

	created := harness.Do[instancevariables.Output](s, actionCIVariableInstanceCreate, map[string]any{"key": key, "value": "instance_test_value"})
	if created.Key != key {
		e.T.Fatalf("instance_create answered key %q, want %q", created.Key, key)
	}

	list := harness.Do[instancevariables.ListOutput](s, actionCIVariableInstanceList, nil)
	if len(list.Variables) == 0 {
		e.T.Errorf("instance_list answered no variables after the create")
	}

	got := harness.Do[instancevariables.Output](s, actionCIVariableInstanceGet, map[string]any{"key": key})
	if got.Key != key {
		e.T.Errorf("instance_get answered key %q, want %q", got.Key, key)
	}

	updated := harness.Do[instancevariables.Output](s, actionCIVariableInstanceUpdate, map[string]any{"key": key, "value": "instance_updated_value"})
	if updated.Key != key {
		e.T.Errorf("instance_update answered key %q, want %q", updated.Key, key)
	}

	harness.DoVoid(s, actionCIVariableInstanceDelete, map[string]any{"key": key})
	refused := harness.Refused(s, actionCIVariableInstanceGet, map[string]any{"key": key}, harness.FailureNotFound)
	e.T.Logf("the deleted instance variable reads as gone: %s", firstLine(refused))
}
