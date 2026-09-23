// surface_tools_test.go covers the projection of standalone surface tools: the
// utility tools that are registered directly rather than through a meta-tool
// dispatcher, and what happens when one of them cannot be projected at all.
package tools

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// TestRegisterSurfaceTools_UnprojectableSpec_PanicsNamingTheTool verifies the
// failure mode a malformed surface tool spec has to have.
//
// These tools are part of the declared MCP surface, so a spec that cannot be
// projected is not a tool that quietly goes missing: it stops startup, and the
// panic names the tool so the spec can be found. Every real spec is compiled
// in, which is why the only way to reach this is a spec written here.
func TestRegisterSurfaceTools_UnprojectableSpec_PanicsNamingTheTool(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("RegisterSurfaceTools accepted a spec that projects to nothing, so a declared tool would go missing silently")
		}
		err, ok := recovered.(error)
		if !ok {
			t.Fatalf("panic value = %v (%T), want an error", recovered, recovered)
		}
		if !strings.Contains(err.Error(), "gitlab_broken_surface_tool") {
			t.Errorf("panic = %q, want it to name the tool whose spec failed", err)
		}
	}()

	// A name and nothing else: the spec carries no description, no route and no
	// owner, so validation refuses it at the first field it reads.
	RegisterSurfaceTools(server, []actioncatalog.SurfaceToolSpec{{Name: "gitlab_broken_surface_tool"}})
}

// refusalActionID matches a dotted token that could be offered as an action
// ID. The refusal a guided flow gives spells no other: the one tool name in it
// has no dot, and each of its sentences ends in a dot followed by a space or a
// line break.
var refusalActionID = regexp.MustCompile(`\b[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*\b`)

// TestStandaloneSurfaceToolSpecs_GuidedFlowRefusal_NamesAnActionTheCatalogServes
// verifies that every guided flow, refused on a client that cannot elicit,
// offers in its place exactly one action this catalog serves, on the Free
// catalog every flow is served on.
//
// The alternative is spelled as a catalog ID inside elicitationtools, which
// imports the four domain packages but never sees the IDs they are aggregated
// under, since the domain half is added here. No source gate reads the
// sentence either: it is rendered through toolutil.ErrorResult, which is not
// one of the error helpers cmd/audit_action_ids reads, and the e2e scenario
// quotes only the issue flow's. This is where all four meet the catalog. The
// flows are driven with no request on the context, which is a client without
// elicitation, so each ends at its first prompt.
func TestStandaloneSurfaceToolSpecs_GuidedFlowRefusal_NamesAnActionTheCatalogServes(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{})
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	args := map[string]map[string]any{
		"gitlab_interactive_issue_create":   {"project_id": "42"},
		"gitlab_interactive_mr_create":      {"project_id": "42"},
		"gitlab_interactive_release_create": {"project_id": "42"},
		"gitlab_interactive_project_create": {},
	}

	flows := 0
	for _, spec := range StandaloneSurfaceToolSpecs(client) {
		if spec.GroupToolName != "gitlab_interactive" {
			continue
		}
		flows++
		t.Run(spec.Name, func(t *testing.T) {
			params, known := args[spec.Name]
			if !known {
				t.Fatalf("guided flow %s has no arguments in this table, so its refusal is held to nothing: add it", spec.Name)
			}
			out, err := spec.Route.Handler(context.Background(), params)
			if err != nil {
				t.Fatalf("%s: a client without elicitation got an error instead of the refusal: %v", spec.Name, err)
			}
			result := spec.FormatResult(out)
			if result == nil || !result.IsError {
				t.Fatalf("%s: result = %+v, want the refusal", spec.Name, result)
			}
			text := extractTextContent(result)
			ids := refusalActionID.FindAllString(text, -1)
			if len(ids) != 1 {
				t.Fatalf("%s: refusal offers %d action IDs %v, want exactly one:\n%s", spec.Name, len(ids), ids, text)
			}
			if _, served := catalog.Action(actioncatalog.ActionID(ids[0])); !served {
				t.Errorf("%s: refusal offers %s, which the Free catalog does not serve:\n%s", spec.Name, ids[0], text)
			}
		})
	}
	if flows != len(args) {
		t.Errorf("found %d guided flows, want the %d this table names", flows, len(args))
	}
}
