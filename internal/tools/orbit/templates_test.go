// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// TestQueryTemplates_GitLabsAnswer_IsPublishedTemplateByTemplate holds
// [QueryTemplates] to the answer GitLab.com gave on 2026-10-29: one document
// asking for the name, description and GQL text of each template, and each
// template published as it came, the text with its line breaks and its quotes.
// The answer is two of GitLab's five templates by name, with descriptions and
// statements this project wrote, which have nothing to do with what GitLab's
// templates of those names ask.
func TestQueryTemplates_GitLabsAnswer_IsPublishedTemplateByTemplate(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := readGraphQL(t, r)
		if !strings.Contains(req.Query, "queryTemplates") || len(req.Variables) != 0 {
			t.Errorf("GraphQL request = %+v, want the queryTemplates document with no variables", req)
		}
		testutil.RespondGraphQL(w, http.StatusOK, readFixture(t, "query_templates.json"))
	}))
	out, err := QueryTemplates(context.Background(), client, QueryTemplatesInput{})
	if err != nil {
		t.Fatalf("QueryTemplates() error: %v", err)
	}
	want := []QueryTemplate{
		{Name: "my_neighbors", Description: "One project and its path", Query: "MATCH (p:Project {id: 1})\nRETURN p.id, p.full_path"},
		{Name: "mrs_fixing_vulnerabilities", Description: "Labels of one title", Query: "MATCH (l:Label {title: \"stock\"})\nRETURN l.id, l.color\nLIMIT 5"},
	}
	if !reflect.DeepEqual(out.Templates, want) {
		t.Errorf("QueryTemplates() = %+v, want %+v", out.Templates, want)
	}
}

// TestQueryTemplates_NullOrEmptyAnswers_AreToldApart holds the two answers
// GitLab gives a list it has nothing in apart: queryTemplates null, which is
// how GitLab answers a user who cannot use Orbit, is an error wrapping
// [ErrOrbitUnavailable], and an empty list is an empty list, never nil.
func TestQueryTemplates_NullOrEmptyAnswers_AreToldApart(t *testing.T) {
	cases := []struct {
		name        string
		answer      string
		unavailable bool
	}{
		{name: "queryTemplates null", answer: `{"orbit":{"queryTemplates":null}}`, unavailable: true},
		{name: "orbit null", answer: `{"orbit":null}`, unavailable: true},
		{name: "an empty list", answer: `{"orbit":{"queryTemplates":[]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				readGraphQL(t, r)
				testutil.RespondGraphQL(w, http.StatusOK, tc.answer)
			}))
			out, err := QueryTemplates(context.Background(), client, QueryTemplatesInput{})
			if tc.unavailable {
				if !errors.Is(err, ErrOrbitUnavailable) {
					t.Fatalf("QueryTemplates() error = %v, want ErrOrbitUnavailable", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("QueryTemplates() error: %v", err)
			}
			if out.Templates == nil || len(out.Templates) != 0 {
				t.Errorf("Templates = %#v, want an empty list", out.Templates)
			}
		})
	}
}

// TestQueryTemplates_RefusedOrErrored_CarriesTheReason verifies that a
// GraphQL error and a refused request are the action's error, the first with
// GitLab's message and the second with GitLab's status. A 403 from the
// GraphQL endpoint is about the token or the account, since GitLab answers an
// Orbit field it does not serve with null, so it carries no Orbit hint, as
// TestOrbit_ActionSpecs_ForbiddenStaysAnError holds for the other GraphQL
// reads.
func TestQueryTemplates_RefusedOrErrored_CarriesTheReason(t *testing.T) {
	t.Run("a GraphQL error", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			readGraphQL(t, r)
			testutil.RespondGraphQLError(w, http.StatusOK, "queryTemplates is not available")
		}))
		_, err := QueryTemplates(context.Background(), client, QueryTemplatesInput{})
		if err == nil || !strings.Contains(err.Error(), "queryTemplates is not available") {
			t.Fatalf("QueryTemplates() error = %v, want GitLab's message", err)
		}
	})
	t.Run("a refused request", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		}))
		_, err := QueryTemplates(context.Background(), client, QueryTemplatesInput{})
		if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), orbitUnavailableHint) {
			t.Fatalf("QueryTemplates() error = %v, want GitLab's 403 without the Orbit availability hint", err)
		}
	})
}

// TestQueryTemplatesDocument_CostsLittle holds the templates document under
// the complexity GitLab refuses a query above. GitLab.com answered it on
// 2026-10-29 without reporting a cost; the estimate is the bound.
func TestQueryTemplatesDocument_CostsLittle(t *testing.T) {
	cost, err := testutil.GitLabQueryComplexity(queryQueryTemplates, nil)
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error: %v", err)
	}
	if cost > testutil.GitLabAuthenticatedMaxComplexity {
		t.Errorf("complexity = %d, over the %d GitLab refuses a query above", cost, testutil.GitLabAuthenticatedMaxComplexity)
	}
}
