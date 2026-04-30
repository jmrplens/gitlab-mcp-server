// hints.go provides next-action hint helpers for MCP tool responses.
// Hints guide LLMs toward related operations they can perform next,
// improving discoverability of available tools and common workflows.
package toolutil

import (
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HintableOutput is an embeddable struct that adds a next_steps field to any
// Output type. Embed it as the FIRST field of an Output struct so that
// next_steps appears first in the serialized JSON, giving LLMs immediate
// guidance before reading the rest of the payload.
//
//	type Output struct {
//	    toolutil.HintableOutput
//	    Name string `json:"name"`
//	}
type HintableOutput struct {
	NextSteps []string `json:"next_steps,omitempty"`
}

// HintSetter is implemented by any Output struct that embeds HintableOutput.
// PopulateHints uses this interface to set extracted hints on the output.
type HintSetter interface {
	// SetNextSteps stores the extracted next-step hints on the output struct.
	SetNextSteps(hints []string)
}

// SetNextSteps stores the given hints in the NextSteps field.
func (h *HintableOutput) SetNextSteps(hints []string) {
	h.NextSteps = hints
}

// PopulateHints extracts next-step hints from the Markdown content of a
// CallToolResult and sets them on the output struct. It is a no-op when
// result is nil, contains no TextContent, or has no hints section.
func PopulateHints(result *mcp.CallToolResult, setter HintSetter) {
	if result == nil || setter == nil {
		return
	}
	for _, c := range result.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			continue
		}
		if hints := ExtractHints(tc.Text); len(hints) > 0 {
			setter.SetNextSteps(hints)
			return
		}
	}
}

// WithHints extracts hints from a CallToolResult and populates them on the
// typed output struct, returning all three handler values in one call.
// This avoids evaluation-order ambiguity in multi-value return statements.
//
// For value Out types (the common case), &out is used internally to satisfy
// the HintSetter pointer receiver. For pointer Out types (*T), the pointer
// itself implements HintSetter. If neither case applies, WithHints is a no-op.
//
//	return toolutil.WithHints(toolutil.ToolResultWithMarkdown(md), out, err)
func WithHints[O any](result *mcp.CallToolResult, out O, err error) (*mcp.CallToolResult, O, error) {
	if err != nil {
		return result, out, err
	}
	// Value Out types: &out is *T which implements HintSetter via embedded *HintableOutput.
	if setter, ok := any(&out).(HintSetter); ok {
		PopulateHints(result, setter)
	} else if ptrSetter, ptrOk := any(out).(HintSetter); ptrOk {
		// Pointer Out types: out is already *T which implements HintSetter directly.
		PopulateHints(result, ptrSetter)
	}
	return result, out, err
}

// HintPreserveLinks reminds the LLM to keep the clickable [text](url)
// markdown links when presenting list results to the user.
const HintPreserveLinks = "When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab"

// WriteHints appends a "💡 Next steps" section to the Markdown builder.
// Each hint is a short string describing a related action the LLM can take
// (e.g. "Use action 'delete' to remove this package").
// If no hints are provided, nothing is written.
func WriteHints(b *strings.Builder, hints ...string) {
	if len(hints) == 0 {
		return
	}
	b.WriteString("\n---\n💡 **Next steps:**\n")
	for _, h := range hints {
		fmt.Fprintf(b, "- %s\n", h)
	}
}

// ExtractHints parses the "💡 Next steps" section from a Markdown tool
// response and returns the individual hint strings. Returns nil when
// the section is absent.
func ExtractHints(md string) []string {
	const marker = "💡 **Next steps:**\n"
	_, after, ok := strings.Cut(md, marker)
	if !ok {
		return nil
	}
	section := after
	var hints []string
	for line := range strings.SplitSeq(section, "\n") {
		if strings.HasPrefix(line, "- ") {
			hints = append(hints, line[2:])
		} else if line == "" {
			continue
		} else {
			break
		}
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}
