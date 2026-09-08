package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// The four state mutations answer with the vulnerability they changed,
// selected as vulnFields: the same node the list and the get answer with, so
// a model that dismissed a vulnerability is handed the whole of it back.

const mutationDismiss = `
mutation($id: VulnerabilityID!, $comment: String, $dismissalReason: VulnerabilityDismissalReason) {
  vulnerabilityDismiss(input: {id: $id, comment: $comment, dismissalReason: $dismissalReason}) {
    vulnerability {` + vulnFields + `
    }
    errors
  }
}
`

const mutationConfirm = `
mutation($id: VulnerabilityID!) {
  vulnerabilityConfirm(input: {id: $id}) {
    vulnerability {` + vulnFields + `
    }
    errors
  }
}
`

const mutationResolve = `
mutation($id: VulnerabilityID!) {
  vulnerabilityResolve(input: {id: $id}) {
    vulnerability {` + vulnFields + `
    }
    errors
  }
}
`

const mutationRevert = `
mutation($id: VulnerabilityID!) {
  vulnerabilityRevertToDetected(input: {id: $id}) {
    vulnerability {` + vulnFields + `
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

// vulnerabilityMutationResponse is the envelope every state mutation answers
// with: one payload under the mutation's own name. A map rather than one
// field per mutation, because each document selects one of the four and a
// struct naming all four would hold three that are always empty.
type vulnerabilityMutationResponse struct {
	Data map[string]gqlMutationPayload `json:"data"`
}

// runVulnerabilityMutation sends one state mutation and reads its payload
// back from under payloadKey, the name of the mutation the document selects.
func runVulnerabilityMutation(ctx context.Context, client *gitlabclient.Client, operation, query, hint string, vars map[string]any, payloadKey string) (MutationOutput, error) {
	var resp vulnerabilityMutationResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: query, Variables: vars}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint(operation, err, hint)
	}

	result := resp.Data[payloadKey]
	if len(result.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("%s: %s", operation, result.Errors[0])
	}
	return MutationOutput{Vulnerability: nodeToItem(result.Vulnerability)}, nil
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

	return runVulnerabilityMutation(ctx, client, "dismiss_vulnerability", mutationDismiss,
		"verify the vulnerability GID is valid and the vulnerability is in a dismissable state", vars,
		"vulnerabilityDismiss")
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

	return runVulnerabilityMutation(ctx, client, "confirm_vulnerability", mutationConfirm,
		"verify the vulnerability GID is valid and the vulnerability is in a confirmable state", map[string]any{"id": input.ID},
		"vulnerabilityConfirm")
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

	return runVulnerabilityMutation(ctx, client, "resolve_vulnerability", mutationResolve,
		"verify the vulnerability GID is valid and the vulnerability is in a resolvable state", map[string]any{"id": input.ID},
		"vulnerabilityResolve")
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

	return runVulnerabilityMutation(ctx, client, "revert_vulnerability", mutationRevert,
		"verify the vulnerability GID is valid and the vulnerability is in resolved or dismissed state", map[string]any{"id": input.ID},
		"vulnerabilityRevertToDetected")
}
