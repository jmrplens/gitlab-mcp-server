// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// tier_params_test.go pins how a call sending a param its action's tier left
// out of the served schema is answered before the SDK validates it (issue
// 1239): with the tier it needs, on the individual surface and on meta, the
// rest left where it was going.
package toolvisibility

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// freeIndex is the index a server of surface publishes over the Free catalog,
// served at a pinned Free tier.
func freeIndex(t *testing.T, surface string) *ToolActions {
	t.Helper()
	catalog, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("SharedBaseCatalog() error = %v", err)
	}
	return NewToolActions(surface, catalog).ServedAt(edition.Free, true)
}

// boardUpdateTool is the individual tool project.board_update is served as.
func boardUpdateTool(t *testing.T) string {
	t.Helper()
	catalog, err := gitlabtools.SharedBaseCatalog(false, gitlabtools.ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("SharedBaseCatalog() error = %v", err)
	}
	action, ok := catalog.Action("project.board_update")
	if !ok {
		t.Fatal("the Free catalog holds no project.board_update")
	}
	return action.IndividualTool.Name
}

// TestNarrowedCallMiddleware_TierPrunedParam_IsAnsweredWithItsTier drives the
// middleware with the Free catalog's index and checks a board update sending
// the Premium assignee_id is answered with the tier it needs on the
// individual surface and on meta, where the SDK would refuse it as an
// unexpected property or the dispatcher send it, and logged under the name
// the answer calls it by, tool/action on meta as the meta dispatcher logs the
// same refusal, and that a call sending only served params is passed on with
// the session's tier for the dispatcher.
func TestNarrowedCallMiddleware_TierPrunedParam_IsAnsweredWithItsTier(t *testing.T) {
	individual := boardUpdateTool(t)
	const sentence = `param "assignee_id" needs GitLab Premium or Ultimate, and this deployment is configured for GitLab Free, so the action's schema at this tier leaves it out and nothing was sent.`
	cases := []struct {
		name      string
		surface   string
		tool      string
		arguments string
		want      string
		logged    string
	}{
		{
			name: "individual", surface: config.ToolSurfaceIndividual, tool: individual,
			arguments: `{"project_id":"42","board_id":7,"assignee_id":5}`, want: individual + ": " + sentence, logged: individual,
		},
		{
			name: "meta", surface: config.ToolSurfaceMeta, tool: "gitlab_project",
			arguments: `{"action":"board_update","params":{"project_id":"42","board_id":7,"assignee_id":5}}`, want: "gitlab_project/board_update: " + sentence,
			logged: "gitlab_project/board_update",
		},
	}
	reached := &mcp.CallToolResult{}
	next := func(context.Context, string, mcp.Request) (mcp.Result, error) { return reached, nil }
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logged bytes.Buffer
			original := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logged, nil)))
			t.Cleanup(func() { slog.SetDefault(original) })
			index := freeIndex(t, tc.surface)
			result, err := NarrowedCallMiddleware(func() *ToolActions { return index })(next)(context.Background(), methodToolsCall, metaCall(tc.tool, tc.arguments))
			if text := answeredText(t, result, err, reached); !strings.HasPrefix(text, tc.want) {
				t.Errorf("NarrowedCallMiddleware text = %q, want it to begin %q", text, tc.want)
			}
			for _, field := range []string{`"msg":"tool call refused"`, `"tool":"` + tc.logged + `"`, `"reason":"invalid_params"`} {
				t.Run(field, func(t *testing.T) {
					if !strings.Contains(logged.String(), field) {
						t.Errorf("the log does not carry %s:\n%s", field, logged.String())
					}
				})
			}
		})
	}

	t.Run("a call sending only served params carries the session's tier on", func(t *testing.T) {
		index := freeIndex(t, config.ToolSurfaceMeta)
		var handed *mcp.CallToolResult
		dispatch := func(ctx context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
			handler := toolutil.MakeMetaHandler("gitlab_project", index.groups["gitlab_project"].routes, nil)
			handed, _, _ = handler(ctx, &mcp.CallToolRequest{}, toolutil.MetaToolInput{Action: "board_update", Params: map[string]any{"project_id": "42", "board_id": 7, "weight": 3}})
			return handed, nil
		}
		if _, err := NarrowedCallMiddleware(func() *ToolActions { return index })(dispatch)(context.Background(), methodToolsCall,
			metaCall("gitlab_project", `{"action":"board_update","params":{"project_id":"42","board_id":7}}`)); err != nil {
			t.Fatalf("NarrowedCallMiddleware error = %v", err)
		}
		if text := handed.Content[0].(*mcp.TextContent).Text; !strings.Contains(text, "this deployment is configured for GitLab Free") {
			t.Errorf("the dispatcher answered %q, want the session's tier it was handed named", text)
		}
	})
}

// TestToolActions_TierPrunedRefusal_LeavesEveryOtherCallAlone verifies the
// calls the check does not answer: a call sending only served params, one
// whose params cannot be read, a tool that runs no route of this index, an
// action its group does not route, and an index nothing told the tier.
func TestToolActions_TierPrunedRefusal_LeavesEveryOtherCallAlone(t *testing.T) {
	meta := freeIndex(t, config.ToolSurfaceMeta)
	individual := freeIndex(t, config.ToolSurfaceIndividual)
	untold := NewToolActions(config.ToolSurfaceMeta, demoCatalog(t))
	cases := []struct {
		name      string
		index     *ToolActions
		tool      string
		arguments string
	}{
		{name: "only served params", index: meta, tool: "gitlab_project", arguments: `{"action":"board_update","params":{"project_id":"42","board_id":7}}`},
		{name: "a pruned name in a value that is no object", index: meta, tool: "gitlab_project", arguments: `{"action":"board_update","params":["assignee_id"]}`},
		{name: "a pruned name the arguments cannot decode", index: individual, tool: boardUpdateTool(t), arguments: `{"assignee_id":5,}`},
		{name: "arguments that are no object", index: meta, tool: "gitlab_project", arguments: `"assignee_id"`},
		{name: "a tool this index runs no route of", index: meta, tool: "gitlab_nothing", arguments: `{"assignee_id":5}`},
		{name: "an action its group does not route", index: meta, tool: "gitlab_project", arguments: `{"action":"nope","params":{"assignee_id":5}}`},
		{name: "an index nothing told the tier", index: untold, tool: "gitlab_demo", arguments: `{"action":"allowed","params":{"assignee_id":5}}`},
		{name: "a route nothing was pruned from", index: NewToolActions(config.ToolSurfaceMeta, demoCatalog(t)).ServedAt(edition.Free, true), tool: "gitlab_demo", arguments: `{"action":"allowed","params":{"assignee_id":5}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if refusal, callName := tc.index.TierPrunedRefusal(tc.tool, json.RawMessage(tc.arguments)); refusal != nil || callName != "" {
				t.Errorf("TierPrunedRefusal() = %+v, %q; want nil and no name", refusal, callName)
			}
		})
	}
	t.Run("an object this cannot read whole names no params", func(t *testing.T) {
		for _, raw := range []string{`{"params":{"a":1},"params":{"b":2}}`, `{"params":}`, `{"params":{"a":1}`} {
			t.Run(raw, func(t *testing.T) {
				if got := paramsMember(json.RawMessage(raw)); got != nil {
					t.Errorf("paramsMember(%s) = %s, want nil", raw, got)
				}
			})
		}
		if got := string(paramsMember(json.RawMessage(`{"other":1,"params":{"a":1}}`))); got != `{"a":1}` {
			t.Errorf("paramsMember() = %s, want the params object", got)
		}
	})
	// An array whose strings and values alternate like an object's members is
	// still no object: its strings are never read as names, which a decoder
	// that only refused to start on an error would do.
	t.Run("a value that is no object has no members", func(t *testing.T) {
		if got := paramsMember(json.RawMessage(`["params",{"a":1}]`)); got != nil {
			t.Errorf("paramsMember() = %s, want nil", got)
		}
		if namesAny(json.RawMessage(`["assignee_id",5]`), map[string]string{"assignee_id": "premium"}) {
			t.Error("namesAny() read an array's string as a member name")
		}
	})
	t.Run("a nil index", func(t *testing.T) {
		var none *ToolActions
		if none.ServedAt(edition.Free, true) != nil {
			t.Error("ServedAt() on a nil index returned one")
		}
	})
}
