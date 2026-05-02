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

// CloneMetaSchemaRoutes creates a shallow snapshot of route maps so consumers
// do not observe later registration or filtering changes.
func CloneMetaSchemaRoutes(routes map[string]ActionMap) map[string]ActionMap {
	return cloneMetaRoutes(routes)
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
		return map[string]any{
			"type":                 "object",
			"description":          "This action has no captured parameter schema. Send an empty object {} or consult the meta-tool description for required fields.",
			"additionalProperties": true,
		}, true
	}
	return route.InputSchema, true
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
