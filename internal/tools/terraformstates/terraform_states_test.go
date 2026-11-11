// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// terraform_states_test.go contains unit tests for the Terraform state MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package terraformstates

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedErr identifies the err expected err constant used by this package.
const errExpectedErr = "expected error"

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// The guidance each handler owes a caller it had to refuse, spelled out here
// rather than read back from the handler. Four sentences for four handlers is
// four values that could trade places, and a hint taken from the source it is
// checking against cannot notice that; written out, a crossing fails.
const (
	wantListNotFoundHint = "verify project_path with project.get; uses GraphQL. " +
		"Terraform states require Maintainer role to view"
	wantGetNotFoundHint     = "verify state name with admin.terraform_state_list; the state may not exist for this project"
	wantDeleteForbiddenHint = "deleting Terraform states requires Maintainer role; deletion is irreversible. " +
		"All versions are removed"
	wantDeleteVersionNotFoundHint = "the project has no state by that name, or the state no version with that serial; " +
		"verify the state name with admin.terraform_state_list, which shows each state's latest serial"
)

// TestList verifies List.
func TestList(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.States) != 1 || out.States[0].Name != "state1" {
		t.Errorf("unexpected states: %+v", out.States)
	}
}

// TestList_Error verifies List when error: the 404 carries the hint that names
// what a caller can actually check, the project path and the Maintainer role,
// rather than whichever hint a sibling handler happens to hold.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectPath: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantListNotFoundHint) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), wantListNotFoundHint)
	}
}

// TestList_ReadsEveryFieldOfEveryState verifies that every field of every
// state arrives where it belongs. The whole slice is compared against states
// no two values of which agree, because the converter assigns seven fields in
// a row and four of them are times: with a fixture that omits one, or a test
// that spot-checks the name alone, two of them could trade places and nothing
// here would fail. The second state is unlocked and live, which GitLab sends
// as a null lockedAt and deletedAt, and those two are left out of its item
// rather than written as the zero time client-go decodes a null into.
func TestList_ReadsEveryFieldOfEveryState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformStates":{"nodes":[
			{"name":"production","latestVersion":{"serial":11,"downloadPath":"/dl/one",
			  "createdAt":"2026-05-06T07:08:09Z","updatedAt":"2026-06-07T08:09:10Z"},
			 "createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z",
			 "lockedAt":"2026-03-04T05:06:07Z","deletedAt":"2026-04-05T06:07:08Z"},
			{"name":"staging","latestVersion":{"serial":22,"downloadPath":"/dl/two",
			  "createdAt":"2025-07-08T09:10:11Z","updatedAt":"2025-08-09T10:11:12Z"},
			 "createdAt":"2025-05-06T07:08:09Z","updatedAt":"2025-06-07T08:09:10Z",
			 "lockedAt":null,"deletedAt":null},
			{"name":"unwritten","latestVersion":null,
			 "createdAt":"2025-09-10T11:12:13Z","updatedAt":"2025-09-10T11:12:13Z",
			 "lockedAt":null,"deletedAt":null}
		]}}}}`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := []StateItem{
		{
			Name: "production", LatestSerial: 11, DownloadPath: "/dl/one",
			LatestVersionCreatedAt: "2026-05-06T07:08:09Z", LatestVersionUpdatedAt: "2026-06-07T08:09:10Z",
			CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-02-03T04:05:06Z",
			LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z",
		},
		{
			Name: "staging", LatestSerial: 22, DownloadPath: "/dl/two",
			LatestVersionCreatedAt: "2025-07-08T09:10:11Z", LatestVersionUpdatedAt: "2025-08-09T10:11:12Z",
			CreatedAt: "2025-05-06T07:08:09Z", UpdatedAt: "2025-06-07T08:09:10Z",
		},
		{
			Name:      "unwritten",
			CreatedAt: "2025-09-10T11:12:13Z", UpdatedAt: "2025-09-10T11:12:13Z",
		},
	}
	if !reflect.DeepEqual(out.States, want) {
		t.Errorf("List() states = %+v, want %+v", out.States, want)
	}
}

// stateItemSchemaFields maps each field StateItem publishes to the field of
// the pinned schema it carries, as object and field name.
var stateItemSchemaFields = map[string][2]string{
	"name":                      {"TerraformState", "name"},
	"latest_serial":             {"TerraformStateVersion", "serial"},
	"download_path":             {"TerraformStateVersion", "downloadPath"},
	"latest_version_created_at": {"TerraformStateVersion", "createdAt"},
	"latest_version_updated_at": {"TerraformStateVersion", "updatedAt"},
	"created_at":                {"TerraformState", "createdAt"},
	"updated_at":                {"TerraformState", "updatedAt"},
	"locked_at":                 {"TerraformState", "lockedAt"},
	"deleted_at":                {"TerraformState", "deletedAt"},
}

// stateSelection is what one document selects of a Terraform
// state: the fields it asks for on the state itself and on its latest
// version, by the schema object each belongs to.
type stateSelection map[string][]string

// selectedFields reads a GraphQL document and returns the fields it selects
// directly on the state found by following path from the operation's root,
// and directly on that state's latestVersion. Only direct selections count,
// so a createdAt asked of the latest version is not taken for the state's,
// and a name that is an argument of the field is not taken for a selection.
func selectedFields(t *testing.T, document string, path ...string) stateSelection {
	t.Helper()
	parsed, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil {
		t.Fatalf("parse the document: %v\n%s", err, document)
	}
	if len(parsed.Operations) != 1 {
		t.Fatalf("the document holds %d operations, want one:\n%s", len(parsed.Operations), document)
	}
	selections := parsed.Operations[0].SelectionSet
	for _, name := range path {
		field := childField(selections, name)
		if field == nil {
			t.Fatalf("the document selects no %s along %v:\n%s", name, path, document)
		}
		selections = field.SelectionSet
	}
	selected := stateSelection{"TerraformState": fieldNames(selections)}
	if version := childField(selections, "latestVersion"); version != nil {
		selected["TerraformStateVersion"] = fieldNames(version.SelectionSet)
	}
	return selected
}

// childField returns the field selected by name directly in a selection set,
// or nil.
func childField(selections ast.SelectionSet, name string) *ast.Field {
	for _, selection := range selections {
		if field, ok := selection.(*ast.Field); ok && field.Name == name {
			return field
		}
	}
	return nil
}

// fieldNames lists the fields selected directly in a selection set.
func fieldNames(selections ast.SelectionSet) []string {
	var names []string
	for _, selection := range selections {
		if field, ok := selection.(*ast.Field); ok {
			names = append(names, field.Name)
		}
	}
	return names
}

// TestStateItem_FieldsHeldToThePinnedSchema holds every field StateItem
// publishes to the field of the pinned schema it carries, in three ways: the
// schema has the field, a field GitLab always sends is always written and one
// it can send null is left out when it did (a field of the latest version
// counts as nullable when the schema lets latestVersion itself be null, which
// it does for a state nothing has written yet), and both documents, the
// list's own and client-go's get, select it on the object it belongs to. The
// get's document is written inside client-go and the list's beside the
// handler, and a field one of them stops selecting would arrive empty on that
// path with every other test green, since a mock answers with whatever its
// fixture holds. The map above must name every published field, so a field
// added to StateItem without a row fails here too.
func TestStateItem_FieldsHeldToThePinnedSchema(t *testing.T) {
	schema, err := graphqlschema.Schema()
	if err != nil {
		t.Fatalf("graphqlschema.Schema() error: %v", err)
	}
	var getDocument, listDocument string
	getClient := testutil.NewTestClient(t, recordGraphQL(t, &getDocument,
		`{"data":{"project":{"terraformState":{"name":"production","createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z"}}}}`))
	if _, err = Get(t.Context(), getClient, GetInput{ProjectPath: "group/project", Name: "production"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	listClient := testutil.NewTestClient(t, recordGraphQL(t, &listDocument,
		`{"data":{"project":{"terraformStates":{"nodes":[]}}}}`))
	if _, err = List(t.Context(), listClient, ListInput{ProjectPath: "group/project"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	documents := map[string]stateSelection{
		"get":  selectedFields(t, getDocument, "project", "terraformState"),
		"list": selectedFields(t, listDocument, "project", "terraformStates", "nodes"),
	}

	for field := range reflect.TypeFor[StateItem]().Fields() {
		if field.Anonymous {
			continue
		}
		jsonName, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		t.Run(jsonName, func(t *testing.T) {
			target, ok := stateItemSchemaFields[jsonName]
			if !ok {
				t.Fatalf("StateItem publishes %q, which no row of stateItemSchemaFields ties to the schema", jsonName)
			}
			object, name := target[0], target[1]
			definition := schema.Types[object].Fields.ForName(name)
			if definition == nil {
				t.Fatalf("the pinned schema has no %s.%s", object, name)
			}
			always := alwaysSent(schema, object, definition)
			if omitted := strings.Contains(options, "omitempty"); omitted == always {
				t.Errorf("%s omitempty = %v, want %v for %s.%s typed %s", jsonName, omitted, !always, object, name, definition.Type)
			}
			for document, selected := range documents {
				t.Run(document, func(t *testing.T) {
					if !slices.Contains(selected[object], name) {
						t.Errorf("the %s document selects no %s on %s (it selects %v), so %s would arrive empty", document, name, object, selected[object], jsonName)
					}
				})
			}
		})
	}
}

// alwaysSent reports whether GitLab sends a field on every state: the schema
// types it non-null and, for a field of the latest version, types
// latestVersion non-null too, which it does not, since a state nothing has
// written yet has no latest version.
func alwaysSent(schema *ast.Schema, object string, definition *ast.FieldDefinition) bool {
	if object == "TerraformStateVersion" {
		return definition.Type.NonNull && schema.Types["TerraformState"].Fields.ForName("latestVersion").Type.NonNull
	}
	return definition.Type.NonNull
}

// TestSelectedFields_ReadsDirectSelectionsOnly verifies the reading the test
// above relies on, on a document written to trip a substring match: the
// state's createdAt is not selected and its latest version's is, and the
// state's name appears only as an argument. Neither may count as selected on
// the state.
func TestSelectedFields_ReadsDirectSelectionsOnly(t *testing.T) {
	document := `query { project(fullPath: "g/p") { terraformState(name: "name") { lockedAt latestVersion { createdAt serial } } } }`

	selected := selectedFields(t, document, "project", "terraformState")

	want := stateSelection{"TerraformState": {"lockedAt", "latestVersion"}, "TerraformStateVersion": {"createdAt", "serial"}}
	if !reflect.DeepEqual(selected, want) {
		t.Errorf("selectedFields() = %v, want %v", selected, want)
	}
	if bare := selectedFields(t, `query { project { terraformState { name } } }`, "project", "terraformState"); !reflect.DeepEqual(bare, stateSelection{"TerraformState": {"name"}}) {
		t.Errorf("selectedFields() without a latest version = %v, want the state's name alone", bare)
	}
}

// TestList_EmptyProject_YieldsAnEmptySliceRatherThanNil verifies that a
// project with no states answers with a slice a caller can range over and a
// formatter can count, which is what the handler's pre-sized make() is for.
func TestList_EmptyProject_YieldsAnEmptySliceRatherThanNil(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformStates":{"nodes":[]}}}}`)
	}))
	out, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.States == nil {
		t.Error("List() states = nil, want an empty slice")
	}
	if len(out.States) != 0 {
		t.Errorf("List() states = %+v, want none", out.States)
	}
}

// TestGet_ReadsEveryFieldOfTheState verifies the single-state converter the
// way the list one is verified, and for the same reason: the fixture gives
// each of the seven fields a value none of which is another, and the whole
// item is compared, so no two assignments can be exchanged without this
// failing.
func TestGet_ReadsEveryFieldOfTheState(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"data":{"project":{"terraformState":{"name":"production","latestVersion":{"serial":11,"downloadPath":"/dl/one",`+
				`"createdAt":"2026-05-06T07:08:09Z","updatedAt":"2026-06-07T08:09:10Z"},`+
				`"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z","lockedAt":"2026-03-04T05:06:07Z","deletedAt":"2026-04-05T06:07:08Z"}}}}`)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := StateItem{
		Name: "production", LatestSerial: 11, DownloadPath: "/dl/one",
		LatestVersionCreatedAt: "2026-05-06T07:08:09Z", LatestVersionUpdatedAt: "2026-06-07T08:09:10Z",
		CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-02-03T04:05:06Z",
		LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z",
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Get() = %+v, want %+v", out, want)
	}
}

// TestGet_QueryNamesTheProjectAndTheState verifies that both identifiers the
// caller chose reach GitLab, and reach it in their own position. client-go
// writes them into the query text rather than into variables, so a handler
// passing the project path for the state name as well sends a document that is
// still valid, still answered by any mock, and asks for the wrong state; the
// two are given values that share nothing so that crossing them fails here.
// TestList_QueryNamesTheProject holds the same property for the one identifier
// the list call carries.
func TestGet_QueryNamesTheProjectAndTheState(t *testing.T) {
	var seen string
	client := testutil.NewTestClient(t, recordGraphQL(t, &seen,
		`{"data":{"project":{"terraformState":{"name":"production","latestVersion":{"serial":11}}}}}`))
	if _, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "production"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(seen, `fullPath: "group/project"`) {
		t.Errorf("query = %q, want it to select the caller's project", seen)
	}
	if !strings.Contains(seen, `name: "production"`) {
		t.Errorf("query = %q, want it to select the caller's state", seen)
	}
}

// TestList_QueryNamesTheProject verifies the list call asks GitLab about the
// project the caller named rather than one written into the handler, and asks
// for GitLab's default page when the caller named no page: the list's own
// document carries the project as a variable, so the variable is what is
// read, and a document written with the project spelled into it would send
// none.
func TestList_QueryNamesTheProject(t *testing.T) {
	var seen graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQLRequest(t, &seen,
		`{"data":{"project":{"terraformStates":{"nodes":[]}}}}`))
	if _, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := map[string]any{"fullPath": "group/project", "first": float64(toolutil.GraphQLDefaultFirst)}
	if !reflect.DeepEqual(seen.Variables, want) {
		t.Errorf("variables = %v, want %v", seen.Variables, want)
	}
}

// pagedStatesBody is one page of a project's states: GitLab counts two and
// hands back the first, with the cursor of the second.
const pagedStatesBody = `{"data":{"project":{"terraformStates":{"count":2,"nodes":[` +
	`{"name":"production","createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z","lockedAt":null,"deletedAt":null,"latestVersion":null}` +
	`],"pageInfo":{"hasNextPage":true,"endCursor":"c2"}}}}}`

// TestList_PagesThroughTheStates verifies the list asks GitLab for the page
// the caller named, first and after, and hands back the cursor of the next
// page and the count GitLab gives. client-go's document took neither and
// selected neither, so a project with more states than GitLab's 100 per page
// read as having 100 and no more, with no way to ask for the rest. The input
// is decoded the way a tool call is, so the test reads only what the caller
// can send.
func TestList_PagesThroughTheStates(t *testing.T) {
	var seen graphQLRequest
	client := testutil.NewTestClient(t, recordGraphQLRequest(t, &seen, pagedStatesBody))
	var input ListInput
	if err := json.Unmarshal([]byte(`{"project_path":"group/project","first":1,"after":"c1"}`), &input); err != nil {
		t.Fatalf("decoding the input: %v", err)
	}

	out, err := List(t.Context(), client, input)
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	if seen.Variables["first"] != float64(1) || seen.Variables["after"] != "c1" || seen.Variables["fullPath"] != "group/project" {
		t.Errorf("variables = %v, want first 1, after c1 and the caller's project", seen.Variables)
	}
	published, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var keys map[string]any
	if err = json.Unmarshal(published, &keys); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if keys["count"] != float64(2) {
		t.Errorf("count = %v, want the 2 GitLab counted", keys["count"])
	}
	wantPage := map[string]any{"has_next_page": true, "end_cursor": "c2"}
	if !reflect.DeepEqual(keys["pagination"], wantPage) {
		t.Errorf("pagination = %v, want %v", keys["pagination"], wantPage)
	}
	if md := FormatListMarkdown(out); !strings.Contains(md, "## Terraform States (2)") || !strings.Contains(md, "next page cursor: `c2`") {
		t.Errorf("the list does not say how many states there are and where the next page starts:\n%s", md)
	}
}

// TestList_RefusedDocument_ReportsGitLabsErrors verifies a document GitLab
// refused is reported with GitLab's own words. GitLab answers it with HTTP 200,
// a null project and a top-level errors array, which client-go does not turn
// into an error, so read as a missing project it would send the caller to
// check a path that was right.
func TestList_RefusedDocument_ReportsGitLabsErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK,
			`{"data":{"project":null},"errors":[{"message":"Field 'count' doesn't exist on type 'TerraformStateConnection'"}]}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "Field 'count' doesn't exist") || !strings.Contains(err.Error(), "gitlab_list_terraform_states") {
		t.Errorf("error = %q, want GitLab's refusal under the list's operation", err)
	}
	if strings.Contains(err.Error(), wantListNotFoundHint) {
		t.Errorf("error = %q, want no hint about the project path for a document GitLab refused", err)
	}
}

// TestList_NoProject_IsNotFoundWithTheHint verifies a project GitLab does not
// find, or does not show the caller, which it answers with a null project and
// no error, is reported as not found and carries the hint that says what to
// check, as client-go's list reported it.
func TestList_NoProject_IsNotFoundWithTheHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":null}}`)
	}))
	_, err := List(t.Context(), client, ListInput{ProjectPath: "group/missing"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantListNotFoundHint) {
		t.Errorf("error = %q, want it to carry %q", err, wantListNotFoundHint)
	}
	if !toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		t.Errorf("error = %v, want it read as a 404", err)
	}
}

// TestListPage_DocumentWithoutPageVariables_IsRefusedBeforeSending verifies
// the one way the page variables can be refused: a document that does not
// declare them, which GitLab would answer with somebody else's page. Nothing
// is sent.
func TestListPage_DocumentWithoutPageVariables_IsRefusedBeforeSending(t *testing.T) {
	var sent atomic.Bool
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sent.Store(true)
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":null}}`)
	}))
	_, err := listPage(t.Context(), client, `query($fullPath: ID!) { project(fullPath: $fullPath) { id } }`,
		ListInput{ProjectPath: "group/project"})
	if err == nil || !strings.Contains(err.Error(), "gitlab_list_terraform_states") {
		t.Errorf("listPage() error = %v, want a refusal under the list's operation", err)
	}
	if sent.Load() {
		t.Error("listPage() sent a document that declares no page variables")
	}
}

// listStatesComplexity is what listStatesQuery costs at a page of a hundred.
// It is the estimate's figure: GitLab.com answered the anonymous measurement
// with an internal error, so no instance has confirmed it yet, and the figure
// is to be replaced by a measured one (a queryComplexity { score } selection
// answers below 200, its own cost being 2) the first time a GitLab is at hand.
const listStatesComplexity = 17

// TestListStatesQuery_Complexity_IsTheRecordedFigureUnderGitLabsLimit
// verifies the list's own document still costs the figure recorded above, and
// that the figure is under the limit GitLab refuses a query above, at the
// largest page a caller can ask for. Holding the figure rather than only the
// limit is what sends whoever changes the selection to measure again: the
// connection is new to this document, and nothing else here can see its cost,
// since the mock judges a document by the schema alone.
func TestListStatesQuery_Complexity_IsTheRecordedFigureUnderGitLabsLimit(t *testing.T) {
	got, err := testutil.GitLabQueryComplexity(listStatesQuery, map[string]any{
		"fullPath": "group/project", "first": toolutil.GraphQLMaxFirst,
	})
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got != listStatesComplexity {
		t.Errorf("listStatesQuery costs %d at first=%d by the estimate, and %d is recorded: measure it and record the figure",
			got, toolutil.GraphQLMaxFirst, listStatesComplexity)
	}
	if got > testutil.GitLabAuthenticatedMaxComplexity {
		t.Errorf("listStatesQuery costs %d at first=%d, over the %d GitLab refuses a query above",
			got, toolutil.GraphQLMaxFirst, testutil.GitLabAuthenticatedMaxComplexity)
	}
}

// graphQLRequest is what a recorded GraphQL POST carried.
type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// recordGraphQLRequest answers every GraphQL POST with body and writes the
// request it was sent into seen, for the test goroutine to assert on after
// the call.
func recordGraphQLRequest(t *testing.T, seen *graphQLRequest, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(seen); err != nil {
			t.Errorf("decoding the GraphQL envelope: %v", err)
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"errors":[{"message":"undecodable envelope"}]}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	})
}

// recordGraphQL answers every GraphQL POST with body and writes the document
// it was sent into seen, so the test goroutine can assert on it after the call
// rather than from inside the handler, where it could not fail the test.
func recordGraphQL(t *testing.T, seen *string, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the GraphQL body: %v", err)
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"errors":[{"message":"unreadable body"}]}`)
			return
		}
		var envelope struct {
			Query string `json:"query"`
		}
		if err = json.Unmarshal(raw, &envelope); err != nil {
			t.Errorf("decoding the GraphQL envelope: %v", err)
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"errors":[{"message":"undecodable envelope"}]}`)
			return
		}
		*seen = envelope.Query
		testutil.RespondJSON(w, http.StatusOK, body)
	})
}

// TestGet verifies Get.
func TestGet(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/graphql" {
			testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3}}}}}`)
			return
		}
		http.NotFound(w, r)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.LatestSerial != 3 {
		t.Errorf("expected serial 3, got %d", out.LatestSerial)
	}
}

// TestGet_Error verifies Get when error: the 404 carries the hint about the
// state name, which is the one thing a caller of this handler chose beyond the
// project, and not the list handler's hint about the project itself.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))
	_, err := Get(t.Context(), client, GetInput{ProjectPath: "x", Name: "y"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantGetNotFoundHint) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), wantGetNotFoundHint)
	}
}

// TestDelete verifies Delete.
func TestDelete(t *testing.T) {
	var deleted atomic.Bool
	client := testutil.NewTestClient(t, deleteHandler(t, "null", &deleted))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !deleted.Load() {
		t.Error("Delete() did not send the delete of an unlocked state")
	}
}

// deleteHandler answers what Delete asks of an unlocked or a locked state:
// project 1 is group/project, the state read answers with lockedAt (a JSON
// value, null for an unlocked state), and the delete records that it arrived.
func deleteHandler(t *testing.T, lockedAt string, deleted *atomic.Bool) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"group/project"}`)
	})
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(raw), `fullPath: \"group/project\"`) || !strings.Contains(string(raw), `name: \"state1\"`) {
			t.Errorf("the state read asked %s (err %v), want state1 of group/project", raw, err)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"data":{"project":{"terraformState":{"name":"state1","createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-01-02T03:04:05Z","lockedAt":`+lockedAt+`}}}}`)
	})
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1", func(w http.ResponseWriter, _ *http.Request) {
		deleted.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// TestDelete_LockedState_IsRefusedWithTheUnlockHint verifies a locked state
// is refused before the delete is sent. GitLab answers the REST delete of a
// locked state with 200 and deletes nothing, so sending it reported a delete
// that never happened (upstream-bugs row 118); the state is read first and a
// lock is reported with the action that releases it.
func TestDelete_LockedState_IsRefusedWithTheUnlockHint(t *testing.T) {
	var deleted atomic.Bool
	client := testutil.NewTestClient(t, deleteHandler(t, `"2026-03-04T05:06:07Z"`, &deleted))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal("Delete() of a locked state = nil, want the refusal")
	}
	for _, want := range []string{"gitlab_delete_terraform_state: ", "locked", "2026-03-04T05:06:07Z", "admin.terraform_state_unlock"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to carry %q", err, want)
			}
		})
	}
	if deleted.Load() {
		t.Error("Delete() sent the delete of a locked state, which GitLab answers as done without doing it")
	}
}

// TestDelete_ProjectPath_IsReadWithoutAProjectLookup verifies a project_id
// that is already a path, URL-encoded or not, is the state read's fullPath as
// it stands: only a numeric ID needs the project's path looked up, since
// GitLab's GraphQL project field takes no ID.
func TestDelete_ProjectPath_IsReadWithoutAProjectLookup(t *testing.T) {
	for _, projectID := range []string{"group/project", "group%2Fproject"} {
		t.Run(projectID, func(t *testing.T) {
			var deleted atomic.Bool
			mux := http.NewServeMux()
			mux.HandleFunc("GET /api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("Delete() looked the project up at %s, want the path used as it stands", r.URL.Path)
				http.NotFound(w, r)
			})
			mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
				raw, _ := io.ReadAll(r.Body)
				if !strings.Contains(string(raw), `fullPath: \"group/project\"`) {
					t.Errorf("the state read asked %s, want group/project", raw)
				}
				testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":{"name":"state1","lockedAt":null}}}}`)
			})
			mux.HandleFunc("DELETE /api/v4/projects/", func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, "/terraform/state/state1") {
					t.Errorf("Delete() sent DELETE %s, want the state's route", r.URL.Path)
				}
				deleted.Store(true)
				w.WriteHeader(http.StatusNoContent)
			})
			client := testutil.NewTestClient(t, mux)
			if err := Delete(t.Context(), client, DeleteInput{ProjectID: toolutil.StringOrInt(projectID), Name: "state1"}); err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !deleted.Load() {
				t.Error("Delete() did not send the delete")
			}
		})
	}
}

// TestDelete_MalformedProjectPath_SendsNothing verifies a project_id that is
// neither an ID nor a path that unescapes is refused before any request.
func TestDelete_MalformedProjectPath_SendsNothing(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Delete() sent %s %s for a project_id it cannot read", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "group%zzproject", Name: "state1"})
	if err == nil || !strings.Contains(err.Error(), "gitlab_delete_terraform_state: ") || !strings.Contains(err.Error(), "group%zzproject") {
		t.Errorf("Delete() error = %v, want the refusal naming the project_id", err)
	}
}

// TestDelete_StateNotFound_SendsNoDelete verifies a state the read does not
// find is reported with the hint that names how to find it, and no delete is
// sent for it.
func TestDelete_StateNotFound_SendsNoDelete(t *testing.T) {
	var deleted atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"group/project"}`)
	})
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":null}}}`)
	})
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/gone", func(w http.ResponseWriter, _ *http.Request) {
		deleted.Store(true)
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, mux)
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "gone"})
	if err == nil || !strings.Contains(err.Error(), "admin.terraform_state_list") {
		t.Errorf("Delete() of a missing state error = %v, want the hint naming admin.terraform_state_list", err)
	}
	if deleted.Load() {
		t.Error("Delete() sent a delete for a state it could not read")
	}
}

// TestDelete_Error verifies Delete when error: a status the hint was not
// written for is still reported, and carries no Maintainer-role advice, since
// the role is not what a 404 is about.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if strings.Contains(err.Error(), wantDeleteForbiddenHint) {
		t.Errorf("error = %q, want no Maintainer-role hint on a 404", err.Error())
	}
}

// TestDelete_Forbidden_HintsMaintainerRoleAndIrreversibility verifies the one
// status Delete's hint was written for. Until this existed the package drove
// Delete only through a 404, so the hint branch never ran and the sentence it
// returns was held by nothing: a hint that traded places with a sibling's
// would have shipped.
func TestDelete_Forbidden_HintsMaintainerRoleAndIrreversibility(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/projects/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"group/project"}`)
	})
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"project":{"terraformState":{"name":"x","lockedAt":null}}}}`)
	})
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/x", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	})
	client := testutil.NewTestClient(t, mux)
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "x"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantDeleteForbiddenHint) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), wantDeleteForbiddenHint)
	}
}

// TestDeleteVersion verifies DeleteVersion.
func TestDeleteVersion(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 5})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// The lock route of a state whose project is named by its full path, as it
// reaches GitLab: client-go percent-encodes the "/" of the path, and the
// client's transport sends the "." of the state name as a dot, so the mock is
// matched on the escaped path.
const (
	lockedProject   = "group/infra"
	lockedState     = "prod.eu"
	lockedStatePath = "/api/v4/projects/group%2Finfra/terraform/state/prod.eu/lock"
)

// lockAnswer is what lib/api/terraform/state.rb answers a lock with: the four
// keys it was sent back as they were, and the ID, holder and time of the lock
// it recorded, the time rendered as Rails renders a timestamp.
const lockAnswer = `{"Operation":"OperationTypeApply","Info":"maintenance","Version":"1.9.5","Path":"",` +
	`"ID":"6d3b0c8e-lock","Who":"root","Created":"2026-11-04T10:11:12.345Z"}`

// wantLockConflictHint and wantUnlockConflictHint are the guidance a refused
// lock and a refused unlock carry, written out rather than read back from the
// constants that produce them.
const (
	wantLockHeldHint = `the state is already locked under lock ID "held-by-ci", held by ci-bot since 2026-11-04T09:00:00.000Z; ` +
		"release it with admin.terraform_state_unlock, passing that ID as lock_id, or no lock_id to release it whatever its ID"
	wantLockConflictHint = "the state is already locked; release the lock with admin.terraform_state_unlock, " +
		"passing the lock_id it was taken under, or no lock_id to release it whatever its ID"
	wantUnlockConflictHint = "the lock is held under another ID: pass the lock_id it was taken under, " +
		"or leave lock_id out to release it whatever its ID"
)

// TestLock_SendsTheLockInfoAndPublishesTheLockGitLabRecorded verifies the
// request GitLab requires and the answer it gives: client-go's Lock sends no
// body and GitLab refused every call without the lock info with 400 (issue
// 1597), so the mock reads the body whole and holds it to the seven keys
// lib/api/terraform/state.rb requires, each with the value the caller gave,
// and the output to the lock GitLab answered with, field by field, rather
// than to what the caller sent: GitLab answers the holder and the time it
// recorded in place of the caller's.
func TestLock_SendsTheLockInfoAndPublishesTheLockGitLabRecorded(t *testing.T) {
	var body atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.EscapedPath() != lockedStatePath {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"unreadable body"}`)
			return
		}
		body.Store(raw)
		testutil.RespondJSON(w, http.StatusOK, lockAnswer)
	}))

	out, err := Lock(t.Context(), client, LockInput{
		ProjectID: lockedProject, Name: lockedState, LockID: "6d3b0c8e-lock",
		Operation: "OperationTypeApply", Info: "maintenance", Who: "ops@laptop",
		Version: "1.9.5", Created: "2026-11-04T10:11:00Z", Path: "",
	})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}

	raw, _ := body.Load().([]byte)
	var sent map[string]any
	if err = json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("the lock body %q is not a JSON object: %v", raw, err)
	}
	wantSent := map[string]any{
		"ID": "6d3b0c8e-lock", "Operation": "OperationTypeApply", "Info": "maintenance", "Who": "ops@laptop",
		"Version": "1.9.5", "Created": "2026-11-04T10:11:00Z", "Path": "",
	}
	if !reflect.DeepEqual(sent, wantSent) {
		t.Errorf("lock body = %v, want %v", sent, wantSent)
	}

	want := LockOutput{
		Success: true, Message: "State 'prod.eu' locked",
		LockID: "6d3b0c8e-lock", Who: "root", Created: "2026-11-04T10:11:12.345Z",
		Operation: "OperationTypeApply", Info: "maintenance", Version: "1.9.5",
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Lock() = %+v, want %+v", out, want)
	}
}

// TestLock_LockIDAlone_SendsEveryKeyGitLabRequires verifies a lock the caller
// gives only its ID: GitLab's requires reads whether each key is present,
// and keeps none of the other six, so each is sent empty rather than left
// out, which GitLab would refuse with 400.
func TestLock_LockIDAlone_SendsEveryKeyGitLabRequires(t *testing.T) {
	var body atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"unreadable body"}`)
			return
		}
		body.Store(raw)
		testutil.RespondJSON(w, http.StatusOK, `{"ID":"only-id","Who":"root","Created":"2026-11-04T10:11:12.345Z","Operation":"","Info":"","Version":"","Path":""}`)
	}))

	out, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: "only-id"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	raw, _ := body.Load().([]byte)
	var sent map[string]any
	if err = json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("the lock body %q is not a JSON object: %v", raw, err)
	}
	wantSent := map[string]any{"ID": "only-id", "Operation": "", "Info": "", "Who": "", "Version": "", "Created": "", "Path": ""}
	if !reflect.DeepEqual(sent, wantSent) {
		t.Errorf("lock body = %v, want %v", sent, wantSent)
	}
	want := LockOutput{Success: true, Message: "State 'state1' locked", LockID: "only-id", Who: "root", Created: "2026-11-04T10:11:12.345Z"}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Lock() = %+v, want %+v", out, want)
	}
}

// TestLock_NoLockID_IsRefusedBeforeAnyRequest verifies a lock with no ID, or
// one of blanks alone, is refused under the parameter's name and sends
// nothing: GitLab takes a lock only under an ID that is not blank, and a
// request without one is answered with an error naming no parameter.
func TestLock_NoLockID_IsRefusedBeforeAnyRequest(t *testing.T) {
	for _, lockID := range []string{"", "  "} {
		t.Run("lock_id "+strconv.Quote(lockID), func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: lockID})
			if err == nil {
				t.Fatal(errExpectedErr)
			}
			want := "gitlab_lock_terraform_state: lock_id is required"
			if !strings.HasPrefix(err.Error(), want) {
				t.Errorf("error = %q, want it to open with %q", err.Error(), want)
			}
		})
	}
}

// TestLock_AlreadyLocked_NamesTheLockToRelease verifies the answer to a state
// somebody already holds. lib/api/terraform/state.rb answers 409 with the
// lock it holds, a body the error leaves out since it is not one of GitLab's
// error messages, so the hint reads it from the capture and names the lock's
// ID, its holder and its time, by which the caller can release that lock
// rather than whatever lock there is. An answer that names no lock, a body
// without an ID or one that is not the lock info, leaves the hint general.
func TestLock_AlreadyLocked_NamesTheLockToRelease(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{
			name: "the lock it holds",
			body: `{"ID":"held-by-ci","Who":"ci-bot","Created":"2026-11-04T09:00:00.000Z","Operation":"","Info":"","Version":"","Path":""}`,
			want: wantLockHeldHint,
		},
		{name: "no lock ID", body: `{"Who":"ci-bot"}`, want: wantLockConflictHint},
		{name: "not the lock info", body: `["locked"]`, want: wantLockConflictHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusConflict, tc.body)
			}))
			_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: "mine"})
			if err == nil {
				t.Fatal(errExpectedErr)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to carry %q", err.Error(), tc.want)
			}
		})
	}
}

// TestLock_Refused_CarriesNoConflictHint verifies the conflict hint follows
// a 409 alone: a lock GitLab refuses for another reason, here the 400 of a
// lock info it does not accept, carries GitLab's reason and no word about a
// lock somebody holds.
func TestLock_Refused_CarriesNoConflictHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"error":"ID is too long (maximum is 255 characters)"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: strings.Repeat("x", 256)})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "ID is too long") || strings.Contains(err.Error(), "already locked") {
		t.Errorf("error = %q, want GitLab's reason and no conflict hint", err.Error())
	}
}

// TestLock_AnswerNotLockInfo_IsAnErrorThatSaysTheLockWasAccepted verifies an
// answer that does not decode as the lock info is reported, and reported as
// what it is: GitLab accepted the lock, so the error points at the state read
// that says whether it holds, rather than reading as a lock refused.
func TestLock_AnswerNotLockInfo_IsAnErrorThatSaysTheLockWasAccepted(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `["not", "a", "lock"]`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: "mine"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	for _, want := range []string{"gitlab_lock_terraform_state: ", "GitLab accepted the lock, but its answer is not the lock info", "admin.terraform_state_get"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to carry %q", err.Error(), want)
			}
		})
	}
}

// TestLockInfoBody_WritesTheLockInfoAsTheBody verifies the request option on
// a request of its own, so what it writes is read without client-go between:
// the body is the lock info and nothing else, under the keys GitLab reads.
func TestLockInfoBody_WritesTheLockInfoAsTheBody(t *testing.T) {
	req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/v4/projects/1/terraform/state/s/lock", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if err = lockInfoBody(lockInfo{ID: "i", Operation: "o", Info: "n", Who: "w", Version: "v", Created: "c", Path: "p"})(req); err != nil {
		t.Fatalf("lockInfoBody() error = %v", err)
	}
	raw, err := req.BodyBytes()
	if err != nil {
		t.Fatalf("BodyBytes: %v", err)
	}
	want := `{"ID":"i","Operation":"o","Info":"n","Who":"w","Version":"v","Created":"c","Path":"p"}`
	if string(raw) != want {
		t.Errorf("body = %s, want %s", raw, want)
	}
}

// TestUnlock_LockID_IsSentForGitLabToMatch verifies the unlock names the lock
// it releases: GitLab releases a lock only when the ID it is sent matches the
// one the lock was taken under, so the caller's lock_id reaches the query as
// ID, the parameter lib/api/terraform/state.rb reads.
func TestUnlock_LockID_IsSentForGitLabToMatch(t *testing.T) {
	var query atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.EscapedPath() != lockedStatePath {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			return
		}
		query.Store(r.URL.Query())
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Unlock(t.Context(), client, UnlockInput{ProjectID: lockedProject, Name: lockedState, LockID: "6d3b0c8e lock&1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	got, _ := query.Load().(url.Values)
	if want := (url.Values{"ID": {"6d3b0c8e lock&1"}}); !reflect.DeepEqual(got, want) {
		t.Errorf("unlock query = %v, want %v", got, want)
	}
	if want := (UnlockOutput{Success: true, Message: "State 'prod.eu' unlocked"}); !reflect.DeepEqual(out, want) {
		t.Errorf("Unlock() = %+v, want %+v", out, want)
	}
}

// TestUnlock_NoLockID_SendsNoID verifies an unlock without a lock_id sends no
// ID at all, which GitLab reads as a release whatever the lock's ID, as
// terraform force-unlock does; an empty ID would be compared with the lock's
// and refused with 409.
func TestUnlock_NoLockID_SendsNoID(t *testing.T) {
	var rawQuery atomic.Value
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery.Store(r.URL.RawQuery)
		w.WriteHeader(http.StatusOK)
	}))
	if _, err := Unlock(t.Context(), client, UnlockInput{ProjectID: "1", Name: "state1"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if got, _ := rawQuery.Load().(string); got != "" {
		t.Errorf("unlock query = %q, want none", got)
	}
}

// TestUnlock_OtherLockID_HintsTheLockIDOrNone verifies the answer to an
// unlock that names another lock: GitLab answers 409, and the error says the
// lock is held under another ID and how to release it.
func TestUnlock_OtherLockID_HintsTheLockIDOrNone(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	_, err := Unlock(t.Context(), client, UnlockInput{ProjectID: "1", Name: "state1", LockID: "not-the-lock"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantUnlockConflictHint) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), wantUnlockConflictHint)
	}
}

// TestUnlock_Refused_CarriesNoConflictHint verifies the unlock's hint follows
// a 409 alone: a refusal for another reason carries GitLab's reason and no
// word about another lock ID.
func TestUnlock_Refused_CarriesNoConflictHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"Write access to this Terraform state is restricted by a protection rule"}`)
	}))
	_, err := Unlock(t.Context(), client, UnlockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "restricted by a protection rule") || strings.Contains(err.Error(), "another ID") {
		t.Errorf("error = %q, want GitLab's reason and no conflict hint", err.Error())
	}
}

// TestLockAndUnlock_EachReportsItsOwnOutcome verifies that the two handlers
// say which of the two things happened, and say it about the state the caller
// named. The two messages differ in one word nothing else reads, so the two
// lines could be exchanged and every other assertion here still passed: a
// model would then be told a state was locked by the call that unlocked it.
func TestLockAndUnlock_EachReportsItsOwnOutcome(t *testing.T) {
	client := testutil.NewTestClient(t, terraformHandler())

	lock, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: "lock-1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if lock.Message != "State 'state1' locked" || !lock.Success {
		t.Errorf("Lock() = %+v, want success and the message that the state was locked", lock)
	}
	unlock, err := Unlock(t.Context(), client, UnlockInput{ProjectID: "1", Name: "state1", LockID: "lock-1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if want := (UnlockOutput{Success: true, Message: "State 'state1' unlocked"}); !reflect.DeepEqual(unlock, want) {
		t.Errorf("Unlock() = %+v, want %+v", unlock, want)
	}
}

// listHints is the guidance a Terraform state list closes with.
const listHints = "\n---\n💡 **Next steps:**\n" +
	"- Use `admin.terraform_state_get` to view details of a specific state\n"

// TestFormatListMarkdown verifies FormatListMarkdown renders the whole table:
// a heading counting the states, one row each, a state nothing has written to
// yet saying so rather than showing a serial of zero, a state no lock is held
// on saying "unlocked" rather than leaving the lock column blank, and a state
// GitLab has deleted but not yet removed saying when, where a live one says
// "no", so a deleted state in the list does not read as live. The page is the
// whole list, so the cursor line says there are no more pages.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{States: []StateItem{
		{Name: "state1", LatestSerial: 3, UpdatedAt: "2026-02-03T04:05:06Z", LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z"},
		{Name: "fresh", UpdatedAt: "2026-01-02T03:04:05Z"},
	}, Count: 2})
	want := "## Terraform States (2)\n\n" +
		"| Name | Latest Serial | Updated | Locked Since | Deleted |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| state1 | 3 | 3 Feb 2026 04:05 UTC | 4 Mar 2026 05:06 UTC | 5 Apr 2026 06:07 UTC |\n" +
		"| fresh | no versions | 2 Jan 2026 03:04 UTC | unlocked | no |\n" +
		"\nShowing 2 items | no more pages\n" +
		listHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatListMarkdown_FirstOfSeveralPages verifies a page that is not the
// whole list says so twice over: the heading counts the states GitLab says
// the project holds rather than the rows shown, and the cursor line names the
// cursor that asks for the next page. A page with a next page and no count,
// which GitLab does not send but a decoder can be handed, says more are
// available instead of a total.
func TestFormatListMarkdown_FirstOfSeveralPages(t *testing.T) {
	page := ListOutput{
		States:     []StateItem{{Name: "production", UpdatedAt: "2026-02-03T04:05:06Z"}},
		Count:      3,
		Pagination: toolutil.GraphQLForwardPaginationOutput{HasNextPage: true, EndCursor: "c1"},
	}
	got := FormatListMarkdown(page)
	if !strings.HasPrefix(got, "## Terraform States (3)\n") {
		t.Errorf("heading of %q, want the 3 GitLab counted", got)
	}
	if !strings.Contains(got, "\nShowing 1 items | next page cursor: `c1`\n") {
		t.Errorf("FormatListMarkdown() = %q, want the cursor of the next page", got)
	}
	page.Count = 0
	if got = FormatListMarkdown(page); !strings.HasPrefix(got, "## Terraform States (1 shown, more available)\n") {
		t.Errorf("heading of %q, want the page counted with more available", got)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// DeleteVersion: error
// ---------------------------------------------------------------------------.

// TestDeleteVersion_Error verifies DeleteVersion when error: a 400 is not the
// status its hint was written for, so the refusal is reported without advice
// about a serial the caller may well have got right.
func TestDeleteVersion_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 99})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if strings.Contains(err.Error(), wantDeleteVersionNotFoundHint) {
		t.Errorf("error = %q, want no serial hint on a 400", err.Error())
	}
}

// TestDeleteVersion_NotFound_SaysTheStateOrTheSerialWasNotFound verifies the
// status DeleteVersion's hint is written for, and that the hint says only what
// a 404 means here: no state by that name, or no version with that serial.
// It used to add that the latest version cannot be deleted, which GitLab
// 19.4.2 does not enforce: it answers a delete of the latest version 204 and
// deletes it, locked state or not.
func TestDeleteVersion_NotFound_SaysTheStateOrTheSerialWasNotFound(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	err := DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 99})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), wantDeleteVersionNotFoundHint) {
		t.Errorf("error = %q, want it to carry %q", err.Error(), wantDeleteVersionNotFoundHint)
	}
	if strings.Contains(err.Error(), "cannot delete") {
		t.Errorf("error = %q, want no claim about a version GitLab would refuse to delete", err.Error())
	}
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown
// ---------------------------------------------------------------------------.

// The lock sentences the cards carry, written out rather than taken from the
// constants that produce them, so a constant that went back to sending a
// reader somewhere else fails here.
const (
	wantLockHint   = "Use `admin.terraform_state_lock` with a lock_id of your choosing to lock it"
	wantUnlockHint = "Use `admin.terraform_state_unlock` to release its lock, with the lock_id it was taken under, " +
		"or with none to clear a stale lock"
)

// stateHints is the guidance a Terraform state card closes with: how to lock
// it, how to release a lock or clear a stale one, and how to delete it.
const stateHints = "\n---\n💡 **Next steps:**\n" +
	"- " + wantLockHint + "\n" +
	"- " + wantUnlockHint + "\n" +
	"- Use `admin.terraform_state_delete` to remove it\n"

// TestFormatStateMarkdown_Coverage verifies FormatStateMarkdown renders the
// whole card for a written state, closing with how to lock it, release its
// lock and delete it.
func TestFormatStateMarkdown_Coverage(t *testing.T) {
	got := FormatStateMarkdown(StateItem{Name: "prod-state", LatestSerial: 42, DownloadPath: "/dl/path"})
	want := "## Terraform State: prod-state\n\n" +
		"- **Latest Serial**: 42\n" +
		"- **Download Path**: `/dl/path`\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatStateMarkdown_Times verifies the card writes the four times each
// under its own label, which is what tells them apart: they share a layout, so
// two of them traded would read as plausible dates. A state locked and
// deleted says both, and the deletion carries the sentence that explains why
// a deleted state is still there to read.
func TestFormatStateMarkdown_Times(t *testing.T) {
	got := FormatStateMarkdown(StateItem{
		Name: "prod-state", LatestSerial: 42, DownloadPath: "/dl/path",
		LatestVersionCreatedAt: "2026-05-06T07:08:09Z", LatestVersionUpdatedAt: "2026-06-07T08:09:10Z",
		CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-02-03T04:05:06Z",
		LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z",
	})
	want := "## Terraform State: prod-state\n\n" +
		"- **Latest Serial**: 42\n" +
		"- **Download Path**: `/dl/path`\n" +
		"- **Latest Version Created**: 6 May 2026 07:08 UTC\n" +
		"- **Latest Version Updated**: 7 Jun 2026 08:09 UTC\n" +
		"- **Created**: 2 Jan 2026 03:04 UTC\n" +
		"- **Updated**: 3 Feb 2026 04:05 UTC\n" +
		"- **Locked**: 4 Mar 2026 05:06 UTC\n" +
		"- **Deleted**: 5 Apr 2026 06:07 UTC\n" +
		"\nGitLab has deleted this state and removes it in the background; until then it can still be read.\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatStateMarkdown_UnwrittenState verifies a state with no serial and
// no download path renders neither row and says why, rather than printing two
// labels with nothing after them.
func TestFormatStateMarkdown_UnwrittenState(t *testing.T) {
	got := FormatStateMarkdown(StateItem{Name: "fresh"})
	want := "## Terraform State: fresh\n\n" +
		"GitLab has recorded no versions of this state.\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatStateMarkdown_HalfWrittenState verifies that a state carrying one
// of the two version fields is not announced as having no versions. The note
// is written only when both are missing, and nothing held that: with the two
// conditions joined by "or" instead of "and" the whole suite stayed green,
// because every card tested had either both fields or neither. The first two
// cases give the card one field, which are also the two states GitLab really
// sends: a serial it omits for a version it has, and a download path it
// withholds. The third gives it neither and the latest version's times, which
// GitLab sends whenever there is a latest version, so they alone prove one.
func TestFormatStateMarkdown_HalfWrittenState(t *testing.T) {
	tests := []struct {
		name  string
		state StateItem
		want  string
	}{
		{
			name:  "download path without a serial",
			state: StateItem{Name: "downloadable", DownloadPath: "/dl/downloadable"},
			want: "## Terraform State: downloadable\n\n" +
				"- **Download Path**: `/dl/downloadable`\n" +
				stateHints,
		},
		{
			name:  "serial without a download path",
			state: StateItem{Name: "serialled", LatestSerial: 7},
			want: "## Terraform State: serialled\n\n" +
				"- **Latest Serial**: 7\n" +
				stateHints,
		},
		{
			name:  "a latest version known only by its times",
			state: StateItem{Name: "timed", LatestVersionCreatedAt: "2026-05-06T07:08:09Z", LatestVersionUpdatedAt: "2026-05-06T07:08:09Z"},
			want: "## Terraform State: timed\n\n" +
				"- **Latest Version Created**: 6 May 2026 07:08 UTC\n" +
				"- **Latest Version Updated**: 6 May 2026 07:08 UTC\n" +
				stateHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatStateMarkdown(tt.state); got != tt.want {
				t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatStateMarkdown_UnnamedState verifies the heading of a state GitLab
// sent no name for: the resource alone, never a label ending in a colon with
// nothing behind it. Every other card here is named, so the branch that drops
// the colon had never run.
func TestFormatStateMarkdown_UnnamedState(t *testing.T) {
	got := FormatStateMarkdown(StateItem{})
	want := "## Terraform State\n\n" +
		"GitLab has recorded no versions of this state.\n" +
		stateHints
	if got != want {
		t.Errorf("FormatStateMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatLockMarkdown
// ---------------------------------------------------------------------------.

// TestFormatLockMarkdown_Coverage verifies FormatLockMarkdown renders the
// whole card of a lock: the outcome as the flag glyph rather than the word
// "true", the lock's ID as code, its holder and time, every key of the lock
// info GitLab answered back, a value of a reader's own escaped, and the way
// to release the lock.
func TestFormatLockMarkdown_Coverage(t *testing.T) {
	got := FormatLockMarkdown(LockOutput{
		Success: true, Message: "State 'x' locked", LockID: "6d3b-lock", Who: "root",
		Created: "2026-11-04T10:11:12.345Z", Operation: "OperationTypeApply", Info: "a|b",
		Version: "1.9.5", Path: "envs/prod",
	})
	want := "## Terraform State Lock\n\n" +
		"- **Success**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: State 'x' locked\n" +
		"- **Lock ID**: `6d3b-lock`\n" +
		"- **Holder**: root\n" +
		"- **Created**: 4 Nov 2026 10:11 UTC\n" +
		"- **Operation**: OperationTypeApply\n" +
		"- **Info**: a&#124;b\n" +
		"- **Terraform Version**: 1.9.5\n" +
		"- **Path**: `envs/prod`\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use `admin.terraform_state_unlock` with this lock_id to release the lock\n"
	if got != want {
		t.Errorf("FormatLockMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatLockMarkdown_LockInfoLeftEmpty verifies a lock whose four echoed
// keys GitLab answered empty writes no label for them, rather than labels
// with nothing after them.
func TestFormatLockMarkdown_LockInfoLeftEmpty(t *testing.T) {
	got := FormatLockMarkdown(LockOutput{Success: true, Message: "State 'x' locked", LockID: "id", Who: "root"})
	want := "## Terraform State Lock\n\n" +
		"- **Success**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: State 'x' locked\n" +
		"- **Lock ID**: `id`\n" +
		"- **Holder**: root\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use `admin.terraform_state_unlock` with this lock_id to release the lock\n"
	if got != want {
		t.Errorf("FormatLockMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatUnlockMarkdown_Coverage verifies FormatUnlockMarkdown renders the
// whole card of a released lock, closing with how to check it.
func TestFormatUnlockMarkdown_Coverage(t *testing.T) {
	got := FormatUnlockMarkdown(UnlockOutput{Success: true, Message: "State 'x' unlocked"})
	want := "## Terraform State Unlock\n\n" +
		"- **Success**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: State 'x' unlocked\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use `admin.terraform_state_get` to check that the state is no longer locked\n"
	if got != want {
		t.Errorf("FormatUnlockMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatListMarkdown: empty
// ---------------------------------------------------------------------------.

// TestFormatListMarkdown_Empty verifies an empty list is the one sentence and
// nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := FormatListMarkdown(ListOutput{States: nil})
	want := "No Terraform states found.\n"
	if got != want {
		t.Errorf("FormatListMarkdown() = %q, want %q", got, want)
	}
}

// TestTerraformStates_EachHandlerReachesItsOwnEndpoint drives all six handlers
// against a mux that serves each REST path and method exactly, so one of the
// four pointed at a sibling's path fails rather than being answered by a
// catch-all. It says nothing of the kind about List and Get: both post to the
// one GraphQL endpoint, which has no sibling path to be pointed at, and what
// they ask for is held by TestGet_QueryNamesTheProjectAndTheState and
// TestList_QueryNamesTheProject instead.
func TestTerraformStates_EachHandlerReachesItsOwnEndpoint(t *testing.T) {
	client := testutil.NewTestClient(t, terraformHandler())

	tests := []struct {
		name string
		call func() error
	}{
		{name: "list", call: func() error {
			_, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"})
			return err
		}},
		{name: "get", call: func() error {
			_, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "state1"})
			return err
		}},
		{name: "delete", call: func() error {
			return Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
		}},
		{name: "delete_version", call: func() error {
			return DeleteVersion(t.Context(), client, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 5})
		}},
		{name: "lock", call: func() error {
			_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1", LockID: "lock-1"})
			return err
		}},
		{name: "unlock", call: func() error {
			_, err := Unlock(t.Context(), client, UnlockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s error = %v, want nil", tt.name, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Shared mock handler
// ---------------------------------------------------------------------------.

// terraformHandler supports terraform handler assertions in terraformstates tests.
func terraformHandler() http.Handler {
	mux := http.NewServeMux()

	graphQLListResp := `{"data":{"project":{"terraformStates":{"nodes":[{"name":"state1","latestVersion":{"serial":5,"downloadPath":"/dl"}}]}}}}`
	graphQLGetResp := `{"data":{"project":{"terraformState":{"name":"state1","latestVersion":{"serial":3,"downloadPath":"/dl/state1"}}}}}`

	// GraphQL endpoint for List and Get. The whole body is read: one Read into
	// a fixed buffer is allowed to return a short prefix, so which of the two
	// documents arrived would have been decided by however much of it happened
	// to be in hand.
	mux.HandleFunc("POST /api/graphql", func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"errors":[{"message":"unreadable body"}]}`)
			return
		}
		if strings.Contains(string(raw), "terraformStates") {
			testutil.RespondJSON(w, http.StatusOK, graphQLListResp)
		} else {
			testutil.RespondJSON(w, http.StatusOK, graphQLGetResp)
		}
	})

	// The project lookup the delete makes for a numeric project_id, before
	// it reads the state, which the GraphQL route above answers unlocked.
	mux.HandleFunc("GET /api/v4/projects/1", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":1,"path_with_namespace":"group/project"}`)
	})

	// Delete state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete version
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/versions/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Lock state, answered with the lock GitLab took
	mux.HandleFunc("POST /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"ID":"lock-1","Who":"root","Created":"2026-11-04T10:11:12.345Z","Operation":"","Info":"","Version":"","Path":""}`)
	})

	// Unlock state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

// TestOperationLabels_EachHandlerReportsUnderItsOwnName holds the six
// operation labels to the handler each was given to.
//
// The label is the first argument of every WrapErr call and becomes the
// prefix of the message a model reads, and nothing in this package asserted
// one: the hint assertions are over the sentence alone, which is a different
// string. Six labels of one type with no reader is six values that could
// trade places, so a caller whose delete was refused would be told the list
// had failed. Each case drives its handler into a refusal GitLab really
// sends and reads the prefix back.
func TestOperationLabels_EachHandlerReportsUnderItsOwnName(t *testing.T) {
	cases := []struct {
		operation string
		call      func(*gitlabclient.Client) error
	}{
		{"gitlab_list_terraform_states", func(c *gitlabclient.Client) error {
			_, err := List(t.Context(), c, ListInput{ProjectPath: "group/project"})
			return err
		}},
		{"gitlab_get_terraform_state", func(c *gitlabclient.Client) error {
			_, err := Get(t.Context(), c, GetInput{ProjectPath: "group/project", Name: "state1"})
			return err
		}},
		{"gitlab_delete_terraform_state", func(c *gitlabclient.Client) error {
			return Delete(t.Context(), c, DeleteInput{ProjectID: "1", Name: "state1"})
		}},
		{"gitlab_delete_terraform_state_version", func(c *gitlabclient.Client) error {
			return DeleteVersion(t.Context(), c, DeleteVersionInput{ProjectID: "1", Name: "state1", Serial: 3})
		}},
		{"gitlab_lock_terraform_state", func(c *gitlabclient.Client) error {
			_, err := Lock(t.Context(), c, LockInput{ProjectID: "1", Name: "state1", LockID: "lock-1"})
			return err
		}},
		{"gitlab_unlock_terraform_state", func(c *gitlabclient.Client) error {
			_, err := Unlock(t.Context(), c, UnlockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.operation, func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
			}))

			err := tc.call(client)
			if err == nil {
				t.Fatalf("%s error = nil, want the refusal", tc.operation)
			}
			if !strings.HasPrefix(err.Error(), tc.operation+": ") {
				t.Errorf("error = %q, want it to open with %q", err.Error(), tc.operation+": ")
			}
		})
	}
}
