//go:build e2e

// service_account_test.go drives the service account builders' pure halves
// against the stub, at both scopes: what a create sends, that the token is
// minted beside the account and reported with it, and what happens when GitLab
// takes the account and refuses the token.

package fixture

import (
	"context"
	"net/http"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// TestCreateProjectServiceAccount_Created_MintsATokenBesideTheAccount checks
// that the builder creates the account, mints its token and reports both.
func TestCreateProjectServiceAccount_Created_MintsATokenBesideTheAccount(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/7/service_accounts", stubCreated(map[string]any{
		"id": 41, "username": "svc-bot", "name": "svc-bot",
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/7/service_accounts/41/personal_access_tokens",
		stubCreated(map[string]any{"id": 88, "name": "svc-bot", "token": "glpat-stub"}))

	got, err := createProjectServiceAccount(t.Context(), client, 7, "svc-bot")
	if err != nil {
		t.Fatalf("createProjectServiceAccount() error = %v, want nil", err)
	}
	want := ServiceAccount{ID: 41, Username: "svc-bot", TokenID: 88}
	if got != want {
		t.Errorf("createProjectServiceAccount() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 2 {
		t.Fatalf("createProjectServiceAccount() sent %d requests, want the account and its token", len(requests))
	}
	if requests[0].Body["username"] != "svc-bot" {
		t.Errorf("the account create sent %v, want the reserved username", requests[0].Body)
	}
	if requests[1].Body["scopes"] == nil {
		t.Errorf("the token create sent %v, want the scopes the fixture mints with", requests[1].Body)
	}
}

// TestCreateProjectServiceAccount_TokenRefused_IsAnError checks that an
// account whose token GitLab refused is a failure rather than an account with
// a zero token, which a case would then ask about and be answered a not-found.
func TestCreateProjectServiceAccount_TokenRefused_IsAnError(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/projects/7/service_accounts", stubCreated(map[string]any{
		"id": 41, "username": "svc-bot",
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/7/service_accounts/41/personal_access_tokens",
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	if _, err := createProjectServiceAccount(t.Context(), client, 7, "svc-bot"); err == nil {
		t.Error("createProjectServiceAccount() error = nil, want the token refusal")
	}
}

// TestCreateGroupServiceAccount_Created_ReadsGitLabsOwnSpelling checks the
// group half, whose answer spells the username under a different Go field
// than the project half does.
func TestCreateGroupServiceAccount_Created_ReadsGitLabsOwnSpelling(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/3/service_accounts", stubCreated(map[string]any{
		"id": 12, "username": "group-bot", "name": "group-bot",
	}))
	stub.answers(http.MethodPost, "/api/v4/groups/3/service_accounts/12/personal_access_tokens",
		stubCreated(map[string]any{"id": 34, "name": "group-bot", "token": "glpat-stub"}))

	got, err := createGroupServiceAccount(t.Context(), client, 3, "group-bot")
	if err != nil {
		t.Fatalf("createGroupServiceAccount() error = %v, want nil", err)
	}
	want := ServiceAccount{ID: 12, Username: "group-bot", TokenID: 34}
	if got != want {
		t.Errorf("createGroupServiceAccount() = %+v, want %+v", got, want)
	}
}

// TestDeleteServiceAccount_Endings_ToleratesOneACaseDeleted covers both
// scopes' removals: an account a case already deleted is not a cleanup
// failure and any other refusal is.
func TestDeleteServiceAccount_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		remove  func(context.Context, *gitlabclient.Client) error
		answer  scriptedAnswer
		wantErr bool
	}{
		{
			name: "project deleted", path: "/api/v4/projects/7/service_accounts/41", answer: stubNoContent(),
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteProjectServiceAccount(ctx, client, 7, 41)
			},
		},
		{
			name: "project already gone", path: "/api/v4/projects/7/service_accounts/41",
			answer: stubRefusal(http.StatusNotFound, "404 Not found"),
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteProjectServiceAccount(ctx, client, 7, 41)
			},
		},
		{
			name: "project refused", path: "/api/v4/projects/7/service_accounts/41",
			answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true,
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteProjectServiceAccount(ctx, client, 7, 41)
			},
		},
		{
			name: "group deleted", path: "/api/v4/groups/3/service_accounts/12", answer: stubNoContent(),
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteGroupServiceAccount(ctx, client, 3, 12)
			},
		},
		{
			name: "group already gone", path: "/api/v4/groups/3/service_accounts/12",
			answer: stubRefusal(http.StatusNotFound, "404 Not found"),
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteGroupServiceAccount(ctx, client, 3, 12)
			},
		},
		{
			name: "group refused", path: "/api/v4/groups/3/service_accounts/12",
			answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true,
			remove: func(ctx context.Context, client *gitlabclient.Client) error {
				return deleteGroupServiceAccount(ctx, client, 3, 12)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, tc.path, tc.answer)

			err := tc.remove(t.Context(), client)
			if (err != nil) != tc.wantErr {
				t.Errorf("the removal returned %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
