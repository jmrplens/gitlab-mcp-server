// Package groupldap implements MCP tool handlers for GitLab group LDAP link operations.
package groupldap

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Output represents a single group LDAP link.
type Output struct {
	toolutil.HintableOutput
	CN           string `json:"cn"`
	Filter       string `json:"filter,omitempty"`
	GroupAccess  int    `json:"group_access"`
	Provider     string `json:"provider"`
	MemberRoleID int64  `json:"member_role_id,omitempty"`
}

// ListOutput holds a list of group LDAP links.
type ListOutput struct {
	toolutil.HintableOutput
	Links []Output `json:"links"`
}

// DeleteOutput confirms the deletion of an LDAP link.
type DeleteOutput = toolutil.DeleteOutput

func toOutput(l *gl.LDAPGroupLink) Output {
	return Output{
		CN:           l.CN,
		Filter:       l.Filter,
		GroupAccess:  int(l.GroupAccess),
		Provider:     l.Provider,
		MemberRoleID: l.MemberRoleID,
	}
}

// ListInput holds parameters for listing group LDAP links.
type ListInput struct {
	GroupID string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// List retrieves all LDAP links for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	links, _, err := client.GL().Groups.ListGroupLDAPLinks(input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, fmt.Errorf("list group LDAP links: %w", err)
	}
	out := make([]Output, len(links))
	for i, l := range links {
		out[i] = toOutput(l)
	}
	return ListOutput{Links: out}, nil
}

// AddInput holds parameters for adding a group LDAP link.
type AddInput struct {
	GroupID      string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	CN           string `json:"cn,omitempty" jsonschema:"LDAP Common Name (CN)"`
	Filter       string `json:"filter,omitempty" jsonschema:"LDAP filter"`
	GroupAccess  int    `json:"group_access" jsonschema:"Access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner),required"`
	Provider     string `json:"provider" jsonschema:"LDAP provider name,required"`
	MemberRoleID *int64 `json:"member_role_id,omitempty" jsonschema:"Custom member role ID"`
}

// Add creates a new group LDAP link.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Provider == "" {
		return Output{}, toolutil.ErrFieldRequired("provider")
	}
	access := gl.AccessLevelValue(input.GroupAccess)
	opts := &gl.AddGroupLDAPLinkOptions{
		GroupAccess:  &access,
		Provider:     &input.Provider,
		MemberRoleID: input.MemberRoleID,
	}
	if input.CN != "" {
		opts.CN = &input.CN
	}
	if input.Filter != "" {
		opts.Filter = &input.Filter
	}
	link, _, err := client.GL().Groups.AddGroupLDAPLink(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, fmt.Errorf("add group LDAP link: %w", err)
	}
	return toOutput(link), nil
}

// DeleteWithCNOrFilterInput holds parameters for deleting a group LDAP link by CN or filter.
type DeleteWithCNOrFilterInput struct {
	GroupID  string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	CN       string `json:"cn,omitempty" jsonschema:"LDAP Common Name to delete"`
	Filter   string `json:"filter,omitempty" jsonschema:"LDAP filter to delete"`
	Provider string `json:"provider,omitempty" jsonschema:"LDAP provider name"`
}

// DeleteWithCNOrFilter deletes a group LDAP link by CN or filter.
func DeleteWithCNOrFilter(ctx context.Context, client *gitlabclient.Client, input DeleteWithCNOrFilterInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	opts := &gl.DeleteGroupLDAPLinkWithCNOrFilterOptions{}
	if input.CN != "" {
		opts.CN = &input.CN
	}
	if input.Filter != "" {
		opts.Filter = &input.Filter
	}
	if input.Provider != "" {
		opts.Provider = &input.Provider
	}
	_, err := client.GL().Groups.DeleteGroupLDAPLinkWithCNOrFilter(input.GroupID, opts, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("delete group LDAP link: %w", err)
	}
	return nil
}

// DeleteForProviderInput holds parameters for deleting a group LDAP link for a specific provider.
type DeleteForProviderInput struct {
	GroupID  string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Provider string `json:"provider" jsonschema:"LDAP provider name,required"`
	CN       string `json:"cn" jsonschema:"LDAP Common Name,required"`
}

// DeleteForProvider deletes a group LDAP link for a specific provider.
func DeleteForProvider(ctx context.Context, client *gitlabclient.Client, input DeleteForProviderInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if input.Provider == "" {
		return toolutil.ErrFieldRequired("provider")
	}
	if input.CN == "" {
		return toolutil.ErrFieldRequired("cn")
	}
	_, err := client.GL().Groups.DeleteGroupLDAPLinkForProvider(input.GroupID, input.Provider, input.CN, gl.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("delete group LDAP link for provider: %w", err)
	}
	return nil
}
