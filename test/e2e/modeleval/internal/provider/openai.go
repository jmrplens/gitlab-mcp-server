//go:build e2e

// openai.go is the chat completions shape, which two providers speak: OpenAI
// itself and DashScope's compatible endpoint for Qwen. They differ in three
// details and in nothing else, so they are one adapter with three fields rather
// than two copies that drift.
//
// This is where the schema injection lived. The old adapter rewrote the dynamic
// execute tool's schema for these two providers alone, adding thirty-six
// parameter names, marking params an object with those properties and
// requiring action and params. Nothing recorded it, and the two columns it
// produced sat in a published cross-vendor table beside two that had received
// the surface as the server publishes it. It is gone, and what replaced it is
// the digest: this adapter sends the schema it was given and hashes what it
// sent.
//
// An answer's message is kept as the bytes it arrived as and handed back as
// those same bytes. [openAIMessage] names what this package reads and builds,
// which is not all an assistant message carries: DashScope returns a thinking
// model's reasoning_content on it, and a message re-marshaled through this
// struct reaches the next request without whatever the endpoint attached to it.
//
// It is also where a model's tool call arrives as a string of JSON rather than
// as an object, which is the whole reason a malformed call is a class here at
// all. The old adapter had five layers of repair for that string, tried them in
// order, and then retried the request up to four times and re-ran the task; a
// model that could not emit a parseable call therefore scored as one that
// could. The string is now parsed once and kept verbatim when it does not
// parse.

package provider

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

const (
	// openAIEndpoint is the chat completions API.
	openAIEndpoint = "https://api.openai.com/v1/chat/completions"
	// qwenEndpoint is DashScope's OpenAI-compatible endpoint, international
	// region. A deployment in another region is configured through
	// [Config.Endpoint].
	qwenEndpoint = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions"

	// openAIMaxTokensField is the ceiling field OpenAI takes. The older
	// max_tokens is refused outright by the reasoning models.
	openAIMaxTokensField = "max_completion_tokens"
	// qwenMaxTokensField is the one DashScope's compatible mode takes.
	qwenMaxTokensField = "max_tokens"
)

// openAIAdapter talks to a chat completions endpoint.
type openAIAdapter struct {
	base
	// maxTokensField is which of the two ceiling fields this endpoint reads.
	maxTokensField string
}

// openAITool is one tool as this API takes it.
type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

// openAIFunction is the tool itself.
type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

// openAIMessage is one turn. A tool result is a message of its own with the
// tool role, addressed to the call by id, which is the second of the three
// second-turn shapes.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIToolCall is one call the model made.
type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

// openAIFunctionCall carries the arguments as a string of JSON, which is what
// makes a malformed call possible on this API and not on Anthropic's.
type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIResponse is one answer.
//
// A choice's message is raw because it is echoed as it arrived; it is decoded
// beside that, for the record's own reading of the turn.
type openAIResponse struct {
	Choices []struct {
		Message      json.RawMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens        int `json:"prompt_tokens"`
		CompletionTokens    int `json:"completion_tokens"`
		PromptTokensDetails struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ToolDigest hashes the tools as this adapter sends them.
func (a openAIAdapter) ToolDigest(tools []Tool) string {
	wire := a.tools(tools)
	entries := make([]toolEntry, 0, len(wire))
	for _, tool := range wire {
		entries = append(entries, toolEntry{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Schema:      tool.Function.Parameters,
		})
	}
	return digestTools(entries)
}

// tools renders the tool list, schemas untouched.
func (a openAIAdapter) tools(tools []Tool) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openAITool{Type: "function", Function: openAIFunction{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Schema,
		}})
	}
	return out
}

// Call sends one request.
//
// The payload is assembled as a map because the two providers disagree about
// the name of one field and about which optional fields exist at all, and a
// struct carrying every variant would send the fields it was not given as
// nulls or omit them by a tag that cannot depend on the provider.
func (a openAIAdapter) Call(ctx context.Context, request Request) (Response, error) {
	payload := map[string]any{
		"model":          a.spec.Model,
		"messages":       a.messages(request),
		"tools":          a.tools(request.Tools),
		a.maxTokensField: a.spec.MaxTokens,
	}
	if a.spec.Temperature != nil {
		payload["temperature"] = *a.spec.Temperature
	}
	if a.spec.ReasoningEffort != "" {
		payload["reasoning_effort"] = a.spec.ReasoningEffort
	}
	if a.spec.EnableThinking != nil {
		payload["enable_thinking"] = *a.spec.EnableThinking
	}

	answer, raw, err := a.exchange(ctx, a.endpoint, map[string]string{
		"Authorization": "Bearer " + a.key,
	}, payload)
	if err != nil {
		return answer, err
	}

	var decoded openAIResponse
	if decodeErr := json.Unmarshal(raw, &decoded); decodeErr != nil {
		return a.failed(answer, modelrecord.TurnServerError, "the answer is not JSON: "+decodeErr.Error())
	}
	if decoded.Error != nil {
		return a.failed(answer, modelrecord.TurnRequestError, decoded.Error.Type+": "+decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return a.failed(answer, modelrecord.TurnServerError, "the answer carried no choices")
	}
	message, messageErr := openAIChoiceMessage(decoded.Choices[0].Message)
	if messageErr != nil {
		return a.failed(answer, modelrecord.TurnServerError,
			"the choice's message does not decode: "+messageErr.Error())
	}
	answer.Blocks = openAIBlocks(message)
	answer.Echo = openAIEcho(decoded.Choices[0].Message)
	answer.Usage = modelrecord.Usage{
		// This API reports the cached tokens inside the prompt total, so the
		// uncached input is the difference. Reporting the total as input and
		// the cached count beside it would bill the same tokens twice in
		// every cost column derived from the record.
		Input:     max(decoded.Usage.PromptTokens-decoded.Usage.PromptTokensDetails.CachedTokens, 0),
		Output:    decoded.Usage.CompletionTokens,
		CacheRead: decoded.Usage.PromptTokensDetails.CachedTokens,
	}
	return answer, nil
}

// messages renders the conversation, the system contract first.
func (a openAIAdapter) messages(request Request) []wireValue {
	out := make([]wireValue, 0, len(request.Messages)+1)
	if request.System != "" {
		out = append(out, wireBuilt(openAIMessage{Role: "system", Content: request.System}))
	}
	for _, message := range request.Messages {
		out = append(out, openAIMessages(message)...)
	}
	return out
}

// openAIEcho keeps an answer's message as the bytes this API sent, so the next
// turn hands back exactly the tool-call identifiers and whatever else the
// provider attached to them.
//
// It is a copy rather than a slice of the response buffer: the answer outlives
// the request that read it, and a caller holding an echo should not depend on
// what else that buffer is used for.
func openAIEcho(message json.RawMessage) json.RawMessage {
	if len(message) == 0 {
		return nil
	}
	return slices.Clone(message)
}

// openAIChoiceMessage reads a choice's message as the record keeps it. A choice
// carrying no message is not an error: the model emitted nothing, which the
// turn records as no blocks.
func openAIChoiceMessage(message json.RawMessage) (openAIMessage, error) {
	if len(message) == 0 {
		return openAIMessage{}, nil
	}
	var decoded openAIMessage
	if err := json.Unmarshal(message, &decoded); err != nil {
		return openAIMessage{}, err
	}
	return decoded, nil
}

// openAIMessages renders one neutral message as the one or more this API takes.
func openAIMessages(message Message) []wireValue {
	if echoed, ok := openAIEchoed(message); ok {
		return []wireValue{wireRaw(echoed)}
	}
	if message.Role == RoleTool {
		out := make([]wireValue, 0, len(message.Results))
		for _, result := range message.Results {
			out = append(out, wireBuilt(openAIMessage{
				Role:       RoleTool,
				ToolCallID: result.CallID,
				Content:    result.Content,
			}))
		}
		return out
	}
	if message.Role != RoleAssistant {
		return []wireValue{wireBuilt(openAIMessage{Role: RoleUser, Content: message.Text})}
	}

	assistant := openAIMessage{Role: RoleAssistant, Content: message.Text}
	var text []string
	if message.Text != "" {
		text = append(text, message.Text)
	}
	for _, block := range message.Blocks {
		switch block.Kind {
		case modelrecord.BlockText:
			text = append(text, block.Text)
		case modelrecord.BlockToolCall:
			assistant.ToolCalls = append(assistant.ToolCalls, openAIToolCall{
				ID:       block.CallID,
				Type:     "function",
				Function: openAIFunctionCall{Name: block.Tool, Arguments: string(block.Arguments)},
			})
		}
	}
	assistant.Content = strings.Join(text, "\n")
	return []wireValue{wireBuilt(assistant)}
}

// openAIEchoed returns an assistant turn as this API itself sent it, which is
// the bytes it sent and not a re-rendering of them.
//
// The echo is decoded only to be checked. A conversation assembled from the
// record carries none, and one carrying something that is not a message is
// rebuilt from the blocks rather than sent as whatever it is.
func openAIEchoed(message Message) (json.RawMessage, bool) {
	if message.Role != RoleAssistant || len(message.Echo) == 0 {
		return nil, false
	}
	var echoed openAIMessage
	if err := json.Unmarshal(message.Echo, &echoed); err != nil || echoed.Role == "" {
		return nil, false
	}
	return message.Echo, true
}

// openAIBlocks reads what the model emitted.
//
// The arguments are parsed exactly once. A string that is not an object is kept
// as it arrived and the call carries no arguments, which is what the runner
// ends the attempt on: a malformed tool call is a thing this model did, and a
// repair would be this package answering for it.
func openAIBlocks(message openAIMessage) []modelrecord.Block {
	var blocks []modelrecord.Block
	if text := strings.TrimSpace(message.Content); text != "" {
		blocks = append(blocks, modelrecord.Block{Kind: modelrecord.BlockText, Text: message.Content})
	}
	for _, call := range message.ToolCalls {
		block := modelrecord.Block{
			Kind:   modelrecord.BlockToolCall,
			Tool:   call.Function.Name,
			CallID: call.ID,
		}
		arguments := strings.TrimSpace(call.Function.Arguments)
		if isJSONObject(arguments) {
			block.Arguments = json.RawMessage(arguments)
		} else {
			block.Raw = call.Function.Arguments
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// isJSONObject reports whether one string is a JSON object, which is the only
// shape a tool call's arguments may take.
//
// The check is for an object rather than for any JSON value because a model
// that answers "null" or "[]" has not sent arguments either, and recording that
// as a well-formed call would put an empty argument set in a column that
// measures argument fidelity.
func isJSONObject(text string) bool {
	var decoded map[string]any
	return json.Unmarshal([]byte(text), &decoded) == nil && decoded != nil
}
