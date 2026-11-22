// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	securityCategoryGIDType = "Security::Category"

	// securityCategoryDestroyMutation asks for the attributes GitLab deleted
	// with the category, which nothing else tells a caller. The category's
	// own id is the one the caller sent, and is not asked back. It stays the
	// handler's own document, and not client-go's DestroySecurityCategory,
	// until that method selects deletedAttributesGid (issue 1784).
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

type categoryMutationEnvelope struct {
	Errors []toolutil.GraphQLError `json:"errors"`
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

// categoryOutput converts the category client-go decoded, which has already
// read its global ids as numbers, and nil as the zero output. Its description
// and template type are nullable in the schema and read as the empty string
// when GitLab answers null.
func categoryOutput(category *gl.SecurityCategory) Output {
	if category == nil {
		return Output{}
	}
	out := Output{
		ID:                 category.ID,
		Name:               category.Name,
		MultipleSelection:  category.MultipleSelection,
		EditableState:      string(category.EditableState),
		SecurityAttributes: make([]AttributeSummary, 0, len(category.SecurityAttributes)),
	}
	if category.Description != nil {
		out.Description = *category.Description
	}
	if category.TemplateType != nil {
		out.TemplateType = string(*category.TemplateType)
	}
	// client-go builds every attribute of the list it decoded, so none is nil:
	// GitLab nulls the whole list rather than one item of it, its items being
	// non-null.
	for _, attribute := range category.SecurityAttributes {
		out.SecurityAttributes = append(out.SecurityAttributes, AttributeSummary{
			ID:            attribute.ID,
			Name:          attribute.Name,
			Color:         attribute.Color,
			Description:   attribute.Description,
			EditableState: string(attribute.EditableState),
		})
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
	return categoryOutput(category), nil
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
	return categoryOutput(category), nil
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

// The create and the update are sent through client-go's SecurityCategories
// service (issue 1784). It answers GitLab's refusal as a *gl.GraphQLResponseError
// that also matches its not-found sentinel, which toolutil reads by its
// messages rather than as a 404, the payload's own errors as an error of its
// own, and a category answered null with no error as the sentinel itself.
//
// A write GitLab ran and answered without the category, null with no error or
// nulled below by its attributes with the errors that null writes, is one a
// fine-grained session is told probably committed (issue 1103): GitLab checks
// the category and its attributes against a fine-grained token only after the
// write ran, and at 19.4.1 neither type declares a fine-grained permission.

func createSecurityCategory(ctx context.Context, client *gitlabclient.Client, namespaceID int64, opts *gl.CreateSecurityCategoryOptions) (*gl.SecurityCategory, error) {
	category, resp, err := client.GL().SecurityCategories.CreateSecurityCategory(namespaceID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.UnconfirmedWrite(client, resp, opCreateCategory, objectCategory, err, err)
	}
	return category, nil
}

func updateSecurityCategory(ctx context.Context, client *gitlabclient.Client, categoryID, namespaceID int64, opts *gl.UpdateSecurityCategoryOptions) (*gl.SecurityCategory, error) {
	category, resp, err := client.GL().SecurityCategories.UpdateSecurityCategory(categoryID, namespaceID, opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.UnconfirmedWrite(client, resp, opUpdateCategory, objectCategory, err, err)
	}
	return category, nil
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
