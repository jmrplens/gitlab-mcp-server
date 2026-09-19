// markdown_hints_test.go exercises the hint collector against formatters this
// file registers itself, so the expectations are fixed by the test rather than
// by whatever the domain packages happen to publish today.
package hints

import (
	"reflect"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintsFixturePkgPath is this package, which is where the output types below
// are declared and so the path HintedActionIDs is asked for.
const hintsFixturePkgPath = "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/hints"

// hintsListOutput stands in for a list output whose guidance names actions, one
// of them twice so the deduplication is exercised and both out of order so the
// sorting is.
type hintsListOutput struct {
	Items []hintsItem `json:"items"`
}

// hintsItem is the row of hintsListOutput, present so FillFixture has a slice
// element to populate and the formatter has rows to render.
type hintsItem struct {
	Name string `json:"name"`
}

// hintsSilentOutput stands in for the many formatters that write no guidance at
// all.
type hintsSilentOutput struct {
	Name string `json:"name"`
}

// hintsEmptyOutput stands in for a formatter that renders nothing, which the
// registry turns into a nil result rather than an empty one.
type hintsEmptyOutput struct {
	Name string `json:"name"`
}

// hintsMixedContentOutput stands in for a formatter that returns content blocks
// of its own, only some of which are text.
type hintsMixedContentOutput struct {
	Name string `json:"name"`
}

func init() {
	toolutil.RegisterMarkdown(func(hintsListOutput) string {
		return "rows\n\n" +
			toolutil.HintAction("issue.list", "see the issues") + "\n" +
			toolutil.HintAction("branch.create", "start a branch") + "\n" +
			toolutil.HintAction("issue.list", "see them again") + "\n"
	})
	toolutil.RegisterMarkdown(func(hintsSilentOutput) string { return "nothing to suggest\n" })
	toolutil.RegisterMarkdown(func(hintsEmptyOutput) string { return "" })
	toolutil.RegisterMarkdownResult(func(hintsMixedContentOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.ImageContent{MIMEType: "image/png", Data: []byte{0}},
			&mcp.TextContent{Text: toolutil.HintAction("project.get", "read the project")},
		}}
	})
}

// TestHintedActionIDs_NamedActions_AreReturnedSortedAndDeduplicated drives the
// collector over this package's own formatters and checks the one that names
// actions reports them once each and in a fixed order, so a failure elsewhere
// reads the same way on every run.
func TestHintedActionIDs_NamedActions_AreReturnedSortedAndDeduplicated(t *testing.T) {
	hinted := HintedActionIDs(t, hintsFixturePkgPath)

	want := []string{"branch.create", "issue.list"}
	if got := hinted["hintsListOutput"]; !reflect.DeepEqual(got, want) {
		t.Errorf("HintedActionIDs()[hintsListOutput] = %v, want %v", got, want)
	}
}

// TestHintedActionIDs_MixedContent_ReadsTheTextBlocks checks that a formatter
// returning a result of its own is read through its text blocks, since a hint
// can only reach a model from one of those.
func TestHintedActionIDs_MixedContent_ReadsTheTextBlocks(t *testing.T) {
	hinted := HintedActionIDs(t, hintsFixturePkgPath)

	want := []string{"project.get"}
	if got := hinted["hintsMixedContentOutput"]; !reflect.DeepEqual(got, want) {
		t.Errorf("HintedActionIDs()[hintsMixedContentOutput] = %v, want %v", got, want)
	}
}

// TestHintedActionIDs_FormattersNamingNothing_AreLeftOut checks that a
// formatter writing no guidance, and one rendering nothing at all, are absent
// from the result rather than recorded empty: plenty of formatters write no
// guidance, and the caller decides whether that matters.
func TestHintedActionIDs_FormattersNamingNothing_AreLeftOut(t *testing.T) {
	hinted := HintedActionIDs(t, hintsFixturePkgPath)

	for _, name := range []string{"hintsSilentOutput", "hintsEmptyOutput"} {
		t.Run(name, func(t *testing.T) {
			if ids, ok := hinted[name]; ok {
				t.Errorf("HintedActionIDs() reported %s = %v, want it left out", name, ids)
			}
		})
	}
}

// TestHintedActionIDs_AnotherPackage_ReportsNothing checks the path filter:
// every formatter of every domain is registered in one process, so a collector
// that ignored the path would answer for all of them.
func TestHintedActionIDs_AnotherPackage_ReportsNothing(t *testing.T) {
	if hinted := HintedActionIDs(t, hintsFixturePkgPath+"/nowhere"); len(hinted) != 0 {
		t.Errorf("HintedActionIDs(other package) = %v, want nothing", hinted)
	}
}

// TestHintedActionIDsForType_UnregisteredType_ReportsNothing checks the branch
// where the registry has no formatter for the type, which renders a nil result
// with no text to read.
func TestHintedActionIDsForType_UnregisteredType_ReportsNothing(t *testing.T) {
	type unregisteredOutput struct{ Name string }

	if ids := hintedActionIDsForType(reflect.TypeFor[unregisteredOutput]()); ids != nil {
		t.Errorf("hintedActionIDsForType(unregistered) = %v, want nil", ids)
	}
}
