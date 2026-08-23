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

// DynamicSchemaActionEntry describes one executable entry in the
// dynamic action catalog, returned as a row of the
// [DynamicSchemaIndex] payload.
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

// DynamicSchemaIndex is the JSON payload returned by the
// "gitlab://schema/dynamic/" resource. It enumerates the canonical
// "domain.action" IDs accepted by gitlab_execute_action and points to
// the per-action schema resource.
type DynamicSchemaIndex struct {
	URITemplate   string                     `json:"uri_template"`
	ExecuteAction string                     `json:"execute_action"`
	ActionCount   int                        `json:"action_count"`
	Actions       []DynamicSchemaActionEntry `json:"actions"`
}

// RegisterDynamicSchemaResources wires the dynamic action catalog
// resources into the MCP server. Two resources are registered:
//
//   - The static "gitlab://schema/dynamic/" index resource, which
//     lists every canonical "domain.action" ID accepted by
//     gitlab_execute_action.
//   - The "gitlab://schema/dynamic/{action}" template resource, which
//     returns action-specific params schemas without the meta-tool-only
//     fields (such as confirm) that gitlab_execute_action adds on top.
//
// When catalog is nil an empty [actioncatalog.Catalog] is used; when
// catalog is non-nil a clone is taken so the original is not mutated
// by the resource handlers.
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

// registerDynamicSchemaIndex registers the static catalog resource that
// lists every canonical "domain.action" ID accepted by
// gitlab_execute_action.
func registerDynamicSchemaIndex(server *mcp.Server, catalog *actioncatalog.Catalog) {
	server.AddResource(&mcp.Resource{
		URI:         dynamicSchemaIndexURI,
		Name:        "dynamic_action_index",
		Title:       "Dynamic Action Index",
		MIMEType:    mimeJSON,
		Description: "Catalog of canonical dynamic action IDs accepted by gitlab_execute_action. Use gitlab://schema/dynamic/{action} to fetch an action-specific params schema by domain.action ID.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return marshalResourceJSON(buildDynamicSchemaIndex(catalog))
	})
}

// registerDynamicSchemaTemplate registers the URI-template resource that
// returns the per-action params schema for a canonical dynamic action
// ID.
func registerDynamicSchemaTemplate(server *mcp.Server, catalog *actioncatalog.Catalog) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: dynamicSchemaTemplateURI,
		Name:        "dynamic_action_schema",
		Title:       "Dynamic Action Schema",
		MIMEType:    mimeJSON,
		Description: "JSON Schema for the params object of a canonical dynamic action ID. Replace {action} with a domain.action ID such as project.get or merge_request.create.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return readDynamicSchemaResource(catalog, req.Params.URI)
	})
}

// readDynamicSchemaResource resolves uri to a single dynamic action
// schema. It returns a resource-not-found error when uri is malformed
// (per [parseDynamicSchemaURI]) or when the action ID is not present
// in catalog.
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

// buildDynamicSchemaIndex builds a sorted index of dynamic action
// entries from the catalog. Entries are sorted alphabetically by ID
// so the rendered JSON is stable across runs.
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

// dynamicActionSchema returns the JSON Schema (as a generic map) for
// the params object of one dynamic action. The schema is cloned via
// [toolutil.CloneMetaSchemaRoutes] so per-action edits do not leak
// between sibling actions. When the action has no captured InputSchema
// a permissive fallback object schema is returned.
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

// enrichDynamicSchema adds x_parameter_guidance and (for destructive
// actions) x_destructive / x_confirmation fields to schema in place.
// The map is returned for fluent use.
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

// dynamicParameterGuidance converts the route's
// [toolutil.ParameterGuidance] map into the JSON shape embedded under
// the schema's x_parameter_guidance key. Returns nil when there is no
// guidance to embed.
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

// dynamicParameterGuidanceEntry renders a single guidance item as a
// JSON map. Only the populated fields of item are included so the
// output stays compact.
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

// dynamicSchemaURI builds the per-action schema URI by appending id
// to the [dynamicSchemaIndexURI] prefix.
func dynamicSchemaURI(id actioncatalog.ActionID) string {
	return dynamicSchemaIndexURI + string(id)
}

// parseDynamicSchemaURI extracts the canonical "domain.action" ID
// from a "gitlab://schema/dynamic/{action}" URI. Returns the empty
// string when uri does not start with [dynamicSchemaIndexURI], has an
// embedded slash, or has an empty ID.
func parseDynamicSchemaURI(uri string) string {
	rest := strings.TrimPrefix(uri, dynamicSchemaIndexURI)
	if rest == uri || rest == "" || strings.Contains(rest, "/") {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(rest))
}

// dynamicRequiredParams extracts and deduplicates the list of required
// parameter names from a JSON Schema. It supports both a top-level
// "required" array and alternative-required branches in "anyOf"/"oneOf".
// Returns nil when schema is nil or has no required parameters.
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

// appendDynamicRequiredParamNames appends each non-empty string in raw
// to names. raw is expected to be either a []any or []string (the two
// shapes the JSON parser can return for a JSON array); any other type
// is ignored.
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

// appendDynamicAlternativeRequiredParams walks any "anyOf" or "oneOf"
// branches in schema and merges their "required" lists into names.
// Non-object alternatives are ignored.
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

// dedupeDynamicStrings returns a slice of strings with consecutive
// duplicates and empty values removed. The input is expected to be
// pre-sorted by the caller (the only deduplication guarantee provided).
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
