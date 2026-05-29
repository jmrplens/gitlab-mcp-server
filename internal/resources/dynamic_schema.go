package resources

import (
	"context"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const dynamicSchemaIndexURI = "gitlab://schema/dynamic/"

const dynamicSchemaTemplateURI = "gitlab://schema/dynamic/{action}"

// DynamicSchemaActionEntry describes one executable dynamic catalog action.
type DynamicSchemaActionEntry struct {
	ID             string   `json:"id"`
	Tool           string   `json:"tool"`
	Domain         string   `json:"domain"`
	Action         string   `json:"action"`
	SchemaURI      string   `json:"schema_uri"`
	MetaSchemaURI  string   `json:"meta_schema_uri,omitempty"`
	Destructive    bool     `json:"destructive"`
	RequiredParams []string `json:"required_params,omitempty"`
}

// DynamicSchemaIndex is the payload returned by the dynamic index resource.
type DynamicSchemaIndex struct {
	URITemplate   string                     `json:"uri_template"`
	ExecuteAction string                     `json:"execute_action"`
	ActionCount   int                        `json:"action_count"`
	Actions       []DynamicSchemaActionEntry `json:"actions"`
}

// RegisterDynamicSchemaResources wires dynamic action catalog resources into
// the MCP server. The index uses canonical domain.action IDs accepted by
// gitlab_execute_action, while the template returns action-specific params
// schemas without adding meta-tool-only params such as confirm.
func RegisterDynamicSchemaResources(server *mcp.Server, catalog *actioncatalog.Catalog) {
	snapshot := catalog
	if snapshot == nil {
		snapshot = actioncatalog.NewCatalog()
	} else {
		snapshot = snapshot.Clone()
	}
	registerDynamicSchemaIndex(server, snapshot)
	registerDynamicSchemaTemplate(server, snapshot)
}

func registerDynamicSchemaIndex(server *mcp.Server, catalog *actioncatalog.Catalog) {
	server.AddResource(&mcp.Resource{
		URI:         dynamicSchemaIndexURI,
		Name:        "dynamic_action_index",
		Title:       "Dynamic Action Index",
		MIMEType:    mimeJSON,
		Description: "Catalog of canonical dynamic action IDs accepted by gitlab_execute_action. Use gitlab://schema/dynamic/{action} to fetch an action-specific params schema by domain.action ID.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return marshalResourceJSON(buildDynamicSchemaIndex(catalog))
	})
}

func registerDynamicSchemaTemplate(server *mcp.Server, catalog *actioncatalog.Catalog) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: dynamicSchemaTemplateURI,
		Name:        "dynamic_action_schema",
		Title:       "Dynamic Action Schema",
		MIMEType:    mimeJSON,
		Description: "JSON Schema for the params object of a canonical dynamic action ID. Replace {action} with a domain.action ID such as project.get or merge_request.create.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return readDynamicSchemaResource(catalog, req.Params.URI)
	})
}

func readDynamicSchemaResource(catalog *actioncatalog.Catalog, uri string) (*mcp.ReadResourceResult, error) {
	actionID := parseDynamicSchemaURI(uri)
	if actionID == "" {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	action, ok := catalog.Action(actioncatalog.ActionID(actionID))
	if !ok {
		return nil, mcp.ResourceNotFoundError(uri)
	}
	return marshalResourceJSON(dynamicActionSchema(action))
}

func buildDynamicSchemaIndex(catalog *actioncatalog.Catalog) DynamicSchemaIndex {
	actions := catalog.Actions()
	entries := make([]DynamicSchemaActionEntry, 0, len(actions))
	for _, action := range actions {
		entries = append(entries, DynamicSchemaActionEntry{
			ID:             string(action.ID),
			Tool:           action.ToolName,
			Domain:         action.Domain,
			Action:         action.Name,
			SchemaURI:      dynamicSchemaURI(action.ID),
			MetaSchemaURI:  action.SchemaURI,
			Destructive:    action.Route.Destructive,
			RequiredParams: dynamicRequiredParams(action.Route.InputSchema),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return DynamicSchemaIndex{
		URITemplate:   dynamicSchemaTemplateURI,
		ExecuteAction: "gitlab_execute_action",
		ActionCount:   len(entries),
		Actions:       entries,
	}
}

func dynamicActionSchema(action actioncatalog.Action) map[string]any {
	route := toolutil.CloneMetaSchemaRoutes(map[string]toolutil.ActionMap{action.ToolName: {action.Name: action.Route}})[action.ToolName][action.Name]
	if route.InputSchema == nil {
		schema := map[string]any{
			"type":                 "object",
			"description":          "This dynamic action has no captured parameter schema. Send an empty params object {} unless the action description says otherwise.",
			"additionalProperties": true,
		}
		return enrichDynamicSchema(schema, action)
	}
	return enrichDynamicSchema(route.InputSchema, action)
}

func enrichDynamicSchema(schema map[string]any, action actioncatalog.Action) map[string]any {
	if guidance := dynamicParameterGuidance(action); len(guidance) > 0 {
		schema["x_parameter_guidance"] = guidance
	}
	if action.Route.Destructive {
		schema["x_destructive"] = true
		schema["x_confirmation"] = map[string]any{
			"location":    "gitlab_execute_action.confirm",
			"description": "Set top-level confirm=true on gitlab_execute_action after explicit user approval; do not put confirm inside params.",
		}
	}
	return schema
}

func dynamicParameterGuidance(action actioncatalog.Action) map[string]any {
	if len(action.Route.ParameterGuidance) == 0 {
		return nil
	}
	guidance := make(map[string]any, len(action.Route.ParameterGuidance))
	for name, item := range action.Route.ParameterGuidance {
		entry := dynamicParameterGuidanceEntry(item)
		if len(entry) > 0 {
			guidance[name] = entry
		}
	}
	return guidance
}

func dynamicParameterGuidanceEntry(item toolutil.ParameterGuidance) map[string]any {
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
	return entry
}

func dynamicSchemaURI(id actioncatalog.ActionID) string {
	return dynamicSchemaIndexURI + string(id)
}

func parseDynamicSchemaURI(uri string) string {
	rest := strings.TrimPrefix(uri, dynamicSchemaIndexURI)
	if rest == uri || rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(rest))
}

func dynamicRequiredParams(schema map[string]any) []string {
	if schema == nil {
		return nil
	}
	var names []string
	names = appendDynamicRequiredParamNames(names, schema["required"])
	names = appendDynamicAlternativeRequiredParams(names, schema)
	sort.Strings(names)
	return dedupeDynamicStrings(names)
}

func appendDynamicRequiredParamNames(names []string, raw any) []string {
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok && name != "" {
				names = append(names, name)
			}
		}
	case []string:
		names = append(names, values...)
	}
	return names
}

func appendDynamicAlternativeRequiredParams(names []string, schema map[string]any) []string {
	for _, keyword := range []string{"anyOf", "oneOf"} {
		alternatives, ok := schema[keyword].([]any)
		if !ok || len(alternatives) == 0 {
			continue
		}
		for _, raw := range alternatives {
			alternative, isObject := raw.(map[string]any)
			if !isObject {
				continue
			}
			names = appendDynamicRequiredParamNames(names, alternative["required"])
		}
	}
	return names
}

func dedupeDynamicStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := values[:0]
	var last string
	for index, value := range values {
		if value == "" || (index > 0 && value == last) {
			continue
		}
		out = append(out, value)
		last = value
	}
	return out
}
