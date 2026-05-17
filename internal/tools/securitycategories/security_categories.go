package securitycategories

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

func toOutput(category *gl.SecurityCategory) Output {
	if category == nil {
		return Output{}
	}
	out := Output{
		ID:                category.ID,
		Name:              category.Name,
		MultipleSelection: category.MultipleSelection,
		EditableState:     string(category.EditableState),
	}
	if category.Description != nil {
		out.Description = *category.Description
	}
	if category.TemplateType != nil {
		out.TemplateType = string(*category.TemplateType)
	}
	for _, attribute := range category.SecurityAttributes {
		out.SecurityAttributes = append(out.SecurityAttributes, attributeSummary(attribute))
	}
	return out
}

func attributeSummary(attribute *gl.SecurityAttribute) AttributeSummary {
	if attribute == nil {
		return AttributeSummary{}
	}
	return AttributeSummary{
		ID:            attribute.ID,
		Name:          attribute.Name,
		Color:         attribute.Color,
		Description:   attribute.Description,
		EditableState: string(attribute.EditableState),
	}
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
	category, _, err := client.GL().SecurityCategories.CreateSecurityCategory(input.NamespaceID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("create security category", err, "verify namespace_id and that the token has permission on a Premium or Ultimate namespace")
	}
	return toOutput(category), nil
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
	category, _, err := client.GL().SecurityCategories.UpdateSecurityCategory(input.CategoryID, input.NamespaceID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("update security category", err, "verify category_id and namespace_id; only editable custom categories can be updated")
	}
	return toOutput(category), nil
}

// Delete deletes a GitLab security category and its associated attributes.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	if err := validatePositiveID(input.CategoryID, "category_id"); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	if _, err := client.GL().SecurityCategories.DestroySecurityCategory(input.CategoryID, gl.WithContext(ctx)); err != nil {
		return toolutil.DeleteOutput{}, toolutil.WrapErrWithHint("delete security category", err, "verify category_id; deleting a category also deletes its associated security attributes")
	}
	return toolutil.DeleteOutput{
		Status:  "success",
		Message: fmt.Sprintf("Successfully deleted security category %d and its attributes.", input.CategoryID),
	}, nil
}
