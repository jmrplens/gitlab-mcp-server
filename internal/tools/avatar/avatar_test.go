// avatar_test.go contains unit tests for the avatar MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package avatar

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestGet verifies Get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/avatar" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://example.com/avatar.png"}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Email: "test@example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("unexpected avatar URL: %s", out.AvatarURL)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestFormatMarkdown verifies FormatMarkdown.
func TestFormatMarkdown(t *testing.T) {
	md := FormatMarkdown(GetOutput{AvatarURL: "https://example.com/avatar.png"})
	if md == "" {
		t.Error("expected non-empty markdown")
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestGet_APIError_Coverage verifies Get when API error coverage.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: "a@b.c"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies Get when success coverage.
func TestGet_Success_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://img.example.com/a.png"}`)
	}))
	out, err := Get(t.Context(), client, GetInput{Email: "a@b.c", Size: 100})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if out.AvatarURL != "https://img.example.com/a.png" {
		t.Errorf("unexpected URL: %s", out.AvatarURL)
	}
}

// TestFormatMarkdown_Coverage verifies FormatMarkdown when coverage.
func TestFormatMarkdown_Coverage(t *testing.T) {
	md := FormatMarkdown(GetOutput{AvatarURL: "https://img.example.com/a.png"})
	if !strings.Contains(md, "https://img.example.com/a.png") {
		t.Error("expected avatar URL in markdown")
	}
}

// TestActionSpecs_Metadata_Coverage verifies avatar action spec metadata.
func TestActionSpecs_Metadata_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"x"}`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "avatar" || specs[0].IndividualTool.Name != "gitlab_get_avatar" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if !strings.Contains(specs[0].Usage, "known email address") {
		t.Fatalf("Usage = %q, want known email address guidance", specs[0].Usage)
	}
	if !strings.Contains(specs[0].IndividualTool.Description, "Returns:") || !strings.Contains(specs[0].IndividualTool.Description, "See also:") {
		t.Fatalf("Description = %q, want Returns/See also guidance", specs[0].IndividualTool.Description)
	}
	if guidance := specs[0].ParameterGuidance["email"]; guidance.SemanticRole != "email_address" {
		t.Fatalf("email guidance = %+v, want email_address semantic role", guidance)
	}
	if guidance := specs[0].ParameterGuidance["email"]; guidance.ExampleBinding == "" {
		t.Fatalf("email guidance missing ExampleBinding: %+v", guidance)
	}
	if !slices.Contains(specs[0].Aliases, "lookup avatar by email") {
		t.Fatalf("Aliases = %v, want lookup avatar by email", specs[0].Aliases)
	}
}

// TestActionSpecs_CallRoute_Coverage verifies the avatar canonical route.
func TestActionSpecs_CallRoute_Coverage(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"avatar_url":"https://x.com/a.png"}`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	res, err := spec.Route.Handler(t.Context(), map[string]any{"email": "a@b.c", "size": float64(100)})
	if err != nil {
		t.Fatalf("Route.Handler: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
}

// TestActionSpecs_CallRouteError covers the avatar canonical route error path.
func TestActionSpecs_CallRouteError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"email": "a@b.c"}); err == nil {
		t.Fatal("expected route error")
	}
}
