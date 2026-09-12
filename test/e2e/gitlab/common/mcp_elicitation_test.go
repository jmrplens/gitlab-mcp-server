//go:build e2e

// mcp_elicitation_test.go covers the MCP elicitation capability end to end:
// the server drives a guided creation flow by asking the client for each
// field, and a scripted client answers.
//
// The old suite proved this against a server it assembled in its own process;
// here it runs against the real binary, whose interactive flow is one of the
// two defects the transport rebuild was written to catch (a nil dereference on
// an eliciting call). The flow is a standalone tool the projection does not
// spell, so it is named directly through Raw, and the session answers the
// server's questions with a scripted responder rather than auto-accepting,
// because the issue flow asks for a title and would fail on an empty answer.

package common

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// interactiveIssueCreateTool is the standalone tool the guided issue flow is
// registered as on the meta and individual surfaces.
const interactiveIssueCreateTool = "gitlab_interactive_issue_create"

// elicitedIssueTitle is the title the scripted responder answers the flow's
// title prompt with, and what the created issue must carry to prove the
// answer reached the handler.
const elicitedIssueTitle = "E2E elicitation test"

// TestElicitation_InteractiveIssueCreate drives gitlab_interactive_issue_create
// against the binary with a scripted client, and asserts the issue the server
// created carries the elicited title.
//
// Replaces: TestElicitation
func TestElicitation_InteractiveIssueCreate(t *testing.T) {
	e := harness.New(t)
	project := fixture.NewProject(e, fixture.WithNamePrefix("elicit"))

	s := e.Session(harness.ServerConfig{
		Surface:      harness.SurfaceIndividual,
		Elicitation:  harness.ElicitationScripted,
		Responder:    scriptedElicitResponder,
		Capabilities: harness.CapabilitiesMinimal,
	})

	result, err := s.Raw(&mcp.CallToolParams{
		Name:      interactiveIssueCreateTool,
		Arguments: map[string]any{"project_id": project.IDParam()},
	})
	if err != nil {
		e.T.Fatalf("%s: %v", interactiveIssueCreateTool, err)
	}
	if result == nil || result.IsError {
		e.T.Fatalf("%s answered an error: %s", interactiveIssueCreateTool, rawText(result))
	}

	var out issues.Output
	if decodeErr := decodeInteractiveResult(result, &out); decodeErr != nil {
		e.T.Fatalf("decoding the interactive issue result: %v", decodeErr)
	}
	if out.IID == 0 {
		e.T.Errorf("the interactive flow answered issue IID %d, want a real issue", out.IID)
	}
	if out.Title != elicitedIssueTitle {
		e.T.Errorf("the created issue title is %q, want the elicited %q", out.Title, elicitedIssueTitle)
	}
}

// scriptedElicitResponder answers each elicitation request with a plausible
// value read from the schema the server asked against: true for a
// confirmation, the first option for a selection, and a title-appropriate
// string for a text field. It is the same policy the old suite's mock handler
// used, so the flow that reached a title of [elicitedIssueTitle] there reaches
// it here.
func scriptedElicitResponder(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
	return &mcp.ElicitResult{Action: "accept", Content: scriptedElicitContent(req)}, nil
}

// scriptedElicitContent fills every property the requested schema names.
func scriptedElicitContent(req *mcp.ElicitRequest) map[string]any {
	content := map[string]any{}
	if req == nil || req.Params == nil {
		return content
	}
	schema, ok := req.Params.RequestedSchema.(map[string]any)
	if !ok {
		return content
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return content
	}
	for key, raw := range properties {
		property, isObject := raw.(map[string]any)
		if !isObject {
			continue
		}
		content[key] = scriptedElicitValue(key, property)
	}
	return content
}

// scriptedElicitValue picks one value for one elicited property.
func scriptedElicitValue(key string, property map[string]any) any {
	switch key {
	case "confirmed":
		return true
	case "selection":
		if options, ok := property["enum"].([]any); ok && len(options) > 0 {
			return options[0]
		}
		return "default"
	default:
		return scriptedElicitText(key)
	}
}

// scriptedElicitText answers a free-text field by its name, so the title the
// issue is created with is the one the test asserts on.
func scriptedElicitText(field string) string {
	switch field {
	case "title":
		return elicitedIssueTitle
	case "description":
		return "Created by the e2e elicitation responder"
	case "labels":
		return "e2e-test"
	default:
		return "e2e-" + field
	}
}

// decodeInteractiveResult reads a tool result into a value, preferring the
// structured content and falling back to the first text block.
func decodeInteractiveResult(result *mcp.CallToolResult, into any) error {
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return err
		}
		return json.Unmarshal(encoded, into)
	}
	return json.Unmarshal([]byte(rawText(result)), into)
}
