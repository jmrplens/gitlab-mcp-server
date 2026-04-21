package groupscim

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/scim/identities" {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"external_uid":"ext-1","user_id":10,"active":true},
				{"external_uid":"ext-2","user_id":20,"active":false}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(out.Identities) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(out.Identities))
	}
	if out.Identities[0].ExternalUID != "ext-1" {
		t.Errorf("expected external_uid ext-1, got %s", out.Identities[0].ExternalUID)
	}
	if out.Identities[1].UserID != 20 {
		t.Errorf("expected user_id 20, got %d", out.Identities[1].UserID)
	}
}

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := List(ctx, client, ListInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestList_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/identities" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := List(context.Background(), client, ListInput{
		GroupID: toolutil.StringOrInt("mygroup"),
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusOK, `{"external_uid":"uid-123","user_id":42,"active":true}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if out.ExternalUID != "uid-123" {
		t.Errorf("expected external_uid uid-123, got %s", out.ExternalUID)
	}
	if out.UserID != 42 {
		t.Errorf("expected user_id 42, got %d", out.UserID)
	}
}

func TestGet_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestGet_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	_, err := Get(context.Background(), client, GetInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

func TestGet_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Get(ctx, client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestGet_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	_, err := Get(context.Background(), client, GetInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new-ext-uid",
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
}

func TestUpdate_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{UID: "uid-123", ExternUID: "new"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestUpdate_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		ExternUID: "new",
	})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

func TestUpdate_MissingExternUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for empty extern_uid, got nil")
	}
}

func TestUpdate_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Update(ctx, client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestUpdate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Update(context.Background(), client, UpdateInput{
		GroupID:   toolutil.StringOrInt("mygroup"),
		UID:       "uid-123",
		ExternUID: "new",
	})
	if err == nil {
		t.Fatal("expected error for 403 response, got nil")
	}
}

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestDelete_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{UID: "uid-123"})
	if err == nil {
		t.Fatal("expected error for empty group_id, got nil")
	}
}

func TestDelete_MissingUID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: toolutil.StringOrInt("mygroup")})
	if err == nil {
		t.Fatal("expected error for empty uid, got nil")
	}
}

func TestDelete_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := Delete(ctx, client, DeleteInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestDelete_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/mygroup/scim/uid-123" {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"Bad Request"}`)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{
		GroupID: toolutil.StringOrInt("mygroup"),
		UID:     "uid-123",
	})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}
