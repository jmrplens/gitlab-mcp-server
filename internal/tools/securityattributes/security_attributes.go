// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package securityattributes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const hexColorSchemaPattern = `^#[0-9A-Fa-f]{6}$`

// The operations the two writes that answer with attributes name in their
// errors, and what each returns in a reader's words, for the answer a
// fine-grained session gets when GitLab ran the write and answered without
// it.
const (
	opCreateAttributes = "create security attributes"
	opUpdateAttribute  = "update security attribute"
	objectAttributes   = "security attributes"
	objectAttribute    = "security attribute"
)

var hexColorPattern = regexp.MustCompile(hexColorSchemaPattern)

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

// BulkUpdateOutput confirms GitLab accepted a bulk security attribute update:
// Status is "queued", since the update runs in the background after GitLab
// answers.
type BulkUpdateOutput struct {
	toolutil.HintableOutput
	Status       string         `json:"status"`
	Message      string         `json:"message"`
	Mode         BulkUpdateMode `json:"mode"`
	GroupIDs     []int64        `json:"group_ids,omitempty"`
	ProjectIDs   []int64        `json:"project_ids,omitempty"`
	AttributeIDs []int64        `json:"attribute_ids"`
}

// attributeOutput converts one attribute client-go decoded, which has already
// read its global ids as numbers, and nil as the zero output.
func attributeOutput(attribute *gl.SecurityAttribute) Output {
	if attribute == nil {
		return Output{}
	}
	out := Output{
		ID:            attribute.ID,
		Name:          attribute.Name,
		Color:         attribute.Color,
		Description:   attribute.Description,
		EditableState: string(attribute.EditableState),
	}
	if attribute.SecurityCategory != nil {
		out.SecurityCategory = categorySummary(attribute.SecurityCategory)
	}
	return out
}

// categorySummary converts the category an attribute belongs to. Its
// description and template type are nullable in the schema, and a category
// GitLab created from no template answers null for both, which reads as the
// empty string.
func categorySummary(category *gl.SecurityCategory) *CategorySummary {
	summary := &CategorySummary{
		ID:                category.ID,
		Name:              category.Name,
		MultipleSelection: category.MultipleSelection,
		EditableState:     string(category.EditableState),
	}
	if category.Description != nil {
		summary.Description = *category.Description
	}
	if category.TemplateType != nil {
		summary.TemplateType = string(*category.TemplateType)
	}
	return summary
}

// attributesOutput converts the attributes a creation answered with.
func attributesOutput(attributes []*gl.SecurityAttribute) CreateOutput {
	out := CreateOutput{Attributes: make([]Output, 0, len(attributes))}
	for _, attribute := range attributes {
		out.Attributes = append(out.Attributes, attributeOutput(attribute))
	}
	return out
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

// attributeInputs converts the attributes a caller asked to create into the
// inputs client-go sends, each value trimmed or normalized.
func attributeInputs(attributes []AttributeInput) *[]gl.SecurityAttributeInput {
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
	return &inputs
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

	opts := &gl.CreateSecurityAttributesOptions{Attributes: attributeInputs(input.Attributes)}
	attributes, err := createSecurityAttributes(ctx, client, input.NamespaceID, input.CategoryID, opts)
	if errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		return CreateOutput{}, err
	}
	if err != nil {
		return CreateOutput{}, toolutil.WrapErrWithHint(opCreateAttributes, err, "verify namespace_id and category_id; requires permission on a Premium or Ultimate namespace")
	}
	return attributesOutput(attributes), nil
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
	if errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		return Output{}, err
	}
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint(opUpdateAttribute, err, "verify attribute_id; only editable custom attributes can be updated")
	}
	return attributeOutput(attribute), nil
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
	// Both target lists are handed over whether or not the caller named one.
	// client-go's BulkUpdateSecurityAttributes appends the global ids of each,
	// and an empty list contributes none, so a nil pointer and a pointer to an
	// empty list build byte-identical requests. Guarding them here would be a
	// copy of that guard, and a copy whose two answers cannot be told apart is
	// dead code rather than a decision. This is not the shape of ProjectUpdate
	// above, where client-go leaves an empty list out of the input and the
	// handler offers it only a non-empty one.
	opts := &gl.BulkUpdateSecurityAttributesOptions{
		GroupIDs:     &input.GroupIDs,
		ProjectIDs:   &input.ProjectIDs,
		AttributeIDs: &input.AttributeIDs,
		Mode:         &mode,
	}
	if err := bulkUpdateSecurityAttributes(ctx, client, opts); err != nil {
		return BulkUpdateOutput{}, toolutil.WrapErrWithHint("bulk update security attributes", err, "verify group_ids, project_ids, attribute_ids, and mode; requires Premium or Ultimate")
	}
	// GitLab's BulkUpdateService enqueues a worker and answers before any
	// attribute is assigned, so what the answer confirms is that the update
	// was accepted. The worker drops some changes without reporting them (a
	// second attribute in a single-selection category, a project that does
	// not exist), and no action here reads a project's attributes, so the
	// message promises nothing about the outcome.
	return BulkUpdateOutput{
		Status:       "queued",
		Message:      "GitLab queued the update and runs it in the background. It reports nothing for a change it then drops, such as a second attribute in a single-selection category or a project that does not exist, and no action of this server reads a project's attributes back.",
		Mode:         input.Mode,
		GroupIDs:     input.GroupIDs,
		ProjectIDs:   input.ProjectIDs,
		AttributeIDs: input.AttributeIDs,
	}, nil
}

// The five mutations are sent through client-go's SecurityAttributes service
// (issue 1784). It reads the response's top-level errors and returns them as a
// *gl.GraphQLResponseError, and the payload's own errors list as an error of
// its own; the update's refusal also matches the not-found sentinel, which
// toolutil reads by its messages rather than as a 404.

// createSecurityAttributes creates the attributes opts names. The payload
// carries them as a list of non-null items, so an attribute GitLab denies
// after the write ran nulls the list and writes one error per denied
// attribute instead of answering it null where it sits. That answer, and a
// list that came back empty with no error, is a write GitLab ran, which a
// fine-grained session is told (issue 1103).
func createSecurityAttributes(ctx context.Context, client *gitlabclient.Client, namespaceID, categoryID int64, opts *gl.CreateSecurityAttributesOptions) ([]*gl.SecurityAttribute, error) {
	attributes, resp, err := client.GL().SecurityAttributes.CreateSecurityAttributes(namespaceID, categoryID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.UnconfirmedWrite(client, resp, opCreateAttributes, objectAttributes, err, err)
	}
	if len(attributes) == 0 {
		return nil, client.Authority().UnconfirmedWrite(opCreateAttributes, objectAttributes, gl.ErrNotFound)
	}
	return attributes, nil
}

// updateSecurityAttribute updates one attribute. client-go answers an update
// GitLab ran and answered without the attribute with its not-found sentinel,
// and one whose attribute was nulled through its non-null category, by a
// grant that reads the attribute and not its category, with the error that
// null writes: each is a write GitLab ran, which a fine-grained session is
// told (issue 1103).
func updateSecurityAttribute(ctx context.Context, client *gitlabclient.Client, attributeID int64, opts *gl.UpdateSecurityAttributeOptions) (*gl.SecurityAttribute, error) {
	attribute, resp, err := client.GL().SecurityAttributes.UpdateSecurityAttribute(attributeID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.UnconfirmedWrite(client, resp, opUpdateAttribute, objectAttribute, err, err)
	}
	return attribute, nil
}

func destroySecurityAttribute(ctx context.Context, client *gitlabclient.Client, attributeID int64) error {
	_, err := client.GL().SecurityAttributes.DestroySecurityAttribute(attributeID, gl.WithContext(ctx))
	return err
}

func projectUpdateSecurityAttribute(ctx context.Context, client *gitlabclient.Client, projectID int64, opts *gl.ProjectUpdateSecurityAttributeOptions) (ProjectUpdateOutput, error) {
	result, _, err := client.GL().SecurityAttributes.ProjectUpdateSecurityAttribute(projectID, opts, gl.WithContext(ctx))
	if err != nil {
		return ProjectUpdateOutput{}, err
	}
	return ProjectUpdateOutput{AddedCount: result.AddedCount, RemovedCount: result.RemovedCount}, nil
}

func bulkUpdateSecurityAttributes(ctx context.Context, client *gitlabclient.Client, opts *gl.BulkUpdateSecurityAttributesOptions) error {
	if opts.AttributeIDs == nil || len(*opts.AttributeIDs) == 0 {
		return toolutil.ErrFieldRequired("attribute_ids")
	}
	if opts.Mode == nil {
		return toolutil.ErrFieldRequired("mode")
	}
	// The two checks above stay because client-go's method dereferences both
	// options without asking, and a nil one would panic in the handler.
	_, err := client.GL().SecurityAttributes.BulkUpdateSecurityAttributes(opts, gl.WithContext(ctx))
	return err
}
