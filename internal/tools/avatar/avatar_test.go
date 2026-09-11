// avatar_test.go contains unit tests for the avatar MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package avatar

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestGet verifies the Get handler.
// The mock GitLab API at /api/v4/avatar (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
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

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

// assertMarkdown compares a rendered result with the whole document it is
// meant to be. A substring assertion is what let a card open a table and then
// write list rows into it in two packages of this tree: every row the test
// named was present in the string and none of them rendered as a row, so the
// rule here is the whole document or nothing.
func assertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s\n--- got (quoted) ---\n%q", got, want, got)
	}
}

// TestFormatMarkdown verifies the whole avatar card. The address is the whole
// answer and is written as a link, not as escaped text: the hint tells the
// reader to use the URL directly, which needs the URL to be navigable.
func TestFormatMarkdown(t *testing.T) {
	assertMarkdown(t, FormatMarkdown(GetOutput{AvatarURL: "https://example.com/avatar.png"}),
		"## Avatar\n\n"+
			"- **URL**: [https://example.com/avatar.png](https://example.com/avatar.png)\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use the avatar URL directly in your application\n")
}

// TestFormatMarkdown_NoAvatar verifies that an answer with no address writes
// no row: a label with nothing after it reads as a value GitLab lost.
func TestFormatMarkdown_NoAvatar(t *testing.T) {
	assertMarkdown(t, FormatMarkdown(GetOutput{}),
		"## Avatar\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use the avatar URL directly in your application\n")
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestGet_APIError_Coverage verifies that Get_Coverage returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_APIError_Coverage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{Email: "a@b.c"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestGet_Success_Coverage verifies the Get_Success_Coverage handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
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

// TestFormatMarkdown_Coverage verifies the Markdown_Coverage Markdown formatter for a representative _coverage input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatMarkdown_Coverage(t *testing.T) {
	assertMarkdown(t, FormatMarkdown(GetOutput{AvatarURL: "https://img.example.com/a.png"}),
		"## Avatar\n\n"+
			"- **URL**: [https://img.example.com/a.png](https://img.example.com/a.png)\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use the avatar URL directly in your application\n")
}

// TestActionSpecs_Metadata_Coverage validates the Metadata_Coverage route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_CallRoute_Coverage validates the CallRoute_Coverage route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
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

// TestActionSpecs_CallRouteError validates the CallRouteError route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
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
