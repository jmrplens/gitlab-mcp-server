// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// contextStandIn answers POST /orbit/context with the body named, after
// holding the request to the JSON body the handler is to send.
func contextStandIn(t *testing.T, wantBody map[string]any, status int, answer string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodPost)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/context")
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type = %q, want JSON: GitLab refuses a form body", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if !reflect.DeepEqual(body, wantBody) {
			t.Errorf("request body = %v, want %v", body, wantBody)
		}
		testutil.RespondJSON(w, status, answer)
	})
}

// TestEntityContext_EachTypeGitLabResolves_IsSummarizedTyped holds
// [EntityContext] to the answers GitLab.com gave on 2026-10-29 for the four
// types it resolved (paths and names replaced, logs cut): each summary
// published in its typed fields, an expansion with the named query GitLab ran
// and its result, and raw sent when the caller chose no format.
func TestEntityContext_EachTypeGitLabResolves_IsSummarizedTyped(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		token  string
		expand bool
		check  func(t *testing.T, entity ContextEntity)
	}{
		{name: "a work item by path#iid, expanded", answer: "context_work_item.json", token: "acme-group/orbit-fixtures/kg-fixtures#1", expand: true, check: checkWorkItemContext},
		{name: "a merge request by path!iid, expanded", answer: "context_merge_request_expand.json", token: "acme-group/orbit-fixtures/kg-fixtures!1", expand: true, check: checkMergeRequestContext},
		{name: "a branch by path@ref", answer: "context_branch.json", token: "acme-group/orbit-fixtures/kg-fixtures@main", check: checkBranchContext},
		{name: "a job by Type[id], expanded", answer: "context_job_expand.json", token: "Job[14831871432]", expand: true, check: checkJobContext},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]any{"tokens": []any{tc.token}, "expand": tc.expand, "response_format": "raw"}
			client := testutil.NewTestClient(t, contextStandIn(t, want, http.StatusOK, readFixture(t, tc.answer)))
			out, err := EntityContext(context.Background(), client, EntityContextInput{Tokens: []string{tc.token}, Expand: tc.expand})
			if err != nil {
				t.Fatalf("EntityContext() error: %v", err)
			}
			if out.Version != "1.3.0" || len(out.Entities) != 1 || !out.Entities[0].Found {
				t.Fatalf("EntityContext() = %+v, want one entity found, answer version 1.3.0", out)
			}
			tc.check(t, out.Entities[0])
		})
	}
}

// expansionQuery is the named query an expansion ran, or "" when there is
// none.
func expansionQuery(e ContextEntity) string {
	if e.Expansion == nil || e.Expansion.Query == nil {
		return ""
	}
	return e.Expansion.Query.Name
}

// checkWorkItemContext holds the work item GitLab.com resolved path#iid to:
// its labels, its type and the context_for_issue expansion.
func checkWorkItemContext(t *testing.T, e ContextEntity) {
	t.Helper()
	s := e.Summary
	if e.Type != "WorkItem" || e.ResolvedToken != "WorkItem[207153944]" || s == nil || s.IID != 1 || s.WorkItemType != "issue" || !reflect.DeepEqual(s.Labels, []string{"enhancement", "good first issue"}) {
		t.Errorf("entity = %+v, summary %+v, want the work item with its labels", e, s)
	}
	if got := expansionQuery(e); got != "context_for_issue" {
		t.Errorf("expansion query = %q, want context_for_issue", got)
	}
}

// checkMergeRequestContext holds the merge request GitLab.com resolved
// path!iid to: its author, two failed jobs with their reason, the changed
// file and the context_for_review expansion with its result.
func checkMergeRequestContext(t *testing.T, e ContextEntity) {
	t.Helper()
	s := e.Summary
	if s == nil || s.Author == nil || s.Author.Username != "fixture-user" || s.SourceBranch != "feature/restock-helper" || len(s.FailedJobs) != 2 || s.FailedJobs[0].FailureReason == nil || *s.FailedJobs[0].FailureReason != "script_failure" {
		t.Fatalf("summary = %+v, want the merge request with its author and two failed jobs", s)
	}
	if len(s.ChangedFiles) != 1 || s.ChangedFiles[0].Path != "src/acme/orders/fulfillment.py" || !reflect.DeepEqual(s.ChangedFiles[0].Lines, []string{"16-21"}) {
		t.Errorf("changed files = %+v, want the one file with its lines", s.ChangedFiles)
	}
	if expansionQuery(e) != "context_for_review" || e.Expansion.Result == nil {
		t.Errorf("expansion = %+v, want context_for_review and its result", e.Expansion)
	}
}

// checkBranchContext holds the branch GitLab.com resolved path@ref to, with
// no expansion since none was asked.
func checkBranchContext(t *testing.T, e ContextEntity) {
	t.Helper()
	s := e.Summary
	if s == nil || s.Name != "main" || s.DefaultBranch == nil || !*s.DefaultBranch || s.PipelineID == 0 || len(s.SHA) != 40 {
		t.Errorf("summary = %+v, want the default branch with its pipeline", s)
	}
	if e.Expansion != nil {
		t.Errorf("expansion = %+v, want none when expand was not asked", e.Expansion)
	}
}

// checkJobContext holds the job GitLab.com resolved Job[id] to: its log, no
// failure reason, and the context_for_job expansion.
func checkJobContext(t *testing.T, e ContextEntity) {
	t.Helper()
	s := e.Summary
	if s == nil || s.ID != 14831871432 || s.Status != "success" || s.FailureReason != nil || s.Log == "" {
		t.Errorf("summary = %+v, want the job with its log and no failure reason", s)
	}
	if got := expansionQuery(e); got != "context_for_job" {
		t.Errorf("expansion query = %q, want context_for_job", got)
	}
}

// contextAnswer is the answer GitLab gives POST /orbit/context for one found
// entity, with the summary and the expansion a test names: the fields
// ContextService and ContextExpansionService write at version 1.3.0 of the
// answer, for what no fixture of 2026-10-29 held.
func contextAnswer(kind, id, summary, expansion string) string {
	return `{"version":"1.3.0","entities":[{"token":"t","resolved_token":"` + kind + `[` + id + `]","type":"` + kind + `","id":"` + id +
		`","found":true,"summary":` + summary + `,"expansion":` + expansion + `}]}`
}

// resolvedContext returns the one entity the handler publishes for an answer.
func resolvedContext(t *testing.T, answer string) ContextEntity {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, answer)
	}))
	out, err := EntityContext(context.Background(), client, EntityContextInput{Tokens: []string{"t"}, Expand: true})
	if err != nil {
		t.Fatalf("EntityContext() error: %v", err)
	}
	if len(out.Entities) != 1 {
		t.Fatalf("EntityContext() = %+v, want one entity", out)
	}
	return out.Entities[0]
}

// TestEntityContext_PipelineAndCommit_PublishEveryFieldGitLabSends holds the
// two types no capture resolved to the fields ContextService writes for them:
// a pipeline's source, jobs and failed jobs, and a commit's author, date, files
// and merge requests.
func TestEntityContext_PipelineAndCommit_PublishEveryFieldGitLabSends(t *testing.T) {
	failure := "script_failure"
	cases := []struct {
		name, kind, summary string
		want                ContextSummary
	}{
		{
			name: "a pipeline", kind: "Pipeline",
			summary: `{"namespace_path":"acme","project_path":"acme/app","id":7,"status":"failed","ref":"main","sha":"abc","source":"push","web_url":"https://gitlab.example.com/p/7","merge_request":null,
				"jobs":[{"name":"ruff","stage":"lint","status":"failed","allow_failure":true,"web_url":"https://gitlab.example.com/j/1"}],
				"failed_jobs":[{"name":"ruff","stage":"lint","failure_reason":"script_failure","allow_failure":true,"web_url":"https://gitlab.example.com/j/1",
					"failed_tests":[{"name":"test_one","classname":"suite","file":"a_test.py","output":"boom"}],"log":"boom","truncated":["failed_tests"]}],
				"truncated":["jobs"]}`,
			want: ContextSummary{
				NamespacePath: "acme", ProjectPath: "acme/app", ID: 7, Status: "failed", Ref: "main", SHA: "abc", Source: "push", WebURL: "https://gitlab.example.com/p/7",
				Jobs: []ContextPipelineJob{{Name: "ruff", Stage: "lint", Status: "failed", AllowFailure: true, WebURL: "https://gitlab.example.com/j/1"}},
				FailedJobs: []ContextFailedJob{{
					Name: "ruff", Stage: "lint", FailureReason: &failure, AllowFailure: true, WebURL: "https://gitlab.example.com/j/1", Log: "boom",
					FailedTests: []ContextFailedTest{{Name: "test_one", ClassName: "suite", File: "a_test.py", Output: "boom"}}, Truncated: []string{"failed_tests"},
				}},
				Truncated: []string{"jobs"},
			},
		},
		{
			name: "a commit", kind: "Commit",
			summary: `{"namespace_path":"acme","project_path":"acme/app","sha":"abc","title":"Fix","description":null,"author_name":"A. Person","authored_date":"2026-10-29T08:00:00Z",
				"web_url":"https://gitlab.example.com/c/abc","pipeline_id":null,"pipeline_status":null,"merge_requests":[{"type":"MergeRequest","id":"5"}],
				"changed_files":[{"path":"new.go","old_path":"old.go","status":"R","lines":["1-2"],"diff":"+x"}],"truncated":[]}`,
			want: ContextSummary{
				NamespacePath: "acme", ProjectPath: "acme/app", SHA: "abc", Title: "Fix", AuthorName: "A. Person", AuthoredDate: "2026-10-29T08:00:00Z",
				WebURL: "https://gitlab.example.com/c/abc", MergeRequests: []any{map[string]any{"type": "MergeRequest", "id": "5"}},
				ChangedFiles: []ContextChangedFile{{Path: "new.go", OldPath: "old.go", Status: "R", Lines: []string{"1-2"}, Diff: "+x"}}, Truncated: []string{},
			},
		},
		{
			name: "a failed job", kind: "Job",
			summary: `{"id":9,"name":"unit","status":"failed","failed_tests":[{"name":"test_one"}],"log":"boom","truncated":["log"]}`,
			want:    ContextSummary{ID: 9, Name: "unit", Status: "failed", FailedTests: []ContextFailedTest{{Name: "test_one"}}, Log: "boom", Truncated: []string{"log"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entity := resolvedContext(t, contextAnswer(tc.kind, "7", tc.summary, "null"))
			if entity.Summary == nil || !reflect.DeepEqual(*entity.Summary, tc.want) {
				t.Errorf("summary = %+v, want %+v", entity.Summary, tc.want)
			}
		})
	}
}

// TestEntityContext_AnExpansionThatFailed_PublishesWhy verifies that a
// neighborhood GitLab did not answer is published with its code, so that it
// never reads as an object with no neighbors: the query that would have run
// where there is one, and for a billing group not chosen the message, the
// address and the groups GitLab answers the query's own 422 with.
func TestEntityContext_AnExpansionThatFailed_PublishesWhy(t *testing.T) {
	const query = `,"query":{"name":"context_for_job","parameters":{"node_ids":["9"],"limit":100}}`
	cases := []struct {
		name      string
		expansion string
		want      ContextExpansionError
		query     string
	}{
		{name: "a type GitLab expands none for", expansion: `{"error":{"code":"expansion_unsupported_type"}}`, want: ContextExpansionError{Code: "expansion_unsupported_type"}},
		{name: "outside the graph", expansion: `{"error":{"code":"outside_graph_scope"}` + query + `}`, want: ContextExpansionError{Code: "outside_graph_scope"}, query: "context_for_job"},
		{name: "credits used up", expansion: `{"error":{"code":"quota_exhausted"}` + query + `}`, want: ContextExpansionError{Code: "quota_exhausted"}, query: "context_for_job"},
		{name: "a timeout", expansion: `{"error":{"code":"timeout"}` + query + `}`, want: ContextExpansionError{Code: "timeout"}, query: "context_for_job"},
		{name: "a query that did not run", expansion: `{"error":{"code":"execution_error"}` + query + `}`, want: ContextExpansionError{Code: "execution_error"}, query: "context_for_job"},
		{name: "no billing group", expansion: `{"error":` + billingRefusal + query + `}`, want: ContextExpansionError{
			Code: billingNamespaceCode, Message: "Select a default group in your preferences to run GitLab Orbit queries.",
			DeepLink: "https://gitlab.com/-/profile/preferences", Candidates: []BillingCandidate{{ID: 10, Name: "acme"}, {ID: 11, Name: "acme-tools"}},
		}, query: "context_for_job"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entity := resolvedContext(t, contextAnswer("Job", "9", `{"id":9,"name":"unit","status":"success","truncated":[]}`, tc.expansion))
			if entity.Expansion == nil || entity.Expansion.Error == nil || !reflect.DeepEqual(*entity.Expansion.Error, tc.want) {
				t.Fatalf("expansion = %+v, want the error %+v", entity.Expansion, tc.want)
			}
			if got := expansionQuery(entity); got != tc.query || entity.Expansion.Result != nil {
				t.Errorf("expansion query = %q, result %v, want %q and no result", got, entity.Expansion.Result, tc.query)
			}
		})
	}
}

// TestEntityContextSchema_OffersOneTokenAsItsExample holds the input schema of
// orbit.context to its example, a list of one token, which find fills the call
// it offers with.
func TestEntityContextSchema_OffersOneTokenAsItsExample(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	schema := orbitActionSpecRoutes(t, client)["context"].InputSchema
	if got := propertySchema(t, schema, "tokens")["examples"]; !reflect.DeepEqual(got, []any{[]any{contextExample}}) {
		t.Errorf("tokens examples = %#v, want a list of %q", got, contextExample)
	}
}

// TestEntityContext_NotFound_IsAResultNamingTheCode verifies that an entity
// GitLab answers found:false (an unsupported type, an epic in a group with
// none) is published as a result naming the token and the code, not as an
// error and not as an empty summary.
func TestEntityContext_NotFound_IsAResultNamingTheCode(t *testing.T) {
	cases := []struct {
		name, answer, token, code string
	}{
		{name: "an unsupported type", answer: "context_unsupported.json", token: "Vulnerability[313450676]", code: "unsupported_type"},
		{name: "an epic not found", answer: "context_not_found.json", token: "acme-group&1", code: "not_found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := map[string]any{"tokens": []any{tc.token}, "expand": false, "response_format": "raw"}
			client := testutil.NewTestClient(t, contextStandIn(t, want, http.StatusOK, readFixture(t, tc.answer)))
			out, err := EntityContext(context.Background(), client, EntityContextInput{Tokens: []string{tc.token}})
			if err != nil {
				t.Fatalf("EntityContext() error: %v", err)
			}
			entity := out.Entities[0]
			if entity.Found || entity.Token != tc.token || entity.Error == nil || entity.Error.Code != tc.code || entity.Summary != nil {
				t.Errorf("entity = %+v, want found false naming %s", entity, tc.code)
			}
		})
	}
}

// TestEntityContext_LLM_IsGitLabsText verifies the llm format is sent as
// GitLab spells it and its text/plain answer published whole.
func TestEntityContext_LLM_IsGitLabsText(t *testing.T) {
	text := readFixture(t, "context_llm.txt")
	want := map[string]any{"tokens": []any{"gid://gitlab/Project/83234203"}, "expand": false, "response_format": "llm"}
	client := testutil.NewTestClient(t, contextStandIn(t, want, http.StatusOK, text))
	out, err := EntityContext(context.Background(), client, EntityContextInput{Tokens: []string{"gid://gitlab/Project/83234203"}, ResponseFormat: "llm"})
	if err != nil {
		t.Fatalf("EntityContext() error: %v", err)
	}
	if out.FormattedText != text || out.Entities != nil || out.ResponseFormat != "llm" {
		t.Errorf("EntityContext() = %+v, want the llm text alone", out)
	}
}

// TestEntityContext_InvalidInput_IsRefusedBeforeSending covers what the
// handler refuses before a request: no token, more than the one token GitLab
// takes (which it answers 400 "tokens must be an array of exactly one
// string"), a blank token and a format the route does not take.
func TestEntityContext_InvalidInput_IsRefusedBeforeSending(t *testing.T) {
	cases := []struct {
		name  string
		input EntityContextInput
		want  string
	}{
		{name: "no token", input: EntityContextInput{}, want: "tokens"},
		{name: "two tokens", input: EntityContextInput{Tokens: []string{"Job[1]", "Job[2]"}}, want: "exactly one token"},
		{name: "a blank token", input: EntityContextInput{Tokens: []string{"  "}}, want: "tokens"},
		{name: "the gql format", input: EntityContextInput{Tokens: []string{"Job[1]"}, ResponseFormat: "gql"}, want: "invalid response_format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("request sent for an input refused before one: %s %s", r.Method, r.URL.Path)
				testutil.RespondJSON(w, http.StatusInternalServerError, `{}`)
			}))
			_, err := EntityContext(context.Background(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("EntityContext() error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

// TestEntityContext_GitLabsRefusal_CarriesItsMessage verifies that GitLab's
// own 400 reaches the caller with its message and the hint.
func TestEntityContext_GitLabsRefusal_CarriesItsMessage(t *testing.T) {
	body := readFixture(t, "context_two_tokens.json")
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, body)
	}))
	_, err := EntityContext(context.Background(), client, EntityContextInput{Tokens: []string{"Job[1]"}})
	if err == nil || !strings.Contains(err.Error(), "tokens must be an array of exactly one string") || !strings.Contains(err.Error(), contextRefusedHint) {
		t.Fatalf("EntityContext() error = %v, want GitLab's message and the hint", err)
	}
}

// TestFormatEntityContextMarkdown_SummaryLogAndExpansion holds the context
// card to its parts: the entity's fields, its description quoted, a failed
// job's log in a fence widened past the backticks it holds, the expansion's
// query, and the found:false result naming its code.
func TestFormatEntityContextMarkdown_SummaryLogAndExpansion(t *testing.T) {
	reason := "script_failure"
	md := FormatEntityContextMarkdown(EntityContextOutput{Version: "1.3.0", Entities: []ContextEntity{{
		Token: "g/p!1", ResolvedToken: "MergeRequest[1]", Type: "MergeRequest", ID: "1", Found: true,
		Summary: &ContextSummary{
			Title: "Fix | it", State: "opened", IID: 1, WebURL: "https://gitlab.example.com/g/p/-/merge_requests/1",
			Description: "line one\nline two",
			FailedJobs:  []ContextFailedJob{{Name: "ruff", Stage: "lint", FailureReason: &reason, Log: "```\nboom"}},
		},
		Expansion: &ContextExpansion{Query: &ContextQuery{Name: "context_for_review"}, Result: map[string]any{"nodes": []any{}}},
	}, {Token: "Project[1]", Found: false, Error: &ContextError{Code: "unsupported_type"}}}})
	if lacks := missingFrom(md, "## Orbit Context", "#### Expansion\n\n```json\n{\n  \"nodes\": []\n}\n```", "### MergeRequest&#91;1]", "Fix &#124; it", "> line one", "````text\n```\nboom\n````", "context_for_review", "unsupported_type", "Project[1]", "orbit.named_query"); len(lacks) > 0 {
		t.Errorf("card lacks %q:\n%s", lacks, md)
	}
	if llm := FormatEntityContextMarkdown(EntityContextOutput{FormattedText: "orbit_context version=1.3.0"}); !strings.Contains(llm, "```text\norbit_context version=1.3.0\n```") {
		t.Errorf("llm card = %q, want the text in one fence", llm)
	}
	if empty := FormatEntityContextMarkdown(EntityContextOutput{Version: "1.3.0"}); !strings.Contains(empty, "GitLab resolved no entity.") {
		t.Errorf("empty card = %q, want it to say nothing was resolved", empty)
	}
	if empty := FormatEntityContextMarkdown(EntityContextOutput{Entities: []ContextEntity{{Token: "t", Found: true}}}); strings.Contains(empty, "no entity") || strings.Contains(empty, "Changed files") {
		t.Errorf("card of an entity with no summary = %q, want its rows alone", empty)
	}
}

// TestFormatEntityContextMarkdown_NotFound_SaysItMayBeUnreadable holds the
// next steps of a not_found to what GitLab says it of besides an object that
// does not exist (ContextService of 19.6.1): one the user cannot read and one
// the access token may not, so a model does not report an object that exists as
// missing. No other answer says it. The llm answers are the captured one, with
// its code, its token or the lines below its first two changed to what
// ContextLlmFormatter of 19.6.1 writes: one line for an entity it did not find,
// and a found one's own texts below its line, which may hold the same words.
func TestFormatEntityContextMarkdown_NotFound_SaysItMayBeUnreadable(t *testing.T) {
	const (
		llmFound  = "orbit_context version=1.4.0 entities=1\n- g/p!1 id=1 found=true\n"
		llmQuoted = `- "g/p!2" found=false error=not_found`
	)
	llmUnsupported := readFixture(t, "context_llm.txt") + "\n"
	llmNotFound := strings.Replace(llmUnsupported, "error=unsupported_type", "error=not_found", 1)
	cases := []struct {
		name     string
		entities []ContextEntity
		text     string
		want     bool
	}{
		{name: "not found", entities: []ContextEntity{{Token: "g/p!1", Error: &ContextError{Code: "not_found"}}}, want: true},
		{name: "not found beside another", entities: []ContextEntity{{Token: "g/p!2", Found: true}, {Token: "g/p!1", Error: &ContextError{Code: "not_found"}}}, want: true},
		{name: "an unsupported type", entities: []ContextEntity{{Token: "Project[1]", Error: &ContextError{Code: "unsupported_type"}}}},
		{name: "no reason given", entities: []ContextEntity{{Token: "Project[1]"}}},
		{name: "found", entities: []ContextEntity{{Token: "g/p!1", Found: true}}},
		{name: "no entity"},
		{name: "llm, not found", text: llmNotFound, want: true},
		{name: "llm, not found with no newline at its end", text: strings.TrimSuffix(llmNotFound, "\n"), want: true},
		{name: "llm, an unsupported type", text: llmUnsupported},
		{name: "llm, an invalid token that quotes it", text: "orbit_context version=1.4.0 entities=1\n" + `- "x found=false error=not_found" found=false error=invalid_token` + "\n"},
		{name: "llm, found with a log that quotes it", text: llmFound + "  log:\n    " + llmQuoted + "\n"},
		{name: "llm, found with a file name that breaks a line before it", text: llmFound + "  changed_files[1]:\n    A a\n" + llmQuoted + "\n"},
		{name: "llm, a second line of no entity", text: "orbit_context version=1.4.0 entities=0\n  found=false error=not_found\n"},
		{name: "llm, the first line alone", text: llmQuoted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			md := FormatEntityContextMarkdown(EntityContextOutput{Entities: tc.entities, FormattedText: tc.text})
			hints := toolutil.ExtractHints(md)
			if got := slices.Contains(hints, contextNotFoundHint); got != tc.want {
				t.Errorf("card says it of an object that cannot be read = %t, want %t:\n%s", got, tc.want, md)
			}
			if !slices.ContainsFunc(hints, func(hint string) bool { return strings.Contains(hint, "orbit.named_query") }) {
				t.Errorf("card names no named query among its next steps:\n%s", md)
			}
		})
	}
	if lacks := missingFrom(contextNotFoundHint, "does not say that the object does not exist", "the user cannot read", "fine-grained access token", "the object's own project or group", "read_orbit is not that permission", "a classic one"); len(lacks) > 0 {
		t.Errorf("hint lacks %q: %s", lacks, contextNotFoundHint)
	}
}

// TestEntityContextUsage_SaysWhatAFineGrainedTokenNeeds holds the usage of
// orbit.context to what the permission of its route does not cover: a
// fine-grained access token also needs to read the object where it is, and
// not_found is also said of one it cannot. It stays within the 600 characters
// past which the llms files print the first sentence alone, which says neither.
func TestEntityContextUsage_SaysWhatAFineGrainedTokenNeeds(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	usage := orbitActionSpecRoutes(t, client)["context"].Usage
	if lacks := missingFrom(usage, "not_found also for an object the user or token may not read", "beside read_orbit, permission to read on the object's own project or group", "left out", "failed expansion"); len(lacks) > 0 {
		t.Errorf("usage lacks %q: %s", lacks, usage)
	}
	if length := utf8.RuneCountInString(usage); length > 600 {
		t.Errorf("usage is %d characters, want 600 at most", length)
	}
}

// TestFormatEntityContextMarkdown_ListsAndAFailedExpansion holds the context
// card to what the pipeline, the commit and a failed expansion add: the jobs,
// the changed files with each diff in a fence, a failed test with its output
// fenced, what GitLab cut, and the reason an expansion answered nothing with
// the group it asks the user to choose.
func TestFormatEntityContextMarkdown_ListsAndAFailedExpansion(t *testing.T) {
	author := &ContextUser{ID: 1, Username: "fixture-user"}
	failedTests := []ContextFailedTest{{Name: "test|one", ClassName: "suite", File: "a_test.py", Output: "```\nboom"}, {Name: "test_two"}}
	md := FormatEntityContextMarkdown(EntityContextOutput{Entities: []ContextEntity{{
		Token: "g/p@abc", ResolvedToken: "g/p@abc", Type: "Commit", ID: "abc", Found: true,
		Summary: &ContextSummary{
			Author: author, AuthorName: "A. Person", AuthoredDate: "2026-10-29T08:00:00Z", Source: "push", Ref: "main", SHA: "abc",
			Truncated:    []string{"jobs", "diffs"},
			ChangedFiles: []ContextChangedFile{{Path: "new.go", OldPath: "old.go", Status: "R", Lines: []string{"1-2", "5"}, Diff: "+x"}, {Path: "gone.go", Status: "D"}},
			Jobs:         []ContextPipelineJob{{Name: "ruff", Stage: "lint", Status: "failed", WebURL: "https://gitlab.example.com/j/1"}},
			FailedTests:  failedTests,
			FailedJobs:   []ContextFailedJob{{Name: "unit", FailedTests: failedTests[:1], Truncated: []string{"failed_tests"}}},
		},
		Expansion: &ContextExpansion{Error: &ContextExpansionError{
			Code: "billing_namespace_required", Message: "Select a default group.", DeepLink: "https://gitlab.com/-/profile/preferences",
			Candidates: []BillingCandidate{{ID: 10, Name: "acme"}, {ID: 11, Name: "acme-tools"}},
		}},
	}}})
	if lacks := missingFrom(
		md,
		"- **Author**: fixture-user", "- **Commit author**: A. Person", "- **Authored**: 29 Oct 2026 08:00 UTC", "- **Source**: push", "- **Ref**: `main`", "- **SHA**: `abc`",
		"- **Cut by GitLab**: jobs, diffs", "- **Cut by GitLab**: failed_tests",
		"| `old.go` to `new.go` | R | 1-2, 5 |", "| `gone.go` | D |", "```diff\n+x\n```",
		"| [ruff](https://gitlab.example.com/j/1) | lint | failed |",
		"| test&#124;one | suite | a_test.py |", "| test_two |", "````text\n```\nboom\n````",
		"- **Expansion error**: `billing_namespace_required`", "- **Expansion message**: Select a default group.",
		"- **Choose the group at**: <https://gitlab.com/-/profile/preferences>", "- **Groups to choose from**: acme (10), acme-tools (11)",
	); len(lacks) > 0 {
		t.Errorf("card lacks %q:\n%s", lacks, md)
	}
	if strings.Contains(md, "Expansion query") || strings.Contains(md, "```json") {
		t.Errorf("card names a query or a result for an expansion that has neither:\n%s", md)
	}
}
