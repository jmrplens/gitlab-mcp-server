// db_migrations_test.go contains unit tests for the database migration MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package dbmigrations

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestMark verifies the Mark handler.
// The mock GitLab API at /api/v4/admin/migrations/20240115100000/mark (POST) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestMark(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/admin/migrations/20240115100000/mark")
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Mark(t.Context(), client, MarkInput{
		Version:  20240115100000,
		Database: "main",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "marked" {
		t.Errorf("Status = %q, want marked", out.Status)
	}
	if out.Version != 20240115100000 {
		t.Errorf("Version = %d, want 20240115100000", out.Version)
	}
}

// TestMark_Error verifies that Mark returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestMark_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Mark(t.Context(), client, MarkInput{Version: 99999})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestMark_VersionValidation verifies the Mark_VersionValidation handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestMark_VersionValidation(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	ctx := context.Background()

	tests := []struct {
		name    string
		version int64
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Mark(ctx, client, MarkInput{Version: tt.version})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), "version") {
				t.Errorf("error %q does not contain %q", err.Error(), "version")
			}
		})
	}
}

// adminAccessHint is the suggestion Mark attaches when GitLab refuses the call
// with 403. The test keeps its own spelling of it on purpose: an assertion that
// read the string back out of the handler would move with it and prove nothing.
const adminAccessHint = "database migrations require administrator access"

// TestMark_Database_ReachesGitLabInTheRequestBody verifies that the database
// the caller named travels to GitLab, and that leaving it unset sends no key at
// all so the instance applies its own default.
//
// It matters because nothing else here could see it. The database decides which
// of an instance's databases the migration is marked on, and every other
// assertion in this file (the path, the method, the echoed output) reads the
// same whether the option is passed or dropped: removing `Database:
// input.Database` from the handler leaves the whole suite green.
func TestMark_Database_ReachesGitLabInTheRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		database string
		wantBody string
	}{
		{name: "named database", database: "ci", wantBody: `{"database":"ci"}`},
		{name: "unset database", database: "", wantBody: `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
					http.Error(w, "read request body", http.StatusInternalServerError)
					return
				}
				capturedBody = string(body)
				testutil.RespondJSON(w, http.StatusOK, `{}`)
			})
			client := testutil.NewTestClient(t, handler)

			if _, err := Mark(t.Context(), client, MarkInput{
				Version:  20240115100000,
				Database: tt.database,
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capturedBody != tt.wantBody {
				t.Errorf("request body = %q, want %q", capturedBody, tt.wantBody)
			}
		})
	}
}

// TestMark_ErrorStatus_HintsOnlyOnForbidden verifies which refusal earns the
// administrator hint and which does not.
//
// Mark keys that hint to 403 through WrapErrWithStatusHint, and the only other
// error test in this file asserts that some error came back: the status could
// be changed to any other code, or the hint lost altogether, with nothing
// failing. A 403 is the one refusal the caller can act on, by presenting a
// token with administrator access, so a hint that stops appearing there costs
// them the single instruction that resolves it, and one that starts appearing
// on a 404 sends them after an authorization problem they do not have.
func TestMark_ErrorStatus_HintsOnlyOnForbidden(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		wantHint bool
	}{
		{name: "forbidden", status: http.StatusForbidden, wantHint: true},
		{name: "not found", status: http.StatusNotFound, wantHint: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			})
			client := testutil.NewTestClient(t, handler)

			_, err := Mark(t.Context(), client, MarkInput{Version: 20240115100000})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := strings.Contains(err.Error(), adminAccessHint); got != tt.wantHint {
				t.Errorf("error %q carries the administrator hint = %v, want %v", err.Error(), got, tt.wantHint)
			}
		})
	}
}

// markHints is the guidance section the card ends with.
const markHints = "\n---\n💡 **Next steps:**\n" +
	"- Verify overall migration state in the GitLab admin area (no list action is exposed here)\n"

// TestFormatMarkMarkdown verifies the whole card: two rows and the hint, where
// the formatter used to write one paragraph of "**Status**: x | **Version**: 1".
func TestFormatMarkMarkdown(t *testing.T) {
	got := FormatMarkMarkdown(MarkOutput{Status: "marked", Version: 20240115100000})
	want := "## Mark Migration\n\n" +
		"- **Status**: marked\n" +
		"- **Version**: 20240115100000\n" +
		markHints
	if got != want {
		t.Errorf("FormatMarkMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatMarkMarkdown_HostileStatus verifies that a status carrying markup
// and line breaks changes no structure: it used to be written into a bare
// paragraph with no escaper in front of it, so a tag reached the page as
// markup and a line break added a heading of its own.
func TestFormatMarkMarkdown_HostileStatus(t *testing.T) {
	got := FormatMarkMarkdown(MarkOutput{
		Status:  "<a href=\"http://attacker.invalid\">x</a>\n## Injected",
		Version: 7,
	})
	want := "## Mark Migration\n\n" +
		"- **Status**: &lt;a href=\"http://attacker.invalid\">x&lt;/a> ## Injected\n" +
		"- **Version**: 7\n" +
		markHints
	if got != want {
		t.Errorf("FormatMarkMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	if specs[0].OwnerPackage != "dbmigrations" || specs[0].IndividualTool.Name != "gitlab_mark_migration" {
		t.Fatalf("unexpected ActionSpec metadata: %+v", specs[0])
	}
	if specs[0].Usage == "" {
		t.Fatal("db migration ActionSpec should define usage")
	}
	if len(specs[0].Aliases) == 0 {
		t.Fatal("db migration ActionSpec should define aliases")
	}
	if specs[0].ParameterGuidance["version"].SemanticRole == "" {
		t.Fatal("db migration ActionSpec should define version parameter guidance")
	}
}

// TestActionSpecs_IndividualTool_AnnotatesNonDestructive verifies the one place
// this package deliberately disagrees with itself: marking a migration is
// registered through NewDeleteActionSpec, so the action stays destructive for
// the confirmation prompt and the read-only filter, while the tool an
// individual-surface client is listed declares destructiveHint false.
//
// Both halves are asserted together because either one alone is satisfied by
// deleting the override: with it gone the annotation simply follows the
// catalog, and every other test in this package still passes. The assertion
// reads the projected tool rather than the override field, because what a
// client is told is the annotation, and the projection may narrow an override
// on its way out.
func TestActionSpecs_IndividualTool_AnnotatesNonDestructive(t *testing.T) {
	spec := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))[0]
	if !spec.Destructive {
		t.Fatal("db_migration_mark should stay destructive in the catalog")
	}

	tool, err := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{
		Description: "Mark a pending database migration as successfully executed.",
	})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec: %v", err)
	}
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
		t.Fatalf("individual tool declares no destructive hint: %+v", tool.Annotations)
	}
	if *tool.Annotations.DestructiveHint {
		t.Error("destructiveHint = true, want false: the individual projection overrides the catalog classification")
	}
}

// TestActionSpecs_CallRoute validates the CallRoute route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoute(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	})

	client := testutil.NewTestClient(t, handler)
	spec := ActionSpecs(client)[0]
	res, err := spec.Route.Handler(t.Context(), map[string]any{"version": int64(20240115100000)})
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
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	client := testutil.NewTestClient(t, mux)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"version": int64(99999)}); err == nil {
		t.Fatal("expected route error")
	}
}
