// middleware_test.go verifies the list-rewrite middleware over a real
// in-memory client/server pair: what a gateway receives from the four
// catalog listings is rewritten, everything that is contract rather than
// prose survives verbatim, and the server's own registries are never
// mutated.
package gatewaycompat_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
)

// semicolonToPeriod is the substitution the package exists for: the rule one
// production gateway enforced on tool descriptions.
var semicolonToPeriod = []gatewaycompat.Substitution{{Old: ";", New: "."}}

// newListServer builds a server carrying a semicolon in every rewritable
// position — descriptions, titles, an annotation title, schema prose — and
// in every position that must survive: names, a pattern, an enum value, a
// default, and a property named "description".
func newListServer(subs []gatewaycompat.Substitution) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	server.AddReceivingMiddleware(gatewaycompat.Middleware(subs))

	server.AddTool(&mcp.Tool{
		Name:        "list;things",
		Description: "Lists things; supports filters",
		Title:       "List; things",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, Title: "Things; listed"},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"filter": map[string]any{
					"type":        "string",
					"description": "One of open; closed",
					"pattern":     "^[a-z;]+$",
					"enum":        []any{"open;now", "closed"},
					"default":     "open;now",
				},
				// A property named like a schema keyword: its nested prose is
				// still prose, its data still data.
				"description": map[string]any{
					"type":        "string",
					"description": "Body text; markdown",
				},
			},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"count": map[string]any{"type": "integer", "description": "Total; capped"},
			},
		},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "review;prompt",
		Description: "Reviews code; thoroughly",
		Title:       "Review; code",
		Arguments: []*mcp.PromptArgument{
			{Name: "target;arg", Description: "What to review; a path", Title: "Target; path"},
		},
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{
			{Role: "user", Content: &mcp.TextContent{Text: "review"}},
		}}, nil
	})

	server.AddResource(&mcp.Resource{
		URI:         "test://doc;1",
		Name:        "doc;one",
		Title:       "Doc; one",
		Description: "A document; served",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "body"}}}, nil
	})

	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "test://doc;x/{id}",
		Name:        "doc;by-id",
		Title:       "Doc; by id",
		Description: "A document; by id",
	}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "body"}}}, nil
	})

	return server
}

// connect wires a client to the server over in-memory transports.
func connect(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestMiddleware_ToolsList_RewritesProseAndPreservesContract verifies the
// tools/list rewrite end to end: description, title, annotation title and
// every schema prose string lose their semicolons, while the name, the
// pattern, the enum values and the default keep theirs.
func TestMiddleware_ToolsList_RewritesProseAndPreservesContract(t *testing.T) {
	session := connect(t, newListServer(semicolonToPeriod))

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}
	if len(res.Tools) != 1 {
		t.Fatalf("tools/list returned %d tools, want 1", len(res.Tools))
	}
	tool := res.Tools[0]

	if tool.Name != "list;things" {
		t.Errorf("tool name = %q; a name is contract and must survive verbatim", tool.Name)
	}
	if tool.Description != "Lists things. supports filters" {
		t.Errorf("tool description = %q, want the semicolon rewritten", tool.Description)
	}
	if tool.Title != "List. things" {
		t.Errorf("tool title = %q, want the semicolon rewritten", tool.Title)
	}
	if tool.Annotations == nil || tool.Annotations.Title != "Things. listed" {
		t.Errorf("annotations = %+v, want the title semicolon rewritten", tool.Annotations)
	}
	if tool.Annotations != nil && !tool.Annotations.ReadOnlyHint {
		t.Error("annotations lost ReadOnlyHint in the clone")
	}

	raw, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal input schema: %v", err)
	}
	schema := string(raw)
	for _, want := range []string{
		`"description":"One of open. closed"`, // prose keyword rewritten
		`"description":"Body text. markdown"`, // prose inside a property named "description"
		`"pattern":"^[a-z;]+$"`,               // pattern is contract
		`"open;now"`,                          // enum value and default are data
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(schema, want) {
				t.Errorf("input schema %s\nmissing %s", schema, want)
			}
		})
	}

	rawOut, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal output schema: %v", err)
	}
	if !strings.Contains(string(rawOut), `"description":"Total. capped"`) {
		t.Errorf("output schema %s missing rewritten description", rawOut)
	}
}

// TestMiddleware_PromptsList_RewritesProseAndPreservesNames verifies the
// prompts listing: prompt and argument prose is rewritten while prompt and
// argument names survive verbatim.
func TestMiddleware_PromptsList_RewritesProseAndPreservesNames(t *testing.T) {
	session := connect(t, newListServer(semicolonToPeriod))

	prompts, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("prompts/list: %v", err)
	}
	if len(prompts.Prompts) != 1 {
		t.Fatalf("prompts/list returned %d prompts, want 1", len(prompts.Prompts))
	}
	prompt := prompts.Prompts[0]
	if prompt.Name != "review;prompt" {
		t.Errorf("prompt name = %q, must survive verbatim", prompt.Name)
	}
	if prompt.Description != "Reviews code. thoroughly" || prompt.Title != "Review. code" {
		t.Errorf("prompt prose = %q / %q, want semicolons rewritten", prompt.Description, prompt.Title)
	}
	if len(prompt.Arguments) != 1 {
		t.Fatalf("prompt has %d arguments, want 1", len(prompt.Arguments))
	}
	arg := prompt.Arguments[0]
	if arg.Name != "target;arg" {
		t.Errorf("argument name = %q, must survive verbatim", arg.Name)
	}
	if arg.Description != "What to review. a path" || arg.Title != "Target. path" {
		t.Errorf("argument prose = %q / %q, want semicolons rewritten", arg.Description, arg.Title)
	}
}

// TestMiddleware_ResourceLists_RewriteProseAndPreserveURIs verifies the two
// resource listings: descriptions and titles are rewritten while URIs, URI
// templates and names survive verbatim.
func TestMiddleware_ResourceLists_RewriteProseAndPreserveURIs(t *testing.T) {
	session := connect(t, newListServer(semicolonToPeriod))

	resources, err := session.ListResources(context.Background(), nil)
	if err != nil {
		t.Fatalf("resources/list: %v", err)
	}
	if len(resources.Resources) != 1 {
		t.Fatalf("resources/list returned %d resources, want 1", len(resources.Resources))
	}
	resource := resources.Resources[0]
	if resource.URI != "test://doc;1" || resource.Name != "doc;one" {
		t.Errorf("resource identity = %q / %q, must survive verbatim", resource.URI, resource.Name)
	}
	if resource.Description != "A document. served" || resource.Title != "Doc. one" {
		t.Errorf("resource prose = %q / %q, want semicolons rewritten", resource.Description, resource.Title)
	}

	templates, err := session.ListResourceTemplates(context.Background(), nil)
	if err != nil {
		t.Fatalf("resources/templates/list: %v", err)
	}
	if len(templates.ResourceTemplates) != 1 {
		t.Fatalf("templates list returned %d templates, want 1", len(templates.ResourceTemplates))
	}
	template := templates.ResourceTemplates[0]
	if template.URITemplate != "test://doc;x/{id}" || template.Name != "doc;by-id" {
		t.Errorf("template identity = %q / %q, must survive verbatim", template.URITemplate, template.Name)
	}
	if template.Description != "A document. by id" || template.Title != "Doc. by id" {
		t.Errorf("template prose = %q / %q, want semicolons rewritten", template.Description, template.Title)
	}
}

// TestMiddleware_RepeatedLists_DoNotCompound verifies the clone-before-write
// contract with a substitution that is not idempotent (";" doubles to ";;"):
// if the middleware mutated the registry's own objects instead of clones,
// the second listing would compound the first rewrite and serve four
// semicolons where the source had one.
func TestMiddleware_RepeatedLists_DoNotCompound(t *testing.T) {
	doubling := []gatewaycompat.Substitution{{Old: ";", New: ";;"}}
	session := connect(t, newListServer(doubling))

	const want = "Lists things;; supports filters"
	for round := 1; round <= 2; round++ {
		res, err := session.ListTools(context.Background(), nil)
		if err != nil {
			t.Fatalf("tools/list round %d: %v", round, err)
		}
		if got := res.Tools[0].Description; got != want {
			t.Fatalf("round %d description = %q, want %q — the registry was mutated in place", round, got, want)
		}
	}
}

// TestMiddleware_NonListResults_PassThrough verifies the payload boundary: a
// tool call result with a semicolon in its text is served untouched, because
// the knob rewrites how the server introduces itself, never what it returns.
func TestMiddleware_NonListResults_PassThrough(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "0.0.1"}, nil)
	server.AddReceivingMiddleware(gatewaycompat.Middleware(semicolonToPeriod))
	server.AddTool(&mcp.Tool{
		Name:        "emit",
		Description: "emits",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
	}, func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "a; b"}}}, nil
	})
	session := connect(t, server)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "emit"})
	if err != nil {
		t.Fatalf("tools/call: %v", err)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	if text.Text != "a; b" {
		t.Errorf("call result text = %q, payload must survive verbatim", text.Text)
	}
}

// TestMiddleware_FailedCall_IsPassedThroughUntouched covers the guard the
// rewriting switch stands behind: a handler that failed, or that answered with
// no result at all, has nothing to rewrite.
//
// The distinction matters because every branch below the guard type-asserts the
// result. A nil result reaching the switch would fall through to the default and
// be returned unchanged anyway, but an error result must keep its error:
// dropping it here would turn a refused call into a successful empty one.
func TestMiddleware_FailedCall_IsPassedThroughUntouched(t *testing.T) {
	t.Parallel()

	failed := errors.New("the handler refused this call")
	tests := []struct {
		name    string
		res     mcp.Result
		err     error
		wantErr error
	}{
		{name: "an error and no result", res: nil, err: failed, wantErr: failed},
		{name: "no result and no error", res: nil, err: nil, wantErr: nil},
		{
			name:    "a result alongside an error is not rewritten",
			res:     &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "x", Description: "a;b"}}},
			err:     failed,
			wantErr: failed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := gatewaycompat.Middleware(semicolonToPeriod)(
				func(context.Context, string, mcp.Request) (mcp.Result, error) {
					return tt.res, tt.err
				},
			)

			res, err := handler(context.Background(), "tools/list", nil)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.res == nil && res != nil {
				t.Errorf("result = %#v, want the handler's own nil", res)
			}
			if tt.res != nil && res != tt.res {
				t.Errorf("result = %#v, want the handler's own result returned unrewritten", res)
			}
		})
	}
}

// TestMiddleware_UnrewritableSchema_KeepsItsOriginalValue covers the two ways a
// tool's schema comes back exactly as it arrived.
//
// A schema that cannot be marshaled has no JSON form to walk, and a schema with
// no prose in it has nothing to change; in both cases the original typed value
// is kept rather than a decoded copy, so a caller's *jsonschema.Schema stays a
// *jsonschema.Schema. Returning the decoded map instead would marshal to the
// same wire bytes while silently changing the type every other layer sees.
//
// The listing is handed to the middleware directly rather than served over a
// session, because one of the cases is a schema the JSON encoder refuses and no
// session could carry it.
func TestMiddleware_UnrewritableSchema_KeepsItsOriginalValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema any
	}{
		{name: "nothing to marshal", schema: make(chan int)},
		{name: "no prose to rewrite", schema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}}},
		{name: "prose the substitutions do not touch", schema: map[string]any{"description": "nothing here"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			handler := gatewaycompat.Middleware(semicolonToPeriod)(
				func(context.Context, string, mcp.Request) (mcp.Result, error) {
					return &mcp.ListToolsResult{Tools: []*mcp.Tool{{Name: "t", InputSchema: tt.schema}}}, nil
				},
			)

			res, err := handler(context.Background(), "tools/list", nil)
			if err != nil {
				t.Fatalf("middleware returned %v for a listing it cannot rewrite", err)
			}
			listed, ok := res.(*mcp.ListToolsResult)
			if !ok || len(listed.Tools) != 1 {
				t.Fatalf("result = %#v, want one listed tool", res)
			}

			if !reflect.DeepEqual(listed.Tools[0].InputSchema, tt.schema) {
				t.Errorf("input schema = %#v, want the original %#v", listed.Tools[0].InputSchema, tt.schema)
			}
		})
	}
}
