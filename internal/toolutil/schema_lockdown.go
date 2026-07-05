package toolutil

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LockdownInputSchemas registers a receiving middleware that rewrites
// tools/list responses so every tool's inputSchema declares
// `additionalProperties: false` at the root and on any nested object schema
// reachable through "properties", "items", "anyOf", "oneOf", or "allOf". It
// also strips jsonschema tag metadata such as ",required" from property
// descriptions after the SDK generates schemas. Schemas converted from SDK
// types are stored back as map[string]any values, so callers inspecting tools
// after this middleware runs should not expect the original concrete schema
// type.
//
// Background. The MCP specification (2025-11-25 §server/tools) requires
// inputSchema to be a valid JSON Schema object but does not mandate
// `additionalProperties`. JSON Schema 2020-12 default semantics treat an
// unspecified `additionalProperties` as `true`, which silently accepts
// unknown fields. When an LLM mistypes an argument name (e.g. "projetc_id"
// instead of "project_id"), the server forwards an empty value to the
// handler, which then fails with a confusing "missing parameter" error
// rather than the actionable "unknown property" diagnostic the LLM needs to
// self-correct.
//
// Schemas that already declare `additionalProperties` (true or false) at a
// given level are left untouched, so meta-tool router branches that
// intentionally permit unknown fields for forward compatibility remain
// intact.
//
// Concurrency. The MCP Go SDK does not expose a public API to enumerate
// registered tools at startup, so the transformation runs inside a
// `tools/list` middleware via [onFirstToolsList], which guards the mutation
// with a `sync.Once`.
func LockdownInputSchemas(server *mcp.Server) {
	onFirstToolsList(server, func(mcpTools []*mcp.Tool) {
		for _, t := range mcpTools {
			if schema := schemaMap(t.InputSchema); schema != nil {
				normalizeSchemaDescriptions(schema)
				lockdownSchemaNode(schema)
				t.InputSchema = schema
			}
		}
	})
}

func schemaMap(schema any) map[string]any {
	if schema == nil {
		return nil
	}
	if typed, ok := schema.(map[string]any); ok {
		return typed
	}
	data, err := json.Marshal(schema)
	if err != nil {
		slog.Warn("failed to marshal MCP input schema", "error", err, "schema_type", fmt.Sprintf("%T", schema), "schema", schema)
		return nil
	}
	var decoded map[string]any
	if unmarshalErr := json.Unmarshal(data, &decoded); unmarshalErr != nil {
		slog.Warn("failed to unmarshal MCP input schema", "error", unmarshalErr, "schema_type", fmt.Sprintf("%T", schema), "schema", schema)
		return nil
	}
	return decoded
}

// lockdownSchemaNode forces additionalProperties=false and ensures a properties
// object exists on any object schema node, recursing through nested schemas.
func lockdownSchemaNode(node map[string]any) {
	if isObjectType(node) {
		if _, present := node["properties"]; !present {
			node["properties"] = map[string]any{}
		}
		if _, present := node["additionalProperties"]; !present {
			node["additionalProperties"] = false
		}
	}

	if props, ok := node["properties"].(map[string]any); ok {
		for _, v := range props {
			if child, isMap := v.(map[string]any); isMap {
				lockdownSchemaNode(child)
			}
		}
	}

	if items, ok := node["items"].(map[string]any); ok {
		lockdownSchemaNode(items)
	}

	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := node[key].([]any); ok {
			for _, v := range arr {
				if child, isMap := v.(map[string]any); isMap {
					lockdownSchemaNode(child)
				}
			}
		}
	}
}

// isObjectType reports whether a JSON Schema node represents an object.
// Schemas without an explicit "type" but with "properties" are treated as
// objects per JSON Schema convention used by jsonschema-go.
func isObjectType(node map[string]any) bool {
	if t, ok := node["type"].(string); ok {
		return t == "object"
	}
	if _, hasProps := node["properties"]; hasProps {
		return true
	}
	return false
}
