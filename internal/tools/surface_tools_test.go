// surface_tools_test.go covers the projection of standalone surface tools: the
// utility tools that are registered directly rather than through a meta-tool
// dispatcher, and what happens when one of them cannot be projected at all.
package tools

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
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
