// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package required

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/freshness"
	_ "github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/serialtypecheck" // serial type-checking under -race, golang/go#81122
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// schemaOf is an input schema with the named properties, requiring the ones
// in required.
func schemaOf(required []string, properties ...string) map[string]any {
	props := map[string]any{}
	for _, name := range properties {
		props[name] = map[string]any{"type": "string"}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if required != nil {
		schema["required"] = required
	}
	return schema
}

// rest and graphql are one request of each kind.
func rest(route string) actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindREST, Route: route}
}

func graphql() actionrequests.RecordRequest {
	return actionrequests.RecordRequest{Kind: actionrequests.KindGraphQL, Operation: "query"}
}

// liveRoute is a record route with params, keyed by the name a request
// record spells it with.
func liveRoute(method, path string, params map[string]apilive.Param) *apilive.Route {
	return &apilive.Route{Method: method, Path: apilive.EndpointPrefix + path, Params: params}
}

// fixtureRoutes are the routes the fixture actions send.
func fixtureRoutes() map[string]*apilive.Route {
	return routesByName([]apilive.Route{
		*liveRoute("POST", "/projects/:id/links", map[string]apilive.Param{
			"id": {}, "target_iid": {Required: true}, "link_type": {}, "note": {Required: true},
		}),
		*liveRoute("GET", "/projects/:id/links", map[string]apilive.Param{"id": {}, "search": {}}),
		*liveRoute("PUT", "/projects/:id/things/:name", map[string]apilive.Param{"id": {}, "name": {}, "title": {}}),
		*liveRoute("GET", "/topics", map[string]apilive.Param{}),
	})
}

// TestJudgeAction_EachDirectionAndEachSilence verifies one action whose
// schema and GitLab agree on one field, disagree both ways on two more, and
// says nothing of a field no route carries: the agreement is judged and not
// reported, each disagreement is a finding in its direction naming the route,
// and the field no route carries is unplaced.
func TestJudgeAction_EachDirectionAndEachSilence(t *testing.T) {
	schema := schemaOf([]string{"link_type", "project_id"}, "project_id", "target_iid", "link_type", "confirm")
	action := actionrequests.RecordAction{
		ID: "issue.link_create", Requests: []actionrequests.RecordRequest{rest("POST /projects/:id/links")}, Paths: [][]int{{0}},
	}
	verdict := judgeAction("issue.link_create", schema, action, fixtureRoutes(), rules{})
	if !verdict.rest || verdict.judged != 3 {
		t.Fatalf("verdict = rest %t, judged %d; want a REST action with three fields judged", verdict.rest, verdict.judged)
	}
	want := []Finding{
		{Action: "issue.link_create", Field: "link_type", Direction: DirectionGitLabOptional, Routes: []string{"POST /projects/:id/links"}},
		{Action: "issue.link_create", Field: "target_iid", Direction: DirectionGitLabRequires, Routes: []string{"POST /projects/:id/links"}},
	}
	if !reflect.DeepEqual(verdict.findings, want) {
		t.Errorf("findings = %+v\nwant %+v", verdict.findings, want)
	}
	if !slices.Equal(verdict.unplaced, []string{"confirm"}) || verdict.unjudged != nil {
		t.Errorf("unplaced %v, unjudged %v; want confirm unplaced and nothing unjudged", verdict.unplaced, verdict.unjudged)
	}
}

// TestJudgeAction_NoRESTRequest_IsNotJudged verifies an action that sends no
// REST request the record holds says nothing about any field: a GraphQL
// action, and a REST action whose route the record does not hold.
func TestJudgeAction_NoRESTRequest_IsNotJudged(t *testing.T) {
	cases := map[string]actionrequests.RecordAction{
		"graphql":       {Requests: []actionrequests.RecordRequest{graphql()}, Paths: [][]int{{0}}},
		"unknown route": {Requests: []actionrequests.RecordRequest{rest("GET /nowhere")}, Paths: [][]int{{0}}},
		"no request":    {},
	}
	for name, action := range cases {
		t.Run(name, func(t *testing.T) {
			verdict := judgeAction("x.y", schemaOf(nil, "project_id"), action, fixtureRoutes(), rules{})
			if verdict.rest || verdict.judged != 0 || verdict.findings != nil || verdict.unplaced != nil {
				t.Errorf("verdict = %+v, want nothing judged", verdict)
			}
		})
	}
}

// TestJudgeAction_WaysDecideGitLabsAnswer verifies the ways an action runs
// fold into one answer per field: a field GitLab requires on one way and that
// another way runs without is optional; one that a way cannot be read for is
// unjudged rather than guessed; and an action whose ways the record does not
// list is unjudged too.
func TestJudgeAction_WaysDecideGitLabsAnswer(t *testing.T) {
	post, list := rest("POST /projects/:id/links"), rest("GET /projects/:id/links")
	cases := []struct {
		name         string
		action       actionrequests.RecordAction
		wantFindings []Finding
		wantUnjudged []string
	}{
		{
			name:   "a way without the field",
			action: actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{post, list}, Paths: [][]int{{0}, {1}}},
			// note is required on the POST and the GET way runs without it,
			// so GitLab leaves it optional, as the schema does.
			wantFindings: nil,
		},
		{
			name:         "a way that cannot be read",
			action:       actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{post, graphql()}, Paths: [][]int{{0}, {1}}},
			wantUnjudged: []string{"note"},
		},
		{
			name:         "a way that requires it beside one that cannot be read",
			action:       actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{graphql(), post}, Paths: [][]int{{0, 1}}},
			wantFindings: []Finding{{Action: "x.y", Field: "note", Direction: DirectionGitLabRequires, Routes: []string{"POST /projects/:id/links"}}},
		},
		{
			name:         "no ways",
			action:       actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{post}},
			wantUnjudged: []string{"note"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			verdict := judgeAction("x.y", schemaOf(nil, "note"), testCase.action, fixtureRoutes(), rules{})
			if !reflect.DeepEqual(verdict.findings, testCase.wantFindings) {
				t.Errorf("findings = %+v, want %+v", verdict.findings, testCase.wantFindings)
			}
			if !slices.Equal(verdict.unjudged, testCase.wantUnjudged) {
				t.Errorf("unjudged = %v, want %v", verdict.unjudged, testCase.wantUnjudged)
			}
		})
	}
}

// TestJudgeAction_AFieldOnTwoRoutes_NamesBothOnce verifies a finding names
// every route the field was found on, sorted and once each.
func TestJudgeAction_AFieldOnTwoRoutes_NamesBothOnce(t *testing.T) {
	post := rest("POST /projects/:id/links")
	action := actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{post, rest("GET /projects/:id/links"), post}, Paths: [][]int{{0, 1, 2}}}
	verdict := judgeAction("x.y", schemaOf(nil, "project_id"), action, fixtureRoutes(), rules{})
	want := []Finding{{Action: "x.y", Field: "project_id", Direction: DirectionGitLabRequires, Routes: []string{"GET /projects/:id/links", "POST /projects/:id/links"}}}
	if !reflect.DeepEqual(verdict.findings, want) {
		t.Errorf("findings = %+v, want %+v", verdict.findings, want)
	}
}

// TestJudgeAll_DeclarationsAndAliasesAnswerOrGoStale assembles a report over
// six actions and holds every part of it: a disagreement a declaration
// answers in its direction is declared, one a declaration answers in the
// other direction stays a finding and leaves that declaration stale, an
// alias that moves a path param onto the field the input names is used, an
// alias onto a field the schema lacks is stale, a declaration for a field
// nothing disagrees on is stale, and the summary counts all of it.
func TestJudgeAll_DeclarationsAndAliasesAnswerOrGoStale(t *testing.T) {
	schemas := map[string]map[string]any{
		"issue.link_create": schemaOf([]string{"link_type", "project_id"}, "project_id", "target_iid", "link_type", "note", "confirm"),
		"thing.update":      schemaOf([]string{"project_id", "thing"}, "project_id", "thing", "title"),
		"thing.list":        schemaOf(nil, "project_id"),
		"graph.read":        schemaOf(nil, "full_path"),
		"topic.list":        schemaOf(nil, "page"),
		"link.list":         schemaOf([]string{"project_id"}, "project_id", "search"),
	}
	record := actionrequests.Record{Actions: []actionrequests.RecordAction{
		{ID: "issue.link_create", Requests: []actionrequests.RecordRequest{rest("POST /projects/:id/links")}, Paths: [][]int{{0}}},
		{ID: "thing.update", Requests: []actionrequests.RecordRequest{rest("PUT /projects/:id/things/:name")}, Paths: [][]int{{0}}},
		{ID: "thing.list", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/links"), graphql()}, Paths: [][]int{{0}, {1}}},
		{ID: "graph.read", Requests: []actionrequests.RecordRequest{graphql()}, Paths: [][]int{{0}}},
		{ID: "topic.list", Requests: []actionrequests.RecordRequest{rest("GET /topics")}, Paths: [][]int{{0}}},
		{ID: "link.list", Requests: []actionrequests.RecordRequest{rest("GET /projects/:id/links")}, Paths: [][]int{{0}}},
	}}
	declared := map[declarationKey]declaration{
		{action: "issue.link_create", field: "link_type"}:  {Direction: DirectionGitLabOptional, Category: categorySoleAttribute, Reason: "answers it"},
		{action: "issue.link_create", field: "target_iid"}: {Direction: DirectionGitLabOptional, Category: categoryConditional, Reason: "wrong way round"},
		{action: "thing.update", field: "title"}:           {Direction: DirectionGitLabRequires, Category: categoryHandlerDefault, Reason: "nothing to answer"},
	}
	aliased := map[aliasKey]paramAlias{
		{action: "thing.update", param: "name"}:      {Field: "thing", Reason: "the input names it thing"},
		{action: "thing.list", param: "search"}:      {Field: "query", Reason: "the schema has no such field"},
		{action: "issue.link_create", param: "nope"}: {Field: "note", Reason: "no route declares it"},
	}
	report := judgeAll(schemas, record, fixtureRoutes(), tables{declared: declared, aliased: aliased})

	wantFindings := []Finding{
		{Action: "issue.link_create", Field: "note", Direction: DirectionGitLabRequires, Routes: []string{"POST /projects/:id/links"}},
		{Action: "issue.link_create", Field: "target_iid", Direction: DirectionGitLabRequires, Routes: []string{"POST /projects/:id/links"}},
	}
	if !reflect.DeepEqual(report.Findings, wantFindings) {
		t.Errorf("findings = %+v\nwant %+v", report.Findings, wantFindings)
	}
	wantDeclared := []Finding{{
		Action: "issue.link_create", Field: "link_type", Direction: DirectionGitLabOptional, Routes: []string{"POST /projects/:id/links"},
		Category: categorySoleAttribute, Reason: "answers it",
	}}
	if !reflect.DeepEqual(report.Declared, wantDeclared) {
		t.Errorf("declared = %+v\nwant %+v", report.Declared, wantDeclared)
	}
	wantStale := []string{
		"issue.link_create nope: no route the action sends declares the parameter, or the schema has no field note",
		"issue.link_create target_iid (gitlab-optional, conditional): answers no disagreement",
		"thing.list search: no route the action sends declares the parameter, or the schema has no field query",
		"thing.update title (gitlab-requires, handler-default): answers no disagreement",
	}
	if !slices.Equal(report.Stale, wantStale) {
		t.Errorf("stale = %q\nwant %q", report.Stale, wantStale)
	}
	wantUnplaced := []ActionFields{{Action: "issue.link_create", Fields: []string{"confirm"}}, {Action: "topic.list", Fields: []string{"page"}}}
	if !reflect.DeepEqual(report.Unplaced, wantUnplaced) {
		t.Errorf("unplaced = %+v, want %+v", report.Unplaced, wantUnplaced)
	}
	wantUnjudged := []ActionFields{{Action: "thing.list", Fields: []string{"project_id"}}}
	if !reflect.DeepEqual(report.Unjudged, wantUnjudged) {
		t.Errorf("unjudged = %+v, want %+v", report.Unjudged, wantUnjudged)
	}
	wantSummary := Summary{
		Actions: 6, Judged: 3, NoRESTRequest: 1, FieldsJudged: 9, FieldsUnplaced: 2, FieldsUnjudged: 1,
		Findings: 2, Declared: 1, Stale: 4,
	}
	if report.Summary != wantSummary {
		t.Errorf("summary = %+v\nwant %+v", report.Summary, wantSummary)
	}
}

// TestJudgeAction_ARenamedPlaceholder_IsHeldToGitLabsAnswer is the planted
// defect of #1688 (P8, mutation M1): access.deploy_key_get's deploy_key_id
// losing ,required. GitLab names the parameter key_id, so by names alone the
// field was unplaced and the gate passed with it optional; through the field
// the record says the call fills :key_id from, the field is judged, the
// planted schema is a finding, and the correct one passes.
func TestJudgeAction_ARenamedPlaceholder_IsHeldToGitLabsAnswer(t *testing.T) {
	routes := routesByName([]apilive.Route{
		*liveRoute("GET", "/projects/:id/deploy_keys/:key_id", map[string]apilive.Param{"id": {}, "key_id": {Required: true}}),
	})
	request := rest("GET /projects/:id/deploy_keys/:key_id")
	planted := schemaOf([]string{"project_id"}, "project_id", "deploy_key_id")
	correct := schemaOf([]string{"project_id", "deploy_key_id"}, "project_id", "deploy_key_id")
	unfilled := actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{request}, Paths: [][]int{{0}}}
	before := judgeAction("access.deploy_key_get", planted, unfilled, routes, rules{})
	if before.findings != nil || !slices.Equal(before.unplaced, []string{"deploy_key_id"}) {
		t.Errorf("by names = findings %+v, unplaced %v; want the field unplaced and nothing found", before.findings, before.unplaced)
	}
	request.Placeholders = map[string]string{"id": "project_id", "key_id": "deploy_key_id"}
	filled := actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{request}, Paths: [][]int{{0}}}
	after := judgeAction("access.deploy_key_get", planted, filled, routes, rules{})
	want := []Finding{{
		Action: "access.deploy_key_get", Field: "deploy_key_id", Direction: DirectionGitLabRequires,
		Routes: []string{"GET /projects/:id/deploy_keys/:key_id"},
	}}
	if !reflect.DeepEqual(after.findings, want) || after.unplaced != nil {
		t.Errorf("through the placeholders = findings %+v, unplaced %v; want %+v", after.findings, after.unplaced, want)
	}
	if fixed := judgeAction("access.deploy_key_get", correct, filled, routes, rules{}); fixed.findings != nil || fixed.judged != 2 {
		t.Errorf("the correct schema = findings %+v, %d judged; want both fields judged and nothing found", fixed.findings, fixed.judged)
	}
}

// TestJudgeAction_AWorkhorseBody_IsJudgedFromItsDeclaration is the planted
// defect of #1688 (P13): repository.commit_create's commit_message losing
// ,required. The record declares the file and the project alone on POST
// /projects/:id/repository/commits, so the field was unplaced and the gate
// passed; through the declared body it is judged, the planted schema is a
// finding, and the correct one passes.
func TestJudgeAction_AWorkhorseBody_IsJudgedFromItsDeclaration(t *testing.T) {
	const route = "POST /projects/:id/repository/commits"
	routes := routesByName([]apilive.Route{*liveRoute("POST", "/projects/:id/repository/commits", map[string]apilive.Param{
		"id": {}, "file": {Required: true, Type: apilive.WorkhorseFile},
	})})
	action := actionrequests.RecordAction{Requests: []actionrequests.RecordRequest{rest(route)}, Paths: [][]int{{0}}}
	planted := schemaOf([]string{"project_id", "branch"}, "project_id", "branch", "commit_message")
	correct := schemaOf([]string{"project_id", "branch", "commit_message"}, "project_id", "branch", "commit_message")
	if before := judgeAction("repository.commit_create", planted, action, routes, rules{}); before.findings != nil || len(before.unplaced) != 2 {
		t.Errorf("with no declaration = findings %+v, unplaced %v; want both body fields unplaced", before.findings, before.unplaced)
	}
	declared := rules{
		bodies:   map[string]workhorseBody{route: {Params: map[string]bool{"branch": true, "commit_message": true}}},
		bodyUsed: map[string]bool{},
	}
	after := judgeAction("repository.commit_create", planted, action, routes, declared)
	want := []Finding{{Action: "repository.commit_create", Field: "commit_message", Direction: DirectionGitLabRequires, Routes: []string{route}}}
	if !reflect.DeepEqual(after.findings, want) || !declared.bodyUsed[route] {
		t.Errorf("with the declaration = findings %+v, used %v; want %+v and the declaration used", after.findings, declared.bodyUsed, want)
	}
	if fixed := judgeAction("repository.commit_create", correct, action, routes, declared); fixed.findings != nil || fixed.judged != 3 {
		t.Errorf("the correct schema = findings %+v, %d judged; want every field judged and nothing found", fixed.findings, fixed.judged)
	}
}

// TestRulesBody_ReadsADeclarationOnlyWhileTheRecordStillNeedsIt verifies the
// body a declaration names is read for its route while the record requires a
// WorkhorseFile param and declares no param of the body, whatever else it
// declares beside the file, is not read once the record declares a param of it
// itself or stops requiring a file, and that a route no declaration names reads
// nothing and is not counted as sent.
func TestRulesBody_ReadsADeclarationOnlyWhileTheRecordStillNeedsIt(t *testing.T) {
	const name = "POST /projects/:id/repository/commits"
	file := apilive.Param{Required: true, Type: apilive.WorkhorseFile}
	cases := []struct {
		name     string
		params   map[string]apilive.Param
		wantBody bool
	}{
		{name: "the file alone", params: map[string]apilive.Param{"id": {}, "file": file}, wantBody: true},
		{name: "the file beside a param the body does not hold", params: map[string]apilive.Param{"id": {}, "file": file, "dry_run": {}}, wantBody: true},
		{name: "a body param declared again", params: map[string]apilive.Param{"id": {}, "file": file, "branch": {Required: true}}},
		{name: "the file optional", params: map[string]apilive.Param{"id": {}, "file": {Type: apilive.WorkhorseFile}}},
		{name: "a file of another type", params: map[string]apilive.Param{"id": {}, "file": {Required: true, Type: "File"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			with := rules{bodies: map[string]workhorseBody{name: {Params: map[string]bool{"branch": true}}}, bodyUsed: map[string]bool{}}
			got := with.body(name, liveRoute("POST", "/projects/:id/repository/commits", tc.params))
			if (got != nil) != tc.wantBody {
				t.Errorf("body = %v, want read %t", got, tc.wantBody)
			}
			answered, sent := with.bodyUsed[name]
			if !sent || answered != tc.wantBody {
				t.Errorf("used = %t, sent %t; want sent and used %t", answered, sent, tc.wantBody)
			}
		})
	}
	with := rules{bodies: map[string]workhorseBody{}, bodyUsed: map[string]bool{}}
	if got := with.body("GET /topics", liveRoute("GET", "/topics", nil)); got != nil || len(with.bodyUsed) != 0 {
		t.Errorf("a route no declaration names = %v, used %v; want nothing", got, with.bodyUsed)
	}
}

// TestStaleBodies_NamesEachWayADeclarationAnswersNothing verifies a body
// declaration for a route no judged action sends, and one whose route is sent
// but no longer answers it, are each stale with the reason, and one a request
// read is not.
func TestStaleBodies_NamesEachWayADeclarationAnswersNothing(t *testing.T) {
	bodies := map[string]workhorseBody{"POST /a": {}, "POST /b": {}, "POST /c": {}}
	got := staleBodies(bodies, map[string]bool{"POST /b": false, "POST /c": true})
	slices.Sort(got)
	want := []string{
		"POST /a: no action this scope judges sends the route, so its Workhorse body answers nothing",
		"POST /b: the record declares a param of the route's body again, or no longer requires a WorkhorseFile param",
	}
	if !slices.Equal(got, want) {
		t.Errorf("staleBodies = %q\nwant %q", got, want)
	}
}

// TestJudgeAll_AWorkhorseBodyNoActionSends_IsStale verifies the whole report
// fails on a body declaration no action's request reads, sorted among the
// other stale entries, and keeps one a request did read.
func TestJudgeAll_AWorkhorseBodyNoActionSends_IsStale(t *testing.T) {
	routes := routesByName([]apilive.Route{*liveRoute("POST", "/projects/:id/repository/commits", map[string]apilive.Param{
		"id": {}, "file": {Required: true, Type: apilive.WorkhorseFile},
	})})
	record := actionrequests.Record{Actions: []actionrequests.RecordAction{{
		ID: "repository.commit_create", Requests: []actionrequests.RecordRequest{rest("POST /projects/:id/repository/commits")}, Paths: [][]int{{0}},
	}}}
	schemas := map[string]map[string]any{"repository.commit_create": schemaOf([]string{"project_id", "branch"}, "project_id", "branch")}
	bodies := map[string]workhorseBody{
		"POST /projects/:id/repository/commits":          {Params: map[string]bool{"branch": true}},
		"POST /projects/:id/repository/files/:file_path": {Params: map[string]bool{"branch": true}},
	}
	report := judgeAll(schemas, record, routes, tables{bodies: bodies})
	want := []string{"POST /projects/:id/repository/files/:file_path: no action this scope judges sends the route, so its Workhorse body answers nothing"}
	if !slices.Equal(report.Stale, want) || report.Summary.Stale != 1 || report.Summary.Findings != 0 || report.Summary.FieldsJudged != 2 {
		t.Errorf("report = stale %q, summary %+v; want the unsent declaration stale and both fields judged", report.Stale, report.Summary)
	}
}

// TestAliasesByAction_TwoAliasesOfOneAction_ShareOneMap verifies the declared
// aliases are indexed under one map per action, so a second alias of an action
// joins the first rather than replacing it. The tree declares two aliases for
// feature_flags.feature_flag_update, but only the run over the tree reaches
// them, and a stacked layer defers that run.
func TestAliasesByAction_TwoAliasesOfOneAction_ShareOneMap(t *testing.T) {
	aliased := map[aliasKey]paramAlias{
		{action: "thing.update", param: "name"}: {Field: "thing"},
		{action: "thing.update", param: "ref"}:  {Field: "branch"},
		{action: "thing.list", param: "search"}: {Field: "query"},
	}
	want := map[string]map[string]string{
		"thing.update": {"name": "thing", "ref": "branch"},
		"thing.list":   {"search": "query"},
	}
	if got := aliasesByAction(aliased); !reflect.DeepEqual(got, want) {
		t.Errorf("aliasesByAction = %v, want %v", got, want)
	}
}

// TestSummaryClean_PassesOnlyWithNoFindingAndNothingStale verifies the gate
// holds both what it fails on, each alone.
func TestSummaryClean_PassesOnlyWithNoFindingAndNothingStale(t *testing.T) {
	cases := map[string]struct {
		summary Summary
		want    bool
	}{
		"clean":   {summary: Summary{Declared: 3}, want: true},
		"finding": {summary: Summary{Findings: 1}},
		"stale":   {summary: Summary{Stale: 1}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			if got := testCase.summary.clean(); got != testCase.want {
				t.Errorf("clean() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestSchemaRequired_ReadsEitherSliceType verifies the required list is read
// whether the schema holds it as []string (a reflected route) or []any (one
// decoded or rewritten by an override), that a non-string entry is skipped,
// and that a schema without one requires nothing.
func TestSchemaRequired_ReadsEitherSliceType(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
		want   map[string]bool
	}{
		{name: "strings", schema: map[string]any{"required": []string{"a", "b"}}, want: map[string]bool{"a": true, "b": true}},
		{name: "any", schema: map[string]any{"required": []any{"a", 7}}, want: map[string]bool{"a": true}},
		{name: "none", schema: map[string]any{}, want: map[string]bool{}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := schemaRequired(testCase.schema); !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("schemaRequired = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestSchemasByAction_TheFirstCatalogHoldingAnActionWins verifies every
// action of every catalog is read once, from the first catalog that holds
// it, so the self-managed build answers for an action both builds hold.
func TestSchemasByAction_TheFirstCatalogHoldingAnActionWins(t *testing.T) {
	first := schemaOf([]string{"project_id"}, "project_id")
	second := schemaOf(nil, "project_id")
	only := schemaOf(nil, "full_path")
	built := []*actioncatalog.Catalog{
		catalogOf(t, "gitlab_issue", actioncatalog.Action{ID: "issue.get", Name: "get", Route: toolutil.ActionRoute{InputSchema: first}}),
		catalogOf(t, "gitlab_issue",
			actioncatalog.Action{ID: "issue.get", Name: "get", Route: toolutil.ActionRoute{InputSchema: second}},
			actioncatalog.Action{ID: "issue.orbit", Name: "orbit", Route: toolutil.ActionRoute{InputSchema: only}},
		),
	}
	got := schemasByAction(built)
	want := map[string]map[string]any{"issue.get": first, "issue.orbit": only}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("schemasByAction = %v, want %v", got, want)
	}
}

// catalogOf is a catalog of one group, the tool named tool, holding actions
// of its domain.
func catalogOf(t *testing.T, tool string, actions ...actioncatalog.Action) *actioncatalog.Catalog {
	t.Helper()
	group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: tool})
	for _, action := range actions {
		group.SetAction(action)
	}
	catalog := actioncatalog.NewCatalog()
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	return catalog
}

// useFixtures points every seam at the fixture: one action whose field
// GitLab requires and the schema leaves out, declared, and one alias it uses.
func useFixtures(t *testing.T) {
	t.Helper()
	originals := struct {
		readRecord   func(string) (actionrequests.Record, error)
		catalogs     func() ([]*actioncatalog.Catalog, error)
		readLive     func(string) (apilive.Document, error)
		declarations func() map[declarationKey]declaration
		aliases      func() map[aliasKey]paramAlias
		bodies       func() map[string]workhorseBody
	}{readRecord, catalogs, readLive, declarations, aliases, bodies}
	t.Cleanup(func() {
		readRecord, catalogs, readLive = originals.readRecord, originals.catalogs, originals.readLive
		declarations, aliases, bodies = originals.declarations, originals.aliases, originals.bodies
	})
	bodies = func() map[string]workhorseBody { return nil }
	readRecord = func(string) (actionrequests.Record, error) {
		return actionrequests.Record{Actions: []actionrequests.RecordAction{
			{ID: "thing.update", Requests: []actionrequests.RecordRequest{rest("PUT /projects/:id/things/:name")}, Paths: [][]int{{0}}},
		}}, nil
	}
	catalogs = func() ([]*actioncatalog.Catalog, error) {
		return []*actioncatalog.Catalog{catalogOf(t, "gitlab_thing", actioncatalog.Action{
			ID: "thing.update", Name: "update",
			Route: toolutil.ActionRoute{InputSchema: schemaOf([]string{"thing"}, "project_id", "thing", "title", "confirm")},
		})}, nil
	}
	readLive = func(string) (apilive.Document, error) {
		return apilive.Document{Routes: []apilive.Route{*liveRoute("PUT", "/projects/:id/things/:name", map[string]apilive.Param{"id": {}, "name": {}, "title": {}})}}, nil
	}
	declarations = func() map[declarationKey]declaration {
		return map[declarationKey]declaration{{action: "thing.update", field: "project_id"}: {
			Direction: DirectionGitLabRequires, Category: categoryHandlerDefault, Reason: "the fixture's",
		}}
	}
	aliases = func() map[aliasKey]paramAlias {
		return map[aliasKey]paramAlias{{action: "thing.update", param: "name"}: {Field: "thing", Reason: "the fixture's"}}
	}
}

// decode reads a report back.
func decode(t *testing.T, content []byte) Report {
	t.Helper()
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		t.Fatalf("decode the report: %v", err)
	}
	return report
}

// TestRun_TheFixture_PassesAndNamesItsInputs verifies a run over the fixture:
// the gate passes with the one disagreement declared, the report names the
// two committed files it read and ends with a newline, and the context lists
// are kept.
func TestRun_TheFixture_PassesAndNamesItsInputs(t *testing.T) {
	useFixtures(t)
	content, clean, err := Run("root", Options{})
	if err != nil || !clean {
		t.Fatalf("Run = clean %t, error %v; want a clean run", clean, err)
	}
	if !strings.HasSuffix(string(content), "}\n") {
		t.Error("the report does not end with a newline")
	}
	report := decode(t, content)
	if report.Requests != actionrequests.RecordPath || report.Record != "docs/development/gitlab-api-live.json" {
		t.Errorf("inputs = %q, %q; want the two committed paths", report.Requests, report.Record)
	}
	if report.Summary.Declared != 1 || len(report.Unplaced) != 1 || report.Findings == nil || report.Stale == nil {
		t.Errorf("report = %+v, want one declared, confirm unplaced and empty gate lists", report)
	}
}

// TestRun_GapsOnly_DropsTheContextLists verifies -gaps-only leaves the
// unplaced and unjudged lists out and keeps the gate's.
func TestRun_GapsOnly_DropsTheContextLists(t *testing.T) {
	useFixtures(t)
	content, _, err := Run("root", Options{GapsOnly: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if report.Unplaced != nil || report.Unjudged != nil || report.Summary.FieldsUnplaced != 1 || len(report.Declared) != 1 {
		t.Errorf("report = %+v, want the context dropped and the counts and declarations kept", report)
	}
}

// TestRun_ADisagreement_FailsTheGate verifies an undeclared disagreement
// fails the gate with the report still written.
func TestRun_ADisagreement_FailsTheGate(t *testing.T) {
	useFixtures(t)
	declarations = func() map[declarationKey]declaration { return nil }
	content, clean, err := Run("root", Options{GapsOnly: true})
	if err != nil || clean {
		t.Fatalf("Run = clean %t, error %v; want the gate to fail with a report", clean, err)
	}
	if report := decode(t, content); report.Summary.Findings != 1 || report.Findings[0].Field != "project_id" {
		t.Errorf("findings = %+v, want project_id", report.Findings)
	}
}

// TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt verifies each input's
// failure surfaces with what failed, and that a report that cannot be encoded
// is an error rather than an empty file.
func TestRun_AnInputThatCannotBeRead_StopsTheRunNamingIt(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		wantErr string
	}{
		{name: "record", arrange: func(*testing.T) {
			readRecord = func(string) (actionrequests.Record, error) { return actionrequests.Record{}, boom }
		}, wantErr: "boom"},
		{name: "live", arrange: func(*testing.T) {
			readLive = func(string) (apilive.Document, error) { return apilive.Document{}, boom }
		}, wantErr: "read the live record: boom"},
		{name: "catalog", arrange: func(*testing.T) {
			catalogs = func() ([]*actioncatalog.Catalog, error) { return nil, boom }
		}, wantErr: "build the action catalog: boom"},
		{name: "marshal", arrange: func(t *testing.T) {
			t.Helper()
			original := marshalIndent
			t.Cleanup(func() { marshalIndent = original })
			marshalIndent = func(any, string, string) ([]byte, error) { return nil, boom }
		}, wantErr: "marshal report: boom"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			useFixtures(t)
			testCase.arrange(t)
			content, clean, err := Run("root", Options{})
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) || clean || content != nil {
				t.Errorf("Run = %q, clean %t, error %v; want no report and an error containing %q", content, clean, err, testCase.wantErr)
			}
		})
	}
}

// TestReadLive_ReadsTheRecordUnderTheRoot verifies the default reader looks
// for the record where the repository keeps it, so a root without one is an
// error naming the file.
func TestReadLive_ReadsTheRecordUnderTheRoot(t *testing.T) {
	_, err := readLive(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), apilive.FileName) {
		t.Errorf("readLive of an empty root = %v, want an error naming %s", err, apilive.FileName)
	}
}

// TestRun_TheTree_RequiresWhatGitLabRequires runs the gate on the
// repository's own catalog, request record and live record, which is what
// make audit-1to1-required does: every parameter the three surfaces require
// is one GitLab requires on every way the action runs, and the reverse, or a
// declaration says why not. It reads the committed request record, so a
// stacked layer that defers that record's refresh skips it.
func TestRun_TheTree_RequiresWhatGitLabRequires(t *testing.T) {
	freshness.SkipIfDeferred(t)
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("find the repository root: %v", err)
	}
	content, clean, err := Run(root, Options{GapsOnly: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	report := decode(t, content)
	if !clean {
		t.Fatalf("requiredness disagreements: %+v\nstale: %v", report.Findings, report.Stale)
	}
	if report.Summary.Judged == 0 || report.Summary.FieldsJudged == 0 || report.Summary.Declared == 0 {
		t.Errorf("summary = %+v, want the tree's actions judged and its declarations used", report.Summary)
	}
}
