// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// tierParamsInput is an input whose fields ask for each tier, one of them a
// field a compatibility alias resolves to.
type tierParamsInput struct {
	ProjectID string `json:"project_id"`
	Name      string `json:"name,omitempty"`
	Assignee  int64  `json:"assignee_id,omitempty" tier:"premium"`
	Health    string `json:"health_status,omitempty" tier:"Ultimate"`
}

// tierParamsRoute is a route over tierParamsInput as a Free catalog prunes it.
func tierParamsRoute(ran *int) ActionRoute {
	route := RouteAction(nil, func(_ context.Context, _ *gitlabclient.Client, _ tierParamsInput) (testOutput, error) {
		*ran++
		return testOutput{Result: "ran"}, nil
	})
	route.TierPruned = TierPrunedFields(reflect.TypeFor[tierParamsInput](), edition.Free)
	return route
}

// TestTierPrunedFields_NamesWhatAskForAHigherTier verifies the fields the
// catalog prunes at a tier, each with the tier it asks for, and none at a
// tier that serves them all.
func TestTierPrunedFields_NamesWhatAskForAHigherTier(t *testing.T) {
	rt := reflect.TypeFor[tierParamsInput]()
	cases := []struct {
		tier edition.Tier
		want map[string]string
	}{
		{tier: edition.Free, want: map[string]string{"assignee_id": "premium", "health_status": "ultimate"}},
		{tier: edition.Premium, want: map[string]string{"health_status": "ultimate"}},
		{tier: edition.Ultimate, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.tier.String(), func(t *testing.T) {
			if got := TierPrunedFields(rt, tc.tier); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("TierPrunedFields(%s) = %v, want %v", tc.tier, got, tc.want)
			}
		})
	}
}

// TestTierPrunedParams_ReadsTheCallAfterItsAliases verifies which params of
// a call a route refuses for their tier: the pruned ones, sorted, after the
// compatibility aliases are resolved and the meta keys stripped, and none
// for a call sending none, for no params, and for a route nothing was pruned
// from.
func TestTierPrunedParams_ReadsTheCallAfterItsAliases(t *testing.T) {
	ran := 0
	route := tierParamsRoute(&ran)
	cases := []struct {
		name   string
		route  ActionRoute
		params map[string]any
		want   []string
	}{
		{name: "two pruned", route: route, params: map[string]any{"project_id": "1", "health_status": "x", "assignee_id": 5}, want: []string{"assignee_id", "health_status"}},
		{name: "an alias of a served param", route: route, params: map[string]any{"project_path": "1", "name": "x"}},
		{name: "the confirmation key", route: route, params: map[string]any{"project_id": "1", "confirm": true}},
		{name: "no params", route: route},
		{name: "a route nothing was pruned from", route: ActionRoute{}, params: map[string]any{"assignee_id": 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TierPrunedParams(tc.route, tc.params); !slices.Equal(got, tc.want) {
				t.Errorf("TierPrunedParams() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTierPrunedParamsRefusal_WordsEachParamAndTheSessionTier pins the answer
// a tier-pruned param gets: what each needs, the tier the session was built
// for, pinned or detected, the way out, and, when nothing told the caller the
// session's tier, a clause that does not name it.
func TestTierPrunedParamsRefusal_WordsEachParamAndTheSessionTier(t *testing.T) {
	ran := 0
	route := tierParamsRoute(&ran)
	cases := []struct {
		name   string
		pruned []string
		tier   edition.Tier
		pinned bool
		known  bool
		want   string
	}{
		{
			name: "one param, a detected tier", pruned: []string{"assignee_id"}, tier: edition.Free, known: true,
			want: `demo/update: param "assignee_id" needs GitLab Premium or Ultimate, and this instance's tier was detected as GitLab Free, so the action's schema at this tier leaves it out and nothing was sent. If the instance is licensed for it, the detection was wrong: ask the operator to pin the tier with GITLAB_MCP_TIER or --tier. Send the call without it to run the action at this tier.`,
		},
		{
			name: "two params, a pinned Premium tier", pruned: []string{"assignee_id", "health_status"}, tier: edition.Premium, pinned: true, known: true,
			want: `demo/update: param "assignee_id" needs GitLab Premium or Ultimate and param "health_status" needs GitLab Ultimate, and this deployment is configured for GitLab Premium, so the action's schema at this tier leaves them out and nothing was sent. If the instance is licensed for it, ask the operator to configure the tier it has with GITLAB_MCP_TIER or --tier. Send the call without them to run the action at this tier.`,
		},
		{
			name: "a tier nothing told", pruned: []string{"health_status"},
			want: `demo/update: param "health_status" needs GitLab Ultimate, and this session's tier does not serve it, so the action's schema at this tier leaves it out and nothing was sent. If the instance is licensed for it, ask the operator to configure the tier it has with GITLAB_MCP_TIER or --tier. Send the call without it to run the action at this tier.`,
		},
		{
			name: "two params and a tier nothing told", pruned: []string{"assignee_id", "health_status"},
			want: `this session's tier does not serve them, so the action's schema at this tier leaves them out and nothing was sent. If the instance is licensed for them, ask the operator`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := TierPrunedParamsRefusal("demo/update", route, tc.pruned, tc.tier, tc.pinned, tc.known)
			text := result.Content[0].(*mcp.TextContent).Text
			if !result.IsError || !strings.Contains(text, tc.want) {
				t.Errorf("TierPrunedParamsRefusal() = %q (error %t), want it to contain %q", text, result.IsError, tc.want)
			}
		})
	}
	t.Run("no param names no refusal", func(t *testing.T) {
		if refusal := TierPrunedParamsRefusal("demo/update", route, nil, edition.Free, true, true); refusal != nil {
			t.Errorf("TierPrunedParamsRefusal(nil) = %+v, want nil", refusal)
		}
	})
	t.Run("an Ultimate session names Ultimate", func(t *testing.T) {
		if got := SessionTierClause(edition.Ultimate, true); got != "this deployment is configured for GitLab Ultimate" {
			t.Errorf("SessionTierClause(Ultimate) = %q", got)
		}
	})
}

// TestMetaTool_TierPrunedParam_IsRefusedInEveryModeAndNeverRuns verifies the
// meta dispatcher refuses a param its action's tier left out with the tier,
// before the handler runs, in every parameter schema mode: in opaque mode,
// where nothing else holds the params, and in compact and full, where the
// action's definition would refuse it as unknown first. The session's tier is
// absent, as it is for a handler no call middleware stands in front of, so
// the answer does not name it.
func TestMetaTool_TierPrunedParam_IsRefusedInEveryModeAndNeverRuns(t *testing.T) {
	for _, mode := range []string{MetaParamSchemaOpaque, MetaParamSchemaCompact, MetaParamSchemaFull} {
		t.Run(mode, func(t *testing.T) {
			ran := 0
			text, refused := callMetaToolRoutes(t, mode, ActionMap{"update": tierParamsRoute(&ran)},
				map[string]any{"action": "update", "params": map[string]any{"project_id": "1", "assignee_id": 5}})
			if !refused || !strings.HasPrefix(text, `meta_modes/update: param "assignee_id" needs GitLab Premium or Ultimate, and this session's tier does not serve it`) || ran != 0 {
				t.Errorf("call = %q (refused %t, ran %d), want the tier refusal and no run", text, refused, ran)
			}
		})
	}
	t.Run("a served call runs", func(t *testing.T) {
		ran := 0
		if text, refused := callMetaToolRoutes(t, MetaParamSchemaCompact, ActionMap{"update": tierParamsRoute(&ran)},
			map[string]any{"action": "update", "params": map[string]any{"project_id": "1"}}); refused || ran != 1 {
			t.Errorf("call = %q (refused %t, ran %d), want it run", text, refused, ran)
		}
	})
}

// TestMakeMetaHandler_TierPrunedParam_NamesTheSessionTierItWasHanded verifies
// the dispatcher words the refusal with the tier the call middleware handed
// over ([WithSessionTier]).
func TestMakeMetaHandler_TierPrunedParam_NamesTheSessionTierItWasHanded(t *testing.T) {
	ran := 0
	handler := MakeMetaHandler("demo", ActionMap{"update": tierParamsRoute(&ran)}, nil)
	ctx := WithSessionTier(context.Background(), edition.Free, false)
	result, _, err := handler(ctx, &mcp.CallToolRequest{}, MetaToolInput{Action: "update", Params: map[string]any{"project_id": "1", "assignee_id": 5}})
	if err != nil || !result.IsError || ran != 0 {
		t.Fatalf("handler = %+v, %v (ran %d), want a refusal and no run", result, err, ran)
	}
	if text := result.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "this instance's tier was detected as GitLab Free") {
		t.Errorf("handler text = %q, want the detected session tier named", text)
	}
}
