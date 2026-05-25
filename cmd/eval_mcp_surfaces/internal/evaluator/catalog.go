package evaluator

import (
	"reflect"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

func convertTools(toolList []*mcp.Tool) []modelTool {
	out := make([]modelTool, 0, len(toolList))
	for _, tool := range toolList {
		if tool == nil {
			continue
		}
		out = append(out, modelToolFromParts(tool.Name, tool.Description, tool.InputSchema))
	}
	return sortedModelTools(out)
}

// convertSnapshotTools resolves convert snapshot tools for evaluator execution.
func convertSnapshotTools(snapshot []snapshotTool) []modelTool {
	out := make([]modelTool, 0, len(snapshot))
	for _, tool := range snapshot {
		out = append(out, modelToolFromParts(tool.Name, tool.Description, tool.InputSchema))
	}
	return sortedModelTools(out)
}

// modelToolFromParts builds a model-facing tool with a fallback object schema.
func modelToolFromParts(name, description string, inputSchema any) modelTool {
	if isNilModelToolSchema(inputSchema) {
		inputSchema = map[string]any{"type": "object"}
	}
	return modelTool{Name: name, Description: description, InputSchema: inputSchema}
}

// isNilModelToolSchema reports whether a schema is nil, including typed-nil maps.
func isNilModelToolSchema(inputSchema any) bool {
	if inputSchema == nil {
		return true
	}
	value := reflect.ValueOf(inputSchema)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// sortedModelTools sorts model tools and marks the final entry cacheable.
func sortedModelTools(out []modelTool) []modelTool {
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) > 0 {
		out[len(out)-1].CacheControl = &cacheControl{Type: "ephemeral"}
	}
	return out
}

// appendCapabilityBridgeTools adds evaluator tools for MCP client capabilities.

func catalogToolNames(catalog []modelTool) map[string]bool {
	names := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		names[tool.Name] = true
	}
	return names
}

// routesFromSnapshot maps routes from snapshot between API and evaluator models.
func routesFromSnapshot(snapshot []snapshotTool) map[string]toolutil.ActionMap {
	routes := make(map[string]toolutil.ActionMap, len(snapshot))
	for _, tool := range snapshot {
		actions := actionEnumFromSchema(tool.InputSchema)
		if len(actions) == 0 {
			continue
		}
		actionMap := make(toolutil.ActionMap, len(actions))
		for _, action := range actions {
			actionMap[action] = toolutil.ActionRoute{}
		}
		routes[tool.Name] = actionMap
	}
	return routes
}

// actionEnumFromSchema derives action enum from schema from tool schema metadata.
func actionEnumFromSchema(schema map[string]any) []string {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	actionProperty, ok := properties["action"].(map[string]any)
	if !ok {
		return nil
	}
	rawEnum, ok := actionProperty["enum"].([]any)
	if !ok {
		return nil
	}
	actions := make([]string, 0, len(rawEnum))
	for _, rawAction := range rawEnum {
		action, okAction := rawAction.(string)
		if okAction && action != "" {
			actions = append(actions, action)
		}
	}
	return actions
}

// modelRunner holds model runner data for the evaluator package.
