// security_categories_test.go contains unit tests for GitLab security category operations.
package securitycategories

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const sampleCategory = `{
	"id": "gid://gitlab/Security::Category/7",
	"name": "Business impact",
	"description": "Business impact labels",
	"multipleSelection": true,
	"editableState": "EDITABLE",
	"templateType": null,
	"securityAttributes": [{
		"id": "gid://gitlab/Security::Attribute/9",
		"name": "High",
		"color": "#FF0000",
		"description": "High impact",
		"editableState": "EDITABLE"
	}]
}`

func categoryGraphQLMux(handlers map[string]http.HandlerFunc) http.Handler {
	return testutil.GraphQLHandler(handlers)
}

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
	if len(out.SecurityAttributes) != 1 || out.SecurityAttributes[0].ID != 9 {
		t.Fatalf("SecurityAttributes = %#v", out.SecurityAttributes)
	}
}

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

func TestUpdate_RequiresChanges(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Update(context.Background(), client, UpdateInput{CategoryID: 7, NamespaceID: 101})
	if err == nil || !strings.Contains(err.Error(), "provide at least one") {
		t.Fatalf("Update() error = %v, want missing changes", err)
	}
}

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

func TestActionSpecs(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	specs := ActionSpecs(client)
	if len(specs) != 3 {
		t.Fatalf("ActionSpecs() len = %d, want 3", len(specs))
	}
	if specs[2].Name != "delete" || !specs[2].Destructive || specs[2].IndividualTool.Name != "gitlab_delete_security_category" {
		t.Fatalf("delete spec = %#v", specs[2])
	}
}
