package toolutil

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// FineGrainedRefusal is the one check every dispatcher makes before it does
// anything on an action's behalf: it answers the call with the reason when a
// fine-grained session may not run the action, and nil when it may.
//
// It reads the authority of the client bound to the request
// ([gitlabclient.AuthorityFrom]), which only a fine-grained credential's
// client carries, so a classic session returns at once and pays nothing.
// What it decides comes from the authority's Decide and what it says from its
// WithheldText (internal/finegrained), so every surface refuses in the
// same words, which name the action by its canonical ID and never a tool.
//
// The dispatchers call it before their own work: in the meta handler right
// after the action is resolved and its arguments validated, before a
// destructive action is confirmed and before the route's safe-mode preview; in
// the individual handler before safe mode; in dynamic execute right after the
// action is resolved; and in the standalone handler first. A withheld write is
// therefore never offered for confirmation nor previewed as something this
// token could do. The receiving middleware answers most calls before the SDK
// validates their arguments, and this is the defense behind it.
//
// prefix is prepended to the refusal: the dynamic surface names its own tool
// there, as every refusal it writes does, and every other caller passes "".
// callName is what the refusal is logged under. The refusal is logged at INFO
// with the reason class [RefusalFineGrained], and the action and its cause at
// DEBUG, never the token or anything of its grant.
func FineGrainedRefusal(ctx context.Context, req *mcp.CallToolRequest, callName, actionID, prefix string) *mcp.CallToolResult {
	authority := gitlabclient.AuthorityFrom(ctx)
	if authority == nil || actionID == "" {
		return nil
	}
	decision := authority.Decide(actionID)
	if decision.Callable {
		return nil
	}
	slog.DebugContext(ctx, "fine-grained session withheld an action", "action", actionID, "cause", string(decision.Cause))
	LogToolRefusal(ctx, req, callName, RefusalFineGrained)
	return ErrorResult(prefix + authority.WithheldText(actionID, decision))
}

// FineGrainedNotes adds to an answer a fine-grained session was served the
// next steps that say what GitLab did to it without saying so, and returns
// it. A classic session's answer is returned as it came.
//
// Three notes, each worded by the authority (internal/finegrained) so every
// surface words them alike: the parts of the answer GitLab leaves empty for
// this credential; on a
// not-found answer of an action that reads GraphQL, that GitLab answers null
// for an object the token cannot see; and on an empty list of an action whose
// answer is a GraphQL list or connection, that GitLab leaves out the items the
// token cannot see. They are written into the answer's own next steps, so the
// tail every dispatcher applies afterwards ([FinishToolResult]) carries them
// into next_steps as well.
//
// A safe-mode preview gets none: it is the answer to a call nothing sent to
// GitLab, so there is no answer of GitLab's for a note to describe. The meta
// and dynamic surfaces reach here with one, since safe mode replaces a
// write's route there ([SafeModeActionFunc]); the individual surface answers
// with its preview before it would.
func FineGrainedNotes(ctx context.Context, actionID string, callResult *mcp.CallToolResult, result any) *mcp.CallToolResult {
	authority := gitlabclient.AuthorityFrom(ctx)
	if authority == nil || actionID == "" || callResult == nil {
		return callResult
	}
	if _, preview := result.(SafeModePreview); preview {
		return callResult
	}
	var notes []string
	if callResult.IsError {
		if mentionsNotFound(resultText(callResult)) {
			notes = append(notes, authority.NullNote(actionID))
		}
	} else {
		notes = append(notes, authority.DegradedNote(authority.Decide(actionID)))
		if emptyCollection(result) {
			notes = append(notes, authority.EmptyNote(actionID))
		}
	}
	appendResultHints(callResult, notes...)
	return callResult
}

// FineGrainedErrorNote is [FineGrainedNotes] for a handler that reported a
// not-found answer as a Go error, which is how most GraphQL handlers report a
// null: the error comes back carrying the note after its own message, and
// still unwraps to the error it was.
func FineGrainedErrorNote(ctx context.Context, actionID string, err error) error {
	authority := gitlabclient.AuthorityFrom(ctx)
	if err == nil || authority == nil || !mentionsNotFound(err.Error()) {
		return err
	}
	note := authority.NullNote(actionID)
	if note == "" {
		return err
	}
	return &notedError{err: err, note: note}
}

// notedError is an error carrying a note for the reader after its message.
type notedError struct {
	err  error
	note string
}

// Error is the wrapped message followed by the note.
func (e *notedError) Error() string { return e.err.Error() + ". " + e.note }

// Unwrap returns the error the note was added to.
func (e *notedError) Unwrap() error { return e.err }

// mentionsNotFound reports whether an answer's words say what it asked for
// was not found, which is how this server words a GraphQL null whatever the
// domain.
func mentionsNotFound(text string) bool {
	return strings.Contains(strings.ToLower(text), "not found")
}

// resultText is the text of an answer's first text block, or "".
func resultText(callResult *mcp.CallToolResult) string {
	for _, c := range callResult.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			return text.Text
		}
	}
	return ""
}

// emptyCollection reports whether a handler's output is an empty list: a slice
// with nothing in it, or a struct whose one exported slice field is empty,
// which is the shape every list output here has (its pagination and next steps
// are not slices of their own). A struct with two lists, or none, is not one.
func emptyCollection(result any) bool {
	value := reflect.ValueOf(result)
	for value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface {
		if value.IsNil() {
			return false
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Slice:
		return value.Len() == 0
	case reflect.Struct:
		var lists []reflect.Value
		for i := range value.NumField() {
			field := value.Type().Field(i)
			if field.IsExported() && field.Type.Kind() == reflect.Slice {
				lists = append(lists, value.Field(i))
			}
		}
		return len(lists) == 1 && lists[0].Len() == 0
	default:
		return false
	}
}

// appendResultHints adds hints to the next steps of an answer's first text
// block, opening a section when the block has none. A blank hint adds
// nothing, and an answer with no text block is left alone.
func appendResultHints(callResult *mcp.CallToolResult, hints ...string) {
	for _, c := range callResult.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			text.Text = AppendHints(text.Text, hints...)
			return
		}
	}
}

// AppendHints returns md with hints added to the server's guidance section:
// as further bullets of the section it already carries, wherever [WriteHints]
// put it, or as a section of their own when it carries none. Blank hints are
// dropped, and with none left md is returned as it came.
//
// A section is extended rather than a second one written, because
// [ExtractHints] reads one section and only one: a leading section is read in
// preference to anything after the body, and a closing one must end the
// response, so a second section would either be ignored or hide the first.
//
//gitlab:allow-unescaped StripControlBytes(hint): a hint is the server's own sentence, as in WriteHints; the ones appended here are the notes internal/finegrained words from the table compiled into the binary.
func AppendHints(md string, hints ...string) string {
	var bullets strings.Builder
	var kept []string
	for _, hint := range hints {
		if blank(hint) {
			continue
		}
		kept = append(kept, hint)
		fmt.Fprintf(&bullets, "- %s\n", StripControlBytes(hint))
	}
	if len(kept) == 0 {
		return md
	}
	if start, ok := leadingHintsBlock(md); ok {
		end := start + bulletRunLength(md[start:])
		return md[:end] + bullets.String() + md[end:]
	}
	if start, ok := lastHintsBlock(md); ok && parseHintBullets(md[start:], true) != nil {
		return strings.TrimRight(md, "\n") + "\n" + bullets.String()
	}
	var b strings.Builder
	b.WriteString(md)
	WriteHints(&b, kept...)
	return b.String()
}

// bulletRunLength is how many bytes of section its leading bullet lines take,
// each with its newline.
func bulletRunLength(section string) int {
	length := 0
	for line := range strings.SplitAfterSeq(section, "\n") {
		if !strings.HasPrefix(line, "- ") || !strings.HasSuffix(line, "\n") {
			break
		}
		length += len(line)
	}
	return length
}
