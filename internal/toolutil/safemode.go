// safemode.go carries the shared Safe Mode preview contract used by every
// tool surface: individual tool wrapping, meta-tool dispatch, and the dynamic
// execute tool.
package toolutil

import (
	"context"
	"encoding/json"
	"log/slog"
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
const SafeModeHint = "Set GITLAB_SAFE_MODE=false to execute this operation"

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
