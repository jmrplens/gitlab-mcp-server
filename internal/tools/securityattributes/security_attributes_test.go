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

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// attributeGraphQLMux returns a GraphQL test handler for security attribute
// mutations keyed by operation name.
func attributeGraphQLMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

// attributeGraphQLInput parses the GraphQL input variables from r and fails the
// calling test if the request does not contain the expected input object.
func attributeGraphQLInput(t *testing.T, r *http.Request) map[string]any {
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
				t.Fatalf("namespaceId = %#v", input["namespaceId"])
			}
			if input["categoryId"] != "gid://gitlab/Security::Category/7" {
				t.Fatalf("categoryId = %#v", input["categoryId"])
			}
			attributes, ok := input["attributes"].([]any)
			if !ok || len(attributes) != 1 {
				t.Fatalf("attributes = %#v", input["attributes"])
			}
			attribute, ok := attributes[0].(map[string]any)
			if !ok {
				t.Fatalf("attribute = %#v", attributes[0])
			}
			if attribute["name"] != "High" || attribute["description"] != "High impact" || attribute["color"] != "#FF0000" {
				t.Fatalf("attribute input = %#v", attribute)
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
				t.Fatalf("id = %#v", input["id"])
			}
			if input["name"] != name || input["description"] != description || input["color"] != color {
				t.Fatalf("input = %#v", input)
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
				t.Fatalf("id = %#v", input["id"])
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
// removeAttributeIds fields, then returns added and removed counts. The handler
// output must preserve those counts for callers.
func TestProjectUpdate_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeProjectUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["projectId"] != "gid://gitlab/Project/42" {
				t.Fatalf("projectId = %#v", input["projectId"])
			}
			if input["addAttributeIds"].([]any)[0] != "gid://gitlab/Security::Attribute/9" {
				t.Fatalf("addAttributeIds = %#v", input["addAttributeIds"])
			}
			if input["removeAttributeIds"].([]any)[0] != "gid://gitlab/Security::Attribute/10" {
				t.Fatalf("removeAttributeIds = %#v", input["removeAttributeIds"])
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeProjectUpdate":{"addedCount":1,"removedCount":1,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := ProjectUpdate(context.Background(), client, ProjectUpdateInput{ProjectID: 42, AddAttributeIDs: []int64{9}, RemoveAttributeIDs: []int64{10}})
	if err != nil {
		t.Fatalf("ProjectUpdate() error = %v", err)
	}
	if out.AddedCount != 1 || out.RemovedCount != 1 {
		t.Fatalf("ProjectUpdate() output = %#v", out)
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
			items := input["items"].([]any)
			if len(items) != 2 || items[0] != "gid://gitlab/Group/5" || items[1] != "gid://gitlab/Project/42" {
				t.Fatalf("items = %#v", items)
			}
			attributes := input["attributes"].([]any)
			if len(attributes) != 1 || attributes[0] != "gid://gitlab/Security::Attribute/9" {
				t.Fatalf("attributes = %#v", attributes)
			}
			if input["mode"] != "REPLACE" {
				t.Fatalf("mode = %#v", input["mode"])
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
	tests := []struct {
		name string
		opts *gl.BulkUpdateSecurityAttributesOptions
		want string
	}{
		{name: "missing attributes", opts: &gl.BulkUpdateSecurityAttributesOptions{Mode: &mode}, want: "attribute_ids is required"},
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

// TestMarkdown_EscapesTableCells_PreservesLinkHint verifies security attribute
// markdown escapes table separators and preserves link guidance.
//
// The test renders values containing pipe characters through create and detail
// formatters, then asserts escaped cells, editable-state output, and the
// preserve-link hint used by catalog responses.
func TestMarkdown_EscapesTableCells_PreservesLinkHint(t *testing.T) {
	attribute := Output{
		ID:               9,
		Name:             "High | Risk",
		Color:            "#FF|0000",
		Description:      "Needs | review",
		EditableState:    "EDITABLE",
		SecurityCategory: &CategorySummary{Name: "Business | Impact"},
	}
	createMarkdown := FormatCreateMarkdown(CreateOutput{Attributes: []Output{attribute}})
	if !strings.Contains(createMarkdown, "High &#124; Risk") || !strings.Contains(createMarkdown, "#FF&#124;0000") || !strings.Contains(createMarkdown, "Business &#124; Impact") {
		t.Fatalf("FormatCreateMarkdown() did not escape table cells:\n%s", createMarkdown)
	}
	if !strings.Contains(createMarkdown, "clickable [text](url) links") {
		t.Fatalf("FormatCreateMarkdown() missing preserve-link hint:\n%s", createMarkdown)
	}

	outputMarkdown := FormatOutputMarkdown(attribute)
	for _, want := range []string{"#FF&#124;0000", "Needs &#124; review", "| Editable state | `EDITABLE` |"} {
		if !strings.Contains(outputMarkdown, want) {
			t.Fatalf("FormatOutputMarkdown() missing %q:\n%s", want, outputMarkdown)
		}
	}
}

// TestFormatCreateMarkdown_Empty verifies that FormatCreateMarkdown reports an
// empty creation response clearly.
//
// The test passes a zero-value CreateOutput and expects the fallback message
// rather than an empty table, which keeps generated tool output understandable.
func TestFormatCreateMarkdown_Empty(t *testing.T) {
	md := FormatCreateMarkdown(CreateOutput{})
	if !strings.Contains(md, "No security attributes returned.") {
		t.Fatalf("FormatCreateMarkdown() =\n%s", md)
	}
}

// TestMarkdownFormatsProjectAndBulkUpdates verifies the project and bulk update
// markdown formatters expose their operational counts and selections.
//
// The test checks added and removed project counts, bulk mode, attribute IDs,
// group IDs, and project IDs so regressions in compact mutation summaries are
// caught by unit tests.
func TestMarkdownFormatsProjectAndBulkUpdates(t *testing.T) {
	projectMarkdown := FormatProjectUpdateMarkdown(ProjectUpdateOutput{AddedCount: 2, RemovedCount: 1})
	if !strings.Contains(projectMarkdown, "| Added | `2` |") || !strings.Contains(projectMarkdown, "| Removed | `1` |") {
		t.Fatalf("FormatProjectUpdateMarkdown() =\n%s", projectMarkdown)
	}

	bulkMarkdown := FormatBulkUpdateMarkdown(BulkUpdateOutput{
		Mode:         BulkUpdateModeReplace,
		GroupIDs:     []int64{5},
		ProjectIDs:   []int64{42},
		AttributeIDs: []int64{9, 10},
	})
	for _, want := range []string{"| Mode | `REPLACE` |", "| Attributes | `[9 10]` |", "| Groups | `[5]` |", "| Projects | `[42]` |"} {
		if !strings.Contains(bulkMarkdown, want) {
			t.Fatalf("FormatBulkUpdateMarkdown() missing %q:\n%s", want, bulkMarkdown)
		}
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
}

func glBulkMode(mode BulkUpdateMode) gl.SecurityAttributeBulkUpdateMode {
	return gl.SecurityAttributeBulkUpdateMode(mode)
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
