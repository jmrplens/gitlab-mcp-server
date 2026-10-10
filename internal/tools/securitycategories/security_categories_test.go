// security_categories_test.go contains unit tests for GitLab security category operations.
//
// The tests mock GitLab GraphQL mutations with [testutil.GraphQLHandler], then
// call handlers directly to verify request payloads, validation, error wrapping,
// output conversion, markdown rendering, and ActionSpec metadata.
package securitycategories

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const sampleCategory = `{
	"id": "gid://gitlab/Security::Category/7",
	"name": "Business impact",
	"description": "Business impact labels",
	"multipleSelection": true,
	"editableState": "EDITABLE",
	"templateType": "APPLICATION",
	"securityAttributes": [{
		"id": "gid://gitlab/Security::Attribute/9",
		"name": "High",
		"color": "#FF0000",
		"description": "High impact",
		"editableState": "EDITABLE"
	}]
}`

// categoryGraphQLMux returns a GraphQL test handler for security category
// mutations keyed by operation name.
func categoryGraphQLMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// graphQLInput parses the GraphQL input variables from r and fails the calling
// test if the request does not contain the expected input object.
func graphQLInput(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Fatalf("ParseGraphQLVariables error: %v", err)
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Fatalf("GraphQL input = %#v, want map", vars["input"])
	}
	return input
}

// TestCreate_Success verifies that Create sends the expected GraphQL input and
// converts the returned security category and attribute IDs.
//
// The mocked securityCategoryCreate mutation asserts namespace, name,
// description, and multiple-selection fields before returning a category with a
// nested attribute. The expected output preserves parsed IDs and template type.
func TestCreate_Success(t *testing.T) {
	description := "Business impact labels"
	multipleSelection := true
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryCreate": func(w http.ResponseWriter, r *http.Request) {
			input := graphQLInput(t, r)
			if input["namespaceId"] != "gid://gitlab/Namespace/101" {
				t.Fatalf("namespaceId = %#v", input["namespaceId"])
			}
			if input["name"] != "Business impact" {
				t.Fatalf("name = %#v", input["name"])
			}
			if input["description"] != description {
				t.Fatalf("description = %#v", input["description"])
			}
			if input["multipleSelection"] != true {
				t.Fatalf("multipleSelection = %#v", input["multipleSelection"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryCreate":{"securityCategory":`+sampleCategory+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Create(context.Background(), client, CreateInput{
		NamespaceID:       101,
		Name:              " Business impact ",
		Description:       &description,
		MultipleSelection: &multipleSelection,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if out.ID != 7 || out.Name != "Business impact" || !out.MultipleSelection {
		t.Fatalf("Create() output = %#v", out)
	}
	if out.TemplateType != "APPLICATION" {
		t.Fatalf("TemplateType = %q, want APPLICATION", out.TemplateType)
	}
	if len(out.SecurityAttributes) != 1 || out.SecurityAttributes[0].ID != 9 {
		t.Fatalf("SecurityAttributes = %#v", out.SecurityAttributes)
	}
}

// distinctCategory is the fixture no two values of which agree: the category
// and its attribute differ in every field, and the two editable states differ
// too. Only such a fixture can tell a converter reading its neighbour's key
// from one reading its own: sampleCategory gives both objects the state
// "EDITABLE", so swapping them changed nothing.
const distinctCategory = `{
	"id": "gid://gitlab/Security::Category/7",
	"name": "Business impact",
	"description": "Ranks a project by the money it moves",
	"multipleSelection": true,
	"editableState": "EDITABLE_ATTRIBUTES",
	"templateType": "BUSINESS_IMPACT",
	"securityAttributes": [{
		"id": "gid://gitlab/Security::Attribute/9",
		"name": "High",
		"color": "#FF0000",
		"description": "Loss of revenue within a day",
		"editableState": "LOCKED"
	}]
}`

// TestCreate_CarriesEveryFieldGitLabSentIntoTheOutput compares the whole
// converted output against the whole fixture rather than a field at a time.
//
// Both gates score branches, so the assignments in categoryNodeOutput and
// attributeNodeSummary are invisible to them: the attribute's name and
// description could be read from each other's key, and the category's editable
// state from its name, with every test still green. A fixture in which no two
// values agree is what makes those swaps fail.
func TestCreate_CarriesEveryFieldGitLabSentIntoTheOutput(t *testing.T) {
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryCreate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryCreate":{"securityCategory":`+distinctCategory+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := Output{
		ID:                7,
		Name:              "Business impact",
		Description:       "Ranks a project by the money it moves",
		MultipleSelection: true,
		EditableState:     "EDITABLE_ATTRIBUTES",
		TemplateType:      "BUSINESS_IMPACT",
		SecurityAttributes: []AttributeSummary{{
			ID:            9,
			Name:          "High",
			Color:         "#FF0000",
			Description:   "Loss of revenue within a day",
			EditableState: "LOCKED",
		}},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Create() output = %#v, want %#v", out, want)
	}
}

// TestCreate_CategoryWithoutOptionalFields_LeavesThemEmpty verifies that a
// category GitLab sends with a null description, a null template type and no
// attributes converts to empty values instead of dereferencing a nil pointer.
//
// Both fields are nullable in the schema, and every other fixture here fills
// them, so the branch that skips a missing one was never taken.
func TestCreate_CategoryWithoutOptionalFields_LeavesThemEmpty(t *testing.T) {
	const bare = `{
		"id": "gid://gitlab/Security::Category/12",
		"name": "Exposure",
		"description": null,
		"multipleSelection": false,
		"editableState": "LOCKED",
		"templateType": null,
		"securityAttributes": null
	}`
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryCreate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryCreate":{"securityCategory":`+bare+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Exposure"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	want := Output{ID: 12, Name: "Exposure", EditableState: "LOCKED", SecurityAttributes: []AttributeSummary{}}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("Create() output = %#v, want %#v", out, want)
	}
}

// TestUpdate_DescriptionOnly_SendsNoName verifies that an update carrying only
// a description reaches GitLab without a name key.
//
// GitLab's SecurityCategoryUpdateInput takes name and description apart, so a
// handler that always sent a name would rename the category to the empty
// string on every description edit. Every other update test passes a name, so
// this is the only case in which the optional branch is not taken.
func TestUpdate_DescriptionOnly_SendsNoName(t *testing.T) {
	description := "Ranks a project by the money it moves"
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := graphQLInput(t, r)
			if _, present := input["name"]; present {
				t.Errorf("input carries name = %#v, want it absent", input["name"])
			}
			if input["description"] != description {
				t.Errorf("description = %#v, want %q", input["description"], description)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryUpdate":{"securityCategory":`+distinctCategory+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Description: &description})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.Description != description {
		t.Errorf("Update() Description = %q, want %q", out.Description, description)
	}
}

// TestHandlers_ReturnContextErrors verifies that all security category handlers
// propagate a cancelled context without masking it as a GitLab API error.
//
// The test cancels the context before invoking create, update, and delete paths.
// Each subtest expects [context.Canceled], preserving caller cancellation
// semantics across GraphQL mutations.
func TestHandlers_ReturnContextErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.NotFoundHandler())
	name := "Business impact"
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := Create(ctx, client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name: "update",
			call: func() error {
				_, err := Update(ctx, client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name: "delete",
			call: func() error {
				_, err := Delete(ctx, client, DeleteInput{CategoryID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("handler error = %v, want context.Canceled", err)
			}
		})
	}
}

// TestHandlers_WrapGitLabErrors verifies that GraphQL mutation errors embedded
// in successful HTTP responses are surfaced to callers.
//
// Each subtest returns a domain-level errors array from the corresponding
// security category mutation. The expected result is an error containing the
// GitLab message instead of a successful zero-value output.
func TestHandlers_WrapGitLabErrors(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		payload  string
		call     func(*gitlabclient.Client) error
	}{
		{
			name:     "create",
			queryKey: "securityCategoryCreate",
			payload:  `{"securityCategoryCreate":{"securityCategory":null,"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name:     "update",
			queryKey: "securityCategoryUpdate",
			payload:  `{"securityCategoryUpdate":{"securityCategory":null,"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				name := "Business impact"
				_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name:     "delete",
			queryKey: "securityCategoryDestroy",
			payload:  `{"securityCategoryDestroy":{"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				tt.queryKey: func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, tt.payload)
				},
			})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), "forbidden") {
				t.Fatalf("handler error = %v, want forbidden", err)
			}
		})
	}
}

// TestHandlers_WrapTopLevelGraphQLErrors verifies that top-level GraphQL errors
// are returned consistently across security category handlers.
//
// The mock responds with a GraphQL errors envelope for every mutation route. The
// test expects each handler to preserve the top-level error text so callers can
// distinguish transport success from GraphQL execution failure.
func TestHandlers_WrapTopLevelGraphQLErrors(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		call     func(*gitlabclient.Client) error
	}{
		{
			name:     "create",
			queryKey: "securityCategoryCreate",
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name:     "update",
			queryKey: "securityCategoryUpdate",
			call: func(client *gitlabclient.Client) error {
				name := "Business impact"
				_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name:     "delete",
			queryKey: "securityCategoryDestroy",
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				tt.queryKey: func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQLError(w, http.StatusOK, "top-level forbidden")
				},
			})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), "top-level forbidden") {
				t.Fatalf("handler error = %v, want top-level GraphQL error", err)
			}
		})
	}
}

// TestHandlers_WrapTransportErrors verifies that non-2xx HTTP responses from the
// GraphQL endpoint are wrapped with the transport status.
//
// Each subtest returns HTTP 403 for a different mutation. The expected error
// includes the status code, protecting the shared transport error path.
func TestHandlers_WrapTransportErrors(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		call     func(*gitlabclient.Client) error
	}{
		{
			name:     "create",
			queryKey: "securityCategoryCreate",
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name:     "update",
			queryKey: "securityCategoryUpdate",
			call: func(client *gitlabclient.Client) error {
				name := "Business impact"
				_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name:     "delete",
			queryKey: "securityCategoryDestroy",
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				tt.queryKey: func(w http.ResponseWriter, _ *http.Request) {
					http.Error(w, "boom", http.StatusForbidden)
				},
			})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), "403") {
				t.Fatalf("handler error = %v, want HTTP 403", err)
			}
		})
	}
}

// TestHandlers_ReturnNotFoundOnEmptyGraphQLPayload verifies that empty mutation
// payloads are treated as missing GitLab resources.
//
// The table covers null category nodes and null payloads for create, update, and
// delete. Each case expects a Not Found error instead of silently accepting a
// partial GraphQL response.
func TestHandlers_ReturnNotFoundOnEmptyGraphQLPayload(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		payload  string
		call     func(*gitlabclient.Client) error
	}{
		{
			name:     "create null category",
			queryKey: "securityCategoryCreate",
			payload:  `{"securityCategoryCreate":{"securityCategory":null,"errors":[]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name:     "create null payload",
			queryKey: "securityCategoryCreate",
			payload:  `{"securityCategoryCreate":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
				return err
			},
		},
		{
			name:     "update null category",
			queryKey: "securityCategoryUpdate",
			payload:  `{"securityCategoryUpdate":{"securityCategory":null,"errors":[]}}`,
			call: func(client *gitlabclient.Client) error {
				name := "Business impact"
				_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name:     "update null payload",
			queryKey: "securityCategoryUpdate",
			payload:  `{"securityCategoryUpdate":null}`,
			call: func(client *gitlabclient.Client) error {
				name := "Business impact"
				_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
				return err
			},
		},
		{
			name:     "delete null payload",
			queryKey: "securityCategoryDestroy",
			payload:  `{"securityCategoryDestroy":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				tt.queryKey: func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, tt.payload)
				},
			})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), "Not Found") {
				t.Fatalf("handler error = %v, want Not Found", err)
			}
		})
	}
}

// TestHandlers_ReturnErrorOnMalformedGraphQLIDs verifies that malformed GraphQL
// global IDs in security category responses fail during output conversion.
//
// The mock replaces valid category or attribute GIDs with invalid strings and
// expects parse-specific errors. This protects callers from receiving outputs
// with ambiguous numeric IDs.
func TestHandlers_ReturnErrorOnMalformedGraphQLIDs(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    string
	}{
		{
			name:    "category id",
			payload: strings.Replace(sampleCategory, "gid://gitlab/Security::Category/7", "bad-category-id", 1),
			want:    "parse security category id",
		},
		{
			name:    "attribute id",
			payload: strings.Replace(sampleCategory, "gid://gitlab/Security::Attribute/9", "bad-attribute-id", 1),
			want:    "parse security attribute id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				"securityCategoryUpdate": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryUpdate":{"securityCategory":`+tt.payload+`,"errors":[]}}`)
				},
			})
			client := testutil.NewTestClient(t, handler)
			name := "Business impact"
			_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Update() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestUpdate_Success verifies that Update sends the selected fields to GraphQL
// and converts the returned security category.
//
// The mock checks the category GID, namespace GID, name, and description before
// returning a sample category. The expected output contains the parsed category
// ID from the GraphQL node.
func TestUpdate_Success(t *testing.T) {
	name := "Application tier"
	description := "Updated description"
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := graphQLInput(t, r)
			if input["id"] != "gid://gitlab/Security::Category/7" {
				t.Fatalf("id = %#v", input["id"])
			}
			if input["namespaceId"] != "gid://gitlab/Namespace/101" {
				t.Fatalf("namespaceId = %#v", input["namespaceId"])
			}
			if input["name"] != name {
				t.Fatalf("name = %#v", input["name"])
			}
			if input["description"] != description {
				t.Fatalf("description = %#v", input["description"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryUpdate":{"securityCategory":`+sampleCategory+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name, Description: &description})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.ID != 7 {
		t.Fatalf("Update() ID = %d, want 7", out.ID)
	}
}

// TestUpdate_RequiresChanges verifies that Update rejects requests with no
// mutable fields before calling GitLab.
//
// The input contains valid IDs but no name, description, or multiple-selection
// change. The expected validation error preserves the contract that update
// actions must carry at least one mutation.
func TestUpdate_RequiresChanges(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101})
	if err == nil || !strings.Contains(err.Error(), "provide at least one") {
		t.Fatalf("Update() error = %v, want missing changes", err)
	}
}

// TestCreate_ValidatesInputBeforeRequest verifies Create validation for the
// namespace ID and required category name.
//
// Each table case uses a not-found handler as a guard and expects a local
// validation error, proving invalid category creation requests do not reach
// GraphQL.
func TestCreate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	tests := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{name: "invalid namespace ID", input: CreateInput{NamespaceID: 0, Name: "Business impact"}, want: "namespace_id must be greater than 0"},
		{name: "blank name", input: CreateInput{NamespaceID: 101, Name: " "}, want: "name is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Create(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Create() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestUpdate_ValidatesInputBeforeRequest verifies Update validation for IDs,
// required changes, and non-blank names.
//
// The table covers invalid category and namespace IDs, missing changes, and a
// blank name. Each case expects the field-specific validation message before any
// network request is attempted.
func TestUpdate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	name := " "
	tests := []struct {
		name  string
		input UpdateInput
		want  string
	}{
		{name: "invalid category ID", input: UpdateInput{CategoryID: 0, NamespaceID: 101, Name: &name}, want: "category_id must be greater than 0"},
		{name: "invalid namespace ID", input: UpdateInput{CategoryID: 7, NamespaceID: -1, Name: &name}, want: "namespace_id must be greater than 0"},
		{name: "missing changes", input: UpdateInput{CategoryID: 7, NamespaceID: 101}, want: "provide at least one"},
		{name: "blank name", input: UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name}, want: "name is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Update(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Update() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestDelete_Success verifies that Delete sends the category GID to the
// securityCategoryDestroy mutation and reports a successful deletion with the
// attributes GitLab deleted along with the category.
//
// The mock records the encoded GraphQL ID and answers with two deleted
// attribute GIDs. The test expects a success status, a message that names the
// deleted category, and both attributes by their numeric IDs.
func TestDelete_Success(t *testing.T) {
	var sentID any
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryDestroy": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil {
				t.Errorf("ParseGraphQLVariables error: %v", err)
			}
			if input, ok := vars["input"].(map[string]any); ok {
				sentID = input["id"]
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryDestroy":{
				"deletedAttributesGid":["gid://gitlab/Security::Attribute/11","gid://gitlab/Security::Attribute/12"],"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if sentID != "gid://gitlab/Security::Category/7" {
		t.Errorf("id = %#v, want the category's global ID", sentID)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "security category 7") {
		t.Errorf("Delete() output = %#v", out)
	}
	if want := []int64{11, 12}; !slices.Equal(out.DeletedAttributeIDs, want) {
		t.Errorf("DeletedAttributeIDs = %v, want %v", out.DeletedAttributeIDs, want)
	}
}

// TestDelete_NoAttributesDeleted_ReportsAnEmptyList verifies that a category
// GitLab deleted without attributes, whether it sends an empty list or null,
// is published with an empty list rather than null, so the answer says none
// went with it.
func TestDelete_NoAttributesDeleted_ReportsAnEmptyList(t *testing.T) {
	for name, deleted := range map[string]string{"empty": `[]`, "null": `null`} {
		t.Run(name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				"securityCategoryDestroy": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryDestroy":{"deletedAttributesGid":`+deleted+`,"errors":[]}}`)
				},
			})
			out, err := Delete(context.Background(), testutil.NewTestClient(t, handler), DeleteInput{CategoryID: 7})
			if err != nil {
				t.Fatalf("Delete() error = %v", err)
			}
			if out.DeletedAttributeIDs == nil || len(out.DeletedAttributeIDs) != 0 {
				t.Errorf("DeletedAttributeIDs = %#v, want an empty, non-nil list", out.DeletedAttributeIDs)
			}
		})
	}
}

// TestDelete_MalformedDeletedAttributeGID_ReturnsAParseError verifies that a
// deleted attribute GitLab names by something other than a global ID is
// reported rather than published as a zero ID.
func TestDelete_MalformedDeletedAttributeGID_ReturnsAParseError(t *testing.T) {
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryDestroy": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryDestroy":{"deletedAttributesGid":["not-a-gid"],"errors":[]}}`)
		},
	})
	_, err := Delete(context.Background(), testutil.NewTestClient(t, handler), DeleteInput{CategoryID: 7})
	if err == nil || !strings.Contains(err.Error(), "parse deleted security attribute id") {
		t.Fatalf("Delete() error = %v, want a parse error naming the deleted attribute id", err)
	}
}

// TestFormatDeleteOutputMarkdown pins the card of a deleted category, with the
// attributes deleted with it and with none.
func TestFormatDeleteOutputMarkdown(t *testing.T) {
	const hints = "\n---\n\U0001F4A1 **Next steps:**\n- Use action 'security_category.create' to define a replacement category\n"
	tests := []struct {
		name string
		ids  []int64
		want string
	}{
		{"with attributes", []int64{11, 12}, "## Security Category Deleted\n\n" +
			"- **Result**: Successfully deleted security category 7 and its attributes.\n" +
			"- **Attributes deleted with it**: 11, 12\n" + hints},
		{"without attributes", []int64{}, "## Security Category Deleted\n\n" +
			"- **Result**: Successfully deleted security category 7 and its attributes.\n" +
			"- **Attributes deleted with it**: none\n" + hints},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDeleteOutputMarkdown(DeleteOutput{
				Status:              "success",
				Message:             "Successfully deleted security category 7 and its attributes.",
				DeletedAttributeIDs: tt.ids,
			})
			if got != tt.want {
				t.Errorf("FormatDeleteOutputMarkdown()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

// TestDelete_ValidatesInputBeforeRequest verifies that Delete rejects an invalid
// category ID before calling GitLab.
//
// A not-found handler guards against accidental transport use; the expected
// result is the local validation error for category_id.
func TestDelete_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Delete(context.Background(), client, DeleteInput{CategoryID: 0})
	if err == nil || !strings.Contains(err.Error(), "category_id must be greater than 0") {
		t.Fatalf("Delete() error = %v, want invalid category ID", err)
	}
}

// TestFormatOutputMarkdown_WithAttributes_RendersTable verifies that a
// security category renders as one card whose attributes are the only table on
// the page, with every GitLab-authored value escaped.
//
// The whole render is compared rather than a set of substrings: a substring
// assertion passes on a row that landed outside the block it was meant for,
// which is the defect class this migration closes. The category and attribute
// names carry pipes, so the comparison also pins the escaping.
func TestFormatOutputMarkdown_WithAttributes_RendersTable(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:                7,
		Name:              "Business | impact",
		Description:       "Business | labels",
		MultipleSelection: true,
		EditableState:     "EDITABLE",
		TemplateType:      "CUSTOM",
		SecurityAttributes: []AttributeSummary{{
			ID:            9,
			Name:          "High | Risk",
			Color:         "#FF0000",
			Description:   "Loss of revenue",
			EditableState: "EDITABLE",
		}},
	})

	want := "## Security Category: Business | impact\n\n" +
		"- **ID**: 7\n" +
		"- **Name**: Business &#124; impact\n" +
		"- **Description**: Business &#124; labels\n" +
		"- **Multiple selection**: ✅\n" +
		"- **Editable state**: `EDITABLE`\n" +
		"- **Template type**: `CUSTOM`\n\n" +
		"### Attributes\n\n" +
		"| ID | Name | Color | Description | Editable state |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| 9 | High &#124; Risk | `#FF0000` | Loss of revenue | `EDITABLE` |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.create' to add an attribute under this category\n" +
		"- Use action 'security_category.update' to rename this category or change its description\n" +
		"- Use action 'security_attribute.project_update' to apply this category's attributes to a project\n"

	if md != want {
		t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatOutputMarkdown_EditableStateDecidesTheHints verifies that the card
// offers only the next steps the category's editable state allows: a locked
// category can neither be renamed nor gain an attribute, and one whose
// attributes alone are editable can gain an attribute but not be renamed.
// Offering a call GitLab refuses is the defect this closes.
func TestFormatOutputMarkdown_EditableStateDecidesTheHints(t *testing.T) {
	const (
		addAttribute   = "- Use action 'security_attribute.create' to add an attribute under this category\n"
		renameCategory = "- Use action 'security_category.update' to rename this category or change its description\n"
		applyToProject = "- Use action 'security_attribute.project_update' to apply this category's attributes to a project\n"
	)

	tests := []struct {
		name  string
		state string
		want  string
	}{
		{name: "locked offers neither", state: "LOCKED", want: applyToProject},
		{name: "editable attributes offers the attribute", state: "EDITABLE_ATTRIBUTES", want: addAttribute + applyToProject},
		{name: "editable offers both", state: "EDITABLE", want: addAttribute + renameCategory + applyToProject},
		{name: "unset offers both and lets GitLab refuse", state: "", want: addAttribute + renameCategory + applyToProject},
		{name: "unknown offers both and lets GitLab refuse", state: "SOMETHING_NEW", want: addAttribute + renameCategory + applyToProject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			head := "## Security Category: Impact\n\n- **ID**: 1\n- **Name**: Impact\n- **Multiple selection**: ❌\n"
			if tt.state != "" {
				head += "- **Editable state**: `" + tt.state + "`\n"
			}
			want := head + "\n---\n💡 **Next steps:**\n" + tt.want
			got := FormatOutputMarkdown(Output{ID: 1, Name: "Impact", EditableState: tt.state})
			if got != want {
				t.Errorf("FormatOutputMarkdown(%q) =\n%s\nwant:\n%s", tt.state, got, want)
			}
		})
	}
}

// TestFormatOutputMarkdown_HostileName verifies that a category name cannot
// open a heading, a list item or a link of its own: the heading escaper
// neutralizes the bracket and the tag, the row escaper neutralizes the pipe,
// and the card's structure is the same as with a benign name.
func TestFormatOutputMarkdown_HostileName(t *testing.T) {
	md := FormatOutputMarkdown(Output{
		ID:   1,
		Name: "x\n## injected\n- **State**: closed",
	})

	want := "## Security Category: x ## injected - **State**: closed\n\n" +
		"- **ID**: 1\n" +
		"- **Name**: x ## injected - **State**: closed\n" +
		"- **Multiple selection**: ❌\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.create' to add an attribute under this category\n" +
		"- Use action 'security_category.update' to rename this category or change its description\n" +
		"- Use action 'security_attribute.project_update' to apply this category's attributes to a project\n"

	if md != want {
		t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatOutputMarkdown_NoName verifies that a category GitLab sent no name
// for opens the generic heading rather than one ending in a colon.
func TestFormatOutputMarkdown_NoName(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 3})
	if !strings.HasPrefix(md, "## Security Category\n\n- **ID**: 3\n") {
		t.Errorf("FormatOutputMarkdown() =\n%s", md)
	}
}

// TestOutputHelpers_HandleNilValues_ReturnZeroValues verifies nil GraphQL nodes
// convert to zero-value category outputs without panicking.
//
// The helper should return an empty category output and no attributes, keeping
// defensive conversion behavior stable for partial GraphQL responses.
func TestOutputHelpers_HandleNilValues_ReturnZeroValues(t *testing.T) {
	if out, err := categoryNodeOutput(nil); err != nil || out.ID != 0 || len(out.SecurityAttributes) != 0 {
		t.Fatalf("categoryNodeOutput(nil) = %#v", out)
	}
}

// TestActionSpecs_Metadata_ExpectedResult verifies the canonical security
// category ActionSpecs expose expected mutation metadata and schemas.
//
// The test checks destructive flags, individual tool names, create schema
// constraints, boolean typing, and update anyOf requirements. This protects
// dynamic, meta, and individual surfaces from drifting away from the security
// category contract.
func TestActionSpecs_Metadata_ExpectedResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("ActionSpecs() len = %d, want 3", len(specs))
	}
	specByName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByName[spec.Name] = spec
	}
	deleteSpec := specByName["delete"]
	if !deleteSpec.Destructive || deleteSpec.IndividualTool.Name != "gitlab_delete_security_category" {
		t.Fatalf("delete spec = %#v", deleteSpec)
	}
	createProperties := specByName["create"].Route.InputSchema["properties"].(map[string]any)
	if createProperties["name"].(map[string]any)["minLength"] != 1 {
		t.Fatalf("create name schema = %#v", createProperties["name"])
	}
	if createProperties["multiple_selection"].(map[string]any)["type"] != "boolean" {
		t.Fatalf("create multiple_selection schema = %#v", createProperties["multiple_selection"])
	}
	updateSchema := specByName["update"].Route.InputSchema
	if anyOf, ok := updateSchema["anyOf"].([]any); !ok || len(anyOf) != 2 {
		t.Fatalf("update anyOf = %#v, want name/description requirement", updateSchema["anyOf"])
	}
}

// declaredActionIDs is the block markdown.go holds, repeated here so a test can
// ask whether anything spells an ID outside it.
var declaredActionIDs = []string{
	actionCategoryCreate, actionCategoryUpdate, actionCategoryDelete,
	actionAttributeCreate, actionAttributeUpdate, actionAttributeDelete,
	actionAttributeProjectUpdate, actionGroupGet, actionProjectGet,
}

// hintActionID returns the canonical action ID [toolutil.HintAction] quoted
// inside hint, or the empty string when the hint names none, which the caller
// reports as an undeclared ID rather than asserting here.
func hintActionID(hint string) string {
	_, rest, ok := strings.Cut(hint, "'")
	if !ok {
		return ""
	}
	id, _, ok := strings.Cut(rest, "'")
	if !ok {
		return ""
	}
	return id
}

// TestActionIDs_MatchTheActionsThisPackageRegisters verifies that the category
// constants are the canonical IDs of the specs this package actually declares.
//
// Nothing in the repository checks that an action ID names an action the
// catalog holds, so a renamed action would leave the constants pointing at an
// ID no surface resolves and every gate would still pass.
func TestActionIDs_MatchTheActionsThisPackageRegisters(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	byName := map[string]string{
		"create": actionCategoryCreate,
		"update": actionCategoryUpdate,
		"delete": actionCategoryDelete,
	}

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			declared, ok := byName[spec.Name]
			if !ok {
				t.Fatalf("spec %q has no declared action ID constant", spec.Name)
			}
			if want := "security_category." + spec.Name; declared != want {
				t.Errorf("constant = %q, want %q", declared, want)
			}
		})
	}
}

// TestActionIDs_RelatedActionsAndHintsSpellOnlyDeclaredIDs verifies that every
// ID the ActionSpec metadata and the card's hints name comes from the one
// declared block.
//
// The two files used to keep their own copies of "security_category.update"
// and "security_attribute.create", which is how a hint and a related action
// can disagree about the same call; this is what keeps them one block.
func TestActionIDs_RelatedActionsAndHintsSpellOnlyDeclaredIDs(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	type use struct{ where, id string }
	var uses []use

	for _, spec := range ActionSpecs(client) {
		for _, id := range spec.RelatedActions {
			uses = append(uses, use{where: "related action of " + spec.Name, id: id})
		}
	}
	for _, state := range []string{"unset", editableStateLocked, editableStateEditableAttributes, "EDITABLE"} {
		hintState := state
		if state == "unset" {
			hintState = ""
		}
		for _, hint := range categoryHints(hintState) {
			uses = append(uses, use{where: "hint for " + state, id: hintActionID(hint)})
		}
	}
	if len(uses) == 0 {
		t.Fatal("no action IDs collected, the test asserts nothing")
	}

	for _, u := range uses {
		t.Run(u.where+" "+u.id, func(t *testing.T) {
			if !slices.Contains(declaredActionIDs, u.id) {
				t.Errorf("action ID %q is spelled outside the declared block", u.id)
			}
		})
	}
}

// fineGrainedClient is a test client carrying the authority a fine-grained
// session's client carries, so a handler reads it the way production does.
func fineGrainedClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()
	client := testutil.NewTestClient(t, handler)
	client.SetAuthority(finegrained.Unevaluated(&finegrained.Table{Version: "19.4.1-ee"}, finegrained.FallbackNone, ""))
	return client
}

// categoryWrite is one of the two writes that answer with a category.
type categoryWrite struct {
	name string
	key  string
	op   string
	call func(*gitlabclient.Client) error
}

// categoryWrites are the create and the update, each with the payload key
// GitLab answers it under and the operation its errors name.
func categoryWrites() []categoryWrite {
	return []categoryWrite{
		{name: "create", key: "securityCategoryCreate", op: "create security category", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, Name: "Business impact"})
			return err
		}},
		{name: "update", key: "securityCategoryUpdate", op: "update security category", call: func(client *gitlabclient.Client) error {
			name := "Business impact"
			_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101, Name: &name})
			return err
		}},
	}
}

// TestWrites_CategoryAnsweredWithout_FineGrainedSessionIsToldTheWriteProbablyCommitted
// verifies the answer to a category write GitLab ran and answered without
// the category: null with no error, which is how GitLab answers a
// fine-grained token the SecurityCategory type does not admit (at 19.4.1 it
// declares no fine-grained permission), and the category nulled below by its
// attributes, a list of non-null items whose type does not admit the token.
// GitLab checks those objects only after the write ran, so a fine-grained
// session is told the write was probably committed instead of not found; a
// classic session keeps the answer it had and is never told so (issue 1103).
func TestWrites_CategoryAnsweredWithout_FineGrainedSessionIsToldTheWriteProbablyCommitted(t *testing.T) {
	const nulledAttributes = `Cannot return null for non-nullable element of type 'SecurityAttribute' for SecurityCategory.securityAttributes`
	answers := []struct {
		name    string
		body    func(key string) string
		classic string
	}{
		{
			name:    "the category null",
			body:    func(key string) string { return `{"data":{"` + key + `":{"securityCategory":null,"errors":[]}}}` },
			classic: "Not Found",
		},
		{
			name: "its attributes nulled",
			body: func(key string) string {
				return `{"data":{"` + key + `":{"securityCategory":{"id":"gid://gitlab/Security::Category/7","name":"Business impact",` +
					`"description":null,"multipleSelection":true,"editableState":"EDITABLE","templateType":null,"securityAttributes":null},` +
					`"errors":[]}},"errors":[{"message":"` + nulledAttributes + `"}]}`
			},
			classic: nulledAttributes,
		},
	}
	for _, write := range categoryWrites() {
		for _, answer := range answers {
			t.Run(write.name+" "+answer.name, func(t *testing.T) {
				handler := categoryGraphQLMux(map[string]http.HandlerFunc{
					write.key: func(w http.ResponseWriter, _ *http.Request) {
						testutil.RespondJSON(w, http.StatusOK, answer.body(write.key))
					},
				})

				fine := write.call(fineGrainedClient(t, handler))
				if !errors.Is(fine, finegrained.ErrUnconfirmedWrite) {
					t.Fatalf("fine-grained error = %v, want an unconfirmed write", fine)
				}
				if want := write.op + ": GitLab answered without the security category this write returns."; !strings.HasPrefix(fine.Error(), want) {
					t.Errorf("fine-grained error = %q, want it to open with %q", fine.Error(), want)
				}

				classic := write.call(testutil.NewTestClient(t, handler))
				if errors.Is(classic, finegrained.ErrUnconfirmedWrite) || !strings.Contains(classic.Error(), answer.classic) {
					t.Errorf("classic error = %v, want the answer it always had, containing %q", classic, answer.classic)
				}
			})
		}
	}
}

// TestWrites_FineGrainedSession_RefusalIsNoUnconfirmedWrite verifies that a
// category write GitLab refused, with an error of its own and no payload, is
// the refusal to read rather than a write it ran.
func TestWrites_FineGrainedSession_RefusalIsNoUnconfirmedWrite(t *testing.T) {
	for _, write := range categoryWrites() {
		t.Run(write.name, func(t *testing.T) {
			handler := categoryGraphQLMux(map[string]http.HandlerFunc{
				write.key: func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondJSON(w, http.StatusOK, `{"data":{"`+write.key+`":null},"errors":[{"message":"Access denied: This operation requires a fine-grained personal access token with the following group permissions: [Security Category: Create]."}]}`)
				},
			})
			err := write.call(fineGrainedClient(t, handler))
			if err == nil || errors.Is(err, finegrained.ErrUnconfirmedWrite) || !strings.Contains(err.Error(), "Access denied") {
				t.Errorf("error = %v, want the refusal and no unconfirmed write", err)
			}
		})
	}
}
