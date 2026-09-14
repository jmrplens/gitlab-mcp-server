//go:build e2e

// groupvariables_test.go covers a group CI variable through its life on
// every surface: created, listed, read, changed, and deleted, with the
// read afterwards refused.

package common

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// groupVariableKey is the key every group of this file gets a variable
// under; the group is the surface's own, so the key needs no scoping.
const groupVariableKey = "E2E_GROUP_VARIABLE"

// groupVariableKeys lists the keys of a variable listing.
func groupVariableKeys(listed []groupvariables.Output) []string {
	keys := make([]string, 0, len(listed))
	for _, variable := range listed {
		keys = append(keys, variable.Key)
	}
	return keys
}

// TestGroupVariables_Lifecycle_CreateListGetUpdateDelete creates a variable
// in a group of each surface's own, finds it in the listing, reads it,
// changes its value, deletes it and shows the read afterwards refused.
//
// Replaces: TestMeta_GroupVariables
func TestGroupVariables_Lifecycle_CreateListGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("variables"))
		params := map[string]any{"group_id": group.IDParam()}
		variable := withParams(params, map[string]any{"key": groupVariableKey})

		created := harness.Do[groupvariables.Output](s, actionGroupVariableCreate, withParams(variable, map[string]any{"value": "first"}))
		if created.Key != groupVariableKey || created.Value != "first" {
			e.T.Fatalf("group_create answered %+v, want the variable %s set to first", created, groupVariableKey)
		}
		listed := harness.Do[groupvariables.ListOutput](s, actionGroupVariableList, params)
		if !containsKey(groupVariableKeys(listed.Variables), groupVariableKey) {
			e.T.Errorf("the group lists the variables %v, want %s among them", groupVariableKeys(listed.Variables), groupVariableKey)
		}
		got := harness.Do[groupvariables.Output](s, actionGroupVariableGet, variable)
		if got.Key != groupVariableKey || got.Value != "first" {
			e.T.Errorf("group_get answered %+v, want %s set to first", got, groupVariableKey)
		}

		updated := harness.Do[groupvariables.Output](s, actionGroupVariableUpdate, withParams(variable, map[string]any{"value": "second"}))
		if updated.Key != groupVariableKey || updated.Value != "second" {
			e.T.Errorf("group_update answered %+v, want %s set to second", updated, groupVariableKey)
		}

		harness.DoVoid(s, actionGroupVariableDelete, variable)
		refused := harness.Refused(s, actionGroupVariableGet, variable, harness.FailureNotFound)
		e.T.Logf("the read of the deleted variable is refused: %s", firstLine(refused))
	})
}
