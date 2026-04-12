// Package elicitation provides a Client for requesting structured user input
// via the MCP elicitation protocol.
//
// The Client is a value type — its zero value is safe to use and acts as a
// no-op when the connected MCP client does not support elicitation. This
// mirrors the pattern used by sampling.Client and progress.Tracker.
//
// SECURITY: All responses are validated against the expected JSON Schema.
// User input is never trusted and must be sanitized by the caller before
// use in API calls.
package elicitation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrElicitationNotSupported is returned when the MCP client does not
// advertise the elicitation capability.
var ErrElicitationNotSupported = errors.New("elicitation: client does not support elicitation capability")

// ErrDeclined is returned when the user explicitly declines an elicitation.
var ErrDeclined = errors.New("elicitation: user declined")

// ErrCancelled is returned when the user dismisses an elicitation without
// making an explicit choice.
var ErrCancelled = errors.New("elicitation: user canceled")

// ErrURLElicitationNotSupported is returned when the MCP client supports
// form elicitation but not URL mode elicitation.
var ErrURLElicitationNotSupported = errors.New("elicitation: client does not support URL elicitation")

// Client sends elicitation requests to the MCP client for user input. Its
// zero value is an inactive client where IsSupported returns false.
type Client struct {
	session *mcp.ServerSession
}

// FromRequest extracts the server session from a CallToolRequest and returns
// a Client. If the connected MCP client does not support elicitation, the
// returned Client is inactive (IsSupported returns false).
func FromRequest(req *mcp.CallToolRequest) Client {
	if req == nil || req.Session == nil {
		return Client{}
	}
	params := req.Session.InitializeParams()
	if params == nil || params.Capabilities.Elicitation == nil {
		return Client{}
	}
	return Client{session: req.Session}
}

// IsSupported returns true if the MCP client supports elicitation.
func (c Client) IsSupported() bool {
	return c.session != nil
}

// IsURLSupported returns true if the MCP client supports URL mode elicitation.
func (c Client) IsURLSupported() bool {
	if c.session == nil {
		return false
	}
	params := c.session.InitializeParams()
	if params == nil || params.Capabilities.Elicitation == nil {
		return false
	}
	return params.Capabilities.Elicitation.URL != nil
}

// Confirm asks the user a yes/no question and returns true if
// the user accepted with confirmed=true, false otherwise. Returns
// ErrDeclined or ErrCancelled if the user did not accept.
func (c Client) Confirm(ctx context.Context, message string) (bool, error) {
	if !c.IsSupported() {
		return false, ErrElicitationNotSupported
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"confirmed": map[string]any{
				"type":        "boolean",
				"title":       "Confirm",
				"description": message,
			},
		},
		"required": []string{"confirmed"},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return false, err
	}

	confirmed, ok := result["confirmed"].(bool)
	return ok && confirmed, nil
}

// PromptText asks the user for free-form text input and returns the value.
func (c Client) PromptText(ctx context.Context, message, fieldName string) (string, error) {
	if !c.IsSupported() {
		return "", ErrElicitationNotSupported
	}
	if fieldName == "" {
		fieldName = "value"
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			fieldName: map[string]any{
				"type":        "string",
				"title":       fieldName,
				"description": message,
			},
		},
		"required": []string{fieldName},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return "", err
	}

	text, ok := result[fieldName].(string)
	if !ok {
		return "", fmt.Errorf("elicitation: response field %q is not a string", fieldName)
	}
	return text, nil
}

// SelectOne asks the user to pick one option from a list.
func (c Client) SelectOne(ctx context.Context, message string, options []string) (string, error) {
	if !c.IsSupported() {
		return "", ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return "", errors.New("elicitation: options list must not be empty")
	}

	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selection": map[string]any{
				"type":        "string",
				"title":       "Selection",
				"description": message,
				"enum":        enumValues,
			},
		},
		"required": []string{"selection"},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return "", err
	}

	selection, ok := result["selection"].(string)
	if !ok {
		return "", errors.New("elicitation: response field 'selection' is not a string")
	}

	// Validate against allowed options (defense in depth)
	if slices.Contains(options, selection) {
		return selection, nil
	}
	return "", fmt.Errorf("elicitation: selected value %q is not in the allowed options", selection)
}

// SelectMulti asks the user to pick one or more options from a list.
// minItems and maxItems constrain the number of selections (0 means no limit).
func (c Client) SelectMulti(ctx context.Context, message string, options []string, minItems, maxItems int) ([]string, error) {
	if !c.IsSupported() {
		return nil, ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return nil, errors.New("elicitation: options list must not be empty")
	}

	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}

	items := map[string]any{
		"type": "string",
		"enum": enumValues,
	}
	arraySchema := map[string]any{
		"type":        "array",
		"title":       "Selections",
		"description": message,
		"items":       items,
	}
	if minItems > 0 {
		arraySchema["minItems"] = minItems
	}
	if maxItems > 0 {
		arraySchema["maxItems"] = maxItems
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selections": arraySchema,
		},
		"required": []string{"selections"},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return nil, err
	}

	raw, ok := result["selections"].([]any)
	if !ok {
		return nil, errors.New("elicitation: response field 'selections' is not an array")
	}

	selections := make([]string, 0, len(raw))
	for _, v := range raw {
		var s string
		s, ok = v.(string)
		if !ok {
			return nil, fmt.Errorf("elicitation: selection element is not a string: %v", v)
		}
		if !slices.Contains(options, s) {
			return nil, fmt.Errorf("elicitation: selected value %q is not in the allowed options", s)
		}
		selections = append(selections, s)
	}
	return selections, nil
}

// SelectOneInt asks the user to pick one integer from a list of allowed values.
func (c Client) SelectOneInt(ctx context.Context, message string, options []int) (int, error) {
	if !c.IsSupported() {
		return 0, ErrElicitationNotSupported
	}
	if len(options) == 0 {
		return 0, errors.New("elicitation: options list must not be empty")
	}

	enumValues := make([]any, len(options))
	for i, o := range options {
		enumValues[i] = o
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"selection": map[string]any{
				"type":        "integer",
				"title":       "Selection",
				"description": message,
				"enum":        enumValues,
			},
		},
		"required": []string{"selection"},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return 0, err
	}

	// JSON numbers are float64 by default
	f, ok := result["selection"].(float64)
	if !ok {
		return 0, errors.New("elicitation: response field 'selection' is not a number")
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errors.New("elicitation: response field 'selection' is not a finite number")
	}
	if f != math.Trunc(f) {
		return 0, fmt.Errorf("elicitation: response value %g is not an integer", f)
	}
	selected := int(f)

	if !slices.Contains(options, selected) {
		return 0, fmt.Errorf("elicitation: selected value %d is not in the allowed options", selected)
	}
	return selected, nil
}

// PromptNumber asks the user for a numeric input within a range.
// minVal and maxVal define inclusive bounds; use math.Inf(-1) and math.Inf(1) for no bounds.
func (c Client) PromptNumber(ctx context.Context, message, fieldName string, minVal, maxVal float64) (float64, error) {
	if !c.IsSupported() {
		return 0, ErrElicitationNotSupported
	}
	if fieldName == "" {
		fieldName = "value"
	}

	prop := map[string]any{
		"type":        "number",
		"title":       fieldName,
		"description": message,
	}
	if !isInf(minVal) {
		prop["minimum"] = minVal
	}
	if !isInf(maxVal) {
		prop["maximum"] = maxVal
	}

	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			fieldName: prop,
		},
		"required": []string{fieldName},
	}

	result, err := c.elicit(ctx, message, schema)
	if err != nil {
		return 0, err
	}

	f, ok := result[fieldName].(float64)
	if !ok {
		return 0, fmt.Errorf("elicitation: response field %q is not a number", fieldName)
	}
	if math.IsNaN(f) {
		return 0, fmt.Errorf("elicitation: response field %q is NaN", fieldName)
	}
	return f, nil
}

// isInf reports whether f is +Inf or -Inf.
func isInf(f float64) bool {
	return math.IsInf(f, 0)
}

// GatherData sends an arbitrary JSON Schema to the client and returns the
// user's response as a map. This is the low-level method underlying the
// convenience methods.
//
// SECURITY: The caller is responsible for validating and sanitizing the
// returned data before using it in API calls.
func (c Client) GatherData(ctx context.Context, message string, schema map[string]any) (map[string]any, error) {
	if !c.IsSupported() {
		return nil, ErrElicitationNotSupported
	}
	return c.elicit(ctx, message, schema)
}

// elicit is the internal method that sends an elicitation request and
// handles the action response.
func (c Client) elicit(ctx context.Context, message string, schema map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slog.Debug("sending elicitation request", "message_length", len(message))

	result, err := c.session.Elicit(ctx, &mcp.ElicitParams{
		Message:         message,
		RequestedSchema: schema,
	})
	if err != nil {
		return nil, fmt.Errorf("elicitation: request failed: %w", err)
	}

	switch result.Action {
	case "accept":
		return result.Content, nil
	case "decline":
		return nil, ErrDeclined
	case "cancel":
		return nil, ErrCancelled
	default:
		return nil, fmt.Errorf("elicitation: unknown action %q", result.Action)
	}
}

// ElicitURL sends a URL-mode elicitation request, directing the user to
// a GitLab page. The URL must belong to the configured GitLab instance
// (SSRF prevention). Returns the action ("accept", "decline", "cancel").
func (c Client) ElicitURL(ctx context.Context, gitlabBaseURL, targetURL, message string) error {
	if !c.IsSupported() {
		return ErrElicitationNotSupported
	}
	if !c.IsURLSupported() {
		return ErrURLElicitationNotSupported
	}
	if err := validateGitLabURL(gitlabBaseURL, targetURL); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	slog.Debug("sending URL elicitation", "url", targetURL)

	result, err := c.session.Elicit(ctx, &mcp.ElicitParams{
		Mode:    "url",
		Message: message,
		URL:     targetURL,
	})
	if err != nil {
		return fmt.Errorf("elicitation: URL request failed: %w", err)
	}

	switch result.Action {
	case "accept":
		return nil
	case "decline":
		return ErrDeclined
	case "cancel":
		return ErrCancelled
	default:
		return fmt.Errorf("elicitation: unknown action %q", result.Action)
	}
}

// validateGitLabURL ensures targetURL belongs to the configured GitLab
// instance to prevent SSRF attacks.
func validateGitLabURL(gitlabBaseURL, targetURL string) error {
	base, err := url.Parse(gitlabBaseURL)
	if err != nil {
		return fmt.Errorf("elicitation: invalid GitLab base URL: %w", err)
	}
	target, err := url.Parse(targetURL)
	if err != nil {
		return fmt.Errorf("elicitation: invalid target URL: %w", err)
	}
	if target.Scheme != "https" && target.Scheme != "http" {
		return fmt.Errorf("elicitation: URL scheme %q is not allowed (must be http or https)", target.Scheme)
	}
	// Compare hostname and port separately to prevent mismatches when one
	// side includes an explicit port and the other does not.
	if !strings.EqualFold(target.Hostname(), base.Hostname()) {
		return fmt.Errorf("elicitation: URL host %q does not match GitLab instance %q", target.Hostname(), base.Hostname())
	}
	if target.Port() != base.Port() {
		return fmt.Errorf("elicitation: URL port %q does not match GitLab instance port %q", target.Port(), base.Port())
	}
	return nil
}

// ConfirmAction asks the user to confirm an action via elicitation.
// Returns nil if confirmed or elicitation is not supported (backward compatible).
// Returns a non-nil *mcp.CallToolResult if the user declined or canceled.
func ConfirmAction(ctx context.Context, req *mcp.CallToolRequest, message string) *mcp.CallToolResult {
	ec := FromRequest(req)
	if !ec.IsSupported() {
		return nil
	}
	confirmed, err := ec.Confirm(ctx, message)
	if err != nil {
		if errors.Is(err, ErrDeclined) || errors.Is(err, ErrCancelled) {
			return CancelledResult("Operation canceled by user.")
		}
		return nil
	}
	if !confirmed {
		return CancelledResult("Operation canceled by user.")
	}
	return nil
}

// CancelledResult returns a non-error tool result indicating the user canceled.
func CancelledResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: message},
		},
	}
}
