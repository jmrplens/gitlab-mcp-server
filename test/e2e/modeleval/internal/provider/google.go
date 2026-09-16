//go:build e2e

// google.go is the Gemini generateContent shape: the model is in the path, a
// turn is a content with parts, a tool call is a functionCall part and a tool
// result is a functionResponse part in a user content, addressed by the name of
// the function rather than by an identifier. That is the third of the three
// second-turn shapes, and the one that needs a name the neutral request does
// not otherwise carry.
//
// This adapter used to sanitize the schemas on their way out, because this API
// refuses a function declaration carrying "$schema" or "additionalProperties".
// It no longer does, and that is not a relaxation: the same removal now happens
// once in [NewTool], before any adapter sees the list, so all four send the same
// schemas and the four digests of one list agree. A sanitizer here would make
// this provider's digest differ from its siblings for a reason that is about
// the API rather than about what the model was shown, which is exactly the
// signal the digest exists to carry.
//
// There is deliberately no tool_config. The old adapter sent VALIDATED, which
// forces a function call; a model that should decline could not, and the
// read-only rows would have measured willingness to call a tool the server did
// not offer.
//
// An answer's parts are kept as the bytes they arrived as and go back as those
// same bytes. [googlePart] names the two fields this package reads and none of
// the rest: a thought part is marked with "thought", and reading a turn into
// that struct and marshaling it again hands the model's own reasoning back as
// ordinary model text. The role is this adapter's rather than the echo's, which
// is the one thing a content wrapper carries that a request must be sure of: a
// content sent without one is read as the user's.

package provider

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

// googleEndpoint is the generateContent API, without the model, which goes in
// the path.
const googleEndpoint = "https://generativelanguage.googleapis.com/v1beta/models/"

// googleAdapter talks to the Gemini API.
type googleAdapter struct{ base }

// googleRequest is one generateContent request.
type googleRequest struct {
	SystemInstruction *googleContent   `json:"system_instruction,omitempty"`
	Contents          []googleTurn     `json:"contents"`
	Tools             []googleTool     `json:"tools,omitempty"`
	GenerationConfig  googleGeneration `json:"generation_config"`
}

// googleTurn is one turn of a request: a role this adapter decides and parts
// that are the bytes this API sent when the turn is its own answer handed back.
//
// The two halves are split for a reason each way. The parts are what a thinking
// model signs, so they go back untouched; the role is the one thing a content
// carries that a request must be sure of, and an answer's own content is not
// guaranteed to spell it, so this adapter says it rather than echoing it.
type googleTurn struct {
	Role  string    `json:"role,omitempty"`
	Parts wireValue `json:"parts"`
}

// googleGeneration is the sampling configuration.
type googleGeneration struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens int      `json:"max_output_tokens"`
}

// googleContent is one turn.
type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

// googlePart is one piece of a turn, as this package reads and builds one.
//
// It is never what an echo is made of, and it names none of the fields that
// make an echo necessary: a thought part is marked with "thought", and a
// thinking model signs its function call with a thoughtSignature it refuses the
// next request without. Both live in the bytes the API sent and nowhere in the
// record, which is why an echoed turn goes back as those bytes rather than as
// this struct marshaled again.
type googlePart struct {
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

// googleFunctionCall is one call the model made.
type googleFunctionCall struct {
	ID   string          `json:"id,omitempty"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}

// googleFunctionResponse is one tool result.
type googleFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// googleTool is the one tool entry a request carries, holding every
// declaration.
type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"function_declarations"`
}

// googleFunctionDeclaration is one tool.
type googleFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// googleResponse is one answer.
//
// A candidate's content is raw because it is echoed as it arrived; it is
// decoded into parts beside that, for the record's own reading of the turn.
type googleResponse struct {
	Candidates []struct {
		Content      json.RawMessage `json:"content"`
		FinishReason string          `json:"finishReason,omitempty"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason,omitempty"`
	} `json:"promptFeedback,omitempty"`
	Error *struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ToolDigest hashes the tools as this adapter sends them.
func (a googleAdapter) ToolDigest(tools []Tool) string {
	wire := a.tools(tools)
	entries := make([]toolEntry, 0, len(tools))
	for _, tool := range wire {
		for _, declaration := range tool.FunctionDeclarations {
			entries = append(entries, toolEntry{
				Name:        declaration.Name,
				Description: declaration.Description,
				Schema:      declaration.Parameters,
			})
		}
	}
	return digestTools(entries)
}

// tools renders the tool list, schemas untouched.
func (a googleAdapter) tools(tools []Tool) []googleTool {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]googleFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		declarations = append(declarations, googleFunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Schema,
		})
	}
	return []googleTool{{FunctionDeclarations: declarations}}
}

// Call sends one request.
func (a googleAdapter) Call(ctx context.Context, request Request) (Response, error) {
	payload := googleRequest{
		Contents: googleContents(request.Messages),
		Tools:    a.tools(request.Tools),
		GenerationConfig: googleGeneration{
			Temperature:     a.spec.Temperature,
			MaxOutputTokens: a.spec.MaxTokens,
		},
	}
	if request.System != "" {
		payload.SystemInstruction = &googleContent{Parts: []googlePart{{Text: request.System}}}
	}

	// The model is part of the path on this API, and the configured endpoint
	// is the collection it hangs under, with or without its separator: a base
	// URL read from a setting is spelled both ways and neither should produce
	// a URL that does not parse.
	endpoint := strings.TrimSuffix(a.endpoint, "/") + "/" + url.PathEscape(a.spec.Model) + ":generateContent"
	answer, raw, err := a.exchange(ctx, endpoint, map[string]string{"x-goog-api-key": a.key}, payload)
	if err != nil {
		return answer, err
	}

	var decoded googleResponse
	if decodeErr := json.Unmarshal(raw, &decoded); decodeErr != nil {
		return a.failed(answer, modelrecord.TurnServerError, "the answer is not JSON: "+decodeErr.Error())
	}
	if decoded.Error != nil {
		return a.failed(answer, modelrecord.TurnRequestError, decoded.Error.Status+": "+decoded.Error.Message)
	}
	if len(decoded.Candidates) == 0 {
		return a.failed(answer, modelrecord.TurnServerError, googleEmptyDetail(decoded))
	}
	content, contentErr := googleCandidateContent(decoded.Candidates[0].Content)
	if contentErr != nil {
		return a.failed(answer, modelrecord.TurnServerError,
			"the candidate's content does not decode: "+contentErr.Error())
	}
	answer.Blocks = googleBlocks(content)
	answer.Echo = googleEcho(decoded.Candidates[0].Content)
	answer.Usage = modelrecord.Usage{
		// The prompt count includes what was served from the cache, and the
		// candidates count excludes the thinking this model was billed for,
		// so neither number is the one a cost column wants as it stands.
		Input:     max(decoded.UsageMetadata.PromptTokenCount-decoded.UsageMetadata.CachedContentTokenCount, 0),
		Output:    decoded.UsageMetadata.CandidatesTokenCount + decoded.UsageMetadata.ThoughtsTokenCount,
		CacheRead: decoded.UsageMetadata.CachedContentTokenCount,
	}
	return answer, nil
}

// googleEmptyDetail says what an answer with no candidate said instead, which
// is where a safety block or a finish reason is the only explanation there is.
func googleEmptyDetail(decoded googleResponse) string {
	detail := []string{"the answer carried no candidates"}
	if decoded.PromptFeedback != nil && decoded.PromptFeedback.BlockReason != "" {
		detail = append(detail, "blockReason="+decoded.PromptFeedback.BlockReason)
	}
	return strings.Join(detail, "; ")
}

// googleEcho keeps an answer's content as the bytes this API sent, thought
// markers and thought signatures included.
//
// It is a copy rather than a slice of the response buffer: the answer outlives
// the request that read it, and a caller holding an echo should not depend on
// what else that buffer is used for.
func googleEcho(content json.RawMessage) json.RawMessage {
	if len(content) == 0 {
		return nil
	}
	return slices.Clone(content)
}

// googleCandidateContent reads a candidate's content as the parts the record
// keeps. A candidate carrying no content is not an error: the model emitted
// nothing, which the turn records as no blocks.
func googleCandidateContent(content json.RawMessage) (googleContent, error) {
	if len(content) == 0 {
		return googleContent{}, nil
	}
	var decoded googleContent
	if err := json.Unmarshal(content, &decoded); err != nil {
		return googleContent{}, err
	}
	return decoded, nil
}

// googleContents renders the conversation.
//
// The call names are remembered as they go past, because a functionResponse
// must carry the name of the function it answers and the neutral tool result
// carries an identifier. A result whose call this conversation never saw is
// sent under the name the result itself records, which is what a runner that
// starts a conversation from a stored turn produces.
func googleContents(messages []Message) []googleTurn {
	names := map[string]string{}
	out := make([]googleTurn, 0, len(messages))
	for _, message := range messages {
		if raw, parts, ok := googleEchoed(message); ok {
			rememberGoogleNames(names, parts)
			out = append(out, googleTurn{Role: roleModel, Parts: wireRaw(raw)})
			continue
		}
		var built []googlePart
		if text := strings.TrimSpace(message.Text); text != "" {
			built = append(built, googlePart{Text: message.Text})
		}
		built = append(built, googleCallParts(message, names)...)
		built = append(built, googleResultParts(message, names)...)
		if len(built) > 0 {
			out = append(out, googleTurn{Role: googleRole(message.Role), Parts: wireBuilt(built)})
		}
	}
	return out
}

// roleModel is what this API calls the assistant.
//
// It is unexported where the three neutral roles are not, and the difference is
// who writes one: a caller builds a [Message] with [RoleAssistant] and this
// adapter translates it, so an exported spelling here would be a fourth role
// nothing outside can use.
const roleModel = "model"

// googleRole maps a role. A tool result travels in a user content here.
func googleRole(role string) string {
	if role == RoleAssistant {
		return roleModel
	}
	return RoleUser
}

// googleCallParts renders the calls of an assistant turn and remembers their
// names.
func googleCallParts(message Message, names map[string]string) []googlePart {
	var parts []googlePart
	for _, block := range message.Blocks {
		if block.Kind != modelrecord.BlockToolCall {
			continue
		}
		names[block.CallID] = block.Tool
		parts = append(parts, googlePart{FunctionCall: &googleFunctionCall{
			ID:   block.CallID,
			Name: block.Tool,
			Args: block.Arguments,
		}})
	}
	return parts
}

// googleResultParts renders the tool results of a message.
func googleResultParts(message Message, names map[string]string) []googlePart {
	var parts []googlePart
	for _, result := range message.Results {
		name := names[result.CallID]
		if name == "" {
			name = result.Tool
		}
		parts = append(parts, googlePart{FunctionResponse: &googleFunctionResponse{
			ID:   result.CallID,
			Name: name,
			// The response is an object because this API requires one. The
			// server's answer is text, so it is carried under one key rather
			// than parsed and spread: a result that happens to be JSON would
			// otherwise reach this model as fields and reach every other one
			// as text, and the surfaces would stop being comparable.
			Response: map[string]any{"content": result.Content, "is_error": result.IsError},
		}})
	}
	return parts
}

// googleEchoed returns an assistant turn's parts as this API itself sent them,
// beside the same parts decoded.
//
// Both halves are used: the bytes are what goes back, and the decoding is read
// for the call names a later functionResponse has to quote and to check that
// the echo is parts at all. A conversation assembled from the record carries no
// echo, and one carrying something else is rebuilt from the blocks rather than
// sent as whatever it is.
func googleEchoed(message Message) (json.RawMessage, []googlePart, bool) {
	if message.Role != RoleAssistant || len(message.Echo) == 0 {
		return nil, nil, false
	}
	var echoed struct {
		Parts json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal(message.Echo, &echoed); err != nil || len(echoed.Parts) == 0 {
		return nil, nil, false
	}
	var parts []googlePart
	if err := json.Unmarshal(echoed.Parts, &parts); err != nil || len(parts) == 0 {
		return nil, nil, false
	}
	return echoed.Parts, parts, true
}

// rememberGoogleNames records the calls of an echoed turn, so the result that
// answers one can name its function.
func rememberGoogleNames(names map[string]string, parts []googlePart) {
	for _, part := range parts {
		if part.FunctionCall != nil {
			names[part.FunctionCall.ID] = part.FunctionCall.Name
		}
	}
}

// googleBlocks reads what the model emitted.
//
// A call arrives with an identifier only from the models that mint one, so an
// absent identifier is filled from the call's position. Every provider's tool
// result is addressed by an identifier here, and a conversation whose two calls
// share the empty one would answer both with the first result.
func googleBlocks(content googleContent) []modelrecord.Block {
	var blocks []modelrecord.Block
	for index, part := range content.Parts {
		switch {
		case part.FunctionCall != nil:
			call := modelrecord.Block{
				Kind:   modelrecord.BlockToolCall,
				Tool:   part.FunctionCall.Name,
				CallID: googleCallID(part.FunctionCall.ID, index),
			}
			if json.Valid(part.FunctionCall.Args) && len(part.FunctionCall.Args) > 0 {
				call.Arguments = part.FunctionCall.Args
			} else {
				call.Raw = string(part.FunctionCall.Args)
			}
			blocks = append(blocks, call)
		case part.Text != "":
			blocks = append(blocks, modelrecord.Block{Kind: modelrecord.BlockText, Text: part.Text})
		}
	}
	return blocks
}

// googleCallID names a call this API did not name.
func googleCallID(id string, index int) string {
	if id != "" {
		return id
	}
	return "google-call-" + strconv.Itoa(index+1)
}
