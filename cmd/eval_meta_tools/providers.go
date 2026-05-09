package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

	headerContentType = "Content-Type"
	contentTypeJSON   = "application/json"
	headerGoogleAuth  = "x-goog-api-key"
)

// modelSpec holds data for main operations.
type modelSpec struct {
	Provider string
	Model    string
}

// String performs the string operation on modelSpec.
func (s modelSpec) String() string {
	if s.Provider == "" {
		return s.Model
	}
	return s.Provider + ":" + s.Model
}

// modelProviderRequest holds data for main operations.
type modelProviderRequest struct {
	Model       string
	MaxTokens   int
	Temperature float64
	System      string
	Tools       []modelTool
	Messages    []modelMessage
}

// modelProvider defines the contract for model provider operations.
type modelProvider interface {
	callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error)
}

// resolveModelSpecs resolves model specs using the GitLab API and returns [[]modelSpec].
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

// parseModelSpec performs the parse model spec operation using the GitLab API and returns [modelSpec].
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

// modelReportLabel is an internal helper for the main package.
func modelReportLabel(specs []modelSpec) string {
	labels := make([]string, 0, len(specs))
	for _, spec := range specs {
		labels = append(labels, spec.String())
	}
	return strings.Join(labels, ",")
}

// apiKeyForModelProvider performs the api key for model provider operation using the GitLab API and returns [string].
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

// modelProviderFor is an internal helper for the main package.
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

// qwenEndpoint is an internal helper for the main package.
func qwenEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("QWEN_CHAT_COMPLETIONS_URL")); endpoint != "" {
		return endpoint
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("QWEN_BASE_URL")), "/"); baseURL != "" {
		return baseURL + "/chat/completions"
	}
	return qwenChatAPI
}

// anthropicProvider holds data for main operations.
type anthropicProvider struct{}

// callOnce performs the call once operation on anthropicProvider.
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
	trace := newModelProviderTrace("anthropic", http.MethodPost, anthropicAPI, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new anthropic request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	respBody, status, retry, err := doModelRequest(client, req, "anthropic")
	trace.setResponse(status, respBody)
	if err != nil {
		return modelResponse{}, retry, withProviderTrace(err, trace)
	}
	var out modelResponse
	if decodeErr := json.Unmarshal(respBody, &out); decodeErr != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("decode anthropic response: %w", decodeErr), trace)
	}
	out.ProviderTrace = trace
	if out.Error != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("anthropic error %s: %s", out.Error.Type, out.Error.Message), trace)
	}
	return out, false, nil
}

// openAIProvider holds data for main operations.
type openAIProvider struct {
	endpoint        string
	name            string
	maxTokenField   string
	disableThinking bool
}

// openAIRequest holds data for main operations.
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

// openAITool holds data for main operations.
type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

// openAIFunction holds data for main operations.
type openAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Strict      *bool  `json:"strict,omitempty"`
}

// openAIMessage holds data for main operations.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIToolCall holds data for main operations.
type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

// openAIFunctionCall holds data for main operations.
type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIResponse holds data for main operations.
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

// callOnce performs the call once operation on openAIProvider.
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
	trace := newModelProviderTrace(p.name, http.MethodPost, p.endpoint, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new %s request: %w", p.name, err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	respBody, status, retry, err := doModelRequest(client, req, p.name)
	trace.setResponse(status, respBody)
	if err != nil {
		return modelResponse{}, retry, withProviderTrace(err, trace)
	}
	var decoded openAIResponse
	if decodeErr := json.Unmarshal(respBody, &decoded); decodeErr != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("decode %s response: %w", p.name, decodeErr), trace)
	}
	if decoded.Error != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("%s error %s: %s", p.name, decoded.Error.Type, decoded.Error.Message), trace)
	}
	if len(decoded.Choices) == 0 {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("%s response contained no choices", p.name), trace)
	}
	blocks, err := openAIToolUseBlocks(decoded.Choices[0].Message)
	if err != nil {
		return modelResponse{}, true, withProviderTrace(err, trace)
	}
	return modelResponse{
		Content: blocks,
		Usage: modelUsage{
			InputTokens:  decoded.Usage.PromptTokens,
			OutputTokens: decoded.Usage.CompletionTokens,
		},
		ProviderTrace: trace,
	}, false, nil
}

// openAITools is an internal helper for the main package.
func openAITools(tools []modelTool) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		function := openAIFunction{Name: tool.Name, Description: tool.Description, Parameters: openAIToolSchema(tool)}
		out = append(out, openAITool{Type: "function", Function: function})
	}
	return out
}

func openAIToolSchema(tool modelTool) any {
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok || tool.Name != dynamicExecuteTool {
		return tool.InputSchema
	}
	updated := cloneOpenAISchema(schema)
	updated["required"] = requiredWithNames(updated["required"], "action", "params")
	updated["additionalProperties"] = false
	addOpenAIExecuteParamHints(updated)
	return updated
}

func cloneOpenAISchema(schema map[string]any) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{}
	}
	var cloned map[string]any
	if unmarshalErr := json.Unmarshal(data, &cloned); unmarshalErr != nil {
		return map[string]any{}
	}
	return cloned
}

func addOpenAIExecuteParamHints(schema map[string]any) {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		properties = map[string]any{}
		schema["properties"] = properties
	}
	paramsSchema, ok := properties["params"].(map[string]any)
	if !ok {
		paramsSchema = map[string]any{"type": "object"}
		properties["params"] = paramsSchema
	}
	paramsSchema["type"] = "object"
	paramsSchema["additionalProperties"] = true
	paramProperties, ok := paramsSchema["properties"].(map[string]any)
	if !ok {
		paramProperties = map[string]any{}
		paramsSchema["properties"] = paramProperties
	}
	for name, paramSchema := range openAICommonExecuteParams() {
		if _, exists := paramProperties[name]; !exists {
			paramProperties[name] = paramSchema
		}
	}
}

func openAICommonExecuteParams() map[string]any {
	stringParam := map[string]any{"type": "string"}
	integerParam := map[string]any{"type": "integer"}
	booleanParam := map[string]any{"type": "boolean"}
	return map[string]any{
		"project_id":              stringParam,
		"group_id":                stringParam,
		"full_path":               stringParam,
		"file_path":               stringParam,
		"branch":                  stringParam,
		"branch_name":             stringParam,
		"ref":                     stringParam,
		"content":                 stringParam,
		"commit_message":          stringParam,
		"tag_name":                stringParam,
		"name":                    stringParam,
		"key":                     stringParam,
		"value":                   stringParam,
		"environment_scope":       stringParam,
		"slug":                    stringParam,
		"duration":                stringParam,
		"scope":                   stringParam,
		"artifact_path":           stringParam,
		"commit_sha":              stringParam,
		"discussion_id":           stringParam,
		"runner_id":               integerParam,
		"job_id":                  integerParam,
		"pipeline_id":             integerParam,
		"trigger_id":              integerParam,
		"schedule_id":             integerParam,
		"user_id":                 integerParam,
		"issue_iid":               integerParam,
		"merge_request_iid":       integerParam,
		"award_id":                integerParam,
		"deploy_key_id":           integerParam,
		"deploy_token_id":         integerParam,
		"package_id":              integerParam,
		"note_id":                 integerParam,
		"enable_ssl_verification": booleanParam,
	}
}

func requiredWithNames(raw any, names ...string) []string {
	seen := map[string]bool{}
	var required []string
	appendName := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		required = append(required, name)
	}
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				appendName(name)
			}
		}
	case []string:
		for _, name := range values {
			appendName(name)
		}
	}
	for _, name := range names {
		appendName(name)
	}
	return required
}

// openAIMessages is an internal helper for the main package.
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

// openAIAssistantMessage is an internal helper for the main package.
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

// openAIUserOrToolMessages is an internal helper for the main package.
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

// openAIToolUseBlocks performs the open a i tool use blocks operation using the GitLab API and returns [[]modelContentBlock].
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

// parseOpenAIToolArguments performs the parse open a i tool arguments operation using the GitLab API and returns [map[string]any].
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
	addedOpening := false
	if !strings.HasPrefix(candidate, "{") {
		candidate = "{" + candidate
		addedOpening = true
		if err := json.Unmarshal([]byte(candidate), &input); err == nil {
			return input, nil
		}
	}
	if !strings.HasSuffix(candidate, "}") {
		candidate += "}"
	} else if addedOpening {
		candidate += "}"
	}
	if err := json.Unmarshal([]byte(candidate), &input); err != nil {
		return nil, err
	}
	return input, nil
}

// googleProvider holds data for main operations.
type googleProvider struct{}

// googleRequest holds data for main operations.
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

// googleContent holds data for main operations.
type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

// googlePart holds data for main operations.
type googlePart struct {
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

// googleFunctionCall holds data for main operations.
type googleFunctionCall struct {
	Name    string
	Args    map[string]any
	RawArgs json.RawMessage
	ID      string
}

// UnmarshalJSON performs the unmarshal j s o n operation on *googleFunctionCall.
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

// MarshalJSON performs the marshal j s o n operation on googleFunctionCall.
func (c googleFunctionCall) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args,omitempty"`
		ID   string         `json:"id,omitempty"`
	}{Name: c.Name, Args: c.Args, ID: c.ID})
}

// googleFunctionResponse holds data for main operations.
type googleFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// googleTool holds data for main operations.
type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"function_declarations"`
}

// googleFunctionDeclaration holds data for main operations.
type googleFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// googleToolConfig holds data for main operations.
type googleToolConfig struct {
	FunctionCallingConfig struct {
		Mode string `json:"mode"`
	} `json:"function_calling_config"`
}

// googleResponse holds data for main operations.
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

// callOnce performs the call once operation on googleProvider.
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
	endpoint := geminiAPIBase + url.PathEscape(request.Model) + ":generateContent"
	trace := newModelProviderTrace("google", http.MethodPost, endpoint, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new google request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set(headerGoogleAuth, apiKey)

	respBody, status, retry, err := doModelRequest(client, req, "google")
	trace.setResponse(status, respBody)
	if err != nil {
		return modelResponse{}, retry, withProviderTrace(err, trace)
	}
	var decoded googleResponse
	if decodeErr := json.Unmarshal(respBody, &decoded); decodeErr != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("decode google response: %w", decodeErr), trace)
	}
	if decoded.Error != nil {
		return modelResponse{}, false, withProviderTrace(fmt.Errorf("google error %s: %s", decoded.Error.Status, decoded.Error.Message), trace)
	}
	if len(decoded.Candidates) == 0 {
		return modelResponse{}, true, withProviderTrace(googleEmptyResponseError(decoded, "no candidates"), trace)
	}
	blocks := googleContentBlocks(decoded.Candidates[0].Content)
	if len(blocks) == 0 && decoded.UsageMetadata.CandidatesTokenCount == 0 {
		return modelResponse{}, true, withProviderTrace(googleEmptyResponseError(decoded, "no tool calls or output tokens"), trace)
	}
	return modelResponse{
		Content: blocks,
		Usage: modelUsage{
			InputTokens:  decoded.UsageMetadata.PromptTokenCount,
			OutputTokens: decoded.UsageMetadata.CandidatesTokenCount,
		},
		ProviderTrace: trace,
	}, false, nil
}

// googleEmptyResponseError is an internal helper for the main package.
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

// googleFunctionDeclarations is an internal helper for the main package.
func googleFunctionDeclarations(tools []modelTool) []googleFunctionDeclaration {
	out := make([]googleFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		out = append(out, googleFunctionDeclaration{Name: tool.Name, Description: tool.Description, Parameters: sanitizeGoogleSchema(tool.InputSchema)})
	}
	return out
}

// googleFunctionCallingMode is an internal helper for the main package.
func googleFunctionCallingMode() string {
	mode := strings.ToUpper(strings.TrimSpace(os.Getenv("EVAL_GOOGLE_FUNCTION_MODE")))
	switch mode {
	case "AUTO", "ANY", "VALIDATED", "NONE":
		return mode
	default:
		return "VALIDATED"
	}
}

// googleContents is an internal helper for the main package.
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

// googleFunctionResponsePayload is an internal helper for the main package.
func googleFunctionResponsePayload(block modelContentBlock) map[string]any {
	response := map[string]any{"is_error": block.IsError}
	var parsed any
	if err := json.Unmarshal([]byte(block.Content), &parsed); err == nil {
		response["content"] = parsed
		if object, ok := parsed.(map[string]any); ok {
			for key, value := range object {
				if _, reserved := response[key]; reserved {
					continue
				}
				response[key] = value
			}
		}
		return response
	}
	response["content"] = block.Content
	return response
}

// googleToolUseBlocks is an internal helper for the main package.
func googleToolUseBlocks(content googleContent) []modelContentBlock {
	blocks := googleContentBlocks(content)
	out := make([]modelContentBlock, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "tool_use" {
			out = append(out, block)
		}
	}
	return out
}

// googleContentBlocks is an internal helper for the main package.
func googleContentBlocks(content googleContent) []modelContentBlock {
	blocks := make([]modelContentBlock, 0, len(content.Parts))
	for index, part := range content.Parts {
		if part.FunctionCall == nil {
			if part.Text != "" {
				blocks = append(blocks, modelContentBlock{Type: "text", Text: part.Text})
			}
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

// sanitizeGoogleSchema is an internal helper for the main package.
func sanitizeGoogleSchema(value any) any {
	return sanitizeGoogleSchemaValue(value)
}

// sanitizeGoogleSchemaValue is an internal helper for the main package.
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

// sanitizeGoogleSchemaProperties is an internal helper for the main package.
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

// sanitizeGoogleSchemaType is an internal helper for the main package.
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

// doModelRequest is an internal helper for the main package.
func doModelRequest(client *http.Client, req *http.Request, provider string) (body []byte, status int, retry bool, err error) {
	resp, err := client.Do(req) // #nosec G704 -- provider URLs come from explicit evaluator configuration, not model-generated input.
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || req.Context().Err() != nil {
			return nil, 0, false, fmt.Errorf("%s request: %w", provider, err)
		}
		return nil, 0, true, fmt.Errorf("%s request: %w", provider, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, true, fmt.Errorf("read %s response: %w", provider, err)
	}
	if len(respBody) > maxResponseBytes {
		return respBody, resp.StatusCode, false, fmt.Errorf("%s response exceeded %d bytes", provider, maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retry = resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return respBody, resp.StatusCode, retry, fmt.Errorf("%s status %d: %s", provider, resp.StatusCode, redactResponse(respBody))
	}
	return respBody, resp.StatusCode, false, nil
}

// newModelProviderTrace records a provider request body without request headers.
func newModelProviderTrace(provider, method, endpoint string, requestBody []byte) *modelProviderTrace {
	trace := &modelProviderTrace{Provider: provider, Method: method, Endpoint: endpoint}
	if len(bytes.TrimSpace(requestBody)) > 0 {
		trace.RequestBody = append(json.RawMessage(nil), requestBody...)
	}
	return trace
}

// setResponse records the provider response body in JSON form when possible.
func (t *modelProviderTrace) setResponse(status int, body []byte) {
	if t == nil {
		return
	}
	t.ResponseStatus = status
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return
	}
	if json.Valid(trimmed) {
		t.ResponseBody = append(json.RawMessage(nil), trimmed...)
		return
	}
	t.ResponseBodyText = string(body)
}

// withProviderTrace attaches provider exchange details to an error.
func withProviderTrace(err error, trace *modelProviderTrace) error {
	if err == nil || trace == nil {
		return err
	}
	return &modelProviderCallError{err: err, Trace: trace}
}
