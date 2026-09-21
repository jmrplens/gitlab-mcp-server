package groupscim

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hintVerifyUID is the error hint shared by SCIM identity tools.
const hintVerifyUID = "verify uid with group_scim.list; SCIM identities exist only after group SAML SSO SCIM provisioning has synchronized users"

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
// ExternUID is spelled the way GitLab spells it. The SDK struct declared the
// key as external_uid until client-go v3.12.0, a key GitLab never sends, so
// the SDK's own field never decoded and this output was filled from the
// captured response instead. v3.12.0 corrected the tag to extern_uid, which
// is why the capture is gone and the SDK struct is read directly.
type Output struct {
	toolutil.HintableOutput
	ExternUID string `json:"extern_uid"`
	UserID    int64  `json:"user_id"`
	Active    bool   `json:"active"`
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
		ExternUID: id.ExternalUID,
		UserID:    id.UserID,
		Active:    id.Active,
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
	ids, _, err := client.GL().GroupSCIM.GetSCIMIdentitiesForGroup(in.GroupID.String(), gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list SCIM identities for group", err, http.StatusNotFound, "verify group_id; Group SCIM requires Premium license, SAML SSO, and SCIM provisioning")
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
	id, _, err := client.GL().GroupSCIM.GetSCIMIdentity(in.GroupID.String(), in.UID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("get SCIM identity", err, hintVerifyUID)
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
	_, err := client.GL().GroupSCIM.UpdateSCIMIdentity(in.GroupID.String(), in.UID, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("update SCIM identity", err, hintVerifyUID)
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
	_, err := client.GL().GroupSCIM.DeleteSCIMIdentity(in.GroupID.String(), in.UID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("delete SCIM identity", err, hintVerifyUID)
	}
	return nil
}
