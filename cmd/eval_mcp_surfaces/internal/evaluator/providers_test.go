package evaluator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveModelSpecs_ParsesConfiguredSources verifies configured model
// sources are resolved in priority order and normalized by provider.
func TestResolveModelSpecs_ParsesConfiguredSources(t *testing.T) {
	t.Setenv("EVAL_MODELS", "openai:gpt-env")
	specs, err := resolveModelSpecs(options{Model: " google:models/gemini-test , qwen:qwen-max "})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	if len(specs) != 2 || specs[0].String() != "google:gemini-test" || specs[1].String() != "qwen:qwen-max" {
		t.Fatalf("specs = %#v, want google and qwen from --model", specs)
	}

	specs, err = resolveModelSpecs(options{Models: "openai:gpt-list"})
	if err != nil {
		t.Fatalf("resolveModelSpecs(--models) error = %v", err)
	}
	if len(specs) != 1 || specs[0].String() != "openai:gpt-list" {
		t.Fatalf("specs = %#v, want openai:gpt-list", specs)
	}

	specs, err = resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs(env) error = %v", err)
	}
	if len(specs) != 1 || specs[0].String() != "openai:gpt-env" {
		t.Fatalf("specs = %#v, want openai:gpt-env", specs)
	}

	t.Setenv("EVAL_MODELS", "")
	t.Setenv("ANTHROPIC_MODEL", "claude-env")
	specs, err = resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs(legacy) error = %v", err)
	}
	if len(specs) != 1 || specs[0].String() != "anthropic:claude-env" {
		t.Fatalf("specs = %#v, want anthropic legacy model", specs)
	}
}

// TestResolveModelSpecs_RejectsEmptySource verifies comma-only model lists do
// not silently fall back after an explicit source is configured.
func TestResolveModelSpecs_RejectsEmptySource(t *testing.T) {
	if _, err := resolveModelSpecs(options{Model: " , "}); err == nil {
		t.Fatal("resolveModelSpecs(comma-only) error = nil, want error")
	}
	if _, err := resolveModelSpecs(options{Model: "bad:model"}); err == nil {
		t.Fatal("resolveModelSpecs(bad provider) error = nil, want error")
	}
	t.Setenv("EVAL_MODELS", "")
	t.Setenv("ANTHROPIC_MODEL", "")
	if specs, err := resolveModelSpecs(options{}); err != nil || len(specs) != 1 || specs[0].String() != defaultModel {
		t.Fatalf("resolveModelSpecs(default) = %#v, %v; want %s", specs, err, defaultModel)
	}
}

// TestModelSpecStringAndReportLabel verifies model string rendering for
// provider-prefixed and unprefixed model specs.
func TestModelSpecStringAndReportLabel(t *testing.T) {
	if got := (modelSpec{Model: "claude"}).String(); got != "claude" {
		t.Fatalf("String(no provider) = %q, want claude", got)
	}
	label := modelReportLabel([]modelSpec{{Provider: providerOpenAI, Model: "gpt"}, {Model: "plain"}})
	if label != "openai:gpt,plain" {
		t.Fatalf("modelReportLabel() = %q", label)
	}
}

// TestParseModelSpec_Errors verifies malformed model specs produce actionable
// configuration errors instead of silently selecting a provider.
func TestParseModelSpec_Errors(t *testing.T) {
	for _, raw := range []string{"", "openai:", "unknown:model"} {
		if _, err := parseModelSpec(raw); err == nil {
			t.Fatalf("parseModelSpec(%q) error = nil, want error", raw)
		}
	}
}

// TestAPIKeyForModelProvider_EnvAndErrors verifies provider key lookup accepts
// supported providers and reports unsupported or missing configuration.
func TestAPIKeyForModelProvider_EnvAndErrors(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", " openai-key ")
	t.Setenv("QWEN_API_KEY", "")
	key, err := apiKeyForModelProvider(providerOpenAI)
	if err != nil {
		t.Fatalf("apiKeyForModelProvider(openai) error = %v", err)
	}
	if key != "openai-key" {
		t.Fatalf("key = %q, want trimmed openai-key", key)
	}
	if _, unsupportedErr := apiKeyForModelProvider("unknown"); unsupportedErr == nil {
		t.Fatal("apiKeyForModelProvider(unknown) error = nil, want error")
	}
	if _, missingErr := apiKeyForModelProvider(providerQwen); missingErr == nil {
		t.Fatal("apiKeyForModelProvider(qwen without env) error = nil, want error")
	}
}

// TestModelProviderForAndQwenEndpoint verifies provider dispatch and Qwen URL
// override behavior.
func TestModelProviderForAndQwenEndpoint(t *testing.T) {
	if _, ok := modelProviderFor(providerGoogle).(googleProvider); !ok {
		t.Fatalf("modelProviderFor(google) = %T, want googleProvider", modelProviderFor(providerGoogle))
	}
	if provider := modelProviderFor(providerOpenAI).(openAIProvider); provider.name != providerOpenAI || provider.maxTokenField != "max_completion_tokens" || provider.disableThinking {
		t.Fatalf("openai provider = %+v", provider)
	}
	if provider := modelProviderFor(providerQwen).(openAIProvider); provider.name != providerQwen || !provider.disableThinking || provider.maxTokenField != "max_tokens" {
		t.Fatalf("qwen provider = %+v", provider)
	}
	if _, ok := modelProviderFor("unknown").(anthropicProvider); !ok {
		t.Fatalf("modelProviderFor(unknown) = %T, want anthropicProvider", modelProviderFor("unknown"))
	}

	t.Setenv("QWEN_BASE_URL", "https://qwen.example/v1/")
	if got := qwenEndpoint(); got != "https://qwen.example/v1/chat/completions" {
		t.Fatalf("qwenEndpoint(base) = %q", got)
	}
	t.Setenv("QWEN_CHAT_COMPLETIONS_URL", "https://override.example/chat")
	if got := qwenEndpoint(); got != "https://override.example/chat" {
		t.Fatalf("qwenEndpoint(override) = %q", got)
	}
}

// TestAnthropicProviderCallOnce_ResponseBranches verifies Anthropic request
// setup, API errors, decode errors, and marshal errors.
func TestAnthropicProviderCallOnce_ResponseBranches(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		request   modelProviderRequest
		wantError bool
	}{
		{name: "success", body: `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":2,"output_tokens":3}}`, request: modelProviderRequest{Model: "claude", MaxTokens: 16}},
		{name: "api error", body: `{"error":{"type":"invalid_request","message":"bad"}}`, request: modelProviderRequest{Model: "claude", MaxTokens: 16}, wantError: true},
		{name: "decode", body: `{`, request: modelProviderRequest{Model: "claude", MaxTokens: 16}, wantError: true},
		{name: "marshal", request: modelProviderRequest{Model: "claude", MaxTokens: 16, Tools: []modelTool{{Name: "bad", InputSchema: func() {}}}}, wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.Header.Get("x-api-key"); got != "secret-key" {
					t.Fatalf("x-api-key = %q, want secret-key", got)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			response, retry, err := anthropicProvider{}.callOnce(context.Background(), client, "secret-key", tc.request)
			if tc.wantError {
				if err == nil {
					t.Fatal("callOnce() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("callOnce() error = %v", err)
			}
			if retry {
				t.Fatal("retry = true, want false")
			}
			if response.Usage.InputTokens != 2 || response.Usage.OutputTokens != 3 {
				t.Fatalf("usage = %+v, want 2/3", response.Usage)
			}
		})
	}
}

// TestAnthropicProviderCallOnce_RequestFailureIsRetryable verifies transport
// failures propagate the retry decision from doModelRequest.
func TestAnthropicProviderCallOnce_RequestFailureIsRetryable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("temporary network failure")
	})}
	_, retry, err := anthropicProvider{}.callOnce(context.Background(), client, "secret-key", modelProviderRequest{Model: "claude", MaxTokens: 16})
	if err == nil {
		t.Fatal("callOnce() error = nil, want error")
	}
	if !retry {
		t.Fatal("retry = false, want true")
	}
}

// TestAnthropicProviderCallOnce_SerializesEmptyToolUseInput verifies Anthropic history keeps input on no-parameter tool calls.
func TestAnthropicProviderCallOnce_SerializesEmptyToolUseInput(t *testing.T) {
	var requestBody []byte
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requestBody = body
		response := `{"content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(response)), Header: make(http.Header)}, nil
	})}

	_, _, err := anthropicProvider{}.callOnce(context.Background(), client, "secret-key", modelProviderRequest{
		Model:     "claude",
		MaxTokens: 16,
		Messages:  []modelMessage{{Role: "assistant", Content: []modelContentBlock{{Type: "tool_use", ID: "toolu_1", Name: capabilityListTool}}}},
	})
	if err != nil {
		t.Fatalf("callOnce() error = %v", err)
	}
	if !bytes.Contains(requestBody, []byte(`"input":{}`)) {
		t.Fatalf("request body = %s, want empty tool_use input object", requestBody)
	}
}

// TestDeepCloneMap_DoesNotShareNestedContainers verifies OpenAI schema cloning
// can mutate retry hints without altering the catalog schema.
func TestDeepCloneMap_DoesNotShareNestedContainers(t *testing.T) {
	original := map[string]any{
		"properties": map[string]any{"params": map[string]any{"type": "object"}},
		"required":   []any{"action"},
		"unchanged":  "value",
	}
	cloned := deepCloneMap(original)
	cloned["properties"].(map[string]any)["params"].(map[string]any)["description"] = "hint"
	cloned["required"].([]any)[0] = "params"

	if _, ok := original["properties"].(map[string]any)["params"].(map[string]any)["description"]; ok {
		t.Fatalf("original properties mutated: %#v", original)
	}
	if original["required"].([]any)[0] != "action" {
		t.Fatalf("original required mutated: %#v", original["required"])
	}
	if deepCloneMap(nil) != nil || deepCloneAny("plain") != "plain" {
		t.Fatal("deep clone nil/scalar behavior changed")
	}
}

// TestParseOpenAIToolArguments_WrapsMissingOpeningBrace verifies ParseOpenAIToolArguments when wraps missing opening brace.
func TestParseOpenAIToolArguments_WrapsMissingOpeningBrace(t *testing.T) {
	input, err := parseOpenAIToolArguments(`"project_id":"42"}`)
	if err != nil {
		t.Fatalf("parseOpenAIToolArguments() error = %v", err)
	}
	if got := input["project_id"]; got != "42" {
		t.Fatalf("project_id = %v, want 42", got)
	}

	prefixed, err := parseOpenAIToolArguments(", {\"action\":\"service_account_update\",\"params\":{\"project_id\":\"42\"}}")
	if err != nil {
		t.Fatalf("parseOpenAIToolArguments(prefixed) error = %v", err)
	}
	if prefixed["action"] != "service_account_update" {
		t.Fatalf("prefixed action = %v, want service_account_update", prefixed["action"])
	}

	fragment, err := parseOpenAIToolArguments("`,\n \"action\":\"service_account_update\",\"params\":{\"project_id\":\"42\"},`")
	if err != nil {
		t.Fatalf("parseOpenAIToolArguments(fragment) error = %v", err)
	}
	if fragment["action"] != "service_account_update" {
		t.Fatalf("fragment action = %v, want service_account_update", fragment["action"])
	}
}

// TestParseOpenAIToolArguments_RepairsCommaBeforeValues verifies recovery for
// OpenAI-compatible providers that emit a stray comma before a JSON value.
func TestParseOpenAIToolArguments_RepairsCommaBeforeValues(t *testing.T) {
	input, err := parseOpenAIToolArguments(`{"action":,"create","params":,{"project_id":"42","deploy_access_levels":[,{"access_level":40}]}}`)
	if err != nil {
		t.Fatalf("parseOpenAIToolArguments() error = %v", err)
	}
	if input["action"] != "create" {
		t.Fatalf("action = %v, want create", input["action"])
	}
	params, ok := input["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v, want object", input["params"])
	}
	levels, ok := params["deploy_access_levels"].([]any)
	if !ok || len(levels) != 1 {
		t.Fatalf("deploy_access_levels = %#v, want one level", params["deploy_access_levels"])
	}
}

// TestGoogleProviderCallOnce_SendsAPIKeyHeader verifies GoogleProviderCallOnce when sends API key header.
func TestGoogleProviderCallOnce_SendsAPIKeyHeader(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.RawQuery != "" {
			t.Fatalf("RawQuery = %q, want empty", req.URL.RawQuery)
		}
		if got := req.Header.Get(headerGoogleAuth); got != "secret-key" {
			t.Fatalf("%s = %q, want secret-key", headerGoogleAuth, got)
		}
		if got := req.Header.Get(headerContentType); got != contentTypeJSON {
			t.Fatalf("%s = %q, want %s", headerContentType, got, contentTypeJSON)
		}
		body := `{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}

	response, retry, err := googleProvider{}.callOnce(context.Background(), client, "secret-key", modelProviderRequest{
		Model:     "gemini-test",
		MaxTokens: 32,
		System:    "system",
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("callOnce() error = %v", err)
	}
	if retry {
		t.Fatal("retry = true, want false")
	}
	if response.Usage.InputTokens != 1 || response.Usage.OutputTokens != 1 {
		t.Fatalf("usage = %+v, want input/output tokens 1", response.Usage)
	}
}

// TestGoogleProviderCallOnce_ResponseBranches verifies Google API/decode/empty
// response branches and retry decisions.
func TestGoogleProviderCallOnce_ResponseBranches(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantRetry bool
	}{
		{name: "api error", body: `{"error":{"status":"INVALID_ARGUMENT","message":"bad"}}`},
		{name: "decode", body: `{`},
		{name: "no candidates", body: `{"promptFeedback":{"blockReason":"SAFETY"}}`, wantRetry: true},
		{name: "empty candidate", body: `{"candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":0}}`, wantRetry: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, retry, err := googleProvider{}.callOnce(context.Background(), client, "key", modelProviderRequest{Model: "gemini", MaxTokens: 8, TraceBodies: true})
			if err == nil {
				t.Fatal("callOnce() error = nil, want error")
			}
			if retry != tc.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, tc.wantRetry)
			}
			var providerErr *modelProviderCallError
			if !errors.As(err, &providerErr) || providerErr.Trace == nil {
				t.Fatalf("error = %v, want provider trace", err)
			}
			rawBody := string(providerErr.Trace.ResponseBody)
			if rawBody == "" {
				rawBody = providerErr.Trace.ResponseBodyText
			}
			if providerErr.Trace.ResponseStatus != http.StatusOK || !strings.Contains(rawBody, strings.TrimSpace(tc.body)) {
				t.Fatalf("provider trace = %+v, want raw error body", providerErr.Trace)
			}
		})
	}
}

// TestGoogleProviderCallOnce_RequestFailureBranches verifies local marshal and
// transport failures are returned with the expected retry metadata.
func TestGoogleProviderCallOnce_RequestFailureBranches(t *testing.T) {
	if _, _, err := (googleProvider{}).callOnce(context.Background(), http.DefaultClient, "key", modelProviderRequest{Model: "gemini", Tools: []modelTool{{Name: "bad", InputSchema: func() {}}}}); err == nil {
		t.Fatal("callOnce(marshal) error = nil, want error")
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("temporary network failure")
	})}
	_, retry, err := googleProvider{}.callOnce(context.Background(), client, "key", modelProviderRequest{Model: "gemini", MaxTokens: 8})
	if err == nil {
		t.Fatal("callOnce(transport) error = nil, want error")
	}
	if !retry {
		t.Fatal("retry = false, want true")
	}
}

// TestGoogleFunctionResponsePayload_PreservesErrorFlag verifies GoogleFunctionResponsePayload preserves error flag.
func TestGoogleFunctionResponsePayload_PreservesErrorFlag(t *testing.T) {
	payload := googleFunctionResponsePayload(modelContentBlock{Content: `{"is_error":false,"value":7}`, IsError: true})
	if got := payload["is_error"]; got != true {
		t.Fatalf("is_error = %v, want true", got)
	}
	if got := payload["value"]; got != float64(7) {
		t.Fatalf("value = %v, want 7", got)
	}
}

// TestDoModelRequest_ContextCancellationIsNotRetryable verifies DoModelRequest when context cancellation is not retryable.
func TestDoModelRequest_ContextCancellationIsNotRetryable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	_, _, retry, err := doModelRequest(client, req, "test")
	if err == nil {
		t.Fatal("doModelRequest() error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if retry {
		t.Fatal("retry = true, want false")
	}
}

// TestOpenAIProviderCallOnce_BuildsRequestAndParsesToolCall verifies the
// OpenAI-compatible provider shapes requests and parses tool calls.
func TestOpenAIProviderCallOnce_BuildsRequestAndParsesToolCall(t *testing.T) {
	client := openAIProviderTestClient(t)

	response, retry, err := openAIProvider{endpoint: "https://qwen.example/chat", name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}.callOnce(context.Background(), client, "secret-key", modelProviderRequest{
		Model:       "qwen-max",
		MaxTokens:   32,
		System:      "system",
		Tools:       []modelTool{{Name: "gitlab_project", Description: "Project", InputSchema: map[string]any{"type": "object"}}},
		Messages:    []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "hello"}}}},
		TraceBodies: true,
	})
	if err != nil {
		t.Fatalf("callOnce() error = %v", err)
	}
	if retry {
		t.Fatal("retry = true, want false")
	}
	assertOpenAIProviderResponse(t, response)
}

func openAIProviderTestClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		assertOpenAIProviderRequest(t, req)
		responseBody := `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"gitlab_project","arguments":"{\"project_id\":\"42\"}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(responseBody)), Header: make(http.Header)}, nil
	})}
}

func assertOpenAIProviderRequest(t *testing.T, req *http.Request) {
	t.Helper()
	if got := req.Header.Get("Authorization"); got != "Bearer secret-key" {
		t.Fatalf("Authorization = %q, want bearer key", got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var payload openAIRequest
	if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
		t.Fatalf("decode payload: %v", decodeErr)
	}
	if payload.MaxTokens != 32 || payload.MaxCompletionTokens != 0 {
		t.Fatalf("token fields = max %d completion %d", payload.MaxTokens, payload.MaxCompletionTokens)
	}
	if payload.EnableThinking == nil || *payload.EnableThinking {
		t.Fatalf("EnableThinking = %#v, want false pointer", payload.EnableThinking)
	}
	if len(payload.Tools) != 1 || payload.Tools[0].Function.Name != "gitlab_project" {
		t.Fatalf("tools = %#v", payload.Tools)
	}
}

func assertOpenAIProviderResponse(t *testing.T, response modelResponse) {
	t.Helper()
	if len(response.Content) != 1 || response.Content[0].Name != "gitlab_project" || response.Content[0].Input["project_id"] != "42" {
		t.Fatalf("content = %#v", response.Content)
	}
	if response.Usage.InputTokens != 3 || response.Usage.OutputTokens != 4 {
		t.Fatalf("usage = %+v, want 3/4", response.Usage)
	}
	if response.ProviderTrace == nil {
		t.Fatal("provider trace = nil, want raw exchange")
	}
	if response.ProviderTrace.Provider != providerQwen || response.ProviderTrace.ResponseStatus != http.StatusOK {
		t.Fatalf("provider trace = %+v, want qwen 200", response.ProviderTrace)
	}
	if !strings.Contains(string(response.ProviderTrace.RequestBody), `"enable_thinking":false`) {
		t.Fatalf("request trace = %s, want enable_thinking false", response.ProviderTrace.RequestBody)
	}
	if !strings.Contains(string(response.ProviderTrace.ResponseBody), `"tool_calls"`) {
		t.Fatalf("response trace = %s, want raw tool_calls", response.ProviderTrace.ResponseBody)
	}
}

// TestOpenAITools_HardensExecuteSchema verifies OpenAI-compatible function
// calling receives the complete dynamic executor envelope schema.
func TestOpenAITools_HardensExecuteSchema(t *testing.T) {
	tools := openAITools([]modelTool{
		{
			Name:        dynamicExecuteActionTool,
			Description: "Execute",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"action":  map[string]any{"type": "string"},
					"params":  map[string]any{"type": "object"},
					"confirm": map[string]any{"type": "boolean"},
				},
				"required": []any{"action"},
			},
		},
	})

	if len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", tools)
	}
	if tools[0].Function.Strict != nil {
		t.Fatalf("strict = %#v, want nil for dynamic execute schema with flexible params", tools[0].Function.Strict)
	}
	schema, ok := tools[0].Function.Parameters.(map[string]any)
	if !ok {
		t.Fatalf("parameters = %T, want map schema", tools[0].Function.Parameters)
	}
	if schema["additionalProperties"] != false {
		t.Fatalf("additionalProperties = %#v, want false", schema["additionalProperties"])
	}
	required := requiredStringSet(schema["required"])
	if !required["action"] || !required["params"] {
		t.Fatalf("required = %#v, want action and params", schema["required"])
	}
	if required["confirm"] {
		t.Fatalf("required = %#v, confirm must remain optional", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	paramsSchema := properties["params"].(map[string]any)
	if paramsSchema["additionalProperties"] != true {
		t.Fatalf("params additionalProperties = %#v, want true for dynamic action params", paramsSchema["additionalProperties"])
	}
	paramProperties := paramsSchema["properties"].(map[string]any)
	for _, want := range []string{"project_id", "trigger_id", "schedule_id", "file_path", "ref"} {
		if _, hasProperty := paramProperties[want]; !hasProperty {
			t.Fatalf("params properties missing %s: %#v", want, paramProperties)
		}
	}
}

// TestOpenAITools_QwenKeepsStrictDisabled verifies Qwen receives the hardened
// execute schema without OpenAI-specific strict metadata.
func TestOpenAITools_QwenKeepsStrictDisabled(t *testing.T) {
	tools := openAITools([]modelTool{{Name: dynamicExecuteActionTool, InputSchema: map[string]any{"type": "object"}}})

	if len(tools) != 1 {
		t.Fatalf("tools = %#v, want one tool", tools)
	}
	if tools[0].Function.Strict != nil {
		t.Fatalf("strict = %#v, want nil for Qwen", tools[0].Function.Strict)
	}
	schema := tools[0].Function.Parameters.(map[string]any)
	if required := requiredStringSet(schema["required"]); !required["action"] || !required["params"] {
		t.Fatalf("required = %#v, want action and params", schema["required"])
	}
}

// TestOpenAITools_DoesNotMutateNonExecuteSchemas verifies hardening is limited
// to gitlab_execute_action.
func TestOpenAITools_DoesNotMutateNonExecuteSchemas(t *testing.T) {
	inputSchema := map[string]any{"type": "object", "required": []any{"query"}}
	tools := openAITools([]modelTool{{Name: dynamicFindTool, InputSchema: inputSchema}})

	if tools[0].Function.Strict != nil {
		t.Fatalf("strict = %#v, want nil for find tool", tools[0].Function.Strict)
	}
	if _, ok := tools[0].Function.Parameters.(map[string]any)["additionalProperties"]; ok {
		t.Fatalf("parameters = %#v, non-execute schema should not be hardened", tools[0].Function.Parameters)
	}
	if strings.Join(requiredNamesFromAny(tools[0].Function.Parameters.(map[string]any)["required"]), ",") != "query" {
		t.Fatalf("parameters = %#v, required params changed", tools[0].Function.Parameters)
	}
}

// TestProviderCallOnce_HTTPErrorTraceIncludesRequestAndRawResponse verifies
// provider failures keep enough HTTP context for trace artifact debugging.
func TestProviderCallOnce_HTTPErrorTraceIncludesRequestAndRawResponse(t *testing.T) {
	const responseText = "temporary upstream outage"
	tests := []struct {
		name         string
		provider     modelProvider
		request      modelProviderRequest
		wantProvider string
	}{
		{name: "anthropic", provider: anthropicProvider{}, request: modelProviderRequest{Model: "claude", MaxTokens: 16, TraceBodies: true}, wantProvider: "anthropic"},
		{name: "openai", provider: openAIProvider{endpoint: "https://openai.example/chat", name: providerOpenAI, maxTokenField: "max_completion_tokens"}, request: modelProviderRequest{Model: "gpt", MaxTokens: 16, TraceBodies: true}, wantProvider: providerOpenAI},
		{name: "google", provider: googleProvider{}, request: modelProviderRequest{Model: "gemini", MaxTokens: 16, TraceBodies: true}, wantProvider: "google"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertProviderHTTPErrorTrace(t, tt.provider, tt.request, tt.wantProvider, responseText)
		})
	}
}

func assertProviderHTTPErrorTrace(t *testing.T, provider modelProvider, request modelProviderRequest, wantProvider, responseText string) {
	t.Helper()
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader(responseText)), Header: make(http.Header)}, nil
	})}

	_, retry, err := provider.callOnce(context.Background(), client, "secret-key", request)
	if err == nil {
		t.Fatal("callOnce() error = nil, want provider error")
	}
	if !retry {
		t.Fatal("retry = false, want true for 503")
	}
	var providerErr *modelProviderCallError
	if !errors.As(err, &providerErr) || providerErr.Trace == nil {
		t.Fatalf("error = %v, want provider trace", err)
	}
	assertProviderTrace(t, providerErr.Trace, request, wantProvider, responseText)
}

func assertProviderTrace(t *testing.T, trace *modelProviderTrace, request modelProviderRequest, wantProvider, responseText string) {
	t.Helper()
	if trace.Provider != wantProvider || trace.Method != http.MethodPost {
		t.Fatalf("trace = %+v, want provider %s POST", trace, wantProvider)
	}
	if trace.ResponseStatus != http.StatusServiceUnavailable || trace.ResponseBodyText != responseText {
		t.Fatalf("trace = %+v, want raw non-JSON 503 response", trace)
	}
	if len(trace.RequestBody) == 0 {
		t.Fatal("request trace is empty, want serialized provider request")
	}
	if !strings.Contains(string(trace.RequestBody), request.Model) && !strings.Contains(trace.Endpoint, request.Model) {
		t.Fatalf("trace request = %s endpoint = %s, want model name", trace.RequestBody, trace.Endpoint)
	}
}

// TestProviderCallOnce_DefaultTraceOmitsRawBodies verifies trace artifacts keep
// provider metadata by default without storing prompt or tool-call payloads.
func TestProviderCallOnce_DefaultTraceOmitsRawBodies(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusServiceUnavailable, Body: io.NopCloser(strings.NewReader("temporary upstream outage")), Header: make(http.Header)}, nil
	})}

	_, _, err := (openAIProvider{endpoint: "https://openai.example/chat", name: providerOpenAI, maxTokenField: "max_completion_tokens"}).callOnce(context.Background(), client, "secret-key", modelProviderRequest{
		Model:     "gpt",
		MaxTokens: 16,
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "sensitive prompt"}}}},
	})
	if err == nil {
		t.Fatal("callOnce() error = nil, want provider error")
	}
	var providerErr *modelProviderCallError
	if !errors.As(err, &providerErr) || providerErr.Trace == nil {
		t.Fatalf("error = %v, want provider trace", err)
	}
	trace := providerErr.Trace
	if trace.ResponseStatus != http.StatusServiceUnavailable {
		t.Fatalf("response status = %d, want 503", trace.ResponseStatus)
	}
	if len(trace.RequestBody) != 0 || len(trace.ResponseBody) != 0 || trace.ResponseBodyText != "" {
		t.Fatalf("trace bodies = request %q response %q text %q, want omitted", trace.RequestBody, trace.ResponseBody, trace.ResponseBodyText)
	}
}

// requiredStringSet returns d string set test data or fails the test.
func requiredStringSet(raw any) map[string]bool {
	out := map[string]bool{}
	for _, name := range requiredNamesFromAny(raw) {
		out[name] = true
	}
	return out
}

// requiredNamesFromAny returns d names from any test data or fails the test.
func requiredNamesFromAny(raw any) []string {
	var names []string
	switch values := raw.(type) {
	case []any:
		for _, value := range values {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
	case []string:
		names = append(names, values...)
	}
	return names
}

// TestOpenAIProviderCallOnce_ResponseErrors verifies OpenAI-compatible error
// responses and invalid tool call payloads are surfaced correctly.
func TestOpenAIProviderCallOnce_ResponseErrors(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantRetry bool
	}{
		{name: "api error", body: `{"error":{"type":"invalid_request","message":"bad request"}}`},
		{name: "no choices", body: `{"choices":[]}`},
		{name: "invalid tool args", wantRetry: true, body: `{"choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"tool","arguments":"not json"}}]}}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, retry, err := openAIProvider{endpoint: "https://openai.example/chat", name: providerOpenAI, maxTokenField: "max_completion_tokens"}.callOnce(context.Background(), client, "key", modelProviderRequest{Model: "gpt", MaxTokens: 8})
			if err == nil {
				t.Fatal("callOnce() error = nil, want error")
			}
			if retry != tc.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, tc.wantRetry)
			}
		})
	}
}

// TestOpenAIProviderCallOnce_RequestConstructionErrors verifies local request
// construction and response decoding errors are reported with retry metadata.
func TestOpenAIProviderCallOnce_RequestConstructionErrors(t *testing.T) {
	cases := []struct {
		name      string
		provider  openAIProvider
		request   modelProviderRequest
		body      string
		transport error
		wantRetry bool
	}{
		{name: "marshal", provider: openAIProvider{endpoint: "https://example.test", name: providerOpenAI}, request: modelProviderRequest{Model: "gpt", Tools: []modelTool{{Name: "bad", InputSchema: func() {}}}}},
		{name: "new request", provider: openAIProvider{endpoint: "://bad", name: providerOpenAI}, request: modelProviderRequest{Model: "gpt"}},
		{name: "transport", provider: openAIProvider{endpoint: "https://example.test", name: providerOpenAI}, request: modelProviderRequest{Model: "gpt"}, transport: errors.New("network"), wantRetry: true},
		{name: "decode", provider: openAIProvider{endpoint: "https://example.test", name: providerOpenAI}, request: modelProviderRequest{Model: "gpt"}, body: `{`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				if tc.transport != nil {
					return nil, tc.transport
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			_, retry, err := tc.provider.callOnce(context.Background(), client, "key", tc.request)
			if err == nil {
				t.Fatal("callOnce() error = nil, want error")
			}
			if retry != tc.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, tc.wantRetry)
			}
		})
	}
}

// TestOpenAIMessages_ConvertsAssistantAndToolMessages verifies model messages
// are converted into OpenAI-compatible assistant, user, and tool messages.
func TestOpenAIMessages_ConvertsAssistantAndToolMessages(t *testing.T) {
	messages := openAIMessages(modelProviderRequest{
		System: "system prompt",
		Messages: []modelMessage{
			{Role: "assistant", Content: []modelContentBlock{{Type: "text", Text: "thinking"}, {Type: "tool_use", ID: "call_1", Name: "gitlab_project", Input: map[string]any{"project_id": "42"}}}},
			{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "continue"}, {Type: "tool_result", ToolUseID: "call_1", Content: `{"ok":true}`}}},
		},
	})
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if messages[0].Role != "system" || messages[0].Content != "system prompt" {
		t.Fatalf("system message = %#v", messages[0])
	}
	if len(messages[1].ToolCalls) != 1 || messages[1].Content != "thinking" {
		t.Fatalf("assistant message = %#v", messages[1])
	}
	if messages[2].Role != "user" || messages[3].Role != "tool" || messages[3].ToolCallID != "call_1" {
		t.Fatalf("user/tool messages = %#v", messages[2:])
	}
}

// TestOpenAIAssistantMessage_UnmarshalableInputFallsBackToEmptyObject verifies
// unmarshalable tool inputs still produce valid OpenAI function-call JSON.
func TestOpenAIAssistantMessage_UnmarshalableInputFallsBackToEmptyObject(t *testing.T) {
	message := openAIAssistantMessage(modelMessage{Role: "assistant", Content: []modelContentBlock{{Type: "tool_use", ID: "call_1", Name: "tool", Input: map[string]any{"bad": func() {}}}}})
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("message = %#v, want empty object arguments", message)
	}
}

// TestGoogleHelpers_CoverContentAndSchemaBranches verifies Google conversion,
// function-mode, empty-response, and schema sanitization helper branches.
func TestGoogleHelpers_CoverContentAndSchemaBranches(t *testing.T) {
	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "auto")
	if got := googleFunctionCallingMode(); got != "AUTO" {
		t.Fatalf("googleFunctionCallingMode() = %q, want AUTO", got)
	}
	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "unknown")
	if got := googleFunctionCallingMode(); got != "VALIDATED" {
		t.Fatalf("googleFunctionCallingMode(default) = %q, want VALIDATED", got)
	}

	contents := googleContents([]modelMessage{
		{Role: "assistant", Content: []modelContentBlock{{Type: "text", Text: "hello"}, {Type: "tool_use", ID: "call_1", Name: "gitlab_project", Input: map[string]any{"project_id": "42"}, ThoughtSignature: "sig"}}},
		{Role: "user", Content: []modelContentBlock{{Type: "tool_result", ToolUseID: "call_1", Content: `{"ok":true}`}, {Type: "tool_result", ToolUseID: "missing", Content: "plain"}}},
	})
	if len(contents) != 2 || contents[0].Role != "model" || contents[1].Parts[0].FunctionResponse.Name != "gitlab_project" || contents[1].Parts[1].FunctionResponse.Name != "tool_result" {
		t.Fatalf("google contents = %#v", contents)
	}
	blocks := googleContentBlocks(googleContent{Parts: []googlePart{{Text: "text"}, {ThoughtSignature: "sig", FunctionCall: &googleFunctionCall{Name: "tool", Args: map[string]any{"a": 1}}}}})
	if len(blocks) != 2 || blocks[1].ID != "google-call-2" || blocks[1].ThoughtSignature != "sig" {
		t.Fatalf("google blocks = %#v", blocks)
	}
	if len(googleToolUseBlocks(googleContent{Parts: []googlePart{{Text: "text"}, {FunctionCall: &googleFunctionCall{Name: "tool", ID: "id"}}}})) != 1 {
		t.Fatal("googleToolUseBlocks() did not filter to one tool call")
	}

	decoded := googleResponse{PromptFeedback: &struct {
		BlockReason        string `json:"blockReason,omitempty"`
		BlockReasonMessage string `json:"blockReasonMessage,omitempty"`
	}{BlockReason: "SAFETY", BlockReasonMessage: "blocked"}}
	decoded.Candidates = append(decoded.Candidates, struct {
		Content       googleContent `json:"content"`
		FinishReason  string        `json:"finishReason,omitempty"`
		FinishMessage string        `json:"finishMessage,omitempty"`
	}{FinishReason: "STOP", FinishMessage: "done"})
	if err := googleEmptyResponseError(decoded, "nothing"); !strings.Contains(err.Error(), "blockReason=SAFETY") || !strings.Contains(err.Error(), "finishMessage=done") {
		t.Fatalf("googleEmptyResponseError() = %v", err)
	}

	schema := sanitizeGoogleSchema(map[string]any{
		"$schema":              "https://json-schema.org",
		"title":                "ignored",
		"additionalProperties": false,
		"type":                 []any{"null", "object"},
		"properties": map[string]any{
			"name": map[string]any{"type": []any{"null", "string"}},
		},
	}).(map[string]any)
	if _, exists := schema["$schema"]; exists || schema["type"] != "object" {
		t.Fatalf("sanitized schema = %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if properties["name"].(map[string]any)["type"] != "string" {
		t.Fatalf("sanitized properties = %#v", properties)
	}
	if got := sanitizeGoogleSchemaValue([]any{map[string]any{"type": []any{"null"}}}).([]any)[0].(map[string]any)["type"]; got == "object" {
		t.Fatalf("sanitizeGoogleSchemaValue(null-only) = %#v, want unchanged union", got)
	}
	if got := sanitizeGoogleSchemaProperties("not-map"); got != "not-map" {
		t.Fatalf("sanitizeGoogleSchemaProperties(non-map) = %#v", got)
	}
	declarations := googleFunctionDeclarations([]modelTool{{Name: "tool", Description: "desc", InputSchema: map[string]any{"type": []any{"null", "object"}}}})
	if len(declarations) != 1 || declarations[0].Parameters.(map[string]any)["type"] != "object" {
		t.Fatalf("googleFunctionDeclarations() = %#v", declarations)
	}
}

// TestGoogleFunctionCallJSON verifies raw args are preserved on unmarshal and
// IDs are included when marshaling function calls back to Google.
func TestGoogleFunctionCallJSON(t *testing.T) {
	var call googleFunctionCall
	if err := json.Unmarshal([]byte(`{"name":"tool","id":"call_1","args":{"project_id":"42"}}`), &call); err != nil {
		t.Fatalf("UnmarshalJSON() error = %v", err)
	}
	if call.Name != "tool" || call.ID != "call_1" || string(call.RawArgs) != `{"project_id":"42"}` {
		t.Fatalf("call = %+v raw=%s", call, call.RawArgs)
	}
	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if !strings.Contains(string(data), `"id":"call_1"`) || !strings.Contains(string(data), `"project_id":"42"`) {
		t.Fatalf("marshaled call = %s", data)
	}

	var empty googleFunctionCall
	if emptyErr := json.Unmarshal([]byte(`{"name":"empty","args":null}`), &empty); emptyErr != nil {
		t.Fatalf("UnmarshalJSON(null args) error = %v", emptyErr)
	}
	if len(empty.Args) != 0 || string(empty.RawArgs) != "null" {
		t.Fatalf("empty args = %#v raw=%s", empty.Args, empty.RawArgs)
	}
}

// TestDoModelRequest_ErrorBranches verifies retry decisions for transient and
// permanent HTTP failures plus defensive response reading limits.
func TestDoModelRequest_ErrorBranches(t *testing.T) {
	cases := []struct {
		name      string
		response  *http.Response
		err       error
		wantRetry bool
	}{
		{name: "network", err: errors.New("network down"), wantRetry: true},
		{name: "read", response: &http.Response{StatusCode: http.StatusOK, Body: errorReadCloser{}}, wantRetry: true},
		{name: "too large", response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBytes+1)))}, wantRetry: false},
		{name: "rate limit", response: &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("retry"))}, wantRetry: true},
		{name: "server", response: &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(strings.NewReader("retry"))}, wantRetry: true},
		{name: "client", response: &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("bad"))}, wantRetry: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return tc.response, tc.err
			})}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.test", nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext() error = %v", err)
			}
			_, _, retry, gotErr := doModelRequest(client, req, "test")
			if gotErr == nil {
				t.Fatal("doModelRequest() error = nil, want error")
			}
			if retry != tc.wantRetry {
				t.Fatalf("retry = %v, want %v", retry, tc.wantRetry)
			}
		})
	}
}

// errorReadCloser holds error read closer data for the evaluator package.
type errorReadCloser struct{}

// Read streams data from errorReadCloser into p.
func (errorReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// Close handles close for errorReadCloser.
func (errorReadCloser) Close() error { return nil }

// TestParseOpenAIToolArguments_RecoversXMLWrappedJSON verifies that tool-call
// arguments wrapped in Hermes-style <tool_call> tags or prefixed by a <think>
// reasoning block (as qwen3.6-flash intermittently emits) are recovered into the
// embedded JSON object, including when the wrapper contains ':' or '"'.
func TestParseOpenAIToolArguments_RecoversXMLWrappedJSON(t *testing.T) {
	cases := map[string]string{
		"tool_call wrapper":         `<tool_call>{"action":"merge_request.list","params":{"state":"merged","per_page":5}}</tool_call>`,
		"named tool_call wrapper":   `<tool_call name="merge_request.list">{"action":"merge_request.list","params":{"state":"merged"}}</tool_call>`,
		"think prefix with colon":   `<think>The user wants state: merged</think>{"action":"merge_request.list","params":{"state":"merged"}}`,
		"brace inside string value": `<tool_call>{"action":"merge_request.list","params":{"q":"a{b}c"}}</tool_call>`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseOpenAIToolArguments(raw)
			if err != nil {
				t.Fatalf("parseOpenAIToolArguments(%q) error = %v", raw, err)
			}
			if got["action"] != "merge_request.list" {
				t.Fatalf("action = %v, want merge_request.list (input=%v)", got["action"], got)
			}
			if _, ok := got["params"].(map[string]any); !ok {
				t.Fatalf("params not recovered as object: %#v", got["params"])
			}
		})
	}
}

// TestResolveModelSpecs_UsesEvalModels verifies ResolveModelSpecs uses eval models.
func TestResolveModelSpecs_UsesEvalModels(t *testing.T) {
	t.Setenv("EVAL_MODELS", "anthropic:claude-sonnet-4-6, google:gemini-3.0-flash, openai:gpt-5.4-mini, qwen:qwen3.6-flash")
	specs, err := resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	got := modelReportLabel(specs)
	want := "anthropic:claude-sonnet-4-6,google:gemini-3.0-flash,openai:gpt-5.4-mini,qwen:qwen3.6-flash"
	if got != want {
		t.Fatalf("modelReportLabel() = %q, want %q", got, want)
	}
}

// TestResolveModelSpecs_IgnoresEmptyEntries verifies ResolveModelSpecs ignores empty entries.
func TestResolveModelSpecs_IgnoresEmptyEntries(t *testing.T) {
	t.Setenv("EVAL_MODELS", "anthropic:claude-sonnet-4-6,")
	specs, err := resolveModelSpecs(options{})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	if len(specs) != 1 || specs[0].String() != "anthropic:claude-sonnet-4-6" {
		t.Fatalf("specs = %+v, want single model", specs)
	}
}

// TestResolveModelSpecs_ModelFlagOverridesEvalModels verifies ResolveModelSpecs when model flag overrides eval models.
func TestResolveModelSpecs_ModelFlagOverridesEvalModels(t *testing.T) {
	t.Setenv("EVAL_MODELS", "google:gemini-3.0-flash")
	specs, err := resolveModelSpecs(options{Model: "claude-haiku-4-6"})
	if err != nil {
		t.Fatalf("resolveModelSpecs() error = %v", err)
	}
	if len(specs) != 1 || specs[0].Provider != providerAnthropic || specs[0].Model != "claude-haiku-4-6" {
		t.Fatalf("specs = %+v, want single legacy Anthropic model", specs)
	}
}

// TestParseModelSpec_RejectsUnsupportedProvider verifies ParseModelSpec rejects unsupported provider.
func TestParseModelSpec_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseModelSpec("local:llama")
	if err == nil || !strings.Contains(err.Error(), "unsupported model provider") {
		t.Fatalf("error = %v, want unsupported provider", err)
	}
}

// TestParseModelSpec_StripsGoogleModelsPrefix verifies ParseModelSpec when strips google models prefix.
func TestParseModelSpec_StripsGoogleModelsPrefix(t *testing.T) {
	spec, err := parseModelSpec("google:models/gemini-3-flash-preview")
	if err != nil {
		t.Fatalf("parseModelSpec() error = %v", err)
	}
	if spec.Model != "gemini-3-flash-preview" {
		t.Fatalf("model = %q, want trimmed Gemini model", spec.Model)
	}
}

// TestAPIKeyForModelProvider_RequiresQwenAPIKey verifies APIKeyForModelProvider requires qwen API key.
func TestAPIKeyForModelProvider_RequiresQwenAPIKey(t *testing.T) {
	t.Setenv("QWEN_API_KEY", "")
	_, err := apiKeyForModelProvider(providerQwen)
	if err == nil {
		t.Fatal("apiKeyForModelProvider() error = nil, want missing QWEN_API_KEY")
	}
	if !strings.Contains(err.Error(), "QWEN_API_KEY") {
		t.Fatalf("error = %v, want QWEN_API_KEY", err)
	}
}

// TestQwenEndpoint_UsesConfiguredBaseURL verifies QwenEndpoint uses configured base URL.
func TestQwenEndpoint_UsesConfiguredBaseURL(t *testing.T) {
	t.Setenv("QWEN_CHAT_COMPLETIONS_URL", "")
	t.Setenv("QWEN_BASE_URL", "https://example.test/v1/")
	if got := qwenEndpoint(); got != "https://example.test/v1/chat/completions" {
		t.Fatalf("qwenEndpoint() = %q", got)
	}
}

// TestOpenAIProvider_CallOnceConvertsToolCall verifies OpenAIProvider when call once converts tool call.
func TestOpenAIProvider_CallOnceConvertsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q, want bearer", got)
		}
		var request openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "gpt-test" || len(request.Tools) != 1 || request.ToolChoice != "required" {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": `{"action":"user.current","params":{}}`,
				},
			}}}}},
			"usage": map[string]any{"prompt_tokens": 11, "completion_tokens": 7},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerOpenAI, maxTokenField: "max_completion_tokens"}
	response, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "gpt-test",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})
	if err != nil || retry {
		t.Fatalf("callOnce() retry=%v error=%v", retry, err)
	}
	if response.Usage.InputTokens != 11 || response.Usage.OutputTokens != 7 {
		t.Fatalf("usage = %+v", response.Usage)
	}
	if len(response.Content) != 1 || response.Content[0].Name != "gitlab" || response.Content[0].Input["action"] != "user.current" {
		t.Fatalf("content = %+v", response.Content)
	}
}

// TestOpenAIProvider_QwenDisablesThinking verifies OpenAIProvider when qwen disables thinking.
func TestOpenAIProvider_QwenDisablesThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openAIRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.EnableThinking == nil || *request.EnableThinking {
			t.Fatalf("enable_thinking = %v, want false", request.EnableThinking)
		}
		if request.ToolChoice != "required" || request.MaxTokens == 0 {
			t.Fatalf("request = %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": `{"action":"user.current","params":{}}`,
				},
			}}}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}
	_, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "qwen3.6-flash",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})
	if err != nil || retry {
		t.Fatalf("callOnce() retry=%v error=%v", retry, err)
	}
}

// TestOpenAIProvider_EmptyToolArgumentsAreRetryable verifies OpenAIProvider when empty tool arguments are retryable.
func TestOpenAIProvider_EmptyToolArgumentsAreRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{map[string]any{
				"id":   "call-1",
				"type": "function",
				"function": map[string]any{
					"name":      "gitlab",
					"arguments": "",
				},
			}}}}},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	provider := openAIProvider{endpoint: server.URL, name: providerQwen, maxTokenField: "max_tokens", disableThinking: true}
	_, retry, err := provider.callOnce(t.Context(), server.Client(), "test-key", modelProviderRequest{
		Model:     "qwen3.6-flash",
		MaxTokens: 128,
		System:    "Use tools.",
		Tools:     []modelTool{{Name: "gitlab", Description: "GitLab", InputSchema: map[string]any{"type": "object"}}},
		Messages:  []modelMessage{{Role: "user", Content: []modelContentBlock{{Type: "text", Text: "Who am I?"}}}},
	})

	if err == nil || !retry {
		t.Fatalf("callOnce() retry=%v error=%v, want retryable empty arguments error", retry, err)
	}
}

// TestOpenAIToolUseBlocks_RepairsLeadingCommaArguments verifies OpenAIToolUseBlocks when repairs leading comma arguments.
func TestOpenAIToolUseBlocks_RepairsLeadingCommaArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab",
			Arguments: `, "action":"project.milestone_create","params":{"project_id":"my-org/tools/gitlab-mcp-server","title":"Evaluation Sprint"}`,
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "project.milestone_create" {
		t.Fatalf("action = %v, want project.milestone_create", blocks[0].Input["action"])
	}
}

// TestOpenAIToolUseBlocks_RepairsInterleavedLeadingCommaArguments verifies OpenAIToolUseBlocks when repairs interleaved leading comma arguments.
func TestOpenAIToolUseBlocks_RepairsInterleavedLeadingCommaArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab",
			Arguments: " , \n, \"action\":\"merge_request.create\",\"params\":{\"project_id\":\"my-org/tools/gitlab-mcp-server\",\"source_branch\":\"feature/eval\",\"target_branch\":\"main\",\"title\":\"Evaluation MR\"}, ",
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "merge_request.create" {
		t.Fatalf("action = %v, want merge_request.create", blocks[0].Input["action"])
	}
}

// TestOpenAIToolUseBlocks_ExtractsWrappedJSONArguments verifies OpenAIToolUseBlocks when extracts wrapped JSON arguments.
func TestOpenAIToolUseBlocks_ExtractsWrappedJSONArguments(t *testing.T) {
	blocks, err := openAIToolUseBlocks(openAIMessage{ToolCalls: []openAIToolCall{{
		ID:   "call-1",
		Type: "function",
		Function: openAIFunctionCall{
			Name:      "gitlab_pipeline",
			Arguments: `<tool_call>{"action":"get","params":{"project_id":"my-org/tools/gitlab-mcp-server","pipeline_id":12345}}</tool_call>`,
		},
	}}})
	if err != nil {
		t.Fatalf("openAIToolUseBlocks() error = %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	if blocks[0].Input["action"] != "get" {
		t.Fatalf("action = %v, want get", blocks[0].Input["action"])
	}
}

// TestGoogleContentConversion_RoundTripsFunctionResponseNames verifies GoogleContentConversion when round trips function response names.
func TestGoogleContentConversion_RoundTripsFunctionResponseNames(t *testing.T) {
	messages := []modelMessage{
		{Role: "assistant", Content: []modelContentBlock{{Type: "tool_use", ID: "call-1", Name: "gitlab", Input: map[string]any{"action": "user.current"}, ThoughtSignature: "thought-token"}}},
		{Role: "user", Content: []modelContentBlock{{Type: "tool_result", ToolUseID: "call-1", Content: "ok"}}},
	}
	contents := googleContents(messages)
	if len(contents) != 2 || len(contents[1].Parts) != 1 || contents[1].Parts[0].FunctionResponse == nil {
		t.Fatalf("contents = %+v", contents)
	}
	if contents[1].Parts[0].FunctionResponse.Name != "gitlab" {
		t.Fatalf("function response name = %q, want gitlab", contents[1].Parts[0].FunctionResponse.Name)
	}
	if contents[1].Parts[0].FunctionResponse.ID != "call-1" {
		t.Fatalf("function response id = %q, want call-1", contents[1].Parts[0].FunctionResponse.ID)
	}
	if contents[0].Parts[0].ThoughtSignature != "thought-token" {
		t.Fatalf("thought signature = %q, want preserved", contents[0].Parts[0].ThoughtSignature)
	}

	blocks := googleToolUseBlocks(googleContent{Parts: []googlePart{{ThoughtSignature: "thought-token", FunctionCall: &googleFunctionCall{Name: "gitlab", Args: map[string]any{"action": "user.current"}, ID: "call-1"}}}})
	if len(blocks) != 1 || blocks[0].Name != "gitlab" || blocks[0].Input["action"] != "user.current" {
		t.Fatalf("blocks = %+v", blocks)
	}
	if blocks[0].ID != "call-1" {
		t.Fatalf("id = %q, want call-1", blocks[0].ID)
	}
	if blocks[0].ThoughtSignature != "thought-token" {
		t.Fatalf("thought signature = %q, want preserved", blocks[0].ThoughtSignature)
	}

	contentBlocks := googleContentBlocks(googleContent{Parts: []googlePart{{Text: "plain response"}, {FunctionCall: &googleFunctionCall{Name: "gitlab", Args: map[string]any{"action": "user.current"}, ID: "call-2"}}}})
	if len(contentBlocks) != 2 || contentBlocks[0].Type != "text" || contentBlocks[0].Text != "plain response" || contentBlocks[1].Type != "tool_use" {
		t.Fatalf("content blocks = %+v, want text block followed by tool_use", contentBlocks)
	}
}

// TestGoogleFunctionCallingMode_DefaultsToValidated verifies GoogleFunctionCallingMode when defaults to validated.
func TestGoogleFunctionCallingMode_DefaultsToValidated(t *testing.T) {
	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "")
	if got := googleFunctionCallingMode(); got != "VALIDATED" {
		t.Fatalf("googleFunctionCallingMode() = %q, want VALIDATED", got)
	}

	t.Setenv("EVAL_GOOGLE_FUNCTION_MODE", "auto")
	if got := googleFunctionCallingMode(); got != "AUTO" {
		t.Fatalf("googleFunctionCallingMode() override = %q, want AUTO", got)
	}
}

// TestSanitizeGoogleSchema_FlattensTypeUnion verifies SanitizeGoogleSchema when flattens type union.
func TestSanitizeGoogleSchema_FlattensTypeUnion(t *testing.T) {
	schema := map[string]any{
		"type": []any{"string", "integer"},
		"properties": map[string]any{
			"project_id": map[string]any{"type": []any{"string", "integer"}},
		},
	}

	got := sanitizeGoogleSchema(schema).(map[string]any)
	if got["type"] != "string" {
		t.Fatalf("type = %#v, want string", got["type"])
	}
	properties := got["properties"].(map[string]any)
	projectID := properties["project_id"].(map[string]any)
	if projectID["type"] != "string" {
		t.Fatalf("project_id.type = %#v, want string", projectID["type"])
	}
}

// TestSanitizeGoogleSchema_PreservesTitleProperty verifies SanitizeGoogleSchema preserves title property.
func TestSanitizeGoogleSchema_PreservesTitleProperty(t *testing.T) {
	schema := map[string]any{
		"title": "Root schema title",
		"type":  "object",
		"properties": map[string]any{
			"title": map[string]any{
				"title":       "Property schema title",
				"type":        "string",
				"description": "Issue title.",
			},
		},
	}

	got := sanitizeGoogleSchema(schema).(map[string]any)
	if _, ok := got["title"]; ok {
		t.Fatalf("root schema title should be removed: %#v", got)
	}
	properties := got["properties"].(map[string]any)
	title, ok := properties["title"].(map[string]any)
	if !ok {
		t.Fatalf("properties.title missing after sanitize: %#v", properties)
	}
	if _, hasTitleKeyword := title["title"]; hasTitleKeyword {
		t.Fatalf("property schema title keyword should be removed: %#v", title)
	}
	if title["type"] != "string" {
		t.Fatalf("properties.title.type = %#v, want string", title["type"])
	}
}

// TestGoogleEmptyResponseError_IncludesFinishAndBlockReasons verifies GoogleEmptyResponseError includes finish and block reasons.
func TestGoogleEmptyResponseError_IncludesFinishAndBlockReasons(t *testing.T) {
	decoded := googleResponse{}
	decoded.Candidates = append(decoded.Candidates, struct {
		Content       googleContent `json:"content"`
		FinishReason  string        `json:"finishReason,omitempty"`
		FinishMessage string        `json:"finishMessage,omitempty"`
	}{FinishReason: "MALFORMED_FUNCTION_CALL", FinishMessage: "malformed tool call"})
	decoded.PromptFeedback = &struct {
		BlockReason        string `json:"blockReason,omitempty"`
		BlockReasonMessage string `json:"blockReasonMessage,omitempty"`
	}{BlockReason: "SAFETY", BlockReasonMessage: "blocked"}

	err := googleEmptyResponseError(decoded, "no tool calls or output tokens")
	message := err.Error()
	for _, want := range []string{"no tool calls or output tokens", "finishReason=MALFORMED_FUNCTION_CALL", "finishMessage=malformed tool call", "blockReason=SAFETY", "blockReasonMessage=blocked"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want %q", message, want)
		}
	}
}

// TestGoogleResponseDecode_PreservesNestedParams verifies GoogleResponseDecode preserves nested params.
func TestGoogleResponseDecode_PreservesNestedParams(t *testing.T) {
	raw := []byte(`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"gitlab_project","args":{"action":"get","params":{"project_id":"my-org/tools/gitlab-mcp-server"}},"id":"call-1"}}]}}]}`)
	var decoded googleResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode google response: %v", err)
	}

	blocks := googleToolUseBlocks(decoded.Candidates[0].Content)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %+v, want one tool call", blocks)
	}
	params, ok := blocks[0].Input["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v, want object", blocks[0].Input["params"])
	}
	if params["project_id"] != "my-org/tools/gitlab-mcp-server" {
		t.Fatalf("project_id = %#v", params["project_id"])
	}
	if string(blocks[0].ProviderRawInput) != `{"action":"get","params":{"project_id":"my-org/tools/gitlab-mcp-server"}}` {
		t.Fatalf("raw input = %s", blocks[0].ProviderRawInput)
	}
}
