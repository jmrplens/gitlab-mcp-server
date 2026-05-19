package securityattributes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

const hexColorSchemaPattern = `^#[0-9A-Fa-f]{6}$`

var hexColorPattern = regexp.MustCompile(hexColorSchemaPattern)

const (
	groupGIDType             = "Group"
	namespaceGIDType         = "Namespace"
	projectGIDType           = "Project"
	securityAttributeGIDType = "Security::Attribute"
	securityCategoryGIDType  = "Security::Category"

	securityAttributeCreateMutation = `
		mutation CreateSecurityAttributes($input: SecurityAttributeCreateInput!) {
			securityAttributeCreate(input: $input) {
				securityAttributes {
					id
					name
					color
					description
					editableState
					securityCategory {
						id
						name
						description
						multipleSelection
						editableState
						templateType
					}
				}
				errors
			}
		}`
	securityAttributeUpdateMutation = `
		mutation UpdateSecurityAttribute($input: SecurityAttributeUpdateInput!) {
			securityAttributeUpdate(input: $input) {
				securityAttribute {
					id
					name
					color
					description
					editableState
					securityCategory {
						id
						name
						description
						multipleSelection
						editableState
						templateType
					}
				}
				errors
			}
		}`
	securityAttributeDestroyMutation = `
		mutation DestroySecurityAttribute($input: SecurityAttributeDestroyInput!) {
			securityAttributeDestroy(input: $input) {
				errors
			}
		}`
	securityAttributeProjectUpdateMutation = `
		mutation ProjectUpdateSecurityAttribute($input: SecurityAttributeProjectUpdateInput!) {
			securityAttributeProjectUpdate(input: $input) {
				addedCount
				removedCount
				errors
			}
		}`
	securityAttributeBulkUpdateMutation = `
		mutation BulkUpdateSecurityAttributes($input: BulkUpdateSecurityAttributesInput!) {
			bulkUpdateSecurityAttributes(input: $input) {
				errors
			}
		}`
)

// BulkUpdateMode is the mode used when applying security attributes in bulk.
type BulkUpdateMode string

const (
	// BulkUpdateModeAdd adds attributes while preserving existing assignments.
	BulkUpdateModeAdd BulkUpdateMode = "ADD"
	// BulkUpdateModeRemove removes attributes from the selected items.
	BulkUpdateModeRemove BulkUpdateMode = "REMOVE"
	// BulkUpdateModeReplace replaces existing assignments with the supplied attributes.
	BulkUpdateModeReplace BulkUpdateMode = "REPLACE"
)

// AttributeInput defines one security attribute to create.
type AttributeInput struct {
	Name        string `json:"name"                  jsonschema:"Security attribute name,required"`
	Description string `json:"description"           jsonschema:"Security attribute description,required"`
	Color       string `json:"color"                 jsonschema:"Security attribute color as a hex code (e.g. #FF0000),required"`
}

// CreateInput defines parameters for creating security attributes.
type CreateInput struct {
	NamespaceID int64            `json:"namespace_id"          jsonschema:"Numeric namespace ID,required"`
	CategoryID  int64            `json:"category_id"           jsonschema:"Numeric security category ID,required"`
	Attributes  []AttributeInput `json:"attributes"            jsonschema:"Security attributes to create,required"`
}

// UpdateInput defines parameters for updating a security attribute.
type UpdateInput struct {
	AttributeID int64   `json:"attribute_id"           jsonschema:"Numeric security attribute ID,required"`
	Name        *string `json:"name,omitempty"         jsonschema:"New security attribute name"`
	Description *string `json:"description,omitempty"  jsonschema:"New security attribute description"`
	Color       *string `json:"color,omitempty"        jsonschema:"New security attribute color as a hex code (e.g. #FF0000)"`
}

// DeleteInput defines parameters for deleting a security attribute.
type DeleteInput struct {
	AttributeID int64 `json:"attribute_id" jsonschema:"Numeric security attribute ID,required"`
}

// ProjectUpdateInput defines parameters for adding or removing attributes on a project.
type ProjectUpdateInput struct {
	ProjectID          int64   `json:"project_id"                    jsonschema:"Numeric project ID,required"`
	AddAttributeIDs    []int64 `json:"add_attribute_ids,omitempty"    jsonschema:"Security attribute IDs to add"`
	RemoveAttributeIDs []int64 `json:"remove_attribute_ids,omitempty" jsonschema:"Security attribute IDs to remove"`
}

// BulkUpdateInput defines parameters for applying attributes to groups and projects in bulk.
type BulkUpdateInput struct {
	GroupIDs     []int64        `json:"group_ids,omitempty"     jsonschema:"Numeric group IDs to update"`
	ProjectIDs   []int64        `json:"project_ids,omitempty"   jsonschema:"Numeric project IDs to update"`
	AttributeIDs []int64        `json:"attribute_ids"           jsonschema:"Security attribute IDs to apply,required"`
	Mode         BulkUpdateMode `json:"mode"                    jsonschema:"Bulk update mode: ADD, REMOVE, or REPLACE,required"`
}

// CategorySummary represents the security category attached to an attribute.
type CategorySummary struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	MultipleSelection bool   `json:"multiple_selection"`
	EditableState     string `json:"editable_state,omitempty"`
	TemplateType      string `json:"template_type,omitempty"`
}

// Output represents a GitLab security attribute.
type Output struct {
	toolutil.HintableOutput
	ID               int64            `json:"id"`
	Name             string           `json:"name"`
	Color            string           `json:"color"`
	Description      string           `json:"description,omitempty"`
	EditableState    string           `json:"editable_state,omitempty"`
	SecurityCategory *CategorySummary `json:"security_category,omitempty"`
}

// CreateOutput contains security attributes created by one request.
type CreateOutput struct {
	toolutil.HintableOutput
	Attributes []Output `json:"attributes"`
}

// ProjectUpdateOutput reports how many security attributes changed on a project.
type ProjectUpdateOutput struct {
	toolutil.HintableOutput
	AddedCount   int64 `json:"added_count"`
	RemovedCount int64 `json:"removed_count"`
}

// BulkUpdateOutput confirms a bulk security attribute update.
type BulkUpdateOutput struct {
	toolutil.HintableOutput
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Mode         BulkUpdateMode `json:"mode"`
	GroupIDs     []int64        `json:"group_ids,omitempty"`
	ProjectIDs   []int64        `json:"project_ids,omitempty"`
	AttributeIDs []int64        `json:"attribute_ids"`
}

type attributeNode struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Color            string        `json:"color"`
	Description      string        `json:"description"`
	EditableState    string        `json:"editableState"`
	SecurityCategory *categoryNode `json:"securityCategory"`
}

type categoryNode struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	Description       *string `json:"description"`
	MultipleSelection bool    `json:"multipleSelection"`
	EditableState     string  `json:"editableState"`
	TemplateType      *string `json:"templateType"`
}

type attributeMutationEnvelope struct {
	Errors []toolutil.GraphQLError `json:"errors"`
}

type securityAttributeCreatePayload struct {
	SecurityAttributes []*attributeNode `json:"securityAttributes"`
	Errors             []string         `json:"errors"`
}

type securityAttributeCreateData struct {
	SecurityAttributeCreate *securityAttributeCreatePayload `json:"securityAttributeCreate"`
}

type securityAttributeCreateResponse struct {
	Data securityAttributeCreateData `json:"data"`
	attributeMutationEnvelope
}

type securityAttributeUpdatePayload struct {
	SecurityAttribute *attributeNode `json:"securityAttribute"`
	Errors            []string       `json:"errors"`
}

type securityAttributeUpdateData struct {
	SecurityAttributeUpdate *securityAttributeUpdatePayload `json:"securityAttributeUpdate"`
}

type securityAttributeUpdateResponse struct {
	Data securityAttributeUpdateData `json:"data"`
	attributeMutationEnvelope
}

type securityAttributeDestroyPayload struct {
	Errors []string `json:"errors"`
}

type securityAttributeDestroyData struct {
	SecurityAttributeDestroy *securityAttributeDestroyPayload `json:"securityAttributeDestroy"`
}

type securityAttributeDestroyResponse struct {
	Data securityAttributeDestroyData `json:"data"`
	attributeMutationEnvelope
}

type securityAttributeProjectUpdatePayload struct {
	AddedCount   int64    `json:"addedCount"`
	RemovedCount int64    `json:"removedCount"`
	Errors       []string `json:"errors"`
}

type securityAttributeProjectUpdateData struct {
	SecurityAttributeProjectUpdate *securityAttributeProjectUpdatePayload `json:"securityAttributeProjectUpdate"`
}

type securityAttributeProjectUpdateResponse struct {
	Data securityAttributeProjectUpdateData `json:"data"`
	attributeMutationEnvelope
}

type securityAttributeBulkUpdatePayload struct {
	Errors []string `json:"errors"`
}

type securityAttributeBulkUpdateData struct {
	BulkUpdateSecurityAttributes *securityAttributeBulkUpdatePayload `json:"bulkUpdateSecurityAttributes"`
}

type securityAttributeBulkUpdateResponse struct {
	Data securityAttributeBulkUpdateData `json:"data"`
	attributeMutationEnvelope
}

func attributeNodeOutput(attribute *attributeNode) (Output, error) {
	if attribute == nil {
		return Output{}, nil
	}
	_, id, err := toolutil.ParseGID(attribute.ID)
	if err != nil {
		return Output{}, fmt.Errorf("parse security attribute id: %w", err)
	}
	out := Output{
		ID:            id,
		Name:          attribute.Name,
		Color:         attribute.Color,
		Description:   attribute.Description,
		EditableState: attribute.EditableState,
	}
	if attribute.SecurityCategory != nil {
		category, categoryErr := categoryNodeSummary(attribute.SecurityCategory)
		if categoryErr != nil {
			return Output{}, categoryErr
		}
		out.SecurityCategory = category
	}
	return out, nil
}

func categoryNodeSummary(category *categoryNode) (*CategorySummary, error) {
	if category == nil {
		return &CategorySummary{}, nil
	}
	_, id, err := toolutil.ParseGID(category.ID)
	if err != nil {
		return nil, fmt.Errorf("parse security category id: %w", err)
	}
	summary := &CategorySummary{
		ID:                id,
		Name:              category.Name,
		MultipleSelection: category.MultipleSelection,
		EditableState:     category.EditableState,
	}
	if category.Description != nil {
		summary.Description = *category.Description
	}
	if category.TemplateType != nil {
		summary.TemplateType = *category.TemplateType
	}
	return summary, nil
}

func attributeNodesOutput(attributes []*attributeNode) (CreateOutput, error) {
	out := CreateOutput{Attributes: make([]Output, 0, len(attributes))}
	for _, attribute := range attributes {
		mapped, err := attributeNodeOutput(attribute)
		if err != nil {
			return CreateOutput{}, err
		}
		out.Attributes = append(out.Attributes, mapped)
	}
	return out, nil
}

func optionalText(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := toolutil.NormalizeText(*value)
	return &normalized
}

func validatePositiveID(value int64, field string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", field)
	}
	return nil
}

func validateHexColor(color, field string) error {
	if strings.TrimSpace(color) == "" {
		return toolutil.ErrFieldRequired(field)
	}
	if !hexColorPattern.MatchString(strings.TrimSpace(color)) {
		return fmt.Errorf("%s must be a hex color in #RRGGBB format", field)
	}
	return nil
}

func validateIDList(values []int64, field string) error {
	if len(values) == 0 {
		return toolutil.ErrFieldRequired(field)
	}
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("%s values must be greater than 0", field)
		}
	}
	return nil
}

func validateAttributeInputs(attributes []AttributeInput) error {
	if len(attributes) == 0 {
		return toolutil.ErrFieldRequired("attributes")
	}
	for index, attribute := range attributes {
		if strings.TrimSpace(attribute.Name) == "" {
			return fmt.Errorf("attributes[%d].name is required", index)
		}
		if strings.TrimSpace(attribute.Description) == "" {
			return fmt.Errorf("attributes[%d].description is required", index)
		}
		if err := validateHexColor(attribute.Color, fmt.Sprintf("attributes[%d].color", index)); err != nil {
			return err
		}
	}
	return nil
}

func createAttributeOptions(attributes []AttributeInput) *gl.CreateSecurityAttributesOptions {
	inputs := make([]gl.SecurityAttributeInput, 0, len(attributes))
	for _, attribute := range attributes {
		name := strings.TrimSpace(attribute.Name)
		description := toolutil.NormalizeText(attribute.Description)
		color := strings.TrimSpace(attribute.Color)
		inputs = append(inputs, gl.SecurityAttributeInput{
			Name:        &name,
			Description: &description,
			Color:       &color,
		})
	}
	return &gl.CreateSecurityAttributesOptions{Attributes: &inputs}
}

func validateBulkMode(mode BulkUpdateMode) error {
	switch mode {
	case BulkUpdateModeAdd, BulkUpdateModeRemove, BulkUpdateModeReplace:
		return nil
	default:
		return errors.New("mode must be one of ADD, REMOVE, or REPLACE")
	}
}

// Create creates one or more GitLab security attributes.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (CreateOutput, error) {
	if err := ctx.Err(); err != nil {
		return CreateOutput{}, err
	}
	if err := validatePositiveID(input.NamespaceID, "namespace_id"); err != nil {
		return CreateOutput{}, err
	}
	if err := validatePositiveID(input.CategoryID, "category_id"); err != nil {
		return CreateOutput{}, err
	}
	if err := validateAttributeInputs(input.Attributes); err != nil {
		return CreateOutput{}, err
	}

	attributes, err := createSecurityAttributes(ctx, client, input.NamespaceID, input.CategoryID, createAttributeOptions(input.Attributes))
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithHint("create security attributes", err, "verify namespace_id and category_id; requires permission on a Premium or Ultimate namespace")
	}
	return attributeNodesOutput(attributes)
}

// Update updates a GitLab security attribute.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validatePositiveID(input.AttributeID, "attribute_id"); err != nil {
		return Output{}, err
	}
	if input.Name == nil && input.Description == nil && input.Color == nil {
		return Output{}, errors.New("update security attribute: provide at least one of name, description, or color")
	}
	if input.Color != nil {
		if err := validateHexColor(*input.Color, "color"); err != nil {
			return Output{}, err
		}
	}

	opts := &gl.UpdateSecurityAttributeOptions{
		Description: optionalText(input.Description),
		Color:       optionalText(input.Color),
	}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Output{}, toolutil.ErrFieldRequired("name")
		}
		opts.Name = &name
	}
	attribute, err := updateSecurityAttribute(ctx, client, input.AttributeID, opts)
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("update security attribute", err, "verify attribute_id; only editable custom attributes can be updated")
	}
	return attributeNodeOutput(attribute)
}

// Delete deletes a GitLab security attribute.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	if err := validatePositiveID(input.AttributeID, "attribute_id"); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	if err := destroySecurityAttribute(ctx, client, input.AttributeID); err != nil {
		return toolutil.DeleteOutput{}, toolutil.WrapErrWithHint("delete security attribute", err, "verify attribute_id; only editable custom attributes can be deleted")
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted security attribute %d.", input.AttributeID),
	}, nil
}

// ProjectUpdate adds or removes security attributes on a GitLab project.
func ProjectUpdate(ctx context.Context, client *gitlabclient.Client, input ProjectUpdateInput) (ProjectUpdateOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectUpdateOutput{}, err
	}
	if err := validatePositiveID(input.ProjectID, "project_id"); err != nil {
		return ProjectUpdateOutput{}, err
	}
	if len(input.AddAttributeIDs) == 0 && len(input.RemoveAttributeIDs) == 0 {
		return ProjectUpdateOutput{}, errors.New("update project security attributes: provide add_attribute_ids or remove_attribute_ids")
	}
	if len(input.AddAttributeIDs) > 0 {
		if err := validateIDList(input.AddAttributeIDs, "add_attribute_ids"); err != nil {
			return ProjectUpdateOutput{}, err
		}
	}
	if len(input.RemoveAttributeIDs) > 0 {
		if err := validateIDList(input.RemoveAttributeIDs, "remove_attribute_ids"); err != nil {
			return ProjectUpdateOutput{}, err
		}
	}

	opts := &gl.ProjectUpdateSecurityAttributeOptions{}
	if len(input.AddAttributeIDs) > 0 {
		opts.AddAttributeIDs = &input.AddAttributeIDs
	}
	if len(input.RemoveAttributeIDs) > 0 {
		opts.RemoveAttributeIDs = &input.RemoveAttributeIDs
	}
	result, err := projectUpdateSecurityAttribute(ctx, client, input.ProjectID, opts)
	if err != nil {
		return ProjectUpdateOutput{}, toolutil.WrapErrWithHint("update project security attributes", err, "verify project_id and attribute IDs; requires permission on the project and Premium or Ultimate")
	}
	return result, nil
}

// BulkUpdate adds, removes, or replaces security attributes on groups and projects.
func BulkUpdate(ctx context.Context, client *gitlabclient.Client, input BulkUpdateInput) (BulkUpdateOutput, error) {
	if err := ctx.Err(); err != nil {
		return BulkUpdateOutput{}, err
	}
	if len(input.GroupIDs) == 0 && len(input.ProjectIDs) == 0 {
		return BulkUpdateOutput{}, errors.New("bulk update security attributes: provide group_ids or project_ids")
	}
	if len(input.GroupIDs) > 0 {
		if err := validateIDList(input.GroupIDs, "group_ids"); err != nil {
			return BulkUpdateOutput{}, err
		}
	}
	if len(input.ProjectIDs) > 0 {
		if err := validateIDList(input.ProjectIDs, "project_ids"); err != nil {
			return BulkUpdateOutput{}, err
		}
	}
	if err := validateIDList(input.AttributeIDs, "attribute_ids"); err != nil {
		return BulkUpdateOutput{}, err
	}
	if err := validateBulkMode(input.Mode); err != nil {
		return BulkUpdateOutput{}, err
	}

	mode := gl.SecurityAttributeBulkUpdateMode(input.Mode)
	opts := &gl.BulkUpdateSecurityAttributesOptions{
		AttributeIDs: &input.AttributeIDs,
		Mode:         &mode,
	}
	if len(input.GroupIDs) > 0 {
		opts.GroupIDs = &input.GroupIDs
	}
	if len(input.ProjectIDs) > 0 {
		opts.ProjectIDs = &input.ProjectIDs
	}
	if err := bulkUpdateSecurityAttributes(ctx, client, opts); err != nil {
		return BulkUpdateOutput{}, toolutil.WrapErrWithHint("bulk update security attributes", err, "verify group_ids, project_ids, attribute_ids, and mode; requires Premium or Ultimate")
	}
	return BulkUpdateOutput{
		Status:       "success",
		Message:      "Successfully updated security attributes in bulk.",
		Mode:         input.Mode,
		GroupIDs:     input.GroupIDs,
		ProjectIDs:   input.ProjectIDs,
		AttributeIDs: input.AttributeIDs,
	}, nil
}

func createSecurityAttributes(ctx context.Context, client *gitlabclient.Client, namespaceID, categoryID int64, opts *gl.CreateSecurityAttributesOptions) ([]*attributeNode, error) {
	attributes := make([]map[string]any, 0, len(*opts.Attributes))
	for _, attribute := range *opts.Attributes {
		attributes = append(attributes, map[string]any{
			"name":        attribute.Name,
			"description": attribute.Description,
			"color":       attribute.Color,
		})
	}
	query := gl.GraphQLQuery{
		Query: securityAttributeCreateMutation,
		Variables: map[string]any{"input": map[string]any{
			"namespaceId": toolutil.FormatGID(namespaceGIDType, namespaceID),
			"categoryId":  toolutil.FormatGID(securityCategoryGIDType, categoryID),
			"attributes":  attributes,
		}},
	}
	var result securityAttributeCreateResponse
	_, err := client.GL().GraphQL.Do(query, &result, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("securityAttributeCreate", result.Errors); topLevelErr != nil {
		return nil, topLevelErr
	}
	payload := result.Data.SecurityAttributeCreate
	if payload == nil {
		return nil, gl.ErrNotFound
	}
	if mutationErr := toolutil.GraphQLMutationError("securityAttributeCreate", payload.Errors); mutationErr != nil {
		return nil, mutationErr
	}
	if len(payload.SecurityAttributes) == 0 {
		return nil, gl.ErrNotFound
	}
	return payload.SecurityAttributes, nil
}

func updateSecurityAttribute(ctx context.Context, client *gitlabclient.Client, attributeID int64, opts *gl.UpdateSecurityAttributeOptions) (*attributeNode, error) {
	input := map[string]any{"id": toolutil.FormatGID(securityAttributeGIDType, attributeID)}
	if opts.Name != nil {
		input["name"] = opts.Name
	}
	if opts.Description != nil {
		input["description"] = opts.Description
	}
	if opts.Color != nil {
		input["color"] = opts.Color
	}
	var result securityAttributeUpdateResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: securityAttributeUpdateMutation, Variables: map[string]any{"input": input}}, &result, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("securityAttributeUpdate", result.Errors); topLevelErr != nil {
		return nil, topLevelErr
	}
	payload := result.Data.SecurityAttributeUpdate
	if payload == nil {
		return nil, gl.ErrNotFound
	}
	if mutationErr := toolutil.GraphQLMutationError("securityAttributeUpdate", payload.Errors); mutationErr != nil {
		return nil, mutationErr
	}
	if payload.SecurityAttribute == nil {
		return nil, gl.ErrNotFound
	}
	return payload.SecurityAttribute, nil
}

func destroySecurityAttribute(ctx context.Context, client *gitlabclient.Client, attributeID int64) error {
	var result securityAttributeDestroyResponse
	query := gl.GraphQLQuery{
		Query:     securityAttributeDestroyMutation,
		Variables: map[string]any{"input": map[string]any{"id": toolutil.FormatGID(securityAttributeGIDType, attributeID)}},
	}
	_, err := client.GL().GraphQL.Do(query, &result, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("securityAttributeDestroy", result.Errors); topLevelErr != nil {
		return topLevelErr
	}
	payload := result.Data.SecurityAttributeDestroy
	if payload == nil {
		return gl.ErrNotFound
	}
	return toolutil.GraphQLMutationError("securityAttributeDestroy", payload.Errors)
}

func projectUpdateSecurityAttribute(ctx context.Context, client *gitlabclient.Client, projectID int64, opts *gl.ProjectUpdateSecurityAttributeOptions) (ProjectUpdateOutput, error) {
	input := map[string]any{"projectId": toolutil.FormatGID(projectGIDType, projectID)}
	if opts.AddAttributeIDs != nil {
		input["addAttributeIds"] = formatGIDs(securityAttributeGIDType, *opts.AddAttributeIDs)
	}
	if opts.RemoveAttributeIDs != nil {
		input["removeAttributeIds"] = formatGIDs(securityAttributeGIDType, *opts.RemoveAttributeIDs)
	}
	var result securityAttributeProjectUpdateResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: securityAttributeProjectUpdateMutation, Variables: map[string]any{"input": input}}, &result, gl.WithContext(ctx))
	if err != nil {
		return ProjectUpdateOutput{}, err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("securityAttributeProjectUpdate", result.Errors); topLevelErr != nil {
		return ProjectUpdateOutput{}, topLevelErr
	}
	projectResult := result.Data.SecurityAttributeProjectUpdate
	if projectResult == nil {
		return ProjectUpdateOutput{}, gl.ErrNotFound
	}
	if mutationErr := toolutil.GraphQLMutationError("securityAttributeProjectUpdate", projectResult.Errors); mutationErr != nil {
		return ProjectUpdateOutput{}, mutationErr
	}
	return ProjectUpdateOutput{AddedCount: projectResult.AddedCount, RemovedCount: projectResult.RemovedCount}, nil
}

func bulkUpdateSecurityAttributes(ctx context.Context, client *gitlabclient.Client, opts *gl.BulkUpdateSecurityAttributesOptions) error {
	if opts.AttributeIDs == nil || len(*opts.AttributeIDs) == 0 {
		return toolutil.ErrFieldRequired("attribute_ids")
	}
	if opts.Mode == nil {
		return toolutil.ErrFieldRequired("mode")
	}
	itemsLen := 0
	if opts.GroupIDs != nil {
		itemsLen += len(*opts.GroupIDs)
	}
	if opts.ProjectIDs != nil {
		itemsLen += len(*opts.ProjectIDs)
	}
	items := make([]string, 0, itemsLen)
	if opts.GroupIDs != nil {
		items = append(items, formatGIDs(groupGIDType, *opts.GroupIDs)...)
	}
	if opts.ProjectIDs != nil {
		items = append(items, formatGIDs(projectGIDType, *opts.ProjectIDs)...)
	}
	input := map[string]any{
		"items":      items,
		"attributes": formatGIDs(securityAttributeGIDType, *opts.AttributeIDs),
		"mode":       *opts.Mode,
	}
	var result securityAttributeBulkUpdateResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: securityAttributeBulkUpdateMutation, Variables: map[string]any{"input": input}}, &result, gl.WithContext(ctx))
	if err != nil {
		return err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("bulkUpdateSecurityAttributes", result.Errors); topLevelErr != nil {
		return topLevelErr
	}
	payload := result.Data.BulkUpdateSecurityAttributes
	if payload == nil {
		return gl.ErrNotFound
	}
	return toolutil.GraphQLMutationError("bulkUpdateSecurityAttributes", payload.Errors)
}

func formatGIDs(typeName string, values []int64) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, toolutil.FormatGID(typeName, value))
	}
	return out
}
