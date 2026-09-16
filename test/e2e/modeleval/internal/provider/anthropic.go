//go:build e2e

// anthropic.go is the Messages API: tools carry an input_schema, an answer is a
// list of content blocks, and a tool result is a block of a user message
// addressed to the call by its id.
//
// Two things here are not the old adapter's. Tool choice is left alone, so a
// model that should decline can; and the usage is read in four numbers rather
// than two, because this is the provider that bills a cache write and a cache
// read at different prices and folding them into one figure is how a published
// table came to show a model consuming a fraction of another's budget while
// sending the same conversation more times.

package provider

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
)

const (
	// anthropicEndpoint is the Messages API.
	anthropicEndpoint = "https://api.anthropic.com/v1/messages"
	// anthropicVersion is the API version header every request carries.
	anthropicVersion = "2023-06-01"
)

// anthropicAdapter talks to the Messages API.
type anthropicAdapter struct{ base }

// anthropicRequest is one Messages request.
type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	System      string             `json:"system,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
}

// anthropicTool is one tool as this API takes it.
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicMessage is one turn.
type anthropicMessage struct {
	Role    string           `json:"role"`
	Content []anthropicBlock `json:"content"`
}

// anthropicBlock is one piece of a turn, in either direction.
type anthropicBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

// anthropicResponse is one answer.
type anthropicResponse struct {
	Content    []anthropicBlock `json:"content"`
	StopReason string           `json:"stop_reason"`
	Usage      struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ToolDigest hashes the tools as this adapter sends them.
func (a anthropicAdapter) ToolDigest(tools []Tool) string {
	wire := a.tools(tools)
	entries := make([]toolEntry, 0, len(wire))
	for _, tool := range wire {
		entries = append(entries, toolEntry{Name: tool.Name, Description: tool.Description, Schema: tool.InputSchema})
	}
	return digestTools(entries)
}

// tools renders the tool list. The schema goes out exactly as it came in: this
// adapter has no opinion about it, which is the property the digest publishes.
func (a anthropicAdapter) tools(tools []Tool) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, anthropicTool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema})
	}
	return out
}

// Call sends one request.
func (a anthropicAdapter) Call(ctx context.Context, request Request) (Response, error) {
	payload := anthropicRequest{
		Model:       a.spec.Model,
		MaxTokens:   a.spec.MaxTokens,
		Temperature: a.spec.Temperature,
		System:      request.System,
		Tools:       a.tools(request.Tools),
		Messages:    anthropicMessages(request.Messages),
	}
	answer, raw, err := a.exchange(ctx, a.endpoint, map[string]string{
		"x-api-key":         a.key,
		"anthropic-version": anthropicVersion,
	}, payload)
	if err != nil {
		return answer, err
	}

	var decoded anthropicResponse
	if decodeErr := json.Unmarshal(raw, &decoded); decodeErr != nil {
		return a.failed(answer, modelrecord.TurnServerError, "the answer is not JSON: "+decodeErr.Error())
	}
	if decoded.Error != nil {
		return a.failed(answer, modelrecord.TurnRequestError, decoded.Error.Type+": "+decoded.Error.Message)
	}
	answer.Blocks = anthropicBlocks(decoded.Content)
	answer.Echo = anthropicEcho(decoded.Content)
	answer.Usage = modelrecord.Usage{
		Input:        decoded.Usage.InputTokens,
		Output:       decoded.Usage.OutputTokens,
		CacheCreated: decoded.Usage.CacheCreationInputTokens,
		CacheRead:    decoded.Usage.CacheReadInputTokens,
	}
	return answer, nil
}

// anthropicEcho keeps an answer's content blocks as this API returned them, so
// the next turn can hand them back with whatever signature they carried.
func anthropicEcho(content []anthropicBlock) json.RawMessage {
	encoded, err := json.Marshal(content)
	if err != nil {
		return nil
	}
	return encoded
}

// anthropicMessages renders the conversation.
func anthropicMessages(messages []Message) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(messages))
	for _, message := range messages {
		if blocks, echoed := anthropicEchoed(message); echoed {
			out = append(out, anthropicMessage{Role: RoleAssistant, Content: blocks})
			continue
		}
		blocks := anthropicContent(message)
		if len(blocks) == 0 {
			continue
		}
		out = append(out, anthropicMessage{Role: anthropicRole(message.Role), Content: blocks})
	}
	return out
}

// anthropicEchoed returns an assistant turn as this API itself rendered it.
func anthropicEchoed(message Message) ([]anthropicBlock, bool) {
	if message.Role != RoleAssistant || len(message.Echo) == 0 {
		return nil, false
	}
	var blocks []anthropicBlock
	if err := json.Unmarshal(message.Echo, &blocks); err != nil || len(blocks) == 0 {
		return nil, false
	}
	return blocks, true
}

// anthropicRole maps a role. A tool result is carried by a user message here,
// which is the first of the three second-turn shapes this package has to know.
func anthropicRole(role string) string {
	if role == RoleAssistant {
		return RoleAssistant
	}
	return RoleUser
}

// anthropicContent renders one message's blocks.
func anthropicContent(message Message) []anthropicBlock {
	var blocks []anthropicBlock
	if text := strings.TrimSpace(message.Text); text != "" {
		blocks = append(blocks, anthropicBlock{Type: "text", Text: message.Text})
	}
	for _, block := range message.Blocks {
		if block.Kind != modelrecord.BlockToolCall {
			continue
		}
		// A call the model made is fed back with the arguments it made, not
		// with a repair of them: a malformed call ends its attempt and never
		// reaches a second turn, so the arguments here are always JSON.
		blocks = append(blocks, anthropicBlock{
			Type:  "tool_use",
			ID:    block.CallID,
			Name:  block.Tool,
			Input: block.Arguments,
		})
	}
	for _, result := range message.Results {
		blocks = append(blocks, anthropicBlock{
			Type:      "tool_result",
			ToolUseID: result.CallID,
			Content:   result.Content,
			IsError:   result.IsError,
		})
	}
	return blocks
}

// anthropicBlocks reads what the model emitted.
//
// A tool call whose input is absent is recorded as a call with no arguments,
// which the runner ends the attempt on. Nothing is reconstructed: this API
// returns a decoded object rather than a string of JSON, so the one way to
// arrive here empty is a model that emitted no input at all.
func anthropicBlocks(content []anthropicBlock) []modelrecord.Block {
	var blocks []modelrecord.Block
	for _, block := range content {
		switch block.Type {
		case "text":
			blocks = append(blocks, modelrecord.Block{Kind: modelrecord.BlockText, Text: block.Text})
		case "thinking":
			blocks = append(blocks, modelrecord.Block{Kind: modelrecord.BlockThinking, Text: block.Thinking})
		case "tool_use":
			call := modelrecord.Block{
				Kind:   modelrecord.BlockToolCall,
				Tool:   block.Name,
				CallID: block.ID,
			}
			if json.Valid(block.Input) {
				call.Arguments = block.Input
			} else {
				call.Raw = string(block.Input)
			}
			blocks = append(blocks, call)
		}
	}
	return blocks
}
