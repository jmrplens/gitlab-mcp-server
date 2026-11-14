// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// hook_guard_test.go holds the two calls that change a system hook to the
// confirmation they owe: none for a call that neither redirects the hook nor
// replaces a secret, and one, before anything is written, for a call that
// does.
package systemhooks

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

// storedSystemHook is a system hook with an address, one URL variable and
// one custom header, so each way of redirecting it or replacing a secret has
// something to change.
const storedSystemHook = `{"id":7,"url":"https://old.example/hooks/abc",` +
	`"url_variables":[{"key":"tenant"}],"custom_headers":[{"key":"Authorization"}]}`

// systemHookCalls counts what the fake GitLab was sent for the hook, by kind.
type systemHookCalls struct {
	reads  atomic.Int64
	writes atomic.Int64
}

// countingSystemHookClient answers every route under system hook 7 and counts what
// it was sent: GET with readStatus and readBody, the hook the guard compares
// the call with, and any other method with the hook as written.
func countingSystemHookClient(t *testing.T, calls *systemHookCalls, readStatus int, readBody string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v4/hooks/7") {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodGet {
			calls.reads.Add(1)
			testutil.RespondJSON(w, readStatus, readBody)
			return
		}
		calls.writes.Add(1)
		testutil.RespondJSON(w, http.StatusOK, storedSystemHook)
	}))
}

// systemHookRefusal is the refusal a call on system hook 7 is answered with
// when nobody can be asked: the question, naming what the call does, and the
// way to proceed.
func systemHookRefusal(op, changes string) string {
	return op + ": Change where webhook 7 in this GitLab instance delivers, or a secret it sends? This call " + changes + "." +
		" Re-send the same call with confirm=true once the user has approved it"
}

// systemHookCase is one call of the two handlers with what the guard is
// expected to make of it against [storedSystemHook]: the changes it asks
// about, or none when the call goes out unasked, and whether it reads the
// hook to decide.
type systemHookCase struct {
	name  string
	call  func(context.Context, *gitlabclient.Client) error
	op    string
	reads int64
	want  string
}

// systemHookCases are the calls the guard is walked through.
func systemHookCases() []systemHookCase {
	edit := func(input EditInput) func(context.Context, *gitlabclient.Client) error {
		input.ID = 7
		return func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Edit(ctx, client, input)
			return err
		}
	}
	variable := func(key string) func(context.Context, *gitlabclient.Client) error {
		return func(ctx context.Context, client *gitlabclient.Client) error {
			return SetURLVariable(ctx, client, SetURLVariableInput{ID: 7, Key: key, Value: "v"})
		}
	}
	return []systemHookCase{
		{name: "an edit of events alone", call: edit(EditInput{PushEvents: new(false)})},
		{name: "an edit that resends the address the hook has", call: edit(EditInput{URL: "https://old.example/hooks/abc"}), reads: 1},
		{
			name: "an edit that sends another address", call: edit(EditInput{URL: "https://new.example/hooks/xyz?token=secret"}),
			op: "system_hook_edit", reads: 1, want: "points it at `https[:]//new.example/...` instead of `https[:]//old.example/...`",
		},
		{
			name: "an edit that sends a token", call: edit(EditInput{Token: "t"}), op: "system_hook_edit",
			want: "replaces its token, which cannot be read back",
		},
		{
			name: "an edit that sends a signing token", call: edit(EditInput{SigningToken: "s"}), op: "system_hook_edit",
			want: "replaces its signing_token, which cannot be read back",
		},
		{name: "a URL variable the hook lacks", call: variable("region"), reads: 1},
		{
			name: "a URL variable the hook has", call: variable("tenant"), op: "system_hook_set_url_variable", reads: 1,
			want: "replaces the url_variables entry `tenant`, whose current value cannot be read back",
		},
	}
}

// TestSystemHookGuard_AsksOnlyForACallThatRedirectsOrReplacesASecret walks
// the calls of [systemHookCases] with nobody to ask. A call that changes
// neither the address nor a value the hook holds is written, after one read
// when only the hook can say so and without one otherwise. A call that does
// is refused with the question, word for word, before any write.
func TestSystemHookGuard_AsksOnlyForACallThatRedirectsOrReplacesASecret(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	for _, tt := range systemHookCases() {
		t.Run(tt.name, func(t *testing.T) {
			var calls systemHookCalls
			client := countingSystemHookClient(t, &calls, http.StatusOK, storedSystemHook)

			err := tt.call(t.Context(), client)
			wantWrites := int64(1)
			if tt.want != "" {
				wantWrites = 0
				if want := systemHookRefusal(tt.op, tt.want); err == nil || err.Error() != want {
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

// TestSystemHookGuard_AConfirmedCall_IsWrittenWithoutARead is the way
// through: with confirm=true on the call, or with confirmation switched off
// for the server, every call of [systemHookCases] is written and the hook is
// not read, since nobody is going to be asked what it changes.
func TestSystemHookGuard_AConfirmedCall_IsWrittenWithoutARead(t *testing.T) {
	confirmed := map[string]struct {
		arguments string
		yolo      string
	}{
		"confirm on the call":     {arguments: `{"action":"admin.system_hook_edit","params":{"id":7},"confirm":true}`},
		"confirmation turned off": {arguments: `{"id":7}`, yolo: "true"},
	}
	for how, way := range confirmed {
		for _, tt := range systemHookCases() {
			t.Run(how+"/"+tt.name, func(t *testing.T) {
				t.Setenv("GITLAB_MCP_YOLO_MODE", way.yolo)
				t.Setenv("AUTOPILOT", "")
				var calls systemHookCalls
				client := countingSystemHookClient(t, &calls, http.StatusOK, storedSystemHook)
				ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{Name: "gitlab_execute_action", Arguments: json.RawMessage(way.arguments)},
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

// TestSystemHookGuard_AHookThatCannotBeRead_IsAskedAbout pins what the read
// answering anything but the hook does. A hook that does not exist has
// nothing to redirect, so the call goes on to GitLab's own refusal. Any other
// failure names the address the call sends as unread, and nothing is
// written: a guard that cannot see the hook does not assume the call is
// harmless.
func TestSystemHookGuard_AHookThatCannotBeRead_IsAskedAbout(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	tests := []struct {
		name   string
		status int
		asked  bool
	}{
		{name: "a hook that does not exist", status: http.StatusNotFound},
		{name: "a read GitLab refuses", status: http.StatusForbidden, asked: true},
		{name: "a read GitLab fails", status: http.StatusInternalServerError, asked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls systemHookCalls
			client := countingSystemHookClient(t, &calls, tt.status, `{"message":"refused"}`)

			_, err := Edit(t.Context(), client, EditInput{ID: 7, URL: "https://old.example/hooks/abc"})
			want := systemHookRefusal("system_hook_edit", "may point it at another host, `https[:]//old.example/...` (the webhook could not be read)")
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

// TestSystemHookGuard_TheRead_CarriesTheCallersContext proves the guard's
// read is sent with the caller's context, so an abandoned call cancels it,
// and that a call abandoned during the read is answered with the context's
// error rather than with a question nobody is there to answer.
func TestSystemHookGuard_TheRead_CarriesTheCallersContext(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "")
	t.Setenv("AUTOPILOT", "")
	ctx, client := testutil.CancelOnArrival(t, func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, storedSystemHook)
	})

	_, err := Edit(ctx, client, EditInput{ID: 7, URL: "https://new.example"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Edit() error = %v, want context.Canceled", err)
	}
}
