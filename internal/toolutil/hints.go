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

// ListHints prepends HintPreserveLinks to list-result next-step hints.
func ListHints(hints ...string) []string {
	out := make([]string, 0, len(hints)+1)
	out = append(out, HintPreserveLinks)
	for _, hint := range hints {
		if hint == "" || hint == HintPreserveLinks {
			continue
		}
		out = append(out, hint)
	}
	return out
}

// The server's guidance section, and what that section becomes when it turns up
// in text the server did not write.
//
// next_steps is a trusted channel: the MCP instructions and the evaluator
// prompts both tell the model these are the server's own suggestions, and
// [ExtractHints] lifts them back out of the rendered Markdown into a structured
// field. Nothing distinguished a section the server wrote from one that arrived
// inside a README, a job log, a project description or a discussion note, so an
// attacker who typed the heading got their bullets promoted into next_steps
// verbatim — as the server's advice, on the server's authority.
//
// Defusing rewrites the glyph as its HTML entity. A Markdown client renders the
// two identically, so the reader loses nothing and sees the text that was
// really there; what changes is that the line is no longer the marker, so
// nothing downstream can mistake it for one.
const (
	hintsHeading        = "\U0001F4A1 **Next steps:**"
	defusedHintsHeading = "&#128161; **Next steps:**"
	// hintsBlockOpening is the whole opening of a server-authored section: the
	// horizontal rule, the heading, and the newline before the first bullet.
	hintsBlockOpening = "---\n" + hintsHeading + "\n"
)

// DefuseHintsHeading rewrites every copy of the server's guidance heading in s
// so it can no longer pass for one. Text with no such heading is returned
// unchanged.
func DefuseHintsHeading(s string) string {
	if !strings.Contains(s, hintsHeading) {
		return s
	}
	return strings.ReplaceAll(s, hintsHeading, defusedHintsHeading)
}

// WriteHints appends a "💡 Next steps" section to the Markdown builder.
// Each hint is a short string describing a related action the LLM can take
// (e.g. "Use action 'delete' to remove this package"). If no hints are
// provided, no section is written.
//
// Whether or not there are hints, any guidance heading already in the builder
// is defused first. This is the one place every formatter passes through on its
// way to a response — including the raw file, job trace, snippet and discussion
// renderers that embed GitLab bytes verbatim — so doing it here covers them all
// without each renderer having to remember, and covers the ones added later.
func WriteHints(b *strings.Builder, hints ...string) {
	defuseWrittenHints(b)
	if len(hints) == 0 {
		return
	}
	b.WriteString("\n" + hintsBlockOpening)
	for _, h := range hints {
		fmt.Fprintf(b, "- %s\n", StripControlBytes(h))
	}
}

// defuseWrittenHints rewrites the builder's contents when they already carry a
// guidance heading, which at this point can only have come from GitLab text a
// formatter embedded.
func defuseWrittenHints(b *strings.Builder) {
	written := b.String()
	if !strings.Contains(written, hintsHeading) {
		return
	}
	b.Reset()
	b.WriteString(DefuseHintsHeading(written))
}

// ExtractHints parses the "💡 Next steps" section from a Markdown tool
// response and returns the individual hint strings. Returns nil when
// the section is absent.
//
// Only a section in a position the server could have written it is read, and
// there are exactly two: [WriteHints] is called either on an empty builder, so
// the section opens the response (nineteen list formatters do this), or after
// the body, so it closes it. A section anywhere in between is content that
// happens to look like guidance — a README, a job log, a project description —
// and content does not get to speak as the server. This used to be the first
// match anywhere in the response, which is what let a file's contents fill
// next_steps.
func ExtractHints(md string) []string {
	if start, ok := leadingHintsBlock(md); ok {
		// Nothing precedes it, so it is the server's; the body follows the
		// bullets and ends them.
		return parseHintBullets(md[start:], false)
	}
	start := lastHintsBlock(md)
	if start < 0 {
		return nil
	}
	return parseHintBullets(md[start:], true)
}

// parseHintBullets reads the bullet list that follows a section opening. When
// mustEndResponse is set, anything after the bullets other than blank lines
// means the section did not close the response and is therefore not one the
// server appended.
func parseHintBullets(section string, mustEndResponse bool) []string {
	var hints []string
	for line := range strings.SplitSeq(section, "\n") {
		switch {
		case strings.HasPrefix(line, "- "):
			hints = append(hints, line[2:])
		case line == "":
			continue
		default:
			if mustEndResponse {
				return nil
			}
			if len(hints) == 0 {
				return nil
			}
			return hints
		}
	}
	if len(hints) == 0 {
		return nil
	}
	return hints
}

// leadingHintsBlock returns the offset just past a section opening the
// response, and whether there was one. The optional newline is what WriteHints
// writes before the rule when the builder it is handed is still empty.
func leadingHintsBlock(md string) (int, bool) {
	for _, prefix := range []string{hintsBlockOpening, "\n" + hintsBlockOpening} {
		if strings.HasPrefix(md, prefix) {
			return len(prefix), true
		}
	}
	return 0, false
}

// lastHintsBlock returns the offset just past the opening of the last
// server-authored guidance section, or -1 when the response carries none. The
// rule has to start a line: a "---" in the middle of one is not a rule, and a
// section the server wrote always follows the line it ended.
func lastHintsBlock(md string) int {
	for i := strings.LastIndex(md, hintsBlockOpening); i >= 0; i = strings.LastIndex(md[:i], hintsBlockOpening) {
		if i == 0 || md[i-1] == '\n' {
			return i + len(hintsBlockOpening)
		}
	}
	return -1
}
