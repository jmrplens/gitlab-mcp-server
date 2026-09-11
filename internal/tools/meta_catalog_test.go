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

// TestRegisterMetaCatalog_NilInputs verifies nil server or catalog inputs are
// ignored without panicking.
//
// Meta catalog registration is called from configurable startup paths; accepting
// nil inputs keeps defensive tests and partial setup flows from crashing. The
// two are passed separately as well as together, because the guard reads them
// left to right: with a nil server the catalog is never looked at, so a pair of
// nils says nothing about what a missing catalog alone does.
func TestRegisterMetaCatalog_NilInputs(t *testing.T) {
	RegisterMetaCatalog(nil, nil)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, &mcp.ServerOptions{SchemaCache: testSchemaCache})
	RegisterMetaCatalog(server, nil)
	if names := toolNamesFromServer(t, server); len(names) != 0 {
		t.Errorf("registered tools = %v, want none from a missing catalog", names)
	}
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
