// schema_lockdown.go implements tools/list schema hardening that adds
// additionalProperties=false to generated object schemas.
package toolutil

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// LockdownInputSchemas registers a receiving middleware that rewrites
// tools/list responses so every tool's inputSchema declares
// `additionalProperties: false` at the root and on any nested object schema
// reachable through "properties", "items", "anyOf", "oneOf", or "allOf".
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
// `tools/list` middleware. To avoid a data race when multiple clients call
// `tools/list` concurrently (each invocation would otherwise mutate the
// shared *Tool.InputSchema map), the actual mutation is guarded by a
// `sync.Once`: the first call performs the lockdown, and concurrent callers
// block until that mutation completes. Subsequent calls are pure reads on
// the (now stable) schema maps and run lock-free.
func LockdownInputSchemas(server *mcp.Server) {
	if server == nil {
		return
	}
	var once sync.Once
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || method != "tools/list" {
				return result, err
			}
			if listResult, ok := result.(*mcp.ListToolsResult); ok && listResult != nil {
				once.Do(func() {
					for _, t := range listResult.Tools {
						if schema, isMap := t.InputSchema.(map[string]any); isMap {
							lockdownSchemaNode(schema)
						}
					}
				})
			}
			return result, nil
		}
	})
}

// lockdownSchemaNode forces additionalProperties=false on any object schema
// node that does not already declare it, recursing through nested schemas.
func lockdownSchemaNode(node map[string]any) {
	if isObjectType(node) {
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
