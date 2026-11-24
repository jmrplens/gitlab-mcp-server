// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// grepStandIn answers GET /orbit/grep with the body named, after holding the
// query string to the one the handler is to send.
func grepStandIn(t *testing.T, want url.Values, status int, answer string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestMethod(t, r, http.MethodGet)
		testutil.AssertRequestPath(t, r, "/api/v4/orbit/grep")
		if got := r.URL.Query(); !reflect.DeepEqual(got, want) {
			t.Errorf("query = %v, want %v", got, want)
		}
		testutil.RespondJSON(w, status, answer)
	})
}

// TestGrep_SubgroupAnswer_PublishesTargetScopesAndEveryResultType holds
// [Grep] to GitLab.com's answer for a subgroup on 2026-10-29 (one result of
// each type kept, paths replaced): the query the route takes, the target, the
// scopes with their status and the reason one was unavailable, and a work
// item, a merge request and a file with its match, each with the token
// orbit.context takes and the id orbit.query filters on where GitLab sends
// them.
func TestGrep_SubgroupAnswer_PublishesTargetScopesAndEveryResultType(t *testing.T) {
	want := url.Values{"search": {"seed"}, "namespace_id": {"145841973"}}
	client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, readFixture(t, "grep_subgroup.json")))
	out, err := Grep(context.Background(), client, GrepInput{Search: " seed ", NamespaceID: 145841973})
	if err != nil {
		t.Fatalf("Grep() error: %v", err)
	}
	if out.Version != "1.2.0" || out.Target == nil || out.Target.Token != "Group[145841973]" || out.Request == nil || out.Request.Limit != 20 {
		t.Fatalf("Grep() = %+v, want the 1.2.0 answer for the group with the default limit", out)
	}
	if len(out.Scopes) == 0 || out.Scopes[0].Status != "ok" || out.Scopes[0].Pagination == nil {
		t.Errorf("first scope = %+v, want an answered scope with its pagination", out.Scopes)
	}
	checkSubgroupHits(t, out.Results)
}

// checkSubgroupHits holds the three hits of the subgroup answer: a work item
// with its token, id and iid, and a file with no token or id, its path and
// its match.
func checkSubgroupHits(t *testing.T, results []GrepResult) {
	t.Helper()
	if len(results) != 3 {
		t.Fatalf("results = %+v, want a work item, a merge request and a file", results)
	}
	workItem, file := results[0], results[2]
	if workItem.Token == nil || *workItem.Token != "WorkItem[207153944]" || workItem.ID != 207153944.0 || workItem.Reference.IID != 1 {
		t.Errorf("work item = %+v, want its token, id and iid", workItem)
	}
	if file.Token != nil || file.ID != nil || file.Reference.Path == "" || len(file.Matches) != 1 || file.Matches[0].Text == "" {
		t.Errorf("file = %+v, want no token or id, its path and its match", file)
	}
}

// TestGrep_EveryHitGitLabBuilds_IsPublishedWhole holds [Grep] to the hits no
// capture of 2026-10-29 answered, in the shapes GitLab's GrepService builds at
// the captured version and its spec pins: a commit, whose id is its SHA, a
// note with what it is on, on a work item and on a commit, and a group's work
// item, which has a group and no project. The scope statuses are the ones it
// writes beside ok.
func TestGrep_EveryHitGitLabBuilds_IsPublishedWhole(t *testing.T) {
	const answer = `{"version":"1.2.0","request":{"search":"fix","limit":20,"per_scope":5},"target":{"type":"Group","id":2,"token":"Group[2]","full_path":"acme"},
		"scopes":[{"name":"commits","status":"ok","backend":"advanced","result_count":1,"pagination":{"has_more":false}},{"name":"milestones","status":"error","reason":{"code":"timeout"}},{"name":"users","status":"skipped","reason":{"code":"limit_reached"}}],
		"results":[
			{"scope":"commits","type":"Commit","id":"abc123","token":"acme/app@abc123","reference":{"project_id":1,"project_path":"acme/app","sha":"abc123"},"text":"Fix search"},
			{"scope":"notes","type":"Note","id":7,"token":"Note[7]","reference":{"id":7,"noteable":"WorkItem[9]","noteable_reference":"acme/app#3"},"text":"First"},
			{"scope":"notes","type":"Note","id":8,"token":"Note[8]","reference":{"id":8,"noteable_reference":"acme/app@abc123"},"text":"On a commit"},
			{"scope":"work_items","type":"WorkItem","id":10,"token":"WorkItem[10]","reference":{"id":10,"iid":4,"namespace_path":"acme"},"text":"An epic","state":"opened"},
			{"scope":"users","type":"User","id":11,"token":"User[11]","reference":{"id":11},"text":"fixture-user"}
		],"result_count":5,"truncated":false}`
	want := url.Values{"search": {"fix"}, "namespace_id": {"2"}}
	client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, answer))
	out, err := Grep(context.Background(), client, GrepInput{Search: "fix", NamespaceID: 2})
	if err != nil {
		t.Fatalf("Grep() error: %v", err)
	}
	wantReferences := []GrepReference{
		{ProjectID: 1, ProjectPath: "acme/app", SHA: "abc123"},
		{ID: 7, Noteable: "WorkItem[9]", NoteableReference: "acme/app#3"},
		{ID: 8, NoteableReference: "acme/app@abc123"},
		{ID: 10, IID: 4, NamespacePath: "acme"},
		{ID: 11},
	}
	wantIDs := []any{"abc123", 7.0, 8.0, 10.0, 11.0}
	if len(out.Results) != len(wantReferences) {
		t.Fatalf("results = %+v, want %d", out.Results, len(wantReferences))
	}
	for i, want := range wantReferences {
		t.Run(out.Results[i].Text, func(t *testing.T) {
			if got := out.Results[i]; !reflect.DeepEqual(got.Reference, want) || got.ID != wantIDs[i] {
				t.Errorf("result = %+v, want id %v and reference %+v", got, wantIDs[i], want)
			}
		})
	}
	if out.Scopes[1].Status != "error" || out.Scopes[1].Reason == nil || out.Scopes[1].Reason.Code != "timeout" {
		t.Errorf("scope = %+v, want the one that failed with its reason", out.Scopes[1])
	}
	md := FormatGrepMarkdown(out)
	if lacks := missingFrom(
		md,
		"| commits | Commit | `acme/app@abc123` | acme/app abc123 |", "| notes | Note | `Note[7]` | acme/app#3 WorkItem&#91;9] |",
		"| notes | Note | `Note[8]` | acme/app\\@abc123 |", "| work_items | WorkItem | `WorkItem[10]` | acme 4 |", "| users | User | `User[11]` |  |", "| milestones | error | timeout |",
	); len(lacks) > 0 {
		t.Errorf("card lacks %q:\n%s", lacks, md)
	}
}

// TestGrep_ALimitUnderGitLabsPerScope_IsSentWithPerScopeAtTheLimit verifies
// that a call giving a limit GitLab's default per_scope of 5 exceeds is sent
// with the two in step, since GitLab refuses them together as a per_scope the
// caller never gave, and that nothing is sent for a limit that does not, or
// when the caller gave a per_scope.
func TestGrep_ALimitUnderGitLabsPerScope_IsSentWithPerScopeAtTheLimit(t *testing.T) {
	cases := []struct {
		name            string
		limit, perScope int
		want            url.Values
	}{
		{name: "limit 1", limit: 1, want: url.Values{"limit": {"1"}, "per_scope": {"1"}}},
		{name: "limit 4", limit: 4, want: url.Values{"limit": {"4"}, "per_scope": {"4"}}},
		{name: "limit 5", limit: 5, want: url.Values{"limit": {"5"}}},
		{name: "limit 4 and per_scope 2", limit: 4, perScope: 2, want: url.Values{"limit": {"4"}, "per_scope": {"2"}}},
		{name: "per_scope alone", perScope: 10, want: url.Values{"per_scope": {"10"}}},
		{name: "neither", want: url.Values{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := url.Values{"search": {"stock"}, "project_id": {"1"}}
			maps.Copy(want, tc.want)
			client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, `{"version":"1.2.0","results":[]}`))
			if _, err := Grep(context.Background(), client, GrepInput{Search: "stock", ProjectID: 1, Limit: tc.limit, PerScope: tc.perScope}); err != nil {
				t.Fatalf("Grep() error: %v", err)
			}
		})
	}
}

// TestGrepSchema_NamesTheTargetAsOneOfThree holds the catalog's input schema
// of orbit.grep to the rule the handler holds a call to: exactly one of the
// three parameters that name the target, so the call find offers names one.
func TestGrepSchema_NamesTheTargetAsOneOfThree(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	encoded, err := json.Marshal(orbitActionSpecRoutes(t, client)["grep"].InputSchema)
	if err != nil {
		t.Fatalf("encode the input schema: %v", err)
	}
	var schema struct {
		OneOf []struct {
			Required []string `json:"required"`
		} `json:"oneOf"`
	}
	if err = json.Unmarshal(encoded, &schema); err != nil {
		t.Fatalf("decode the input schema: %v", err)
	}
	var names []string
	for _, branch := range schema.OneOf {
		names = append(names, branch.Required...)
	}
	if want := []string{"namespace_id", "project_id", "full_path"}; !reflect.DeepEqual(names, want) {
		t.Errorf("oneOf requires %v, want %v", names, want)
	}
}

// TestGrep_ProjectAnswer_NamesTheScopeGitLabCouldNotSearch holds [Grep] to
// GitLab.com's answer for a project by id on 2026-10-29: the projects scope,
// which a project search cannot read, comes back unavailable with GitLab's
// reason rather than as an empty scope.
func TestGrep_ProjectAnswer_NamesTheScopeGitLabCouldNotSearch(t *testing.T) {
	want := url.Values{"search": {"compute_total"}, "project_id": {"83234203"}}
	client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, readFixture(t, "grep_project.json")))
	out, err := Grep(context.Background(), client, GrepInput{Search: "compute_total", ProjectID: 83234203})
	if err != nil {
		t.Fatalf("Grep() error: %v", err)
	}
	var reasons []string
	for _, scope := range out.Scopes {
		if scope.Status == "unavailable" && scope.Reason != nil {
			reasons = append(reasons, scope.Name+":"+scope.Reason.Code)
		}
	}
	if len(reasons) == 0 || out.Target == nil || out.Target.Type != "Project" {
		t.Errorf("Grep() target %+v, unavailable scopes %v, want the project with the scope it could not search", out.Target, reasons)
	}
}

// TestGrep_ScopesLimitsAndFormat_AreSentAsGitLabTakesThem verifies the query
// of a call naming scopes as aliases, a limit, a per-scope bound and the llm
// format: the scopes as the comma list GitLab.com answered on 2026-10-29, the
// numbers as given, the full path without the spaces around it, and GitLab's
// text published whole. A branch hit's token is path@branch, and a raw answer
// the caller named a format for names it.
func TestGrep_ScopesLimitsAndFormat_AreSentAsGitLabTakesThem(t *testing.T) {
	t.Run("aliases, limit and per_scope", func(t *testing.T) {
		want := url.Values{"search": {"stock"}, "project_id": {"83234203"}, "scopes": {"mr,issue,code,branch"}, "limit": {"10"}, "per_scope": {"3"}}
		client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, readFixture(t, "grep_aliases_branch.json")))
		out, err := Grep(context.Background(), client, GrepInput{Search: "stock", ProjectID: 83234203, Scopes: []string{"mr", " issue ", "code", "branch", ""}, Limit: 10, PerScope: 3})
		if err != nil {
			t.Fatalf("Grep() error: %v", err)
		}
		if len(out.Results) != 1 || out.Results[0].Token == nil || *out.Results[0].Token != "acme-group/orbit-fixtures/kg-fixtures@feature/restock-helper" || out.Results[0].Reference.Name == "" {
			t.Errorf("results = %+v, want the branch with its path@branch token", out.Results)
		}
	})
	t.Run("llm by full path", func(t *testing.T) {
		text := readFixture(t, "grep_llm.txt")
		want := url.Values{"search": {"seed"}, "full_path": {"acme-group"}, "response_format": {"llm"}}
		client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, text))
		out, err := Grep(context.Background(), client, GrepInput{Search: "seed", FullPath: " acme-group\n", ResponseFormat: "llm"})
		if err != nil {
			t.Fatalf("Grep() error: %v", err)
		}
		if out.FormattedText != text || out.Results != nil || out.ResponseFormat != "llm" {
			t.Errorf("Grep() = %+v, want the llm text alone", out)
		}
	})
	for _, format := range []string{"raw", "json"} {
		t.Run(format+", sent and named as raw", func(t *testing.T) {
			want := url.Values{"search": {"stock"}, "project_id": {"83234203"}, "response_format": {"raw"}}
			client := testutil.NewTestClient(t, grepStandIn(t, want, http.StatusOK, readFixture(t, "grep_aliases_branch.json")))
			out, err := Grep(context.Background(), client, GrepInput{Search: "stock", ProjectID: 83234203, ResponseFormat: format})
			if err != nil {
				t.Fatalf("Grep() error: %v", err)
			}
			if out.ResponseFormat != "raw" || len(out.Results) != 1 {
				t.Errorf("Grep() = %+v, want the raw answer named raw", out)
			}
		})
	}
}

// TestGrepGuidance_NamespaceIDIsAGroup holds what is served about the
// namespace_id of orbit.grep to the targets GitLab searches, a group or a
// project: the catalog's default for the name offers a user namespace too,
// which GitLab answers not found.
func TestGrepGuidance_NamespaceIDIsAGroup(t *testing.T) {
	guidance := servedGuidance(t, "grep", "namespace_id")
	if guidance.SemanticRole != "scope_group" {
		t.Errorf("namespace_id guidance = %+v, want the role of a group", guidance)
	}
	if lacks := missingFrom(guidance.ValueSource, "ID of the group to search", "group.get"); len(lacks) > 0 {
		t.Errorf("namespace_id guidance lacks %q: %s", lacks, guidance.ValueSource)
	}
	if confusions := strings.Join(guidance.CommonConfusions, " "); !strings.Contains(confusions, "user namespace is answered not found") {
		t.Errorf("namespace_id confusions = %q, want a user namespace refused", confusions)
	}
}

// TestGrepSchema_ScopesSayWhatBranchesNeed holds the description of scopes to
// what GitLab asks for the one scope it searches only when named: a project to
// search, whose code the user can read, and read_branch there for a
// fine-grained access token, without which it answers the scope unavailable.
func TestGrepSchema_ScopesSayWhatBranchesNeed(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	description, _ := propertySchema(t, orbitActionSpecRoutes(t, client)["grep"].InputSchema, "scopes")["description"].(string)
	if lacks := missingFrom(description, "Every scope but branches when omitted", "branches is a project's alone", "read its code", "read_branch", "not_available_for_target"); len(lacks) > 0 {
		t.Errorf("scopes description lacks %q: %s", lacks, description)
	}
}

// TestGrep_OutOfBounds_IsRefusedBeforeSending covers what [Grep] refuses
// before a request. GitLab.com refused with 400 on 2026-10-29 a search under 2
// bytes, whose message gives the 256 bytes too, two targets and a per-scope
// bound above the limit; its source (ee/lib/api/orbit/data.rb) declares the one
// target, a limit of 1 to 50, a per-scope bound of 1 to 10 and the two formats;
// and the ceiling on the scopes is this server's own. Each bound itself is
// sent.
func TestGrep_OutOfBounds_IsRefusedBeforeSending(t *testing.T) {
	cases := []struct {
		name  string
		input GrepInput
		want  string
	}{
		{name: "a search of one byte", input: GrepInput{Search: " a ", ProjectID: 1}, want: "between 2 and 256 bytes"},
		{name: "a search of 257 bytes", input: GrepInput{Search: strings.Repeat("a", 257), ProjectID: 1}, want: "between 2 and 256 bytes"},
		{name: "no target", input: GrepInput{Search: "stock"}, want: "exactly one of namespace_id, project_id and full_path"},
		{name: "two targets", input: GrepInput{Search: "stock", ProjectID: 1, FullPath: "acme-group"}, want: "exactly one of namespace_id, project_id and full_path"},
		{name: "a limit of 51", input: GrepInput{Search: "stock", ProjectID: 1, Limit: 51}, want: "limit is 1 to 50"},
		{name: "a negative limit", input: GrepInput{Search: "stock", ProjectID: 1, Limit: -1}, want: "limit is 1 to 50"},
		{name: "a per_scope of 11", input: GrepInput{Search: "stock", ProjectID: 1, PerScope: 11}, want: "per_scope is 1 to 10"},
		{name: "a per_scope over the limit", input: GrepInput{Search: "stock", ProjectID: 1, Limit: 2, PerScope: 5}, want: "per_scope cannot exceed limit"},
		{name: "a negative per_scope", input: GrepInput{Search: "stock", ProjectID: 1, PerScope: -1}, want: "per_scope is 1 to 10"},
		{name: "twenty-one scopes", input: GrepInput{Search: "stock", ProjectID: 1, Scopes: strings.Fields(strings.Repeat("mr ", maxGrepScopes+1))}, want: "at most 20 are taken"},
		{name: "the bounds themselves", input: GrepInput{Search: strings.Repeat("a", 256), ProjectID: 1, Limit: 50, PerScope: 10, Scopes: strings.Fields(strings.Repeat("mr ", maxGrepScopes))}, want: ""},
		{name: "the shortest search, per_scope at the limit", input: GrepInput{Search: "ab", ProjectID: 1, Limit: 3, PerScope: 3}, want: ""},
		{name: "the gql format", input: GrepInput{Search: "stock", ProjectID: 1, ResponseFormat: "gql"}, want: "invalid response_format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent := false
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				sent = true
				testutil.RespondJSON(w, http.StatusOK, `{"version":"1.2.0","results":[]}`)
			}))
			_, err := Grep(context.Background(), client, tc.input)
			if tc.want == "" {
				if err != nil || !sent {
					t.Fatalf("Grep() error = %v, sent %t, want the request sent", err, sent)
				}
				return
			}
			if sent {
				t.Errorf("request sent for an input refused before one")
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Grep() error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

// TestGrep_GitLabsRefusals_CarryTheirMessage verifies that GitLab's own 400s
// reach the caller with GitLab's words and the hint, in both of the shapes
// GitLab.com answered them in on 2026-10-29.
func TestGrep_GitLabsRefusals_CarryTheirMessage(t *testing.T) {
	cases := []struct {
		name, answer, message string
	}{
		{name: "a bad scope", answer: "grep_bad_scope.json", message: "scopes does not have a valid value"},
		{name: "two targets", answer: "grep_two_targets.json", message: "mutually exclusive"},
		{name: "a short search", answer: "grep_short_search.json", message: "search must contain between 2 and 256 bytes"},
		{name: "per_scope over limit", answer: "grep_per_scope_over_limit.json", message: "per_scope cannot exceed limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := readFixture(t, tc.answer)
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusBadRequest, body)
			}))
			_, err := Grep(context.Background(), client, GrepInput{Search: "stock", ProjectID: 83234203, Scopes: []string{"nope"}})
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), grepRefusedHint) {
				t.Fatalf("Grep() error = %v, want GitLab's %q and the hint", err, tc.message)
			}
		})
	}
}

// TestFormatGrepMarkdown_ResultsAndMatches holds the grep card to a table of
// the hits with the token each carries, an open hit told from a closed one by
// its state, GitLab's text escaped, a file's match
// in a fence sized to it, the scopes GitLab could not search, and the steps to
// orbit.context and orbit.query.
func TestFormatGrepMarkdown_ResultsAndMatches(t *testing.T) {
	token := "WorkItem[1]"
	mergeRequest := "MergeRequest[5]"
	md := FormatGrepMarkdown(GrepOutput{
		Version: "1.2.0", ResultCount: 3,
		Target: &GrepTarget{Type: "Group", Token: "Group[2]", FullPath: "acme-group"},
		Scopes: []GrepScope{{Name: "projects", Status: "unavailable", Reason: &GrepScopeReason{Code: "not_available_for_search_type"}}},
		Results: []GrepResult{
			{Scope: "work_items", Type: "WorkItem", ID: 1.0, Token: &token, Text: "Add | helper", State: "opened", Reference: GrepReference{IID: 1, ProjectPath: "acme-group/p"}},
			{Scope: "merge_requests", Type: "MergeRequest", ID: 5.0, Token: &mergeRequest, Text: "Seed helper", State: "closed", Reference: GrepReference{IID: 2, ProjectPath: "acme-group/p"}},
			{Scope: "blobs", Type: "File", Text: "x", Reference: GrepReference{Path: "src/a.py", ProjectPath: "acme-group/p"}, Matches: []GrepMatch{{StartLine: 3, EndLine: 4, Text: "```\ncode"}}},
		},
	})
	if lacks := missingFrom(md, "## Orbit Grep", "| Scope | Type | Token | Where | State | Text |", "| `WorkItem[1]` | acme-group/p 1 | opened | Add &#124; helper |", "| `MergeRequest[5]` | acme-group/p 2 | closed | Seed helper |", "src/a.py", "````text\n```\ncode\n````", "not_available_for_search_type", "orbit.context", "orbit.query"); len(lacks) > 0 {
		t.Errorf("card lacks %q:\n%s", lacks, md)
	}
	if strings.Contains(md, "GitLab ended the search") {
		t.Errorf("card says the search was cut, and it was not:\n%s", md)
	}
	cut := FormatGrepMarkdown(GrepOutput{Truncated: true, Scopes: []GrepScope{{Name: "commits", Status: "ok"}}})
	if !strings.Contains(cut, "GitLab ended the search at the limit") || strings.Contains(cut, "Scopes not searched") || strings.Contains(cut, "### Results") {
		t.Errorf("card of a cut search = %q, want it said, no scope listed as not searched and no table of no hit", cut)
	}
	if llm := FormatGrepMarkdown(GrepOutput{FormattedText: "orbit_grep version=1.2.0"}); !strings.Contains(llm, "```text\norbit_grep version=1.2.0\n```") {
		t.Errorf("llm card = %q, want the text in one fence", llm)
	}
}
