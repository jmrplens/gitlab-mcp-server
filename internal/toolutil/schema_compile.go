package toolutil

import (
	"encoding/json"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// compiledSchemaCache maps a caller-supplied stable cache key to the
// *jsonschema.Schema compiled from a projected tool's map schema. Keys must
// uniquely identify the schema content (surface + tool + tier); the schema
// projection pipeline is a pure function of those inputs.
var compiledSchemaCache sync.Map

// CompileToolSchemas replaces a projected tool's map-based input and output
// schemas with process-cached *jsonschema.Schema equivalents. Passing the
// SDK a stable schema pointer instead of a map removes the per-registration
// JSON remarshal inside mcp.AddTool and lets a shared [mcp.SchemaCache]
// (keyed by pointer identity) skip schema resolution on subsequent
// registrations — the dominant cost when the HTTP server pool builds a new
// MCP server per token.
//
// The conversion is a faithful roundtrip: the maps are produced by
// marshaling jsonschema output, and jsonschema.Schema preserves non-standard
// keys such as x_destructive. On any marshal error the original map is kept,
// preserving current behavior. An empty cacheKey disables compilation, so
// callers that cannot guarantee a content-stable key keep the map path.
func CompileToolSchemas(tool *mcp.Tool, cacheKey string) {
	if tool == nil || cacheKey == "" {
		return
	}
	if m, ok := tool.InputSchema.(map[string]any); ok {
		tool.InputSchema = compiledSchema(cacheKey+"|in", m)
	}
	if m, ok := tool.OutputSchema.(map[string]any); ok {
		tool.OutputSchema = compiledSchema(cacheKey+"|out", m)
	}
}

// compiledSchema returns the cached compiled schema for key, converting and
// caching the map form on first use. It falls back to the original map when
// the conversion fails so registration behavior never changes on error.
func compiledSchema(key string, schema map[string]any) any {
	if cached, ok := compiledSchemaCache.Load(key); ok {
		return cached
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	compiled := new(jsonschema.Schema)
	if unmarshalErr := json.Unmarshal(data, compiled); unmarshalErr != nil {
		return schema
	}
	actual, _ := compiledSchemaCache.LoadOrStore(key, compiled)
	return actual
}
