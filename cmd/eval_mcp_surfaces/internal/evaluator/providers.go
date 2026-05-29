package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	// providerAnthropic identifies the provider anthropic constant used by this package.
	providerAnthropic = "anthropic"
	// providerGoogle identifies the provider google constant used by this package.
	providerGoogle = "google"
	// providerOpenAI identifies the provider OpenAI constant used by this package.
	providerOpenAI = "openai"
	// providerQwen identifies the provider qwen constant used by this package.
	providerQwen = "qwen"

	// openAIChatAPI identifies the OpenAI chat API constant used by this package.
	openAIChatAPI = "https://api.openai.com/v1/chat/completions"
	// qwenChatAPI identifies the qwen chat API constant used by this package.
	qwenChatAPI = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions"
	// geminiAPIBase identifies the gemini API base constant used by this package.
	geminiAPIBase = "https://generativelanguage.googleapis.com/v1beta/models/"

	// headerContentType identifies the header content type constant used by this package.
	headerContentType = "Content-Type"
	// contentTypeJSON identifies the content type JSON constant used by this package.
	contentTypeJSON = "application/json"
	// headerGoogleAuth identifies the header google auth constant used by this package.
	headerGoogleAuth = "x-goog-api-key"
)

// modelSpec holds model spec data for the evaluator package.
type modelSpec struct {
	Provider string
	Model    string
}

// String returns the display label for modelSpec.
func (s modelSpec) String() string {
	if s.Provider == "" {
		return s.Model
	}
	return s.Provider + ":" + s.Model
}

// modelProviderRequest captures model-provider model provider request data.
type modelProviderRequest struct {
	Model       string
	MaxTokens   int
	Temperature float64
	System      string
	Tools       []modelTool
	Messages    []modelMessage
	TraceBodies bool
}

// modelProvider defines the contract for model provider operations.
type modelProvider interface {
	callOnce(ctx context.Context, client *http.Client, apiKey string, request modelProviderRequest) (modelResponse, bool, error)
}

// resolveModelSpecs resolves model specs and returns [[]modelSpec].
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

// parseModelSpec handles parse model spec and returns [modelSpec].
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

// modelReportLabel prepares model report label for model-provider evaluation.
func modelReportLabel(specs []modelSpec) string {
	labels := make([]string, 0, len(specs))
	for _, spec := range specs {
		labels = append(labels, spec.String())
	}
	return strings.Join(labels, ",")
}

// apiKeyForModelProvider handles API key for model provider and returns [string].
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

// modelProviderFor prepares model provider for for model-provider evaluation.
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

// qwenEndpoint prepares qwen endpoint for model-provider evaluation.
func qwenEndpoint() string {
	if endpoint := strings.TrimSpace(os.Getenv("QWEN_CHAT_COMPLETIONS_URL")); endpoint != "" {
		return endpoint
	}
	if baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("QWEN_BASE_URL")), "/"); baseURL != "" {
		return baseURL + "/chat/completions"
	}
	return qwenChatAPI
}

// anthropicProvider models the Anthropic anthropic provider payload.
type anthropicProvider struct{}

// callOnce sends one model request through anthropicProvider and reports whether failures are retryable.
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
	trace := newModelProviderTrace("anthropic", http.MethodPost, anthropicAPI, body, request.TraceBodies)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPI, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new anthropic request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")

	respBody, status, retry, err := doModelRequest(client, req, "anthropic")
	trace.setResponse(status, respBody, request.TraceBodies)
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

// openAIProvider models the OpenAI-compatible OpenAI provider payload.
type openAIProvider struct {
	endpoint        string
	name            string
	maxTokenField   string
	disableThinking bool
}

// openAIRequest models the OpenAI-compatible OpenAI request payload.
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

// openAITool models the OpenAI-compatible OpenAI tool payload.
type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

// openAIFunction models the OpenAI-compatible OpenAI function payload.
type openAIFunction struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
	Strict      *bool  `json:"strict,omitempty"`
}

// openAIMessage models the OpenAI-compatible OpenAI message payload.
type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
}

// openAIToolCall models the OpenAI-compatible OpenAI tool call payload.
type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

// openAIFunctionCall models the OpenAI-compatible OpenAI function call payload.
type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// openAIResponse models the OpenAI-compatible OpenAI response payload.
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

// callOnce sends one model request through openAIProvider and reports whether failures are retryable.
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
	trace := newModelProviderTrace(p.name, http.MethodPost, p.endpoint, body, request.TraceBodies)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new %s request: %w", p.name, err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	respBody, status, retry, err := doModelRequest(client, req, p.name)
	trace.setResponse(status, respBody, request.TraceBodies)
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

// openAITools resolves OpenAI tools for evaluator execution.
func openAITools(tools []modelTool) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		function := openAIFunction{Name: tool.Name, Description: tool.Description, Parameters: openAIToolSchema(tool)}
		out = append(out, openAITool{Type: "function", Function: function})
	}
	return out
}

// openAIToolSchema derives OpenAI tool schema from tool schema metadata.
func openAIToolSchema(tool modelTool) any {
	schema, ok := tool.InputSchema.(map[string]any)
	if !ok || tool.Name != dynamicExecuteActionTool {
		return tool.InputSchema
	}
	updated := cloneOpenAISchema(schema)
	updated["required"] = requiredWithNames(updated["required"], "action", "params")
	updated["additionalProperties"] = false
	addOpenAIExecuteParamHints(updated)
	return updated
}

// cloneOpenAISchema derives clone OpenAI schema from tool schema metadata.
func cloneOpenAISchema(schema map[string]any) map[string]any {
	data, err := json.Marshal(schema)
	if err != nil {
		slog.Warn("failed to marshal OpenAI tool schema; using recursive schema fallback", "error", err)
		return deepCloneMap(schema)
	}
	var cloned map[string]any
	if unmarshalErr := json.Unmarshal(data, &cloned); unmarshalErr != nil {
		slog.Warn("failed to unmarshal OpenAI tool schema clone; using recursive schema fallback", "error", unmarshalErr)
		return deepCloneMap(schema)
	}
	return cloned
}

// deepCloneMap clones map without sharing mutable maps.
func deepCloneMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = deepCloneAny(value)
	}
	return cloned
}

// deepCloneAny clones any without sharing mutable maps.
func deepCloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return deepCloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = deepCloneAny(item)
		}
		return cloned
	default:
		return typed
	}
}

// addOpenAIExecuteParamHints builds add OpenAI execute param hints for retry and repair feedback.
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

// openAICommonExecuteParams derives OpenAI common execute params from task and schema inputs.
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

// requiredWithNames returns required with names names for provider schemas.
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

// openAIMessages builds OpenAI messages for retry and repair feedback.
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

// openAIAssistantMessage builds OpenAI assistant message for retry and repair feedback.
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

// openAIUserOrToolMessages builds OpenAI user or tool messages for retry and repair feedback.
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

// openAIToolUseBlocks coordinates OpenAI tool use blocks and returns [[]modelContentBlock].
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

// parseOpenAIToolArguments coordinates parse OpenAI tool arguments and returns [map[string]any].
func parseOpenAIToolArguments(arguments string) (map[string]any, error) {
	input := map[string]any{}
	if err := json.Unmarshal([]byte(arguments), &input); err == nil {
		return input, nil
	}
	candidate := strings.TrimSpace(arguments)
	if parsedInput, ok := parsePrefixedOpenAIJSONObject(candidate); ok {
		return parsedInput, nil
	}
	if parsedInput, ok := parseOpenAIJSONObjectFragment(candidate); ok {
		return parsedInput, nil
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
		if repaired := repairOpenAIArgumentValueCommas(candidate); repaired != candidate {
			if json.Unmarshal([]byte(repaired), &input) == nil {
				return input, nil
			}
		}
		return nil, err
	}
	return input, nil
}

func repairOpenAIArgumentValueCommas(candidate string) string {
	var out strings.Builder
	changed := false
	for index := 0; index < len(candidate); index++ {
		char := candidate[index]
		out.WriteByte(char)
		if char != ':' && char != '{' && char != '[' {
			continue
		}
		next := index + 1
		for next < len(candidate) && isJSONWhitespace(candidate[next]) {
			out.WriteByte(candidate[next])
			next++
		}
		if next >= len(candidate) || candidate[next] != ',' {
			index = next - 1
			continue
		}
		afterComma := next + 1
		for afterComma < len(candidate) && isJSONWhitespace(candidate[afterComma]) {
			afterComma++
		}
		if afterComma >= len(candidate) {
			index = next
			continue
		}
		changed = true
		if char == ':' && !lastBuilderByteIsWhitespace(&out) {
			out.WriteByte(' ')
		}
		index = afterComma - 1
	}
	if !changed {
		return candidate
	}
	return out.String()
}

func isJSONWhitespace(char byte) bool {
	return char == ' ' || char == '\t' || char == '\r' || char == '\n'
}

func lastBuilderByteIsWhitespace(builder *strings.Builder) bool {
	if builder.Len() == 0 {
		return false
	}
	s := builder.String()
	return isJSONWhitespace(s[len(s)-1])
}

func parseOpenAIJSONObjectFragment(candidate string) (map[string]any, bool) {
	start := strings.Index(candidate, "\"")
	if start < 0 {
		return nil, false
	}
	fragment := strings.Trim(candidate[start:], " \t\r\n,`")
	if fragment == "" {
		return nil, false
	}
	for _, raw := range []string{"{" + fragment, "{" + strings.TrimRight(strings.TrimSpace(fragment), ",") + "}"} {
		input := map[string]any{}
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			return input, true
		}
	}
	return nil, false
}

func parsePrefixedOpenAIJSONObject(candidate string) (map[string]any, bool) {
	start := strings.Index(candidate, "{")
	if start < 0 {
		return nil, false
	}
	prefix := strings.TrimSpace(candidate[:start])
	if strings.ContainsAny(prefix, "\":") {
		return nil, false
	}
	end := strings.LastIndex(candidate, "}")
	if end <= start {
		return nil, false
	}
	input := map[string]any{}
	if err := json.Unmarshal([]byte(candidate[start:end+1]), &input); err != nil {
		return nil, false
	}
	return input, true
}

// googleProvider models the Google Gemini Google provider payload.
type googleProvider struct{}

// googleRequest models the Google Gemini Google request payload.
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

// googleContent models the Google Gemini Google content payload.
type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

// googlePart models the Google Gemini Google part payload.
type googlePart struct {
	ThoughtSignature string                  `json:"thoughtSignature,omitempty"`
	Text             string                  `json:"text,omitempty"`
	FunctionCall     *googleFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *googleFunctionResponse `json:"functionResponse,omitempty"`
}

// googleFunctionCall models the Google Gemini Google function call payload.
type googleFunctionCall struct {
	Name    string
	Args    map[string]any
	RawArgs json.RawMessage
	ID      string
}

// UnmarshalJSON decodes googleFunctionCall from the provider JSON shape.
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

// MarshalJSON encodes googleFunctionCall into the JSON shape expected by the provider.
func (c googleFunctionCall) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name string         `json:"name"`
		Args map[string]any `json:"args,omitempty"`
		ID   string         `json:"id,omitempty"`
	}{Name: c.Name, Args: c.Args, ID: c.ID})
}

// googleFunctionResponse models the Google Gemini Google function response payload.
type googleFunctionResponse struct {
	ID       string         `json:"id,omitempty"`
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// googleTool models the Google Gemini Google tool payload.
type googleTool struct {
	FunctionDeclarations []googleFunctionDeclaration `json:"function_declarations"`
}

// googleFunctionDeclaration models the Google Gemini Google function declaration payload.
type googleFunctionDeclaration struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

// googleToolConfig models the Google Gemini Google tool config payload.
type googleToolConfig struct {
	FunctionCallingConfig struct {
		Mode string `json:"mode"`
	} `json:"function_calling_config"`
}

// googleResponse models the Google Gemini Google response payload.
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

// callOnce sends one model request through googleProvider and reports whether failures are retryable.
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
	trace := newModelProviderTrace("google", http.MethodPost, endpoint, body, request.TraceBodies)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return modelResponse{}, false, fmt.Errorf("new google request: %w", err)
	}
	req.Header.Set(headerContentType, contentTypeJSON)
	req.Header.Set(headerGoogleAuth, apiKey)

	respBody, status, retry, err := doModelRequest(client, req, "google")
	trace.setResponse(status, respBody, request.TraceBodies)
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

// googleEmptyResponseError prepares google empty response error for model-provider evaluation.
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

// googleFunctionDeclarations prepares google function declarations for model-provider evaluation.
func googleFunctionDeclarations(tools []modelTool) []googleFunctionDeclaration {
	out := make([]googleFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		out = append(out, googleFunctionDeclaration{Name: tool.Name, Description: tool.Description, Parameters: sanitizeGoogleSchema(tool.InputSchema)})
	}
	return out
}

// googleFunctionCallingMode prepares google function calling mode for model-provider evaluation.
func googleFunctionCallingMode() string {
	mode := strings.ToUpper(strings.TrimSpace(os.Getenv("EVAL_GOOGLE_FUNCTION_MODE")))
	switch mode {
	case "AUTO", "ANY", "VALIDATED", "NONE":
		return mode
	default:
		return "VALIDATED"
	}
}

// googleContents prepares google contents for model-provider evaluation.
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

// googleFunctionResponsePayload prepares google function response payload for model-provider evaluation.
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

// googleToolUseBlocks prepares google tool use blocks for model-provider evaluation.
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

// googleContentBlocks prepares google content blocks for model-provider evaluation.
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

// sanitizeGoogleSchema sanitizes google schema for provider compatibility.
func sanitizeGoogleSchema(value any) any {
	return sanitizeGoogleSchemaValue(value)
}

// sanitizeGoogleSchemaValue sanitizes google schema value for provider compatibility.
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

// sanitizeGoogleSchemaProperties sanitizes google schema properties for provider compatibility.
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

// sanitizeGoogleSchemaType sanitizes google schema type for provider compatibility.
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

// doModelRequest prepares do model request for model-provider evaluation.
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

// newModelProviderTrace records provider metadata and optionally captures raw
// request bodies for explicit debugging sessions.
func newModelProviderTrace(provider, method, endpoint string, requestBody []byte, includeBody bool) *modelProviderTrace {
	trace := &modelProviderTrace{Provider: provider, Method: method, Endpoint: endpoint}
	if includeBody && len(bytes.TrimSpace(requestBody)) > 0 {
		trace.RequestBody = append(json.RawMessage(nil), requestBody...)
	}
	return trace
}

// setResponse records provider status and optionally captures raw response
// bodies for explicit debugging sessions.
func (t *modelProviderTrace) setResponse(status int, body []byte, includeBody bool) {
	if t == nil {
		return
	}
	t.ResponseStatus = status
	if !includeBody {
		return
	}
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
