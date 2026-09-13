//go:build e2e

// civariables_test.go covers CI/CD variables through the ci_variable group:
// a project's own, and a group's, which the same group of actions reaches
// under its group_ prefix.

package common

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/civariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupvariables"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// variableKeyFor spells a variable key GitLab accepts for one surface, since
// a key may hold letters, digits and underscores and nothing else.
func variableKeyFor(prefix string, surface harness.Surface) string {
	return prefix + "_" + strings.ToUpper(string(surface))
}

// projectVariableKeys returns the keys of a project variable listing.
func projectVariableKeys(listed []civariables.Output) []string {
	keys := make([]string, 0, len(listed))
	for _, variable := range listed {
		keys = append(keys, variable.Key)
	}
	return keys
}

// The optional arguments of the project variable actions, spelled with the
// values their handlers treat as absent.
//
// They are spelled rather than omitted because the individual surface
// validates a call against the served schema, and these fields are marked
// required there: their json tags carry no omitempty, which is what the SDK
// derives requiredness from. The call means the same on every surface with
// them spelled this way, and a model that omits them is refused, which is
// the finding this batch reports rather than fixes.
var (
	variableCreateDefaults = map[string]any{
		"description": "", "protected": false, "masked": false,
		"masked_and_hidden": false, "raw": false, "environment_scope": "",
	}
	variableUpdateDefaults = map[string]any{
		"description": "", "protected": false, "masked": false,
		"raw": false, "environment_scope": "",
	}
	variableScopeDefault = map[string]any{"environment_scope": ""}
)

// TestCIVariable_ProjectLifecycle_CreateGetListUpdateDelete creates a
// variable per surface on a shared project, reads it back, finds it in the
// listing, changes its value, deletes it and asserts the read afterwards is
// refused as not found.
//
// Replaces: TestIndividual_CIVariables, TestMeta_CIVariables
func TestCIVariable_ProjectLifecycle_CreateGetListUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Project {
		return fixture.NewProject(e, fixture.WithNamePrefix("civars"))
	}, func(e *harness.Env, surface harness.Surface, project fixture.Project) {
		s := e.On(surface)
		key := variableKeyFor("E2E_VAR", surface)
		params := map[string]any{"project_id": project.IDParam(), "key": key}

		created := harness.Do[civariables.Output](s, actionCIVariableCreate, withParams(withParams(params, variableCreateDefaults), map[string]any{"value": "hello-e2e"}))
		if created.Key != key || created.Value != "hello-e2e" {
			e.T.Fatalf("variable create answered %+v, want %s=hello-e2e", created, key)
		}
		got := harness.Do[civariables.Output](s, actionCIVariableGet, withParams(params, variableScopeDefault))
		if got.Key != key || got.Value != "hello-e2e" {
			e.T.Errorf("variable get answered %+v, want %s=hello-e2e", got, key)
		}
		listed := harness.Do[civariables.ListOutput](s, actionCIVariableList, map[string]any{"project_id": project.IDParam()})
		if !slices.Contains(projectVariableKeys(listed.Variables), key) {
			e.T.Errorf("the variable listing does not hold %s: %v", key, projectVariableKeys(listed.Variables))
		}
		updated := harness.Do[civariables.Output](s, actionCIVariableUpdate, withParams(withParams(params, variableUpdateDefaults), map[string]any{"value": "updated-value"}))
		if updated.Key != key || updated.Value != "updated-value" {
			e.T.Errorf("variable update answered %+v, want %s=updated-value", updated, key)
		}

		harness.DoVoid(s, actionCIVariableDelete, withParams(params, variableScopeDefault))
		refused := harness.Refused(s, actionCIVariableGet, withParams(params, variableScopeDefault), harness.FailureNotFound)
		e.T.Logf("the read after the delete is refused: %s", firstLine(refused))
	})
}

// TestCIVariable_GroupLifecycle_ListCreateGetUpdateDelete does the same on
// a shared group through the group_ actions of the ci_variable group: the
// listing before and after, a create, a read, an update and a delete.
//
// Replaces: TestMeta_CIVariablesGroup
func TestCIVariable_GroupLifecycle_ListCreateGetUpdateDelete(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("civars"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		key := variableKeyFor("E2E_GRP", surface)
		scope := map[string]any{"group_id": group.IDParam()}
		params := withParams(scope, map[string]any{"key": key})

		before := harness.Do[groupvariables.ListOutput](s, actionCIVariableGroupList, scope)
		if slices.Contains(groupVariableKeys(before.Variables), key) {
			e.T.Fatalf("the group already holds %s before it was created: %v", key, groupVariableKeys(before.Variables))
		}

		created := harness.Do[groupvariables.Output](s, actionCIVariableGroupCreate, withParams(params, map[string]any{"value": "test-value"}))
		if created.Key != key || created.Value != "test-value" {
			e.T.Fatalf("group variable create answered %+v, want %s=test-value", created, key)
		}
		got := harness.Do[groupvariables.Output](s, actionCIVariableGroupGet, params)
		if got.Key != key || got.Value != "test-value" {
			e.T.Errorf("group variable get answered %+v, want %s=test-value", got, key)
		}
		updated := harness.Do[groupvariables.Output](s, actionCIVariableGroupUpdate, withParams(params, map[string]any{"value": "updated-value"}))
		if updated.Key != key || updated.Value != "updated-value" {
			e.T.Errorf("group variable update answered %+v, want %s=updated-value", updated, key)
		}

		harness.DoVoid(s, actionCIVariableGroupDelete, params)
		after := harness.Do[groupvariables.ListOutput](s, actionCIVariableGroupList, scope)
		if slices.Contains(groupVariableKeys(after.Variables), key) {
			e.T.Errorf("the group still holds %s after its delete: %v", key, groupVariableKeys(after.Variables))
		}
	})
}
