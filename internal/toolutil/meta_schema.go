package toolutil

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// MetaSchemaIndexURI is the static URI returning the full meta-tool action catalog.
const MetaSchemaIndexURI = "gitlab://schema/meta/"

// MetaSchemaTemplateURI is the URI template for per-action params schemas.
const MetaSchemaTemplateURI = "gitlab://schema/meta/{tool}/{action}"

// MetaSchemaIndexEntry is a single tool entry in the resource index payload.
type MetaSchemaIndexEntry struct {
	Tool    string   `json:"tool"`
	Actions []string `json:"actions"`
}

// MetaSchemaIndex is the payload returned by the schema index resource.
type MetaSchemaIndex struct {
	URITemplate string                 `json:"uri_template"`
	Tools       []MetaSchemaIndexEntry `json:"tools"`
}

// MetaSchemaActionEntry describes one meta-tool action in the tool-call index.
type MetaSchemaActionEntry struct {
	Action      string `json:"action"`
	SchemaURI   string `json:"schema_uri"`
	Destructive bool   `json:"destructive"`
}

// MetaSchemaToolEntry describes one meta-tool in the tool-call index.
type MetaSchemaToolEntry struct {
	Tool        string                  `json:"tool"`
	ActionCount int                     `json:"action_count"`
	Actions     []MetaSchemaActionEntry `json:"actions"`
}

// MetaSchemaDiscoveryIndex is a model-controlled schema discovery payload.
type MetaSchemaDiscoveryIndex struct {
	URITemplate string                `json:"uri_template"`
	ToolCount   int                   `json:"tool_count"`
	ActionCount int                   `json:"action_count"`
	Tools       []MetaSchemaToolEntry `json:"tools"`
}

// MetaSchemaRegistry stores the visible meta-tool route snapshot used by
// model-controlled schema discovery actions.
type MetaSchemaRegistry struct {
	mu     sync.RWMutex
	routes map[string]ActionMap
}

// NewMetaSchemaRegistry creates a registry initialized with a route snapshot.
func NewMetaSchemaRegistry(routes map[string]ActionMap) *MetaSchemaRegistry {
	registry := &MetaSchemaRegistry{}
	registry.SetRoutes(routes)
	return registry
}

// SetRoutes replaces the registry contents with a defensive route snapshot.
func (r *MetaSchemaRegistry) SetRoutes(routes map[string]ActionMap) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = CloneMetaSchemaRoutes(routes)
}

// Routes returns a defensive copy of the registry contents.
func (r *MetaSchemaRegistry) Routes() map[string]ActionMap {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return CloneMetaSchemaRoutes(r.routes)
}

// CloneMetaSchemaRoutes creates a defensive snapshot of route maps so consumers
// do not observe later registration, filtering, or schema map changes.
func CloneMetaSchemaRoutes(routes map[string]ActionMap) map[string]ActionMap {
	out := make(map[string]ActionMap, len(routes))
	for tool, actions := range routes {
		actionCopy := make(ActionMap, len(actions))
		for action, route := range actions {
			routeCopy := route
			if route.InputSchema != nil {
				routeCopy.InputSchema = cloneSchemaMap(route.InputSchema)
			}
			if route.OutputSchema != nil {
				routeCopy.OutputSchema = cloneSchemaMap(route.OutputSchema)
			}
			routeCopy.ParameterGuidance = cloneParameterGuidanceMap(route.ParameterGuidance)
			routeCopy.Aliases = cloneRouteStrings(route.Aliases)
			routeCopy.Tags = cloneRouteStrings(route.Tags)
			routeCopy.RelatedActions = cloneRouteStrings(route.RelatedActions)
			actionCopy[action] = routeCopy
		}
		out[tool] = actionCopy
	}
	return out
}

// BuildMetaSchemaIndex builds the resource-compatible schema index payload.
func BuildMetaSchemaIndex(routes map[string]ActionMap) MetaSchemaIndex {
	tools := make([]MetaSchemaIndexEntry, 0, len(routes))
	for tool, actions := range routes {
		names := sortedActionNames(actions)
		tools = append(tools, MetaSchemaIndexEntry{Tool: tool, Actions: names})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Tool < tools[j].Tool })
	return MetaSchemaIndex{URITemplate: MetaSchemaTemplateURI, Tools: tools}
}

// BuildMetaSchemaDiscoveryIndex builds the richer tool-call schema index payload.
func BuildMetaSchemaDiscoveryIndex(routes map[string]ActionMap) MetaSchemaDiscoveryIndex {
	tools := make([]MetaSchemaToolEntry, 0, len(routes))
	actionCount := 0
	toolNames := make([]string, 0, len(routes))
	for tool := range routes {
		toolNames = append(toolNames, tool)
	}
	sort.Strings(toolNames)
	for _, tool := range toolNames {
		actions := routes[tool]
		entry := buildMetaSchemaToolEntry(tool, actions)
		actionCount += entry.ActionCount
		tools = append(tools, entry)
	}
	return MetaSchemaDiscoveryIndex{
		URITemplate: MetaSchemaTemplateURI,
		ToolCount:   len(tools),
		ActionCount: actionCount,
		Tools:       tools,
	}
}

// BuildMetaSchemaDiscoveryIndexForTool builds the tool-call index for one meta-tool.
func BuildMetaSchemaDiscoveryIndexForTool(routes map[string]ActionMap, tool string) (MetaSchemaDiscoveryIndex, bool) {
	actions, ok := routes[tool]
	if !ok {
		return MetaSchemaDiscoveryIndex{}, false
	}
	entry := buildMetaSchemaToolEntry(tool, actions)
	return MetaSchemaDiscoveryIndex{
		URITemplate: MetaSchemaTemplateURI,
		ToolCount:   1,
		ActionCount: entry.ActionCount,
		Tools:       []MetaSchemaToolEntry{entry},
	}, true
}

// LookupMetaActionSchema returns the per-action params schema for a tool/action pair.
func LookupMetaActionSchema(routes map[string]ActionMap, tool, action string) (map[string]any, bool) {
	actions, ok := routes[tool]
	if !ok {
		return nil, false
	}
	route, ok := actions[action]
	if !ok {
		return nil, false
	}
	if route.InputSchema == nil {
		schema := map[string]any{
			"type":                 "object",
			"description":          "This action has no captured parameter schema. Send an empty object {} or consult the meta-tool description for required fields.",
			"additionalProperties": true,
		}
		return enrichParameterGuidanceSchema(enrichDestructiveSchema(schema, route.Destructive), route.ParameterGuidance), true
	}
	return enrichParameterGuidanceSchema(enrichDestructiveSchema(cloneSchemaMap(route.InputSchema), route.Destructive), route.ParameterGuidance), true
}

func enrichDestructiveSchema(schema map[string]any, destructive bool) map[string]any {
	if !destructive {
		return schema
	}
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = make(map[string]any)
		schema["properties"] = properties
	}
	if _, exists := properties["confirm"]; !exists {
		properties["confirm"] = map[string]any{
			"type":        "boolean",
			"description": "Set true to explicitly confirm this destructive action instead of relying on MCP elicitation.",
		}
	}
	schema["x_destructive"] = true
	return schema
}

func enrichParameterGuidanceSchema(schema map[string]any, guidance map[string]ParameterGuidance) map[string]any {
	if len(guidance) == 0 {
		return schema
	}
	encoded := make(map[string]any, len(guidance))
	for name, item := range guidance {
		entry := make(map[string]any, 4)
		if item.SemanticRole != "" {
			entry["semantic_role"] = item.SemanticRole
		}
		if item.ValueSource != "" {
			entry["value_source"] = item.ValueSource
		}
		if len(item.CommonConfusions) > 0 {
			entry["common_confusions"] = append([]string(nil), item.CommonConfusions...)
		}
		if item.ExampleBinding != "" {
			entry["example_binding"] = item.ExampleBinding
		}
		if len(entry) > 0 {
			encoded[name] = entry
		}
	}
	if len(encoded) > 0 {
		schema["x_parameter_guidance"] = encoded
	}
	return schema
}

func cloneSchemaMap(value map[string]any) map[string]any {
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = cloneSchemaValue(item)
	}
	return out
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSchemaValue(item)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		copy(out, typed)
		return out
	default:
		return value
	}
}

// ParseMetaSchemaURI extracts the tool and action segments from a schema URI.
func ParseMetaSchemaURI(uri string) (tool, action string) {
	rest := strings.TrimPrefix(uri, MetaSchemaIndexURI)
	if rest == uri {
		return "", ""
	}
	parts := strings.Split(rest, "/")
	if len(parts) != 2 {
		return "", ""
	}
	if parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// MetaSchemaURI returns the resource URI for a tool/action schema.
func MetaSchemaURI(tool, action string) string {
	return fmt.Sprintf("gitlab://schema/meta/%s/%s", tool, action)
}

func buildMetaSchemaToolEntry(tool string, actions ActionMap) MetaSchemaToolEntry {
	actionNames := sortedActionNames(actions)
	actionEntries := make([]MetaSchemaActionEntry, 0, len(actionNames))
	for _, action := range actionNames {
		route := actions[action]
		actionEntries = append(actionEntries, MetaSchemaActionEntry{
			Action:      action,
			SchemaURI:   MetaSchemaURI(tool, action),
			Destructive: route.Destructive,
		})
	}
	return MetaSchemaToolEntry{Tool: tool, ActionCount: len(actionEntries), Actions: actionEntries}
}

func sortedActionNames(actions ActionMap) []string {
	names := make([]string, 0, len(actions))
	for action := range actions {
		names = append(names, action)
	}
	sort.Strings(names)
	return names
}
