package toolutil

import (
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mdTestOutput struct{ Name string }
type mdTestListOutput struct{ Count int }
type mdTestResultOutput struct{ URL string }
type mdUnregisteredOutput struct{}

func TestRegisterMarkdown_StringFormatter(t *testing.T) {
	// Clean state: register formatters locally by resetting the map.
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(v mdTestOutput) string {
		return "## " + v.Name
	})

	got := MarkdownForResult(mdTestOutput{Name: "hello"})
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	tc, ok := got.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", got.Content[0])
	}
	if tc.Text != "## hello" {
		t.Errorf("text = %q, want %q", tc.Text, "## hello")
	}
}

func TestRegisterMarkdownResult_ResultFormatter(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdownResult(func(v mdTestResultOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "custom: " + v.URL},
			},
		}
	})

	got := MarkdownForResult(mdTestResultOutput{URL: "https://example.com"})
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "custom: https://example.com" {
		t.Errorf("text = %q, want %q", tc.Text, "custom: https://example.com")
	}
}

func TestMarkdownForResult_NilReturnsSuccess(t *testing.T) {
	got := MarkdownForResult(nil)
	if got == nil {
		t.Fatal("nil input should return success result")
	}
}

func TestMarkdownForResult_UnknownTypeReturnsNil(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	got := MarkdownForResult(mdUnregisteredOutput{})
	if got != nil {
		t.Errorf("expected nil for unregistered type, got %v", got)
	}
}

func TestMarkdownForResult_EmptyStringReturnsNil(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(_ mdTestOutput) string { return "" })

	got := MarkdownForResult(mdTestOutput{Name: "empty"})
	if got != nil {
		t.Errorf("expected nil for empty markdown, got %v", got)
	}
}

func TestMarkdownForResult_ResultFormatterTakesPriority(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(_ mdTestOutput) string { return "string" })
	RegisterMarkdownResult(func(_ mdTestOutput) *mcp.CallToolResult {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "result"}},
		}
	})

	got := MarkdownForResult(mdTestOutput{Name: "both"})
	if got == nil {
		t.Fatal("expected non-nil")
	}
	tc := got.Content[0].(*mcp.TextContent)
	if tc.Text != "result" {
		t.Errorf("result formatter should take priority, got %q", tc.Text)
	}
}

func TestRegisterMarkdown_ConcurrentSafety(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			RegisterMarkdown(func(v mdTestListOutput) string {
				return "list"
			})
			_ = MarkdownForResult(mdTestListOutput{Count: n})
		}(i)
	}
	wg.Wait()
}

func TestStripTrailingLineWhitespace(t *testing.T) {
	input := "hello   \nworld\t\t\nok"
	want := "hello\nworld\nok"
	if got := stripTrailingLineWhitespace(input); got != want {
		t.Errorf("stripTrailingLineWhitespace = %q, want %q", got, want)
	}
}
