package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/derive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
)

// declarationDrivers are the arguments that make each declared action send
// what its declaration stands for: the work item filter that takes an epic
// list off the REST endpoint, and the empty assignee list that makes a work
// item update read the item before writing it.
var declarationDrivers = map[string]map[string]any{
	"group.epic_create":          {"full_path": "group", "title": "t"},
	"group.epic_get":             {"full_path": "group", "epic_iid": 1},
	"group.epic_list":            {"full_path": "group", "author_username": "someone"},
	"group.epic_update":          {"full_path": "group", "epic_iid": 1, "title": "t"},
	"issue.work_item_create":     {"full_path": "group/project", "work_item_type_id": "gid://gitlab/WorkItems::Type/1", "title": "t"},
	"issue.work_item_get":        {"full_path": "group/project", "work_item_iid": 1},
	"issue.work_item_list":       {"full_path": "group/project"},
	"issue.work_item_update":     {"full_path": "group/project", "work_item_iid": 1, "title": "t", "assignee_ids": []int64{}},
	"admin.terraform_state_get":  {"project_path": "group/project", "name": "state"},
	"admin.terraform_state_list": {"project_path": "group/project"},
}

// graphQLAnswers are what the stand-in answers each document with, keyed by a
// word only that document carries, longest first, so every handler gets past
// the requests in front of the one its declaration stands for.
var graphQLAnswers = []struct{ key, body string }{
	{"GetWorkItemID", `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}}`},
	{"GetWorkItem", `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","title":"t"}}}}`},
	{"ListWorkItems", `{"data":{"namespace":{"workItems":{"nodes":[],"pageInfo":{"hasNextPage":false,"hasPreviousPage":false}}}}}`},
	{"workItemCreate", `{"data":{"workItemCreate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","title":"t"},"errors":[]}}}`},
	{"workItemUpdate", `{"data":{"workItemUpdate":{"workItem":{"id":"gid://gitlab/WorkItem/1","iid":"1","title":"t"},"errors":[]}}}`},
	{"terraformState", `{"data":{"project":{"terraformStates":{"nodes":[]},"terraformState":{"name":"state"}}}}`},
}

// documentRecorder is a stand-in GitLab that keeps every GraphQL document a
// handler posts and answers it from graphQLAnswers. It runs on the httptest
// server's goroutine, so it records and never stops the test.
type documentRecorder struct {
	t         *testing.T
	mu        sync.Mutex
	documents []string
}

// ServeHTTP records the document and answers it. A REST request is answered
// not found, since no declaration here stands for one.
func (r *documentRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/api/graphql" {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
		return
	}
	body, err := io.ReadAll(req.Body)
	var payload struct {
		Query string `json:"query"`
	}
	if err == nil {
		err = json.Unmarshal(body, &payload)
	}
	if err != nil {
		r.t.Errorf("the stand-in could not read a request to %s: %v", req.URL.Path, err)
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"unreadable"}`)
		return
	}
	r.mu.Lock()
	r.documents = append(r.documents, payload.Query)
	r.mu.Unlock()
	for _, answer := range graphQLAnswers {
		if strings.Contains(payload.Query, answer.key) {
			testutil.RespondJSON(w, http.StatusOK, answer.body)
			return
		}
	}
	testutil.RespondJSON(w, http.StatusOK, `{"data":{}}`)
}

// sent returns the documents recorded so far.
func (r *documentRecorder) sent() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.documents)
}

// documentShape is what the join reads of a document: the operation's name,
// its root fields and every object position it selects, fragments spread in.
type documentShape struct {
	name      string
	roots     []string
	positions []string
}

// shapeOf parses a document without a schema and reads its shape.
func shapeOf(t *testing.T, document string) documentShape {
	t.Helper()
	doc, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil {
		t.Fatalf("parse %q: %v", document, err)
	}
	if len(doc.Operations) != 1 {
		t.Fatalf("the document holds %d operations, want one", len(doc.Operations))
	}
	op := doc.Operations[0]
	shape := documentShape{name: op.Name}
	var walk func(prefix string, set ast.SelectionSet)
	walk = func(prefix string, set ast.SelectionSet) {
		for _, selection := range set {
			switch typed := selection.(type) {
			case *ast.Field:
				if len(typed.SelectionSet) == 0 {
					continue
				}
				name := typed.Alias
				if name == "" {
					name = typed.Name
				}
				path := name
				if prefix == "" {
					shape.roots = append(shape.roots, name)
				} else {
					path = prefix + "." + name
				}
				shape.positions = append(shape.positions, path)
				walk(path, typed.SelectionSet)
			case *ast.InlineFragment:
				walk(prefix, typed.SelectionSet)
			case *ast.FragmentSpread:
				walk(prefix, doc.Fragments.ForName(typed.Name).SelectionSet)
			}
		}
	}
	walk("", op.SelectionSet)
	slices.Sort(shape.positions)
	shape.positions = slices.Compact(shape.positions)
	return shape
}

// sameOperation reports whether a sent document is the one a declared
// document stands for: the same operation name where the declared one has
// one, and the same root fields where it does not, as client-go's Terraform
// state queries have none.
func sameOperation(declared, sent documentShape) bool {
	if declared.name != "" {
		return sent.name == declared.name
	}
	return slices.Equal(sent.roots, declared.roots)
}

// declaredGraphQL is one GraphQL request a declaration stands for.
type declaredGraphQL struct {
	action, category string
	request          derive.Request
}

// declaredDocuments lists every GraphQL request the declarations stand for.
func declaredDocuments() []declaredGraphQL {
	var out []declaredGraphQL
	for _, declaration := range requestDeclarations {
		for _, request := range declaration.Requests {
			if request.Kind == derive.KindGraphQL {
				out = append(out, declaredGraphQL{action: declaration.Action, category: declaration.Category, request: request})
			}
		}
	}
	return out
}

// TestRequestDeclarations_DeclareWhatTheHandlerSends runs the handler of
// every action a declaration stands in for, through the catalog with a
// client-go of the version the program compiles against, and holds each
// declared GraphQL document to the one the handler made client-go send: the
// same operation and the same object positions. The documents are written by
// hand because client-go assembles them at run time, from text/template
// (the work item methods, a list's fragment rendered from the fields the
// handler asks for) or fmt.Sprintf (the Terraform states), which no walk of
// the source evaluates; this is what tells a client-go bump that changed a
// template, or a handler that asks for other fields, from a declaration
// still telling the truth. The client is an Enterprise one, since a work item
// list asks for the Enterprise features only there and the declaration
// stands for the widest list it sends.
func TestRequestDeclarations_DeclareWhatTheHandlerSends(t *testing.T) {
	declared := declaredDocuments()
	recorders := map[string]*documentRecorder{}
	for _, one := range declared {
		if _, ran := recorders[one.action]; !ran {
			recorders[one.action] = driveDeclaredAction(t, one.action)
		}
	}
	for _, declared := range declared {
		t.Run(declared.action+" "+declared.request.Name, func(t *testing.T) {
			recorder := recorders[declared.action]
			want := shapeOf(t, declared.request.Document)
			for _, document := range recorder.sent() {
				got := shapeOf(t, document)
				if !sameOperation(want, got) {
					continue
				}
				if !slices.Equal(got.positions, want.positions) {
					t.Errorf("%s declares positions\n%s\nand the handler sends\n%s", declared.action,
						strings.Join(want.positions, "\n"), strings.Join(got.positions, "\n"))
				}
				return
			}
			t.Errorf("%s sent no %s %s among %d documents", declared.action, declared.category, declared.request.Name, len(recorder.sent()))
		})
	}
	if len(declarationDrivers) != len(recorders) {
		t.Errorf("declarationDrivers holds %d actions and %d were declared; drop the drivers no declaration needs", len(declarationDrivers), len(recorders))
	}
}

// driveDeclaredAction runs one action's handler, as the catalog binds it to an
// Enterprise client, against a stand-in that records what it posts.
func driveDeclaredAction(t *testing.T, id string) *documentRecorder {
	t.Helper()
	params, driven := declarationDrivers[id]
	if !driven {
		t.Fatalf("no arguments drive %s; add them to declarationDrivers", id)
	}
	recorder := &documentRecorder{t: t}
	client := testutil.NewTestClient(t, recorder)
	client.SetEnterprise(true)
	catalog, err := gitlabtools.BuildActionCatalog(client, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("build the catalog: %v", err)
	}
	action, found := catalog.Action(actioncatalog.ActionID(id))
	if !found {
		t.Fatalf("the catalog holds no %s", id)
	}
	// The handler's own answer is beside the point: what it sent before
	// returning is what the declaration is held to.
	_, _ = action.Route.Handler(context.Background(), params)
	return recorder
}

// TestSameOperation_MatchesByNameOrRootFields verifies a named declared
// document is matched by its name alone and an unnamed one by its root
// fields.
func TestSameOperation_MatchesByNameOrRootFields(t *testing.T) {
	cases := []struct {
		name           string
		declared, sent documentShape
		want           bool
	}{
		{name: "same name", declared: documentShape{name: "A", roots: []string{"x"}}, sent: documentShape{name: "A", roots: []string{"y"}}, want: true},
		{name: "other name", declared: documentShape{name: "A", roots: []string{"x"}}, sent: documentShape{name: "B", roots: []string{"x"}}},
		{name: "unnamed, same roots", declared: documentShape{roots: []string{"x"}}, sent: documentShape{name: "B", roots: []string{"x"}}, want: true},
		{name: "unnamed, other roots", declared: documentShape{roots: []string{"x"}}, sent: documentShape{roots: []string{"y"}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := sameOperation(testCase.declared, testCase.sent); got != testCase.want {
				t.Errorf("sameOperation() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestShapeOf_ReadsPositionsThroughFragments verifies the shape of a document
// names every object position once, aliased ones by their alias, with inline
// fragments and named fragments spread in.
func TestShapeOf_ReadsPositionsThroughFragments(t *testing.T) {
	got := shapeOf(t, `query Q { a { b { id } ... on T { c { id } } ...F } other: d { e { id } } }
fragment F on T { b { id } f { id } }`)
	want := documentShape{name: "Q", roots: []string{"a", "other"}, positions: []string{"a", "a.b", "a.c", "a.f", "other", "other.e"}}
	if got.name != want.name || !slices.Equal(got.roots, want.roots) || !slices.Equal(got.positions, want.positions) {
		t.Errorf("shapeOf = %+v, want %+v", got, want)
	}
}
