// mutations.go provides mutation handlers for dismissing, confirming,
// resolving, and reverting vulnerabilities via the GitLab GraphQL API.
package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GraphQL mutation fragment shared by all state mutations.
const mutationVulnFields = `
      id
      title
      severity
      state
      reportType
      detectedAt
      dismissedAt
      resolvedAt
      confirmedAt
      dismissalReason
      primaryIdentifier {
        name
        externalType
        externalId
        url
      }
      scanner {
        name
        vendor
      }
`

const mutationDismiss = `
mutation($id: VulnerabilityID!, $comment: String, $dismissalReason: VulnerabilityDismissalReason) {
  vulnerabilityDismiss(input: {id: $id, comment: $comment, dismissalReason: $dismissalReason}) {
    vulnerability {` + mutationVulnFields + `
    }
    errors
  }
}
`

const mutationConfirm = `
mutation($id: VulnerabilityID!) {
  vulnerabilityConfirm(input: {id: $id}) {
    vulnerability {` + mutationVulnFields + `
    }
    errors
  }
}
`

const mutationResolve = `
mutation($id: VulnerabilityID!) {
  vulnerabilityResolve(input: {id: $id}) {
    vulnerability {` + mutationVulnFields + `
    }
    errors
  }
}
`

const mutationRevert = `
mutation($id: VulnerabilityID!) {
  vulnerabilityRevertToDetected(input: {id: $id}) {
    vulnerability {` + mutationVulnFields + `
    }
    errors
  }
}
`

// MutationOutput is the output for vulnerability state mutations.
type MutationOutput struct {
	toolutil.HintableOutput
	Vulnerability Item `json:"vulnerability"`
}

// gqlMutationPayload is the shared result shape for all vulnerability state mutations.
type gqlMutationPayload struct {
	Vulnerability gqlVulnerabilityNode `json:"vulnerability"`
	Errors        []string             `json:"errors"`
}

// Dismiss.

// DismissInput is the input for dismissing a vulnerability.
type DismissInput struct {
	ID              string `json:"id" jsonschema:"Vulnerability GID (e.g. gid://gitlab/Vulnerability/42),required"`
	Comment         string `json:"comment,omitempty" jsonschema:"Reason for dismissal"`
	DismissalReason string `json:"dismissal_reason,omitempty" jsonschema:"Dismissal reason: ACCEPTABLE_RISK, FALSE_POSITIVE, MITIGATING_CONTROL, USED_IN_TESTS, NOT_APPLICABLE"`
}

// Dismiss dismisses a vulnerability via the GitLab GraphQL API.
func Dismiss(ctx context.Context, client *gitlabclient.Client, input DismissInput) (MutationOutput, error) {
	if input.ID == "" {
		return MutationOutput{}, toolutil.ErrRequiredString("dismiss_vulnerability", "id")
	}

	vars := map[string]any{"id": input.ID}
	if input.Comment != "" {
		vars["comment"] = input.Comment
	}
	if input.DismissalReason != "" {
		vars["dismissalReason"] = input.DismissalReason
	}

	var resp struct {
		Data struct {
			VulnerabilityDismiss gqlMutationPayload `json:"vulnerabilityDismiss"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationDismiss,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("dismiss_vulnerability", err, "verify the vulnerability GID is valid and the vulnerability is in a dismissable state")
	}

	if len(resp.Data.VulnerabilityDismiss.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("dismiss_vulnerability: %s", resp.Data.VulnerabilityDismiss.Errors[0])
	}

	return MutationOutput{Vulnerability: nodeToItem(resp.Data.VulnerabilityDismiss.Vulnerability)}, nil
}

// Confirm.

// ConfirmInput is the input for confirming a vulnerability.
type ConfirmInput struct {
	ID string `json:"id" jsonschema:"Vulnerability GID (e.g. gid://gitlab/Vulnerability/42),required"`
}

// Confirm confirms a vulnerability via the GitLab GraphQL API.
func Confirm(ctx context.Context, client *gitlabclient.Client, input ConfirmInput) (MutationOutput, error) {
	if input.ID == "" {
		return MutationOutput{}, toolutil.ErrRequiredString("confirm_vulnerability", "id")
	}

	var resp struct {
		Data struct {
			VulnerabilityConfirm gqlMutationPayload `json:"vulnerabilityConfirm"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationConfirm,
		Variables: map[string]any{"id": input.ID},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("confirm_vulnerability", err, "verify the vulnerability GID is valid and the vulnerability is in a confirmable state")
	}

	if len(resp.Data.VulnerabilityConfirm.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("confirm_vulnerability: %s", resp.Data.VulnerabilityConfirm.Errors[0])
	}

	return MutationOutput{Vulnerability: nodeToItem(resp.Data.VulnerabilityConfirm.Vulnerability)}, nil
}

// Resolve.

// ResolveInput is the input for resolving a vulnerability.
type ResolveInput struct {
	ID string `json:"id" jsonschema:"Vulnerability GID (e.g. gid://gitlab/Vulnerability/42),required"`
}

// Resolve resolves a vulnerability via the GitLab GraphQL API.
func Resolve(ctx context.Context, client *gitlabclient.Client, input ResolveInput) (MutationOutput, error) {
	if input.ID == "" {
		return MutationOutput{}, toolutil.ErrRequiredString("resolve_vulnerability", "id")
	}

	var resp struct {
		Data struct {
			VulnerabilityResolve gqlMutationPayload `json:"vulnerabilityResolve"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationResolve,
		Variables: map[string]any{"id": input.ID},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("resolve_vulnerability", err, "verify the vulnerability GID is valid and the vulnerability is in a resolvable state")
	}

	if len(resp.Data.VulnerabilityResolve.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("resolve_vulnerability: %s", resp.Data.VulnerabilityResolve.Errors[0])
	}

	return MutationOutput{Vulnerability: nodeToItem(resp.Data.VulnerabilityResolve.Vulnerability)}, nil
}

// Revert.

// RevertInput is the input for reverting a vulnerability to detected state.
type RevertInput struct {
	ID string `json:"id" jsonschema:"Vulnerability GID (e.g. gid://gitlab/Vulnerability/42),required"`
}

// Revert reverts a vulnerability to detected state via the GitLab GraphQL API.
func Revert(ctx context.Context, client *gitlabclient.Client, input RevertInput) (MutationOutput, error) {
	if input.ID == "" {
		return MutationOutput{}, toolutil.ErrRequiredString("revert_vulnerability", "id")
	}

	var resp struct {
		Data struct {
			VulnerabilityRevertToDetected gqlMutationPayload `json:"vulnerabilityRevertToDetected"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     mutationRevert,
		Variables: map[string]any{"id": input.ID},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("revert_vulnerability", err, "verify the vulnerability GID is valid and the vulnerability is in resolved or dismissed state")
	}

	if len(resp.Data.VulnerabilityRevertToDetected.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("revert_vulnerability: %s", resp.Data.VulnerabilityRevertToDetected.Errors[0])
	}

	return MutationOutput{Vulnerability: nodeToItem(resp.Data.VulnerabilityRevertToDetected.Vulnerability)}, nil
}
