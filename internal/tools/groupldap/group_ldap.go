// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupldap

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The hints a GitLab refusal of an LDAP action is wrapped with, one per shape
// of input, each written out whole. Only the add and the delete by CN or
// filter choose between a common name and a filter; the list and the sync
// take the group alone, and the delete for a provider takes a common name and
// the provider, and no filter at all.
const (
	groupLDAPHint         = "verify group_id with group.get; group LDAP links need an LDAP provider configured on the GitLab instance; requires Premium/Ultimate and Owner access"
	groupLDAPLinkHint     = "verify group_id with group.get; provide exactly one of cn or filter, and use the LDAP provider name configured on the GitLab instance; requires Premium/Ultimate and Owner access"
	groupLDAPProviderHint = "verify group_id with group.get; pass the cn and the LDAP provider name of the link, as configured on the GitLab instance; requires Premium/Ultimate and Owner access"
)

// The operations the two link actions that take a selector report under.
const (
	opAddLink    = "add group LDAP link"
	opDeleteLink = "delete group LDAP link"
)

// selectorError refuses a link call that names both a common name and a
// filter, or neither, before anything is sent. GitLab declares
// exactly_one_of :cn, :filter on the add and on the delete by CN or filter
// (ee/lib/api/ldap_group_links.rb) and refuses the first call with "cn, filter
// are mutually exclusive" and the second with "cn, filter are missing, exactly
// one parameter must be provided". Refusing here saves the round trip and
// words the refusal the same on every surface: the individual tool and the
// meta tool reach it with both calls and the dynamic one with a call naming
// both, which none of their dispatchers refuses, while the dynamic dispatcher
// refuses a call naming neither first, and so does the meta tool's full
// parameter-schema mode both, against the catalog's oneOf. The individual
// tool is served without that oneOf, since the Anthropic Messages API
// refuses a tool list holding one at the root.
//
// A selector counts as named when it is not empty, which is also when the
// handlers send it. GitLab counts a key it is sent whatever its value, and it
// is sent only the selectors counted here, so the two agree, and the oneOf
// counts the same way (toolutil.SchemaOneOfRequired): a common name beside an
// empty filter is served on every surface.
func selectorError(operation, cn, filter string) error {
	if cn != "" && filter != "" {
		return fmt.Errorf("%s: send exactly one of cn or filter, not both", operation)
	}
	if cn == "" && filter == "" {
		return fmt.Errorf("%s: exactly one of cn or filter is required", operation)
	}
	return nil
}

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

// groupExists reports whether a group is reachable. It is used only to
// disambiguate a 404 from ListGroupLDAPLinks (a group with no LDAP links versus
// a missing group). It takes a plain groupID so the unrelated GetGroupOptions
// existence-probe literal is not attributed to an LDAP input struct: the
// LDAP-links list endpoint (GET /groups/:id/ldap_group_links) accepts no query
// parameters, so ListInput intentionally carries only group_id.
func groupExists(ctx context.Context, client *gitlabclient.Client, groupID string) bool {
	_, _, err := client.GL().Groups.GetGroup(groupID, &gl.GetGroupOptions{}, gl.WithContext(ctx))
	return err == nil
}

// List retrieves all LDAP links for a group.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	links, _, err := client.GL().Groups.ListGroupLDAPLinks(input.GroupID, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) && groupExists(ctx, client, input.GroupID) {
			return ListOutput{Links: []Output{}}, nil
		}
		return ListOutput{}, toolutil.WrapErrWithHint("list group LDAP links", err, groupLDAPHint)
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
	CN           string `json:"cn,omitempty" jsonschema:"LDAP Common Name (CN) of the group to link. Send exactly one of cn or filter, not both."`
	Filter       string `json:"filter,omitempty" jsonschema:"LDAP filter selecting the users to link. Send exactly one of cn or filter, not both."`
	GroupAccess  int    `json:"group_access" jsonschema:"Default access level for members of the LDAP group (5=Minimal access, 10=Guest, 15=Planner, 20=Reporter, 25=Security Manager, 30=Developer, 40=Maintainer, 50=Owner). An LDAP group link takes Minimal access on a subgroup as well, and no other level,required"`
	Provider     string `json:"provider" jsonschema:"LDAP provider name,required"`
	MemberRoleID *int64 `json:"member_role_id,omitempty" tier:"ultimate" jsonschema:"Custom member role ID the link's members get (Ultimate)"`
}

// Add creates a new group LDAP link.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.Provider == "" {
		return Output{}, toolutil.ErrFieldRequired("provider")
	}
	if err := selectorError(opAddLink, input.CN, input.Filter); err != nil {
		return Output{}, err
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
		return Output{}, toolutil.WrapErrWithHint(opAddLink, err, groupLDAPLinkHint)
	}
	return toOutput(link), nil
}

// DeleteWithCNOrFilterInput holds parameters for deleting a group LDAP link by CN or filter.
type DeleteWithCNOrFilterInput struct {
	GroupID  string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	CN       string `json:"cn,omitempty" jsonschema:"LDAP Common Name of the link to delete. Send exactly one of cn or filter, not both."`
	Filter   string `json:"filter,omitempty" jsonschema:"LDAP filter of the link to delete. Send exactly one of cn or filter, not both."`
	Provider string `json:"provider,omitempty" jsonschema:"LDAP provider name,required"`
}

// DeleteWithCNOrFilter deletes a group LDAP link by CN or filter.
func DeleteWithCNOrFilter(ctx context.Context, client *gitlabclient.Client, input DeleteWithCNOrFilterInput) error {
	if input.GroupID == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if err := selectorError(opDeleteLink, input.CN, input.Filter); err != nil {
		return err
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
		return toolutil.WrapErrWithHint(opDeleteLink, err, groupLDAPLinkHint)
	}
	return nil
}

// SyncInput holds parameters for triggering a group LDAP sync.
type SyncInput struct {
	GroupID string `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// Sync triggers a synchronization of the group's LDAP group links. The GitLab
// API returns 202 Accepted; the actual sync runs asynchronously in the
// background.
func Sync(ctx context.Context, client *gitlabclient.Client, input SyncInput) (DeleteOutput, error) {
	if input.GroupID == "" {
		return DeleteOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	_, err := client.GL().Groups.SyncGroupWithLDAP(input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithHint("sync group with LDAP", err, groupLDAPHint)
	}
	return DeleteOutput{Status: "success", Message: "LDAP synchronization started; it runs asynchronously in the background."}, nil
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
		return toolutil.WrapErrWithHint("delete group LDAP link for provider", err, groupLDAPProviderHint)
	}
	return nil
}
