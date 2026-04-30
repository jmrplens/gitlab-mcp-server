// not_found.go provides a structured result builder for resources that do
// not exist or are inaccessible. It returns an informational MCP result with
// actionable hints instead of propagating a raw 404 error.
package toolutil

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NotFoundResult creates an informational MCP tool result for resources that
// do not exist or are not accessible. Instead of returning a Go error (which
// would be logged as ERROR and produce an opaque error message for the LLM),
// this returns a structured Markdown result with actionable next steps.
//
// The result has IsError=true to signal the tool could not fulfill the request,
// but the content is rich and helpful — the LLM can act on the suggestions.
//
// Use this in register.go handler closures when IsHTTPStatus(err, 404) is true
// for "get" operations. Pass nil error back to the SDK so the call is logged
// at INFO level instead of ERROR.
func NotFoundResult(resource, identifier string, hints ...string) *mcp.CallToolResult {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s %s Not Found\n\n", EmojiQuestion, resource)
	fmt.Fprintf(&b, "The %s **%s** does not exist or is not accessible with your current permissions.\n\n", strings.ToLower(resource), identifier)
	WriteHints(&b, hints...)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: b.String()},
		},
		IsError: true,
	}
}
