// terraform_states_test.go contains unit tests for the Terraform state MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package terraformstates

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

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
	wantDeleteVersionNotFoundHint = "verify serial with admin.terraform_state_list; cannot delete the latest version. " +
		"Use admin.terraform_state_delete to remove the entire state"
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
			{"name":"production","latestVersion":{"serial":11,"downloadPath":"/dl/one"},
			 "createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z",
			 "lockedAt":"2026-03-04T05:06:07Z","deletedAt":"2026-04-05T06:07:08Z"},
			{"name":"staging","latestVersion":{"serial":22,"downloadPath":"/dl/two"},
			 "createdAt":"2025-05-06T07:08:09Z","updatedAt":"2025-06-07T08:09:10Z",
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
			CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-02-03T04:05:06Z",
			LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z",
		},
		{
			Name: "staging", LatestSerial: 22, DownloadPath: "/dl/two",
			CreatedAt: "2025-05-06T07:08:09Z", UpdatedAt: "2025-06-07T08:09:10Z",
		},
	}
	if !reflect.DeepEqual(out.States, want) {
		t.Errorf("List() states = %+v, want %+v", out.States, want)
	}
}

// stateItemSchemaFields maps each field StateItem publishes to the field of
// the pinned schema it carries, as object and field name.
var stateItemSchemaFields = map[string][2]string{
	"name":          {"TerraformState", "name"},
	"latest_serial": {"TerraformStateVersion", "serial"},
	"download_path": {"TerraformStateVersion", "downloadPath"},
	"created_at":    {"TerraformState", "createdAt"},
	"updated_at":    {"TerraformState", "updatedAt"},
	"locked_at":     {"TerraformState", "lockedAt"},
	"deleted_at":    {"TerraformState", "deletedAt"},
}

// stateSelection is what one client-go document selects of a Terraform
// state: the fields it asks for on the state itself and on its latest
// version, by the schema object each belongs to.
type stateSelection map[string][]string

// selectedFields reads a client-go document and returns the fields it selects
// directly on the state found by following path from the operation's root,
// and directly on that state's latestVersion. Only direct selections count,
// so a createdAt asked of the latest version is not taken for the state's,
// and a name that is an argument of the field is not taken for a selection.
func selectedFields(t *testing.T, document string, path ...string) stateSelection {
	t.Helper()
	parsed, err := parser.ParseQuery(&ast.Source{Input: document})
	if err != nil {
		t.Fatalf("parse client-go's document: %v\n%s", err, document)
	}
	if len(parsed.Operations) != 1 {
		t.Fatalf("client-go's document holds %d operations, want one:\n%s", len(parsed.Operations), document)
	}
	selections := parsed.Operations[0].SelectionSet
	for _, name := range path {
		field := childField(selections, name)
		if field == nil {
			t.Fatalf("client-go's document selects no %s along %v:\n%s", name, path, document)
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
// schema has the field, a field the schema types non-null is always written
// and a nullable one is left out when GitLab sent null, and both of
// client-go's documents, the list's and the get's, select it on the object it
// belongs to. The documents are written inside client-go, and a field one of
// them stops selecting would arrive empty on that path with every other test
// green, since a mock answers with whatever its fixture holds. The map above
// must name every published field, so a field added to StateItem without a
// row fails here too.
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
			if omitted := strings.Contains(options, "omitempty"); omitted == definition.Type.NonNull {
				t.Errorf("%s omitempty = %v, want %v for %s.%s typed %s", jsonName, omitted, !definition.Type.NonNull, object, name, definition.Type)
			}
			for document, selected := range documents {
				t.Run(document, func(t *testing.T) {
					if !slices.Contains(selected[object], name) {
						t.Errorf("client-go's %s document selects no %s on %s (it selects %v), so %s would arrive empty", document, name, object, selected[object], jsonName)
					}
				})
			}
		})
	}
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
			`{"data":{"project":{"terraformState":{"name":"production","latestVersion":{"serial":11,"downloadPath":"/dl/one"},`+
				`"createdAt":"2026-01-02T03:04:05Z","updatedAt":"2026-02-03T04:05:06Z","lockedAt":"2026-03-04T05:06:07Z","deletedAt":"2026-04-05T06:07:08Z"}}}}`)
	}))
	out, err := Get(t.Context(), client, GetInput{ProjectPath: "group/project", Name: "production"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	want := StateItem{
		Name: "production", LatestSerial: 11, DownloadPath: "/dl/one",
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
// project the caller named rather than one written into the handler.
func TestList_QueryNamesTheProject(t *testing.T) {
	var seen string
	client := testutil.NewTestClient(t, recordGraphQL(t, &seen,
		`{"data":{"project":{"terraformStates":{"nodes":[]}}}}`))
	if _, err := List(t.Context(), client, ListInput{ProjectPath: "group/project"}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !strings.Contains(seen, `fullPath: "group/project"`) {
		t.Errorf("query = %q, want it to select the caller's project", seen)
	}
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	err := Delete(t.Context(), client, DeleteInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
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
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))
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

// TestLock verifies Lock.
func TestLock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestLock_Error verifies Lock when error.
func TestLock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"already locked"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestLock_BadRequest_HintsSDKLimitation verifies that the guaranteed 400 from
// GitLab (client-go sends no Terraform lock-info body) is wrapped with a hint
// pointing at the terraform CLI and admin.terraform_state_unlock.
func TestLock_BadRequest_HintsSDKLimitation(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"400 Bad request"}`)
	}))
	_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
	if !strings.Contains(err.Error(), "lock-info body") || !strings.Contains(err.Error(), "admin.terraform_state_unlock") {
		t.Errorf("error = %q, want SDK-limitation hint with unlock alternative", err.Error())
	}
}

// TestUnlock verifies Unlock.
func TestUnlock(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	out, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if !out.Success {
		t.Error("expected success")
	}
}

// TestUnlock_Error verifies Unlock when error.
func TestUnlock_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusConflict, `{"message":"not locked"}`)
	}))
	_, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
	if err == nil {
		t.Fatal(errExpectedErr)
	}
}

// TestLockAndUnlock_EachReportsItsOwnOutcome verifies that the two handlers
// say which of the two things happened, and say it about the state the caller
// named. Both answer LockOutput{Success: true} and differ only in one word of
// a message nothing used to read, so the two lines could be exchanged and
// every assertion in this package still passed: a model would then be told a
// state was locked by the call that unlocked it.
func TestLockAndUnlock_EachReportsItsOwnOutcome(t *testing.T) {
	client := testutil.NewTestClient(t, terraformHandler())

	tests := []struct {
		name string
		call func() (LockOutput, error)
		want LockOutput
	}{
		{
			name: "lock",
			call: func() (LockOutput, error) {
				return Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
			},
			want: LockOutput{Success: true, Message: "State 'state1' locked"},
		},
		{
			name: "unlock",
			call: func() (LockOutput, error) {
				return Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
			},
			want: LockOutput{Success: true, Message: "State 'state1' unlocked"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call()
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s() = %+v, want %+v", tt.name, got, tt.want)
			}
		})
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
// "no", so a deleted state in the list does not read as live.
func TestFormatListMarkdown(t *testing.T) {
	got := FormatListMarkdown(ListOutput{States: []StateItem{
		{Name: "state1", LatestSerial: 3, UpdatedAt: "2026-02-03T04:05:06Z", LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z"},
		{Name: "fresh", UpdatedAt: "2026-01-02T03:04:05Z"},
	}})
	want := "## Terraform States (2)\n\n" +
		"| Name | Latest Serial | Updated | Locked Since | Deleted |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| state1 | 3 | 3 Feb 2026 04:05 UTC | 4 Mar 2026 05:06 UTC | 5 Apr 2026 06:07 UTC |\n" +
		"| fresh | no versions | 2 Jan 2026 03:04 UTC | unlocked | no |\n" +
		listHints
	if got != want {
		t.Errorf("FormatListMarkdown() =\n%q\nwant:\n%q", got, want)
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

// TestDeleteVersion_NotFound_HintsTheSerialAndTheWholeStateAlternative
// verifies the status DeleteVersion's hint was written for. The package used
// to drive this handler only through a 400, so the branch that names the
// serial and points at deleting the whole state never ran.
func TestDeleteVersion_NotFound_HintsTheSerialAndTheWholeStateAlternative(t *testing.T) {
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
}

// ---------------------------------------------------------------------------
// FormatStateMarkdown
// ---------------------------------------------------------------------------.

// The two lock sentences a card carries, written out rather than taken from
// the constants that produce them. Building the expectation out of
// hintLockNeedsCLI would let that constant go back to telling a reader to call
// gitlab_lock_terraform_state, the call GitLab refuses every time and the
// whole reason the sentence was rewritten, with every test still green.
const (
	wantLockHint = "Locking a state needs the terraform CLI against the GitLab HTTP backend: " +
		"`admin.terraform_state_lock` sends no lock-info body, which GitLab refuses"
	wantUnlockHint = "Use `admin.terraform_state_unlock` to clear a stale lock"
)

// stateHints is the guidance a Terraform state card closes with: what
// locking really needs, how to clear a stale lock, and how to delete.
const stateHints = "\n---\n💡 **Next steps:**\n" +
	"- " + wantLockHint + "\n" +
	"- " + wantUnlockHint + "\n" +
	"- Use `admin.terraform_state_delete` to remove it\n"

// TestFormatStateMarkdown_Coverage verifies FormatStateMarkdown renders the
// whole card for a written state, and that the guidance no longer sends a
// reader to gitlab_lock_terraform_state, which GitLab refuses on every call.
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
		CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-02-03T04:05:06Z",
		LockedAt: "2026-03-04T05:06:07Z", DeletedAt: "2026-04-05T06:07:08Z",
	})
	want := "## Terraform State: prod-state\n\n" +
		"- **Latest Serial**: 42\n" +
		"- **Download Path**: `/dl/path`\n" +
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
// because every card tested had either both fields or neither. Each case here
// gives the card one field, which are also the two states GitLab really
// sends: a serial it omits for a version it has, and a download path it
// withholds.
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
// whole card, with the outcome as the flag glyph rather than the word "true".
func TestFormatLockMarkdown_Coverage(t *testing.T) {
	got := FormatLockMarkdown(LockOutput{Success: true, Message: "State 'x' locked"})
	want := "## Terraform State Lock\n\n" +
		"- **Success**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: State 'x' locked\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + wantLockHint + "\n" +
		"- " + wantUnlockHint + "\n"
	if got != want {
		t.Errorf("FormatLockMarkdown() =\n%q\nwant:\n%q", got, want)
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
			_, err := Lock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
		{name: "unlock", call: func() error {
			_, err := Unlock(t.Context(), client, LockInput{ProjectID: "1", Name: "state1"})
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

	// Delete state
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Delete version
	mux.HandleFunc("DELETE /api/v4/projects/1/terraform/state/state1/versions/5", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Lock state
	mux.HandleFunc("POST /api/v4/projects/1/terraform/state/state1/lock", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
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
			_, err := Lock(t.Context(), c, LockInput{ProjectID: "1", Name: "state1"})
			return err
		}},
		{"gitlab_unlock_terraform_state", func(c *gitlabclient.Client) error {
			_, err := Unlock(t.Context(), c, LockInput{ProjectID: "1", Name: "state1"})
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
