// Package groupsaml implements MCP tool handlers for GitLab group SAML link operations.
package groupsaml

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents a single group SAML link.
type Output struct {
	toolutil.HintableOutput
	Name         string `json:"name"`
	AccessLevel  int    `json:"access_level"`
	MemberRoleID int64  `json:"member_role_id,omitempty"`
	Provider     string `json:"provider,omitempty"`
}

// ListOutput holds a list of group SAML links.
type ListOutput struct {
	toolutil.HintableOutput
	Links []Output `json:"links"`
}

func toOutput(l *gl.SAMLGroupLink) Output {
	return Output{
		Name:         l.Name,
		AccessLevel:  int(l.AccessLevel),
		MemberRoleID: l.MemberRoleID,
		Provider:     l.Provider,
	}
}

// ListInput holds parameters for listing group SAML links.
type ListInput struct {
	GroupID string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// List retrieves all SAML links for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	links, _, err := client.GL().Groups.ListGroupSAMLLinks(input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, fmt.Errorf("list group SAML links: %w", err)
	}
	out := make([]Output, len(links))
	for i, l := range links {
		out[i] = toOutput(l)
	}
	return ListOutput{Links: out}, nil
}

// GetInput holds parameters for getting a single group SAML link.
type GetInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group,required"`
}

// Get retrieves a single SAML link for a group.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return Output{}, toolutil.ErrFieldRequired("saml_group_name")
	}
	link, _, err := client.GL().Groups.GetGroupSAMLLink(input.GroupID, input.SAMLGroupName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, fmt.Errorf("get group SAML link: %w", err)
	}
	return toOutput(link), nil
}

// AddInput holds parameters for adding a group SAML link.
type AddInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group,required"`
	AccessLevel   int    `json:"access_level" jsonschema:"Access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner),required"`
	MemberRoleID  *int64 `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
	Provider      string `json:"provider,omitempty" jsonschema:"SAML provider name"`
}

// Add creates a new group SAML link.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return Output{}, toolutil.ErrFieldRequired("saml_group_name")
	}
	access := gl.AccessLevelValue(input.AccessLevel)
	opts := &gl.AddGroupSAMLLinkOptions{
		SAMLGroupName: &input.SAMLGroupName,
		AccessLevel:   &access,
		MemberRoleID:  input.MemberRoleID,
	}
	if input.Provider != "" {
		opts.Provider = &input.Provider
	}
	link, _, err := client.GL().Groups.AddGroupSAMLLink(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, fmt.Errorf("add group SAML link: %w", err)
	}
	return toOutput(link), nil
}

// DeleteInput holds parameters for deleting a group SAML link.
type DeleteInput struct {
	GroupID       string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	SAMLGroupName string `json:"saml_group_name" jsonschema:"Name of the SAML group to delete,required"`
}

// Delete removes a group SAML link.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.SAMLGroupName == "" {
		return toolutil.ErrFieldRequired("saml_group_name")
	}
	_, err := client.GL().Groups.DeleteGroupSAMLLink(input.GroupID, input.SAMLGroupName, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("delete group SAML link: %w", err)
	}
	return nil
}
