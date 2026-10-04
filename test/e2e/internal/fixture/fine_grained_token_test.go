//go:build e2e

// fine_grained_token_test.go drives the fine-grained token builder's pure
// half against the stub: what a creation sends, what it reads back, and the
// two answers it refuses because a scenario on them would test the wrong
// credential.

package fixture

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestCreateFineGrainedToken_Created_SendsTheGrantAndReadsTheToken checks that
// the creation carries every scope the test wrote, with its access level, its
// permissions and the namespaces it names, and carries no classic scopes,
// since GitLab takes one or the other.
func TestCreateFineGrainedToken_Created_SendsTheGrantAndReadsTheToken(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/user/personal_access_tokens", stubCreated(map[string]any{
		"id": 52, "name": "e2e-fgtok", "token": "glpat-granular", "granular": true, "scopes": []string{"granular"},
	}))

	scopes := append(StartupScopes(), GranularScope{
		Access: AccessSelectedMemberships, Permissions: []string{"read_project"}, ProjectIDs: []int64{7},
	})
	got, err := createFineGrainedToken(t.Context(), client, fineGrainedTokenRequest{Name: "e2e-fgtok", ExpiresAt: "2026-12-31", GranularScopes: scopes})
	if err != nil {
		t.Fatalf("createFineGrainedToken() error = %v, want nil", err)
	}
	if want := (Token{ID: 52, Value: "glpat-granular", Name: "e2e-fgtok"}); got != want {
		t.Errorf("createFineGrainedToken() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createFineGrainedToken() sent %d requests, want 1", len(requests))
	}
	body := requests[0].Body
	if _, sent := body["scopes"]; sent {
		t.Errorf("the creation carried classic scopes %v beside the grant", body["scopes"])
	}
	if body["expires_at"] != "2026-12-31" || body["name"] != "e2e-fgtok" {
		t.Errorf("the creation named %v expiring %v", body["name"], body["expires_at"])
	}
	sent, _ := body["granular_scopes"].([]any)
	if len(sent) != len(scopes) {
		t.Fatalf("the creation carried %d scopes, want the %d the test wrote: %v", len(sent), len(scopes), body["granular_scopes"])
	}
	last, _ := sent[len(sent)-1].(map[string]any)
	if last["access"] != AccessSelectedMemberships || !slices.Equal(last["project_ids"].([]any), []any{float64(7)}) {
		t.Errorf("the selected scope was sent as %v", last)
	}
	if _, named := last["group_ids"]; named {
		t.Errorf("a scope naming no group sent group_ids: %v", last)
	}
	first, _ := sent[0].(map[string]any)
	if first["access"] != AccessUser || len(first["permissions"].([]any)) != 3 {
		t.Errorf("the user scope was sent as %v", first)
	}
}

// TestCreateFineGrainedToken_WrongAnswers_AreRefused checks the answers the
// builder does not hand a scenario: a refusal, a token with no value, and a
// token GitLab does not report as fine-grained, which a scenario would test
// as a classic token and pass.
func TestCreateFineGrainedToken_WrongAnswers_AreRefused(t *testing.T) {
	cases := []struct {
		name   string
		answer scriptedAnswer
	}{
		{name: "refused", answer: stubRefusal(http.StatusBadRequest, "400 Bad request - granular_scopes is invalid")},
		{name: "no value", answer: stubCreated(map[string]any{"id": 52, "name": "e2e-fgtok", "granular": true})},
		{name: "not granular", answer: stubCreated(map[string]any{"id": 52, "name": "e2e-fgtok", "token": "glpat-classic", "granular": false})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPost, "/api/v4/user/personal_access_tokens", tc.answer)

			if got, err := createFineGrainedToken(t.Context(), client, fineGrainedTokenRequest{Name: "e2e-fgtok", GranularScopes: StartupScopes()}); err == nil {
				t.Errorf("createFineGrainedToken() = %+v, want an error", got)
			}
		})
	}
}

// TestWaitForGrantedAccess_WaitsUntilEveryNamespaceIsSeen checks the wait a
// creation holds behind: it returns once the user's credential reads every
// project and group the scopes name, and says which one it never saw.
func TestWaitForGrantedAccess_WaitsUntilEveryNamespaceIsSeen(t *testing.T) {
	scopes := []GranularScope{
		{Access: AccessUser, Permissions: []string{"read_user"}},
		{Access: AccessSelectedMemberships, Permissions: []string{"read_project"}, ProjectIDs: []int64{7}, GroupIDs: []int64{9}},
	}
	cases := []struct {
		name             string
		projects, groups []int64
		wantErr          string
	}{
		{name: "both seen", projects: []int64{7}, groups: []int64{9}},
		{name: "the project not yet", groups: []int64{9}, wantErr: "project 7"},
		{name: "the group not yet", projects: []int64{7}, wantErr: "group 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			for _, id := range tc.projects {
				stub.projects[id] = &stubObject{ID: id, Name: "p", Path: "g/p"}
			}
			for _, id := range tc.groups {
				stub.groups[id] = &stubObject{ID: id, Name: "g", Path: "g"}
			}

			err := waitForGrantedAccess(t.Context(), client, scopes, 50*time.Millisecond)
			if tc.wantErr == "" && err != nil {
				t.Errorf("waitForGrantedAccess() = %v, want nil once both are seen", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("waitForGrantedAccess() = %v, want a timeout naming %s", err, tc.wantErr)
			}
		})
	}
}

// TestStartupScopes_AreTheFourTheServerStartsOn pins the grant a session needs
// to start in phase B, by the names GitLab assigns: changing one changes what
// every fine-grained scenario's server can read about itself.
func TestStartupScopes_AreTheFourTheServerStartsOn(t *testing.T) {
	got := StartupScopes()
	want := []GranularScope{
		{Access: AccessUser, Permissions: []string{"read_user", "read_namespace", "read_personal_access_token"}},
		{Access: AccessInstance, Permissions: []string{"read_metadata"}},
	}
	if len(got) != len(want) {
		t.Fatalf("StartupScopes() = %+v, want %+v", got, want)
	}
	for i := range want {
		t.Run(want[i].Access, func(t *testing.T) {
			if got[i].Access != want[i].Access || !slices.Equal(got[i].Permissions, want[i].Permissions) || got[i].ProjectIDs != nil || got[i].GroupIDs != nil {
				t.Errorf("scope %d = %+v, want %+v", i, got[i], want[i])
			}
		})
	}
	got[0].Permissions[0] = "changed"
	if StartupScopes()[0].Permissions[0] != "read_user" {
		t.Error("StartupScopes() hands every caller the same slices, so one scenario can change another's grant")
	}
}
