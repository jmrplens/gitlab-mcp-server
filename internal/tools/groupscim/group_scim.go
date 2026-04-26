// Package groupscim implements GitLab SCIM identity operations for groups
// including list, get, update, and delete.
package groupscim

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds parameters for listing SCIM identities for a group.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// GetInput holds parameters for getting a single SCIM identity.
type GetInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UID     string               `json:"uid"      jsonschema:"SCIM external UID of the user,required"`
}

// UpdateInput holds parameters for updating a SCIM identity.
type UpdateInput struct {
	GroupID   toolutil.StringOrInt `json:"group_id"   jsonschema:"Group ID or URL-encoded path,required"`
	UID       string               `json:"uid"         jsonschema:"SCIM external UID of the user,required"`
	ExternUID string               `json:"extern_uid"  jsonschema:"New external UID value,required"`
}

// DeleteInput holds parameters for deleting a SCIM identity.
type DeleteInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	UID     string               `json:"uid"      jsonschema:"SCIM external UID of the user,required"`
}

// Output represents a SCIM identity.
type Output struct {
	toolutil.HintableOutput
	ExternalUID string `json:"external_uid"`
	UserID      int64  `json:"user_id"`
	Active      bool   `json:"active"`
}

// ListOutput holds the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Identities []Output `json:"identities"`
}

// UpdateOutput holds the update confirmation.
type UpdateOutput struct {
	toolutil.HintableOutput
	Updated bool   `json:"updated"`
	Message string `json:"message"`
}

func toOutput(id *gl.GroupSCIMIdentity) Output {
	if id == nil {
		return Output{}
	}
	return Output{
		ExternalUID: id.ExternalUID,
		UserID:      id.UserID,
		Active:      id.Active,
	}
}

// List returns all SCIM identities for a group.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	ids, _, err := client.GL().GroupSCIM.GetSCIMIdentitiesForGroup(in.GroupID.String())
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list SCIM identities for group", err, http.StatusNotFound, "verify group_id \u2014 SCIM provisioning requires Premium license and SAML SSO")
	}
	out := ListOutput{Identities: make([]Output, 0, len(ids))}
	for _, id := range ids {
		out.Identities = append(out.Identities, toOutput(id))
	}
	return out, nil
}

// Get returns a single SCIM identity.
func Get(ctx context.Context, client *gitlabclient.Client, in GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID.String() == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if in.UID == "" {
		return Output{}, toolutil.ErrFieldRequired("uid")
	}
	id, _, err := client.GL().GroupSCIM.GetSCIMIdentity(in.GroupID.String(), in.UID)
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get SCIM identity", err, http.StatusNotFound, "verify uid with gitlab_list_group_scim_identities")
	}
	return toOutput(id), nil
}

// Update modifies a SCIM identity.
func Update(ctx context.Context, client *gitlabclient.Client, in UpdateInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.UID == "" {
		return toolutil.ErrFieldRequired("uid")
	}
	if in.ExternUID == "" {
		return toolutil.ErrFieldRequired("extern_uid")
	}
	opts := &gl.UpdateSCIMIdentityOptions{
		ExternUID: new(in.ExternUID),
	}
	_, err := client.GL().GroupSCIM.UpdateSCIMIdentity(in.GroupID.String(), in.UID, opts)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("update SCIM identity", err, http.StatusNotFound, "verify uid with gitlab_list_group_scim_identities")
	}
	return nil
}

// Delete removes a SCIM identity.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.UID == "" {
		return toolutil.ErrFieldRequired("uid")
	}
	_, err := client.GL().GroupSCIM.DeleteSCIMIdentity(in.GroupID.String(), in.UID)
	if err != nil {
		return toolutil.WrapErrWithStatusHint("delete SCIM identity", err, http.StatusNotFound, "verify uid with gitlab_list_group_scim_identities")
	}
	return nil
}
