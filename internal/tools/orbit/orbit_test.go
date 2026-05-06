package orbit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

func TestStatus_Success_ExpectedOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{
			"status": "healthy",
			"timestamp": "2026-04-28T12:00:00Z",
			"version": "0.5.0",
			"components": [
				{"name": "clickhouse", "status": "healthy", "replicas": {"ready": 3, "desired": 3}, "metrics": {"kind": "Deployment"}}
			]
		}`)
	}))

	out, err := Status(context.Background(), client, StatusInput{})
	if err != nil {
		t.Fatalf("Status() error: %v", err)
	}
	if out.Status != "healthy" || out.Version != "0.5.0" {
		t.Fatalf("Status() = status %q version %q, want healthy 0.5.0", out.Status, out.Version)
	}
	if len(out.Components) != 1 || out.Components[0].Replicas.Ready != 3 {
		t.Fatalf("Status() components = %+v, want clickhouse replicas", out.Components)
	}
}

func TestSchema_WithExpandAndFormat_ForwardsQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/schema")
		testutil.AssertQueryParam(t, r, "expand", "User,Project")
		testutil.AssertQueryParam(t, r, "format", "llm")
		testutil.RespondJSON(w, http.StatusOK, `{
			"schema_version": "1.0",
			"domains": [{"name": "core", "description": "Core entities", "node_names": ["User", "Project"]}],
			"nodes": [{"name": "User"}],
			"edges": [{"name": "AUTHORED", "description": "Authorship", "variants": [{"source_type": "User", "target_type": "Issue"}]}]
		}`)
	}))

	out, err := Schema(context.Background(), client, SchemaInput{Expand: []string{"User", "Project"}, Format: "llm"})
	if err != nil {
		t.Fatalf("Schema() error: %v", err)
	}
	if out.SchemaVersion != "1.0" || len(out.Domains) != 1 || len(out.Edges) != 1 {
		t.Fatalf("Schema() = %+v, want decoded schema", out)
	}
}

func TestTools_Success_ExpectedOutput(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/tools")
		testutil.RespondJSON(w, http.StatusOK, `[
			{"name": "query_graph", "description": "Execute graph queries", "parameters": {"type": "object"}}
		]`)
	}))

	out, err := Tools(context.Background(), client, ToolsInput{})
	if err != nil {
		t.Fatalf("Tools() error: %v", err)
	}
	if len(out.Tools) != 1 || out.Tools[0].Name != "query_graph" {
		t.Fatalf("Tools() = %+v, want query_graph", out.Tools)
	}
}

func TestQuery_Success_ForwardsRawQuery(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/query")
		var got map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if string(got["query"]) != `{"query_type":"traversal"}` {
			t.Fatalf("query body = %s, want traversal query", got["query"])
		}
		if string(got["response_format"]) != `"raw"` {
			t.Fatalf("response_format = %s, want raw", got["response_format"])
		}
		testutil.RespondJSON(w, http.StatusOK, `{
			"result": [{"_id": "1", "_type": "Project"}],
			"query_type": "traversal",
			"raw_query_strings": ["SELECT ..."],
			"row_count": 1
		}`)
	}))

	out, err := Query(context.Background(), client, QueryInput{
		Query:               map[string]any{"query_type": "traversal"},
		ResponseFormatInput: ResponseFormatInput{ResponseFormat: "raw"},
	})
	if err != nil {
		t.Fatalf("Query() error: %v", err)
	}
	resultJSON, err := json.Marshal(out.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if out.QueryType != "traversal" || out.RowCount != 1 || !strings.Contains(string(resultJSON), "Project") {
		t.Fatalf("Query() = %+v, want traversal result", out)
	}
}

func TestGraphStatus_RequiresExactlyOneScope(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for invalid input: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))

	tests := []struct {
		name  string
		input GraphStatusInput
	}{
		{name: "none", input: GraphStatusInput{}},
		{name: "multiple", input: GraphStatusInput{NamespaceID: 1, ProjectID: 2}},
		{name: "negative namespace", input: GraphStatusInput{NamespaceID: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GraphStatus(context.Background(), client, tt.input)
			if err == nil {
				t.Fatal("GraphStatus() error = nil, want validation error")
			}
		})
	}
}

func TestGraphStatus_Success_ByFullPath(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/graph_status")
		testutil.AssertQueryParam(t, r, "full_path", "gitlab-org/gitlab")
		testutil.RespondJSON(w, http.StatusOK, `{
			"projects": {"indexed": 3, "total_known": 4},
			"domains": [{"name": "SDLC", "items": [{"name": "MergeRequest", "count": 42}]}],
			"indexing": {"state": "indexed", "last_duration_ms": 99}
		}`)
	}))

	out, err := GraphStatus(context.Background(), client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
	if err != nil {
		t.Fatalf("GraphStatus() error: %v", err)
	}
	if out.Projects.Indexed != 3 || out.Indexing.State != "indexed" || out.Domains[0].Items[0].Count != 42 {
		t.Fatalf("GraphStatus() = %+v, want indexed graph status", out)
	}
}

func TestOrbit_ValidationErrors_ReturnActionableErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called for invalid input: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))

	tests := []struct {
		name string
		call func() error
		want string
	}{
		{
			name: "invalid response format",
			call: func() error {
				_, err := Status(context.Background(), client, StatusInput{ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"}})
				return err
			},
			want: "use raw or llm",
		},
		{
			name: "empty query",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{})
				return err
			},
			want: "query",
		},
		{
			name: "unmarshalable query",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{Query: map[string]any{"bad": func() {}}})
				return err
			},
			want: "JSON object",
		},
		{
			name: "invalid schema format",
			call: func() error {
				_, err := Schema(context.Background(), client, SchemaInput{Format: "xml"})
				return err
			},
			want: "use raw or llm",
		},
		{
			name: "invalid query response format",
			call: func() error {
				_, err := Query(context.Background(), client, QueryInput{
					Query:               map[string]any{"query_type": "traversal"},
					ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"},
				})
				return err
			},
			want: "use raw or llm",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestOrbit_HTTPErrorHints_ReturnExpectedGuidance(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		status int
		call   func(context.Context, *gitlabclient.Client) error
		want   string
	}{
		{
			name:   "not found",
			path:   "/api/v4/orbit/status",
			status: http.StatusNotFound,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Status(ctx, client, StatusInput{})
				return err
			},
			want: "knowledge_graph",
		},
		{
			name:   "schema forbidden",
			path:   "/api/v4/orbit/schema",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Schema(ctx, client, SchemaInput{})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "tools forbidden",
			path:   "/api/v4/orbit/tools",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Tools(ctx, client, ToolsInput{})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "forbidden",
			path:   "/api/v4/orbit/query",
			status: http.StatusForbidden,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := Query(ctx, client, QueryInput{Query: map[string]any{"query_type": "traversal"}})
				return err
			},
			want: "Knowledge Graph enabled",
		},
		{
			name:   "bad request",
			path:   "/api/v4/orbit/graph_status",
			status: http.StatusBadRequest,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
				return err
			},
			want: "check the Orbit query",
		},
		{
			name:   "service unavailable",
			path:   "/api/v4/orbit/graph_status",
			status: http.StatusServiceUnavailable,
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
				return err
			},
			want: "temporarily unavailable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestPath(t, r, tt.path)
				testutil.RespondJSON(w, tt.status, `{"message":"error"}`)
			}))
			err := tt.call(context.Background(), client)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestWrapOrbitErr_GenericError_UsesWrappedMessage(t *testing.T) {
	err := wrapOrbitErr("orbit_status", errors.New("network unavailable"))
	if err == nil || !strings.Contains(err.Error(), "orbit_status") || !strings.Contains(err.Error(), "network unavailable") {
		t.Fatalf("wrapOrbitErr() = %v, want operation context and source error", err)
	}
}

func TestOrbitHandlers_ContextCancellation_ReturnsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("handler should not be called after context cancellation: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	ctx := testutil.CancelledCtx(t)
	tests := []struct {
		name string
		call func() error
	}{
		{name: "status", call: func() error { _, err := Status(ctx, client, StatusInput{}); return err }},
		{name: "schema", call: func() error { _, err := Schema(ctx, client, SchemaInput{}); return err }},
		{name: "tools", call: func() error { _, err := Tools(ctx, client, ToolsInput{}); return err }},
		{name: "query", call: func() error {
			_, err := Query(ctx, client, QueryInput{Query: map[string]any{"query_type": "traversal"}})
			return err
		}},
		{name: "graph status", call: func() error {
			_, err := GraphStatus(ctx, client, GraphStatusInput{FullPath: "gitlab-org/gitlab"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err == nil {
				t.Fatal("handler error = nil, want context cancellation error")
			}
		})
	}
}

func TestOrbit_RegisterTools_RegistersToolDefinitions(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterTools(server, client)
	names := registeredToolNames(t, server)
	for _, want := range []string{"gitlab_orbit_status", "gitlab_orbit_schema", "gitlab_orbit_tools", "gitlab_orbit_query", "gitlab_orbit_graph_status"} {
		if !containsTool(names, want) {
			t.Fatalf("RegisterTools() missing %s in %v", want, names)
		}
	}
}

func TestOrbit_RegisterMeta_RegistersMetaTool(t *testing.T) {
	client, err := gitlabclient.NewClientWithToken("https://gitlab.example.com", "test-token", false)
	if err != nil {
		t.Fatalf("NewClientWithToken() error: %v", err)
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	RegisterMeta(server, client)
	if names := registeredToolNames(t, server); !containsTool(names, "gitlab_orbit") {
		t.Fatalf("RegisterMeta() missing gitlab_orbit in %v", names)
	}
}

func TestOrbit_RegisterMeta_CallThroughMCP(t *testing.T) {
	session := newOrbitMetaSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/status")
		testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy","version":"0.5.0"}`)
	}))

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "gitlab_orbit",
		Arguments: map[string]any{"action": "status", "params": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatal("gitlab_orbit status returned error result")
	}
}

func TestOrbit_RegisterTools_CallThroughMCP(t *testing.T) {
	session := newOrbitSession(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v4/orbit/status":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"status":"healthy","version":"0.5.0"}`)
		case "/api/v4/orbit/schema":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"schema_version":"1.0"}`)
		case "/api/v4/orbit/tools":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `[{"name":"query_graph","description":"Execute graph queries","parameters":{"type":"object"}}]`)
		case "/api/v4/orbit/query":
			testutil.AssertRequestMethod(t, r, http.MethodPost)
			testutil.RespondJSON(w, http.StatusOK, `{"result":[],"query_type":"traversal","row_count":0}`)
		case "/api/v4/orbit/graph_status":
			testutil.AssertRequestMethod(t, r, http.MethodGet)
			testutil.RespondJSON(w, http.StatusOK, `{"projects":{"indexed":1,"total_known":1},"indexing":{"state":"indexed"}}`)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))

	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "gitlab_orbit_status", args: map[string]any{}},
		{name: "gitlab_orbit_schema", args: map[string]any{}},
		{name: "gitlab_orbit_tools", args: map[string]any{}},
		{name: "gitlab_orbit_query", args: map[string]any{"query": map[string]any{"query_type": "traversal"}}},
		{name: "gitlab_orbit_graph_status", args: map[string]any{"full_path": "gitlab-org/gitlab"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.name, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool() error: %v", err)
			}
			if result.IsError {
				t.Fatalf("CallTool() returned error result for %s", tt.name)
			}
		})
	}
}

func TestFormatQueryMarkdown_IncludesPrettyJSON(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType: "traversal",
		RowCount:  1,
		Result:    []any{map[string]any{"name": "alpha"}},
	})
	if !strings.Contains(md, "```json") || !strings.Contains(md, "alpha") {
		t.Fatalf("FormatQueryMarkdown() = %q, want JSON result", md)
	}
}

func TestOrbitMarkdownFormatters_IncludeExpectedSections(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want []string
	}{
		{
			name: "status structured",
			md: FormatStatusMarkdown(StatusOutput{
				Status:  "healthy",
				Version: "0.5.0",
				Components: []StatusComponent{
					{Name: "clickhouse", Status: "healthy", Replicas: &StatusReplicas{Ready: 3, Desired: 3}},
				},
			}),
			want: []string{"Orbit Status", "clickhouse", "3/3"},
		},
		{
			name: "status formatted",
			md:   FormatStatusMarkdown(StatusOutput{FormattedText: "status: healthy"}),
			want: []string{"```text", "status: healthy"},
		},
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{
				SchemaVersion: "1.0",
				Domains:       []SchemaDomain{{Name: "core", Description: "Core entities", NodeNames: []string{"User"}}},
				Nodes:         []any{map[string]any{"name": "User"}},
				Edges:         []SchemaEdge{{Name: "AUTHORED"}},
			}),
			want: []string{"Orbit Schema", "Schema version", "core"},
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query_graph", Description: "Execute graph queries"}}}),
			want: []string{"Orbit Tools", "query_graph"},
		},
		{
			name: "graph status structured",
			md: FormatGraphStatusMarkdown(GraphStatusOutput{
				Projects: &GraphStatusProjects{Indexed: 2, TotalKnown: 3},
				Domains:  []GraphStatusDomain{{Name: "SDLC", Items: []GraphStatusDomainItem{{Name: "Issue", Count: 4}}}},
				Indexing: &GraphStatusIndexing{State: "indexed", LastDurationMs: 5},
			}),
			want: []string{"Orbit Graph Status", "Indexed projects", "SDLC"},
		},
		{
			name: "graph status formatted",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{FormattedText: "indexing: indexed"}),
			want: []string{"```text", "indexing: indexed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, want := range tt.want {
				if !strings.Contains(tt.md, want) {
					t.Fatalf("markdown = %q, want substring %q", tt.md, want)
				}
			}
		})
	}
}

func TestOrbitMarkdownFormatters_UseSafeFences(t *testing.T) {
	md := FormatQueryMarkdown(QueryOutput{
		QueryType:       "traversal",
		RawQueryStrings: []string{"MATCH (n) RETURN ```"},
		Result:          map[string]any{"text": "contains ``` fenced text"},
	})

	if !strings.Contains(md, "````text\nMATCH (n) RETURN ```\n````") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick text fence", md)
	}
	if !strings.Contains(md, "````json\n") || !strings.Contains(md, "contains ``` fenced text") {
		t.Fatalf("FormatQueryMarkdown() = %q, want four-backtick JSON fence", md)
	}
}

func TestOrbitMarkdownFormatters_EscapeTableCells(t *testing.T) {
	tests := []struct {
		name string
		md   string
		want string
	}{
		{
			name: "schema",
			md: FormatSchemaMarkdown(SchemaOutput{Domains: []SchemaDomain{{
				Name:        "core|domain",
				Description: "Core\nentities",
				NodeNames:   []string{"User|Account"},
			}}}),
			want: "core&#124;domain | Core entities | User&#124;Account",
		},
		{
			name: "tools",
			md:   FormatToolsMarkdown(ToolsOutput{Tools: []ToolDefinition{{Name: "query|graph", Description: "Run\nqueries"}}}),
			want: "`query&#124;graph` | Run queries",
		},
		{
			name: "graph status",
			md:   FormatGraphStatusMarkdown(GraphStatusOutput{Domains: []GraphStatusDomain{{Name: "SDLC|core", Items: []GraphStatusDomainItem{{Name: "Issue|Bug", Count: 4}}}}}),
			want: "SDLC&#124;core | Issue&#124;Bug: 4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.md, tt.want) {
				t.Fatalf("markdown = %q, want escaped substring %q", tt.md, tt.want)
			}
		})
	}
}

func TestOrbitMarkdownAndDynamicJSON_FallbackBranches(t *testing.T) {
	if md := FormatStatusMarkdown(StatusOutput{}); !strings.Contains(md, "No Orbit status data") {
		t.Fatalf("FormatStatusMarkdown() = %q, want empty-state text", md)
	}
	if md := FormatToolsMarkdown(ToolsOutput{}); !strings.Contains(md, "No Orbit tools") {
		t.Fatalf("FormatToolsMarkdown() = %q, want empty-state text", md)
	}
	if md := FormatQueryMarkdown(QueryOutput{QueryType: "traversal", RawQueryStrings: []string{"MATCH (n) RETURN n"}}); !strings.Contains(md, "Raw Query Strings") || !strings.Contains(md, "MATCH (n)") {
		t.Fatalf("FormatQueryMarkdown() = %q, want raw query strings", md)
	}
	if md := FormatQueryMarkdown(QueryOutput{QueryType: "traversal"}); strings.Contains(md, "### Result") {
		t.Fatalf("FormatQueryMarkdown() = %q, did not expect result section", md)
	}
	if prettyAny(func() {}) == "" {
		t.Fatal("prettyAny() returned empty fallback")
	}
	if got := decodeRaw(json.RawMessage(`{`)); got != "{" {
		t.Fatalf("decodeRaw() = %v, want raw fallback", got)
	}
	if decodeRaw(nil) != nil {
		t.Fatal("decodeRaw(nil) = non-nil, want nil")
	}
	if out := convertStatus(nil); out.Status != "" {
		t.Fatalf("convertStatus(nil) = %+v, want zero value", out)
	}
	if out := convertSchema(nil); out.SchemaVersion != "" {
		t.Fatalf("convertSchema(nil) = %+v, want zero value", out)
	}
	if out := convertTools(nil); len(out.Tools) != 0 {
		t.Fatalf("convertTools(nil) = %+v, want zero tools", out)
	}
	if out := convertQuery(nil); out.QueryType != "" {
		t.Fatalf("convertQuery(nil) = %+v, want zero value", out)
	}
	if out := convertGraphStatus(nil); out.Projects != nil {
		t.Fatalf("convertGraphStatus(nil) = %+v, want zero value", out)
	}
}

func TestGraphStatusOptions_SetsEachSupportedScope(t *testing.T) {
	tests := []struct {
		name  string
		input GraphStatusInput
		check func(*testing.T, *gl.GetGraphStatusOptions)
	}{
		{
			name:  "namespace",
			input: GraphStatusInput{NamespaceID: 123},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.NamespaceID == nil || *opts.NamespaceID != 123 {
					t.Fatalf("NamespaceID = %v, want 123", opts.NamespaceID)
				}
			},
		},
		{
			name:  "project",
			input: GraphStatusInput{ProjectID: 456},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.ProjectID == nil || *opts.ProjectID != 456 {
					t.Fatalf("ProjectID = %v, want 456", opts.ProjectID)
				}
			},
		},
		{
			name:  "full path with llm format",
			input: GraphStatusInput{FullPath: " gitlab-org/gitlab ", ResponseFormatInput: ResponseFormatInput{ResponseFormat: "llm"}},
			check: func(t *testing.T, opts *gl.GetGraphStatusOptions) {
				t.Helper()
				if opts.FullPath == nil || *opts.FullPath != "gitlab-org/gitlab" {
					t.Fatalf("FullPath = %v, want trimmed path", opts.FullPath)
				}
				if opts.ResponseFormat == nil || string(*opts.ResponseFormat) != "llm" {
					t.Fatalf("ResponseFormat = %v, want llm", opts.ResponseFormat)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := graphStatusOptions(tt.input)
			if err != nil {
				t.Fatalf("graphStatusOptions() error: %v", err)
			}
			tt.check(t, opts)
		})
	}
}

func TestGraphStatusOptions_InvalidProjectAndFormat(t *testing.T) {
	tests := []struct {
		name  string
		input GraphStatusInput
	}{
		{name: "negative project ID", input: GraphStatusInput{ProjectID: -1}},
		{name: "invalid response format", input: GraphStatusInput{FullPath: "gitlab-org/gitlab", ResponseFormatInput: ResponseFormatInput{ResponseFormat: "xml"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := graphStatusOptions(tt.input); err == nil {
				t.Fatalf("graphStatusOptions(%+v) error = nil, want validation error", tt.input)
			}
		})
	}
}

func TestOrbitConverters_SkipNilNestedEntriesAndPreserveOptionalFields(t *testing.T) {
	status := convertStatus(&gl.OrbitStatus{Components: []*gl.OrbitStatusComponent{nil, {Name: "api", Status: "healthy"}}})
	if len(status.Components) != 1 || status.Components[0].Name != "api" || status.Components[0].Replicas != nil {
		t.Fatalf("convertStatus() = %+v, want one component without replicas", status.Components)
	}

	schema := convertSchema(&gl.OrbitSchema{
		Domains: []*gl.OrbitSchemaDomain{nil, {Name: "core", NodeNames: []string{"Project"}}},
		Edges: []*gl.OrbitSchemaEdge{nil, {
			Name:     "AUTHORED",
			Variants: []*gl.OrbitSchemaEdgeVariant{nil, {SourceType: "User", TargetType: "Issue"}},
		}},
	})
	if len(schema.Domains) != 1 || len(schema.Edges) != 1 || len(schema.Edges[0].Variants) != 1 {
		t.Fatalf("convertSchema() = %+v, want nil entries skipped", schema)
	}

	tools := convertTools(&gl.OrbitTools{Tools: []*gl.OrbitTool{nil, {Name: "query_graph", Parameters: json.RawMessage(`{"type":"object"}`)}}})
	if len(tools.Tools) != 1 || tools.Tools[0].Name != "query_graph" {
		t.Fatalf("convertTools() = %+v, want one tool", tools.Tools)
	}

	started := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	completed := started.Add(2 * time.Minute)
	duration := int64(120000)
	lastErr := "index timeout"
	graphStatus := convertGraphStatus(&gl.OrbitGraphStatus{
		Domains: []*gl.OrbitGraphStatusDomain{nil, {Name: "SDLC", Items: []*gl.OrbitGraphStatusDomainItem{nil, {Name: "Issue", Count: 7}}}},
		Indexing: &gl.OrbitGraphStatusIndexing{
			State:           "error",
			LastStartedAt:   &started,
			LastCompletedAt: &completed,
			LastDurationMs:  &duration,
			LastError:       &lastErr,
		},
	})
	if len(graphStatus.Domains) != 1 || len(graphStatus.Domains[0].Items) != 1 {
		t.Fatalf("convertGraphStatus() domains = %+v, want nil entries skipped", graphStatus.Domains)
	}
	if graphStatus.Indexing.LastStartedAt == "" || graphStatus.Indexing.LastCompletedAt == "" || graphStatus.Indexing.LastError != lastErr {
		t.Fatalf("convertGraphStatus() indexing = %+v, want optional fields", graphStatus.Indexing)
	}
}

func registeredToolNames(t *testing.T, server *mcp.Server) []string {
	t.Helper()
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func newOrbitSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	return newOrbitMCPSession(t, handler, RegisterTools)
}

func newOrbitMetaSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	return newOrbitMCPSession(t, handler, RegisterMeta)
}

func newOrbitMCPSession(t *testing.T, handler http.Handler, registerFn func(*mcp.Server, *gitlabclient.Client)) *mcp.ClientSession {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	registerFn(server, client)
	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})
	return session
}

func containsTool(names []string, want string) bool {
	return slices.Contains(names, want)
}
