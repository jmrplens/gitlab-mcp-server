// security_attributes_test.go contains unit tests for GitLab security attribute operations.
//
// The tests mock GitLab GraphQL mutations with [testutil.GraphQLHandler], then
// call handlers directly to verify request payloads, validation, error wrapping,
// output conversion, markdown rendering, and ActionSpec metadata.
package securityattributes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const sampleAttribute = `{
	"id": "gid://gitlab/Security::Attribute/9",
	"name": "High",
	"color": "#FF0000",
	"description": "High impact",
	"editableState": "EDITABLE",
	"securityCategory": {
		"id": "gid://gitlab/Security::Category/7",
		"name": "Business impact",
		"description": "Business impact labels",
		"multipleSelection": true,
		"editableState": "EDITABLE",
		"templateType": "APPLICATION"
	}
}`

// sampleAttributeBareCategory is the same attribute under a category GitLab
// created from no template: the schema declares description and templateType
// nullable, and a category with neither answers null for both.
const sampleAttributeBareCategory = `{
	"id": "gid://gitlab/Security::Attribute/9",
	"name": "High",
	"color": "#FF0000",
	"description": "High impact",
	"editableState": "EDITABLE",
	"securityCategory": {
		"id": "gid://gitlab/Security::Category/7",
		"name": "Business impact",
		"description": null,
		"multipleSelection": false,
		"editableState": "LOCKED",
		"templateType": null
	}
}`

// attributeGraphQLMux returns a GraphQL test handler for security attribute
// mutations keyed by operation name.
func attributeGraphQLMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// attributeGraphQLInput parses the GraphQL input variables from r and reports a
// request that carries no input object. It reports rather than aborts because
// every caller runs it inside an httptest handler, on the server's own
// goroutine: a FailNow there kills that goroutine and leaves the call under
// test waiting on a response nobody will write. A nil map reads as a request
// carrying nothing, so the assertions after it report too.
func attributeGraphQLInput(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	vars, err := testutil.ParseGraphQLVariables(r)
	if err != nil {
		t.Errorf("ParseGraphQLVariables error: %v", err)
		return nil
	}
	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Errorf("GraphQL input = %#v, want map", vars["input"])
	}
	return input
}

// TestCreate_Success verifies that Create sends the expected GraphQL input and
// converts the returned security attribute and category IDs.
//
// The mocked securityAttributeCreate mutation asserts that namespace, category,
// and attribute fields are normalized before request dispatch, then returns a
// populated attribute node. The test expects the output to preserve the parsed
// attribute ID, category ID, and template type.
func TestCreate_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeCreate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["namespaceId"] != "gid://gitlab/Namespace/101" {
				t.Errorf("namespaceId = %#v", input["namespaceId"])
			}
			if input["categoryId"] != "gid://gitlab/Security::Category/7" {
				t.Errorf("categoryId = %#v", input["categoryId"])
			}
			if attributes, listed := input["attributes"].([]any); !listed || len(attributes) != 1 {
				t.Errorf("attributes = %#v", input["attributes"])
			} else if attribute, object := attributes[0].(map[string]any); !object {
				t.Errorf("attribute = %#v", attributes[0])
			} else if attribute["name"] != "High" || attribute["description"] != "High impact" || attribute["color"] != "#FF0000" {
				t.Errorf("attribute input = %#v", attribute)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeCreate":{"securityAttributes":[`+sampleAttribute+`],"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Create(context.Background(), client, CreateInput{
		NamespaceID: 101,
		CategoryID:  7,
		Attributes:  []AttributeInput{{Name: " High ", Description: "High impact", Color: "#FF0000"}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(out.Attributes) != 1 || out.Attributes[0].ID != 9 || out.Attributes[0].SecurityCategory.ID != 7 {
		t.Fatalf("Create() output = %#v", out)
	}
	if out.Attributes[0].SecurityCategory.TemplateType != "APPLICATION" {
		t.Fatalf("TemplateType = %q, want APPLICATION", out.Attributes[0].SecurityCategory.TemplateType)
	}
}

// TestCreate_RequiresAttributes verifies that Create rejects an empty attribute
// list before it reaches the GraphQL transport.
//
// The test uses a not-found handler as a guard: a successful validation failure
// returns an "attributes is required" error without issuing any HTTP request.
func TestCreate_RequiresAttributes(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7})
	if err == nil || !strings.Contains(err.Error(), "attributes is required") {
		t.Fatalf("Create() error = %v, want attributes required", err)
	}
}

// TestCreate_ValidatesInputBeforeRequest verifies Create validation for IDs and
// required attribute fields.
//
// Each table case uses an invalid input shape and expects the specific
// validation message for that field. The not-found handler ensures the test
// fails if validation accidentally allows a network call.
func TestCreate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	tests := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{
			name:  "invalid namespace ID",
			input: CreateInput{NamespaceID: 0, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}},
			want:  "namespace_id must be greater than 0",
		},
		{
			name:  "invalid category ID",
			input: CreateInput{NamespaceID: 101, CategoryID: -1, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}},
			want:  "category_id must be greater than 0",
		},
		{
			name:  "blank name",
			input: CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: " ", Description: "High impact", Color: "#FF0000"}}},
			want:  "attributes[0].name is required",
		},
		{
			name:  "blank description",
			input: CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: " ", Color: "#FF0000"}}},
			want:  "attributes[0].description is required",
		},
		{
			name:  "blank color",
			input: CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: " "}}},
			want:  "attributes[0].color is required",
		},
		{
			name:  "invalid color",
			input: CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "red"}}},
			want:  "attributes[0].color must be a hex color",
		},
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

// TestHandlers_ReturnContextErrors verifies that all security attribute handlers
// propagate a cancelled context without masking it as a GitLab API error.
//
// The test cancels the context before invoking create, update, delete, project
// update, and bulk update paths. Each subtest expects [context.Canceled], which
// preserves caller cancellation semantics.
func TestHandlers_ReturnContextErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := testutil.NewTestClient(t, http.NotFoundHandler())
	name := "High"
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "create",
			call: func() error {
				_, err := Create(ctx, client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
				return err
			},
		},
		{
			name: "update",
			call: func() error {
				_, err := Update(ctx, client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
		},
		{
			name: "delete",
			call: func() error {
				_, err := Delete(ctx, client, DeleteInput{AttributeID: 9})
				return err
			},
		},
		{
			name: "project update",
			call: func() error {
				_, err := ProjectUpdate(ctx, client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}})
				return err
			},
		},
		{
			name: "bulk update",
			call: func() error {
				_, err := BulkUpdate(ctx, client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd})
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
// security attribute mutation. The expected result is an error containing the
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
			queryKey: "securityAttributeCreate",
			payload:  `{"securityAttributeCreate":{"securityAttributes":[],"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
				return err
			},
		},
		{
			name:     "update",
			queryKey: "securityAttributeUpdate",
			payload:  `{"securityAttributeUpdate":{"securityAttribute":null,"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				name := "High"
				_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
		},
		{
			name:     "delete",
			queryKey: "securityAttributeDestroy",
			payload:  `{"securityAttributeDestroy":{"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{AttributeID: 9})
				return err
			},
		},
		{
			name:     "project update",
			queryKey: "securityAttributeProjectUpdate",
			payload:  `{"securityAttributeProjectUpdate":{"addedCount":0,"removedCount":0,"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}})
				return err
			},
		},
		{
			name:     "bulk update",
			queryKey: "bulkUpdateSecurityAttributes",
			payload:  `{"bulkUpdateSecurityAttributes":{"errors":["forbidden"]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := BulkUpdate(context.Background(), client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := attributeGraphQLMux(map[string]http.HandlerFunc{
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
// are returned consistently across security attribute handlers.
//
// The mock responds with a GraphQL errors envelope for every mutation route. The
// test expects each handler to preserve the top-level error text so callers can
// distinguish transport success from GraphQL execution failure.
func TestHandlers_WrapTopLevelGraphQLErrors(t *testing.T) {
	runSecurityAttributeMutationErrorCases(t, "top-level forbidden", "top-level GraphQL error", func(w http.ResponseWriter) {
		testutil.RespondGraphQLError(w, http.StatusOK, "top-level forbidden")
	})
}

type mutationErrorCase struct {
	name     string
	queryKey string
	call     func(*gitlabclient.Client) error
}

func securityAttributeMutationErrorCases() []mutationErrorCase {
	return []mutationErrorCase{
		{name: "create", queryKey: "securityAttributeCreate", call: func(client *gitlabclient.Client) error {
			_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
			return err
		}},
		{name: "update", queryKey: "securityAttributeUpdate", call: func(client *gitlabclient.Client) error {
			name := "High"
			_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
			return err
		}},
		{name: "delete", queryKey: "securityAttributeDestroy", call: func(client *gitlabclient.Client) error {
			_, err := Delete(context.Background(), client, DeleteInput{AttributeID: 9})
			return err
		}},
		{name: "project update", queryKey: "securityAttributeProjectUpdate", call: func(client *gitlabclient.Client) error {
			_, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}})
			return err
		}},
		{name: "bulk update", queryKey: "bulkUpdateSecurityAttributes", call: func(client *gitlabclient.Client) error {
			_, err := BulkUpdate(context.Background(), client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd})
			return err
		}},
	}
}

func runSecurityAttributeMutationErrorCases(t *testing.T, wantSubstring, wantLabel string, respond func(http.ResponseWriter)) {
	t.Helper()
	for _, tt := range securityAttributeMutationErrorCases() {
		t.Run(tt.name, func(t *testing.T) {
			handler := attributeGraphQLMux(map[string]http.HandlerFunc{tt.queryKey: func(w http.ResponseWriter, _ *http.Request) { respond(w) }})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), wantSubstring) {
				t.Fatalf("handler error = %v, want %s", err, wantLabel)
			}
		})
	}
}

// TestHandlers_WrapTransportErrors verifies that non-2xx HTTP responses from the
// GraphQL endpoint are wrapped with the transport status.
//
// Each subtest returns HTTP 403 for a different mutation. The expected error
// includes the status code, protecting the shared error path used by all
// security attribute operations.
func TestHandlers_WrapTransportErrors(t *testing.T) {
	runSecurityAttributeMutationErrorCases(t, "403", "HTTP 403", func(w http.ResponseWriter) {
		http.Error(w, "boom", http.StatusForbidden)
	})
}

// TestHandlers_ReturnNotFoundOnEmptyGraphQLPayload verifies that empty mutation
// payloads are treated as missing GitLab resources.
//
// The table covers null payloads and empty node results for create, update,
// delete, project update, and bulk update. Each case expects a Not Found error
// instead of silently accepting a partial GraphQL response.
func TestHandlers_ReturnNotFoundOnEmptyGraphQLPayload(t *testing.T) {
	tests := []struct {
		name     string
		queryKey string
		payload  string
		call     func(*gitlabclient.Client) error
	}{
		{
			name:     "create empty attributes",
			queryKey: "securityAttributeCreate",
			payload:  `{"securityAttributeCreate":{"securityAttributes":[],"errors":[]}}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
				return err
			},
		},
		{
			name:     "create null payload",
			queryKey: "securityAttributeCreate",
			payload:  `{"securityAttributeCreate":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
				return err
			},
		},
		{
			name:     "update null attribute",
			queryKey: "securityAttributeUpdate",
			payload:  `{"securityAttributeUpdate":{"securityAttribute":null,"errors":[]}}`,
			call: func(client *gitlabclient.Client) error {
				name := "High"
				_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
		},
		{
			name:     "update null payload",
			queryKey: "securityAttributeUpdate",
			payload:  `{"securityAttributeUpdate":null}`,
			call: func(client *gitlabclient.Client) error {
				name := "High"
				_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
		},
		{
			name:     "delete null payload",
			queryKey: "securityAttributeDestroy",
			payload:  `{"securityAttributeDestroy":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := Delete(context.Background(), client, DeleteInput{AttributeID: 9})
				return err
			},
		},
		{
			name:     "project update null payload",
			queryKey: "securityAttributeProjectUpdate",
			payload:  `{"securityAttributeProjectUpdate":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}})
				return err
			},
		},
		{
			name:     "bulk update null payload",
			queryKey: "bulkUpdateSecurityAttributes",
			payload:  `{"bulkUpdateSecurityAttributes":null}`,
			call: func(client *gitlabclient.Client) error {
				_, err := BulkUpdate(context.Background(), client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := attributeGraphQLMux(map[string]http.HandlerFunc{
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
// global IDs in security attribute responses fail during output conversion.
//
// The mock replaces valid category or attribute GIDs with invalid strings and
// expects parse-specific errors. This protects callers from receiving outputs
// with ambiguous numeric IDs.
func TestHandlers_ReturnErrorOnMalformedGraphQLIDs(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		call    func(*gitlabclient.Client) error
		want    string
	}{
		{
			name:    "attribute id",
			payload: strings.Replace(sampleAttribute, "gid://gitlab/Security::Attribute/9", "bad-attribute-id", 1),
			call: func(client *gitlabclient.Client) error {
				name := "High"
				_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
			want: "parse security attribute id",
		},
		{
			name:    "category id",
			payload: strings.Replace(sampleAttribute, "gid://gitlab/Security::Category/7", "bad-category-id", 1),
			call: func(client *gitlabclient.Client) error {
				name := "High"
				_, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name})
				return err
			},
			want: "parse security category id",
		},
		{
			name:    "create attribute id",
			payload: strings.Replace(sampleAttribute, "gid://gitlab/Security::Attribute/9", "bad-attribute-id", 1),
			call: func(client *gitlabclient.Client) error {
				_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7, Attributes: []AttributeInput{{Name: "High", Description: "High impact", Color: "#FF0000"}}})
				return err
			},
			want: "parse security attribute id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := attributeGraphQLMux(map[string]http.HandlerFunc{
				"securityAttributeCreate": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeCreate":{"securityAttributes":[`+tt.payload+`],"errors":[]}}`)
				},
				"securityAttributeUpdate": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+tt.payload+`,"errors":[]}}`)
				},
			})
			client := testutil.NewTestClient(t, handler)
			err := tt.call(client)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("handler error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestUpdate_Success verifies that Update sends the selected fields to GraphQL
// and converts the returned security attribute.
//
// The mock checks the attribute GID, name, description, and color values before
// returning a sample attribute node. The expected output contains the parsed
// attribute ID and a non-nil category summary.
func TestUpdate_Success(t *testing.T) {
	name := "Critical"
	description := "Critical impact"
	color := "#990000"
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["id"] != "gid://gitlab/Security::Attribute/9" {
				t.Errorf("id = %#v", input["id"])
			}
			if input["name"] != name || input["description"] != description || input["color"] != color {
				t.Errorf("input = %#v", input)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+sampleAttribute+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name, Description: &description, Color: &color})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.ID != 9 || out.SecurityCategory == nil {
		t.Fatalf("Update() output = %#v", out)
	}
}

// TestUpdate_ResponseFields_LandOnTheFieldThatNamesThem verifies that every
// value the attribute GitLab answers with reaches the output field of the same
// name, the nested category included. Until this existed the suite asserted
// the two parsed IDs and nothing else, so the converter could read a
// neighbour's key (the name from the color, the category's name from its
// editable state) and stay green on every run.
func TestUpdate_ResponseFields_LandOnTheFieldThatNamesThem(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeUpdate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+sampleAttribute+`,"errors":[]}}`)
		},
	})

	name := "Critical"
	out, err := Update(context.Background(), testutil.NewTestClient(t, handler), UpdateInput{AttributeID: 9, Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.ID != 9 || out.Name != "High" || out.Color != "#FF0000" || out.Description != "High impact" || out.EditableState != "EDITABLE" {
		t.Fatalf("Update() attribute = %#v", out)
	}
	if out.SecurityCategory == nil {
		t.Fatalf("Update() category = nil, want the category the response carries")
	}
	want := CategorySummary{
		ID:                7,
		Name:              "Business impact",
		Description:       "Business impact labels",
		MultipleSelection: true,
		EditableState:     "EDITABLE",
		TemplateType:      "APPLICATION",
	}
	if *out.SecurityCategory != want {
		t.Fatalf("Update() category = %#v, want %#v", *out.SecurityCategory, want)
	}
}

// TestUpdate_NullableCategoryFields_ReadAsEmptyStrings verifies that the two
// nullable fields of a security category convert to the empty string rather
// than to a dereference of nil. The schema declares description and
// templateType nullable, so a category GitLab created from no template really
// does answer null for both.
func TestUpdate_NullableCategoryFields_ReadAsEmptyStrings(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeUpdate": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+sampleAttributeBareCategory+`,"errors":[]}}`)
		},
	})

	name := "Critical"
	out, err := Update(context.Background(), testutil.NewTestClient(t, handler), UpdateInput{AttributeID: 9, Name: &name})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.SecurityCategory == nil {
		t.Fatalf("Update() category = nil, want the category the response carries")
	}
	want := CategorySummary{ID: 7, Name: "Business impact", MultipleSelection: false, EditableState: "LOCKED"}
	if *out.SecurityCategory != want {
		t.Fatalf("Update() category = %#v, want %#v", *out.SecurityCategory, want)
	}
}

// TestUpdate_WithoutAName_SendsOnlyTheFieldsSupplied verifies that a change
// naming no name leaves name out of the mutation input instead of sending it
// empty, and that the description it does send is normalized. GitLab reads a
// supplied field as the new value, so an empty name in the input is a rename
// to nothing rather than a field the caller left alone; and a model that
// writes a two-line description writes the line break as the two characters
// it typed, which normalization turns into one.
func TestUpdate_WithoutAName_SendsOnlyTheFieldsSupplied(t *testing.T) {
	description := `Critical impact\nSecond line`
	color := "#990000"
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if value, ok := input["name"]; ok {
				t.Errorf("name = %#v, want the field left out entirely", value)
			}
			if input["description"] != "Critical impact\nSecond line" {
				t.Errorf("description = %#v, want the escape turned into a real newline", input["description"])
			}
			if input["color"] != color {
				t.Errorf("color = %#v, want %q", input["color"], color)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+sampleAttribute+`,"errors":[]}}`)
		},
	})

	if _, err := Update(context.Background(), testutil.NewTestClient(t, handler), UpdateInput{AttributeID: 9, Description: &description, Color: &color}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

// TestUpdate_ValidatesInputBeforeRequest verifies Update validation for the
// attribute ID, required change set, name, and color format.
//
// The table uses invalid inputs against a not-found handler and expects the
// specific validation message for each failure, ensuring bad requests are
// rejected locally before GraphQL execution.
func TestUpdate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	name := " "
	color := "123456"
	tests := []struct {
		name  string
		input UpdateInput
		want  string
	}{
		{name: "invalid attribute ID", input: UpdateInput{AttributeID: 0, Color: &color}, want: "attribute_id must be greater than 0"},
		{name: "missing changes", input: UpdateInput{AttributeID: 9}, want: "provide at least one"},
		{name: "blank name", input: UpdateInput{AttributeID: 9, Name: &name}, want: "name is required"},
		{name: "invalid color", input: UpdateInput{AttributeID: 9, Color: &color}, want: "color must be a hex color"},
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

// TestDelete_Success verifies that Delete sends the attribute GID to the
// securityAttributeDestroy mutation and reports a successful deletion.
//
// The mock asserts the encoded GraphQL ID and returns an empty errors array. The
// test expects a success status and a message that names the deleted attribute.
func TestDelete_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeDestroy": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["id"] != "gid://gitlab/Security::Attribute/9" {
				t.Errorf("id = %#v", input["id"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeDestroy":{"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Delete(context.Background(), client, DeleteInput{AttributeID: 9})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "security attribute 9") {
		t.Fatalf("Delete() output = %#v", out)
	}
}

// TestDelete_ValidatesInputBeforeRequest verifies that Delete rejects an invalid
// attribute ID before calling GitLab.
//
// A not-found handler guards against accidental transport use; the expected
// result is the local validation error for attribute_id.
func TestDelete_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Delete(context.Background(), client, DeleteInput{AttributeID: 0})
	if err == nil || !strings.Contains(err.Error(), "attribute_id must be greater than 0") {
		t.Fatalf("Delete() error = %v, want invalid attribute ID", err)
	}
}

// TestProjectUpdate_Success verifies that ProjectUpdate encodes project and
// attribute IDs as GraphQL global IDs.
//
// The mocked mutation checks the projectId, addAttributeIds, and
// removeAttributeIds fields, then returns added and removed counts. The two
// counts differ on purpose: while both read 1 the handler could report the
// removals as additions and the reverse, and nothing here could tell.
func TestProjectUpdate_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeProjectUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["projectId"] != "gid://gitlab/Project/42" {
				t.Errorf("projectId = %#v", input["projectId"])
			}
			if ids, ok := input["addAttributeIds"].([]any); !ok || len(ids) != 1 || ids[0] != "gid://gitlab/Security::Attribute/9" {
				t.Errorf("addAttributeIds = %#v", input["addAttributeIds"])
			}
			if ids, ok := input["removeAttributeIds"].([]any); !ok || len(ids) != 1 || ids[0] != "gid://gitlab/Security::Attribute/10" {
				t.Errorf("removeAttributeIds = %#v", input["removeAttributeIds"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeProjectUpdate":{"addedCount":2,"removedCount":1,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}, RemoveAttributeIDs: []int64{10}})
	if err != nil {
		t.Fatalf("ProjectUpdate() error = %v", err)
	}
	if out.AddedCount != 2 || out.RemovedCount != 1 {
		t.Fatalf("ProjectUpdate() output = %#v, want added 2 and removed 1", out)
	}
}

// TestProjectUpdate_OneDirection_LeavesTheOtherFieldOutOfTheRequest verifies
// that an add-only or remove-only request carries only the field it asked for.
// The guard that decides this reads len() > 0, and at zero "send an empty
// list" and "send nothing" are different requests: an empty
// removeAttributeIds tells GitLab a removal set was supplied, and the suite
// drove both directions together so neither side was ever observed alone.
func TestProjectUpdate_OneDirection_LeavesTheOtherFieldOutOfTheRequest(t *testing.T) {
	tests := []struct {
		name    string
		input   ProjectUpdateInput
		present string
		absent  string
		wantGID string
	}{
		{
			name:    "add only",
			input:   ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}},
			present: "addAttributeIds",
			absent:  "removeAttributeIds",
			wantGID: "gid://gitlab/Security::Attribute/9",
		},
		{
			name:    "remove only",
			input:   ProjectUpdateInput{ProjectID: 42, RemoveAttributeIDs: []int64{10}},
			present: "removeAttributeIds",
			absent:  "addAttributeIds",
			wantGID: "gid://gitlab/Security::Attribute/10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := attributeGraphQLMux(map[string]http.HandlerFunc{
				"securityAttributeProjectUpdate": func(w http.ResponseWriter, r *http.Request) {
					input := attributeGraphQLInput(t, r)
					if ids, ok := input[tt.present].([]any); !ok || len(ids) != 1 || ids[0] != tt.wantGID {
						t.Errorf("%s = %#v, want [%q]", tt.present, input[tt.present], tt.wantGID)
					}
					if value, ok := input[tt.absent]; ok {
						t.Errorf("%s = %#v, want the field left out entirely", tt.absent, value)
					}
					testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeProjectUpdate":{"addedCount":2,"removedCount":1,"errors":[]}}`)
				},
			})

			if _, err := ProjectUpdate(context.Background(), testutil.NewTestClient(t, handler), tt.input); err != nil {
				t.Fatalf("ProjectUpdate() error = %v", err)
			}
		})
	}
}

// TestProjectUpdate_ValidatesInputBeforeRequest verifies ProjectUpdate rejects
// invalid project IDs, empty operations, and invalid attribute IDs locally.
//
// Each table case expects a field-specific validation message and uses a
// not-found handler to prove the request never reaches the GraphQL transport.
func TestProjectUpdate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	tests := []struct {
		name  string
		input ProjectUpdateInput
		want  string
	}{
		{name: "invalid project ID", input: ProjectUpdateInput{ProjectID: 0, AddAttributeIDs: []int64{9}}, want: "project_id must be greater than 0"},
		{name: "missing operations", input: ProjectUpdateInput{ProjectID: 42}, want: "provide add_attribute_ids or remove_attribute_ids"},
		{name: "invalid add attribute ID", input: ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{0}}, want: "add_attribute_ids values must be greater than 0"},
		{name: "invalid remove attribute ID", input: ProjectUpdateInput{ProjectID: 42, RemoveAttributeIDs: []int64{-1}}, want: "remove_attribute_ids values must be greater than 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ProjectUpdate(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("ProjectUpdate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestBulkUpdate_Success verifies that BulkUpdate builds the combined item list
// and requested mutation mode for groups and projects.
//
// The mock checks that group and project targets are encoded in order, attribute
// IDs are encoded as security attribute GIDs, and REPLACE is forwarded as the
// mutation mode. The output should report success with the same mode.
func TestBulkUpdate_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"bulkUpdateSecurityAttributes": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if items, ok := input["items"].([]any); !ok || len(items) != 2 || items[0] != "gid://gitlab/Group/5" || items[1] != "gid://gitlab/Project/42" {
				t.Errorf("items = %#v", input["items"])
			}
			if attributes, ok := input["attributes"].([]any); !ok || len(attributes) != 1 || attributes[0] != "gid://gitlab/Security::Attribute/9" {
				t.Errorf("attributes = %#v", input["attributes"])
			}
			if input["mode"] != "REPLACE" {
				t.Errorf("mode = %#v", input["mode"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"bulkUpdateSecurityAttributes":{"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := BulkUpdate(context.Background(), client, BulkUpdateInput{
		GroupIDs:     []int64{5},
		ProjectIDs:   []int64{42},
		AttributeIDs: []int64{9},
		Mode:         BulkUpdateModeReplace,
	})
	if err != nil {
		t.Fatalf("BulkUpdate() error = %v", err)
	}
	if out.Status != "success" || out.Mode != BulkUpdateModeReplace {
		t.Fatalf("BulkUpdate() output = %#v", out)
	}
	// The echo is what tells a caller which targets the request reached, and
	// the two lists are the same shape: without this, groups and projects
	// could be echoed under each other's name.
	if len(out.GroupIDs) != 1 || out.GroupIDs[0] != 5 {
		t.Errorf("BulkUpdate() group_ids = %#v, want [5]", out.GroupIDs)
	}
	if len(out.ProjectIDs) != 1 || out.ProjectIDs[0] != 42 {
		t.Errorf("BulkUpdate() project_ids = %#v, want [42]", out.ProjectIDs)
	}
	if len(out.AttributeIDs) != 1 || out.AttributeIDs[0] != 9 {
		t.Errorf("BulkUpdate() attribute_ids = %#v, want [9]", out.AttributeIDs)
	}
}

// TestBulkUpdate_GroupsOnly_SendsOnlyTheGroupItems verifies that a request
// naming no project sends the groups alone and echoes no project. Every bulk
// test used to name a project, so the project list was never empty: the
// validation that only runs for a non-empty list could have run for an empty
// one and refused every group-only call, and the item list could have been
// sized off a list that is not there. REMOVE is the mode here because it was
// the one branch of the three that nothing exercised.
func TestBulkUpdate_GroupsOnly_SendsOnlyTheGroupItems(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"bulkUpdateSecurityAttributes": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if items, ok := input["items"].([]any); !ok || len(items) != 2 || items[0] != "gid://gitlab/Group/5" || items[1] != "gid://gitlab/Group/6" {
				t.Errorf("items = %#v, want the two groups and nothing else", input["items"])
			}
			if input["mode"] != "REMOVE" {
				t.Errorf("mode = %#v, want REMOVE", input["mode"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"bulkUpdateSecurityAttributes":{"errors":[]}}`)
		},
	})

	out, err := BulkUpdate(context.Background(), testutil.NewTestClient(t, handler), BulkUpdateInput{
		GroupIDs:     []int64{5, 6},
		AttributeIDs: []int64{9},
		Mode:         BulkUpdateModeRemove,
	})
	if err != nil {
		t.Fatalf("BulkUpdate() error = %v", err)
	}
	if out.Mode != BulkUpdateModeRemove || len(out.ProjectIDs) != 0 {
		t.Fatalf("BulkUpdate() output = %#v, want mode REMOVE and no projects", out)
	}
}

// TestBulkUpdate_InvalidMode verifies that BulkUpdate rejects modes outside the
// GitLab-supported ADD, REMOVE, and REPLACE values.
//
// The invalid UPSERT mode should fail validation before any request is sent,
// keeping unsupported destructive operations away from GitLab.
func TestBulkUpdate_InvalidMode(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := BulkUpdate(context.Background(), client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: "UPSERT"})
	if err == nil || !strings.Contains(err.Error(), "ADD, REMOVE, or REPLACE") {
		t.Fatalf("BulkUpdate() error = %v, want invalid mode", err)
	}
}

// TestBulkUpdate_ValidatesInputBeforeRequest verifies BulkUpdate validation for
// targets, attributes, and positive numeric IDs.
//
// Each table case uses a malformed target or attribute set and expects the
// corresponding validation message. The not-found handler makes an unexpected
// network call visible as a test failure.
func TestBulkUpdate_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	tests := []struct {
		name  string
		input BulkUpdateInput
		want  string
	}{
		{name: "missing targets", input: BulkUpdateInput{AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd}, want: "provide group_ids or project_ids"},
		{name: "invalid group ID", input: BulkUpdateInput{GroupIDs: []int64{0}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd}, want: "group_ids values must be greater than 0"},
		{name: "invalid project ID", input: BulkUpdateInput{ProjectIDs: []int64{-1}, AttributeIDs: []int64{9}, Mode: BulkUpdateModeAdd}, want: "project_ids values must be greater than 0"},
		{name: "missing attributes", input: BulkUpdateInput{ProjectIDs: []int64{42}, Mode: BulkUpdateModeAdd}, want: "attribute_ids is required"},
		{name: "invalid attribute ID", input: BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{0}, Mode: BulkUpdateModeAdd}, want: "attribute_ids values must be greater than 0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BulkUpdate(context.Background(), client, tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("BulkUpdate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestBulkUpdateSecurityAttributes_ValidatesOptions verifies the lower-level
// GraphQL bulk update helper requires attributes and mode.
//
// The helper receives partially populated client-go options and should return
// local validation errors before building the mutation request.
func TestBulkUpdateSecurityAttributes_ValidatesOptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	mode := glBulkMode(BulkUpdateModeAdd)
	ids := []int64{9}
	noIDs := []int64{}
	tests := []struct {
		name string
		opts *gl.BulkUpdateSecurityAttributesOptions
		want string
	}{
		{name: "missing attributes", opts: &gl.BulkUpdateSecurityAttributesOptions{Mode: &mode}, want: "attribute_ids is required"},
		// An empty list is a different shape from a missing one, and the only
		// caller in this package rejects it first, so nothing else can reach
		// this branch of the helper's own guard.
		{name: "empty attributes", opts: &gl.BulkUpdateSecurityAttributesOptions{AttributeIDs: &noIDs, Mode: &mode}, want: "attribute_ids is required"},
		{name: "missing mode", opts: &gl.BulkUpdateSecurityAttributesOptions{AttributeIDs: &ids}, want: "mode is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bulkUpdateSecurityAttributes(context.Background(), client, tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("bulkUpdateSecurityAttributes() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// testAttribute is the attribute the Markdown tests render. Every
// GitLab-authored value carries a pipe, so a comparison of the whole render
// pins the escaping as well as the layout.
var testAttribute = Output{
	ID:            9,
	Name:          "High | Risk",
	Color:         "#FF|0000",
	Description:   "Needs | review",
	EditableState: "EDITABLE",
	SecurityCategory: &CategorySummary{
		ID:                3,
		Name:              "Business | Impact",
		MultipleSelection: true,
		EditableState:     "EDITABLE",
		TemplateType:      "CUSTOM",
	},
}

// TestFormatOutputMarkdown_RendersTheCard verifies that one security attribute
// renders as a card: one list item per field, the category as a nested object
// under its own label, and every GitLab-authored value escaped. The whole
// render is compared, since a substring assertion passes on a row that landed
// outside the block it was meant for.
func TestFormatOutputMarkdown_RendersTheCard(t *testing.T) {
	want := "## Security Attribute: High | Risk\n\n" +
		"- **ID**: 9\n" +
		"- **Name**: High &#124; Risk\n" +
		"- **Color**: `#FF|0000`\n" +
		"- **Description**: Needs &#124; review\n" +
		"- **Editable state**: `EDITABLE`\n" +
		"- **Category**:\n" +
		"  - **ID**: 3\n" +
		"  - **Name**: Business &#124; Impact\n" +
		"  - **Multiple selection**: ✅\n" +
		"  - **Editable state**: `EDITABLE`\n" +
		"  - **Template type**: `CUSTOM`\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.update' to rename, re-describe or recolor this attribute\n" +
		"- Use action 'security_attribute.project_update' to apply this attribute to a project\n" +
		"- Use action 'security_attribute.bulk_update' to apply it to many groups or projects at once\n"

	if got := FormatOutputMarkdown(testAttribute); got != want {
		t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatOutputMarkdown_LockedAttributeIsNotOfferedAnEdit verifies that a
// template-provided attribute is not offered a call GitLab refuses.
func TestFormatOutputMarkdown_LockedAttributeIsNotOfferedAnEdit(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 9, Name: "High", EditableState: "LOCKED"})

	want := "## Security Attribute: High\n\n" +
		"- **ID**: 9\n" +
		"- **Name**: High\n" +
		"- **Editable state**: `LOCKED`\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.project_update' to apply this attribute to a project\n" +
		"- Use action 'security_attribute.bulk_update' to apply it to many groups or projects at once\n"

	if md != want {
		t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatCreateMarkdown_RendersTheCollection verifies that the attributes
// one request created render as a table of objects sharing columns, with the
// count in the heading and no instruction to preserve links a table without
// links cannot honor.
func TestFormatCreateMarkdown_RendersTheCollection(t *testing.T) {
	want := "## Security Attributes Created (1)\n\n" +
		"| ID | Name | Color | Description | Category | Editable state |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 9 | High &#124; Risk | `#FF\\|0000` | Needs &#124; review | Business &#124; Impact | `EDITABLE` |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.project_update' to apply these attributes to a project\n" +
		"- Use action 'security_attribute.bulk_update' to apply them to many groups or projects at once\n"

	got := FormatCreateMarkdown(CreateOutput{Attributes: []Output{testAttribute}})
	if got != want {
		t.Errorf("FormatCreateMarkdown() =\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "clickable [text](url) links") {
		t.Error("FormatCreateMarkdown() tells the model to keep links a table without links cannot have")
	}
}

// TestFormatOutputMarkdown_UnnamedAttribute_OpensTheGenericHeading verifies
// that an attribute GitLab sent no name for still opens a heading, rather than
// one trailing a colon and nothing. The name is what the heading is built
// from, so the empty case is the only one that can produce it.
func TestFormatOutputMarkdown_UnnamedAttribute_OpensTheGenericHeading(t *testing.T) {
	md := FormatOutputMarkdown(Output{ID: 9, Name: "  ", EditableState: "EDITABLE"})

	want := "## Security Attribute\n\n" +
		"- **ID**: 9\n" +
		"- **Editable state**: `EDITABLE`\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.update' to rename, re-describe or recolor this attribute\n" +
		"- Use action 'security_attribute.project_update' to apply this attribute to a project\n" +
		"- Use action 'security_attribute.bulk_update' to apply it to many groups or projects at once\n"

	if md != want {
		t.Errorf("FormatOutputMarkdown() =\n%s\nwant:\n%s", md, want)
	}
}

// TestFormatCreateMarkdown_AttributeWithoutACategory_LeavesTheCellEmpty
// verifies that an attribute carrying no category still renders its row, with
// an empty category cell. Reading the category's name off a nil pointer is the
// one way this table can panic, and until now every row the tests rendered
// had a category.
func TestFormatCreateMarkdown_AttributeWithoutACategory_LeavesTheCellEmpty(t *testing.T) {
	want := "## Security Attributes Created (1)\n\n" +
		"| ID | Name | Color | Description | Category | Editable state |\n" +
		"| --- | --- | --- | --- | --- | --- |\n" +
		"| 9 | High | `#FF0000` | High impact |  | `EDITABLE` |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.project_update' to apply these attributes to a project\n" +
		"- Use action 'security_attribute.bulk_update' to apply them to many groups or projects at once\n"

	got := FormatCreateMarkdown(CreateOutput{Attributes: []Output{{
		ID:            9,
		Name:          "High",
		Color:         "#FF0000",
		Description:   "High impact",
		EditableState: "EDITABLE",
	}}})
	if got != want {
		t.Errorf("FormatCreateMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatCreateMarkdown_Empty verifies that a creation response carrying no
// attributes renders the one empty-list sentence rather than a heading
// counting zero above an empty table.
func TestFormatCreateMarkdown_Empty(t *testing.T) {
	if got, want := FormatCreateMarkdown(CreateOutput{}), "No security attributes found.\n"; got != want {
		t.Errorf("FormatCreateMarkdown() = %q, want %q", got, want)
	}
}

// TestFormatProjectUpdateMarkdown_RendersBothCounts verifies that the project
// update result renders as a card carrying both counts, zero included: nothing
// added is the answer to a request that asked for a removal.
func TestFormatProjectUpdateMarkdown_RendersBothCounts(t *testing.T) {
	want := "## Project Security Attributes Updated\n\n" +
		"- **Added**: 2\n" +
		"- **Removed**: 1\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.bulk_update' to apply the same change to many groups or projects at once\n" +
		"- Use action 'security_attribute.project_update' to change this project's attributes again\n"

	if got := FormatProjectUpdateMarkdown(ProjectUpdateOutput{AddedCount: 2, RemovedCount: 1}); got != want {
		t.Errorf("FormatProjectUpdateMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatBulkUpdateMarkdown_RendersIDsAsALists verifies that the bulk
// update result renders every identifier list as the comma-separated numbers a
// reader can act on, rather than as Go's "[9 10]" container syntax, and that
// an empty list writes no row at all.
func TestFormatBulkUpdateMarkdown_RendersIDsAsALists(t *testing.T) {
	want := "## Security Attributes Updated in Bulk\n\n" +
		"- **Status**: success\n" +
		"- **Mode**: REPLACE\n" +
		"- **Attributes**: 9, 10\n" +
		"- **Groups**: 5\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'security_attribute.project_update' to change one project's attributes instead\n" +
		"- Use action 'security_category.create' to add a category to classify further\n"

	got := FormatBulkUpdateMarkdown(BulkUpdateOutput{
		Status:       "success",
		Mode:         BulkUpdateModeReplace,
		GroupIDs:     []int64{5},
		AttributeIDs: []int64{9, 10},
	})
	if got != want {
		t.Errorf("FormatBulkUpdateMarkdown() =\n%s\nwant:\n%s", got, want)
	}
}

// TestOutputHelpers_HandleNilValues_ReturnZeroValues verifies nil GraphQL nodes
// convert to zero-value outputs without panicking.
//
// The test exercises attribute, category summary, and attribute slice helpers to
// keep defensive conversion behavior stable for partial GraphQL responses.
func TestOutputHelpers_HandleNilValues_ReturnZeroValues(t *testing.T) {
	if out, err := attributeNodeOutput(nil); err != nil || out.ID != 0 || out.SecurityCategory != nil {
		t.Fatalf("attributeNodeOutput(nil) = %#v", out)
	}
	if summary, err := categoryNodeSummary(nil); err != nil || summary == nil || summary.ID != 0 {
		t.Fatalf("categoryNodeSummary(nil) = %#v, want zero summary", summary)
	}
	if out, err := attributeNodesOutput(nil); err != nil || len(out.Attributes) != 0 {
		t.Fatalf("attributeNodesOutput(nil) = %#v", out)
	}
	// An attribute node carrying no category converts to an output carrying
	// none, rather than to one carrying an empty category object a reader
	// would take for a category GitLab named.
	out, err := attributeNodeOutput(&attributeNode{ID: "gid://gitlab/Security::Attribute/9", Name: "High"})
	if err != nil || out.ID != 9 || out.Name != "High" || out.SecurityCategory != nil {
		t.Fatalf("attributeNodeOutput(no category) = %#v, err = %v", out, err)
	}
}

func glBulkMode(mode BulkUpdateMode) gl.SecurityAttributeBulkUpdateMode {
	return gl.SecurityAttributeBulkUpdateMode(mode)
}

// declaredActionIDs is every canonical action ID this package may name,
// written out here rather than read from the constants so that a reviewer
// comparing this list with the catalog is comparing the catalog with
// something, not with itself.
var declaredActionIDs = map[string]bool{
	"security_attribute.create":         true,
	"security_attribute.update":         true,
	"security_attribute.delete":         true,
	"security_attribute.project_update": true,
	"security_attribute.bulk_update":    true,
	"security_category.create":          true,
	"security_category.update":          true,
	"security_category.delete":          true,
	"project.get":                       true,
	"group.get":                         true,
}

// hintedActionIDs returns every action ID the rendered Markdown names, read
// back out of the hint sentence [toolutil.HintAction] writes.
func hintedActionIDs(markdown string) []string {
	var ids []string
	for _, rest := range strings.Split(markdown, "Use action '")[1:] {
		if id, _, found := strings.Cut(rest, "'"); found {
			ids = append(ids, id)
		}
	}
	return ids
}

// TestActionIDs_EveryIDThisPackageNames_IsOneOfTheDeclaredOnes verifies that
// the related actions of all five specs and the hints of all four Markdown
// formatters name only the IDs declared in action_specs.go.
//
// Nothing in the repository checks these strings against the catalog: the
// discovery audit only counts an empty related list, so a wrong spelling
// passes every gate and answers a model "unknown action" the moment it
// follows the hint. This package kept two constant blocks naming the same
// calls under two spellings, which is how such a pair drifts; the block is one
// now, and this is what holds it there.
func TestActionIDs_EveryIDThisPackageNames_IsOneOfTheDeclaredOnes(t *testing.T) {
	for id, constant := range map[string]string{
		"security_attribute.create":         actionAttributeCreate,
		"security_attribute.update":         actionAttributeUpdate,
		"security_attribute.delete":         actionAttributeDelete,
		"security_attribute.project_update": actionAttributeProjectUpdate,
		"security_attribute.bulk_update":    actionAttributeBulkUpdate,
		"security_category.create":          actionCategoryCreate,
		"security_category.update":          actionCategoryUpdate,
		"security_category.delete":          actionCategoryDelete,
		"project.get":                       actionProjectGet,
		"group.get":                         actionGroupGet,
	} {
		t.Run("constant "+id, func(t *testing.T) {
			if constant != id {
				t.Errorf("constant = %q, want %q", constant, id)
			}
		})
	}

	client := testutil.NewTestClient(t, http.NotFoundHandler())
	for _, spec := range ActionSpecs(client) {
		t.Run("related actions of "+spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				if !declaredActionIDs[related] {
					t.Errorf("related action %q is not a declared ID", related)
				}
			}
		})
	}

	renders := map[string]string{
		"attribute":      FormatOutputMarkdown(testAttribute),
		"locked":         FormatOutputMarkdown(Output{ID: 9, EditableState: editableStateLocked}),
		"create":         FormatCreateMarkdown(CreateOutput{Attributes: []Output{testAttribute}}),
		"project update": FormatProjectUpdateMarkdown(ProjectUpdateOutput{}),
		"bulk update":    FormatBulkUpdateMarkdown(BulkUpdateOutput{Status: "success"}),
	}
	for name, markdown := range renders {
		t.Run("hints of "+name, func(t *testing.T) {
			ids := hintedActionIDs(markdown)
			if len(ids) == 0 {
				t.Fatalf("render names no action at all:\n%s", markdown)
			}
			for _, id := range ids {
				if !declaredActionIDs[id] {
					t.Errorf("hint names %q, which is not a declared ID", id)
				}
			}
		})
	}
}

// TestActionSpecs_Metadata_ExpectedResult verifies the canonical security
// attribute ActionSpecs expose expected mutation metadata and schemas.
//
// The test checks destructive flags, individual tool names, related actions,
// description text, schema minItems, color pattern, anyOf target requirements,
// and mode enum values. This protects dynamic, meta, and individual surfaces
// from drifting away from the security attribute contract.
func TestActionSpecs_Metadata_ExpectedResult(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := ActionSpecs(client)
	if len(specs) != 5 {
		t.Fatalf("ActionSpecs() len = %d, want 5", len(specs))
	}

	specByName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByName[spec.Name] = spec
	}
	deleteSpec := specByName["delete"]
	if !deleteSpec.Destructive || deleteSpec.IndividualTool.Name != "gitlab_delete_security_attribute" {
		t.Fatalf("delete spec = %#v", deleteSpec)
	}
	bulkUpdateSpec := specByName["bulk_update"]
	if !bulkUpdateSpec.Destructive || bulkUpdateSpec.IndividualTool.Name != "gitlab_bulk_update_security_attributes" {
		t.Fatalf("bulk update spec = %#v", bulkUpdateSpec)
	}
	projectUpdateSpec := specByName["project_update"]
	if !projectUpdateSpec.Destructive || projectUpdateSpec.IndividualTool.Name != "gitlab_update_project_security_attributes" {
		t.Fatalf("project update spec = %#v", projectUpdateSpec)
	}
	if strings.Count(strings.Join(specs[0].RelatedActions, ","), "security_category.create") != 1 {
		t.Fatalf("create related actions = %#v, want security_category.create once", specs[0].RelatedActions)
	}
	if !strings.Contains(bulkUpdateSpec.IndividualTool.Description, "selected target/attribute IDs") {
		t.Fatalf("bulk update description = %q", bulkUpdateSpec.IndividualTool.Description)
	}

	createProperties := specByName["create"].Route.InputSchema["properties"].(map[string]any)
	attributes := createProperties["attributes"].(map[string]any)
	if attributes["minItems"] != 1 {
		t.Fatalf("create attributes schema = %#v, want minItems 1", attributes)
	}
	color := attributes["items"].(map[string]any)["properties"].(map[string]any)["color"].(map[string]any)
	if color["pattern"] != hexColorSchemaPattern {
		t.Fatalf("attribute color schema = %#v, want hex pattern", color)
	}

	bulkSchema := bulkUpdateSpec.Route.InputSchema
	if anyOf, ok := bulkSchema["anyOf"].([]any); !ok || len(anyOf) != 2 {
		t.Fatalf("bulk update anyOf = %#v, want group/project requirement", bulkSchema["anyOf"])
	}
	bulkProperties := bulkSchema["properties"].(map[string]any)
	if bulkProperties["attribute_ids"].(map[string]any)["minItems"] != 1 {
		t.Fatalf("bulk attribute_ids schema = %#v", bulkProperties["attribute_ids"])
	}
	modeEnum := bulkProperties["mode"].(map[string]any)["enum"].([]string)
	if strings.Join(modeEnum, ",") != "ADD,REMOVE,REPLACE" {
		t.Fatalf("bulk mode enum = %#v", modeEnum)
	}
}
