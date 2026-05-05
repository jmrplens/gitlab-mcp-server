package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	providerAnthropic = "anthropic"
	providerGoogle    = "google"
	providerOpenAI    = "openai"
	providerQwen      = "qwen"

	openAIChatAPI = "https://api.openai.com/v1/chat/completions"
	qwenChatAPI   = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions"
	geminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models/"
)

type modelSpec struct {
	Provider string
	Model    string
}

func (s modelSpec) String() string {
	if s.Provider == "" {
		return s.Model
	}
	return s.Provider + ":" + s.Model
}

type modelProviderRequest struct {
	Model       string
	MaxTokens   int
	Temperature float64
	System      string
	Tools       []modelTool
	Messages    []modelMessage
}

type modelProvider interface {
	callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error)
}

func resolveModelSpecs(opts options) ([]modelSpec, error) {
	source := strings.TrimSpace(opts.Model)
	if source == "" {
		source = strings.TrimSpace(opts.Models)
	}
	if source == "" {
		source = strings.TrimSpace(os.Getenv("EVAL_MODELS"))
	}
	if source == "" {
		legacy := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
		if legacy == "" {
			legacy = strings.TrimPrefix(defaultModel, providerAnthropic+":")
		}
		source = providerAnthropic + ":" + legacy
	}

	var specs []modelSpec
	for raw := range strings.SplitSeq(source, ",") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		spec, err := parseModelSpec(raw)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, errors.New("no models configured")
	}
	return specs, nil
}

func parseModelSpec(raw string) (modelSpec, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return modelSpec{}, errors.New("empty model spec")
	}
	provider, model, found := strings.Cut(raw, ":")
	if !found {
		provider = providerAnthropic
		model = raw
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if model == "" {
		return modelSpec{}, fmt.Errorf("empty model in %q", raw)
	}
	if provider == providerGoogle {
		model = strings.TrimPrefix(model, "models/")
	}
	switch provider {
	case providerAnthropic, providerGoogle, providerOpenAI, providerQwen:
		return modelSpec{Provider: provider, Model: model}, nil
	default:
		return modelSpec{}, fmt.Errorf("unsupported model provider %q in %q", provider, raw)
	}
}

func modelReportLabel(specs []modelSpec) string {
	labels := make([]string, 0, len(specs))
	for _, spec := range specs {
		labels = append(labels, spec.String())
	}
	return strings.Join(labels, ",")
}

func apiKeyForModelProvider(provider string) (string, error) {
	keyNames := map[string][]string{
		providerAnthropic: {"ANTHROPIC_API_KEY"},
		providerGoogle:    {"GOOGLE_API_KEY"},
		providerOpenAI:    {"OPENAI_API_KEY"},
		providerQwen:      {"QWEN_API_KEY"},
	}[provider]
	if len(keyNames) == 0 {
		return "", fmt.Errorf("unsupported model provider %q", provider)
	}
	for _, keyName := range keyNames {
		value := strings.TrimSpace(os.Getenv(keyName))
		if value != "" {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s is required in the environment or .env for provider %s", strings.Join(keyNames, " or "), provider)
}

func modelProviderFor(provider string) modelProvider {
	switch provider {
	case providerGoogle:
		return googleProvider{}
	case providerOpenAI:
		return openAIProvider{endpoint: openAIChatAPI, name: providerOpenAI, maxTokenField: "max_completion_tokens"}
	case providerQwen:
		return openAIProvider{endpoint: qwenEndpoint(), name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}
	default:
		return anthropicProvider{}
	}
}

func qwenEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("QWEN_CHAT_COMPLETIONS_URL")); endpoint != "" {
		return endpoint
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("QWEN_BASE_URL")), "/"); baseURL != "" {
		return baseURL + "/chat/completions"
	}
	return qwenChatAPI
}

type anthropicProvider struct{}

func (anthropicProvider) callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error) {
	payload := anthropicRequest{
		Model:       request.Model,
		MaxTokens:   request.MaxTokens,
		Temperature: request.Temperature,
		System:      request.System,
		Tools:       request.Tools,
		ToolChoice:  map[string]string{"type": "any"},
		Messages:    request.Messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("marshal anthropic request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	respBody, retry, err := doModelRequest(client, req, "anthropic")
	if err != nil {
		return modelResponse{}, retry, err
	}
	var out modelResponse
	if decodeErr := json.Unmarshal(respBody, &out); decodeErr != nil {
		return modelResponse{}, false, fmt.Errorf("decode anthropic response: %w", decodeErr)
	}
	if out.Error != nil {
		return modelResponse{}, false, fmt.Errorf("anthropic error %s: %s", out.Error.Type, out.Error.Message)
	}
	return out, false, nil
}

type openAIProvider struct {
	endpoint        string
	name            string
	maxTokenField   string
	disableThinking bool
}

type openAIRequest struct {
	Model               string          `json:"model"`
	Temperature         float64         `json:"temperature"`
	MaxTokens           int             `json:"max_tokens,omitempty"`
	MaxCompletionTokens int             `json:"max_completion_tokens,omitempty"`
	EnableThinking      *bool           `json:"enable_thinking,omitempty"`
	Tools               []openAITool    `json:"tools"`
	ToolChoice          string          `json:"tool_choice"`
	Messages            []openAIMessage `json:"messages"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIResponse struct {
	Choices []struct {
		Message openAIMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (p openAIProvider) callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error) {
	payload := openAIRequest{
		Model:       request.Model,
		Temperature: request.Temperature,
		Tools:       openAITools(request.Tools),
		ToolChoice:  "required",
		Messages:    openAIMessages(request),
	}
	if p.maxTokenField == "max_completion_tokens" {
		payload.MaxCompletionTokens = request.MaxTokens
	} else {
		payload.MaxTokens = request.MaxTokens
	}
	if p.disableThinking {
		enableThinking := false
		payload.EnableThinking = &enableThinking
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("marshal %s request: %w", p.name, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new %s request: %w", p.name, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	respBody, retry, err := doModelRequest(client, req, p.name)
	if err != nil {
		return modelResponse{}, retry, err
	}
	var decoded openAIResponse
	if decodeErr := json.Unmarshal(respBody, &decoded); decodeErr != nil {
		return modelResponse{}, false, fmt.Errorf("decode %s response: %w", p.name, decodeErr)
	}
	if decoded.Error != nil {
		return modelResponse{}, false, fmt.Errorf("%s error %s: %s", p.name, decoded.Error.Type, decoded.Error.Message)
	}
	if len(decoded.Choices) == 0 {
		return modelResponse{}, false, fmt.Errorf("%s response contained no choices", p.name)
	}
	blocks, err := openAIToolUseBlocks(decoded.Choices[0].Message)
	if err != nil {
		return modelResponse{}, true, err
	}
	return modelResponse{
		Content: blocks,
		Usage: modelUsage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
		},
	}, false, nil
}

func openAITools(tools []modelTool) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openAITool{Type: "function", Function: openAIFunction{Name: tool.Name, Description: tool.Description, Parameters: tool.InputSchema}})
	}
	return out
}

func openAIMessages(request modelProviderRequest) []openAIMessage {
	out := []openAIMessage{{Role: "system", Content: request.System}}
	for _, message := range request.Messages {
		switch message.Role {
		case "assistant":
			out = append(out, openAIAssistantMessage(message))
		default:
			out = append(out, openAIUserOrToolMessages(message)...)
		}
	}
	return out
}

func openAIAssistantMessage(message modelMessage) openAIMessage {
	assistant := openAIMessage{Role: "assistant"}
	var text []string
	for _, block := range message.Content {
		switch block.Type {
		case "tool_use":
			args, err := json.Marshal(block.Input)
			if err != nil {
				args = []byte("{}")
			}
			assistant.ToolCalls = append(assistant.ToolCalls, openAIToolCall{ID: block.ID, Type: "function", Function: openAIFunctionCall{Name: block.Name, Arguments: string(args)}})
		case "text":
			if block.Text != "" {
				text = append(text, block.Text)
			}
		}
	}
	assistant.Content = strings.Join(text, "\n")
	return assistant
}

func openAIUserOrToolMessages(message modelMessage) []openAIMessage {
	var out []openAIMessage
	var text []string
	for _, block := range message.Content {
		switch block.Type {
		case "tool_result":
			out = append(out, openAIMessage{Role: "tool", ToolCallID: block.ToolUseID, Content: block.Content})
		case "text":
			if block.Text != "" {
				text = append(text, block.Text)
			}
		}
	}
	if len(text) > 0 {
		out = append([]openAIMessage{{Role: "user", Content: strings.Join(text, "\n")}}, out...)
	}
	return out
}

func openAIToolUseBlocks(message openAIMessage) ([]modelContentBlock, error) {
	blocks := make([]modelContentBlock, 0, len(message.ToolCalls))
	for _, call := range message.ToolCalls {
		arguments := strings.TrimSpace(call.Function.Arguments)
		if arguments == "" {
			return nil, fmt.Errorf("%s tool call %s returned empty JSON arguments", call.Function.Name, call.ID)
		}
		input, err := parseOpenAIToolArguments(arguments)
		if err != nil {
			return nil, fmt.Errorf("%s tool call %s returned invalid JSON arguments: %w", call.Function.Name, call.ID, err)
		}
		blocks = append(blocks, modelContentBlock{Type: "tool_use", ID: call.ID, Name: call.Function.Name, Input: input})
	}
	return blocks, nil
}

func parseOpenAIToolArguments(arguments string) (map[string]any, error) {
	input := map[string]any{}
	if err := json.Unmarshal([]byte(arguments), &input); err == nil {
		return input, nil
	}
	candidate := strings.TrimSpace(arguments)
	if start := strings.Index(candidate, "{"); start >= 0 {
		prefix := strings.TrimSpace(candidate[:start])
		if prefix != "" && !strings.ContainsAny(prefix, "\":,") {
			if end := strings.LastIndex(candidate, "}"); end > start {
				objectCandidate := candidate[start : end+1]
				if err := json.Unmarshal([]byte(objectCandidate), &input); err == nil {
					return input, nil
				}
			}
		}
	}
	candidate = strings.Trim(candidate, " \t\r\n,")
	if candidate == "" {
		return nil, errors.New("empty arguments after normalization")
	}
	wrapped := false
	if !strings.HasPrefix(candidate, "{") {
		candidate = "{" + candidate
		wrapped = true
	}
	if wrapped || !strings.HasSuffix(candidate, "}") {
		candidate += "}"
	}
	if err := json.Unmarshal([]byte(candidate), &input); err != nil {
		return nil, err
	}
	return input, nil
}

type googleProvider struct{}

type googleRequest struct {
	SystemInstruction googleContent    `json:"system_instruction"`
	Contents          []googleContent  `json:"contents"`
	Tools             []googleTool     `json:"tools"`
	ToolConfig        googleToolConfig `json:"tool_config"`
	GenerationConfig  struct {
		Temperature     float64 `json:"temperature"`
		MaxOutputTokens int     `json:"max_output_tokens"`
	} `json:"generation_config"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googlePart struct {
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

type googleFunctionCall struct {
	Name    string
	Args    map[string]any
	RawArgs json.RawMessage
	ID      string
}

func (c *googleFunctionCall) UnmarshalJSON(data []byte) error {
	var raw struct {
		Name string          `json:"name"`
		Args json.RawMessage `json:"args"`
		ID   string          `json:"id,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.Name = raw.Name
	c.ID = raw.ID
	c.RawArgs = append(c.RawArgs[:0], raw.Args...)
	c.Args = map[string]any{}
	if len(bytes.TrimSpace(raw.Args)) == 0 || bytes.Equal(bytes.TrimSpace(raw.Args), []byte("null")) {
		return nil
	}
	return json.Unmarshal(raw.Args, &c.Args)
}

func (c googleFunctionCall) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args,omitempty"`
		ID   string         `json:"id,omitempty"`
	}{Name: c.Name, Args: c.Args, ID: c.ID})
}

type googleFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"function_declarations"`
}

type googleFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type googleToolConfig struct {
	FunctionCallingConfig struct {
		Mode string `json:"mode"`
	} `json:"function_calling_config"`
}

type googleResponse struct {
	Candidates []struct {
		Content       googleContent `json:"content"`
		FinishReason  string        `json:"finishReason,omitempty"`
		FinishMessage string        `json:"finishMessage,omitempty"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	PromptFeedback *struct {
		BlockReason        string `json:"blockReason,omitempty"`
		BlockReasonMessage string `json:"blockReasonMessage,omitempty"`
	} `json:"promptFeedback,omitempty"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

func (googleProvider) callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error) {
	payload := googleRequest{
		SystemInstruction: googleContent{Parts: []googlePart{{Text: request.System}}},
		Contents:          googleContents(request.Messages),
		Tools:             []googleTool{{FunctionDeclarations: googleFunctionDeclarations(request.Tools)}},
	}
	payload.ToolConfig.FunctionCallingConfig.Mode = googleFunctionCallingMode()
	payload.GenerationConfig.Temperature = request.Temperature
	payload.GenerationConfig.MaxOutputTokens = request.MaxTokens
	body, err := json.Marshal(payload)
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("marshal google request: %w", err)
	}
	endpoint := geminiAPIBase + url.PathEscape(request.Model) + ":generateContent?key=" + url.QueryEscape(apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new google request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	respBody, retry, err := doModelRequest(client, req, "google")
	if err != nil {
		return modelResponse{}, retry, err
	}
	var decoded googleResponse
	if decodeErr := json.Unmarshal(respBody, &decoded); decodeErr != nil {
		return modelResponse{}, false, fmt.Errorf("decode google response: %w", decodeErr)
	}
	if decoded.Error != nil {
		return modelResponse{}, false, fmt.Errorf("google error %s: %s", decoded.Error.Status, decoded.Error.Message)
	}
	if len(decoded.Candidates) == 0 {
		return modelResponse{}, true, googleEmptyResponseError(decoded, "no candidates")
	}
	blocks := googleToolUseBlocks(decoded.Candidates[0].Content)
	if len(blocks) == 0 && decoded.UsageMetadata.CandidatesTokenCount == 0 {
		return modelResponse{}, true, googleEmptyResponseError(decoded, "no tool calls or output tokens")
	}
	return modelResponse{
		Content: blocks,
		Usage: modelUsage{
			InputTokens:  decoded.UsageMetadata.PromptTokenCount,
			OutputTokens: decoded.UsageMetadata.CandidatesTokenCount,
		},
	}, false, nil
}

func googleEmptyResponseError(decoded googleResponse, summary string) error {
	parts := []string{"google response contained " + summary}
	if len(decoded.Candidates) > 0 {
		candidate := decoded.Candidates[0]
		if candidate.FinishReason != "" {
			parts = append(parts, "finishReason="+candidate.FinishReason)
		}
		if candidate.FinishMessage != "" {
			parts = append(parts, "finishMessage="+candidate.FinishMessage)
		}
	}
	if decoded.PromptFeedback != nil {
		if decoded.PromptFeedback.BlockReason != "" {
			parts = append(parts, "blockReason="+decoded.PromptFeedback.BlockReason)
		}
		if decoded.PromptFeedback.BlockReasonMessage != "" {
			parts = append(parts, "blockReasonMessage="+decoded.PromptFeedback.BlockReasonMessage)
		}
	}
	return errors.New(strings.Join(parts, "; "))
}

func googleFunctionDeclarations(tools []modelTool) []googleFunctionDeclaration {
	out := make([]googleFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		out = append(out, googleFunctionDeclaration{Name: tool.Name, Description: tool.Description, Parameters: sanitizeGoogleSchema(tool.InputSchema)})
	}
	return out
}

func googleFunctionCallingMode() string {
	mode := strings.ToUpper(strings.TrimSpace(os.Getenv("EVAL_GOOGLE_FUNCTION_MODE")))
	switch mode {
	case "AUTO", "ANY", "VALIDATED", "NONE":
		return mode
	default:
		return "VALIDATED"
	}
}

func googleContents(messages []modelMessage) []googleContent {
	out := make([]googleContent, 0, len(messages))
	callNames := map[string]string{}
	for _, message := range messages {
		role := "user"
		if message.Role == "assistant" {
			role = "model"
		}
		content := googleContent{Role: role}
		for _, block := range message.Content {
			switch block.Type {
			case "text":
				content.Parts = append(content.Parts, googlePart{Text: block.Text})
			case "tool_use":
				callNames[block.ID] = block.Name
				content.Parts = append(content.Parts, googlePart{ThoughtSignature: block.ThoughtSignature, FunctionCall: &googleFunctionCall{Name: block.Name, Args: block.Input, ID: block.ID}})
			case "tool_result":
				name := callNames[block.ToolUseID]
				if name == "" {
					name = "tool_result"
				}
				content.Parts = append(content.Parts, googlePart{FunctionResponse: &googleFunctionResponse{ID: block.ToolUseID, Name: name, Response: googleFunctionResponsePayload(block)}})
			}
		}
		if len(content.Parts) > 0 {
			out = append(out, content)
		}
	}
	return out
}

func googleFunctionResponsePayload(block modelContentBlock) map[string]any {
	response := map[string]any{"is_error": block.IsError}
	var parsed any
	if err := json.Unmarshal([]byte(block.Content), &parsed); err == nil {
		response["content"] = parsed
		if object, ok := parsed.(map[string]any); ok {
			maps.Copy(response, object)
		}
		return response
	}
	response["content"] = block.Content
	return response
}

func googleToolUseBlocks(content googleContent) []modelContentBlock {
	blocks := make([]modelContentBlock, 0, len(content.Parts))
	for index, part := range content.Parts {
		if part.FunctionCall == nil {
			continue
		}
		id := part.FunctionCall.ID
		if id == "" {
			id = fmt.Sprintf("google-call-%d", index+1)
		}
		blocks = append(blocks, modelContentBlock{Type: "tool_use", ID: id, Name: part.FunctionCall.Name, Input: part.FunctionCall.Args, ProviderRawInput: part.FunctionCall.RawArgs, ThoughtSignature: part.ThoughtSignature})
	}
	return blocks
}

func sanitizeGoogleSchema(value any) any {
	return sanitizeGoogleSchemaValue(value)
}

func sanitizeGoogleSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			switch key {
			case "$schema", "additionalProperties", "title":
				continue
			case "properties":
				out[key] = sanitizeGoogleSchemaProperties(child)
			case "type":
				out[key] = sanitizeGoogleSchemaType(child)
			default:
				out[key] = sanitizeGoogleSchemaValue(child)
			}
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, child := range typed {
			out = append(out, sanitizeGoogleSchemaValue(child))
		}
		return out
	default:
		return value
	}
}

func sanitizeGoogleSchemaProperties(value any) any {
	properties, ok := value.(map[string]any)
	if !ok {
		return sanitizeGoogleSchemaValue(value)
	}
	out := make(map[string]any, len(properties))
	for name, child := range properties {
		out[name] = sanitizeGoogleSchemaValue(child)
	}
	return out
}

func sanitizeGoogleSchemaType(value any) any {
	values, ok := value.([]any)
	if !ok {
		return value
	}
	for _, candidate := range values {
		text, isString := candidate.(string)
		if isString && text != "null" {
			return text
		}
	}
	return value
}

func doModelRequest(client *http.Client, req *http.Request, provider string) (body []byte, retry bool, err error) {
	resp, err := client.Do(req) //nolint:gosec // Provider URLs come from explicit evaluator configuration, not model-generated input.
	if err != nil {
		return nil, true, fmt.Errorf("%s request: %w", provider, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, true, fmt.Errorf("read %s response: %w", provider, err)
	}
	if len(respBody) > maxResponseBytes {
		return nil, false, fmt.Errorf("%s response exceeded %d bytes", provider, maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retry, fmt.Errorf("%s status %d: %s", provider, resp.StatusCode, redactResponse(respBody))
	}
	return respBody, false, nil
}
