// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// topics_test.go contains unit tests for the topic MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package topics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestUpdate_EmptyDescription_ReachesTheBody holds a topic's description to
// an empty value removing it (issue 1306).
func TestUpdate_EmptyDescription_ReachesTheBody(t *testing.T) {
	input, err := toolutil.UnmarshalParams[UpdateInput](map[string]any{"topic_id": 1, "description": ""})
	if err != nil {
		t.Fatalf("UnmarshalParams() error: %v", err)
	}
	client, recorder := testutil.RecordWrites(t, `{}`, `{"id":1,"name":"go"}`)
	if _, err = Update(t.Context(), client, input); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	recorder.AssertMembers(t, map[string]string{"description": `""`})
}

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// topicJSON identifies the topic JSON constant used by this package.
const topicJSON = `{"id":1,"name":"go","title":"Go","description":"The Go programming language","total_projects_count":42,"organization_id":7,"avatar_url":"https://example.com/go.png"}`

// wantTopic is what topicJSON has to decode to, whole. No two of its fields
// carry the same value, so a converter that reads one of GitLab's fields into
// the neighboring output field produces a different object rather than an
// equal one: description and avatar_url could be crossed in topicToItem with
// the whole suite still green, because no test here read either of them.
var wantTopic = TopicItem{
	ID:                 1,
	Name:               "go",
	Title:              "Go",
	Description:        "The Go programming language",
	TotalProjectsCount: 42,
	OrganizationID:     7,
	AvatarURL:          "https://example.com/go.png",
}

// pathTopics identifies the path topics constant used by this package.
const pathTopics = "/api/v4/topics"

// pathTopicOne identifies the path topic one constant used by this package.
const pathTopicOne = "/api/v4/topics/1"

// errExpErrZeroTopicID identifies the err exp err zero topic ID constant used by this package.
const errExpErrZeroTopicID = "expected error for zero topic_id"

// errExpErrNegTopicID identifies the err exp err neg topic ID constant used by this package.
const errExpErrNegTopicID = "expected error for negative topic_id"

// testTopicID identifies the test topic ID constant used by this package.
const testTopicID = "topic_id"

// fmtExpErrMentionTopicID identifies the fmt exp err mention topic ID constant used by this package.
const fmtExpErrMentionTopicID = "expected error to mention topic_id, got %q"

// TestList_Success verifies that a listed topic arrives whole: every field
// GitLab sent under the output field that names it.
func TestList_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(out.Topics))
	}
	if out.Topics[0] != wantTopic {
		t.Errorf("listed topic = %+v, want %+v", out.Topics[0], wantTopic)
	}
}

// TestList_ReportsWhereThePageEnds verifies that the list carries GitLab's own
// pagination headers back to the caller. A list that reports none is a list a
// model cannot ask for the rest of, and dropping the block is a straight-line
// edit no mutant and no condition covers.
func TestList_ReportsWhereThePageEnds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodGet {
			testutil.RespondJSONWithPagination(w, http.StatusOK, `[`+topicJSON+`]`, testutil.PaginationHeaders{
				Page: "2", PerPage: "20", Total: "45", TotalPages: "9", NextPage: "3", PrevPage: "1",
			})
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	// Every number differs, so a block assembled from the wrong header is a
	// different block rather than an equal one.
	want := toolutil.PaginationOutput{
		Page: 2, PerPage: 20, TotalItems: 45, TotalPages: 9, NextPage: 3, PrevPage: 1, HasMore: true,
	}
	if out.Pagination != want {
		t.Errorf("pagination = %+v, want %+v", out.Pagination, want)
	}
}

// TestList_WithSearch verifies List when with search.
func TestList_WithSearch(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.URL.Query().Get("search") == "go" {
			testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{Search: "go"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(out.Topics))
	}
}

// TestList_WithoutProjects_IsSentOnlyWhenSet verifies List sends
// without_projects=true when the caller asks for the topics no project uses,
// the filter GET /topics declares and client-go v3.16.0 added, and no such key
// otherwise.
func TestList_WithoutProjects_IsSentOnlyWhenSet(t *testing.T) {
	tests := []struct {
		name  string
		input ListInput
		want  string
		sent  bool
	}{
		{"set", ListInput{WithoutProjects: true}, "true", true},
		{"unset", ListInput{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var query url.Values
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query = r.URL.Query()
				testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			}))
			if _, err := List(t.Context(), client, tt.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if got, sent := query.Get("without_projects"), query.Has("without_projects"); got != tt.want || sent != tt.sent {
				t.Errorf("without_projects = %q (sent: %v), want %q (sent: %v)", got, sent, tt.want, tt.sent)
			}
		})
	}
}

// TestList_KeysetPagination verifies List forwards per_page, keyset,
// page_token, order_by, sort and search to the GitLab API. All six are driven
// at once with values none of them shares, so a handler that writes one
// caller's value under another's parameter name is a different query string
// rather than an equal one.
func TestList_KeysetPagination(t *testing.T) {
	var q url.Values
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodGet {
			q = r.URL.Query()
			testutil.RespondJSON(w, http.StatusOK, `[`+topicJSON+`]`)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := List(t.Context(), client, ListInput{
		PerPage: 50,
		OrderBy: "name",
		Sort:    "desc",
		Search:  "gopher",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Topics) != 1 {
		t.Fatalf("expected 1 topic, got %d", len(out.Topics))
	}
	checks := map[string]string{
		"order_by": "name",
		"sort":     "desc",
		"per_page": "50",
		"search":   "gopher",
	}
	for key, want := range checks {
		t.Run(key, func(t *testing.T) {
			if got := q.Get(key); got != want {
				t.Errorf("query %q = %q, want %q", key, got, want)
			}
		})
	}
}

// topicHandlerCase is one topic handler, the HTTP status its corrective hint
// was written for, and the opening of that hint. Every opening is distinct, so
// a hint attached to the wrong handler is as visible as one attached at the
// wrong status.
type topicHandlerCase struct {
	name       string
	hintStatus int
	hintOpens  string
	call       func(ctx context.Context, client *gitlabclient.Client) error
}

// topicHandlerCases lists all five handlers with the status-to-hint pairing
// each one declares.
func topicHandlerCases() []topicHandlerCase {
	return []topicHandlerCase{
		{"list", http.StatusForbidden, "topic listing is public on most instances", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := List(ctx, c, ListInput{})
			return err
		}},
		{"get", http.StatusNotFound, "verify topic id (numeric) with admin.topic_list", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Get(ctx, c, GetInput{TopicID: 1})
			return err
		}},
		{"create", http.StatusForbidden, "requires administrator access; name must be unique on the instance", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Create(ctx, c, CreateInput{Name: "go"})
			return err
		}},
		{"update", http.StatusForbidden, "requires administrator access; verify id with admin.topic_list", func(ctx context.Context, c *gitlabclient.Client) error {
			_, err := Update(ctx, c, UpdateInput{TopicID: 1, Title: "Go"})
			return err
		}},
		{"delete", http.StatusForbidden, "requires administrator access; deletion is irreversible", func(ctx context.Context, c *gitlabclient.Client) error {
			return Delete(ctx, c, DeleteInput{TopicID: 1})
		}},
	}
}

// statusOnlyClient answers every request with one status and no body, which is
// all the status check behind each hint reads.
func statusOnlyClient(t *testing.T, status int) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
}

// TestTopicHandlers_HintReachesTheCallerAtItsOwnStatusOnly verifies that each
// handler's corrective hint is attached at the status it was written for and
// at no other, which is the whole of what WrapErrWithStatusHint decides.
//
// Nothing else held that pairing. A hint keyed to the wrong status still
// compiles, still returns an error, and reads exactly like the tests this
// replaces, which asserted only that an error came back: Get's 404 could be
// changed to 403, retiring the one hint that tells a model the topic id was
// wrong, with the whole suite still green.
func TestTopicHandlers_HintReachesTheCallerAtItsOwnStatusOnly(t *testing.T) {
	// A status no hint here is keyed to, so the other leg of the branch is the
	// one taken.
	const unhintedStatus = http.StatusBadRequest
	for _, tc := range topicHandlerCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("at its own status", func(t *testing.T) {
				err := tc.call(t.Context(), statusOnlyClient(t, tc.hintStatus))
				if err == nil {
					t.Fatalf("status %d: expected an error, got nil", tc.hintStatus)
				}
				if want := "Suggestion: " + tc.hintOpens; !strings.Contains(err.Error(), want) {
					t.Errorf("status %d: error %q does not carry %q", tc.hintStatus, err, want)
				}
			})
			t.Run("at another status", func(t *testing.T) {
				err := tc.call(t.Context(), statusOnlyClient(t, unhintedStatus))
				if err == nil {
					t.Fatalf("status %d: expected an error, got nil", unhintedStatus)
				}
				if strings.Contains(err.Error(), "Suggestion:") {
					t.Errorf("status %d: hint attached to a status it was not written for: %q", unhintedStatus, err)
				}
			})
		})
	}
}

// TestGet_Success verifies that a fetched topic arrives whole: every field
// GitLab sent under the output field that names it.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Get(t.Context(), client, GetInput{TopicID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic != wantTopic {
		t.Errorf("fetched topic = %+v, want %+v", out.Topic, wantTopic)
	}
}

// TestCreate_Success verifies that the topic GitLab answers a creation with
// reaches the caller whole.
func TestCreate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopics && r.Method == http.MethodPost {
			testutil.RespondJSON(w, http.StatusCreated, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Create(t.Context(), client, CreateInput{Name: "go", Title: "Go"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic != wantTopic {
		t.Errorf("created topic = %+v, want %+v", out.Topic, wantTopic)
	}
}

// TestUpdate_Success verifies that the topic GitLab answers an update with
// reaches the caller whole.
func TestUpdate_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))

	out, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Title: "Golang"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic != wantTopic {
		t.Errorf("updated topic = %+v, want %+v", out.Topic, wantTopic)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == pathTopicOne && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	}))

	err := Delete(t.Context(), client, DeleteInput{TopicID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestGet_InvalidTopicID verifies Get when invalid topic ID.
func TestGet_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Get(t.Context(), client, GetInput{TopicID: 0})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	_, err = Get(t.Context(), client, GetInput{TopicID: -1})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// TestUpdate_InvalidTopicID verifies Update when invalid topic ID.
func TestUpdate_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := Update(t.Context(), client, UpdateInput{TopicID: 0, Title: "x"})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	_, err = Update(t.Context(), client, UpdateInput{TopicID: -1, Title: "x"})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// TestDelete_InvalidTopicID verifies Delete when invalid topic ID.
func TestDelete_InvalidTopicID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	err := Delete(t.Context(), client, DeleteInput{TopicID: 0})
	if err == nil {
		t.Fatal(errExpErrZeroTopicID)
	}
	if !strings.Contains(err.Error(), testTopicID) {
		t.Errorf(fmtExpErrMentionTopicID, err)
	}
	err = Delete(t.Context(), client, DeleteInput{TopicID: -1})
	if err == nil {
		t.Fatal(errExpErrNegTopicID)
	}
}

// The Markdown formatters are covered whole-output in markdown_test.go,
// beside the card and the list vocabulary they now write.

// TestCreate_WithAllOptionalFields verifies that each field the caller filled
// in reaches GitLab under its own name and carries its own value, and that
// nothing else is sent. Every value here is distinct, because asserting only
// that the body mentions "title" and "description" passes just as happily on a
// handler that writes the title into the description key.
func TestCreate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Create(t.Context(), client, CreateInput{
		Name: "go", Title: "Go language", Description: "Everything about Go",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic != wantTopic {
		t.Errorf("created topic = %+v, want %+v", out.Topic, wantTopic)
	}
	assertSentBody(t, capturedBody, map[string]string{
		"name": "go", "title": "Go language", "description": "Everything about Go",
	})
}

// TestUpdate_WithAllOptionalFields verifies that each field the caller filled
// in reaches GitLab under its own name and carries its own value, and that
// nothing else is sent. The rename is the one that went unheld: the guard
// around it could be inverted, so that a caller's new name never left this
// process and an empty one was sent whenever they gave none, with the whole
// suite still green.
func TestUpdate_WithAllOptionalFields(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusOK, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Update(t.Context(), client, UpdateInput{
		TopicID: 1, Name: "golang", Title: "Golang", Description: new("Updated desc"),
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Topic != wantTopic {
		t.Errorf("updated topic = %+v, want %+v", out.Topic, wantTopic)
	}
	assertSentBody(t, capturedBody, map[string]string{
		"name": "golang", "title": "Golang", "description": "Updated desc",
	})
}

// TestUpdate_OmittedFieldsAreNotSent verifies the other half of each guard: a
// field the caller left empty is left out of the request rather than sent as an
// empty string, which on a rename would clear the topic's name.
//
// Each optional field is driven on its own rather than all three at once,
// because a body carrying every field cannot say which guard admitted which:
// one field set is the only shape in which the two legs of a guard are told
// apart.
func TestUpdate_OmittedFieldsAreNotSent(t *testing.T) {
	cases := []struct {
		name  string
		input UpdateInput
		want  map[string]string
	}{
		{"name only", UpdateInput{TopicID: 1, Name: "golang"}, map[string]string{"name": "golang"}},
		{"title only", UpdateInput{TopicID: 1, Title: "Golang"}, map[string]string{"title": "Golang"}},
		{"description only", UpdateInput{TopicID: 1, Description: new("Updated desc")}, map[string]string{"description": "Updated desc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedBody string
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("read request body: %v", err)
						http.Error(w, "read request body", http.StatusInternalServerError)
						return
					}
					capturedBody = string(body)
					testutil.RespondJSON(w, http.StatusOK, topicJSON)
					return
				}
				http.NotFound(w, r)
			}))
			if _, err := Update(t.Context(), client, tc.input); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			assertSentBody(t, capturedBody, tc.want)
		})
	}
}

// TestCreate_OmittedFieldsAreNotSent verifies the same of a creation: the name
// is the only field GitLab requires, and a title or description the caller did
// not give is absent from the request rather than sent empty.
func TestCreate_OmittedFieldsAreNotSent(t *testing.T) {
	var capturedBody string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read request body: %v", err)
				http.Error(w, "read request body", http.StatusInternalServerError)
				return
			}
			capturedBody = string(body)
			testutil.RespondJSON(w, http.StatusCreated, topicJSON)
			return
		}
		http.NotFound(w, r)
	}))
	if _, err := Create(t.Context(), client, CreateInput{Name: "go"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	assertSentBody(t, capturedBody, map[string]string{"name": "go"})
}

// assertSentBody holds the request body to exactly the fields want names, with
// exactly the values it gives them. Decoding into a map rather than a struct is
// what makes a field nobody asked to send visible: a struct would ignore it.
func assertSentBody(t *testing.T, body string, want map[string]string) {
	t.Helper()
	var sent map[string]string
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("request body %q is not an object of strings: %v", body, err)
	}
	if !maps.Equal(sent, want) {
		t.Errorf("request body = %v, want %v", sent, want)
	}
}

// TestTopics_UnreadableOrganizationID verifies that every topic handler
// returns an error rather than a half-filled topic when GitLab sends
// organization_id as something that is not a number. client-go models
// organization_id on its own Topic as of v3.12.0, so the SDK's decoder is what
// refuses it now that the captured read is retired.
func TestTopics_UnreadableOrganizationID(t *testing.T) {
	// A list answers with an array and the rest with an object, so each case
	// drives a client of its own rather than one shared handler.
	poisoned := func(body string) *gitlabclient.Client {
		return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, body)
		}))
	}
	testutil.AssertUnreadableBodyRefused(t, []testutil.CapturedCase{
		{Name: "list", Call: func() error {
			client := poisoned(`[{"id":1,"name":"go","title":"Go","organization_id":"not-a-number"}]`)
			_, err := List(context.Background(), client, ListInput{})
			return err
		}},
		{Name: "get", Call: func() error {
			client := poisoned(`{"id":1,"name":"go","title":"Go","organization_id":"not-a-number"}`)
			_, err := Get(context.Background(), client, GetInput{TopicID: 1})
			return err
		}},
		{Name: "create", Call: func() error {
			client := poisoned(`{"id":1,"name":"go","title":"Go","organization_id":"not-a-number"}`)
			_, err := Create(context.Background(), client, CreateInput{Name: "go", Title: "Go"})
			return err
		}},
		{Name: "update", Call: func() error {
			client := poisoned(`{"id":1,"name":"go","title":"Go","organization_id":"not-a-number"}`)
			_, err := Update(context.Background(), client, UpdateInput{TopicID: 1, Title: "Golang"})
			return err
		}},
	})
}

// topicAvatarForm is what a mock reads off a topic create or update sent
// with an avatar: the method, the file part's name and bytes, and the value
// fields.
type topicAvatarForm struct {
	method, filename, content string
	values                    map[string][]string
}

// topicAvatarHandler answers a create or an update with the topic, handing
// the multipart form it was sent to the test goroutine.
func topicAvatarHandler(t *testing.T, forms chan<- topicAvatarForm) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		form, err := testutil.ReadMultipartForm(r, 1<<20)
		if err != nil {
			t.Errorf("the request was not a multipart form: %v", err)
			http.Error(w, "not multipart", http.StatusBadRequest)
			return
		}
		part, header, err := testutil.FormFile(form, "avatar")
		if err != nil {
			t.Errorf("the request carried no avatar part: %v", err)
			http.Error(w, "no avatar", http.StatusBadRequest)
			return
		}
		content, _ := io.ReadAll(part)
		_ = part.Close()
		forms <- topicAvatarForm{method: r.Method, filename: header.Filename, content: string(content), values: form.Value}
		testutil.RespondJSON(w, http.StatusOK, topicJSON)
	})
}

// TestCreateAndUpdate_WithAvatar_UploadIt verifies that a topic create and a
// topic update given an avatar send the image under avatar with its file
// name, read from base64 content or from a local file, and every option as
// a field of the form beside it.
func TestCreateAndUpdate_WithAvatar_UploadIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "go.png")
	if err := os.WriteFile(file, []byte("file-image"), 0o600); err != nil {
		t.Fatalf("write the avatar file: %v", err)
	}
	fromBase64 := &AvatarInput{Filename: "go.png", ContentBase64: base64.StdEncoding.EncodeToString([]byte("b64-image"))}
	fromFile := &AvatarInput{Filename: "go.png", FilePath: file}
	cases := []struct {
		name        string
		call        func(*gitlabclient.Client) (TopicItem, error)
		wantMethod  string
		wantContent string
		wantValues  map[string]string
	}{
		{
			name: "create from base64", wantMethod: http.MethodPost, wantContent: "b64-image",
			wantValues: map[string]string{"name": "go", "title": "Go", "description": "The language"},
			call: func(client *gitlabclient.Client) (TopicItem, error) {
				out, err := Create(t.Context(), client, CreateInput{Name: "go", Title: "Go", Description: "The language", Avatar: fromBase64})
				return out.Topic, err
			},
		},
		{
			name: "update from a local file", wantMethod: http.MethodPut, wantContent: "file-image",
			wantValues: map[string]string{"title": "Golang"},
			call: func(client *gitlabclient.Client) (TopicItem, error) {
				out, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Title: "Golang", Avatar: fromFile})
				return out.Topic, err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			forms := make(chan topicAvatarForm, 1)
			client := testutil.NewTestClient(t, topicAvatarHandler(t, forms))
			topic, err := tc.call(client)
			if err != nil {
				t.Fatalf("%s error: %v", tc.name, err)
			}
			if topic != wantTopic {
				t.Errorf("topic = %+v, want %+v", topic, wantTopic)
			}
			form := <-forms
			if form.method != tc.wantMethod || form.filename != "go.png" || form.content != tc.wantContent {
				t.Errorf("form = %s %q holding %q, want %s go.png holding %q", form.method, form.filename, form.content, tc.wantMethod, tc.wantContent)
			}
			sent := map[string]string{}
			for name, values := range form.values {
				sent[name] = strings.Join(values, "|")
			}
			if !maps.Equal(sent, tc.wantValues) {
				t.Errorf("form fields = %v, want %v", sent, tc.wantValues)
			}
		})
	}
}

// TestCreateAndUpdate_AvatarTheyCannotSend_AreRefusedBeforeAnyRequest
// verifies that an avatar with no file name or no readable content, and an
// update asked both to upload an avatar and to remove it, are refused naming
// the action, and that nothing reaches GitLab.
func TestCreateAndUpdate_AvatarTheyCannotSend_AreRefusedBeforeAnyRequest(t *testing.T) {
	image := base64.StdEncoding.EncodeToString([]byte("img"))
	cases := []struct {
		name string
		call func(*gitlabclient.Client) error
		want string
	}{
		{name: "create with no file name", want: "create_topic: avatar.filename is required", call: func(client *gitlabclient.Client) error {
			_, err := Create(t.Context(), client, CreateInput{Name: "go", Title: "Go", Avatar: &AvatarInput{ContentBase64: image}})
			return err
		}},
		{name: "create with no source", want: "create_topic avatar: either file_path or content_base64 is required", call: func(client *gitlabclient.Client) error {
			_, err := Create(t.Context(), client, CreateInput{Name: "go", Title: "Go", Avatar: &AvatarInput{Filename: "go.png"}})
			return err
		}},
		{name: "update with no file name", want: "update_topic: avatar.filename is required", call: func(client *gitlabclient.Client) error {
			_, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Avatar: &AvatarInput{ContentBase64: image}})
			return err
		}},
		{name: "update with both sources", want: "update_topic avatar: provide either file_path or content_base64", call: func(client *gitlabclient.Client) error {
			_, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Avatar: &AvatarInput{Filename: "go.png", FilePath: "/tmp/go.png", ContentBase64: image}})
			return err
		}},
		{name: "update uploading and removing", want: "update_topic: avatar and remove_avatar cannot be combined", call: func(client *gitlabclient.Client) error {
			_, err := Update(t.Context(), client, UpdateInput{TopicID: 1, RemoveAvatar: true, Avatar: &AvatarInput{Filename: "go.png", ContentBase64: image}})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("a request with an avatar it cannot send reached GitLab: %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			if err := tc.call(client); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// TestUpdate_RemoveAvatar_SendsTheBlankAvatarGitLabRemovesItFor verifies
// that remove_avatar sends avatar as an empty value in the JSON body, which
// PUT /topics/:id answers with remove_avatar! (lib/api/topics.rb), and that
// an update without it sends no avatar at all.
func TestUpdate_RemoveAvatar_SendsTheBlankAvatarGitLabRemovesItFor(t *testing.T) {
	client, recorder := testutil.RecordWrites(t, `{}`, topicJSON)
	if _, err := Update(t.Context(), client, UpdateInput{TopicID: 1, RemoveAvatar: true}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	recorder.AssertMembers(t, map[string]string{"avatar": `""`})

	client, recorder = testutil.RecordWrites(t, `{}`, topicJSON)
	if _, err := Update(t.Context(), client, UpdateInput{TopicID: 1, Title: "Golang"}); err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if got, ok := recorder.Member("avatar"); ok {
		t.Errorf("an update without remove_avatar sent avatar = %s", got)
	}
}
