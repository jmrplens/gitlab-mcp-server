// Package customattributes implements MCP tools for GitLab Custom Attributes API.
package customattributes

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

var validResourceTypes = []string{"user", "group", "project"}

// AttributeItem represents a single custom attribute.
type AttributeItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing custom attributes.
type ListInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   int64  `json:"resource_id" jsonschema:"ID of the resource,required"`
}

// ListOutput is the output for listing custom attributes.
type ListOutput struct {
	toolutil.HintableOutput
	Attributes []AttributeItem `json:"attributes"`
}

// List lists custom attributes for a resource (admin).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ResourceID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("list_custom_attributes", "resource_id")
	}

	var attrs []*gl.CustomAttribute
	var err error

	switch input.ResourceType {
	case "user":
		attrs, _, err = client.GL().CustomAttribute.ListCustomUserAttributes(input.ResourceID, gl.WithContext(ctx))
	case "group":
		attrs, _, err = client.GL().CustomAttribute.ListCustomGroupAttributes(input.ResourceID, gl.WithContext(ctx))
	case "project":
		attrs, _, err = client.GL().CustomAttribute.ListCustomProjectAttributes(input.ResourceID, gl.WithContext(ctx))
	default:
		return ListOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_custom_attributes", err, http.StatusNotFound, "verify resource_type (user, group, project) and resource_id \u2014 requires admin access")
	}

	items := make([]AttributeItem, 0, len(attrs))
	for _, a := range attrs {
		items = append(items, AttributeItem{Key: a.Key, Value: a.Value})
	}
	return ListOutput{Attributes: items}, nil
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------.

// GetInput is the input for getting a custom attribute.
type GetInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   int64  `json:"resource_id" jsonschema:"ID of the resource,required"`
	Key          string `json:"key" jsonschema:"Attribute key,required"`
}

// GetOutput is the output for getting a custom attribute.
type GetOutput struct {
	toolutil.HintableOutput
	AttributeItem
}

// Get gets a single custom attribute by key (admin).
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ResourceID <= 0 {
		return GetOutput{}, toolutil.ErrRequiredInt64("get_custom_attribute", "resource_id")
	}

	var attr *gl.CustomAttribute
	var err error

	switch input.ResourceType {
	case "user":
		attr, _, err = client.GL().CustomAttribute.GetCustomUserAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	case "group":
		attr, _, err = client.GL().CustomAttribute.GetCustomGroupAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	case "project":
		attr, _, err = client.GL().CustomAttribute.GetCustomProjectAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	default:
		return GetOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_custom_attribute", err, http.StatusNotFound, "verify resource_type, resource_id, and key \u2014 requires admin access")
	}
	return GetOutput{AttributeItem: AttributeItem{Key: attr.Key, Value: attr.Value}}, nil
}

// ---------------------------------------------------------------------------
// Set
// ---------------------------------------------------------------------------.

// SetInput is the input for setting a custom attribute.
type SetInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   int64  `json:"resource_id" jsonschema:"ID of the resource,required"`
	Key          string `json:"key" jsonschema:"Attribute key,required"`
	Value        string `json:"value" jsonschema:"Attribute value,required"`
}

// SetOutput is the output for setting a custom attribute.
type SetOutput struct {
	toolutil.HintableOutput
	AttributeItem
}

// Set sets (creates or updates) a custom attribute (admin).
func Set(ctx context.Context, client *gitlabclient.Client, input SetInput) (SetOutput, error) {
	if input.ResourceID <= 0 {
		return SetOutput{}, toolutil.ErrRequiredInt64("set_custom_attribute", "resource_id")
	}

	ca := gl.CustomAttribute{Key: input.Key, Value: input.Value}
	var attr *gl.CustomAttribute
	var err error

	switch input.ResourceType {
	case "user":
		attr, _, err = client.GL().CustomAttribute.SetCustomUserAttribute(input.ResourceID, ca, gl.WithContext(ctx))
	case "group":
		attr, _, err = client.GL().CustomAttribute.SetCustomGroupAttribute(input.ResourceID, ca, gl.WithContext(ctx))
	case "project":
		attr, _, err = client.GL().CustomAttribute.SetCustomProjectAttribute(input.ResourceID, ca, gl.WithContext(ctx))
	default:
		return SetOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return SetOutput{}, toolutil.WrapErrWithStatusHint("set_custom_attribute", err, http.StatusNotFound, "verify resource_type and resource_id \u2014 requires admin access")
	}
	return SetOutput{AttributeItem: AttributeItem{Key: attr.Key, Value: attr.Value}}, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// DeleteInput is the input for deleting a custom attribute.
type DeleteInput struct {
	ResourceType string `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   int64  `json:"resource_id" jsonschema:"ID of the resource,required"`
	Key          string `json:"key" jsonschema:"Attribute key to delete,required"`
}

// Delete deletes a custom attribute (admin).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.ResourceID <= 0 {
		return toolutil.ErrRequiredInt64("delete_custom_attribute", "resource_id")
	}

	var err error

	switch input.ResourceType {
	case "user":
		_, err = client.GL().CustomAttribute.DeleteCustomUserAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	case "group":
		_, err = client.GL().CustomAttribute.DeleteCustomGroupAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	case "project":
		_, err = client.GL().CustomAttribute.DeleteCustomProjectAttribute(input.ResourceID, input.Key, gl.WithContext(ctx))
	default:
		return toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_custom_attribute", err, http.StatusNotFound, "verify resource_type, resource_id, and key \u2014 requires admin access")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
