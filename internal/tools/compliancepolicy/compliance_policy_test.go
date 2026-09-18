// compliance_policy_test.go contains unit tests for the GitLab group compliance framework policy MCP tool handlers (get, update).
package compliancepolicy

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

// TestGet verifies the Get handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.Handler
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

// TestUpdate verifies the Update handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestUpdate(t *testing.T) {
	nsID := int64(456)
	zeroID := int64(0)

	tests := []struct {
		name       string
		input      UpdateInput
		handler    http.Handler
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
			name:    "rejects nil csp_namespace_id",
			input:   UpdateInput{CSPNamespaceID: nil},
			handler: testutil.ForbiddenHandler(t),
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

// TestUpdate_RejectedNamespace_HintNamesTheTopLevelGroupRule drives both
// refusals GitLab gives a csp_namespace_id it will not accept — 400 for a
// malformed one and 422 for one it understands and declines — and asserts each
// comes back carrying the namespace hint and not the licensing one.
//
// Why it matters: the handler reaches that hint through a two-legged condition,
// and only the 400 leg was ever exercised, so the 422 leg could be deleted
// without a test noticing. A 422 would then fall through to the status-hint
// branch and a caller who named a subgroup, a project, or a namespace GitLab
// has locked since the last update would be told to check the license and the
// Owner role instead — a correct-looking answer pointing at the wrong problem.
// Asserting the absence of the licensing wording is what pins which of the two
// branches produced the message; asserting its presence alone would pass on
// either.
func TestUpdate_RejectedNamespace_HintNamesTheTopLevelGroupRule(t *testing.T) {
	nsID := int64(999)

	// Neither body repeats a word the hint carries, so a match below can only
	// have come from the hint and never from GitLab's message echoed back.
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{
			name:   "bad request",
			status: http.StatusBadRequest,
			body:   `{"error":"400 Bad request"}`,
		},
		{
			name:   "unprocessable entity",
			status: http.StatusUnprocessableEntity,
			body:   `{"message":"namespace is not eligible"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, tt.status, tt.body)
			}))

			_, err := Update(context.Background(), client, UpdateInput{CSPNamespaceID: &nsID})
			if err == nil {
				t.Fatal("expected error for a csp_namespace_id GitLab refuses")
			}
			errText := err.Error()
			for _, want := range []string{"csp_namespace_id", "top-level group", "lock"} {
				if !strings.Contains(errText, want) {
					t.Errorf("error missing %q: %v", want, err)
				}
			}
			if strings.Contains(errText, "Ultimate license") {
				t.Errorf("a refused namespace was reported as a licensing problem: %v", err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FormatOutputMarkdown
// ---------------------------------------------------------------------------

// TestFormatOutputMarkdown validates the Markdown formatter for compliance policy
// settings output, covering both set and unset CSPNamespaceID values. The whole
// render is compared, since a substring assertion cannot see a row that landed
// outside the block it was meant for.
func TestFormatOutputMarkdown(t *testing.T) {
	nsID := int64(42)

	tests := []struct {
		name   string
		output Output
		want   string
	}{
		{
			name:   "formats output with csp_namespace_id set",
			output: Output{CSPNamespaceID: &nsID},
			want: "## Compliance Policy Settings\n\n" +
				"- **CSP Namespace ID**: 42\n\n" +
				"---\n💡 **Next steps:**\n" +
				"- Use action 'compliance_policy.update' to bind the compliance security policy project to a top-level group\n" +
				"- Use action 'group.get' to read the group this namespace ID names\n",
		},
		{
			name:   "formats output with nil csp_namespace_id",
			output: Output{CSPNamespaceID: nil},
			want: "## Compliance Policy Settings\n\n" +
				"- **CSP Namespace ID**: not set\n\n" +
				"---\n💡 **Next steps:**\n" +
				"- Use action 'compliance_policy.update' to bind the compliance security policy project to a top-level group\n" +
				"- Use action 'group.get' to read the group this namespace ID names\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.output); got != tt.want {
				t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_Metadata_EachSpecDescribesItsOwnAction asserts that each of
// the two specs carries the discovery metadata of the action it names rather
// than its sibling's: guidance for exactly the parameters its own input struct
// accepts, and a description and a related-action list that point at the other
// action instead of back at itself.
//
// Why it matters: both specs come out of one options function that branches on
// the action name, and everything that branch decides — usage, aliases,
// parameter guidance, description, related actions — is swapped wholesale if
// the branch inverts. Both specs stay fully populated afterwards, so every
// "is it non-empty" assertion still passes while a model is told the read
// action takes csp_namespace_id and that the update action is for reading.
// Naming the property each side must hold, rather than repeating the strings
// the function currently produces, is what makes the swap visible.
func TestActionSpecs_Metadata_EachSpecDescribesItsOwnAction(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	specByName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByName[spec.Name] = spec
	}

	tests := []struct {
		name string
		// guidedParams are the json names the action's own input struct
		// accepts, so guidance for anything else names an argument a caller
		// cannot send and guidance for none of them leaves the one argument
		// GitLab requires undescribed.
		guidedParams []string
		sibling      string
	}{
		{name: "get", guidedParams: nil, sibling: "update"},
		{name: "update", guidedParams: []string{"csp_namespace_id"}, sibling: "get"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByName[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %q", tt.name)
			}
			sibling, ok := specByName[tt.sibling]
			if !ok {
				t.Fatalf("missing sibling ActionSpec for %q", tt.sibling)
			}

			assertGuidesExactly(t, spec, tt.guidedParams)
			assertPointsAtSibling(t, spec, sibling)
			assertRelatesToSibling(t, spec, tt.name, tt.sibling)
		})
	}
}

// assertGuidesExactly holds a spec's parameter guidance to the parameters its
// own input struct accepts, and to saying something about each of them.
func assertGuidesExactly(t *testing.T, spec toolutil.ActionSpec, want []string) {
	t.Helper()

	if len(spec.ParameterGuidance) != len(want) {
		t.Errorf("ParameterGuidance keys = %v, want exactly %v", guidanceKeys(spec.ParameterGuidance), want)
	}
	for _, param := range want {
		guidance, guided := spec.ParameterGuidance[param]
		if !guided {
			t.Errorf("ParameterGuidance has no entry for %q", param)
			continue
		}
		if guidance.ValueSource == "" || guidance.ExampleBinding == "" {
			t.Errorf("ParameterGuidance[%q] is present but says nothing: %+v", param, guidance)
		}
	}
}

// assertPointsAtSibling holds a spec's individual-tool description to naming
// the other action's tool and never its own, which is the cross-reference a
// model follows when the action it picked is not the one it wanted.
func assertPointsAtSibling(t *testing.T, spec, sibling toolutil.ActionSpec) {
	t.Helper()

	got := spec.IndividualTool.Description
	if !strings.Contains(got, sibling.IndividualTool.Name) {
		t.Errorf("description does not point at the sibling tool %q: %s", sibling.IndividualTool.Name, got)
	}
	if strings.Contains(got, spec.IndividualTool.Name) {
		t.Errorf("description sends a reader back to this same tool %q: %s", spec.IndividualTool.Name, got)
	}
}

// assertRelatesToSibling holds a spec's related actions to naming the other
// action of the pair and never itself.
func assertRelatesToSibling(t *testing.T, spec toolutil.ActionSpec, name, sibling string) {
	t.Helper()

	if want := "compliance_policy." + sibling; !slices.Contains(spec.RelatedActions, want) {
		t.Errorf("RelatedActions = %v, want it to name %q", spec.RelatedActions, want)
	}
	if self := "compliance_policy." + name; slices.Contains(spec.RelatedActions, self) {
		t.Errorf("RelatedActions names the action itself (%q): %v", self, spec.RelatedActions)
	}
}

// guidanceKeys reports a guidance map's keys so a failure can name what was
// found rather than only how many entries there were.
func guidanceKeys(m map[string]toolutil.ParameterGuidance) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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
