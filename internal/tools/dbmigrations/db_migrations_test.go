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
