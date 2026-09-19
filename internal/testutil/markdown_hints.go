// markdown_hints.go collects the canonical action IDs a package's Markdown
// invites a model to call, by rendering the package's own formatters rather
// than by reading its constants: a hint spelled as a literal at the call site
// is published exactly like one behind a constant, and a test that reads only
// the constants cannot see it.
package testutil

import (
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintActionPattern matches the action ID inside the sentence
// [toolutil.HintAction] writes, which is the form a model reads it in.
var hintActionPattern = regexp.MustCompile(`Use action '([^']+)'`)

// HintedActionIDs renders every registered Markdown formatter whose output
// type is declared in pkgPath, driving each with a populated fixture, and
// returns the action IDs each rendering invites a model to call, keyed by the
// output type's name.
//
// A type whose rendering names no action is left out of the result rather than
// recorded empty, since plenty of formatters write no guidance at all; the
// caller decides whether finding nothing at all is a failure. The fixture is
// [FixtureMultiPage] because a list formatter writes its guidance only once it
// has rows, and the zero value would render an empty-collection message with
// no hints in it.
//
// Registration happens in each package's init, so the caller must already
// import whatever registers the formatters it expects to see.
func HintedActionIDs(tb testing.TB, pkgPath string) map[string][]string {
	tb.Helper()

	hinted := make(map[string][]string)
	for _, outputType := range toolutil.RegisteredMarkdownTypes() {
		if outputType.PkgPath() != pkgPath {
			continue
		}
		ids := hintedActionIDsForType(outputType)
		if len(ids) > 0 {
			hinted[outputType.Name()] = ids
		}
	}
	return hinted
}

// hintedActionIDsForType renders one output type and returns the action IDs
// its guidance names, sorted and deduplicated so a failure reads the same way
// on every run.
func hintedActionIDsForType(outputType reflect.Type) []string {
	result := toolutil.MarkdownForResult(FillFixture(outputType, FixtureOptions{State: FixtureMultiPage}).Interface())
	if result == nil {
		return nil
	}

	seen := make(map[string]struct{})
	var ids []string
	for _, match := range hintActionPattern.FindAllStringSubmatch(resultText(result), -1) {
		if _, dup := seen[match[1]]; dup {
			continue
		}
		seen[match[1]] = struct{}{}
		ids = append(ids, match[1])
	}
	sort.Strings(ids)
	return ids
}

// resultText joins the text blocks of a tool result, which is what a client
// shows and so the only place a hint can reach a model from.
func resultText(result *mcp.CallToolResult) string {
	var b strings.Builder
	for _, content := range result.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
			b.WriteString("\n")
		}
	}
	return b.String()
}
