// mdregistry_test.go contains unit tests for the type-based Markdown formatter
// registry: RegisterMarkdown, RegisterMarkdownResult, MarkdownForResult dispatch,
// result formatter priority over string formatters, concurrent-safety of
// RegisterMarkdown, and stripTrailingLineWhitespace.
package toolutil

import (
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mdTestOutput is a test-only type registered with RegisterMarkdown
// to verify string formatter dispatch.
type mdTestOutput struct{ Name string }

// mdTestListOutput is a test-only type used to verify concurrent-safe
// registration and lookup of string formatters.
type mdTestListOutput struct{ Count int }

// mdTestResultOutput is a test-only type registered with RegisterMarkdownResult
// to verify result formatter dispatch.
type mdTestResultOutput struct{ URL string }

// mdUnregisteredOutput is a test-only type that is intentionally never
// registered, used to verify that MarkdownForResult returns nil for
// unknown types.
type mdUnregisteredOutput struct{}

// TestRegisterMarkdown_StringFormatter verifies that RegisterMarkdown stores
// a string formatter and that MarkdownForResult correctly invokes it and wraps
// the returned string in a TextContent [mcp.CallToolResult]. The test resets
// the registry to a clean state before registering, then asserts the output
// text matches the expected "## hello" value.
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

// TestRegisterMarkdownResult_ResultFormatter verifies that RegisterMarkdownResult
// stores a custom result formatter and that MarkdownForResult returns the
// formatter's output unchanged. The test resets the registry, registers a
// formatter that prepends "custom: " to the URL, and asserts the content text
// matches the expected value.
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

// TestMarkdownForResult_NilReturnsSuccess verifies that MarkdownForResult
// returns a non-nil success result when called with a nil input, so callers
// do not need to nil-guard the return value for the nil case.
func TestMarkdownForResult_NilReturnsSuccess(t *testing.T) {
	got := MarkdownForResult(nil)
	if got == nil {
		t.Fatal("nil input should return success result")
	}
}

// TestMarkdownForResult_UnknownTypeReturnsNil verifies that MarkdownForResult
// returns nil when no formatter is registered for the concrete type of the
// input value. The test resets the registry before the assertion to guarantee
// a clean state.
func TestMarkdownForResult_UnknownTypeReturnsNil(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	got := MarkdownForResult(mdUnregisteredOutput{})
	if got != nil {
		t.Errorf("expected nil for unregistered type, got %v", got)
	}
}

// TestMarkdownForResult_EmptyStringReturnsNil verifies that MarkdownForResult
// returns nil when a registered string formatter returns an empty string,
// signaling that the caller should fall back to a default representation.
func TestMarkdownForResult_EmptyStringReturnsNil(t *testing.T) {
	stringFormatters = sync.Map{}
	resultFormatters = sync.Map{}

	RegisterMarkdown(func(_ mdTestOutput) string { return "" })

	got := MarkdownForResult(mdTestOutput{Name: "empty"})
	if got != nil {
		t.Errorf("expected nil for empty markdown, got %v", got)
	}
}

// TestMarkdownForResult_ResultFormatterTakesPriority verifies that when both a
// string formatter and a result formatter are registered for the same type,
// MarkdownForResult uses the result formatter and ignores the string formatter.
// The test asserts that the content text is "result" (from the result formatter)
// rather than "string" (from the string formatter).
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

// TestRegisterMarkdown_ConcurrentSafety verifies that concurrent calls to
// RegisterMarkdown and MarkdownForResult on the same type do not cause data
// races or panics. The test launches 100 goroutines that each register a
// formatter and immediately invoke MarkdownForResult, relying on the Go
// race detector to surface any unsafe concurrent access.
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

// TestStripTrailingLineWhitespace verifies that stripTrailingLineWhitespace
// removes trailing spaces and tabs from each line without affecting the line
// content itself. The test asserts that trailing whitespace is stripped from
// lines with mixed whitespace characters while the non-whitespace content
// and newline structure are preserved.
func TestStripTrailingLineWhitespace(t *testing.T) {
	input := "hello   \nworld\t\t\nok"
	want := "hello\nworld\nok"
	if got := stripTrailingLineWhitespace(input); got != want {
		t.Errorf("stripTrailingLineWhitespace = %q, want %q", got, want)
	}
}

// TestRegisteredMarkdownTypeNames_ReturnsRegisteredTypes verifies that
// RegisteredMarkdownTypeNames returns names for both string and result
// formatters that have been registered.
func TestRegisteredMarkdownTypeNames_ReturnsRegisteredTypes(t *testing.T) {
	RegisterMarkdown(func(_ mdTestOutput) string { return "s" })
	RegisterMarkdownResult(func(_ mdTestResultOutput) *mcp.CallToolResult { return nil })

	names := RegisteredMarkdownTypeNames()
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["toolutil.mdTestOutput"] {
		t.Errorf("expected mdTestOutput in registered types, got %v", names)
	}
	if !found["toolutil.mdTestResultOutput"] {
		t.Errorf("expected mdTestResultOutput in registered types, got %v", names)
	}
}

// TestMarkdownForResult_DeleteOutputViaInit verifies that the init()
// function in mdregistry.go registers the DeleteOutput formatter correctly
// and that it produces the expected success emoji + message output.
func TestMarkdownForResult_DeleteOutputViaInit(t *testing.T) {
	// Re-register: earlier tests in this file reset global maps, wiping init() state.
	RegisterMarkdown(func(v DeleteOutput) string {
		return EmojiSuccess + " " + v.Message
	})

	result := MarkdownForResult(DeleteOutput{Message: "Project deleted"})
	if result == nil {
		t.Fatal("expected non-nil result for DeleteOutput")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	want := EmojiSuccess + " Project deleted"
	if tc.Text != want {
		t.Errorf("text = %q, want %q", tc.Text, want)
	}
}

func TestMarkdownForResult_VoidOutputViaInit(t *testing.T) {
	// Re-register: earlier tests in this file reset global maps, wiping init() state.
	RegisterMarkdown(func(v VoidOutput) string {
		return EmojiSuccess + " " + v.Message
	})

	result := MarkdownForResult(VoidOutput{Message: "Action completed"})
	if result == nil {
		t.Fatal("expected non-nil result for VoidOutput")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	want := EmojiSuccess + " Action completed"
	if tc.Text != want {
		t.Errorf("text = %q, want %q", tc.Text, want)
	}
}

func TestFormatDeleteOutput_ReturnsEmojiPlusMessage(t *testing.T) {
	t.Parallel()
	got := formatDeleteOutput(DeleteOutput{Status: "success", Message: "branch deleted"})
	want := EmojiSuccess + " branch deleted"
	if got != want {
		t.Fatalf("formatDeleteOutput = %q, want %q", got, want)
	}
}

func TestFormatVoidOutput_ReturnsEmojiPlusMessage(t *testing.T) {
	t.Parallel()
	got := formatVoidOutput(VoidOutput{Status: "success", Message: "action completed"})
	want := EmojiSuccess + " action completed"
	if got != want {
		t.Fatalf("formatVoidOutput = %q, want %q", got, want)
	}
}
