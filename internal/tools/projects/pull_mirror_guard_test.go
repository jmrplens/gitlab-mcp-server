// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

// toolProjectUpdate is the individual tool project.update projects, the other
// action that can arm the overwrite.
const toolProjectUpdate = "gitlab_project_update"

// The operation labels each guarded handler opens its refusals with.
const (
	opConfigurePullMirror = "projectConfigurePullMirror"
	opProjectUpdate       = "projectUpdate"
)

// configuredMirrorSource is how the question names the source of a call that
// sends no URL and so keeps the one the project already pulls from.
const configuredMirrorSource = "its configured mirror source"

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
// holds, which decides what an unsent flag inherits. It pulls from another
// repository than mirrorSourceURL, so a call naming that URL re-aims it.
func storedMirrorJSON(overwrites bool) string {
	return storedMirrorAt(overwrites, "https://github.com/example/old.git")
}

// storedMirrorAt renders the configuration of a mirror that pulls from source,
// the url GitLab reports for it.
//
// The mirror is always enabled, because that is the only one GET
// /projects/:id/mirror/pull describes: it answers a mirror that is off 400 not
// mirrored, which notMirroredJSON is. A fixture that described one with
// enabled false would test the guard against an answer GitLab never sends.
func storedMirrorAt(overwrites bool, source string) string {
	return `{"id":5,"enabled":true,"url":"` + source + `",` +
		`"mirror_overwrites_diverged_branches":` + jsonBool(overwrites) + `}`
}

// maskedMirrorSourceURL is how GitLab reports mirrorSourceURL for a mirror that
// authenticates: Project#safe_import_url replaces the user and the password
// with ***** each.
const maskedMirrorSourceURL = "https://*****:*****@github.com/example/repo.git"

// unparseableMirrorSource is a URL net/url refuses, for an invalid escape.
const unparseableMirrorSource = "https://github.com/example/%zz.git"

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
	if want := mirrorRefusal(opConfigurePullMirror, mirrorSourceURL); err.Error() != want {
		t.Errorf("refusal =\n%q\nwant\n%q", err.Error(), want)
	}
	if got := calls.writes.Load(); got != 0 {
		t.Errorf("the configuration was applied %d time(s) despite being refused", got)
	}
}

// mirrorRefusal is the refusal a client that cannot ask receives from the
// operation op for project 42 pulling from source: the guard's question in
// English, exactly as it was asked before the question became a catalog
// message, then how to proceed. The model reads it in English whatever
// language the person is asked in.
func mirrorRefusal(op, source string) string {
	return op + ": Configure project 42 to pull from " + source + " and overwrite diverged branches?" +
		" Every branch the mirror pulls that has diverged from that repository is replaced by its version on the next sync," +
		" and the replaced commits cannot be recovered through the API." +
		" Re-send the same call with confirm=true once the user has approved it"
}

// mirrorOffRefusal is the refusal a client that cannot ask receives from the
// operation op for a call that leaves project 42's mirror off and sets the
// flag on it, pulling from source once it is turned on: the question says
// that the overwrite waits for the mirror, which a call that leaves it off
// does not start.
func mirrorOffRefusal(op, source string) string {
	return op + ": Leave the pull mirror of project 42 off and set it to overwrite diverged branches from " + source +
		" once it is turned on? From the first sync after that, every branch the mirror pulls that has diverged" +
		" from that repository is replaced by its version, and the replaced commits cannot be recovered through the API." +
		" Re-send the same call with confirm=true once the user has approved it"
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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

	_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID:                        "42",
		MirrorOverwritesDivergedBranches: new(true),
	})
	if err == nil {
		t.Fatal("arming the flag on a running mirror was accepted without confirmation")
	}
	assertRefusalMentions(t, err, "confirm=true", configuredMirrorSource)
	if want := mirrorRefusal(opConfigurePullMirror, configuredMirrorSource); err.Error() != want {
		t.Errorf("refusal =\n%q\nwant\n%q", err.Error(), want)
	}
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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
// mirroring off is never interactive when the call does not set the overwrite
// flag, whether it leaves the stored flag alone or clears it, and that it pays
// for no read: a disabled mirror pulls nothing, and asking the user to approve
// the remedy would be the guard working backwards.
func TestConfigurePullMirror_DisablingTheMirror_NeverAsks(t *testing.T) {
	tests := []struct {
		name string
		flag *bool
	}{
		{name: "leaving the stored flag alone"},
		{name: "clearing the flag", flag: new(false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true))

			if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
				ProjectID:                        "42",
				Enabled:                          new(false),
				MirrorOverwritesDivergedBranches: tt.flag,
				URL:                              mirrorSourceURL,
			}); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if got := calls.reads.Load(); got != 0 {
				t.Errorf("disabling the mirror paid for %d inspection read(s)", got)
			}
			if got := calls.writes.Load(); got != 1 {
				t.Errorf("writes = %d, want the configuration applied once", got)
			}
		})
	}
}

// TestConfigurePullMirror_SettingTheOverwriteWhileDisabling_IsRefused verifies
// the call issue 1752 found going out unasked: one that turns the mirror off
// and sets the overwrite flag at once. GitLab keeps the flag on a mirror that
// is off and describes no such mirror, GET /projects/:id/mirror/pull answering
// it 400 not mirrored, so the later call that turns the mirror back on reads
// nothing to ask about, and every sync from then on replaces the project's
// diverged branches. The call that stores the flag is therefore the one asked,
// whether it names a URL or keeps the configured one, with a question saying
// that the overwrite waits for the mirror to be turned back on. It needs no
// read to know that, since it says itself what it leaves behind.
func TestConfigurePullMirror_SettingTheOverwriteWhileDisabling_IsRefused(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		source string
	}{
		{name: "keeping the configured source", source: configuredMirrorSource},
		{name: "naming a source", url: mirrorSourceURL, source: mirrorSourceURL},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

			_, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
				ProjectID:                        "42",
				Enabled:                          new(false),
				URL:                              tt.url,
				MirrorOverwritesDivergedBranches: new(true),
			})
			if err == nil {
				t.Fatal("setting the overwrite while turning the mirror off was accepted without confirmation")
			}
			if want := mirrorOffRefusal(opConfigurePullMirror, tt.source); err.Error() != want {
				t.Errorf("refusal =\n%q\nwant\n%q", err.Error(), want)
			}
			if got := calls.reads.Load(); got != 0 {
				t.Errorf("reads = %d, want none: the call says what it leaves behind", got)
			}
			if got := calls.writes.Load(); got != 0 {
				t.Errorf("the configuration was applied %d time(s) despite being refused", got)
			}
		})
	}
}

// TestConfigurePullMirror_ClearingTheOverwrite_NeverAsks verifies that sending
// the flag as false proceeds without a prompt and without a read, which is the
// other remedy: the mirror keeps running and stops replacing branches.
func TestConfigurePullMirror_ClearingTheOverwrite_NeverAsks(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(true))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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

// TestConfigurePullMirror_ResendingTheSourceItPullsFrom_Proceeds verifies that
// a URL is a re-aim only when it names another repository: an overwriting
// mirror sent the source it already pulls from, here with credentials GitLab
// reports masked, keeps pulling what the user approved when the overwrite was
// armed, so the call is the unrelated edit the guard lets through. The read
// that tells the two apart is the one inspection read.
func TestConfigurePullMirror_ResendingTheSourceItPullsFrom_Proceeds(t *testing.T) {
	var calls guardCalls
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorAt(true, maskedMirrorSourceURL))

	if _, err := ConfigurePullMirror(t.Context(), client, ConfigurePullMirrorInput{
		ProjectID: "42",
		URL:       "https://user:token@github.com/example/repo.git",
	}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got := calls.reads.Load(); got != 1 {
		t.Errorf("reads = %d, want the one inspection read", got)
	}
	if got := calls.writes.Load(); got != 1 {
		t.Errorf("writes = %d, want the configuration applied once", got)
	}
}

// TestSameMirrorSource_Values verifies which URLs name the repository a mirror
// already pulls from, as GET /projects/:id/mirror/pull reports it. Credentials
// never count, since GitLab reports them masked, and a scheme or host differing
// only in case is the same one. Everything else has to match as written, so a
// spelling GitLab might normalize, such as a default port or a trailing slash,
// still reads as another source: the guard errs toward asking. A URL net/url
// cannot parse, or one where an @ after the host leaves GitLab reading another
// host than net/url does, is never the same source either.
func TestSameMirrorSource_Values(t *testing.T) {
	tests := []struct {
		name    string
		sent    string
		current string
		want    bool
	}{
		{name: "the same URL", sent: mirrorSourceURL, current: mirrorSourceURL, want: true},
		{name: "credentials against GitLab's mask", sent: "https://user:token@github.com/example/repo.git", current: maskedMirrorSourceURL, want: true},
		{name: "a user alone against its mask", sent: "https://user@github.com/example/repo.git", current: "https://*****@github.com/example/repo.git", want: true},
		{name: "no credentials against a masked pair", sent: mirrorSourceURL, current: maskedMirrorSourceURL, want: true},
		{name: "the host in capitals", sent: "https://GITHUB.COM/example/repo.git", current: mirrorSourceURL, want: true},
		{name: "the scheme in capitals", sent: "HTTPS://github.com/example/repo.git", current: mirrorSourceURL, want: true},
		{name: "another repository", sent: mirrorSourceURL, current: "https://github.com/example/old.git", want: false},
		{name: "the path in another case", sent: "https://github.com/Example/repo.git", current: mirrorSourceURL, want: false},
		{name: "another host", sent: "https://gitlab.com/example/repo.git", current: mirrorSourceURL, want: false},
		{name: "another scheme", sent: "http://github.com/example/repo.git", current: mirrorSourceURL, want: false},
		{name: "a default port written out", sent: "https://github.com:443/example/repo.git", current: mirrorSourceURL, want: false},
		{name: "a trailing slash", sent: mirrorSourceURL + "/", current: mirrorSourceURL, want: false},
		{name: "an @ after the host", sent: "https://github.com/example@evil.example/repo.git", current: "https://github.com/example@evil.example/repo.git", want: false},
		{name: "a sent URL net/url refuses", sent: unparseableMirrorSource, current: mirrorSourceURL, want: false},
		{name: "a reported URL net/url refuses", sent: mirrorSourceURL, current: unparseableMirrorSource, want: false},
		{name: "both refused alike", sent: unparseableMirrorSource, current: unparseableMirrorSource, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameMirrorSource(tt.sent, tt.current); got != tt.want {
				t.Errorf("sameMirrorSource(%q, %q) = %v, want %v", tt.sent, tt.current, got, tt.want)
			}
		})
	}
}

// TestConfigurePullMirror_DisabledMirror_AsksWhenTheCallSetsTheOverwrite
// verifies the state rule on a mirror that is switched off, read as GitLab
// answers it: GET /projects/:id/mirror/pull refuses such a mirror with 400 not
// mirrored, the answer a project with no mirror gets, and so hides whatever
// flag it stores. A call that arms nothing while the mirror stays off
// proceeds, and one that sets the flag asks, whether it sends enabled false or
// leaves it out. Either way it is asked whether to leave the mirror off with
// the overwrite set: GitLab assigns the mirror's switch only when the call
// sends enabled, so one that leaves it out leaves the mirror off as surely as
// one that sends it false, and is not asked about a sync that will not come.
//
// Turning the mirror back on without the flag proceeds: the read shows no
// stored flag to inherit, so the question for a flag set while the mirror was
// off is asked on the call that sets it (issue 1752), and a flag stored before
// that question existed, or through another client, is not seen.
func TestConfigurePullMirror_DisabledMirror_AsksWhenTheCallSetsTheOverwrite(t *testing.T) {
	tests := []struct {
		name      string
		input     ConfigurePullMirrorInput
		refusal   string
		wantReads int64
	}{
		{
			name:      "aiming it at a source",
			input:     ConfigurePullMirrorInput{ProjectID: "42", URL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "setting the flag",
			input:     ConfigurePullMirrorInput{ProjectID: "42", MirrorOverwritesDivergedBranches: new(true)},
			refusal:   mirrorOffRefusal(opConfigurePullMirror, configuredMirrorSource),
			wantReads: 1,
		},
		{
			name: "setting the flag and keeping it off",
			input: ConfigurePullMirrorInput{
				ProjectID: "42", Enabled: new(false), MirrorOverwritesDivergedBranches: new(true),
			},
			refusal: mirrorOffRefusal(opConfigurePullMirror, configuredMirrorSource),
		},
		{
			name:      "turning it back on without the flag",
			input:     ConfigurePullMirrorInput{ProjectID: "42", Enabled: new(true)},
			wantReads: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := guardedClient(t, &calls, http.StatusBadRequest, notMirroredJSON)

			_, err := ConfigurePullMirror(t.Context(), client, tt.input)
			wantWrites := int64(1)
			if tt.refusal == "" {
				if err != nil {
					t.Fatalf(fmtUnexpErr, err)
				}
			} else {
				wantWrites = 0
				if err == nil || err.Error() != tt.refusal {
					t.Errorf("refusal =\n%v\nwant\n%q", err, tt.refusal)
				}
			}
			if got := calls.reads.Load(); got != tt.wantReads {
				t.Errorf("reads = %d, want %d", got, tt.wantReads)
			}
			if got := calls.writes.Load(); got != wantWrites {
				t.Errorf("writes = %d, want %d", got, wantWrites)
			}
		})
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

// TestConfigurePullMirror_FirstMirrorArmingTheOverwrite_IsRefused verifies a
// call that gives a project with no mirror a source and the overwrite and says
// nothing about enabled. GitLab assigns the mirror's switch only when the call
// sends it (the route's handler passes on only the params the call declared,
// and Repositories::PullMirrors::Attributes keeps only those), so the mirror
// stays off and nothing is replaced yet. The call is asked all the same, since
// it stores the flag that the call turning the mirror on would not see, and
// the question says that the overwrite waits for the mirror to be turned on.
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
	if want := mirrorOffRefusal(opConfigurePullMirror, mirrorSourceURL); err.Error() != want {
		t.Errorf("refusal =\n%q\nwant\n%q", err.Error(), want)
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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
	client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))

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
			client := guardedClient(t, &calls, http.StatusOK, storedMirrorJSON(false))
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
// project would pull from when the call names a URL. The URL is the caller's
// own parameter, so a credential written into its userinfo must not be echoed
// into a prompt, and a value that is not an absolute URL is withheld rather
// than shown. A call that names none is asked about the configured source,
// which the two refusal tests above pin.
func TestPullMirrorSourceForPrompt_Values(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
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

// updatedProjectJSON is the project PUT /projects/:id answers with in the
// update tests below: enough of one for the handler to build its output.
const updatedProjectJSON = `{"id":42,"name":"repo","path_with_namespace":"group/repo","web_url":"https://gitlab.example.com/group/repo"}`

// mirrorRead is what GitLab answers the guard's inspection read with: a
// status other than 200 expresses a project with no mirror, an instance that
// mounts no pull mirror route and an unreadable configuration, as guardHandler
// does for the dedicated route.
type mirrorRead struct {
	status int
	body   string
}

// The five answers the update tests below give the inspection read.
// readRouteNotMounted is GitLab CE's, whose API has no pull mirror route and
// answers it as Grape answers any path it does not route.
var (
	readPlainMirror       = mirrorRead{status: http.StatusOK, body: storedMirrorJSON(false)}
	readOverwritingMirror = mirrorRead{status: http.StatusOK, body: storedMirrorJSON(true)}
	readNotMirrored       = mirrorRead{status: http.StatusBadRequest, body: notMirroredJSON}
	readRouteNotMounted   = mirrorRead{status: http.StatusNotFound, body: `{"error":"404 Not Found"}`}
	readUnavailable       = mirrorRead{status: http.StatusInternalServerError, body: `{"message":"500 Internal Server Error"}`}
)

// readMirrorAt answers the inspection read with a mirror that pulls from
// source, for the cases that turn on whether a call names the source the
// mirror already has.
func readMirrorAt(overwrites bool, source string) mirrorRead {
	return mirrorRead{status: http.StatusOK, body: storedMirrorAt(overwrites, source)}
}

// updateGuardHandler answers the two requests a project update that touches
// the pull mirror can send: the read of the mirror configuration the guard
// inspects, answered with read, and the update itself.
func updateGuardHandler(calls *guardCalls, read mirrorRead) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == pathProject42MirrorPull:
			calls.reads.Add(1)
			testutil.RespondJSON(w, read.status, read.body)
		case r.Method == http.MethodPut && r.URL.Path == pathProject42:
			calls.writes.Add(1)
			testutil.RespondJSON(w, http.StatusOK, updatedProjectJSON)
		default:
			http.NotFound(w, r)
		}
	}
}

// updateGuardedClient builds a client whose mirror read and project update
// are served by updateGuardHandler.
func updateGuardedClient(t *testing.T, calls *guardCalls, read mirrorRead) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, updateGuardHandler(calls, read))
}

// armingUpdate is a project update that enables the mirror, aims it at
// mirrorSourceURL and arms the overwrite, the update every confirmation test
// below sends.
func armingUpdate() UpdateInput {
	return UpdateInput{
		ProjectID: "42", Mirror: new(true), ImportURL: mirrorSourceURL,
		MirrorOverwritesDivergedBranches: new(true),
	}
}

// TestUpdate_LeavingTheOverwriteArmed_IsRefusedWithoutConfirmation verifies
// that project.update is held to the rule project.pull_mirror_configure is
// held to. PUT /projects/:id takes mirror, import_url and
// mirror_overwrites_diverged_branches too, the same three settings under the
// names the project route gives them, so an update that leaves the project
// pulling with the overwrite armed, and is the call that arms or re-aims it,
// discards the project's own commits on each diverged branch the mirror pulls
// exactly as the dedicated route would. An update that sets the flag while it
// turns the mirror off, or leaves off a mirror the read reports as not
// mirrored, whether by sending mirror false or by not sending it, is asked too,
// with the question that says the overwrite waits for the mirror: GitLab keeps
// the flag and hides it until the mirror is on again, so the update that turns
// it on could not be asked about it (issue 1752). An update that sets the flag
// where the read fails for another reason cannot tell whether the mirror runs,
// and is asked the question of a mirror that does.
//
// Each case is refused when nothing confirmed it, with the guard's question in
// English and how to proceed, and GitLab is never asked to apply it. The
// inspection read is asserted as well, since a call that sends both flags
// already says what it leaves behind and must not pay for one.
//
// A URL naming the source the mirror already pulls from re-aims nothing, but
// it does not excuse a flag sent beside it: arming the overwrite, or re-sending
// mirror=true to an overwriting mirror, with that same URL still asks. A URL
// that differs from the source in its path's case, that GitLab would read past
// an @ after the host, or that either side gives in a form net/url refuses is
// not the same source, and asks.
func TestUpdate_LeavingTheOverwriteArmed_IsRefusedWithoutConfirmation(t *testing.T) {
	tests := []struct {
		name      string
		read      mirrorRead
		input     UpdateInput
		source    string
		off       bool
		wantReads int64
	}{
		{
			name:      "arming the flag on a running mirror",
			read:      readPlainMirror,
			input:     UpdateInput{ProjectID: "42", MirrorOverwritesDivergedBranches: new(true)},
			source:    configuredMirrorSource,
			wantReads: 1,
		},
		{
			name:      "arming the flag beside an unrelated setting",
			read:      readPlainMirror,
			input:     UpdateInput{ProjectID: "42", Description: new("mirrored"), MirrorOverwritesDivergedBranches: new(true)},
			source:    configuredMirrorSource,
			wantReads: 1,
		},
		{
			name:      "repointing a mirror that already overwrites",
			read:      readOverwritingMirror,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			source:    mirrorSourceURL,
			wantReads: 1,
		},
		{
			name:   "enabling a mirror together with the flag",
			read:   readNotMirrored,
			input:  armingUpdate(),
			source: mirrorSourceURL,
		},
		{
			name:      "arming the flag on a project with no mirror",
			read:      readNotMirrored,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL, MirrorOverwritesDivergedBranches: new(true)},
			source:    mirrorSourceURL,
			off:       true,
			wantReads: 1,
		},
		{
			name:      "setting the flag beside an unrelated setting on a mirror that is off",
			read:      readNotMirrored,
			input:     UpdateInput{ProjectID: "42", Description: new("mirrored"), MirrorOverwritesDivergedBranches: new(true)},
			source:    configuredMirrorSource,
			off:       true,
			wantReads: 1,
		},
		{
			name:      "arming the flag where no pull mirror route is mounted",
			read:      readRouteNotMounted,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL, MirrorOverwritesDivergedBranches: new(true)},
			source:    mirrorSourceURL,
			off:       true,
			wantReads: 1,
		},
		{
			name:      "a configuration the guard cannot read",
			read:      readUnavailable,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			source:    mirrorSourceURL,
			wantReads: 1,
		},
		{
			name:      "setting the flag where the configuration cannot be read",
			read:      readUnavailable,
			input:     UpdateInput{ProjectID: "42", Description: new("mirrored"), MirrorOverwritesDivergedBranches: new(true)},
			source:    configuredMirrorSource,
			wantReads: 1,
		},
		{
			name:      "arming the flag with the source the mirror already pulls from",
			read:      readMirrorAt(false, mirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL, MirrorOverwritesDivergedBranches: new(true)},
			source:    mirrorSourceURL,
			wantReads: 1,
		},
		{
			name:      "re-sending mirror=true to an overwriting mirror with the source it already has",
			read:      readMirrorAt(true, mirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", Mirror: new(true), ImportURL: mirrorSourceURL},
			source:    mirrorSourceURL,
			wantReads: 1,
		},
		{
			name:      "repointing to a path that differs only in case",
			read:      readMirrorAt(true, mirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", ImportURL: "https://github.com/Example/repo.git"},
			source:    "https://github.com/Example/repo.git",
			wantReads: 1,
		},
		{
			name:      "a source GitLab would read past its last @",
			read:      readMirrorAt(true, "https://github.com/example@evil.example/repo.git"),
			input:     UpdateInput{ProjectID: "42", ImportURL: "https://github.com/example@evil.example/repo.git"},
			source:    "https://github.com/example@evil.example/repo.git",
			wantReads: 1,
		},
		{
			name:      "a source this server cannot parse",
			read:      readMirrorAt(true, unparseableMirrorSource),
			input:     UpdateInput{ProjectID: "42", ImportURL: unparseableMirrorSource},
			source:    toolutil.RedactedPlaceholder,
			wantReads: 1,
		},
		{
			name:      "a reported source this server cannot parse",
			read:      readMirrorAt(true, unparseableMirrorSource),
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			source:    mirrorSourceURL,
			wantReads: 1,
		},
		{
			name:   "setting the flag while turning a running mirror off",
			read:   readPlainMirror,
			input:  UpdateInput{ProjectID: "42", Mirror: new(false), MirrorOverwritesDivergedBranches: new(true)},
			source: configuredMirrorSource,
			off:    true,
		},
		{
			name: "setting the flag and a source on a mirror that stays off",
			read: readNotMirrored,
			input: UpdateInput{
				ProjectID: "42", Mirror: new(false), ImportURL: mirrorSourceURL,
				MirrorOverwritesDivergedBranches: new(true),
			},
			source: mirrorSourceURL,
			off:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := updateGuardedClient(t, &calls, tt.read)

			_, err := Update(t.Context(), client, tt.input)
			if err == nil {
				t.Fatal("an update leaving the diverged-branch overwrite armed went out without confirmation")
			}
			want := mirrorRefusal(opProjectUpdate, tt.source)
			if tt.off {
				want = mirrorOffRefusal(opProjectUpdate, tt.source)
			}
			if err.Error() != want {
				t.Errorf("refusal =\n%q\nwant\n%q", err.Error(), want)
			}
			if got := calls.writes.Load(); got != 0 {
				t.Errorf("the update was applied %d time(s) despite being refused", got)
			}
			if got := calls.reads.Load(); got != tt.wantReads {
				t.Errorf("reads = %d, want %d", got, tt.wantReads)
			}
		})
	}
}

// TestUpdate_NotLeavingTheOverwriteArmed_ProceedsWithoutAsking verifies the
// other half of the rule on project.update: an update that cannot arm the
// overwrite, or that leaves it disarmed, goes out without a question, and the
// inspection read is spent only where the answer depends on what the project
// already holds. Most project updates name no mirror setting at all, and
// asking on the ones that switch the overwrite off, or the mirror without
// setting the overwrite, would be the guard working backwards.
//
// Turning a mirror that is off back on without the flag proceeds too, and so
// does aiming one at a source without either flag, which leaves it off. GitLab
// answers the read of such a mirror 400 not mirrored and so hides a flag it
// stores, which is why the update that sets the flag while the mirror is off is
// the one asked.
//
// The cases on an instance that mounts no pull mirror route are GitLab CE's,
// where the read is answered 404 and there is no mirror to inherit an overwrite
// from: refusing them would ask about a mirror the instance ignores. The cases
// that re-send an overwriting mirror the source it already pulls from re-aim
// nothing, whether the URL carries credentials GitLab reports masked or a host
// in other capitals, so they are the unrelated edit the guard lets through.
func TestUpdate_NotLeavingTheOverwriteArmed_ProceedsWithoutAsking(t *testing.T) {
	tests := []struct {
		name      string
		read      mirrorRead
		input     UpdateInput
		wantReads int64
	}{
		{
			name:  "an unrelated setting on a mirror that overwrites",
			read:  readOverwritingMirror,
			input: UpdateInput{ProjectID: "42", Description: new("renamed")},
		},
		{
			name: "mirror settings that cannot arm it",
			read: readOverwritingMirror,
			input: UpdateInput{
				ProjectID: "42", MirrorTriggerBuilds: new(true), OnlyMirrorProtectedBranches: new(true),
				MirrorUserID: new(int64(7)),
			},
		},
		{
			name:  "disabling the mirror",
			read:  readOverwritingMirror,
			input: UpdateInput{ProjectID: "42", Mirror: new(false), ImportURL: mirrorSourceURL},
		},
		{
			name: "disabling the mirror and clearing the flag",
			read: readOverwritingMirror,
			input: UpdateInput{
				ProjectID: "42", Mirror: new(false), ImportURL: mirrorSourceURL,
				MirrorOverwritesDivergedBranches: new(false),
			},
		},
		{
			name:  "clearing the flag",
			read:  readOverwritingMirror,
			input: UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL, MirrorOverwritesDivergedBranches: new(false)},
		},
		{
			name:      "turning a mirror that is off back on without the flag",
			read:      readNotMirrored,
			input:     UpdateInput{ProjectID: "42", Mirror: new(true)},
			wantReads: 1,
		},
		{
			name:      "aiming a mirror that is off at a source",
			read:      readNotMirrored,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "repointing a mirror that does not overwrite",
			read:      readPlainMirror,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "enabling a mirror that does not overwrite",
			read:      readPlainMirror,
			input:     UpdateInput{ProjectID: "42", Mirror: new(true)},
			wantReads: 1,
		},
		{
			name:      "a first mirror without the flag",
			read:      readNotMirrored,
			input:     UpdateInput{ProjectID: "42", Mirror: new(true), ImportURL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "an import_url where no pull mirror route is mounted",
			read:      readRouteNotMounted,
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "enabling the mirror where no pull mirror route is mounted",
			read:      readRouteNotMounted,
			input:     UpdateInput{ProjectID: "42", Mirror: new(true)},
			wantReads: 1,
		},
		{
			name:      "re-sending the source an overwriting mirror pulls from",
			read:      readMirrorAt(true, mirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", ImportURL: mirrorSourceURL},
			wantReads: 1,
		},
		{
			name:      "re-sending that source with the credentials GitLab reports masked",
			read:      readMirrorAt(true, maskedMirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", ImportURL: "https://user:token@github.com/example/repo.git"},
			wantReads: 1,
		},
		{
			name:      "re-sending that source with its host in capitals",
			read:      readMirrorAt(true, mirrorSourceURL),
			input:     UpdateInput{ProjectID: "42", ImportURL: "https://GitHub.com/example/repo.git"},
			wantReads: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := updateGuardedClient(t, &calls, tt.read)

			out, err := Update(t.Context(), client, tt.input)
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if out.ID != 42 {
				t.Errorf("ID = %d, want the updated project", out.ID)
			}
			if got := calls.writes.Load(); got != 1 {
				t.Errorf("writes = %d, want the update applied once", got)
			}
			if got := calls.reads.Load(); got != tt.wantReads {
				t.Errorf("reads = %d, want %d", got, tt.wantReads)
			}
		})
	}
}

// TestUpdate_ArmingTheOverwrite_ProceedsWhenConfirmed verifies the ways past
// the question on project.update, which are the dedicated route's: the
// reserved confirm key in each shape a surface sends it, flat on the
// individual tool, inside params on the meta tool and beside params on
// gitlab_execute_action, and the operator's bypass under either of its names.
// A confirmed call does not need to know what it is about to arm, so none of
// them pays for the inspection read.
//
// The confirmation is read from the raw tool call rather than from the typed
// input, because the reserved key is stripped before unmarshalling, so each
// request is built the way its surface sends it.
func TestUpdate_ArmingTheOverwrite_ProceedsWhenConfirmed(t *testing.T) {
	const fields = `"project_id":"42","mirror":true,"import_url":"` + mirrorSourceURL + `","mirror_overwrites_diverged_branches":true`
	tests := []struct {
		name      string
		env       map[string]string
		tool      string
		arguments string
	}{
		{name: "confirm on the individual tool", tool: toolProjectUpdate, arguments: `{` + fields + `,"confirm":true}`},
		{name: "confirm inside params on the meta tool", tool: "gitlab_project", arguments: `{"action":"update","params":{` + fields + `,"confirm":true}}`},
		{name: "confirm beside params on gitlab_execute_action", tool: "gitlab_execute_action", arguments: `{"action":"project.update","params":{` + fields + `},"confirm":true}`},
		{name: "YOLO mode", env: map[string]string{"GITLAB_MCP_YOLO_MODE": "true"}},
		{name: "AUTOPILOT", env: map[string]string{"AUTOPILOT": "true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GITLAB_MCP_YOLO_MODE", "")
			t.Setenv("AUTOPILOT", "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			var calls guardCalls
			client := updateGuardedClient(t, &calls, readPlainMirror)
			ctx := t.Context()
			if tt.tool != "" {
				ctx = toolutil.ContextWithRequest(ctx, &mcp.CallToolRequest{
					Params: &mcp.CallToolParamsRaw{Name: tt.tool, Arguments: json.RawMessage(tt.arguments)},
				})
			}

			if _, err := Update(ctx, client, armingUpdate()); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if got := calls.writes.Load(); got != 1 {
				t.Errorf("writes = %d, want the confirmed update applied once", got)
			}
			if got := calls.reads.Load(); got != 0 {
				t.Errorf("a confirmed update paid for %d inspection read(s)", got)
			}
		})
	}
}

// TestUpdate_ElicitingClient_TheAnswerDecidesTheCall verifies that a client
// which can prompt is asked about a project update that arms the overwrite,
// and that every outcome but an approval leaves the project untouched: the
// first pass returns the pending input request, an approval applies the update
// once, and a "no" in the form, a declined or dismissed dialog, an answer to
// another question and a confirmation state this server did not issue are each
// refused.
func TestUpdate_ElicitingClient_TheAnswerDecidesTheCall(t *testing.T) {
	answer := func(action string, content map[string]any) mcp.InputResponseMap {
		return mcp.InputResponseMap{pullMirrorOverwriteConfirmID: &mcp.ElicitResult{Action: action, Content: content}}
	}
	tests := []struct {
		name        string
		answers     mcp.InputResponseMap
		state       string
		wantPending bool
		wantError   string
		wantWrites  int64
	}{
		{name: "the first pass asks", wantPending: true},
		{name: "approved", answers: answer("accept", map[string]any{"confirmed": true}), wantWrites: 1},
		{
			name: "refused in the form", answers: answer("accept", map[string]any{"confirmed": false}),
			wantError: opProjectUpdate + ": the user declined overwriting the project's diverged branches; send mirror_overwrites_diverged_branches=false to mirror without replacing them",
		},
		{name: "declined", answers: answer("decline", nil), wantError: opProjectUpdate + ": confirmation failed: elicitation: user declined"},
		{name: "dismissed", answers: answer("cancel", nil), wantError: opProjectUpdate + ": confirmation failed: elicitation: user canceled"},
		{
			name: "an answer to another question", answers: answer("accept", map[string]any{"title": "unrelated"}),
			wantError: opProjectUpdate + ": confirmation failed: elicitation: the answer did not satisfy the requested schema",
		},
		{name: "a state this server did not issue", state: "not a state this server issued", wantError: mirrorRefusal(opProjectUpdate, mirrorSourceURL)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls guardCalls
			client := updateGuardedClient(t, &calls, readPlainMirror)
			req := elicitingRequest(tt.answers)
			req.Params.Name = toolProjectUpdate
			req.Params.RequestState = tt.state
			ctx := toolutil.ContextWithRequest(t.Context(), req)

			_, err := Update(ctx, client, armingUpdate())
			switch {
			case tt.wantPending:
				if _, pending := toolutil.InputRequiredResultFromError(err); !pending {
					t.Errorf("error = %v, want the pending input request", err)
				}
			case tt.wantError != "":
				if err == nil || err.Error() != tt.wantError {
					t.Errorf("error = %v, want %q", err, tt.wantError)
				}
			case err != nil:
				t.Errorf(fmtUnexpErr, err)
			}
			if got := calls.writes.Load(); got != tt.wantWrites {
				t.Errorf("writes = %d, want %d", got, tt.wantWrites)
			}
		})
	}
}

// TestUpdate_ThroughTheRoute_HonorsTheReservedConfirmKey drives the path every
// surface takes to project.update, the route's own handler, given the
// parameters as a map with the reserved key among them and the raw call in the
// context, and checks that the input schema advertises the key.
//
// The schema matters on the individual surface, where it closes the object
// with additionalProperties: false and a client that validates before sending
// would refuse a confirm it does not declare. The unconfirmed half is asserted
// beside the confirmed one, since a key that is accepted but never consulted
// would pass the first half alone.
func TestUpdate_ThroughTheRoute_HonorsTheReservedConfirmKey(t *testing.T) {
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
			client := updateGuardedClient(t, &calls, readPlainMirror)
			route := projectSpecsByTool(t, ActionSpecs(client, true))[toolProjectUpdate].Route
			props, _ := route.InputSchema["properties"].(map[string]any)
			if _, declared := props["confirm"]; !declared {
				t.Error("the project.update input schema does not declare the reserved confirm key")
			}

			params := map[string]any{
				"project_id": "42", "mirror": true, "import_url": mirrorSourceURL,
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
				Params: &mcp.CallToolParamsRaw{Name: toolProjectUpdate, Arguments: arguments},
			})

			_, err := route.Handler(ctx, params)
			if tt.confirm && err != nil {
				t.Fatalf("a confirmed update was refused: %v", err)
			}
			if !tt.confirm {
				if err == nil {
					t.Fatal("an unconfirmed update arming the overwrite was accepted through the route")
				}
				assertRefusalMentions(t, err, opProjectUpdate+":", "confirm=true")
			}
			if got := calls.writes.Load(); got != tt.wantWrite {
				t.Errorf("writes = %d, want %d", got, tt.wantWrite)
			}
		})
	}
}
