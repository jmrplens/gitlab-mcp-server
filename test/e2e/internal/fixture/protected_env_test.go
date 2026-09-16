//go:build e2e

// protected_env_test.go drives the protection builders' pure halves against
// the stub, at both scopes: what a protection sends with and without an
// approver, the two endings an unprotection has, and what the group read-back
// answers.

package fixture

import (
	"net/http"
	"testing"
)

// TestProtectEnvironment_ApprovalRule_IsSentOnlyWhenAnApproverIsNamed pins the
// one decision the project-scoped protection makes: an approval rule is what
// holds a deployment back, and a world that wanted none must not send one.
func TestProtectEnvironment_ApprovalRule_IsSentOnlyWhenAnApproverIsNamed(t *testing.T) {
	cases := []struct {
		name         string
		approver     int64
		wantApproval bool
	}{
		{name: "with an approver", approver: 17, wantApproval: true},
		{name: "without one", approver: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodPost, "/api/v4/projects/8/protected_environments", stubCreated(map[string]any{
				"name": "production",
			}))

			got, err := protectEnvironment(t.Context(), client, 8, "production", tc.approver)
			if err != nil {
				t.Fatalf("protectEnvironment() error = %v, want nil", err)
			}
			if got.Name != "production" {
				t.Errorf("protectEnvironment() = %+v, want the protected environment name", got)
			}

			requests := stub.recordedRequests()
			if len(requests) != 1 {
				t.Fatalf("protectEnvironment() sent %d requests, want 1", len(requests))
			}
			_, sentApproval := requests[0].Body["approval_rules"]
			if sentApproval != tc.wantApproval {
				t.Errorf("protectEnvironment() sent %v, want approval_rules present = %t", requests[0].Body, tc.wantApproval)
			}
		})
	}
}

// TestUnprotectEnvironment_Endings_ToleratesOneACaseLifted covers both scopes'
// unprotections: a protection a case already lifted is not a cleanup failure
// and any other refusal is.
func TestUnprotectEnvironment_Endings_ToleratesOneACaseLifted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "lifted", answer: stubNoContent()},
		{name: "already lifted", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run("project/"+tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/projects/8/protected_environments/production", tc.answer)

			err := unprotectEnvironment(t.Context(), client, 8, "production")
			if (err != nil) != tc.wantErr {
				t.Errorf("unprotectEnvironment() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
		t.Run("group/"+tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/groups/2/protected_environments/staging", tc.answer)

			err := unprotectGroupEnvironment(t.Context(), client, 2, "staging")
			if (err != nil) != tc.wantErr {
				t.Errorf("unprotectGroupEnvironment() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestProtectGroupEnvironment_Created_SendsTheDeployAccess checks that the
// group protection carries the access level GitLab demands with the name.
func TestProtectGroupEnvironment_Created_SendsTheDeployAccess(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/2/protected_environments", stubCreated(map[string]any{
		"name": "staging",
	}))

	got, err := protectGroupEnvironment(t.Context(), client, 2, "staging")
	if err != nil {
		t.Fatalf("protectGroupEnvironment() error = %v, want nil", err)
	}
	if got.Name != "staging" {
		t.Errorf("protectGroupEnvironment() = %+v, want the protected environment name", got)
	}
	requests := stub.recordedRequests()
	if len(requests) != 1 || requests[0].Body["deploy_access_levels"] == nil {
		t.Errorf("protectGroupEnvironment() sent %v, want a deploy access level", requests)
	}
}

// TestGroupEnvironmentIsProtected_Answers covers the three answers the
// read-back distinguishes.
func TestGroupEnvironmentIsProtected_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "protected", answer: stubOK(map[string]any{"name": "staging"}), want: true},
		{name: "lifted", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/groups/2/protected_environments/staging", tc.answer)

			got, err := GroupEnvironmentIsProtected(t.Context(), client, 2, "staging")
			if (err != nil) != tc.wantErr {
				t.Fatalf("GroupEnvironmentIsProtected() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GroupEnvironmentIsProtected() = %t, want %t", got, tc.want)
			}
		})
	}
}
