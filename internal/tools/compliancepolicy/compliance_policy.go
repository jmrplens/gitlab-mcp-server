// Package compliancepolicy implements MCP tools for GitLab admin-level
// compliance policy settings management.
package compliancepolicy

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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
		return Output{}, toolutil.WrapErrWithMessage("get compliance policy settings", err)
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

	opts := &gl.UpdateAdminCompliancePolicySettingsOptions{
		CSPNamespaceID: in.CSPNamespaceID,
	}
	result, _, err := client.GL().AdminCompliancePolicySettings.UpdateCompliancePolicySettings(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("update compliance policy settings", err)
	}

	return Output{
		CSPNamespaceID: result.CSPNamespaceID,
	}, nil
}
