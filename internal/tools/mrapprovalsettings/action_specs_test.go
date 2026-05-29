// action_specs_test.go contains integration tests for the MR approval settings
// tool closures in ActionSpecs routes with mock GitLab API responses.
package mrapprovalsettings

import (
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const approvalSettingsJSON = `{
	"allow_author_approval":{"value":true,"locked":false,"inherited_from":""},
	"allow_committer_approval":{"value":true,"locked":false,"inherited_from":""},
	"allow_overrides_to_approver_list_per_merge_request":{"value":false,"locked":false,"inherited_from":""},
	"retain_approvals_on_push":{"value":true,"locked":false,"inherited_from":""},
	"selective_code_owner_removals":{"value":false,"locked":false,"inherited_from":""},
	"require_password_to_approve":{"value":false,"locked":false,"inherited_from":""},
	"require_reauthentication_to_approve":{"value":false,"locked":false,"inherited_from":""}
}`

// TestActionSpecs_Metadata verifies MR approval settings action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 4 {
		t.Fatalf("len(ActionSpecs) = %d, want 4", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "mrapprovalsettings" || spec.IndividualTool.Name == "" {
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

// TestActionSpecs_CallRoutes verifies all registered MR approval settings routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			testutil.RespondJSON(w, http.StatusOK, approvalSettingsJSON)
		case http.MethodPut:
			testutil.RespondJSON(w, http.StatusOK, approvalSettingsJSON)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_get_group_mr_approval_settings", map[string]any{"group_id": "42"}},
		{"gitlab_update_group_mr_approval_settings", map[string]any{"group_id": "42"}},
		{"gitlab_get_project_mr_approval_settings", map[string]any{"project_id": "42"}},
		{"gitlab_update_project_mr_approval_settings", map[string]any{"project_id": "42"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestFormatOutputMarkdown_EmptyScope verifies that FormatOutputMarkdown handles
// an empty scope string correctly, covering the init() function's registered formatter.
func TestFormatOutputMarkdown_EmptyScope(t *testing.T) {
	out := Output{
		AllowAuthorApproval:              SettingOutput{Value: true, Locked: false},
		AllowCommitterApproval:           SettingOutput{Value: false, Locked: true, InheritedFrom: "group"},
		RequireReauthenticationToApprove: SettingOutput{Value: true, Locked: false},
	}
	md := FormatOutputMarkdown(out, "")
	if md == "" {
		t.Fatal("expected non-empty markdown output for empty scope")
	}
}

// TestMarkdownRegistry_OutputType verifies that the init() registered formatter
// is callable through MarkdownForResult, covering the closure in init().
func TestMarkdownRegistry_OutputType(t *testing.T) {
	out := Output{
		AllowAuthorApproval: SettingOutput{Value: true, Locked: false},
	}
	result := toolutil.MarkdownForResult(out)
	if result == nil {
		t.Fatal("expected MarkdownForResult to return non-nil for Output type")
	}
}
