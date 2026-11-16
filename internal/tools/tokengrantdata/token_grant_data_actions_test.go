// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// token_grant_data_actions_test.go holds the two token create actions that
// publish [tokengrantdata.SchemaOverrides] to the schema a full meta tool
// validates their params against.
//
// It is an external test package because users and impersonationtokens import
// this one: only a _test package may import back the other way.
package tokengrantdata_test

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/impersonationtokens"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestSchemaOverrides_EachTokenCreateAction_CountsOnlyANonEmptyGrant
// validates calls against the schema each token create action is served
// under a full meta tool: an empty array or a null for scopes or
// granular_scopes counts as not sent, as the handler reads it, so a call with
// one filled and the other empty is accepted, and one with only empty or null
// alternatives, or both filled, is refused.
func TestSchemaOverrides_EachTokenCreateAction_CountsOnlyANonEmptyGrant(t *testing.T) {
	grant := []any{map[string]any{"access": "user", "permissions": []any{"read_user"}}}
	calls := []struct {
		name      string
		grants    map[string]any
		wantValid bool
	}{
		{name: "scopes alone", grants: map[string]any{"scopes": []any{"k8s_proxy"}}, wantValid: true},
		{name: "the grant alone", grants: map[string]any{"granular_scopes": grant}, wantValid: true},
		{name: "scopes beside an empty grant", grants: map[string]any{"scopes": []any{"k8s_proxy"}, "granular_scopes": []any{}}, wantValid: true},
		{name: "the grant beside null scopes", grants: map[string]any{"scopes": nil, "granular_scopes": grant}, wantValid: true},
		{name: "both", grants: map[string]any{"scopes": []any{"k8s_proxy"}, "granular_scopes": grant}},
		{name: "empty scopes alone", grants: map[string]any{"scopes": []any{}}},
		{name: "a null grant alone", grants: map[string]any{"granular_scopes": nil}},
		{name: "neither", grants: map[string]any{}},
	}
	actions := []struct {
		tool  string
		specs []toolutil.ActionSpec
		base  map[string]any
	}{
		{tool: "gitlab_create_current_user_pat", specs: users.ActionSpecs(nil), base: map[string]any{"name": "tok"}},
		{tool: "gitlab_create_impersonation_token", specs: impersonationtokens.ActionSpecs(nil), base: map[string]any{"user_id": 1, "name": "tok"}},
	}
	for _, action := range actions {
		t.Run(action.tool, func(t *testing.T) {
			resolved := resolvedActionSchema(t, action.specs, action.tool)
			for _, call := range calls {
				t.Run(call.name, func(t *testing.T) {
					arguments := maps.Clone(action.base)
					maps.Copy(arguments, call.grants)
					if err := resolved.Validate(arguments); (err == nil) != call.wantValid {
						t.Errorf("Validate(%v) error = %v, want valid %t", arguments, err, call.wantValid)
					}
				})
			}
		})
	}
}

// resolvedActionSchema returns the resolved full meta schema of the action
// whose individual tool is named tool.
func resolvedActionSchema(t *testing.T, specs []toolutil.ActionSpec, tool string) *jsonschema.Resolved {
	t.Helper()
	for _, spec := range specs {
		if spec.IndividualTool.Name != tool {
			continue
		}
		data, err := json.Marshal(toolutil.MetaActionSchema(spec.Route))
		if err != nil {
			t.Fatalf("marshal %s schema: %v", tool, err)
		}
		var decoded jsonschema.Schema
		if err = json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("decode %s schema: %v", tool, err)
		}
		resolved, err := decoded.Resolve(nil)
		if err != nil {
			t.Fatalf("resolve %s schema: %v", tool, err)
		}
		return resolved
	}
	t.Fatalf("no action publishes the individual tool %s", tool)
	return nil
}
