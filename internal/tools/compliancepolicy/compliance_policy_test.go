package compliancepolicy

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when context is cancelled",
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			ctx: func() context.Context {
				ctx := testutil.CancelledCtx(t)
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
			name:  "rejects nil csp_namespace_id",
			input: UpdateInput{CSPNamespaceID: nil},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Fatal("handler should not be called for nil csp_namespace_id")
			}),
			wantErr: true,
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
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal error"}`)
			}),
			wantErr: true,
		},
		{
			name:    "returns error when context is cancelled",
			input:   UpdateInput{CSPNamespaceID: &nsID},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
			ctx: func() context.Context {
				ctx := testutil.CancelledCtx(t)
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

// TestUpdate_BadRequestHint verifies that invalid CSP namespace IDs return
// actionable guidance about top-level groups and GitLab's update lock.
func TestUpdate_BadRequestHint(t *testing.T) {
	nsID := int64(999)
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"csp_namespace_id is invalid"}`)
	}))

	_, err := Update(context.Background(), client, UpdateInput{CSPNamespaceID: &nsID})
	if err == nil {
		t.Fatal("expected error for invalid csp_namespace_id")
	}
	errText := err.Error()
	for _, want := range []string{"csp_namespace_id", "top-level group", "lock"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
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
// ActionSpec route execution
// ---------------------------------------------------------------------------

// TestActionSpecs_Metadata verifies compliance policy action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "compliancepolicy" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s is empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s are empty", spec.Name)
		}
	}
}

// TestActionSpecs_CallRoutes validates that both canonical routes return valid results.
func TestActionSpecs_CallRoutes(t *testing.T) {
	client := newCompliancePolicyRouteClient(t)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

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
			spec, ok := specByTool[tt.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.tool)
			}
			result, callErr := spec.Route.Handler(t.Context(), tt.args)
			if callErr != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, callErr)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// newCompliancePolicyRouteClient creates a client backed by mock handlers for both compliance policy tools.
func newCompliancePolicyRouteClient(t *testing.T) *gitlabclient.Client {
	t.Helper()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/admin/security/compliance_policy_settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":100}`)
	})
	handler.HandleFunc("PUT /api/v4/admin/security/compliance_policy_settings", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"csp_namespace_id":200}`)
	})

	return testutil.NewTestClient(t, handler)
}
