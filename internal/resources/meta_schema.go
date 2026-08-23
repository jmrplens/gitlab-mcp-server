package resources

import (
	"context"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// metaSchemaIndexURI is the static URI returning the full meta-tool
// action catalog as a JSON object.
const metaSchemaIndexURI = "gitlab://schema/meta/"

// metaSchemaTemplateURI is the URI template for per-action params
// schemas exposed by the meta surface.
const metaSchemaTemplateURI = "gitlab://schema/meta/{tool}/{action}"

// MetaSchemaIndexEntry is a single tool entry in the [MetaSchemaIndex]
// payload: the meta-tool name and the list of its supported action
// names.
type MetaSchemaIndexEntry struct {
	Tool    string   `json:"tool"`
	Actions []string `json:"actions"`
}

// MetaSchemaIndex is the JSON payload returned by the
// "gitlab://schema/meta/" resource. It enumerates the meta-tool
// catalog and points to the per-action schema template.
type MetaSchemaIndex struct {
	URITemplate string                 `json:"uri_template"`
	Tools       []MetaSchemaIndexEntry `json:"tools"`
}

// RegisterMetaSchemaResources wires the index resource and the
// per-action template resource into the MCP server. Both resources are
// read-only and do not require a GitLab client; callers pass the exact
// meta-tool routes that are visible on this server after configuration
// filters have been applied.
//
// The routes argument is cloned (via [toolutil.CloneMetaSchemaRoutes])
// so later route registrations do not leak into already-wired servers.
func RegisterMetaSchemaResources(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	snapshot := cloneMetaSchemaRoutes(routes)
	registerMetaSchemaIndex(server, snapshot)
	registerMetaSchemaTemplate(server, snapshot)
}

// registerMetaSchemaIndex registers the static catalog resource that
// lists every visible meta-tool and its supported action names. The
// underlying payload is built once via [buildMetaSchemaIndex] and
// returned verbatim on every read.
func registerMetaSchemaIndex(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	server.AddResource(&mcp.Resource{
		URI:         metaSchemaIndexURI,
		Name:        "meta_schema_index",
		Title:       "Meta-Tool Schema Index",
		MIMEType:    mimeJSON,
		Description: "Catalog of every registered meta-tool and its actions. Use the gitlab://schema/meta/{tool}/{action} template resource to fetch the JSON Schema for a specific action's params.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconConfig,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return marshalResourceJSON(buildMetaSchemaIndex(routes))
	})
}

// registerMetaSchemaTemplate registers the URI-template resource that
// returns a JSON Schema for one meta-tool action's params object. The
// tool and action are parsed from the request URI via
// [parseMetaSchemaURI].
func registerMetaSchemaTemplate(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: metaSchemaTemplateURI,
		Name:        "meta_action_schema",
		Title:       "Meta-Tool Action Schema",
		MIMEType:    mimeJSON,
		Description: "JSON Schema for the `params` property of a specific meta-tool action. Replace {tool} with a meta-tool name (e.g. gitlab_merge_request) and {action} with one of its actions (e.g. create). Use the `gitlab://schema/meta/` index resource to enumerate valid combinations.",
		Annotations: toolutil.ResourceDetail,
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

// cloneMetaSchemaRoutes creates a shallow snapshot of route maps so
// resource handlers do not observe later registration changes from
// other server builds. It is a thin wrapper around
// [toolutil.CloneMetaSchemaRoutes] that exists for testability.
func cloneMetaSchemaRoutes(routes map[string]toolutil.ActionMap) map[string]toolutil.ActionMap {
	return toolutil.CloneMetaSchemaRoutes(routes)
}

// buildMetaSchemaIndex builds a deterministic snapshot of all
// registered meta-tools and their actions, sorted alphabetically by
// tool name and action name.
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

// lookupMetaActionSchema returns the per-action params schema for the
// given tool/action pair. It returns false when the tool or action is
// unknown. When the route exists but has no captured InputSchema, a
// permissive fallback object schema (with "additionalProperties: true"
// and a guidance description) is returned along with true, so clients
// always get a usable JSON Schema.
func lookupMetaActionSchema(routes map[string]toolutil.ActionMap, tool, action string) (map[string]any, bool) {
	return toolutil.LookupMetaActionSchema(routes, tool, action)
}

// parseMetaSchemaURI extracts the {tool} and {action} segments from a
// "gitlab://schema/meta/<tool>/<action>" URI. Returns empty strings on
// any shape mismatch (extra slashes, missing segments, empty values).
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
