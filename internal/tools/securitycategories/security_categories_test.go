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
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
// securityCategoryDestroy mutation and reports a successful deletion.
//
// The mock asserts the encoded GraphQL ID and returns an empty errors array. The
// test expects a success status and a message that names the deleted category.
func TestDelete_Success(t *testing.T) {
	handler := categoryGraphQLMux(map[string]http.HandlerFunc{
		"securityCategoryDestroy": func(w http.ResponseWriter, r *http.Request) {
			input := graphQLInput(t, r)
			if input["id"] != "gid://gitlab/Security::Category/7" {
				t.Fatalf("id = %#v", input["id"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityCategoryDestroy":{"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Delete(context.Background(), client, DeleteInput{CategoryID: 7})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "security category 7") {
		t.Fatalf("Delete() output = %#v", out)
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

// TestFormatOutputMarkdown_WithAttributes_RendersTable verifies security
// category markdown escapes table cells and includes nested attributes.
//
// The rendered output contains pipe characters in category and attribute names.
// The test expects escaped table cells plus multiple-selection, template type,
// and attribute rows so pkgsite-visible examples remain stable.
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
			EditableState: "EDITABLE",
		}},
	})
	for _, want := range []string{"Business &#124; impact", "Business &#124; labels", "| Multiple selection | true |", "| Template type | `CUSTOM` |", "High &#124; Risk"} {
		if !strings.Contains(md, want) {
			t.Fatalf("FormatOutputMarkdown() missing %q:\n%s", want, md)
		}
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
