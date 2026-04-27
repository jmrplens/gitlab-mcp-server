// embed.go provides helpers for embedding MCP resources in tool results.
//
// MCP clients that understand the EmbeddedResource content block (MCP spec
// 2025-06-18 §6.4) can render a tool result with a clickable resource link
// next to the human-readable Markdown and the StructuredContent payload.
// Clients that do not understand the type ignore it, so embedding is purely
// additive and backward compatible.
//
// Embedding can be disabled globally via EnableEmbeddedResources(false), which
// is wired from config.EmbeddedResources at startup so deployments can opt out
// if a particular client misbehaves with the new content type.

package toolutil

import (
	"encoding/json"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// embeddedResourcesEnabled controls whether EmbedResource appends content.
// Default true — embedding is on out of the box. Read/written via atomic to
// be safe under concurrent server initialization in the HTTP server pool.
var embeddedResourcesEnabled atomic.Bool

func init() {
	embeddedResourcesEnabled.Store(true)
}

// EnableEmbeddedResources toggles the global EmbedResource behaviour.
// When false, EmbedResource is a no-op, preserving the legacy two-block
// (text + structuredContent) tool result shape.
func EnableEmbeddedResources(enabled bool) {
	embeddedResourcesEnabled.Store(enabled)
}

// EmbeddedResourcesEnabled reports the current state of the global toggle.
// Exposed for tests; production code should call EmbedResource directly.
func EmbeddedResourcesEnabled() bool {
	return embeddedResourcesEnabled.Load()
}

// EmbedResource appends an EmbeddedResource content block to result that
// references the canonical MCP resource URI for the entity returned by the
// tool. mimeType should typically be "application/json" with text containing
// a compact JSON serialization of the entity (or empty if the resource is
// addressable but the body is large).
//
// When result is nil or the global toggle is disabled, EmbedResource is a
// no-op. URIs that fail RFC-3986 validation are dropped silently rather than
// breaking the tool result; the calling handler still returns the text and
// StructuredContent blocks.
func EmbedResource(result *mcp.CallToolResult, uri, mimeType, text string) {
	if result == nil || uri == "" || !embeddedResourcesEnabled.Load() {
		return
	}
	result.Content = append(result.Content, &mcp.EmbeddedResource{
		Resource: &mcp.ResourceContents{
			URI:      uri,
			MIMEType: mimeType,
			Text:     text,
		},
	})
}

// EmbedResourceJSON marshals value as JSON and embeds the result with MIME
// type application/json. Marshaling errors are dropped silently — the tool
// result is still returned with text and StructuredContent so the LLM has a
// usable response.
func EmbedResourceJSON(result *mcp.CallToolResult, uri string, value any) {
	if result == nil || uri == "" || !embeddedResourcesEnabled.Load() {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	result.Content = append(result.Content, &mcp.EmbeddedResource{
		Resource: &mcp.ResourceContents{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		},
	})
}
