package securitycategories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The operations the two writes that answer with a category name in their
// errors, and the category in a reader's words, for the answer a fine-grained
// session gets when GitLab ran the write and answered without it.
const (
	opCreateCategory = "create security category"
	opUpdateCategory = "update security category"
	objectCategory   = "security category"
)

const (
	namespaceGIDType        = "Namespace"
	securityCategoryGIDType = "Security::Category"

	securityCategoryCreateMutation = `
		mutation CreateSecurityCategory($input: SecurityCategoryCreateInput!) {
			securityCategoryCreate(input: $input) {
				securityCategory {
					id
					name
					description
					multipleSelection
					editableState
					templateType
					securityAttributes {
						id
						name
						color
						description
						editableState
					}
				}
				errors
			}
		}`
	securityCategoryUpdateMutation = `
		mutation UpdateSecurityCategory($input: SecurityCategoryUpdateInput!) {
			securityCategoryUpdate(input: $input) {
				securityCategory {
					id
					name
					description
					multipleSelection
					editableState
					templateType
					securityAttributes {
						id
						name
						color
						description
						editableState
					}
				}
				errors
			}
		}`
	// securityCategoryDestroyMutation asks for the attributes GitLab deleted
	// with the category, which nothing else tells a caller. The category's
	// own id is the one the caller sent, and is not asked back.
	securityCategoryDestroyMutation = `
		mutation DestroySecurityCategory($input: SecurityCategoryDestroyInput!) {
			securityCategoryDestroy(input: $input) {
				deletedAttributesGid
				errors
			}
		}`
)

// CreateInput defines parameters for creating a security category.
type CreateInput struct {
	NamespaceID       int64   `json:"namespace_id"                  jsonschema:"Numeric namespace ID,required"`
	Name              string  `json:"name"                          jsonschema:"Security category name,required"`
	Description       *string `json:"description,omitempty"         jsonschema:"Security category description"`
	MultipleSelection *bool   `json:"multiple_selection,omitempty"  jsonschema:"Whether multiple attributes can be selected for the category"`
}

// UpdateInput defines parameters for updating a security category.
type UpdateInput struct {
	CategoryID  int64   `json:"category_id"            jsonschema:"Numeric security category ID,required"`
	NamespaceID int64   `json:"namespace_id"           jsonschema:"Numeric namespace ID,required"`
	Name        *string `json:"name,omitempty"         jsonschema:"New security category name"`
	Description *string `json:"description,omitempty"  jsonschema:"New security category description"`
}

// DeleteInput defines parameters for deleting a security category.
type DeleteInput struct {
	CategoryID int64 `json:"category_id" jsonschema:"Numeric security category ID,required"`
}

// AttributeSummary represents a security attribute nested in a category response.
type AttributeSummary struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Color         string `json:"color"`
	Description   string `json:"description,omitempty"`
	EditableState string `json:"editable_state,omitempty"`
}

// DeleteOutput confirms a deleted security category and names the security
// attributes GitLab deleted with it: a category takes its attributes along, so
// an attribute ID the caller still holds may no longer exist.
type DeleteOutput struct {
	toolutil.HintableOutput
	Status              string  `json:"status"`
	Message             string  `json:"message"`
	DeletedAttributeIDs []int64 `json:"deleted_attribute_ids"`
}

// Output represents a GitLab security category.
type Output struct {
	toolutil.HintableOutput
	ID                 int64              `json:"id"`
	Name               string             `json:"name"`
	Description        string             `json:"description,omitempty"`
	MultipleSelection  bool               `json:"multiple_selection"`
	EditableState      string             `json:"editable_state,omitempty"`
	TemplateType       string             `json:"template_type,omitempty"`
	SecurityAttributes []AttributeSummary `json:"security_attributes,omitempty"`
}

type categoryNode struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        *string         `json:"description"`
	MultipleSelection  bool            `json:"multipleSelection"`
	EditableState      string          `json:"editableState"`
	TemplateType       *string         `json:"templateType"`
	SecurityAttributes []attributeNode `json:"securityAttributes"`
}

type attributeNode struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Color         string `json:"color"`
	Description   string `json:"description"`
	EditableState string `json:"editableState"`
}

type categoryMutationEnvelope struct {
	Errors []toolutil.GraphQLError `json:"errors"`
}

// categoryMutationPayload is the payload the create and the update answer
// with: the category, and the errors of a write GitLab ran and refused to
// keep.
type categoryMutationPayload struct {
	SecurityCategory *categoryNode `json:"securityCategory"`
	Errors           []string      `json:"errors"`
}

type securityCategoryCreateData struct {
	SecurityCategoryCreate *categoryMutationPayload `json:"securityCategoryCreate"`
}

type securityCategoryCreateResponse struct {
	Data securityCategoryCreateData `json:"data"`
	categoryMutationEnvelope
}

type securityCategoryUpdateData struct {
	SecurityCategoryUpdate *categoryMutationPayload `json:"securityCategoryUpdate"`
}

type securityCategoryUpdateResponse struct {
	Data securityCategoryUpdateData `json:"data"`
	categoryMutationEnvelope
}

type securityCategoryDestroyPayload struct {
	DeletedAttributesGID []string `json:"deletedAttributesGid"`
	Errors               []string `json:"errors"`
}

type securityCategoryDestroyData struct {
	SecurityCategoryDestroy *securityCategoryDestroyPayload `json:"securityCategoryDestroy"`
}

type securityCategoryDestroyResponse struct {
	Data securityCategoryDestroyData `json:"data"`
	categoryMutationEnvelope
}

func categoryNodeOutput(category *categoryNode) (Output, error) {
	if category == nil {
		return Output{}, nil
	}
	_, id, err := toolutil.ParseGID(category.ID)
	if err != nil {
		return Output{}, fmt.Errorf("parse security category id: %w", err)
	}
	out := Output{
		ID:                 id,
		Name:               category.Name,
		MultipleSelection:  category.MultipleSelection,
		EditableState:      category.EditableState,
		SecurityAttributes: make([]AttributeSummary, 0, len(category.SecurityAttributes)),
	}
	if category.Description != nil {
		out.Description = *category.Description
	}
	if category.TemplateType != nil {
		out.TemplateType = *category.TemplateType
	}
	for _, attribute := range category.SecurityAttributes {
		mapped, attributeErr := attributeNodeSummary(attribute)
		if attributeErr != nil {
			return Output{}, attributeErr
		}
		out.SecurityAttributes = append(out.SecurityAttributes, mapped)
	}
	return out, nil
}

func attributeNodeSummary(attribute attributeNode) (AttributeSummary, error) {
	_, id, err := toolutil.ParseGID(attribute.ID)
	if err != nil {
		return AttributeSummary{}, fmt.Errorf("parse security attribute id: %w", err)
	}
	return AttributeSummary{
		ID:            id,
		Name:          attribute.Name,
		Color:         attribute.Color,
		Description:   attribute.Description,
		EditableState: attribute.EditableState,
	}, nil
}

func optionalText(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := toolutil.NormalizeText(*value)
	return &normalized
}

func validateNamespaceID(namespaceID int64) error {
	if namespaceID <= 0 {
		return errors.New("namespace_id must be greater than 0")
	}
	return nil
}

func validatePositiveID(value int64, field string) error {
	if value <= 0 {
		return fmt.Errorf("%s must be greater than 0", field)
	}
	return nil
}

// Create creates a GitLab security category.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validateNamespaceID(input.NamespaceID); err != nil {
		return Output{}, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Output{}, toolutil.ErrFieldRequired("name")
	}

	opts := &gl.CreateSecurityCategoryOptions{
		Name:              name,
		Description:       optionalText(input.Description),
		MultipleSelection: input.MultipleSelection,
	}
	category, err := createSecurityCategory(ctx, client, input.NamespaceID, opts)
	if errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		return Output{}, err
	}
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint(opCreateCategory, err, "verify namespace_id and that the token has permission on a Premium or Ultimate namespace")
	}
	return categoryNodeOutput(category)
}

// Update updates a GitLab security category.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if err := validatePositiveID(input.CategoryID, "category_id"); err != nil {
		return Output{}, err
	}
	if err := validateNamespaceID(input.NamespaceID); err != nil {
		return Output{}, err
	}
	if input.Name == nil && input.Description == nil {
		return Output{}, errors.New("update security category: provide at least one of name or description")
	}

	opts := &gl.UpdateSecurityCategoryOptions{Description: optionalText(input.Description)}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Output{}, toolutil.ErrFieldRequired("name")
		}
		opts.Name = &name
	}
	category, err := updateSecurityCategory(ctx, client, input.CategoryID, input.NamespaceID, opts)
	if errors.Is(err, finegrained.ErrUnconfirmedWrite) {
		return Output{}, err
	}
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint(opUpdateCategory, err, "verify category_id and namespace_id; only editable custom categories can be updated")
	}
	return categoryNodeOutput(category)
}

// Delete deletes a GitLab security category and its associated attributes,
// and reports the attributes GitLab deleted with it.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	if err := validatePositiveID(input.CategoryID, "category_id"); err != nil {
		return DeleteOutput{}, err
	}
	deleted, err := destroySecurityCategory(ctx, client, input.CategoryID)
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithHint("delete security category", err, "verify category_id; deleting a category also deletes its associated security attributes")
	}
	attributeIDs, err := attributeIDsFromGIDs(deleted)
	if err != nil {
		return DeleteOutput{}, err
	}
	return DeleteOutput{
		Status:              "success",
		Message:             fmt.Sprintf("Successfully deleted security category %d and its attributes.", input.CategoryID),
		DeletedAttributeIDs: attributeIDs,
	}, nil
}

// attributeIDsFromGIDs reads the numeric ids out of the attribute global ids
// GitLab reports deleted, as an empty list rather than nil when there are
// none, so a category that held no attribute reads as having taken none along.
func attributeIDsFromGIDs(gids []string) ([]int64, error) {
	ids := make([]int64, 0, len(gids))
	for _, gid := range gids {
		_, id, err := toolutil.ParseGID(gid)
		if err != nil {
			return nil, fmt.Errorf("parse deleted security attribute id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func createSecurityCategory(ctx context.Context, client *gitlabclient.Client, namespaceID int64, opts *gl.CreateSecurityCategoryOptions) (*categoryNode, error) {
	input := map[string]any{
		"namespaceId": toolutil.FormatGID(namespaceGIDType, namespaceID),
		"name":        opts.Name,
	}
	if opts.Description != nil {
		input["description"] = opts.Description
	}
	if opts.MultipleSelection != nil {
		input["multipleSelection"] = opts.MultipleSelection
	}
	var result securityCategoryCreateResponse
	resp, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: securityCategoryCreateMutation, Variables: map[string]any{"input": input}}, &result, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return categoryWritten(client, resp, opCreateCategory, "securityCategoryCreate", result.Errors, result.Data.SecurityCategoryCreate)
}

// categoryWritten reads what a category write answered: GitLab's refusal,
// the payload's own errors, or the category. A write GitLab ran and answered
// without the category, null with no error or nulled below by its attributes
// with the errors that null writes, is one a fine-grained session is told
// probably committed (issue 1103): GitLab checks the category and its
// attributes against a fine-grained token only after the write ran, and at
// 19.4.1 neither type declares a fine-grained permission.
func categoryWritten(client *gitlabclient.Client, resp *gl.Response, operation, field string, responseErrors []toolutil.GraphQLError, payload *categoryMutationPayload) (*categoryNode, error) {
	if topLevelErr := toolutil.GraphQLTopLevelError(field, responseErrors); topLevelErr != nil {
		return nil, toolutil.UnconfirmedWrite(client, resp, operation, objectCategory, topLevelErr, topLevelErr)
	}
	if payload == nil {
		return nil, gl.ErrNotFound
	}
	if mutationErr := toolutil.GraphQLMutationError(field, payload.Errors); mutationErr != nil {
		return nil, mutationErr
	}
	if payload.SecurityCategory == nil {
		return nil, client.Authority().UnconfirmedWrite(operation, objectCategory, gl.ErrNotFound)
	}
	return payload.SecurityCategory, nil
}

func updateSecurityCategory(ctx context.Context, client *gitlabclient.Client, categoryID, namespaceID int64, opts *gl.UpdateSecurityCategoryOptions) (*categoryNode, error) {
	input := map[string]any{
		"id":          toolutil.FormatGID(securityCategoryGIDType, categoryID),
		"namespaceId": toolutil.FormatGID(namespaceGIDType, namespaceID),
	}
	if opts.Name != nil {
		input["name"] = opts.Name
	}
	if opts.Description != nil {
		input["description"] = opts.Description
	}
	var result securityCategoryUpdateResponse
	resp, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: securityCategoryUpdateMutation, Variables: map[string]any{"input": input}}, &result, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return categoryWritten(client, resp, opUpdateCategory, "securityCategoryUpdate", result.Errors, result.Data.SecurityCategoryUpdate)
}

// destroySecurityCategory deletes one category and returns the global ids of
// the attributes GitLab deleted with it.
func destroySecurityCategory(ctx context.Context, client *gitlabclient.Client, categoryID int64) ([]string, error) {
	var result securityCategoryDestroyResponse
	query := gl.GraphQLQuery{
		Query:     securityCategoryDestroyMutation,
		Variables: map[string]any{"input": map[string]any{"id": toolutil.FormatGID(securityCategoryGIDType, categoryID)}},
	}
	_, err := client.GL().GraphQL.Do(query, &result, gl.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	if topLevelErr := toolutil.GraphQLTopLevelError("securityCategoryDestroy", result.Errors); topLevelErr != nil {
		return nil, topLevelErr
	}
	payload := result.Data.SecurityCategoryDestroy
	if payload == nil {
		return nil, gl.ErrNotFound
	}
	if mutationErr := toolutil.GraphQLMutationError("securityCategoryDestroy", payload.Errors); mutationErr != nil {
		return nil, mutationErr
	}
	return payload.DeletedAttributesGID, nil
}
