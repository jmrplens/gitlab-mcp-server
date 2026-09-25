package toolutil

import (
	"context"
	"fmt"
	"net/http"
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
// is annotated for a detail, since it answers a get.
//
// Both halves of the sentence are contained here, each by the escaper its slot
// takes: the identifier because it is text the caller typed and a get handler
// echoes it as it arrived, and the label because it is not always the constant
// this once claimed it was. Two packages read it off the result instead, and
// they disagreed about whose job the escaping was: badges escaped it at its own
// call site and orbit passed it through raw, which is the shape a rule stated
// in a comment rather than in code always ends up in. The heading takes the
// same label through the card writer, which applies the heading escaper.
//
// Use this in register.go handler closures when IsHTTPStatus(err, 404) is true
// for "get" operations. Pass nil error back to the SDK so the call is logged
// at INFO level instead of ERROR.
func NotFoundResult(resource, identifier string, hints ...string) *mcp.CallToolResult {
	var b strings.Builder
	c := NewCard(&b, EmojiQuestion+" "+resource+" Not Found")
	c.Note(fmt.Sprintf("The %s **%s** does not exist or is not accessible with your current permissions.",
		EscapeMdTableCell(strings.ToLower(resource)), EscapeMdTableCell(identifier)))
	c.End(hints...)
	return ErrorResultAnnotated(b.String(), ContentDetail)
}

// ParamText renders a tool argument's value the way the caller wrote it, for
// the identifier a not-found result names. A route wrapper reads the arguments
// as they were decoded, before anything coerced them, so a JSON number arrives
// as a float64, and %v prints one of a million or more in exponent form: a
// package_id of 31234567 was named "3.1234567e+07" on the dynamic, meta and
// individual surfaces alike. A whole number is printed as the integer it is,
// and anything else as fmt prints it.
//
// It renders a value and never looks up a name: which key the value is read
// from is [ActionRoute.WrapNotFound]'s concern, which hands its builder the
// arguments with the documented aliases already resolved.
func ParamText(value any) string {
	if text, ok := numericIDString(value); ok {
		return text
	}
	return fmt.Sprint(value)
}

// WrapNotFound returns the route with GitLab's 404 answered by the value
// notFound builds from the call's arguments, in place of the error, now and on
// every later rebinding. The value is a typed not-found output whose formatter
// renders [NotFoundResult].
//
// notFound reads the arguments as the handler read them: with the documented
// parameter aliases resolved against the route's input type, which is what
// [UnmarshalParams] does before any handler runs. The meta surface hands a
// route the arguments as the caller spelled them and resolves the aliases only
// inside that call, on a copy, so a builder reading the map it was handed named
// a project given as project_path, or a milestone given as iid, as "<nil>",
// although the request GitLab refused was for exactly that project. The map
// notFound receives is a copy wherever anything was resolved, so the handler's
// own arguments are never changed.
func (route ActionRoute) WrapNotFound(notFound func(params map[string]any) any) ActionRoute {
	target := route.InputType
	return route.WrapHandler(func(next ActionFunc) ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && IsHTTPStatus(err, http.StatusNotFound) {
				return notFound(normalizeParamAliases(stripReservedKeys(input), target)), nil
			}
			return result, err
		}
	})
}
