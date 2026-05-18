package dynamic

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ControllerSurfaceSpecs returns explicit surface metadata for Dynamic
// controller tools. Registration stays custom because these tools expose
// controller-specific Markdown results and, for execute, action-dependent output.
func ControllerSurfaceSpecs(registry *Registry) []actioncatalog.SurfaceToolSpec {
	if registry == nil {
		registry = NewRegistry(nil)
	}
	return []actioncatalog.SurfaceToolSpec{
		dynamicControllerSpec(findToolName, "GitLab Find Action", "Find GitLab catalog actions by searching with domain keywords (e.g. 'project create', 'merge request approve', 'pipeline retry', 'issue delete', 'ci variable'). Returns exact schemas, required params, safety metadata, and execute examples. ALWAYS use this before gitlab_execute_tool when the canonical action ID or params schema is not already known; do NOT invent action IDs.", "find", toolutil.RouteRequestFunc(func(ctx context.Context, req *mcp.CallToolRequest, input FindInput) (FindOutput, error) {
			_, out, err := registry.Find(ctx, req, input)
			return out, err
		}), toolutil.IconSearch, true),
		dynamicControllerSpec(executeToolName, "GitLab Execute Tool", "Execute one GitLab catalog action by canonical action ID (e.g. domain.action). Always include params as an object: {\"action\":\"domain.action\",\"params\":{...}}; use params:{} only for actions with no parameters. Use gitlab_find_action first unless the exact action ID and all required param names are already known. Do NOT guess or invent action IDs. Include ONLY the exact param names from the action schema; do NOT invent extra params. Destructive actions require confirm=true.", "execute", dynamicExecuteRoute(registry), toolutil.IconServer, false),
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
	route.OutputSchema = map[string]any{"type": "object", "additionalProperties": true}
	return route
}
