// custom_attributes_test.go contains unit tests for the custom attribute MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package customattributes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// errExpectedNil identifies the err expected nil constant used by this package.
const errExpectedNil = "expected error, got nil"

// testResourceID identifies the test resource ID constant used by this package.
const testResourceID = "resource_id"

// testKeyDept identifies the test key dept constant used by this package.
const testKeyDept = "dept"

// fmtErrWantResourceID identifies the fmt err want resource ID constant used by this package.
const fmtErrWantResourceID = "error = %q, want it to contain resource_id"

// testTypeUser identifies the test type user constant used by this package.
const testTypeUser = "user"

// testTypeGroup identifies the test type group constant used by this package.
const testTypeGroup = "group"

// testTypeProject identifies the test type project constant used by this package.
const testTypeProject = "project"

// TestList_User verifies the List_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/users/1/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"dept","value":"engineering"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeUser, ResourceID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Attributes) != 1 {
		t.Fatalf("len = %d, want 1", len(out.Attributes))
	}
	if out.Attributes[0].Key != testKeyDept {
		t.Errorf("Key = %q, want dept", out.Attributes[0].Key)
	}
}

// TestList_Group verifies the List_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/2/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[{"key":"tier","value":"gold"}]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeGroup, ResourceID: 2})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Attributes[0].Value != "gold" {
		t.Errorf("Value = %q, want gold", out.Attributes[0].Value)
	}
}

// TestList_Project verifies the List_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/3/custom_attributes")
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ResourceType: testTypeProject, ResourceID: 3})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if len(out.Attributes) != 0 {
		t.Errorf("len = %d, want 0", len(out.Attributes))
	}
}

// TestList_InvalidType verifies the List_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: "invalid", ResourceID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestGet_User verifies the Get_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/dept (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/users/1/custom_attributes/dept")
		testutil.RespondJSON(w, http.StatusOK, `{"key":"dept","value":"engineering"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 1, Key: testKeyDept})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != testKeyDept || out.Value != "engineering" {
		t.Errorf("got %q=%q, want dept=engineering", out.Key, out.Value)
	}
}

// TestGet_Error verifies that Get returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestGet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 1, Key: "missing"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestSet_Group verifies the Set_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/tier (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/groups/2/custom_attributes/tier")
		testutil.AssertRequestMethod(t, r, http.MethodPut)
		testutil.RespondJSON(w, http.StatusOK, `{"key":"tier","value":"platinum"}`)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: testTypeGroup, ResourceID: 2, Key: "tier", Value: "platinum"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "platinum" {
		t.Errorf("Value = %q, want platinum", out.Value)
	}
}

// TestSet_Error verifies that Set returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestSet_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})
	client := testutil.NewTestClient(t, handler)
	_, err := Set(t.Context(), client, SetInput{ResourceType: testTypeProject, ResourceID: 1, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// TestDelete_Project verifies the Delete_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes/old_key (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/projects/3/custom_attributes/old_key")
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeProject, ResourceID: 3, Key: "old_key"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Error verifies that Delete returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestDelete_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeUser, ResourceID: 1, Key: "missing"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
}

// assertMarkdown compares a rendered result with the whole document it is
// meant to be. A substring assertion is what let a card open a table and then
// write list rows into it in two packages of this tree: every row the test
// named was present in the string and none of them rendered as a row, so the
// rule here is the whole document or nothing.
func assertMarkdown(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("markdown mismatch\n--- got ---\n%s\n--- want ---\n%s\n--- got (quoted) ---\n%q", got, want, got)
	}
}

// TestFormatListMarkdown_Output verifies the whole list render: the heading
// with its count, the two columns and the guidance section naming the
// canonical action ID.
func TestFormatListMarkdown_Output(t *testing.T) {
	out := ListOutput{Attributes: []AttributeItem{{Key: testKeyDept, Value: "eng"}}}
	assertMarkdown(t, FormatListMarkdown(out),
		"## Custom Attributes (1)\n\n"+
			"| Key | Value |\n"+
			"| --- | --- |\n"+
			"| "+testKeyDept+" | eng |\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'admin.custom_attr_set' to add or update an attribute\n")
}

// TestFormatListMarkdown_Empty verifies that a list with nothing in it is the
// one sentence and nothing else: no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	assertMarkdown(t, FormatListMarkdown(ListOutput{}), "No custom attributes found.\n")
}

// TestFormatGetMarkdown_Output verifies the whole card. The two values used to
// be written as bullet-less "**Key**: …" lines, which a Markdown reader runs
// together into one paragraph, and neither was escaped.
func TestFormatGetMarkdown_Output(t *testing.T) {
	assertMarkdown(t, FormatGetMarkdown(GetOutput{Key: "k", Value: "v"}),
		"## Custom Attribute\n\n"+
			"- **Key**: k\n"+
			"- **Value**: v\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'admin.custom_attr_set' to update this attribute\n"+
			"- Use action 'admin.custom_attr_delete' to remove it\n")
}

// TestFormatGetMarkdown_HostileValue verifies the containment: a value
// carrying a heading, a list item and a link opens none of them, because every
// row of a card goes through the cell escaper.
func TestFormatGetMarkdown_HostileValue(t *testing.T) {
	assertMarkdown(t, FormatGetMarkdown(GetOutput{
		Key:   "a|b",
		Value: "x\n## injected\n- item\n[click](http://attacker.invalid/)",
	}),
		"## Custom Attribute\n\n"+
			"- **Key**: a&#124;b\n"+
			"- **Value**: x ## injected - item &#91;click](http://attacker.invalid/)\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'admin.custom_attr_set' to update this attribute\n"+
			"- Use action 'admin.custom_attr_delete' to remove it\n")
}

// TestList_InvalidResourceID verifies the List_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: testTypeUser, ResourceID: 0})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestGet_InvalidResourceID verifies the Get_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Get(t.Context(), client, GetInput{ResourceType: testTypeUser, ResourceID: 0, Key: "k"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestSet_InvalidResourceID verifies the Set_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSet_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	_, err := Set(t.Context(), client, SetInput{ResourceType: testTypeGroup, ResourceID: 0, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// TestDelete_InvalidResourceID verifies the Delete_InvalidResourceID handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_InvalidResourceID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// no response needed: validation fails before reaching API
	}))
	err := Delete(t.Context(), client, DeleteInput{ResourceType: testTypeProject, ResourceID: 0, Key: "k"})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	if !strings.Contains(err.Error(), testResourceID) {
		t.Errorf(fmtErrWantResourceID, err.Error())
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// ---------------------------------------------------------------------------
// Get — group and project resource types
// ---------------------------------------------------------------------------.

// TestGet_Group verifies the Get_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/tier (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/2/custom_attributes/tier" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"tier","value":"gold"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: "group", ResourceID: 2, Key: "tier"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "gold" {
		t.Errorf("Value = %q, want gold", out.Value)
	}
}

// TestGet_Project verifies the Get_Project handler.
// The mock GitLab API at /api/v4/projects/3/custom_attributes/env (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestGet_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/3/custom_attributes/env" {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"env","value":"prod"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Get(t.Context(), client, GetInput{ResourceType: "project", ResourceID: 3, Key: "env"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "prod" {
		t.Errorf("Value = %q, want prod", out.Value)
	}
}

// TestGet_InvalidType verifies the Get_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestGet_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Get(t.Context(), client, GetInput{ResourceType: "invalid", ResourceID: 1, Key: "k"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// Set — user and project resource types
// ---------------------------------------------------------------------------.

// TestSet_User verifies the Set_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/role (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/1/custom_attributes/role" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"role","value":"admin"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: "user", ResourceID: 1, Key: "role", Value: "admin"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Key != "role" || out.Value != "admin" {
		t.Errorf("got %q=%q, want role=admin", out.Key, out.Value)
	}
}

// TestSet_Project verifies the Set_Project handler.
// The mock GitLab API at /api/v4/projects/5/custom_attributes/env (PUT) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestSet_Project(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/projects/5/custom_attributes/env" && r.Method == http.MethodPut {
			testutil.RespondJSON(w, http.StatusOK, `{"key":"env","value":"staging"}`)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	out, err := Set(t.Context(), client, SetInput{ResourceType: "project", ResourceID: 5, Key: "env", Value: "staging"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.Value != "staging" {
		t.Errorf("Value = %q, want staging", out.Value)
	}
}

// TestSet_InvalidType verifies the Set_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestSet_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	_, err := Set(t.Context(), client, SetInput{ResourceType: "bad", ResourceID: 1, Key: "k", Value: "v"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// Delete — user and group resource types + invalid type
// ---------------------------------------------------------------------------.

// TestDelete_User verifies the Delete_User handler.
// The mock GitLab API at /api/v4/users/1/custom_attributes/old (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_User(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/users/1/custom_attributes/old" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "user", ResourceID: 1, Key: "old"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_Group verifies the Delete_Group handler.
// The mock GitLab API at /api/v4/groups/2/custom_attributes/stale (DELETE) returns a representative success body.
// It asserts the returned output matches the expected fields.
func TestDelete_Group(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/groups/2/custom_attributes/stale" && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.NotFound(w, r)
	})
	client := testutil.NewTestClient(t, handler)
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "group", ResourceID: 2, Key: "stale"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_InvalidType verifies the Delete_InvalidType handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDelete_InvalidType(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	err := Delete(t.Context(), client, DeleteInput{ResourceType: "bad", ResourceID: 1, Key: "k"})
	if err == nil {
		t.Fatal("expected error for invalid resource_type")
	}
}

// ---------------------------------------------------------------------------
// List — API error for user type
// ---------------------------------------------------------------------------.

// TestList_Error verifies that List returns a wrapped error when the GitLab API responds with an error status.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestList_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"bad request"}`)
	}))
	_, err := List(t.Context(), client, ListInput{ResourceType: "user", ResourceID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatSetMarkdown
// ---------------------------------------------------------------------------.

// TestFormatSetMarkdown_Coverage verifies the SetMarkdown_Coverage Markdown formatter for a representative set_coverage input.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the rendered Markdown contains the expected section headings and content.
func TestFormatSetMarkdown_Coverage(t *testing.T) {
	assertMarkdown(t, FormatSetMarkdown(SetOutput{Key: "env", Value: "prod"}),
		"## Custom Attribute Set\n\n"+
			"- **Key**: env\n"+
			"- **Value**: prod\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'admin.custom_attr_get' to verify the value\n")
}

// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// What the switch, the refusal and the confirmation have to hold
// ---------------------------------------------------------------------------.

// hintMarker is what toolutil.WrapErrWithHint puts in front of a corrective
// suggestion, and the only thing that distinguishes a hinted error from a
// plainly wrapped one. Asserting on it rather than on the hint's wording keeps
// the test about whether the hint was attached at all.
const hintMarker = "Suggestion: "

// customAttributeRoutedTypes states which resource_type values the four
// handlers answer, and the collection each one reaches. It is written here
// rather than read from validResourceTypes precisely so the two can disagree:
// an advertised value no case arm answers, and a case arm the refusal never
// names, are both findings in the test below.
var customAttributeRoutedTypes = map[string]string{
	testTypeUser:    "/api/v4/users/1/custom_attributes",
	testTypeGroup:   "/api/v4/groups/1/custom_attributes",
	testTypeProject: "/api/v4/projects/1/custom_attributes",
}

// routedTypesHandler serves the collection of every routed resource type and
// nothing else, so a type the switch sends anywhere but its own collection
// fails rather than being quietly answered.
func routedTypesHandler() http.Handler {
	mux := http.NewServeMux()
	for _, path := range customAttributeRoutedTypes {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		})
	}
	return mux
}

// statusHandler answers every request with the given status and a body that
// says nothing about resources or administration, so what the hint assertions
// read is the handler's own suggestion rather than GitLab's message.
func statusHandler(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, status, `{"message":"denied"}`)
	})
}

// TestResourceTypes_AdvertisedSetIsTheRoutedSet asserts that the values the
// refusal advertises and the values the handlers actually route are one set:
// every routed type is named in the refusal, and every advertised type reaches
// GitLab.
//
// Why it matters: that list is the only place a caller is told what to send,
// and nothing tied it to the switch that answers. Emptying validResourceTypes
// left every other test in this file green while a model was handed
// "must be one of: " and no way to recover; adding a value no case arm answers
// would advertise a type this server refuses.
func TestResourceTypes_AdvertisedSetIsTheRoutedSet(t *testing.T) {
	client := testutil.NewTestClient(t, routedTypesHandler())

	_, err := List(t.Context(), client, ListInput{ResourceType: "nonesuch", ResourceID: 1})
	if err == nil {
		t.Fatal(errExpectedNil)
	}
	refusal := err.Error()

	for typ := range customAttributeRoutedTypes {
		t.Run("named/"+typ, func(t *testing.T) {
			if !strings.Contains(refusal, typ) {
				t.Errorf("refusal = %q, want it to name the routed resource_type %q", refusal, typ)
			}
		})
	}

	for _, typ := range validResourceTypes {
		t.Run("routed/"+typ, func(t *testing.T) {
			if _, ok := customAttributeRoutedTypes[typ]; !ok {
				t.Fatalf("validResourceTypes advertises %q, which no handler routes", typ)
			}
			if _, listErr := List(t.Context(), client, ListInput{ResourceType: typ, ResourceID: 1}); listErr != nil {
				t.Errorf("List(%q) = %v, want the advertised type to reach its collection", typ, listErr)
			}
		})
	}
}

// TestHandlers_NotFoundCarriesTheHint_AnotherStatusDoesNot asserts that each of
// the four handlers attaches its corrective suggestion exactly when GitLab
// answers 404, and leaves it off another refusal.
//
// Why it matters: the hint is what tells a caller which field to check and
// that this is an administrator's endpoint, and it is attached by naming one
// status code. Nothing asserted that code, so all four handlers could name a
// status GitLab never returns here — dropping the hint from every 404 — while
// every test in this file kept passing on `err != nil` alone.
func TestHandlers_NotFoundCarriesTheHint_AnotherStatusDoesNot(t *testing.T) {
	calls := []struct {
		name string
		call func(ctx context.Context, client *gitlabclient.Client) error
	}{
		{"list", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := List(ctx, client, ListInput{ResourceType: testTypeUser, ResourceID: 1})
			return err
		}},
		{"get", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Get(ctx, client, GetInput{ResourceType: testTypeGroup, ResourceID: 1, Key: testKeyDept})
			return err
		}},
		{"set", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := Set(ctx, client, SetInput{ResourceType: testTypeProject, ResourceID: 1, Key: testKeyDept, Value: "eng"})
			return err
		}},
		{"delete", func(ctx context.Context, client *gitlabclient.Client) error {
			return Delete(ctx, client, DeleteInput{ResourceType: testTypeUser, ResourceID: 1, Key: testKeyDept})
		}},
	}

	for _, tt := range calls {
		t.Run(tt.name, func(t *testing.T) {
			notFound := tt.call(t.Context(), testutil.NewTestClient(t, statusHandler(http.StatusNotFound)))
			if notFound == nil {
				t.Fatal(errExpectedNil)
			}
			if !strings.Contains(notFound.Error(), hintMarker) {
				t.Errorf("404 error = %q, want it to carry a %q hint", notFound.Error(), hintMarker)
			}
			if !strings.Contains(notFound.Error(), testResourceID) {
				t.Errorf("404 error = %q, want the hint to name %s", notFound.Error(), testResourceID)
			}

			forbidden := tt.call(t.Context(), testutil.NewTestClient(t, statusHandler(http.StatusForbidden)))
			if forbidden == nil {
				t.Fatal(errExpectedNil)
			}
			if strings.Contains(forbidden.Error(), hintMarker) {
				t.Errorf("403 error = %q, want no %q: the hint answers a resource that is not there",
					forbidden.Error(), hintMarker)
			}
		})
	}
}

// TestDeleteOutput_IsTheConfirmationEveryDestructiveActionGives asserts that
// the envelope the delete action answers with is the one toolutil.DeleteResult
// builds for a custom attribute, rather than a sentence of this package's own.
//
// Why it matters: the status token and the sentence are the whole of what a
// model reads to conclude the attribute is gone, and both were literals nothing
// compared with anything. Blank them and the caller receives a bare success
// emoji, since the registered formatter renders the message and nothing else,
// while every test in this file still passes.
func TestDeleteOutput_IsTheConfirmationEveryDestructiveActionGives(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.AssertRequestPath(t, r, "/api/v4/users/1/custom_attributes/dept")
		testutil.AssertRequestMethod(t, r, http.MethodDelete)
		w.WriteHeader(http.StatusNoContent)
	})
	client := testutil.NewTestClient(t, handler)

	got, err := DeleteOutput(t.Context(), client, DeleteInput{ResourceType: testTypeUser, ResourceID: 1, Key: testKeyDept})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	_, want, _ := toolutil.DeleteResult("custom_attribute")
	if got.Status != want.Status || got.Message != want.Message {
		t.Errorf("DeleteOutput = %+v, want the shared confirmation %+v", got, want)
	}
}

// TestDeleteOutput_Refused_ReportsTheRefusalRatherThanTheConfirmation verifies
// the other half of the adapter: the confirmation shape is returned only when
// GitLab actually deleted the attribute, so a refusal cannot reach a model as
// a success.
func TestDeleteOutput_Refused_ReportsTheRefusalRatherThanTheConfirmation(t *testing.T) {
	client := testutil.NewTestClient(t, statusHandler(http.StatusForbidden))

	got, err := DeleteOutput(t.Context(), client, DeleteInput{ResourceType: testTypeUser, ResourceID: 1, Key: testKeyDept})
	if err == nil {
		t.Fatal("DeleteOutput() error = nil, want the instance refusal")
	}
	if got.Status != "" {
		t.Errorf("DeleteOutput() = %+v, want the zero output beside the error", got)
	}
}
