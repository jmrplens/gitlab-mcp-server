package resources

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// metaSchemaIndexURI is the static URI returning the full meta-tool action
// catalog as a JSON object.
const metaSchemaIndexURI = toolutil.MetaSchemaIndexURI

// metaSchemaTemplateURI is the URI template for per-action params schemas.
const metaSchemaTemplateURI = toolutil.MetaSchemaTemplateURI

// MetaSchemaIndexEntry is a single tool entry in the index resource payload.
type MetaSchemaIndexEntry = toolutil.MetaSchemaIndexEntry

// MetaSchemaIndex is the payload returned by the index resource.
type MetaSchemaIndex = toolutil.MetaSchemaIndex

// RegisterMetaSchemaResources wires the index resource and the per-action
// template resource into the MCP server. Both are read-only and do not need
// a GitLab client; callers pass the exact meta-tool routes that are visible
// on this server after configuration filters have been applied.
func RegisterMetaSchemaResources(server *mcp.Server, routes map[string]toolutil.ActionMap) {
	snapshot := toolutil.CloneMetaSchemaRoutes(routes)
	registerMetaSchemaIndex(server, snapshot)
	registerMetaSchemaTemplate(server, snapshot)
}

// registerMetaSchemaIndex registers the static catalog resource that lists
// every visible meta-tool and its supported action names.
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
		return marshalResourceJSON(toolutil.BuildMetaSchemaIndex(routes))
	})
}

// registerMetaSchemaTemplate registers the URI-template resource that returns
// a JSON Schema for one meta-tool action's params object.
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
		tool, action := toolutil.ParseMetaSchemaURI(req.Params.URI)
		if tool == "" || action == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		schema, ok := toolutil.LookupMetaActionSchema(routes, tool, action)
		if !ok {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		return marshalResourceJSON(schema)
	})
}

// parseMetaSchemaURI extracts the {tool} and {action} segments from a
// gitlab://schema/meta/<tool>/<action> URI. Returns empty strings on any
// shape mismatch (extra slashes, missing segments, empty values).
func parseMetaSchemaURI(uri string) (tool, action string) {
	return toolutil.ParseMetaSchemaURI(uri)
}
