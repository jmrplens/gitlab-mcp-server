// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// billingRefusal is the body GitLab refuses a graph query with, 422, for a
// user who has chosen no default group for Orbit billing: the shape
// GoverningNamespaceError writes and doc/api/orbit.md documents.
const billingRefusal = `{"code":"billing_namespace_required","message":"Select a default group in your preferences to run GitLab Orbit queries.","deep_link":"https://gitlab.com/-/profile/preferences","candidates":[{"id":10,"name":"acme"},{"id":11,"name":"acme-tools"}]}`

// TestQueries_GitLabAsksForABillingGroup_TheCallerIsToldWhere verifies that
// the 422 GitLab refuses a graph query with reaches the caller as what it is,
// on both routes that run one: GitLab's message, where the group is chosen and
// the groups to choose from, and never as a fault in the query or its
// parameters.
func TestQueries_GitLabAsksForABillingGroup_TheCallerIsToldWhere(t *testing.T) {
	cases := []struct {
		name string
		call func(context.Context, *testClient) error
	}{
		{name: "named query", call: func(ctx context.Context, c *testClient) error {
			_, err := NamedQuery(ctx, c, NamedQueryInput{Name: "recent_merges"})
			return err
		}},
		{name: "query", call: func(ctx context.Context, c *testClient) error {
			_, err := Query(ctx, c, QueryInput{Query: projectByIDGQL})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, billingRefusal)
			}))
			err := tc.call(context.Background(), client)
			if !toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
				t.Fatalf("error = %v, want GitLab's 422", err)
			}
			if lacks := missingFrom(
				err.Error(),
				"GitLab answered billing_namespace_required: Select a default group in your preferences to run GitLab Orbit queries.",
				"Chosen at https://gitlab.com/-/profile/preferences",
				"Groups to choose from: acme (10), acme-tools (11)",
				billingNamespaceHint,
			); len(lacks) > 0 {
				t.Errorf("error = %v, lacks %q", err, lacks)
			}
			if strings.Contains(err.Error(), namedQueryRefusedHint) || strings.Contains(err.Error(), queryRefusedHint) {
				t.Errorf("error = %v, blames the query for a group the user has not chosen", err)
			}
		})
	}
}

// TestBillingNamespaceRefusal_OnlyGitLabsShape_IsRead holds the reading of the
// 422 to GitLab's shape: a key more, another code, a body that is no JSON and
// an error with no answer are not reflected, and the hint alone follows them.
// What GitLab leaves out is left out, an address a browser does not open with
// it, and a list of groups long enough to bury the rest is cut where every
// reflected message is.
func TestBillingNamespaceRefusal_OnlyGitLabsShape_IsRead(t *testing.T) {
	groups := strings.TrimSuffix(strings.Repeat(`{"id":1,"name":"group"},`, 1000), ",")
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			name: "no link and no candidate", body: `{"code":"billing_namespace_required","message":"Select a group.","deep_link":"","candidates":[]}`,
			want: "GitLab answered billing_namespace_required: Select a group.",
		},
		{
			name: "a link no browser opens", body: `{"code":"billing_namespace_required","message":"Select a group.","deep_link":"javascript:alert(1)","candidates":[{"id":3,"name":"a\nb"}]}`,
			want: "GitLab answered billing_namespace_required: Select a group. Groups to choose from: a b (3)",
		},
		{name: "a key more", body: `{"code":"billing_namespace_required","message":"x","upstream":"web-03"}`},
		{name: "another code", body: `{"code":"internal_error","message":"x"}`},
		{name: "not JSON", body: `<html>bad gateway</html>`},
		{name: "GitLab's message shape", body: `{"message":"422 Unprocessable entity"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := unprocessable(t, tc.body)
			if got := billingNamespaceRefusal(err); got != tc.want {
				t.Errorf("billingNamespaceRefusal() = %q, want %q", got, tc.want)
			}
			if !strings.Contains(err.Error(), billingNamespaceHint) {
				t.Errorf("error = %v, want the billing group named whatever answered 422", err)
			}
		})
	}
	t.Run("a list that buries the rest", func(t *testing.T) {
		got := billingNamespaceRefusal(unprocessable(t, `{"code":"billing_namespace_required","message":"Select a group.","deep_link":"","candidates":[`+groups+`]}`))
		if len(got) > 3000 || !strings.HasSuffix(got, "...") {
			t.Errorf("billingNamespaceRefusal() is %d bytes ending %q, want it cut", len(got), got[len(got)-10:])
		}
	})
	t.Run("an error with no answer", func(t *testing.T) {
		if got := billingNamespaceRefusal(errors.New("dial tcp: refused")); got != "" {
			t.Errorf("billingNamespaceRefusal() = %q, want none", got)
		}
	})
}

// unprocessable returns the error a named query meets when GitLab answers 422
// with body.
func unprocessable(t *testing.T, body string) error {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, body)
	}))
	_, err := NamedQuery(context.Background(), client, NamedQueryInput{Name: "recent_merges"})
	if err == nil {
		t.Fatal("NamedQuery() error = nil, want GitLab's 422")
	}
	return err
}
