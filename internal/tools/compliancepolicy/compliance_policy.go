package compliancepolicy

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// GetInput holds parameters for retrieving compliance policy settings.
type GetInput struct{}

// UpdateInput holds parameters for updating compliance policy settings.
type UpdateInput struct {
	CSPNamespaceID *int64 `json:"csp_namespace_id,omitempty" jsonschema:"Namespace ID for the compliance security policy project"`
}

// Output represents compliance policy settings.
type Output struct {
	toolutil.HintableOutput
	CSPNamespaceID *int64 `json:"csp_namespace_id"`
}

// Get retrieves the current admin compliance policy settings.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	result, _, err := client.GL().AdminCompliancePolicySettings.GetCompliancePolicySettings(gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("get compliance policy settings", err, http.StatusForbidden, "compliance policies require Ultimate license and Owner role")
	}

	return Output{
		CSPNamespaceID: result.CSPNamespaceID,
	}, nil
}

// Update modifies the admin compliance policy settings.
func Update(ctx context.Context, client *gitlabclient.Client, in UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.CSPNamespaceID == nil {
		return Output{}, errors.New("csp_namespace_id is required; GitLab rejects empty compliance policy update requests")
	}

	opts := &gl.UpdateAdminCompliancePolicySettingsOptions{
		CSPNamespaceID: in.CSPNamespaceID,
	}
	result, _, err := client.GL().AdminCompliancePolicySettings.UpdateCompliancePolicySettings(opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) || toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) {
			return Output{}, toolutil.WrapErrWithHint("update compliance policy settings", err,
				"csp_namespace_id must be an existing top-level group namespace; GitLab may lock CSP namespace changes for several minutes after each update")
		}
		return Output{}, toolutil.WrapErrWithStatusHint("update compliance policy settings", err, http.StatusForbidden, "updating compliance policies requires Ultimate license and Owner role")
	}

	return Output{
		CSPNamespaceID: result.CSPNamespaceID,
	}, nil
}
