// security_attributes_test.go contains unit tests for GitLab security attribute operations.
package securityattributes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
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
		"templateType": "APPLICATION"
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
	if out.Attributes[0].SecurityCategory.TemplateType != "APPLICATION" {
		t.Fatalf("TemplateType = %q, want APPLICATION", out.Attributes[0].SecurityCategory.TemplateType)
	}
}

func TestCreate_RequiresAttributes(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Create(context.Background(), client, CreateInput{NamespaceID: 101, CategoryID: 7})
	if err == nil || !strings.Contains(err.Error(), "attributes is required") {
		t.Fatalf("Create() error = %v, want attributes required", err)
	}
}

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

func TestDelete_ValidatesInputBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.NotFoundHandler())
	_, err := Delete(context.Background(), client, DeleteInput{AttributeID: 0})
	if err == nil || !strings.Contains(err.Error(), "attribute_id must be greater than 0") {
		t.Fatalf("Delete() error = %v, want invalid attribute ID", err)
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

func TestMarkdownEscapesTableCellsAndPreserveLinkHint(t *testing.T) {
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

func TestFormatCreateMarkdown_Empty(t *testing.T) {
	md := FormatCreateMarkdown(CreateOutput{})
	if !strings.Contains(md, "No security attributes returned.") {
		t.Fatalf("FormatCreateMarkdown() =\n%s", md)
	}
}

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

func TestOutputHelpersHandleNilValues(t *testing.T) {
	if out := toOutput(nil); out.ID != 0 || out.SecurityCategory != nil {
		t.Fatalf("toOutput(nil) = %#v", out)
	}
	if summary := categorySummary(nil); summary != nil {
		t.Fatalf("categorySummary(nil) = %#v, want nil", summary)
	}
	if out := toCreateOutput(nil); len(out.Attributes) != 0 {
		t.Fatalf("toCreateOutput(nil) = %#v", out)
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
	if strings.Count(strings.Join(specs[0].RelatedActions, ","), "security_category.create") != 1 {
		t.Fatalf("create related actions = %#v, want security_category.create once", specs[0].RelatedActions)
	}
	if !strings.Contains(specs[4].IndividualTool.Description, "selected target/attribute IDs") {
		t.Fatalf("bulk update description = %q", specs[4].IndividualTool.Description)
	}
}
