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

// addMixedContentTool registers a tool whose result carries every content
// block type with a float-priority annotation, plus text blocks with the
// rounding edge cases (0.4 rounds down to an omitted 0; an integer priority
// is passed through without cloning).
func addMixedContentTool(server *mcp.Server) {
	low := &mcp.Annotations{Audience: []mcp.Role{"assistant"}, Priority: 0.4}
	integer := &mcp.Annotations{Audience: []mcp.Role{"assistant"}, Priority: 1}
	server.AddTool(&mcp.Tool{
		Name:        "mixed",
		Description: "mixed content tool",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "low", Annotations: low},
				&mcp.TextContent{Text: "int", Annotations: integer},
				&mcp.ImageContent{Data: []byte{1}, MIMEType: "image/png", Annotations: contentAnnotations},
				&mcp.AudioContent{Data: []byte{1}, MIMEType: "audio/mp3", Annotations: contentAnnotations},
				&mcp.ResourceLink{URI: "test://doc", Name: "doc", Annotations: contentAnnotations},
				&mcp.EmbeddedResource{
					Resource:    &mcp.ResourceContents{URI: "test://doc", Text: "body"},
					Annotations: contentAnnotations,
				},
			},
		}, nil
	})
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

// TestCallTool_CodexClient_RoundsEveryContentBlockType verifies the rewrite
// covers every annotated content block type and both rounding directions:
// 0.4 rounds down to 0 (omitted on the wire), 0.6 rounds up to 1, and an
// already-integer priority is preserved.
func TestCallTool_CodexClient_RoundsEveryContentBlockType(t *testing.T) {
	server := newTestServer()
	addMixedContentTool(server)
	session := connectTo(t, server, codexImpl)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "mixed", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(res.Content) != 6 {
		t.Fatalf("content length = %d, want 6", len(res.Content))
	}
	annotationsOf := func(block mcp.Content) *mcp.Annotations {
		switch c := block.(type) {
		case *mcp.TextContent:
			return c.Annotations
		case *mcp.ImageContent:
			return c.Annotations
		case *mcp.AudioContent:
			return c.Annotations
		case *mcp.ResourceLink:
			return c.Annotations
		case *mcp.EmbeddedResource:
			return c.Annotations
		default:
			t.Fatalf("unexpected content type %T", block)
			return nil
		}
	}
	wantPriorities := []float64{0, 1, 1, 1, 1, 1}
	for i, want := range wantPriorities {
		a := annotationsOf(res.Content[i])
		if a == nil {
			t.Fatalf("content[%d] annotations dropped; want audience preserved", i)
		}
		if a.Priority != want {
			t.Errorf("content[%d] priority = %v, want %v", i, a.Priority, want)
		}
		if len(a.Audience) == 0 {
			t.Errorf("content[%d] audience dropped", i)
		}
	}
}

// TestCallTool_GenericClient_MixedContentKeepsFloats verifies the same mixed
// result reaches non-Codex clients with the original float priorities.
func TestCallTool_GenericClient_MixedContentKeepsFloats(t *testing.T) {
	server := newTestServer()
	addMixedContentTool(server)
	session := connectTo(t, server, genericImpl)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "mixed", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	if tc.Annotations == nil || tc.Annotations.Priority != 0.4 {
		t.Errorf("content[0] priority = %+v, want 0.4", tc.Annotations)
	}
	img, ok := res.Content[2].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[2] is %T, want *mcp.ImageContent", res.Content[2])
	}
	if img.Annotations == nil || img.Annotations.Priority != 0.6 {
		t.Errorf("image priority = %+v, want 0.6", img.Annotations)
	}
}

// TestMiddleware_ErrorAndNilResultPassThrough verifies the middleware
// forwards handler errors and nil results untouched instead of sanitizing.
func TestMiddleware_ErrorAndNilResultPassThrough(t *testing.T) {
	wantErr := context.DeadlineExceeded
	handler := clientcompat.Middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		return nil, wantErr
	})
	res, err := handler(context.Background(), "tools/call", &mcp.CallToolRequest{})
	if err == nil || res != nil {
		t.Fatalf("handler = (%v, %v), want (nil, error) passthrough", res, err)
	}

	nilHandler := clientcompat.Middleware()(func(_ context.Context, _ string, _ mcp.Request) (mcp.Result, error) {
		//nolint:nilnil // Intentional: the middleware must forward a nil result untouched.
		return nil, nil
	})
	res, err = nilHandler(context.Background(), "tools/call", &mcp.CallToolRequest{})
	if err != nil || res != nil {
		t.Fatalf("handler = (%v, %v), want (nil, nil) passthrough", res, err)
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

// TestProfileFromClientInfo_Branches verifies detection over every input
// shape, including the defensive nil clientInfo the public API cannot
// produce through a real session.
func TestProfileFromClientInfo_Branches(t *testing.T) {
	tests := []struct {
		name string
		impl *mcp.Implementation
		want clientcompat.Profile
	}{
		{name: "nil", impl: nil, want: clientcompat.ProfileDefault},
		{name: "codex_name", impl: &mcp.Implementation{Name: "codex-mcp-client"}, want: clientcompat.ProfileCodex},
		{name: "codex_title", impl: &mcp.Implementation{Name: "wrapper", Title: "Codex"}, want: clientcompat.ProfileCodex},
		{name: "case_insensitive", impl: &mcp.Implementation{Name: "CODEX-cli"}, want: clientcompat.ProfileCodex},
		{name: "generic", impl: &mcp.Implementation{Name: "claude-code", Title: "Claude Code"}, want: clientcompat.ProfileDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clientcompat.ProfileFromClientInfoForTest(tt.impl); got != tt.want {
				t.Errorf("profileFromClientInfo(%+v) = %v, want %v", tt.impl, got, tt.want)
			}
		})
	}
}

// TestProfileForRequest_DefensiveBranches verifies the session-resolution
// fallbacks: nil request, request without a session, and a server session
// whose initialize handshake has not happened yet (nil InitializeParams).
func TestProfileForRequest_DefensiveBranches(t *testing.T) {
	if got := clientcompat.ProfileForRequestForTest(nil); got != clientcompat.ProfileDefault {
		t.Errorf("profileForRequest(nil) = %v, want default", got)
	}
	if got := clientcompat.ProfileForRequestForTest(&mcp.CallToolRequest{}); got != clientcompat.ProfileDefault {
		t.Errorf("profileForRequest(no session) = %v, want default", got)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	serverTransport, _ := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	req := &mcp.CallToolRequest{Session: ss}
	if got := clientcompat.ProfileForRequestForTest(req); got != clientcompat.ProfileDefault {
		t.Errorf("profileForRequest(uninitialized session) = %v, want default", got)
	}
}

// TestRoundPriority_Branches verifies the pure rounding helper: nil and
// integer-valued annotations pass through unchanged (no clone), fractional
// values are rounded on a copy.
func TestRoundPriority_Branches(t *testing.T) {
	if got := clientcompat.RoundPriorityForTest(nil); got != nil {
		t.Errorf("roundPriority(nil) = %+v, want nil", got)
	}
	integer := &mcp.Annotations{Priority: 1}
	if got := clientcompat.RoundPriorityForTest(integer); got != integer {
		t.Errorf("roundPriority(integer) returned a clone; want same pointer")
	}
	zero := &mcp.Annotations{Audience: []mcp.Role{"user"}}
	if got := clientcompat.RoundPriorityForTest(zero); got != zero {
		t.Errorf("roundPriority(zero) returned a clone; want same pointer")
	}
	frac := &mcp.Annotations{Priority: 0.6}
	got := clientcompat.RoundPriorityForTest(frac)
	if got == frac || got.Priority != 1 {
		t.Errorf("roundPriority(0.6) = %+v (same=%v), want cloned priority 1", got, got == frac)
	}
	if frac.Priority != 0.6 {
		t.Errorf("roundPriority mutated its input: %v", frac.Priority)
	}
}

// TestRoundContentPriorities_EdgeBranches verifies the empty-slice fast path
// and the pass-through of content types outside the annotated set
// (ToolUseContent is a sampling-only block with no annotations field).
func TestRoundContentPriorities_EdgeBranches(t *testing.T) {
	if got := clientcompat.RoundContentPrioritiesForTest(nil); got != nil {
		t.Errorf("roundContentPriorities(nil) = %v, want nil", got)
	}
	//nolint:staticcheck // Deprecated sampling type used on purpose: it is the only SDK Content outside the annotated set.
	unknown := &mcp.ToolUseContent{ID: "x", Name: "y"}
	out := clientcompat.RoundContentPrioritiesForTest([]mcp.Content{unknown})
	if len(out) != 1 || out[0] != mcp.Content(unknown) {
		t.Errorf("unknown content type not passed through: %+v", out)
	}
}

// TestSanitizeForCodex_UnhandledResultPassesThrough verifies result types
// outside the sanitizer's switch are returned unchanged.
func TestSanitizeForCodex_UnhandledResultPassesThrough(t *testing.T) {
	res := &mcp.ListToolsResult{}
	if got := clientcompat.SanitizeForCodexForTest(res); got != mcp.Result(res) {
		t.Errorf("sanitizeForCodex(ListToolsResult) = %v, want same pointer", got)
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
