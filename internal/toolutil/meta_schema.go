package toolutil

import (
	"fmt"
	"sort"
	"strconv"
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

// SetRoutes replaces the registry contents with a snapshot of routes, taken
// with [CloneMetaSchemaRoutes]: the maps are the registry's own, the schemas
// inside them are shared with the caller and frozen.
func (r *MetaSchemaRegistry) SetRoutes(routes map[string]ActionMap) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = CloneMetaSchemaRoutes(routes)
}

// Routes returns a snapshot of the registry contents, taken with
// [CloneMetaSchemaRoutes]: later SetRoutes calls do not reach it, and the
// schemas in it are shared and must not be mutated.
func (r *MetaSchemaRegistry) Routes() map[string]ActionMap {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return CloneMetaSchemaRoutes(r.routes)
}

// CloneMetaSchemaRoutes returns a snapshot of the route maps: the two map
// levels are new, so later insertions and deletions in routes do not reach
// the snapshot, and every route in it is a [CloneActionRoute] copy.
//
// The schemas are not copied. A route's InputSchema, OutputSchema and
// ParameterGuidance are frozen and shared by every consumer in the process,
// which is what lets a catalog cached per configuration serve every server
// without a copy per server; the copies this function used to make were half
// of the heap at a hundred pooled credentials. A consumer that must change a
// schema derives its own through [DeriveSchema].
func CloneMetaSchemaRoutes(routes map[string]ActionMap) map[string]ActionMap {
	out := make(map[string]ActionMap, len(routes))
	for tool, actions := range routes {
		actionCopy := make(ActionMap, len(actions))
		for action, route := range actions {
			actionCopy[action] = CloneActionRoute(route)
		}
		out[tool] = actionCopy
	}
	return out
}

// CloneActionRoute returns a copy of the route that owns its string slices
// and shares everything else. The handler, the types and the frozen schema
// and guidance maps describe the same action and are not copied; the
// Aliases, Tags and RelatedActions slices are, so a caller may append to
// them without reaching the original.
func CloneActionRoute(route ActionRoute) ActionRoute {
	route.Aliases = cloneRouteStrings(route.Aliases)
	route.Tags = cloneRouteStrings(route.Tags)
	route.RelatedActions = cloneRouteStrings(route.RelatedActions)
	return route
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
	return MetaActionSchema(route), true
}

// MetaActionSchema returns the params schema served for one meta-tool action:
// the route's input schema with the destructive confirmation property and the
// parameter guidance added, or a permissive placeholder when the route
// captured no schema. For a route of a shared catalog the result is built
// once per process and shared; the caller must not mutate it.
func MetaActionSchema(route ActionRoute) map[string]any {
	if route.InputSchema == nil {
		schema := map[string]any{
			"type":                 "object",
			"description":          "This action has no captured parameter schema. Send an empty object {} or consult the meta-tool description for required fields.",
			"additionalProperties": true,
		}
		return enrichParameterGuidanceSchema(enrichDestructiveSchema(schema, route.Destructive), route.ParameterGuidance)
	}
	transform := "meta-action|destructive=" + strconv.FormatBool(route.Destructive) + "|guidance=" + ParameterGuidanceIdentity(route.ParameterGuidance)
	derived := DeriveSchema(route.InputSchema, transform, func() any {
		return enrichParameterGuidanceSchema(enrichDestructiveSchema(cloneSchemaMap(route.InputSchema), route.Destructive), route.ParameterGuidance)
	})
	schema, _ := derived.(map[string]any)
	return schema
}

// enrichDestructiveSchema adds the confirm property and the x_destructive
// marker to a destructive action's schema, in place: the caller owns schema,
// having copied it from wherever it came.
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
