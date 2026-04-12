// Package compliancepolicy tests validate the Get, Update, FormatOutputMarkdown,
// and RegisterTools functions for the admin compliance policy settings MCP tools.
// Tests cover success paths, API error responses (403, 400, 500), context
// cancellation, nil/non-nil CSPNamespaceID, markdown formatting, and full MCP
// round-trip registration.
package compliancepolicy

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const testVersion = "0.0.0-test"

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// TestGet validates the Get function across success, error, nil-field, and
// context-cancellation scenarios.
func TestGet(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ctx        func() context.Context
		wantErr    bool
		wantNilCSP bool
		wantCSP    int64
	}{
		{
			name: "returns compliance policy settings with csp_namespace_id",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, "/api/v4/admin/security/compliance_policy_settings")
				testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":123}`)
			}),
			wantCSP: 123,
		},
		{
			name: "returns nil csp_namespace_id when field is absent",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{}`)
			}),
			wantNilCSP: true,
		},
		{
			name: "returns error on 403 forbidden",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			}),
			wantErr: true,
		},
		{
			name: "returns error on 500 internal server error",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"internal error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when context is cancelled",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)

			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}

			out, err := Get(ctx, client, GetInput{})
			if (err != nil) != tt.wantErr {
				t.Fatalf("Get() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNilCSP {
				if out.CSPNamespaceID != nil {
					t.Errorf("expected nil CSPNamespaceID, got %d", *out.CSPNamespaceID)
				}
				return
			}
			if out.CSPNamespaceID == nil {
				t.Fatal("expected non-nil CSPNamespaceID, got nil")
			}
			if *out.CSPNamespaceID != tt.wantCSP {
				t.Errorf("CSPNamespaceID = %d, want %d", *out.CSPNamespaceID, tt.wantCSP)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// TestUpdate validates the Update function across success, error, nil-input, and
// context-cancellation scenarios.
func TestUpdate(t *testing.T) {
	nsID := int64(456)
	zeroID := int64(0)

	tests := []struct {
		name       string
		input      UpdateInput
		handler    http.HandlerFunc
		ctx        func() context.Context
		wantErr    bool
		wantNilCSP bool
		wantCSP    int64
	}{
		{
			name:  "updates csp_namespace_id successfully",
			input: UpdateInput{CSPNamespaceID: &nsID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPut)
				testutil.AssertRequestPath(t, r, "/api/v4/admin/security/compliance_policy_settings")
				testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":456}`)
			}),
			wantCSP: 456,
		},
		{
			name:  "updates with nil csp_namespace_id",
			input: UpdateInput{CSPNamespaceID: nil},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{}`)
			}),
			wantNilCSP: true,
		},
		{
			name:  "updates with zero value csp_namespace_id",
			input: UpdateInput{CSPNamespaceID: &zeroID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":0}`)
			}),
			wantCSP: 0,
		},
		{
			name:  "returns error on 400 bad request",
			input: UpdateInput{CSPNamespaceID: &nsID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 500 internal server error",
			input: UpdateInput{CSPNamespaceID: &nsID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"internal error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when context is cancelled",
			input:   UpdateInput{CSPNamespaceID: &nsID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)

			ctx := context.Background()
			if tt.ctx != nil {
				ctx = tt.ctx()
			}

			out, err := Update(ctx, client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantNilCSP {
				if out.CSPNamespaceID != nil {
					t.Errorf("expected nil CSPNamespaceID, got %d", *out.CSPNamespaceID)
				}
				return
			}
			if out.CSPNamespaceID == nil {
				t.Fatal("expected non-nil CSPNamespaceID, got nil")
			}
			if *out.CSPNamespaceID != tt.wantCSP {
				t.Errorf("CSPNamespaceID = %d, want %d", *out.CSPNamespaceID, tt.wantCSP)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------

// TestFormatOutputMarkdown validates the Markdown formatter for compliance policy
// settings output, covering both set and unset CSPNamespaceID values.
func TestFormatOutputMarkdown(t *testing.T) {
	nsID := int64(42)

	tests := []struct {
		name     string
		output   Output
		contains []string
		excludes []string
	}{
		{
			name:   "formats output with csp_namespace_id set",
			output: Output{CSPNamespaceID: &nsID},
			contains: []string{
				"## Compliance Policy Settings",
				"| CSP Namespace ID | 42 |",
				"Field",
				"Value",
			},
			excludes: []string{
				"_not set_",
			},
		},
		{
			name:   "formats output with nil csp_namespace_id",
			output: Output{CSPNamespaceID: nil},
			contains: []string{
				"## Compliance Policy Settings",
				"| CSP Namespace ID | _not set_ |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatOutputMarkdown(tt.output)
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected output to contain %q, got:\n%s", s, result)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(result, s) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", s, result)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RegisterTools
// ---------------------------------------------------------------------------

// TestRegisterTools_NoPanic verifies that RegisterTools registers both compliance
// policy tools on an MCP server without panicking.
func TestRegisterTools_NoPanic(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)
}

// TestRegisterTools_CallAllThroughMCP validates that both registered tools can be
// invoked through a full MCP client-server round-trip and return valid results.
func TestRegisterTools_CallAllThroughMCP(t *testing.T) {
	session := newCompliancePolicyMCPSession(t)
	ctx := context.Background()

	tools := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"get_settings", "gitlab_get_compliance_policy_settings", map[string]any{}},
		{"update_settings", "gitlab_update_compliance_policy_settings", map[string]any{"csp_namespace_id": 200}},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if callErr != nil {
				t.Fatalf("CallTool(%s) error: %v", tt.tool, callErr)
			}
			if result.IsError {
				for _, c := range result.Content {
					if tc, ok := c.(*mcp.TextContent); ok {
						t.Fatalf("CallTool(%s) returned error: %s", tt.tool, tc.Text)
					}
				}
				t.Fatalf("CallTool(%s) returned IsError=true", tt.tool)
			}
		})
	}
}

// newCompliancePolicyMCPSession creates a full MCP client-server session backed
// by mock handlers for both compliance policy tools.
func newCompliancePolicyMCPSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/admin/security/compliance_policy_settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":100}`)
	})
	handler.HandleFunc("PUT /api/v4/admin/security/compliance_policy_settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":200}`)
	})

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: testVersion}, nil)
	RegisterTools(server, client)

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()

	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: testVersion}, nil)
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}
