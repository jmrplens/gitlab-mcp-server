package groupsaml

import (
	"context"
	"net/http"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathGroupSAML    = "/api/v4/groups/mygroup/saml_group_links"
	pathGroupSAMLOne = "/api/v4/groups/mygroup/saml_group_links/saml-devs"
)

func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupSAML {
			testutil.RespondJSON(w, http.StatusOK, `[
				{"name":"saml-devs","access_level":30,"member_role_id":0,"provider":""}
			]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(out.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(out.Links))
	}
	if out.Links[0].Name != "saml-devs" {
		t.Errorf("Name = %q, want %q", out.Links[0].Name, "saml-devs")
	}
}

func TestList_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := List(context.Background(), client, ListInput{})
	if err == nil {
		t.Fatal("List() expected error for missing group_id, got nil")
	}
}

func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == pathGroupSAMLOne {
			testutil.RespondJSON(w, http.StatusOK, `{"name":"saml-devs","access_level":30}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(context.Background(), client, GetInput{GroupID: "mygroup", SAMLGroupName: "saml-devs"})
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if out.Name != "saml-devs" {
		t.Errorf("Name = %q, want %q", out.Name, "saml-devs")
	}
}

func TestGet_MissingSAMLGroupName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Get(context.Background(), client, GetInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Get() expected error for missing saml_group_name, got nil")
	}
}

func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == pathGroupSAML {
			testutil.RespondJSON(w, http.StatusCreated, `{"name":"saml-devs","access_level":30}`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Add(context.Background(), client, AddInput{
		GroupID:       "mygroup",
		SAMLGroupName: "saml-devs",
		AccessLevel:   30,
	})
	if err != nil {
		t.Fatalf("Add() unexpected error: %v", err)
	}
	if out.Name != "saml-devs" {
		t.Errorf("Name = %q, want %q", out.Name, "saml-devs")
	}
}

func TestAdd_MissingGroupID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Add(context.Background(), client, AddInput{SAMLGroupName: "saml-devs", AccessLevel: 30})
	if err == nil {
		t.Fatal("Add() expected error for missing group_id, got nil")
	}
}

func TestAdd_MissingSAMLGroupName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	_, err := Add(context.Background(), client, AddInput{GroupID: "mygroup", AccessLevel: 30})
	if err == nil {
		t.Fatal("Add() expected error for missing saml_group_name, got nil")
	}
}

func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == pathGroupSAMLOne {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(context.Background(), client, DeleteInput{GroupID: "mygroup", SAMLGroupName: "saml-devs"})
	if err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
}

func TestDelete_MissingSAMLGroupName(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	err := Delete(context.Background(), client, DeleteInput{GroupID: "mygroup"})
	if err == nil {
		t.Fatal("Delete() expected error for missing saml_group_name, got nil")
	}
}
