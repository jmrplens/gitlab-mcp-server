package toolutil

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// EnrichPaginationConstraints registers a receiving middleware that walks
// every tools/list response and injects JSON Schema numeric constraints on
// the standard pagination property names so LLM clients see the bounds
// directly in tools/list rather than only through prose in the description.
//
// The middleware operates per property name:
//
//   - `page`     gets `minimum: 1`
//   - `per_page` gets `minimum: 1` and `maximum: 100`
//
// Existing constraints are preserved: if a schema already declares
// `minimum` or `maximum` on these properties the middleware leaves them
// untouched so domain-specific overrides remain authoritative. Only nodes
// whose `type` is `integer` or `number` (or unset, defaulting to integer
// per the Go SDK's int-typed pagination input) are modified, so unrelated
// properties named `page` on a custom schema cannot be silently mutated.
//
// The transformation runs after LockdownInputSchemas so it sees the same
// fully-populated schema set every list/tools response carries.
func EnrichPaginationConstraints(server *mcp.Server) {
	if server == nil {
		return
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || method != "tools/list" {
				return result, err
			}
			if listResult, ok := result.(*mcp.ListToolsResult); ok && listResult != nil {
				for _, t := range listResult.Tools {
					if schema, ok := t.InputSchema.(map[string]any); ok {
						enrichPaginationNode(schema)
					}
				}
			}
			return result, nil
		}
	})
}

// enrichPaginationNode adds page/per_page numeric bounds to any matching
// property in the given schema node, then recurses through nested schemas.
func enrichPaginationNode(node map[string]any) {
	if props, ok := node["properties"].(map[string]any); ok {
		if page, ok := props["page"].(map[string]any); ok && isIntegerLike(page) {
			setIfAbsent(page, "minimum", float64(1))
		}
		if perPage, ok := props["per_page"].(map[string]any); ok && isIntegerLike(perPage) {
			setIfAbsent(perPage, "minimum", float64(1))
			setIfAbsent(perPage, "maximum", float64(100))
		}
		for _, v := range props {
			if child, ok := v.(map[string]any); ok {
				enrichPaginationNode(child)
			}
		}
	}

	if items, ok := node["items"].(map[string]any); ok {
		enrichPaginationNode(items)
	}

	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := node[key].([]any); ok {
			for _, v := range arr {
				if child, ok := v.(map[string]any); ok {
					enrichPaginationNode(child)
				}
			}
		}
	}
}

// isIntegerLike reports whether the schema node represents a numeric type
// (`integer`, `number`, or unset). Anything else (string, object, array)
// is excluded so the middleware never bounds a non-numeric field that
// happens to be named `page`/`per_page`.
func isIntegerLike(node map[string]any) bool {
	t, ok := node["type"]
	if !ok {
		return true
	}
	if s, ok := t.(string); ok {
		return s == "integer" || s == "number"
	}
	return false
}

// setIfAbsent assigns value to key only when the key is missing, so
// upstream schema authors keep authority over explicit constraints.
func setIfAbsent(node map[string]any, key string, value any) {
	if _, ok := node[key]; !ok {
		node[key] = value
	}
}
