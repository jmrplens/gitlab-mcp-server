// pull_mirror_guard_test.go contains unit tests for the confirmation guard in
// front of a pull-mirror configuration that overwrites diverged branches.
package projects

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// mirrorSourceURL is the source a guarded call points the mirror at.
const mirrorSourceURL = "https://github.com/example/repo.git"

// toolPullMirrorConfigure is the individual tool the guarded action projects.
const toolPullMirrorConfigure = "gitlab_project_pull_mirror_configure"

// guardCalls counts what a configure call sent GitLab, so a test can assert
// both that the PUT was refused and that the guard's inspection read was
// skipped where it costs nothing.
type guardCalls struct {
	reads  atomic.Int64
	writes atomic.Int64
}

// guardHandler answers the pull-mirror endpoint: GET with the stored
// configuration the guard inspects, PUT with the updated one. Passing a status
// other than 200 for the read makes GitLab refuse to describe the mirror, which
// is how a project with no mirror and an unreadable one are both expressed.
func guardHandler(t *testing.T, calls *guardCalls, readStatus int, readBody string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != pathProject42MirrorPull {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			calls.reads.Add(1)
			testutil.RespondJSON(w, readStatus, readBody)
		case http.MethodPut:
			calls.writes.Add(1)
			testutil.RespondJSON(w, http.StatusOK, pullMirrorJSON)
		default:
			http.NotFound(w, r)
		}
	}
}

// storedMirrorJSON renders the pull-mirror configuration a project already
// holds, which decides what an unsent flag inherits.
func storedMirrorJSON(enabled, overwrites bool) string {
	return `{"id":5,"enabled":` + jsonBool(enabled) + `,"url":"https://github.com/example/old.git",` +
		`"mirror_overwrites_diverged_branches":` + jsonBool(overwrites) + `}`
}

// jsonBool renders a bool as the JSON literal, so the fixture reads as the
// document GitLab sends.
func jsonBool(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// notMirroredJSON is GitLab's refusal to describe the pull mirror of a project
// that has none, which the guard reads as nothing to inherit.
const notMirroredJSON = `{"message":"400 Bad request - The project is not mirrored"}`

// assertRefusalMentions checks that a refusal says each of the things a caller
// needs from it: what would happen, to which source, and how to proceed.
func assertRefusalMentions(t *testing.T, err error, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

// guardedClient builds a client whose pull-mirror endpoint is served by
// guardHandler.
func guardedClient(t *testing.T, calls *guardCalls, readStatus int, readBody string) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, guardHandler(t, calls, readStatus, readBody))
}

// TestConfigurePullMirror_ArmingTheOverwrite_IsRefusedWithoutConfirmation
// verifies that a configuration which leaves the mirror overwriting diverged
// branches is refused when nothing confirmed it, that the refusal names the
// source and how to proceed, and that GitLab was never asked to apply it.
//
// This is the case the guard exists for: the URL is a parameter a model can
// take from untrusted text, and the overwrite is what makes the pull replace
// this project's own commits.
func TestConfigurePullMirror_ArmingTheOverwrite_IsRefusedWithoutConfirmation(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("arming the diverged-branch overwrite was accepted without confirmation")
	}
	assertRefusalMentions(t, err, "confirm=true", "github.com/example/repo.git", "overwrite diverged branches")
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// TestConfigurePullMirror_ArmingTheOverwrite_ProceedsWithExplicitConfirm
// verifies that an explicit confirm=true applies the configuration.
//
// The confirmation is read from the raw tool call, not from the typed input:
// the reserved confirm key is stripped before unmarshalling, so a test that set
// the struct field would pass while the real MCP path stayed blocked. The
// request is therefore built the way a caller sends it.
func TestConfigurePullMirror_ArmingTheOverwrite_ProceedsWithExplicitConfirm(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_project_pull_mirror_configure",
			Arguments: json.RawMessage(`{"project_id":"42","url":"` + mirrorSourceURL + `","mirror_overwrites_diverged_branches":true,"confirm":true}`),
		},
	})

	out, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 5 {
		t.Errorf("ID = %d, want the configured mirror", out.ID)
	}
	if got := calls.reads.Load(); got != 0 {
		t.Errorf("a confirmed call paid for %d inspection read(s)", got)
	}
}

// TestConfigurePullMirror_ArmingTheOverwrite_ProceedsWithNestedConfirm
// verifies the dispatcher call shape, where the reserved key travels inside
// params rather than at the top level.
func TestConfigurePullMirror_ArmingTheOverwrite_ProceedsWithNestedConfirm(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_execute_action",
			Arguments: json.RawMessage(`{"action":"project.pull_mirror_configure","params":{"project_id":"42","mirror_overwrites_diverged_branches":true,"confirm":true}}`),
		},
	})

	if _, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		MirrorOverwritesDivergedBranches: new(true),
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestConfigurePullMirror_YOLOMode_SkipsTheConfirmation verifies that the
// operator's own bypass applies here like it does to a destructive route, and
// that it is checked before the inspection read.
func TestConfigurePullMirror_YOLOMode_SkipsTheConfirmation(t *testing.T) {
	t.Setenv("GITLAB_MCP_YOLO_MODE", "true")
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.reads.Load(); got != 0 {
		t.Errorf("YOLO mode paid for %d inspection read(s)", got)
	}
}

// TestConfigurePullMirror_ArmingTheFlagOnARunningMirror_IsRefused verifies the
// call that says nothing about the source or about enabled: the mirror is
// already running, so setting the flag alone is what starts the replacing, and
// the prompt names the source the project already has.
func TestConfigurePullMirror_ArmingTheFlagOnARunningMirror_IsRefused(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("arming the flag on a running mirror was accepted without confirmation")
	}
	assertRefusalMentions(t, err, "confirm=true", "its configured mirror source")
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// TestConfigurePullMirror_UnreadableConfirmationState_IsRefused verifies that a
// confirmation exchange this server cannot read is refused rather than
// bypassed: the state travels through the client, and a mangled one must not
// be the way past the guard.
func TestConfigurePullMirror_UnreadableConfirmationState_IsRefused(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	req := elicitingRequest(nil)
	req.Params.RequestState = "not a state this server issued"
	ctx := toolutil.ContextWithRequest(t.Context(), req)

	_, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("an unreadable confirmation state applied the configuration")
	}
	assertRefusalMentions(t, err, "confirm=true")
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// TestConfigurePullMirror_DisablingTheMirror_NeverAsks verifies that turning
// mirroring off is never interactive, even when the call carries the overwrite
// flag: a disabled mirror pulls nothing, and asking the user to approve the
// remedy would be the guard working backwards.
func TestConfigurePullMirror_DisablingTheMirror_NeverAsks(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, true))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(false),
		MirrorOverwritesDivergedBranches: new(true),
		URL:                              mirrorSourceURL,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.reads.Load(); got != 0 {
		t.Errorf("disabling the mirror paid for %d inspection read(s)", got)
	}
}

// TestConfigurePullMirror_ClearingTheOverwrite_NeverAsks verifies that sending
// the flag as false proceeds without a prompt and without a read, which is the
// other remedy: the mirror keeps running and stops replacing branches.
func TestConfigurePullMirror_ClearingTheOverwrite_NeverAsks(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, true))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(false),
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.reads.Load(); got != 0 {
		t.Errorf("clearing the overwrite paid for %d inspection read(s)", got)
	}
}

// TestConfigurePullMirror_UnrelatedEdit_NeverAsks verifies that changing a
// setting which cannot arm the overwrite is neither read nor confirmed, even on
// a mirror that already overwrites. A guard that fires on every edit of the
// same action is a guard a caller learns to pass through.
func TestConfigurePullMirror_UnrelatedEdit_NeverAsks(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, true))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:           "42",
		MirrorTriggerBuilds: new(true),
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.reads.Load(); got != 0 {
		t.Errorf("an unrelated edit paid for %d inspection read(s)", got)
	}
}

// TestConfigurePullMirror_RepointingAnOverwritingMirror_IsRefused verifies the
// two-step shape the flag alone would miss: the project already overwrites, so
// a call that only changes the URL aims that overwrite at a repository this
// call names, and its branches replace the project's.
func TestConfigurePullMirror_RepointingAnOverwritingMirror_IsRefused(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, true))

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		URL:       mirrorSourceURL,
	})
	if err == nil {
		t.Fatal("repointing an overwriting mirror was accepted without confirmation")
	}
	if !strings.Contains(err.Error(), "confirm=true") {
		t.Errorf("error should tell the caller how to confirm, got: %v", err)
	}
	if got := calls.reads.Load(); got != 1 {
		t.Errorf("reads = %d, want the one inspection read", got)
	}
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// TestConfigurePullMirror_RepointingAPlainMirror_Proceeds verifies that the
// same call on a mirror that does not overwrite is an ordinary update: GitLab
// stops updating a diverged branch instead of replacing it, so nothing of this
// project is lost and nothing is asked.
func TestConfigurePullMirror_RepointingAPlainMirror_Proceeds(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		URL:       mirrorSourceURL,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.writes.Load(); got != 1 {
		t.Errorf("writes = %d, want the configuration applied once", got)
	}
}

// TestConfigurePullMirror_DisabledMirror_ProceedsUntilItIsEnabled verifies both
// halves of the state rule on a mirror that is configured to overwrite but is
// switched off: arming nothing while it stays off proceeds, and the call that
// turns it on is the one that asks, because that is when the overwrite starts
// happening.
func TestConfigurePullMirror_DisabledMirror_ProceedsUntilItIsEnabled(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false, true))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		URL:       mirrorSourceURL,
	}); err != nil {
		t.Fatalf("configuring a disabled mirror: %v", err)
	}

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		Enabled:   new(true),
	})
	if err == nil {
		t.Fatal("enabling a mirror that overwrites diverged branches was accepted without confirmation")
	}
	if !strings.Contains(err.Error(), "its configured mirror source") {
		t.Errorf("a call that names no URL should say the source is the configured one, got: %v", err)
	}
}

// TestConfigurePullMirror_FirstMirrorWithoutTheOverwrite_Proceeds verifies that
// setting up a mirror on a project that has none is an ordinary create: there
// is no stored configuration to inherit an overwrite from, so the guard asks
// nothing.
func TestConfigurePullMirror_FirstMirrorWithoutTheOverwrite_Proceeds(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusBadRequest, notMirroredJSON)

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		Enabled:   new(true),
		URL:       mirrorSourceURL,
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.writes.Load(); got != 1 {
		t.Errorf("writes = %d, want the configuration applied once", got)
	}
}

// TestConfigurePullMirror_FirstMirrorArmingTheOverwrite_IsRefused verifies the
// unknown the "not mirrored" answer leaves: the call arms the overwrite and
// says nothing about enabled, and whether GitLab activates a mirror created
// without that flag is not something its API documents. The guess that costs a
// prompt is taken over the guess that skips one.
func TestConfigurePullMirror_FirstMirrorArmingTheOverwrite_IsRefused(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusBadRequest, notMirroredJSON)

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("a first mirror that overwrites diverged branches was accepted without confirmation")
	}
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// TestConfigurePullMirror_UnreadableConfiguration_FailsClosed verifies that a
// read the guard cannot interpret is answered by asking: without the stored
// configuration it cannot tell an ordinary repoint from aiming an existing
// overwrite somewhere else, and assuming the harmless case would be the one
// mistake it exists to prevent.
func TestConfigurePullMirror_UnreadableConfiguration_FailsClosed(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		URL:       mirrorSourceURL,
	})
	if err == nil {
		t.Fatal("an unreadable configuration was treated as harmless")
	}
	if !strings.Contains(err.Error(), "confirm=true") {
		t.Errorf("error should tell the caller how to confirm, got: %v", err)
	}
}

// elicitingRequest builds a tool call from a client that supports form
// elicitation over the multi round-trip mechanism, carrying the answers the
// user has already given.
func elicitingRequest(answers mcp.InputResponseMap) *mcp.CallToolRequest {
	return &mcp.CallToolRequest{
		Session: &mcp.ServerSession{},
		Params: &mcp.CallToolParamsRaw{
			Name:      "gitlab_project_pull_mirror_configure",
			Arguments: json.RawMessage(`{"project_id":"42"}`),
			Meta: mcp.Meta{
				"io.modelcontextprotocol/protocolVersion": "2026-07-28",
				"io.modelcontextprotocol/clientCapabilities": map[string]any{
					"elicitation": map[string]any{"form": map[string]any{}},
				},
			},
			InputResponses: answers,
		},
	}
}

// TestConfigurePullMirror_ElicitingClient_AsksBeforeArming verifies that a
// client which can prompt is asked rather than refused: the first pass returns
// the pending input request the surface turns into an input-required result.
func TestConfigurePullMirror_ElicitingClient_AsksBeforeArming(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	ctx := toolutil.ContextWithRequest(t.Context(), elicitingRequest(nil))
	_, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		URL:                              mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("the guard applied the configuration instead of asking")
	}
	if _, pending := toolutil.InputRequiredResultFromError(err); !pending {
		t.Errorf("error = %v, want the pending input request", err)
	}
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) while the prompt was pending", got)
	}
}

// TestConfigurePullMirror_ElicitedAnswer_DecidesTheCall verifies both answers a
// user can give: accepting applies the configuration, declining refuses it and
// names the way to mirror without replacing branches.
func TestConfigurePullMirror_ElicitedAnswer_DecidesTheCall(t *testing.T) {
	tests := []struct {
		name      string
		content   map[string]any
		wantError string
	}{
		{name: "accepted", content: map[string]any{"confirmed": true}},
		{name: "declined", content: map[string]any{"confirmed": false}, wantError: "mirror_overwrites_diverged_branches=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

			ctx := toolutil.ContextWithRequest(t.Context(), elicitingRequest(mcp.InputResponseMap{
				pullMirrorOverwriteConfirmID: &mcp.ElicitResult{Action: "accept", Content: tt.content},
			}))
			_, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
				ProjectID:                        "42",
				Enabled:                          new(true),
				URL:                              mirrorSourceURL,
				MirrorOverwritesDivergedBranches: new(true),
			})
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf(fmtUnexpErr, err)
				}
				if got := calls.writes.Load(); got != 1 {
					t.Errorf("writes = %d, want the approved configuration applied once", got)
				}
				return
			}
			if err == nil {
				t.Fatal("a declined confirmation applied the configuration")
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error should mention %q, got: %v", tt.wantError, err)
			}
		})
	}
}

// TestConfigurePullMirror_MalformedElicitationAnswer_IsRefused verifies that an
// answer to another question does not arm the overwrite: the exchange failed,
// and a failed confirmation fails closed rather than proceeding.
func TestConfigurePullMirror_MalformedElicitationAnswer_IsRefused(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))

	ctx := toolutil.ContextWithRequest(t.Context(), elicitingRequest(mcp.InputResponseMap{
		pullMirrorOverwriteConfirmID: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"title": "unrelated"}},
	}))
	_, err := ConfigurePullMirror(ctx, client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		Enabled:                          new(true),
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("a malformed confirmation applied the configuration")
	}
	if !strings.Contains(err.Error(), "confirmation failed") {
		t.Errorf("error should report the failed exchange, got: %v", err)
	}
}

// TestConfigurePullMirror_ThroughTheRoute_HonorsTheReservedConfirmKey drives
// the path a surface actually takes: the route's own handler, given the
// parameters as a map with the reserved key among them and the raw call in the
// context.
//
// Two things could only fail here. The typed input is unmarshalled strictly, so
// a confirm key the struct did not declare would be refused as an unknown
// field before the handler ran; and the guard reads the confirmation from the
// raw call rather than from that map, so the request has to reach it through
// the context the surface installs. The unconfirmed half is asserted beside it,
// since a key that is accepted but never consulted would pass the first half
// alone.
func TestConfigurePullMirror_ThroughTheRoute_HonorsTheReservedConfirmKey(t *testing.T) {
	tests := []struct {
		name      string
		confirm   bool
		wantWrite int64
	}{
		{name: "confirmed", confirm: true, wantWrite: 1},
		{name: "unconfirmed", confirm: false, wantWrite: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true, false))
			route := projectSpecsByTool(t, ActionSpecs(client, false))[toolPullMirrorConfigure].Route

			params := map[string]any{
				"project_id": "42", "enabled": true, "url": mirrorSourceURL,
				"mirror_overwrites_diverged_branches": true,
			}
			if tt.confirm {
				params["confirm"] = true
			}
			arguments, marshalErr := json.Marshal(params)
			if marshalErr != nil {
				t.Fatalf("marshal params: %v", marshalErr)
			}
			ctx := toolutil.ContextWithRequest(t.Context(), &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: toolPullMirrorConfigure, Arguments: arguments},
			})

			_, err := route.Handler(ctx, params)
			if tt.confirm && err != nil {
				t.Fatalf("a confirmed call was refused: %v", err)
			}
			if !tt.confirm {
				if err == nil {
					t.Fatal("an unconfirmed call was accepted through the route")
				}
				assertRefusalMentions(t, err, "confirm=true")
			}
			if got := calls.writes.Load(); got != tt.wantWrite {
				t.Errorf("writes = %d, want %d", got, tt.wantWrite)
			}
		})
	}
}

// TestPullMirrorSourceForPrompt_Values verifies what the confirmation says the
// project would pull from. The URL is the caller's own parameter, so a
// credential written into its userinfo must not be echoed into a prompt, and a
// value that is not an absolute URL is withheld rather than shown.
func TestPullMirrorSourceForPrompt_Values(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty names the configured source", raw: "", want: "its configured mirror source"},
		{name: "host and path survive", raw: mirrorSourceURL, want: mirrorSourceURL},
		{name: "userinfo is dropped", raw: "https://user:token@github.com/example/repo.git", want: mirrorSourceURL},
		{name: "query and fragment are dropped", raw: "https://github.com/example/repo.git?token=secret#frag", want: mirrorSourceURL},
		{name: "scp syntax is withheld", raw: "git@github.com:example/repo.git", want: toolutil.RedactedPlaceholder},
		{name: "unparseable text is withheld", raw: "https://exa mple.com/\x7f", want: toolutil.RedactedPlaceholder},
		{name: "a bare path names no scheme and is withheld", raw: "github.com/example/repo.git", want: toolutil.RedactedPlaceholder},
		{name: "a local path names no host and is withheld", raw: "file:///srv/git/repo.git", want: toolutil.RedactedPlaceholder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pullMirrorSourceForPrompt(tt.raw); got != tt.want {
				t.Errorf("pullMirrorSourceForPrompt(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
