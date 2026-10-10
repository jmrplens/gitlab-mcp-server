package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// gqlMutationPayload is the shared result shape for all vulnerability state
// mutations. The vulnerability is a pointer so that a state change GitLab ran
// and answered without it is told apart from one that answered it.
type gqlMutationPayload struct {
	Vulnerability *gqlVulnerabilityNode `json:"vulnerability"`
	Errors        []string              `json:"errors"`
}

// vulnerabilityMutationResponse is the envelope every state mutation answers
// with: one payload under the mutation's own name. A map rather than one
// field per mutation, because each document selects one of the four and a
// struct naming all four would hold three that are always empty. The payload
// is a pointer so that a mutation GitLab refused, which it answers with the
// payload null and the reason in the top-level errors, is told apart from one
// that ran.
type vulnerabilityMutationResponse struct {
	Data   map[string]*gqlMutationPayload `json:"data"`
	Errors []toolutil.GraphQLError        `json:"errors"`
}

// runVulnerabilityMutation sends one state mutation and reads its payload
// back from under payloadKey, the name of the mutation the document selects.
//
// A mutation GitLab refused answers HTTP 200 with the payload null and one
// top-level errors[] entry, which client-go does not turn into an error: a
// refusal of the vulnerability's permission, a fine-grained token's grant
// lacking it among them (GitLab's sentence naming the permission). Reading the
// payload alone reported that refusal as a state change that happened, with
// an empty vulnerability.
func runVulnerabilityMutation(ctx context.Context, client *gitlabclient.Client, operation, query, hint string, vars map[string]any, payloadKey string) (MutationOutput, error) {
	var resp vulnerabilityMutationResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: query, Variables: vars}, &resp, gl.WithContext(ctx))
	if err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint(operation, err, hint)
	}

	result := resp.Data[payloadKey]
	if result == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError(operation, resp.Errors); graphQLErr != nil {
			return MutationOutput{}, graphQLErr
		}
		return MutationOutput{}, fmt.Errorf("%s: GitLab answered with no %s payload", operation, payloadKey)
	}
	if len(result.Errors) > 0 {
		return MutationOutput{}, fmt.Errorf("%s: %s", operation, result.Errors[0])
	}
	// No vulnerability and no error is a state change GitLab ran and answered
	// without it. Vulnerability: Update grants the four state permissions and
	// not read_vulnerability, which the vulnerability is checked against only
	// after the change ran, so a fine-grained token granted the one and not the
	// other is answered this way, and the session is told the change probably
	// committed rather than handed an empty vulnerability as its result (issue
	// 1103). A session with no authority keeps the answer it always had.
	if result.Vulnerability == nil {
		if unconfirmed := client.Authority().UnconfirmedWrite(operation, "vulnerability", nil); unconfirmed != nil {
			return MutationOutput{}, unconfirmed
		}
		result.Vulnerability = &gqlVulnerabilityNode{}
	}
	return MutationOutput{Vulnerability: nodeToItem(*result.Vulnerability)}, nil
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
