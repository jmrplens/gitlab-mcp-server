package users

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

func TestGetUserActivities_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/user/activities" {
			testutil.RespondJSON(w, http.StatusOK, `[{"username":"testuser","last_activity_on":"2026-06-01"}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserActivities(context.Background(), client, GetUserActivitiesInput{})
	if err != nil {
		t.Fatalf("GetUserActivities() unexpected error: %v", err)
	}
	if len(out.Activities) != 1 {
		t.Fatalf("len(out.Activities) = %d, want 1", len(out.Activities))
	}
}

func TestGetUserMemberships_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/42/memberships" {
			testutil.RespondJSON(w, http.StatusOK, `[{"source_id":1,"source_name":"my-project","source_type":"Project","access_level":30}]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 42})
	if err != nil {
		t.Fatalf("GetUserMemberships() unexpected error: %v", err)
	}
	if len(out.Memberships) != 1 {
		t.Fatalf("len(out.Memberships) = %d, want 1", len(out.Memberships))
	}
	if out.Memberships[0].SourceName != "my-project" {
		t.Errorf("out.Memberships[0].SourceName = %q, want %q", out.Memberships[0].SourceName, "my-project")
	}
}

func TestGetUserMemberships_InvalidUserID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	_, err := GetUserMemberships(context.Background(), client, GetUserMembershipsInput{UserID: 0})
	if err == nil {
		t.Fatal("expected error for invalid user_id, got nil")
	}
}

func TestDeleteUserIdentity_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/users/42/identities/ldap" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := DeleteUserIdentity(context.Background(), client, DeleteUserIdentityInput{UserID: 42, Provider: "ldap"})
	if err != nil {
		t.Fatalf("DeleteUserIdentity() unexpected error: %v", err)
	}
	if !out.Deleted {
		t.Error("out.Deleted = false, want true")
	}
}
