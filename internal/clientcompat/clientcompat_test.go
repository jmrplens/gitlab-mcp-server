// Package clientcompat_test verifies the per-client response compatibility
// middleware end to end: a real mcp.Server with the middleware installed is
// connected over in-memory transports by clients that identify as Codex and
// as a generic client, and the tests assert that only the Codex session gets
// the float priority rounded to a spec-legal integer while every other
// field — audience, structuredContent, outputSchema, icons — survives for
// everyone.
package clientcompat_test

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/clientcompat"
)

// contentAnnotations mirrors the annotations the production formatters attach
// to markdown content blocks: an assistant audience with a float priority —
// the exact shape that breaks Codex's bundled rmcp parser.
var contentAnnotations = &mcp.Annotations{
	Audience: []mcp.Role{"assistant"},
	Priority: 0.6,
}

// newTestServer builds an mcp.Server with the clientcompat middleware and one
// tool, resource, resource template, and prompt, each carrying a
// float-priority annotation plus the metadata that must survive sanitizing.
func newTestServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	server.AddReceivingMiddleware(clientcompat.Middleware())

	schema := map[string]any{"type": "object", "properties": map[string]any{}}
	server.AddTool(&mcp.Tool{
		Name:         "echo",
		Description:  "echo tool",
		InputSchema:  schema,
		OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}},
		Icons:        []mcp.Icon{{Source: "data:image/svg+xml;base64,AAAA"}},
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: true, Title: "Echo"},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "## result", Annotations: contentAnnotations}},
			StructuredContent: map[string]any{"ok": true},
		}, nil
	})

	server.AddResource(&mcp.Resource{
		URI:         "test://doc",
		Name:        "doc",
		MIMEType:    "text/markdown",
		Annotations: contentAnnotations,
		Icons:       []mcp.Icon{{Source: "data:image/svg+xml;base64,AAAA"}},
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "body"}},
		}, nil
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "test://doc/{id}",
		Name:        "doc-by-id",
		Annotations: contentAnnotations,
		Icons:       []mcp.Icon{{Source: "data:image/svg+xml;base64,AAAA"}},
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "body"}},
		}, nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:  "greet",
		Icons: []mcp.Icon{{Source: "data:image/svg+xml;base64,AAAA"}},
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "hello", Annotations: contentAnnotations}},
			},
		}, nil
	})

	return server
}

// connect wires a client with the given implementation identity to a fresh
// test server over in-memory transports and returns the client session.
func connect(t *testing.T, impl *mcp.Implementation) *mcp.ClientSession {
	t.Helper()
	return connectTo(t, newTestServer(), impl)
}

// connectTo attaches a client session to an existing server so tests can run
// several differently-identified sessions against one shared tool registry.
func connectTo(t *testing.T, server *mcp.Server, impl *mcp.Implementation) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(impl, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

var (
	codexImpl   = &mcp.Implementation{Name: "codex-mcp-client", Title: "Codex", Version: "0.148.0"}
	genericImpl = &mcp.Implementation{Name: "claude-code", Title: "Claude Code", Version: "2.0.0"}
)

// callEcho invokes the echo tool and returns the result.
func callEcho(t *testing.T, session *mcp.ClientSession) *mcp.CallToolResult {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return res
}

// textContent extracts the single expected text block from a tool result.
func textContent(t *testing.T, res *mcp.CallToolResult) *mcp.TextContent {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("content length = %d, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc
}

// TestCallTool_CodexClient_RoundsPriorityKeepsEverythingElse verifies the
// Codex workaround is minimal: the float priority (which breaks Codex's
// bundled rmcp parser) is rounded to an integer, while audience, the
// markdown text, and structuredContent all survive.
func TestCallTool_CodexClient_RoundsPriorityKeepsEverythingElse(t *testing.T) {
	res := callEcho(t, connect(t, codexImpl))
	tc := textContent(t, res)
	if tc.Annotations == nil {
		t.Fatal("annotations dropped entirely; want audience preserved")
	}
	if tc.Annotations.Priority != 1 {
		t.Errorf("priority = %v, want rounded to 1", tc.Annotations.Priority)
	}
	if len(tc.Annotations.Audience) != 1 || tc.Annotations.Audience[0] != "assistant" {
		t.Errorf("audience = %v, want [assistant]", tc.Annotations.Audience)
	}
	if tc.Text != "## result" {
		t.Errorf("text = %q, want unchanged markdown", tc.Text)
	}
	if res.StructuredContent == nil {
		t.Error("structuredContent dropped; want preserved")
	}
}

// TestCallTool_GenericClient_KeepsFloatPriority verifies non-Codex clients
// keep the float priority untouched.
func TestCallTool_GenericClient_KeepsFloatPriority(t *testing.T) {
	res := callEcho(t, connect(t, genericImpl))
	tc := textContent(t, res)
	if tc.Annotations == nil {
		t.Fatal("content annotations stripped for generic client")
	}
	if got := tc.Annotations.Priority; got != 0.6 {
		t.Errorf("priority = %v, want 0.6", got)
	}
	if res.StructuredContent == nil {
		t.Error("structuredContent stripped for generic client")
	}
}

// TestListTools_CodexClient_Untouched verifies tools/list is not modified for
// Codex: outputSchema, icons, and the tool annotations its approval policy
// reads (readOnlyHint) all pass through — ToolAnnotations carries no floats.
func TestListTools_CodexClient_Untouched(t *testing.T) {
	session := connect(t, codexImpl)
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(res.Tools))
	}
	tool := res.Tools[0]
	if tool.OutputSchema == nil {
		t.Error("outputSchema dropped; want preserved")
	}
	if len(tool.Icons) == 0 {
		t.Error("icons dropped; want preserved")
	}
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Errorf("tool annotations = %+v, want readOnlyHint preserved", tool.Annotations)
	}
}

// TestListResourcesAndTemplates_CodexClient_RoundsPriorityOnly verifies
// resource listings get only the float priority rounded for Codex sessions;
// audience and icons stay.
func TestListResourcesAndTemplates_CodexClient_RoundsPriorityOnly(t *testing.T) {
	session := connect(t, codexImpl)
	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if len(resources.Resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(resources.Resources))
	}
	r := resources.Resources[0]
	if r.Annotations == nil || r.Annotations.Priority != 1 || len(r.Annotations.Audience) == 0 {
		t.Errorf("resource annotations = %+v, want priority rounded to 1 and audience kept", r.Annotations)
	}
	if len(r.Icons) == 0 {
		t.Error("resource icons dropped; want preserved")
	}
	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 {
		t.Fatalf("templates = %d, want 1", len(templates.ResourceTemplates))
	}
	rt := templates.ResourceTemplates[0]
	if rt.Annotations == nil || rt.Annotations.Priority != 1 {
		t.Errorf("template annotations = %+v, want priority rounded to 1", rt.Annotations)
	}
}

// TestListResources_GenericClient_KeepsFloatPriority verifies resource
// metadata stays fully intact for non-Codex sessions.
func TestListResources_GenericClient_KeepsFloatPriority(t *testing.T) {
	session := connect(t, genericImpl)
	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	if r := resources.Resources[0]; r.Annotations == nil || r.Annotations.Priority != 0.6 {
		t.Errorf("resource annotations = %+v, want priority 0.6", r.Annotations)
	}
}

// TestGetPrompt_CodexClient_RoundsMessagePriority verifies prompts/get gets
// only the float priority rounded on message content annotations for Codex
// sessions.
func TestGetPrompt_CodexClient_RoundsMessagePriority(t *testing.T) {
	session := connect(t, codexImpl)
	got, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{Name: "greet"})
	if err != nil {
		t.Fatalf("GetPrompt: %v", err)
	}
	tc, ok := got.Messages[0].Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("message content is %T, want *mcp.TextContent", got.Messages[0].Content)
	}
	if tc.Annotations == nil || tc.Annotations.Priority != 1 {
		t.Errorf("message annotations = %+v, want priority rounded to 1", tc.Annotations)
	}
}

// TestSharedRegistry_CodexSessionDoesNotLeakIntoOthers verifies the clone
// discipline: a Codex session and a generic session share one server, the
// Codex session receives sanitized results first, and the generic session
// must still see the original float priorities afterwards.
func TestSharedRegistry_CodexSessionDoesNotLeakIntoOthers(t *testing.T) {
	server := newTestServer()
	codexSession := connectTo(t, server, codexImpl)
	genericSession := connectTo(t, server, genericImpl)

	codexResources, err := codexSession.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("codex ListResources: %v", err)
	}
	if codexResources.Resources[0].Annotations.Priority != 1 {
		t.Error("codex session kept float priority")
	}

	genericResources, err := genericSession.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("generic ListResources: %v", err)
	}
	if got := genericResources.Resources[0].Annotations.Priority; got != 0.6 {
		t.Errorf("registry mutated by codex session: priority = %v, want 0.6", got)
	}

	codexCall := callEcho(t, codexSession)
	if tc := textContent(t, codexCall); tc.Annotations.Priority != 1 {
		t.Error("codex call kept float priority")
	}
	genericCall := callEcho(t, genericSession)
	if tc := textContent(t, genericCall); tc.Annotations.Priority != 0.6 {
		t.Error("generic call sanitized after codex call on same server")
	}
}

// TestDetection_TitleOnly verifies the profile also matches when only the
// title identifies Codex (defensive against clientInfo.name changes).
func TestDetection_TitleOnly(t *testing.T) {
	impl := &mcp.Implementation{Name: "some-wrapper", Title: "Codex IDE", Version: "1.0.0"}
	res := callEcho(t, connect(t, impl))
	if tc := textContent(t, res); tc.Annotations.Priority != 1 {
		t.Error("float priority kept: title-based Codex detection failed")
	}
}

// TestMiddleware_NoSession_PassesThrough verifies requests without a server
// session (defensive path) are left untouched.
func TestMiddleware_NoSession_PassesThrough(t *testing.T) {
	result := &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "x", Annotations: contentAnnotations}},
	}
	handler := clientcompat.Middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return result, nil
	})
	res, err := handler(context.Background(), "tools/call", &mcp.CallToolRequest{})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	got, ok := res.(*mcp.CallToolResult)
	if !ok {
		t.Fatalf("result is %T, want *mcp.CallToolResult", res)
	}
	tc, ok := got.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", got.Content[0])
	}
	if tc.Annotations.Priority != 0.6 {
		t.Error("sessionless request was sanitized; want passthrough")
	}
}

// TestEnabled_EnvKillSwitch verifies CLIENT_COMPAT=off disables the
// middleware installation gate while any other value keeps it enabled.
func TestEnabled_EnvKillSwitch(t *testing.T) {
	t.Setenv("CLIENT_COMPAT", "")
	if !clientcompat.Enabled() {
		t.Error("Enabled() = false with empty env, want true")
	}
	t.Setenv("CLIENT_COMPAT", "off")
	if clientcompat.Enabled() {
		t.Error("Enabled() = true with CLIENT_COMPAT=off, want false")
	}
	t.Setenv("CLIENT_COMPAT", "OFF")
	if clientcompat.Enabled() {
		t.Error("Enabled() = true with CLIENT_COMPAT=OFF, want false")
	}
	t.Setenv("CLIENT_COMPAT", "auto")
	if !clientcompat.Enabled() {
		t.Error("Enabled() = false with CLIENT_COMPAT=auto, want true")
	}
}
