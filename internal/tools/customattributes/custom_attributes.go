// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package customattributes

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The resource types the actions take.
const (
	resourceUser    = "user"
	resourceGroup   = "group"
	resourceProject = "project"
)

var validResourceTypes = []string{resourceUser, resourceGroup, resourceProject}

// fieldResourceID is the input field every handler reads the resource from.
const fieldResourceID = "resource_id"

// AttributeItem represents a single custom attribute.
type AttributeItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// resolveResourceID answers the integer client-go's custom attribute methods
// take. GitLab's routes take the ID of the resource, the URL-encoded path of a
// group or a project, or the username of a user, and client-go types the
// parameter as an int64 (docs/development/upstream-bugs.md, row 158), so a value
// that is not a number is read into its ID first. A number sends nothing, and
// one no resource can have is refused here as a missing one.
//
// resource_id is not one of the path identifiers the dispatchers decode by name,
// so the URL-encoded form is decoded here: client-go escapes the path itself.
func resolveResourceID(ctx context.Context, client *gitlabclient.Client, operation, resourceType string, value toolutil.StringOrInt) (int64, error) {
	if value == "" {
		return 0, toolutil.ErrRequiredString(operation, fieldResourceID)
	}
	if id, err := value.Int64(); err == nil {
		if id <= 0 {
			return 0, toolutil.ErrRequiredInt64(operation, fieldResourceID)
		}
		return id, nil
	}
	name := toolutil.PlainPath(string(value))
	switch resourceType {
	case resourceUser:
		return resolveUserID(ctx, client, operation, name)
	case resourceGroup:
		return resolveGroupID(ctx, client, operation, name)
	case resourceProject:
		return resolveProjectID(ctx, client, operation, name)
	}
	return 0, toolutil.ErrInvalidEnum("resource_type", resourceType, validResourceTypes)
}

// resolveUserID reads a username into its user's ID. The list answers an empty
// page for a username nobody has, where the two routes below answer 404.
func resolveUserID(ctx context.Context, client *gitlabclient.Client, operation, username string) (int64, error) {
	//gitlab:request optional: a username is read into its ID only when the caller names the user by it, since client-go's custom attribute methods take an integer
	users, _, err := client.GL().Users.ListUsers(&gl.ListUsersOptions{Username: &username}, gl.WithContext(ctx))
	if err != nil {
		return 0, lookupFailure(operation, resourceUser, username, err)
	}
	if len(users) == 0 {
		return 0, &unresolvedError{resourceType: resourceUser, identifier: username}
	}
	return users[0].ID, nil
}

// resolveGroupID reads a group's path into its ID.
func resolveGroupID(ctx context.Context, client *gitlabclient.Client, operation, path string) (int64, error) {
	//gitlab:request optional: a path is read into its ID only when the caller names the group by it, since client-go's custom attribute methods take an integer
	group, _, err := client.GL().Groups.GetGroup(path, &gl.GetGroupOptions{
		WithProjects: new(false), //nolint:staticcheck // SA1019: the ID is all this read needs, and without it GitLab answers with up to 100 projects as well
	}, gl.WithContext(ctx))
	if err != nil {
		return 0, lookupFailure(operation, resourceGroup, path, err)
	}
	return group.ID, nil
}

// resolveProjectID reads a project's path into its ID.
func resolveProjectID(ctx context.Context, client *gitlabclient.Client, operation, path string) (int64, error) {
	//gitlab:request optional: a path is read into its ID only when the caller names the project by it, since client-go's custom attribute methods take an integer
	project, _, err := client.GL().Projects.GetProject(path, nil, gl.WithContext(ctx))
	if err != nil {
		return 0, lookupFailure(operation, resourceProject, path, err)
	}
	return project.ID, nil
}

// lookupFailure is what a lookup's error comes to: a 404 says the name names
// nothing, which [AnswerUnresolved] answers, and any other refusal is GitLab's
// to explain.
func lookupFailure(operation, resourceType, identifier string, err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
		return &unresolvedError{resourceType: resourceType, identifier: identifier, cause: err}
	}
	return toolutil.WrapErrWithMessage(operation, err)
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------.

// ListInput is the input for listing custom attributes.
type ListInput struct {
	ResourceType string               `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   toolutil.StringOrInt `json:"resource_id" jsonschema:"ID of the resource, the URL-encoded path of a group or a project, or the username of a user,required"`
}

// ListOutput is the output for listing custom attributes.
type ListOutput struct {
	toolutil.HintableOutput
	Attributes []AttributeItem `json:"attributes"`
}

// List lists custom attributes for a resource (admin).
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	id, err := resolveResourceID(ctx, client, "list_custom_attributes", input.ResourceType, input.ResourceID)
	if err != nil {
		return ListOutput{}, err
	}

	var attrs []*gl.CustomAttribute

	switch input.ResourceType {
	case resourceUser:
		attrs, _, err = client.GL().CustomAttribute.ListCustomUserAttributes(id, gl.WithContext(ctx))
	case resourceGroup:
		attrs, _, err = client.GL().CustomAttribute.ListCustomGroupAttributes(id, gl.WithContext(ctx))
	case resourceProject:
		attrs, _, err = client.GL().CustomAttribute.ListCustomProjectAttributes(id, gl.WithContext(ctx))
	default:
		return ListOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_custom_attributes", err, http.StatusNotFound, "verify resource_type (user, group, project) and resource_id. Requires admin access")
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
	ResourceType string               `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   toolutil.StringOrInt `json:"resource_id" jsonschema:"ID of the resource, the URL-encoded path of a group or a project, or the username of a user,required"`
	Key          string               `json:"key" jsonschema:"Attribute key,required"`
}

// GetOutput is the output for getting a custom attribute.
type GetOutput struct {
	toolutil.HintableOutput
	AttributeItem
}

// Get gets a single custom attribute by key (admin).
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	id, err := resolveResourceID(ctx, client, "get_custom_attribute", input.ResourceType, input.ResourceID)
	if err != nil {
		return GetOutput{}, err
	}

	var attr *gl.CustomAttribute

	switch input.ResourceType {
	case resourceUser:
		attr, _, err = client.GL().CustomAttribute.GetCustomUserAttribute(id, input.Key, gl.WithContext(ctx))
	case resourceGroup:
		attr, _, err = client.GL().CustomAttribute.GetCustomGroupAttribute(id, input.Key, gl.WithContext(ctx))
	case resourceProject:
		attr, _, err = client.GL().CustomAttribute.GetCustomProjectAttribute(id, input.Key, gl.WithContext(ctx))
	default:
		return GetOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_custom_attribute", err, http.StatusNotFound, "verify resource_type, resource_id, and key. Requires admin access")
	}
	return GetOutput{Key: attr.Key, Value: attr.Value}, nil
}

// ---------------------------------------------------------------------------
// Set
// ---------------------------------------------------------------------------.

// SetInput is the input for setting a custom attribute.
type SetInput struct {
	ResourceType string               `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   toolutil.StringOrInt `json:"resource_id" jsonschema:"ID of the resource, the URL-encoded path of a group or a project, or the username of a user,required"`
	Key          string               `json:"key" jsonschema:"Attribute key,required"`
	Value        string               `json:"value" jsonschema:"Attribute value,required"`
}

// SetOutput is the output for setting a custom attribute.
type SetOutput struct {
	toolutil.HintableOutput
	AttributeItem
}

// Set sets (creates or updates) a custom attribute (admin).
func Set(ctx context.Context, client *gitlabclient.Client, input SetInput) (SetOutput, error) {
	id, err := resolveResourceID(ctx, client, "set_custom_attribute", input.ResourceType, input.ResourceID)
	if err != nil {
		return SetOutput{}, err
	}

	ca := gl.CustomAttribute{Key: input.Key, Value: input.Value}
	var attr *gl.CustomAttribute

	switch input.ResourceType {
	case resourceUser:
		attr, _, err = client.GL().CustomAttribute.SetCustomUserAttribute(id, ca, gl.WithContext(ctx))
	case resourceGroup:
		attr, _, err = client.GL().CustomAttribute.SetCustomGroupAttribute(id, ca, gl.WithContext(ctx))
	case resourceProject:
		attr, _, err = client.GL().CustomAttribute.SetCustomProjectAttribute(id, ca, gl.WithContext(ctx))
	default:
		return SetOutput{}, toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return SetOutput{}, toolutil.WrapErrWithStatusHint("set_custom_attribute", err, http.StatusNotFound, "verify resource_type and resource_id. Requires admin access")
	}
	return SetOutput{Key: attr.Key, Value: attr.Value}, nil
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------.

// DeleteInput is the input for deleting a custom attribute.
type DeleteInput struct {
	ResourceType string               `json:"resource_type" jsonschema:"Resource type: user, group, or project,required"`
	ResourceID   toolutil.StringOrInt `json:"resource_id" jsonschema:"ID of the resource, the URL-encoded path of a group or a project, or the username of a user,required"`
	Key          string               `json:"key" jsonschema:"Attribute key to delete,required"`
}

// Delete deletes a custom attribute (admin).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	id, err := resolveResourceID(ctx, client, "delete_custom_attribute", input.ResourceType, input.ResourceID)
	if err != nil {
		return err
	}

	switch input.ResourceType {
	case resourceUser:
		_, err = client.GL().CustomAttribute.DeleteCustomUserAttribute(id, input.Key, gl.WithContext(ctx))
	case resourceGroup:
		_, err = client.GL().CustomAttribute.DeleteCustomGroupAttribute(id, input.Key, gl.WithContext(ctx))
	case resourceProject:
		_, err = client.GL().CustomAttribute.DeleteCustomProjectAttribute(id, input.Key, gl.WithContext(ctx))
	default:
		return toolutil.ErrInvalidEnum("resource_type", input.ResourceType, validResourceTypes)
	}
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete_custom_attribute", err, http.StatusNotFound, "verify resource_type, resource_id, and key. Requires admin access")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Formatters
// ---------------------------------------------------------------------------.
