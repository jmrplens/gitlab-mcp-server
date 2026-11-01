// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// protected_env_test.go drives the protection builders' pure halves against
// the stub, at both scopes: what a protection sends with and without an
// approver, the two endings an unprotection has, and what the group read-back
// answers.

package fixture

import (
	"fmt"
	"net/http"
	"slices"
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

// TestNewGroupProtectedEnvironment_Detached_ProtectsADeploymentTier drives the
// builder whole and holds the name it protects to what a group protection
// takes: a deployment tier, which GitLab 19.5 refuses anything else for with
// 400 "name does not have a valid value". The builder protected a generated
// name until 2026-11-01, and the model evaluation's world for the group
// unprotect case stopped on that refusal.
func TestNewGroupProtectedEnvironment_Detached_ProtectsADeploymentTier(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/groups/2/protected_environments", stubCreated(map[string]any{"name": "staging"}))
	stub.answers(http.MethodDelete, "/api/v4/groups/2/protected_environments/staging", stubNoContent())

	got := NewGroupProtectedEnvironment(e, Group{ID: 2})
	tiers := []string{"production", "staging", "testing", "development", "other"}
	requests := stub.recordedRequests()
	if len(requests) != 1 || !slices.Contains(tiers, fmt.Sprint(requests[0].Body["name"])) || got.Name != "staging" {
		t.Errorf("NewGroupProtectedEnvironment() = %+v after sending %v, want one protection of a deployment tier (%v)", got, requests, tiers)
	}
}

// TestNewProtectedEnvironment_Detached_ProtectsTheEnvironmentForItsApprover
// drives the project builder whole: the environment it was given is
// protected with the deploy access level and the approver's rule, handed back
// as GitLab answered, and unprotected when the test ends.
func TestNewProtectedEnvironment_Detached_ProtectsTheEnvironmentForItsApprover(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/protected_environments", stubCreated(map[string]any{"name": "env-run"}))
	stub.answers(http.MethodDelete, "/api/v4/projects/2/protected_environments/env-run", stubNoContent())

	got := NewProtectedEnvironment(e, Project{ID: 2}, Environment{ID: 5, Name: "env-run"}, 17)

	sent := requestTo(t, stub, http.MethodPost, "/api/v4/projects/2/protected_environments").Body
	if got.Name != "env-run" || sent["name"] != "env-run" || sent["deploy_access_levels"] == nil || sent["approval_rules"] == nil {
		t.Errorf("NewProtectedEnvironment() = %+v after sending %v, want env-run protected with its deploy access and approval rule", got, sent)
	}
}

// TestNewDeploymentApproval_Detached_BuildsTheWholeApprovalWorld drives the
// approval builder whole: an environment, its protection, a commit and a
// deployment of that commit into the environment, each asked for in the
// project it was given and handed back together.
func TestNewDeploymentApproval_Detached_BuildsTheWholeApprovalWorld(t *testing.T) {
	stub, e := detachedStub(t)
	stub.answers(http.MethodPost, "/api/v4/projects/2/environments", stubCreated(map[string]any{"id": 5, "name": "approval-run"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/protected_environments", stubCreated(map[string]any{"name": "approval-run"}))
	stub.answers(http.MethodDelete, "/api/v4/projects/2/protected_environments/approval-run", stubNoContent())
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/commits", stubCreated(map[string]any{"id": "abc123", "short_id": "abc123"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/deployments", stubCreated(map[string]any{"id": 6, "sha": "abc123", "status": "running"}))

	got := NewDeploymentApproval(e, Project{ID: 2, DefaultBranch: "main"})

	environment := Environment{ID: 5, Name: "approval-run"}
	want := DeploymentApproval{
		Environment: environment,
		Protected:   ProtectedEnvironment{Name: "approval-run"},
		Deployment:  Deployment{ID: 6, Environment: environment, SHA: "abc123", Status: "running"},
	}
	if got != want {
		t.Errorf("NewDeploymentApproval() = %+v, want %+v", got, want)
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
