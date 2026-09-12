// safe_mode.go carries the shared Safe Mode preview contract used by every
// tool surface: individual tool wrapping, meta-tool dispatch, and the dynamic
// execute tool.

package toolutil

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// SafeModePreview is the structured response returned when a mutating
// operation is intercepted by Safe Mode. Status is always "blocked", Mode is
// "safe", Tool names the intercepted tool or canonical action, Params mirrors
// the would-be call arguments, and Hint tells the operator how to disable safe
// mode.
type SafeModePreview struct {
	Status string          `json:"status"`
	Mode   string          `json:"mode"`
	Tool   string          `json:"tool"`
	Params json.RawMessage `json:"params"`
	Hint   string          `json:"hint"`
}

// SafeModeHint is the operator-facing hint attached to every safe-mode preview.
const SafeModeHint = "Set GITLAB_MCP_SAFE_MODE=false to execute this operation"

// NewSafeModePreview builds a preview for name, marshaling params defensively:
// when params cannot be marshaled the preview still reports the blocked
// operation with a null params payload rather than failing the call.
func NewSafeModePreview(name string, params any) SafeModePreview {
	encoded, err := json.Marshal(params)
	if err != nil {
		slog.Warn("safe mode: failed to marshal preview params", "tool", name, "error", err)
		encoded = []byte("null")
	}
	return SafeModePreview{
		Status: "blocked",
		Mode:   "safe",
		Tool:   name,
		Params: encoded,
		Hint:   SafeModeHint,
	}
}

// SafeModeActionFunc returns an [ActionFunc] that returns a [SafeModePreview]
// for name instead of executing anything. It is used to neutralize mutating
// catalog actions at registration time, so dispatcher surfaces (meta-tools and
// the dynamic execute tool) preview each mutating action individually while
// their read-only actions keep executing.
func SafeModeActionFunc(name string) ActionFunc {
	return func(ctx context.Context, params map[string]any) (any, error) {
		// The context was discarded here, and with it the only evidence that
		// safe mode did anything. A preview is a successful result, so the span
		// and the log stream both showed an ordinary tool call that worked: an
		// operator could not tell a deployment in safe mode from one doing real
		// work, nor count how often it intercepted, which is the first question
		// anybody asks after turning it on.
		LogToolRefusal(ctx, nil, name, RefusalSafeMode)
		return NewSafeModePreview(name, params), nil
	}
}

// The rows and the heading a preview card is made of, shared by the writer
// and by [ParseSafeModePreview], which reads the card back.
const (
	safeModeHeadingPrefix = EmojiStop + " Safe mode blocked "
	safeModeStatusRow     = "- **Status**: "
	safeModeModeRow       = "- **Mode**: "
	safeModeToolRow       = "- **Tool**: "
	safeModeParamsTitle   = "Parameters"
)

// FormatSafeModePreviewMarkdown renders a preview as a card, the way every
// other refusal is rendered: the intercepted tool in the heading, the status
// and mode rows, the tool as a code span, the would-be arguments in a JSON
// fence, and the operator's hint last. It is the registered formatter for
// [SafeModePreview], so the dispatcher surfaces render the preview through
// it, and [SafeModePreviewResult] is what the individual surface answers
// with.
func FormatSafeModePreviewMarkdown(p SafeModePreview) string {
	var b strings.Builder
	c := NewCard(&b, safeModeHeadingPrefix+p.Tool)
	c.Field("Status", p.Status)
	c.Field("Mode", p.Mode)
	c.Code("Tool", p.Tool)
	c.Fence(safeModeParamsTitle, "json", string(p.Params))
	if p.Hint == "" {
		c.End()
	} else {
		c.End(p.Hint)
	}
	return b.String()
}

// SafeModePreviewResult renders a preview as the error result the individual
// surface answers an intercepted call with: the tool did not run, so it
// produced none of the output its schema describes, and the specification is
// unconditional about what a declared schema obliges.
func SafeModePreviewResult(p SafeModePreview) *mcp.CallToolResult {
	return ErrorResultAnnotated(FormatSafeModePreviewMarkdown(p), ContentMutate)
}

// ParseSafeModePreview reads a preview back out of the card
// [FormatSafeModePreviewMarkdown] wrote, and reports whether the text is one:
// a card whose status row says blocked and whose mode row says safe. It is
// the one reader a test or an evaluator needs, so that none of them parses
// the card by hand.
func ParseSafeModePreview(text string) (SafeModePreview, bool) {
	var p SafeModePreview
	var params []string
	inFence := false
	for line := range strings.SplitSeq(text, "\n") {
		switch {
		case strings.HasPrefix(line, "```"):
			inFence = !inFence
		case inFence:
			params = append(params, line)
		case strings.HasPrefix(line, safeModeStatusRow):
			p.Status = strings.TrimPrefix(line, safeModeStatusRow)
		case strings.HasPrefix(line, safeModeModeRow):
			p.Mode = strings.TrimPrefix(line, safeModeModeRow)
		case strings.HasPrefix(line, safeModeToolRow):
			p.Tool = strings.Trim(strings.TrimPrefix(line, safeModeToolRow), "` ")
		}
	}
	if len(params) > 0 {
		p.Params = json.RawMessage(strings.Join(params, "\n"))
	}
	if hints := ExtractHints(text); len(hints) > 0 {
		p.Hint = hints[0]
	}
	return p, p.Status == "blocked" && p.Mode == "safe"
}

func init() {
	RegisterMarkdown(FormatSafeModePreviewMarkdown)
}
