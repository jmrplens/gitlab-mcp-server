// job_token_scope_test.go contains unit tests for the job token scope MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package jobtokenscope

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// Every writing handler here builds a body GitLab reads and nothing answered
// back: a PATCH carrying one flag, and two POSTs each carrying one identifier.
// A mock that only answers cannot tell the flag the caller chose from its
// opposite, nor the target the caller named from a constant, so the mock keeps
// what it was sent and each writing handler is held to it.

// capturedRequest is what the mock was sent: how the handler addressed the
// request and what it carried.
type capturedRequest struct {
	Method string
	Path   string
	Body   []byte
}

// captureRequest answers with response, or with status alone when response is
// empty, and keeps the request it was sent so a test can assert what GitLab
// received rather than what the handler returned.
func captureRequest(t *testing.T, status int, response string, into *capturedRequest) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read request body", http.StatusInternalServerError)
			return
		}
		*into = capturedRequest{Method: r.Method, Path: r.URL.Path, Body: body}
		if response == "" {
			w.WriteHeader(status)
			return
		}
		testutil.RespondJSON(w, status, response)
	})
}

// assertAddressed fails when the captured request did not reach the endpoint
// the action is named for, so a body assertion can never pass on a request
// that went somewhere else.
func assertAddressed(t *testing.T, got capturedRequest, method, path string) {
	t.Helper()
	if got.Method != method {
		t.Errorf("method = %q, want %q", got.Method, method)
	}
	if got.Path != path {
		t.Errorf("path = %q, want %q", got.Path, path)
	}
}

// TestGetAccessSettings_Success reads the settings both ways. The whole answer
// of this action is one boolean, and a fixture that only ever says true cannot
// tell a handler reading GitLab's flag from one returning a constant, which
// would report every project as restricted, including the ones open to any job
// token.
func TestGetAccessSettings_Success(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{name: "restricted to the allowlist", body: `{"inbound_enabled": true}`, want: true},
		{name: "open to any project", body: `{"inbound_enabled": false}`, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, captureRequest(t, http.StatusOK, tc.body, &got))
			out, err := GetAccessSettings(t.Context(), client, GetAccessSettingsInput{ProjectID: "42"})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertAddressed(t, got, http.MethodGet, "/api/v4/projects/42/job_token_scope")
			if out.InboundEnabled != tc.want {
				t.Errorf("InboundEnabled = %t, want %t", out.InboundEnabled, tc.want)
			}
		})
	}
}

// gitLabRefusal is the message GitLab's own body carries in the error tests,
// distinct from anything this package writes so an assertion can tell the two
// apart.
const gitLabRefusal = "job token scope is not available here"

// detailFor is the parenthesised detail a wrapped error carries for a refusal
// at status. Everything but a 404 reaches the wrapper as GitLab's own parsed
// body; a 404 reaches it as client-go's shared ErrNotFound, whose message is
// the status text.
func detailFor(status int) string {
	if status == http.StatusNotFound {
		return "Not Found"
	}
	return "{message: " + gitLabRefusal + "}"
}

// refusingHandler answers every request with status and GitLab's own error
// body.
func refusingHandler(status int) http.Handler {
	body := fmt.Sprintf(`{"message":%q}`, gitLabRefusal)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, body)
	})
}

// TestHandlers_EachHint_IsCarriedOnlyByTheStatusItIsWrittenFor drives every
// handler at the status its hint names and at one other, and replaces eight
// tests that asserted an error had come back and nothing else.
//
// Three properties were unheld, and each is a straight-line literal no gate
// reads. The status: every hint here is attached by WrapErrWithStatusHint for
// one code, and two handlers could have exchanged 403 for 404 with no test
// noticing. The hint text itself, which is what a model is told to do next.
// And the operation label the wrapped error opens with, which is what names
// the failing action in a log. Six of the replaced tests also sent
// `{"message":msgServerError}`, which is not JSON, so GitLab's message was
// never parsed and no assertion about it could have held.
//
// The hint leg asserts the composed `(<detail>). Suggestion: <hint>` rather
// than the hint alone: the parenthesised group is emitted only when a detail
// was extracted, so one substring holds both halves. The other leg asserts no
// suggestion at all, which is what pins the code.
//
// What that detail is depends on the status, and the reason is upstream:
// client-go's CheckResponse answers every 404 with one shared ErrNotFound
// sentinel before it reads the body, so GitLab's own message survives on the
// 403 and 400 legs and is replaced by "Not Found" on the 404 ones. Spelling it
// out here rather than asserting around it keeps the test honest about what a
// caller really sees when a project id is wrong.
func TestHandlers_EachHint_IsCarriedOnlyByTheStatusItIsWrittenFor(t *testing.T) {
	const otherStatus = http.StatusBadRequest

	cases := []struct {
		name      string
		operation string
		status    int
		hint      string
		call      func(context.Context, *gitlabclient.Client) error
	}{
		{
			name:      "get_access_settings",
			operation: "get_job_token_access_settings",
			status:    http.StatusNotFound,
			hint:      "verify project_id with project.get; CI/CD job token settings are at project level",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := GetAccessSettings(ctx, c, GetAccessSettingsInput{ProjectID: "42"})
				return err
			},
		},
		{
			name:      "patch_access_settings",
			operation: "patch_job_token_access_settings",
			status:    http.StatusForbidden,
			hint:      "updating job token access settings requires Maintainer role; verify project_id",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := PatchAccessSettings(ctx, c, PatchAccessSettingsInput{ProjectID: "42", Enabled: true})
				return err
			},
		},
		{
			name:      "list_inbound_allowlist",
			operation: "list_job_token_inbound_allowlist",
			status:    http.StatusNotFound,
			hint:      "verify project_id; allowlist may be empty if inbound scope is disabled",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListInboundAllowlist(ctx, c, ListInboundAllowlistInput{ProjectID: "42"})
				return err
			},
		},
		{
			name:      "add_project_allowlist",
			operation: "add_project_job_token_allowlist",
			status:    http.StatusForbidden,
			hint:      "adding to inbound allowlist requires Maintainer role on source project; verify target_project_id exists and is accessible",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := AddProjectAllowlist(ctx, c, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
				return err
			},
		},
		{
			name:      "remove_project_allowlist",
			operation: "remove_project_job_token_allowlist",
			status:    http.StatusNotFound,
			hint:      "verify target_project_id is on the allowlist with job.token_scope_list_inbound; requires Maintainer role",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				return RemoveProjectAllowlist(ctx, c, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
			},
		},
		{
			name:      "list_group_allowlist",
			operation: "list_job_token_group_allowlist",
			status:    http.StatusNotFound,
			hint:      "verify project_id; group allowlist requires GitLab 17.0+",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := ListGroupAllowlist(ctx, c, ListGroupAllowlistInput{ProjectID: "42"})
				return err
			},
		},
		{
			name:      "add_group_allowlist",
			operation: "add_group_job_token_allowlist",
			status:    http.StatusForbidden,
			hint:      "adding to group allowlist requires Maintainer role on project; verify target_group_id exists; requires GitLab 17.0+",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				_, err := AddGroupAllowlist(ctx, c, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
				return err
			},
		},
		{
			name:      "remove_group_allowlist",
			operation: "remove_group_job_token_allowlist",
			status:    http.StatusNotFound,
			hint:      "verify target_group_id is on the allowlist with job.token_scope_list_groups; requires Maintainer role",
			call: func(ctx context.Context, c *gitlabclient.Client) error {
				return RemoveGroupAllowlist(ctx, c, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hinted := tc.call(t.Context(), testutil.NewTestClient(t, refusingHandler(tc.status)))
			if hinted == nil {
				t.Fatalf("expected an error from a %d answer", tc.status)
			}
			wantHinted := "(" + detailFor(tc.status) + "). Suggestion: " + tc.hint
			if !strings.Contains(hinted.Error(), wantHinted) {
				t.Errorf("error at %d = %q,\nwant it to carry %q", tc.status, hinted, wantHinted)
			}
			if !strings.HasPrefix(hinted.Error(), tc.operation+": ") {
				t.Errorf("error at %d = %q, want it to open with the operation %q", tc.status, hinted, tc.operation)
			}

			plain := tc.call(t.Context(), testutil.NewTestClient(t, refusingHandler(otherStatus)))
			if plain == nil {
				t.Fatalf("expected an error from a %d answer", otherStatus)
			}
			if !strings.Contains(plain.Error(), "("+detailFor(otherStatus)+")") {
				t.Errorf("error at %d = %q, want GitLab's own message in it", otherStatus, plain)
			}
			if strings.Contains(plain.Error(), "Suggestion: ") {
				t.Errorf("error at %d = %q, want no suggestion: this hint is written for %d alone", otherStatus, plain, tc.status)
			}
		})
	}
}

// TestPatchAccessSettings_TheFlagTheCallerChose_IsWhatGitLabReceives drives
// the handler both ways and reads the body back. This is the one action here
// whose whole effect is a single boolean, and nothing observed it: the mock
// answered 204 whatever arrived, so a handler sending the opposite of what the
// caller asked would have turned the inbound restriction on for someone
// turning it off, and every assertion would still have passed. The body is
// compared as text rather than decoded, because a decoded false cannot be told
// from a field that was never sent.
func TestPatchAccessSettings_TheFlagTheCallerChose_IsWhatGitLabReceives(t *testing.T) {
	cases := []struct {
		name     string
		enabled  bool
		wantBody string
	}{
		{name: "enabling the inbound restriction", enabled: true, wantBody: `{"enabled":true}`},
		{name: "disabling the inbound restriction", enabled: false, wantBody: `{"enabled":false}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got capturedRequest
			client := testutil.NewTestClient(t, captureRequest(t, http.StatusNoContent, "", &got))

			out, err := PatchAccessSettings(t.Context(), client, PatchAccessSettingsInput{ProjectID: "42", Enabled: tc.enabled})
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertAddressed(t, got, http.MethodPatch, "/api/v4/projects/42/job_token_scope")
			if body := strings.TrimSpace(string(got.Body)); body != tc.wantBody {
				t.Errorf("request body = %s, want %s", body, tc.wantBody)
			}
			if out.Status != "updated" {
				t.Errorf("Status = %q, want %q", out.Status, "updated")
			}
		})
	}
}

// TestListInboundAllowlist_EachProjectField_ComesFromItsOwnSourceField holds
// the whole row rather than its id and name, because the two string fields
// left unasserted could trade places without a test noticing: a path and a web
// URL are both strings, and the card built from the row prints the path in a
// column of its own. Every value in the fixture is distinct, so no assignment
// is indistinguishable from its neighbor.
func TestListInboundAllowlist_EachProjectField_ComesFromItsOwnSourceField(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusOK, `[
		{"id": 10, "name": "project-a", "path_with_namespace": "group/project-a", "web_url": "https://gitlab.example.com/group/project-a"},
		{"id": 11, "name": "project-b", "path_with_namespace": "other/project-b", "web_url": "https://gitlab.example.com/other/project-b"}
	]`, &got))

	out, err := ListInboundAllowlist(t.Context(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodGet, "/api/v4/projects/42/job_token_scope/allowlist")

	want := []AllowlistProjectItem{
		{ID: 10, Name: "project-a", PathWithNamespace: "group/project-a", WebURL: "https://gitlab.example.com/group/project-a"},
		{ID: 11, Name: "project-b", PathWithNamespace: "other/project-b", WebURL: "https://gitlab.example.com/other/project-b"},
	}
	if !slices.Equal(out.Projects, want) {
		t.Errorf("Projects =\n %+v\nwant\n %+v", out.Projects, want)
	}
}

// TestAddProjectAllowlist_TheTargetTheCallerNamed_IsWhatGitLabReceives reads
// the posted body back as text, so the assertion names the field GitLab reads.
// The output was already held to both of its identifiers; what nothing held
// was the request, and a handler posting a constant in place of the caller's
// target would grant inbound access to the wrong project while answering with
// the entry the mock invented.
//
// As text rather than decoded into the option struct the handler encoded,
// which is what this test did first: decoding names the Go field and not the
// key on the wire, so the assertion moved with the json tag and could never
// have told the wire key GitLab reads from any other spelling of it.
func TestAddProjectAllowlist_TheTargetTheCallerNamed_IsWhatGitLabReceives(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusCreated,
		`{"source_project_id": 42, "target_project_id": 99}`, &got))

	out, err := AddProjectAllowlist(t.Context(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodPost, "/api/v4/projects/42/job_token_scope/allowlist")

	wantBody := `{"target_project_id":99}`
	if body := strings.TrimSpace(string(got.Body)); body != wantBody {
		t.Errorf("request body = %s, want %s", body, wantBody)
	}
	if out.SourceProjectID != 42 || out.TargetProjectID != 99 {
		t.Errorf("output = {source %d, target %d}, want {source 42, target 99}", out.SourceProjectID, out.TargetProjectID)
	}
}

// TestRemoveProjectAllowlist_Success checks that the delete reaches the entry
// the caller named. The two identifiers this action carries are both in the
// path and nowhere else, so the path is the only place a test can see which
// project was removed from whose allowlist.
func TestRemoveProjectAllowlist_Success(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusNoContent, "", &got))
	if err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodDelete, "/api/v4/projects/42/job_token_scope/allowlist/99")
}

// TestListGroupAllowlist_EachGroupField_ComesFromItsOwnSourceField holds the
// whole row for the reason its project sibling does, and for one more: a
// top-level group's name and full path are the same string, so the fixture
// this replaced gave two of the three assignments the same value and could not
// have told them apart even had it asserted them. The groups here are nested,
// so name, path and URL all differ.
func TestListGroupAllowlist_EachGroupField_ComesFromItsOwnSourceField(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusOK, `[
		{"id": 5, "name": "my-group", "full_path": "parent/my-group", "web_url": "https://gitlab.example.com/groups/parent/my-group"},
		{"id": 6, "name": "other-group", "full_path": "parent/other-group", "web_url": "https://gitlab.example.com/groups/parent/other-group"}
	]`, &got))

	out, err := ListGroupAllowlist(t.Context(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodGet, "/api/v4/projects/42/job_token_scope/groups_allowlist")

	want := []AllowlistGroupItem{
		{ID: 5, Name: "my-group", FullPath: "parent/my-group", WebURL: "https://gitlab.example.com/groups/parent/my-group"},
		{ID: 6, Name: "other-group", FullPath: "parent/other-group", WebURL: "https://gitlab.example.com/groups/parent/other-group"},
	}
	if !slices.Equal(out.Groups, want) {
		t.Errorf("Groups =\n %+v\nwant\n %+v", out.Groups, want)
	}
}

// TestAddGroupAllowlist_TheTargetTheCallerNamed_IsWhatGitLabReceives is the
// group half of the same demand, read back as text for the same reason, and
// holds the answered entry's own two identifiers as well: the source project
// was unasserted, so the pair could have been exchanged on the way out and
// only the target was ever read.
func TestAddGroupAllowlist_TheTargetTheCallerNamed_IsWhatGitLabReceives(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusCreated,
		`{"source_project_id": 42, "target_group_id": 5}`, &got))

	out, err := AddGroupAllowlist(t.Context(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodPost, "/api/v4/projects/42/job_token_scope/groups_allowlist")

	wantBody := `{"target_group_id":5}`
	if body := strings.TrimSpace(string(got.Body)); body != wantBody {
		t.Errorf("request body = %s, want %s", body, wantBody)
	}
	if out.SourceProjectID != 42 || out.TargetGroupID != 5 {
		t.Errorf("output = {source %d, group %d}, want {source 42, group 5}", out.SourceProjectID, out.TargetGroupID)
	}
}

// TestRemoveGroupAllowlist_Success is the group half of the same demand: the
// project and the group it revokes access for are both segments of the path.
func TestRemoveGroupAllowlist_Success(t *testing.T) {
	var got capturedRequest
	client := testutil.NewTestClient(t, captureRequest(t, http.StatusNoContent, "", &got))
	if err := RemoveGroupAllowlist(t.Context(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertAddressed(t, got, http.MethodDelete, "/api/v4/projects/42/job_token_scope/groups_allowlist/5")
}

// assertRequiredInt64 holds the whole refusal a zero identifier produces to
// the operation and the parameter it belongs to.
//
// Both are arguments at the guard rather than branches of it, and the four
// guards here are the same two lines with two strings changed, so a guard
// naming its sibling's operation and its sibling's parameter refuses exactly
// as loudly and tells a model to correct a parameter the tool it called does
// not have. The expected sentence is composed here rather than taken from
// toolutil.ErrRequiredInt64, so that a value read from the guard cannot
// satisfy the assertion by supplying its own expectation.
func assertRequiredInt64(t *testing.T, err error, operation, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected a refusal for a zero %s, got nil", field)
	}
	want := operation + ": " + field + " is required (must be > 0). " +
		"Ensure you use the exact parameter name '" + field + "' as documented in the tool description"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// TestAddProjectAllowlist_ZeroTargetProjectID holds the guard's refusal to the
// operation and parameter of the project-add half.
func TestAddProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := AddProjectAllowlist(t.Context(), client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	assertRequiredInt64(t, err, "add_project_job_token_allowlist", "target_project_id")
}

// TestRemoveProjectAllowlist_ZeroTargetProjectID holds the guard's refusal to
// the operation and parameter of the project-remove half.
func TestRemoveProjectAllowlist_ZeroTargetProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := RemoveProjectAllowlist(t.Context(), client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 0})
	assertRequiredInt64(t, err, "remove_project_job_token_allowlist", "target_project_id")
}

// TestAddGroupAllowlist_ZeroTargetGroupID holds the guard's refusal to the
// operation and parameter of the group-add half.
func TestAddGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := AddGroupAllowlist(t.Context(), client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	assertRequiredInt64(t, err, "add_group_job_token_allowlist", "target_group_id")
}

// TestRemoveGroupAllowlist_ZeroTargetGroupID holds the guard's refusal to the
// operation and parameter of the group-remove half.
func TestRemoveGroupAllowlist_ZeroTargetGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := RemoveGroupAllowlist(t.Context(), client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 0})
	assertRequiredInt64(t, err, "remove_group_job_token_allowlist", "target_group_id")
}

// markdownText reads the one text block a formatter's result carries.
func markdownText(t *testing.T, r *mcp.CallToolResult) string {
	t.Helper()
	if r == nil {
		t.Fatal(errExpNonNilResult)
	}
	content, ok := r.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("result content is %T, want *mcp.TextContent", r.Content[0])
	}
	return content.Text
}

// TestFormatAccessSettingsMarkdown checks the whole card of an enforced job
// token scope.
func TestFormatAccessSettingsMarkdown(t *testing.T) {
	want := "## Job Token Access Settings\n\n" +
		"- **Inbound job token access**: limited to the allowlist\n" +
		accessSettingsHints
	if got := markdownText(t, FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: true})); got != want {
		t.Errorf("FormatAccessSettingsMarkdown(enabled)\n got %q\nwant %q", got, want)
	}
}

// accessSettingsHints is the guidance section the settings card closes with.
const accessSettingsHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'job.token_scope_list_inbound' to see the projects the allowlist holds\n" +
	"- Use action 'job.token_scope_patch' to turn the restriction on or off\n"

// TestFormatListInboundAllowlistMarkdown_Empty checks that an empty allowlist
// is the one sentence and nothing else.
func TestFormatListInboundAllowlistMarkdown_Empty(t *testing.T) {
	const want = "No projects on the job token inbound allowlist found.\n"
	if got := markdownText(t, FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{})); got != want {
		t.Errorf("FormatListInboundAllowlistMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// TestFormatListGroupAllowlistMarkdown_Empty checks that an empty group
// allowlist is the one sentence and nothing else.
func TestFormatListGroupAllowlistMarkdown_Empty(t *testing.T) {
	const want = "No groups on the job token allowlist found.\n"
	if got := markdownText(t, FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{})); got != want {
		t.Errorf("FormatListGroupAllowlistMarkdown(empty)\n got %q\nwant %q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// errExpNonNilResult identifies the err exp non nil result constant used by this package.
const errExpNonNilResult = "expected non-nil result"

// errExpCancelledCtx identifies the err exp cancelled ctx constant used by this package.
const errExpCancelledCtx = "expected error for canceled context"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// GetAccessSettings — canceled context
// ---------------------------------------------------------------------------.

// TestGetAccessSettings_CancelledContext verifies GetAccessSettings when cancelled context.
func TestGetAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := GetAccessSettings(ctx, client, GetAccessSettingsInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// PatchAccessSettings: canceled context
// ---------------------------------------------------------------------------.

// TestPatchAccessSettings_CancelledContext verifies PatchAccessSettings when cancelled context.
func TestPatchAccessSettings_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := PatchAccessSettings(ctx, client, PatchAccessSettingsInput{ProjectID: "42", Enabled: false})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListInboundAllowlist: canceled context, pagination
// ---------------------------------------------------------------------------.

// TestListInboundAllowlist_CancelledContext verifies ListInboundAllowlist when cancelled context.
func TestListInboundAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListInboundAllowlist(ctx, client, ListInboundAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListInboundAllowlist_WithPagination verifies ListInboundAllowlist when with pagination.
func TestListInboundAllowlist_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id": 10, "name": "proj-a", "path_with_namespace": "grp/proj-a", "web_url": "https://gitlab.example.com/grp/proj-a"},
			{"id": 11, "name": "proj-b", "path_with_namespace": "grp/proj-b", "web_url": "https://gitlab.example.com/grp/proj-b"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "5", TotalPages: "3", NextPage: "2"})
	}))
	out, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{
		ProjectID: "42",
		Page:      1, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(out.Projects))
	}
	if out.Pagination.TotalPages != 3 {
		t.Errorf("TotalPages = %d, want 3", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestListInboundAllowlist_KeysetOrdering verifies ListInboundAllowlist forwards
// the keyset pagination cursor and order_by/sort query parameters to the API.
func TestListInboundAllowlist_KeysetOrdering(t *testing.T) {
	var gotQuery url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{
		ProjectID:  "42",
		OrderBy:    "id",
		Sort:       "desc",
		Pagination: "keyset", PageToken: "tok-1",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := gotQuery.Get("order_by"); got != "id" {
		t.Errorf("order_by = %q, want id", got)
	}
	if got := gotQuery.Get("sort"); got != "desc" {
		t.Errorf("sort = %q, want desc", got)
	}
	if got := gotQuery.Get("pagination"); got != "keyset" {
		t.Errorf("pagination = %q, want keyset", got)
	}
	if got := gotQuery.Get("page_token"); got != "tok-1" {
		t.Errorf("page_token = %q, want tok-1", got)
	}
}

// TestListInboundAllowlist_Empty checks that an allowlist with nothing on it
// is published as an empty array rather than as null. A length of zero cannot
// tell those apart, and the difference reaches the caller: a model reading
// "projects": null has to decide whether the field failed or the list is
// empty, while [] says only the second.
func TestListInboundAllowlist_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListInboundAllowlist(context.Background(), client, ListInboundAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Projects == nil {
		t.Error("Projects is nil, want an empty slice so the field publishes [] rather than null")
	}
	if len(out.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(out.Projects))
	}
}

// ---------------------------------------------------------------------------
// AddProjectAllowlist: canceled context
// ---------------------------------------------------------------------------.

// TestAddProjectAllowlist_CancelledContext verifies AddProjectAllowlist when cancelled context.
func TestAddProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddProjectAllowlist(ctx, client, AddProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveProjectAllowlist — canceled context
// ---------------------------------------------------------------------------.

// TestRemoveProjectAllowlist_CancelledContext verifies RemoveProjectAllowlist when cancelled context.
func TestRemoveProjectAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := RemoveProjectAllowlist(ctx, client, RemoveProjectAllowlistInput{ProjectID: "42", TargetProjectID: 99})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// ListGroupAllowlist: canceled context, pagination, empty
// ---------------------------------------------------------------------------.

// TestListGroupAllowlist_CancelledContext verifies ListGroupAllowlist when cancelled context.
func TestListGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := ListGroupAllowlist(ctx, client, ListGroupAllowlistInput{ProjectID: "42"})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// TestListGroupAllowlist_WithPagination verifies ListGroupAllowlist when with pagination.
func TestListGroupAllowlist_WithPagination(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSONWithPagination(w, http.StatusOK, `[
			{"id": 5, "name": "group-a", "full_path": "group-a", "web_url": "https://gitlab.example.com/groups/group-a"},
			{"id": 6, "name": "group-b", "full_path": "group-b", "web_url": "https://gitlab.example.com/groups/group-b"}
		]`, testutil.PaginationHeaders{Page: "1", PerPage: "2", Total: "4", TotalPages: "2", NextPage: "2"})
	}))
	out, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{
		ProjectID: "42",
		Page:      1, PerPage: 2,
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(out.Groups))
	}
	if out.Pagination.TotalPages != 2 {
		t.Errorf("TotalPages = %d, want 2", out.Pagination.TotalPages)
	}
	if out.Pagination.NextPage != 2 {
		t.Errorf("NextPage = %d, want 2", out.Pagination.NextPage)
	}
}

// TestListGroupAllowlist_KeysetOrdering verifies ListGroupAllowlist forwards the
// keyset pagination cursor and order_by/sort query parameters to the API.
func TestListGroupAllowlist_KeysetOrdering(t *testing.T) {
	var gotQuery url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	_, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{
		ProjectID:  "42",
		OrderBy:    "id",
		Sort:       "asc",
		Pagination: "keyset", PageToken: "tok-2",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := gotQuery.Get("order_by"); got != "id" {
		t.Errorf("order_by = %q, want id", got)
	}
	if got := gotQuery.Get("sort"); got != "asc" {
		t.Errorf("sort = %q, want asc", got)
	}
	if got := gotQuery.Get("pagination"); got != "keyset" {
		t.Errorf("pagination = %q, want keyset", got)
	}
	if got := gotQuery.Get("page_token"); got != "tok-2" {
		t.Errorf("page_token = %q, want tok-2", got)
	}
}

// TestListGroupAllowlist_Empty holds the group allowlist to the same published
// shape its project sibling is held to: an empty array, never null.
func TestListGroupAllowlist_Empty(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	out, err := ListGroupAllowlist(context.Background(), client, ListGroupAllowlistInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Groups == nil {
		t.Error("Groups is nil, want an empty slice so the field publishes [] rather than null")
	}
	if len(out.Groups) != 0 {
		t.Errorf("expected 0 groups, got %d", len(out.Groups))
	}
}

// ---------------------------------------------------------------------------
// AddGroupAllowlist: canceled context
// ---------------------------------------------------------------------------.

// TestAddGroupAllowlist_CancelledContext verifies AddGroupAllowlist when cancelled context.
func TestAddGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	_, err := AddGroupAllowlist(ctx, client, AddGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// RemoveGroupAllowlist: canceled context
// ---------------------------------------------------------------------------.

// TestRemoveGroupAllowlist_CancelledContext verifies RemoveGroupAllowlist when cancelled context.
func TestRemoveGroupAllowlist_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	ctx := testutil.CancelledCtx(t)
	err := RemoveGroupAllowlist(ctx, client, RemoveGroupAllowlistInput{ProjectID: "42", TargetGroupID: 5})
	if err == nil {
		t.Fatal(errExpCancelledCtx)
	}
}

// ---------------------------------------------------------------------------
// FormatAccessSettingsMarkdown — disabled state
// ---------------------------------------------------------------------------.

// TestFormatAccessSettingsMarkdown_Disabled checks the card of a project with
// the job token scope off. The row names the restriction rather than the
// switch: "Inbound access: disabled" read as though access itself were denied,
// and it is the opposite — every project's job token may reach this one.
func TestFormatAccessSettingsMarkdown_Disabled(t *testing.T) {
	want := "## Job Token Access Settings\n\n" +
		"- **Inbound job token access**: not limited (any project's job token may access this project)\n" +
		accessSettingsHints
	if got := markdownText(t, FormatAccessSettingsMarkdown(AccessSettingsOutput{InboundEnabled: false})); got != want {
		t.Errorf("FormatAccessSettingsMarkdown(disabled)\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListInboundAllowlistMarkdown — with data
// ---------------------------------------------------------------------------.

// TestFormatListInboundAllowlistMarkdown_WithData verifies FormatListInboundAllowlistMarkdown when with data.
func TestFormatListInboundAllowlistMarkdown_WithData(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{
		Projects: []AllowlistProjectItem{
			{ID: 10, Name: "proj-a", PathWithNamespace: "grp/proj-a", WebURL: "https://gitlab.example.com/grp/proj-a"},
			{ID: 11, Name: "proj-b", PathWithNamespace: "grp/proj-b", WebURL: "https://gitlab.example.com/grp/proj-b"},
		},
	})
	// The name carries the link, so a reader is never asked to click a column
	// that says "View" and nothing about where it goes.
	want := "## Job Token Inbound Allowlist (2)\n\n" +
		"| ID | Name | Path |\n| --- | --- | --- |\n" +
		"| 10 | [proj-a](https://gitlab.example.com/grp/proj-a) | grp/proj-a |\n" +
		"| 11 | [proj-b](https://gitlab.example.com/grp/proj-b) | grp/proj-b |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'job.token_scope_add_project' to allow another project\n" +
		"- Use action 'job.token_scope_remove_project' to remove one from the allowlist\n"
	if got := markdownText(t, r); got != want {
		t.Errorf("FormatListInboundAllowlistMarkdown()\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatAddProjectAllowlistMarkdown
// ---------------------------------------------------------------------------.

// TestFormatAddProjectAllowlistMarkdown checks the whole card of the entry
// that was created: a create result returns the object, so it is a card.
func TestFormatAddProjectAllowlistMarkdown(t *testing.T) {
	r := FormatAddProjectAllowlistMarkdown(InboundAllowItemOutput{SourceProjectID: 42, TargetProjectID: 99})
	want := "## Job Token Inbound Allowlist Entry\n\n" +
		"- **Project**: 42\n" +
		"- **Allowed project**: 99\n" +
		"\nThe allowed project's job token may now reach this project.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.token_scope_list_inbound' to see the whole allowlist\n" +
		"- Use action 'job.token_scope_get' to check whether the restriction is enforced at all\n"
	if got := markdownText(t, r); got != want {
		t.Errorf("FormatAddProjectAllowlistMarkdown()\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListGroupAllowlistMarkdown — with data
// ---------------------------------------------------------------------------.

// TestFormatListGroupAllowlistMarkdown_WithData verifies FormatListGroupAllowlistMarkdown when with data.
func TestFormatListGroupAllowlistMarkdown_WithData(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{
		Groups: []AllowlistGroupItem{
			{ID: 5, Name: "group-a", FullPath: "group-a", WebURL: "https://gitlab.example.com/groups/group-a"},
			{ID: 6, Name: "group-b", FullPath: "org/group-b", WebURL: "https://gitlab.example.com/groups/org/group-b"},
		},
	})
	want := "## Job Token Group Allowlist (2)\n\n" +
		"| ID | Name | Path |\n| --- | --- | --- |\n" +
		"| 5 | [group-a](https://gitlab.example.com/groups/group-a) | group-a |\n" +
		"| 6 | [group-b](https://gitlab.example.com/groups/org/group-b) | org/group-b |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- When presenting these results, always include the clickable [text](url) links from the table so the user can navigate to GitLab\n" +
		"- Use action 'job.token_scope_add_group' to allow another group\n" +
		"- Use action 'job.token_scope_remove_group' to remove one from the allowlist\n"
	if got := markdownText(t, r); got != want {
		t.Errorf("FormatListGroupAllowlistMarkdown()\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatAddGroupAllowlistMarkdown
// ---------------------------------------------------------------------------.

// TestFormatAddGroupAllowlistMarkdown checks the whole card of the group entry
// that was created.
func TestFormatAddGroupAllowlistMarkdown(t *testing.T) {
	r := FormatAddGroupAllowlistMarkdown(GroupAllowlistItemOutput{SourceProjectID: 42, TargetGroupID: 5})
	want := "## Job Token Group Allowlist Entry\n\n" +
		"- **Project**: 42\n" +
		"- **Allowed group**: 5\n" +
		"\nEvery project in the allowed group may now reach this project with its job token.\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'job.token_scope_list_groups' to see the whole group allowlist\n" +
		"- Use action 'job.token_scope_get' to check whether the restriction is enforced at all\n"
	if got := markdownText(t, r); got != want {
		t.Errorf("FormatAddGroupAllowlistMarkdown()\n got %q\nwant %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListInboundAllowlistMarkdown — with pipe character in name
// ---------------------------------------------------------------------------.

// TestFormatListInboundAllowlistMarkdown_EscapesPipes verifies FormatListInboundAllowlistMarkdown when escapes pipes.
func TestFormatListInboundAllowlistMarkdown_EscapesPipes(t *testing.T) {
	r := FormatListInboundAllowlistMarkdown(ListInboundAllowlistOutput{
		Projects: []AllowlistProjectItem{
			{ID: 10, Name: "proj|special", PathWithNamespace: "grp/proj-special", WebURL: "https://gitlab.example.com/grp/proj-special"},
		},
	})
	const wantRow = "| 10 | [proj&#124;special](https://gitlab.example.com/grp/proj-special) | grp/proj-special |\n"
	if got := markdownText(t, r); !strings.Contains(got, wantRow) {
		t.Errorf("FormatListInboundAllowlistMarkdown() missing %q:\n%s", wantRow, got)
	}
}

// ---------------------------------------------------------------------------
// FormatListGroupAllowlistMarkdown — with pipe character in name
// ---------------------------------------------------------------------------.

// TestFormatListGroupAllowlistMarkdown_EscapesPipes verifies FormatListGroupAllowlistMarkdown when escapes pipes.
func TestFormatListGroupAllowlistMarkdown_EscapesPipes(t *testing.T) {
	r := FormatListGroupAllowlistMarkdown(ListGroupAllowlistOutput{
		Groups: []AllowlistGroupItem{
			{ID: 5, Name: "group|special", FullPath: "group-special", WebURL: "https://gitlab.example.com/groups/group-special"},
		},
	})
	const wantRow = "| 5 | [group&#124;special](https://gitlab.example.com/groups/group-special) | group-special |\n"
	if got := markdownText(t, r); !strings.Contains(got, wantRow) {
		t.Errorf("FormatListGroupAllowlistMarkdown() missing %q:\n%s", wantRow, got)
	}
}
