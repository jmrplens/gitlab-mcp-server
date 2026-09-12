package toolutil

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NotFoundResult creates an informational MCP tool result for resources that
// do not exist or are not accessible. Instead of returning a Go error (which
// would be logged as ERROR and produce an opaque error message for the LLM),
// this returns a card with actionable next steps.
//
// The result has IsError=true to signal the tool could not fulfill the request,
// but the content is rich and helpful: the LLM can act on the suggestions. It
// is annotated for a detail, since it answers a get, and the identifier the
// caller passed is escaped on its way into the sentence, because it is text
// the caller typed and a get handler echoes it as it arrived.
//
// Use this in register.go handler closures when IsHTTPStatus(err, 404) is true
// for "get" operations. Pass nil error back to the SDK so the call is logged
// at INFO level instead of ERROR. The resource is the label the calling
// formatter passes as a constant, "User" or "Project Badge", never anything
// read from GitLab.
func NotFoundResult(resource, identifier string, hints ...string) *mcp.CallToolResult {
	var b strings.Builder
	c := NewCard(&b, EmojiQuestion+" "+resource+" Not Found")
	c.Note(fmt.Sprintf("The %s **%s** does not exist or is not accessible with your current permissions.", strings.ToLower(resource), EscapeMdTableCell(identifier)))
	c.End(hints...)
	return ErrorResultAnnotated(b.String(), ContentDetail)
}
