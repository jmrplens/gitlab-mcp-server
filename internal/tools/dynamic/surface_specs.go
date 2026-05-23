package dynamic

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ControllerSurfaceSpecs returns explicit surface metadata for Dynamic
// controller tools. Registration stays custom because these tools expose
// controller-specific Markdown results and, for execute, action-dependent output.
func ControllerSurfaceSpecs(registry *Registry) []actioncatalog.SurfaceToolSpec {
	if registry == nil {
		registry = NewRegistry(nil)
	}
	return []actioncatalog.SurfaceToolSpec{
		dynamicControllerSpec(findToolName, "GitLab Find Action", findToolDescription, "find", toolutil.RouteRequestFunc(func(ctx context.Context, req *mcp.CallToolRequest, input FindInput) (FindOutput, error) {
			_, out, err := registry.Find(ctx, req, input)
			return out, err
		}), toolutil.IconSearch, true),
		dynamicControllerSpec(executeActionToolName, "GitLab Execute Action", executeActionToolDescription, "execute", dynamicExecuteRoute(registry), toolutil.IconServer, false),
	}
}

func dynamicControllerSpec(name, title, description, actionName string, route toolutil.ActionRoute, icons []mcp.Icon, readOnly bool) actioncatalog.SurfaceToolSpec {
	return actioncatalog.SurfaceToolSpec{
		Name:          name,
		Title:         title,
		Description:   description,
		GroupToolName: "gitlab_dynamic",
		BaseDomain:    "dynamic",
		ActionName:    actionName,
		SurfaceKind:   actioncatalog.SurfaceKindDynamicController,
		Route:         route,
		Icons:         icons,
		OwnerPackage:  "dynamic",
		ReadOnly:      readOnly,
		Destructive:   !readOnly,
		Idempotent:    readOnly,
		OpenWorld:     true,
	}
}

func dynamicExecuteRoute(registry *Registry) toolutil.ActionRoute {
	route := toolutil.RouteRequestFunc(func(ctx context.Context, req *mcp.CallToolRequest, input ExecuteInput) (any, error) {
		_, out, err := registry.Execute(ctx, req, input)
		return out, err
	})
	route.Destructive = true
	route.OutputSchema = toolutil.ActionDispatchOutputSchema()
	return route
}
