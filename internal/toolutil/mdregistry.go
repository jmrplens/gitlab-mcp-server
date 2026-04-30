// mdregistry.go provides a type-based registry for Markdown formatters.
// Sub-packages self-register their formatters via init(), and the central
// dispatcher resolves any output struct to its Markdown representation
// with a single map lookup instead of a giant type-switch.
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
	RegisterMarkdown(func(v DeleteOutput) string {
		return EmojiSuccess + " " + v.Message
	})
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
