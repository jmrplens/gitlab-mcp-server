package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestRegisterMetaCatalog_NilInputs verifies that each of the three nil
// combinations registers nothing and does not panic.
//
// Meta catalog registration is called from configurable startup paths, so a
// partial setup must not take the process down. Nothing inside the function
// answers for that: there is no guard in front of its loop, because either nil
// is already refused one call down and an early return here could therefore
// not be observed. That refusal is the whole contract, and this is the only
// thing holding it.
//
// The third combination is the one that earns its place, and it is the one
// this test lacked. A nil server with a real catalog is the only shape that
// walks the loop at all, so it is the only one that would notice if
// AddMetaTool ever stopped refusing a nil server; the other two never reach
// the loop body, because Groups() answers nil for a nil catalog.
func TestRegisterMetaCatalog_NilInputs(t *testing.T) {
	RegisterMetaCatalog(nil, nil)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, nil)
	if names := toolNamesFromServer(t, server); len(names) != 0 {
		t.Errorf("registered tools = %v, want none from a missing catalog", names)
	}

	// A catalog carrying a group, so the loop body runs rather than being
	// skipped by an empty group list: a nil server has to be refused by
	// AddMetaTool for this to return at all.
	catalog := actioncatalog.NewCatalog()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_nil_server_probe"})
	group.SetAction(actioncatalog.Action{
		Name:  "get",
		Route: toolutil.Route(func(context.Context, map[string]any) (any, error) { return map[string]any{}, nil }),
	})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}
	RegisterMetaCatalog(nil, catalog)
}

// TestRegisterMetaCatalog_GroupFormatterRendersTheResult pins that a group's
// own formatter is what its registered dispatcher renders results with, and
// that the shared Markdown dispatcher only fills in for a group that declares
// none.
//
// Neither direction fails loudly. A group whose formatter is dropped still
// answers, in the shared dispatcher's rendering rather than its own; a group
// left with no formatter at all still answers too, because MakeMetaHandler
// falls back to a JSON one of its own, which is not the Markdown every other
// tool of this server serves.
func TestRegisterMetaCatalog_GroupFormatterRendersTheResult(t *testing.T) {
	const sentinel = "rendered by the group's own formatter"

	spec := toolutil.NewActionSpec("list", toolutil.RouteAction(nil,
		func(_ context.Context, _ *gitlabclient.Client, _ struct{}) (struct{}, error) {
			return struct{}{}, nil
		}), toolutil.ActionSpecOptions{
		ReadOnly:     true,
		OwnerPackage: "tools",
	})
	group, err := actioncatalog.GroupFromSpecs(actioncatalog.GroupOptions{
		ToolName:     "gitlab_test_formatter",
		Title:        "Formatter probe",
		Description:  "Formatter probe group.",
		OwnerPackage: "tools",
		SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		ReadOnly:     true,
		FormatResult: func(any) *mcp.CallToolResult {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: sentinel}}}
		},
	}, []toolutil.ActionSpec{spec})
	if err != nil {
		t.Fatalf("GroupFromSpecs() error = %v", err)
	}
	catalog := actioncatalog.NewCatalog()
	if addErr := catalog.AddGroup(group); addErr != nil {
		t.Fatalf("AddGroup() error = %v", addErr)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, catalog)

	session := connectServerForTools(t, server)
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_test_formatter",
		Arguments: map[string]any{"action": "list"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() IsError = true: %#v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("the dispatcher returned no content at all")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, sentinel) {
		t.Errorf("content = %q, want the group's declared formatter to have rendered it", text.Text)
	}
}
