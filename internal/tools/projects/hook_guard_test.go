// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hook_guard_test.go holds the three calls that change a project webhook to
// the confirmation they owe: none for a call that neither redirects the hook
// nor replaces a secret, and one, before anything is written, for a call
// that does.
package projects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// storedHook is a webhook with an address, one URL variable and one custom
// header beside an entry GitLab sent as null, so each way of redirecting it
// or replacing a secret has something to change.
const storedHook = `{"id":7,"url":"https://old.example/hooks/abc","project_id":42,` +
	`"url_variables":[{"key":"tenant"}],"custom_headers":[null,{"key":"Authorization"}]}`

// hookCalls counts what the fake GitLab was sent for the hook, by kind.
type hookCalls struct {
	reads  atomic.Int64
	writes atomic.Int64
}

// hookClient answers every route under the project's hook 7 and counts what
// it was sent: GET with readStatus and readBody, the hook the guard compares
// the call with, and any other method with the hook as written.
func hookClient(t *testing.T, calls *hookCalls, readStatus int, readBody string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v4/projects/42/hooks/7") {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			calls.reads.Add(1)
			testutil.RespondJSON(w, readStatus, readBody)
			return
		}
		calls.writes.Add(1)
		testutil.RespondJSON(w, http.StatusOK, storedHook)
	}))
}

// hookRefusal is the refusal a call on hook 7 of project 42 is answered with
// when nobody can be asked: the question, naming what the call does, and the
// way to proceed.
func hookRefusal(op, changes string) string {
	return op + ": Change where webhook 7 in project `42` delivers, or a secret it sends? This call " + changes + "." +
		" Re-send the same call with confirm=true once the user has approved it"
}

// hookCallCases are the calls the three handlers take, each with what the
// guard is expected to make of it against [storedHook]: the changes it asks
// about, or none when the call goes out unasked, and whether it reads the
// hook to decide.
func hookCallCases() []struct {
	name  string
	call  func(context.Context, *gitlabclient.Client) error
	op    string
	reads int64
	want  string
} {
	edit := func(input EditHookInput) func(context.Context, *gitlabclient.Client) error {
		input.ProjectID, input.HookID = "42", 7
		return func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := EditHook(ctx, client, input)
			return err
		}
	}
	header := func(key string) func(context.Context, *gitlabclient.Client) error {
		return func(ctx context.Context, client *gitlabclient.Client) error {
			return SetCustomHeader(ctx, client, SetCustomHeaderInput{ProjectID: "42", HookID: 7, Key: key, Value: "v"})
		}
	}
	variable := func(key string) func(context.Context, *gitlabclient.Client) error {
		return func(ctx context.Context, client *gitlabclient.Client) error {
			return SetWebhookURLVariable(ctx, client, SetWebhookURLVariableInput{ProjectID: "42", HookID: 7, Key: key, Value: "v"})
		}
	}
	return []struct {
		name  string
		call  func(context.Context, *gitlabclient.Client) error
		op    string
		reads int64
		want  string
	}{
		{name: "an edit of events alone", call: edit(EditHookInput{PushEvents: new(false)})},
		{name: "an edit that resends the address the hook has", call: edit(EditHookInput{URL: "https://old.example/hooks/abc"}), reads: 1},
		{
			name: "an edit that sends another address", call: edit(EditHookInput{URL: "https://new.example/hooks/xyz?token=secret"}),
			op: "projectEditHook", reads: 1, want: "points it at `https[:]//new.example/...` instead of `https[:]//old.example/...`",
		},
		{
			name: "an edit that sends a token", call: edit(EditHookInput{Token: "t"}),
			op: "projectEditHook", want: "replaces its token, which cannot be read back",
		},
		{
			name: "an edit that sends a signing token", call: edit(EditHookInput{SigningToken: "s"}),
			op: "projectEditHook", want: "replaces its signing_token, which cannot be read back",
		},
		{
			name: "an edit that sends keys the hook lacks",
			call: edit(EditHookInput{
				URLVariables:  []HookURLVariableInput{{Key: "region", Value: "v"}},
				CustomHeaders: []HookCustomHeaderInput{{Key: "X-Fresh", Value: "v"}},
			}),
			reads: 1,
		},
		{
			name: "an edit that sends keys the hook has",
			call: edit(EditHookInput{
				URLVariables:  []HookURLVariableInput{{Key: "tenant", Value: "v"}},
				CustomHeaders: []HookCustomHeaderInput{{Key: "Authorization", Value: "v"}},
			}),
			op: "projectEditHook", reads: 1,
			want: "replaces the url_variables entry `tenant`, whose current value cannot be read back" +
				" and replaces the custom_headers entry `Authorization`, whose current value cannot be read back",
		},
		{name: "a custom header the hook lacks", call: header("X-Fresh"), reads: 1},
		{
			name: "a custom header the hook has", call: header("Authorization"), op: "projectSetCustomHeader", reads: 1,
			want: "replaces the custom_headers entry `Authorization`, whose current value cannot be read back",
		},
		{name: "a URL variable the hook lacks", call: variable("region"), reads: 1},
		{
			name: "a URL variable the hook has", call: variable("tenant"), op: "projectSetWebhookURLVariable", reads: 1,
			want: "replaces the url_variables entry `tenant`, whose current value cannot be read back",
		},
	}
}

// TestProjectHookGuard_AsksOnlyForACallThatRedirectsOrReplacesASecret walks
// the calls of [hookCallCases] with nobody to ask. A call that changes
// neither the address nor a value the hook holds is written, after one read
// when only the hook can say so and without one otherwise. A call that does
// is refused with the question, word for word, before any write.
func TestProjectHookGuard_AsksOnlyForACallThatRedirectsOrReplacesASecret(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	for _, tt := range hookCallCases() {
		t.Run(tt.name, func(t *testing.T) {
			var calls hookCalls
			client := hookClient(t, &calls, http.StatusOK, storedHook)

			err := tt.call(t.Context(), client)
			wantWrites := int64(1)
			if tt.want != "" {
				wantWrites = 0
				if want := hookRefusal(tt.op, tt.want); err == nil || err.Error() != want {
					t.Errorf("refusal =\n%v\nwant\n%q", err, want)
				}
			} else if err != nil {
				t.Errorf("error = %v, want the call to go out unasked", err)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != tt.reads || writes != wantWrites {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want %d and %d", reads, writes, tt.reads, wantWrites)
			}
		})
	}
}

// TestProjectHookGuard_AConfirmedCall_IsWrittenWithoutARead is the way
// through: with confirm=true on the call, in the shape each surface sends
// it, or with confirmation switched off for the server, every call of
// [hookCallCases] is written and the hook is not read, since nobody is going
// to be asked what it changes.
func TestProjectHookGuard_AConfirmedCall_IsWrittenWithoutARead(t *testing.T) {
	confirmed := map[string]struct {
		arguments string
		yolo      string
	}{
		"confirm flat":            {arguments: `{"project_id":"42","hook_id":7,"confirm":true}`},
		"confirm inside params":   {arguments: `{"action":"hook_edit","params":{"project_id":"42","hook_id":7,"confirm":true}}`},
		"confirm beside params":   {arguments: `{"action":"project.hook_edit","params":{"project_id":"42","hook_id":7},"confirm":true}`},
		"confirmation turned off": {arguments: `{"project_id":"42","hook_id":7}`, yolo: "true"},
	}
	for how, way := range confirmed {
		for _, tt := range hookCallCases() {
			t.Run(how+"/"+tt.name, func(t *testing.T) {
				t.Setenv("GITLAB_MCP_YOLO_MODE", way.yolo)
				t.Setenv("AUTOPILOT", "")
				var calls hookCalls
				client := hookClient(t, &calls, http.StatusOK, storedHook)
				ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{Name: "gitlab_project_hook_edit", Arguments: json.RawMessage(way.arguments)},
				})

				if err := tt.call(ctx, client); err != nil {
					t.Errorf("error = %v, want the confirmed call to be written", err)
				}
				if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != 0 || writes != 1 {
					t.Errorf("GitLab was sent %d read(s) and %d write(s), want 0 and 1", reads, writes)
				}
			})
		}
	}
}

// TestProjectHookGuard_AHookThatCannotBeRead_IsAskedAbout pins what the read
// answering anything but the hook does. A hook that does not exist has
// nothing to redirect, so the call goes on to GitLab's own refusal. Any other
// failure names the address and every key the call sends as unread, and
// nothing is written: a guard that cannot see the hook does not assume the
// call is harmless.
func TestProjectHookGuard_AHookThatCannotBeRead_IsAskedAbout(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	input := EditHookInput{
		ProjectID: "42", HookID: 7, URL: "https://old.example/hooks/abc",
		CustomHeaders: []HookCustomHeaderInput{{Key: "X-Fresh", Value: "v"}},
	}
	tests := []struct {
		name   string
		status int
		body   string
		asked  bool
	}{
		{name: "a hook that does not exist", status: http.StatusNotFound, body: `{"message":"404 Not found"}`},
		{name: "a read GitLab refuses", status: http.StatusForbidden, body: `{"message":"403 Forbidden"}`, asked: true},
		{name: "a read GitLab fails", status: http.StatusInternalServerError, body: `{"message":"500"}`, asked: true},
		{name: "a body that is no hook", status: http.StatusOK, body: `[`, asked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls hookCalls
			client := hookClient(t, &calls, tt.status, tt.body)

			_, err := EditHook(t.Context(), client, input)
			want := hookRefusal("projectEditHook", "may point it at another host, `https[:]//old.example/...` (the webhook could not be read)"+
				" and may replace the custom_headers entry `X-Fresh` (the webhook could not be read)")
			wantWrites := int64(1)
			if tt.asked {
				wantWrites = 0
				if err == nil || err.Error() != want {
					t.Errorf("refusal =\n%v\nwant\n%q", err, want)
				}
			} else if err != nil {
				t.Errorf("error = %v, want the edit to go on to the write", err)
			}
			if reads, writes := calls.reads.Load(), calls.writes.Load(); reads != 1 || writes != wantWrites {
				t.Errorf("GitLab was sent %d read(s) and %d write(s), want 1 and %d", reads, writes, wantWrites)
			}
		})
	}
}

// TestProjectHookGuard_TheRead_CarriesTheCallersContext proves the guard's
// read is sent with the caller's context, so an abandoned call cancels it,
// and that a call abandoned during the read is answered with the context's
// error rather than with a question nobody is there to answer.
func TestProjectHookGuard_TheRead_CarriesTheCallersContext(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storedHook)
	})

	_, err := EditHook(ctx, client, EditHookInput{ProjectID: "42", HookID: 7, URL: "https://new.example"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EditHook() error = %v, want context.Canceled", err)
	}
}
