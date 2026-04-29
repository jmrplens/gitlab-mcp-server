// meta_schema.go registers the gitlab://schema/meta/* MCP resources, which
// expose per-action JSON Schemas for every meta-tool registered in this
// server. The resources are advisory: they let LLMs discover the exact
// params shape for a chosen meta-tool action without having to inspect the
// full meta-tool description or guess from examples.
//
// Two URIs are exposed:
//   - gitlab://schema/meta/             — index resource listing every
//     registered meta-tool and its actions.
//   - gitlab://schema/meta/{tool}/{action} — JSON Schema for the action's
//     params property.

package resources

import (
	"context"
	"maps"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// metaSchemaIndexURI is the static URI returning the full meta-tool action
// catalog as a JSON object.
const metaSchemaIndexURI = "gitlab://schema/meta/"

// metaSchemaTemplateURI is the URI template for per-action params schemas.
const metaSchemaTemplateURI = "gitlab://schema/meta/{tool}/{action}"

// MetaSchemaIndexEntry is a single tool entry in the index resource payload.
type MetaSchemaIndexEntry struct {
	Tool    string   `json:"tool"`
	Actions []string `json:"actions"`
}

// MetaSchemaIndex is the payload returned by the index resource.
type MetaSchemaIndex struct {
	URITemplate string                 `json:"uri_template"`
	Tools       []MetaSchemaIndexEntry `json:"tools"`
}

// RegisterMetaSchemaResources wires the index resource and the per-action
// template resource into the MCP server. Both are read-only and do not need
// a GitLab client; callers pass the exact meta-tool routes that are visible
// on this server after configuration filters have been applied.
func RegisterMetaSchemaResources(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	snapshot := cloneMetaSchemaRoutes(routes)
	registerMetaSchemaIndex(server, snapshot)
	registerMetaSchemaTemplate(server, snapshot)
}

func registerMetaSchemaIndex(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	server.AddResource(&mcp.Resource{
		URI:         metaSchemaIndexURI,
		Name:        "meta_schema_index",
		Title:       "Meta-Tool Schema Index",
		MIMEType:    mimeJSON,
		Description: "Catalog of every registered meta-tool and its actions. Use the gitlab://schema/meta/{tool}/{action} template resource to fetch the JSON Schema for a specific action's params.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return marshalResourceJSON(buildMetaSchemaIndex(routes))
	})
}

func registerMetaSchemaTemplate(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: metaSchemaTemplateURI,
		Name:        "meta_action_schema",
		Title:       "Meta-Tool Action Schema",
		MIMEType:    mimeJSON,
		Description: "JSON Schema for the `params` property of a specific meta-tool action. Replace {tool} with a meta-tool name (e.g. gitlab_merge_request) and {action} with one of its actions (e.g. create). Use the `gitlab://schema/meta/` index resource to enumerate valid combinations.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		tool, action := parseMetaSchemaURI(req.Params.URI)
		if tool == "" || action == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		schema, ok := lookupMetaActionSchema(routes, tool, action)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return marshalResourceJSON(schema)
	})
}

func cloneMetaSchemaRoutes(routes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	out := make(map[string]toolutil.ActionMap, len(routes))
	for tool, actions := range routes {
		actionCopy := make(toolutil.ActionMap, len(actions))
		maps.Copy(actionCopy, actions)
		out[tool] = actionCopy
	}
	return out
}

// buildMetaSchemaIndex builds a deterministic snapshot of all registered
// meta-tools and their actions, sorted alphabetically.
func buildMetaSchemaIndex(routes map[string]toolutil.ActionMap) MetaSchemaIndex {
	tools := make([]MetaSchemaIndexEntry, 0, len(routes))
	for tool, actions := range routes {
		names := make([]string, 0, len(actions))
		for action := range actions {
			names = append(names, action)
		}
		sort.Strings(names)
		tools = append(tools, MetaSchemaIndexEntry{Tool: tool, Actions: names})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Tool < tools[j].Tool })
	return MetaSchemaIndex{URITemplate: metaSchemaTemplateURI, Tools: tools}
}

// lookupMetaActionSchema returns the per-action params schema for the given
// tool/action pair. Returns false when the tool or action is unknown. When
// the route exists but has no captured InputSchema, returns a permissive
// fallback object schema (with `additionalProperties: true` and a guidance
// description) and true, so clients always get a usable JSON Schema.
func lookupMetaActionSchema(routes map[string]toolutil.ActionMap, tool, action string) (map[string]any, bool) {
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

// parseMetaSchemaURI extracts the {tool} and {action} segments from a
// gitlab://schema/meta/<tool>/<action> URI. Returns empty strings on any
// shape mismatch (extra slashes, missing segments, empty values).
func parseMetaSchemaURI(uri string) (tool, action string) {
	rest := strings.TrimPrefix(uri, metaSchemaIndexURI)
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
