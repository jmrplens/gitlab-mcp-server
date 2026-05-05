package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestParseOpenAIToolArguments_WrapsMissingOpeningBrace verifies that ParseOpenAIToolArguments handles the wraps missing opening brace scenario correctly.
func TestParseOpenAIToolArguments_WrapsMissingOpeningBrace(t *testing.T) {
	input, err := parseOpenAIToolArguments(`"project_id":"42"}`)
	if err != nil {
		t.Fatalf("parseOpenAIToolArguments() error = %v", err)
	}
	if got := input["project_id"]; got != "42" {
		t.Fatalf("project_id = %v, want 42", got)
	}
}

// TestGoogleProviderCallOnce_SendsAPIKeyHeader verifies that googleProvider.callOnce handles the sends api key header scenario correctly.
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

// TestGoogleFunctionResponsePayload_PreservesErrorFlag verifies that googleFunctionResponsePayload handles the preserves error flag scenario correctly.
func TestGoogleFunctionResponsePayload_PreservesErrorFlag(t *testing.T) {
	payload := googleFunctionResponsePayload(modelContentBlock{Content: `{"is_error":false,"value":7}`, IsError: true})
	if got := payload["is_error"]; got != true {
		t.Fatalf("is_error = %v, want true", got)
	}
	if got := payload["value"]; got != float64(7) {
		t.Fatalf("value = %v, want 7", got)
	}
}

// TestDoModelRequest_ContextCancellationIsNotRetryable verifies that doModelRequest handles the context cancellation is not retryable scenario correctly.
func TestDoModelRequest_ContextCancellationIsNotRetryable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.test", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	_, retry, err := doModelRequest(client, req, "test")
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
