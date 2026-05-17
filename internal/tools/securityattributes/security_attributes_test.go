// security_attributes_test.go contains unit tests for GitLab security attribute operations.
package securityattributes

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
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
		"templateType": null
	}
}`

func attributeGraphQLMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

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
}

func TestCreate_RequiresAttributes(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7})
	if err == nil || !strings.Contains(err.Error(), "attributes is required") {
		t.Fatalf("Create() error = %v, want attributes required", err)
	}
}

func TestUpdate_Success(t *testing.T) {
	name := "Critical"
	color := "#990000"
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["id"] != "gid://gitlab/Security::Attribute/9" {
				t.Fatalf("id = %#v", input["id"])
			}
			if input["name"] != name || input["color"] != color {
				t.Fatalf("input = %#v", input)
			}
			testutil.RespondGraphQL(w, http.StatusOK, `{"securityAttributeUpdate":{"securityAttribute":`+sampleAttribute+`,"errors":[]}}`)
		},
	})

	client := testutil.NewTestClient(t, handler)
	out, err := Update(context.Background(), client, UpdateInput{AttributeID: 9, Name: &name, Color: &color})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if out.ID != 9 || out.SecurityCategory == nil {
		t.Fatalf("Update() output = %#v", out)
	}
}

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

func TestProjectUpdate_Success(t *testing.T) {
	handler := attributeGraphQLMux(map[string]http.HandlerFunc{
		"securityAttributeProjectUpdate": func(w http.ResponseWriter, r *http.Request) {
			input := attributeGraphQLInput(t, r)
			if input["projectId"] != "gid://gitlab/Project/42" {
				t.Fatalf("projectId = %#v", input["projectId"])
			}
			if got := input["addAttributeIds"].([]any)[0]; got != "gid://gitlab/Security::Attribute/9" {
				t.Fatalf("addAttributeIds = %#v", input["addAttributeIds"])
			}
			if got := input["removeAttributeIds"].([]any)[0]; got != "gid://gitlab/Security::Attribute/10" {
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

func TestBulkUpdate_InvalidMode(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := BulkUpdate(context.Background(), client, BulkUpdateInput{ProjectIDs: []int64{42}, AttributeIDs: []int64{9}, Mode: "UPSERT"})
	if err == nil || !strings.Contains(err.Error(), "ADD, REMOVE, or REPLACE") {
		t.Fatalf("BulkUpdate() error = %v, want invalid mode", err)
	}
}

func TestActionSpecs(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := ActionSpecs(client)
	if len(specs) != 5 {
		t.Fatalf("ActionSpecs() len = %d, want 5", len(specs))
	}
	if specs[2].Name != "delete" || !specs[2].Destructive || specs[2].IndividualTool.Name != "gitlab_delete_security_attribute" {
		t.Fatalf("delete spec = %#v", specs[2])
	}
	if specs[4].Name != "bulk_update" || !specs[4].Destructive || specs[4].IndividualTool.Name != "gitlab_bulk_update_security_attributes" {
		t.Fatalf("bulk update spec = %#v", specs[4])
	}
}
