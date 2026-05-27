// action_specs_test.go contains canonical-route tests for Pages actions.
package pages

import (
	"context"
	"net/http"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const pagesActionPagesJSON = `{"url":"https://p.io","is_unique_domain_enabled":true,"force_https":true,"primary_domain":"p.io"}`

const pagesActionDomainJSON = `{"domain":"example.com","auto_ssl_enabled":true,"url":"https://example.com","project_id":42,"verified":true,"verification_code":"abc","certificate":{"subject":"example.com","expired":false}}`

// TestActionSpecs_Metadata verifies canonical metadata for Pages actions.
func TestActionSpecs_Metadata(t *testing.T) {
	byTool := pagesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pagesActionHandler())))
	if len(byTool) != 9 {
		t.Fatalf("unique individual tools = %d, want 9", len(byTool))
	}
	for toolName, spec := range byTool {
		if spec.OwnerPackage != "pages" {
			t.Fatalf("OwnerPackage for %s = %q, want pages", toolName, spec.OwnerPackage)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s should not be empty", toolName)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s should not be empty", toolName)
		}
	}
	if byTool["gitlab_pages_domain_get"].ParameterGuidance["domain"].SemanticRole == "" {
		t.Fatal("gitlab_pages_domain_get should define domain parameter guidance")
	}
	if !byTool["gitlab_pages_unpublish"].Route.Destructive {
		t.Fatal("gitlab_pages_unpublish should be destructive")
	}
}

// TestActionSpecs_CallAllRoutes exercises every Pages tool through its canonical route.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := pagesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pagesActionHandler())))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_pages_get", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_update", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_unpublish", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_domain_list_all", map[string]any{}},
		{"gitlab_pages_domain_list", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_domain_get", map[string]any{argProjectID: "42", argDomain: testDomain}},
		{"gitlab_pages_domain_create", map[string]any{argProjectID: "42", argDomain: "new.com"}},
		{"gitlab_pages_domain_update", map[string]any{argProjectID: "42", argDomain: testDomain}},
		{"gitlab_pages_domain_delete", map[string]any{argProjectID: "42", argDomain: testDomain}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
		})
	}
}

// TestActionSpecs_DeleteErrors verifies destructive routes propagate backend errors.
func TestActionSpecs_DeleteErrors(t *testing.T) {
	byTool := pagesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))))

	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_pages_unpublish", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_domain_delete", map[string]any{argProjectID: "42", argDomain: testDomain}},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected error from delete with failing backend")
			}
		})
	}
}

// TestActionSpecs_DeleteOutputs verifies destructive routes preserve legacy success messages.
func TestActionSpecs_DeleteOutputs(t *testing.T) {
	byTool := pagesSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, pagesActionHandler())))

	for _, tt := range []struct {
		tool    string
		args    map[string]any
		message string
	}{
		{"gitlab_pages_unpublish", map[string]any{argProjectID: "42"}, "Successfully deleted pages."},
		{"gitlab_pages_domain_delete", map[string]any{argProjectID: "42", argDomain: testDomain}, "Successfully deleted pages domain example.com."},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			out, ok := result.(toolutil.DeleteOutput)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want toolutil.DeleteOutput", tt.tool, result)
			}
			if out.Message != tt.message {
				t.Fatalf("delete message = %q, want %q", out.Message, tt.message)
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("should not reach API when confirm is declined")
	}))
	byTool := pagesSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	for _, toolName := range []string{"gitlab_pages_unpublish", "gitlab_pages_domain_delete"} {
		toolutil.RegisterSurfaceToolFromSpec(server, byTool[toolName], toolutil.SurfaceToolRegisterOptions{
			Description: "Test Pages destructive confirmation.",
			Icons:       toolutil.IconFile,
		})
	}

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	for _, tt := range []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_pages_unpublish", map[string]any{argProjectID: "42"}},
		{"gitlab_pages_domain_delete", map[string]any{argProjectID: "42", argDomain: testDomain}},
	} {
		t.Run(tt.tool, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if callErr != nil {
				t.Fatalf("CallTool returned transport error: %v", callErr)
			}
			if result == nil {
				t.Fatal("expected non-nil result when confirmation is declined")
			}
		})
	}
}

func pagesActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesActionPagesJSON)
	})
	handler.HandleFunc("PATCH /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesActionPagesJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/pages", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler.HandleFunc("GET /api/v4/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+pagesActionDomainJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+pagesActionDomainJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesActionDomainJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/pages/domains", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, pagesActionDomainJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, pagesActionDomainJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/pages/domains/example.com", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func pagesSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
