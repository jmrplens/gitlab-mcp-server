package toolutil

import (
	"reflect"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var (
	stringFormatters sync.Map // reflect.Type → func(any) string
	resultFormatters sync.Map // reflect.Type → func(any) *mcp.CallToolResult
)

func init() {
	RegisterMarkdown(formatDeleteOutput)
	RegisterMarkdown(formatVoidOutput)
}

// formatDeleteOutput renders a DeleteOutput confirmation as a success string.
func formatDeleteOutput(v DeleteOutput) string {
	return EmojiSuccess + " " + v.Message
}

// formatVoidOutput renders a VoidOutput confirmation as a success string.
func formatVoidOutput(v VoidOutput) string {
	return EmojiSuccess + " " + v.Message
}

// RegisterMarkdown registers a Markdown string formatter for type T.
// Subsequent calls to [MarkdownForResult] with a value of type T will
// invoke fn and wrap the returned string in a [mcp.CallToolResult].
func RegisterMarkdown[T any](fn func(T) string) {
	var zero T
	t := reflect.TypeOf(zero)
	stringFormatters.Store(t, func(v any) string {
		val, ok := v.(T)
		if !ok {
			return ""
		}
		return fn(val)
	})
}

// RegisterMarkdownPair registers two Markdown string formatters.
func RegisterMarkdownPair[A, B any](first func(A) string, second func(B) string) {
	RegisterMarkdown(first)
	RegisterMarkdown(second)
}

// RegisterMarkdownTriple registers three Markdown string formatters.
func RegisterMarkdownTriple[A, B, C any](first func(A) string, second func(B) string, third func(C) string) {
	RegisterMarkdown(first)
	RegisterMarkdown(second)
	RegisterMarkdown(third)
}

// RegisterMarkdownResult registers a result formatter for type T.
// Use this for types that need custom [mcp.CallToolResult] construction
// (e.g. uploads with image content).
func RegisterMarkdownResult[T any](fn func(T) *mcp.CallToolResult) {
	var zero T
	t := reflect.TypeOf(zero)
	resultFormatters.Store(t, func(v any) *mcp.CallToolResult {
		val, ok := v.(T)
		if !ok {
			return nil
		}
		return fn(val)
	})
}

// MarkdownForResult resolves a tool output to its Markdown [mcp.CallToolResult].
// Returns nil for nil input (caller should handle), returns nil when no
// formatter is registered for the concrete type.
func MarkdownForResult(result any) *mcp.CallToolResult {
	if result == nil {
		return SuccessResult("ok")
	}

	t := reflect.TypeOf(result)

	// Result formatters take priority (e.g. uploads with image content).
	if fn, ok := resultFormatters.Load(t); ok {
		if f, fOK := fn.(func(any) *mcp.CallToolResult); fOK {
			return f(result)
		}
	}

	if fn, ok := stringFormatters.Load(t); ok {
		if f, fOK := fn.(func(any) string); fOK {
			return wrapMarkdown(f(result))
		}
	}

	return nil
}

// wrapMarkdown converts a Markdown string into a CallToolResult with
// trailing whitespace stripped and assistant-only audience annotations.
func wrapMarkdown(md string) *mcp.CallToolResult {
	if md == "" {
		return nil
	}
	md = stripTrailingLineWhitespace(md)
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: md, Annotations: ContentAssistant},
		},
	}
}

// stripTrailingLineWhitespace removes trailing spaces and tabs from each line.
func stripTrailingLineWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

// RegisteredMarkdownTypeNames returns the type names of all registered
// Markdown formatters (both string and result variants). Used by validation
// tests to verify sub-packages self-register their formatters.
func RegisteredMarkdownTypeNames() []string {
	var names []string
	stringFormatters.Range(func(key, _ any) bool {
		if t, ok := key.(reflect.Type); ok {
			names = append(names, t.String())
		}
		return true
	})
	resultFormatters.Range(func(key, _ any) bool {
		if t, ok := key.(reflect.Type); ok {
			names = append(names, t.String())
		}
		return true
	})
	return names
}

// HasRegisteredMarkdownFormatter reports whether a Markdown formatter
// has been registered for the concrete type of v. v may be a value or
// a pointer; pointer values are dereferenced to their element type for
// the registry lookup. Returns false for nil and for unregistered types.
//
// This is the authoritative source for "is there a Markdown formatter
// for this output type?" queries from external tools (e.g.
// cmd/audit_discovery_completeness uses it to gate the
// `missing_next_steps` check).
func HasRegisteredMarkdownFormatter(v any) bool {
	if v == nil {
		return false
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return false
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if _, ok := stringFormatters.Load(t); ok {
		return true
	}
	if _, ok := resultFormatters.Load(t); ok {
		return true
	}
	return false
}

// MarkdownFormatterCount returns the number of registered Markdown
// formatters (string + result variants). Useful for sanity checks in
// tests and tools that need to know "are formatters even loaded?".
func MarkdownFormatterCount() int {
	n := 0
	stringFormatters.Range(func(_, _ any) bool { n++; return true })
	resultFormatters.Range(func(_, _ any) bool { n++; return true })
	return n
}
