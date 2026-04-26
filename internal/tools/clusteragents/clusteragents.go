// Package clusteragents implements MCP tools for GitLab Kubernetes cluster agents.
package clusteragents

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListAgents.

// ListAgentsInput defines parameters for the list agents operation.
type ListAgentsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Page      int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// AgentItem holds data for clusteragents operations.
type AgentItem struct {
	toolutil.HintableOutput
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	CreatedByUserID int64  `json:"created_by_user_id,omitempty"`
}

// ListAgentsOutput represents the response from the list agents operation.
type ListAgentsOutput struct {
	toolutil.HintableOutput
	Agents     []AgentItem               `json:"agents"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListAgents lists agents for the clusteragents package.
func ListAgents(ctx context.Context, client *gitlabclient.Client, input ListAgentsInput) (ListAgentsOutput, error) {
	opts := &gl.ListAgentsOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	agents, resp, err := client.GL().ClusterAgents.ListAgents(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListAgentsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_cluster_agents", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; cluster agents require Maintainer role to view")
	}
	items := make([]AgentItem, 0, len(agents))
	for _, a := range agents {
		items = append(items, AgentItem{ID: a.ID, Name: a.Name, CreatedByUserID: a.CreatedByUserID})
	}
	return ListAgentsOutput{Agents: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetAgent.

// GetAgentInput defines parameters for the get agent operation.
type GetAgentInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID   int64                `json:"agent_id" jsonschema:"Agent ID,required"`
}

// GetAgent retrieves agent for the clusteragents package.
func GetAgent(ctx context.Context, client *gitlabclient.Client, input GetAgentInput) (AgentItem, error) {
	if input.AgentID <= 0 {
		return AgentItem{}, toolutil.ErrRequiredInt64("gitlab_get_cluster_agent", "agent_id")
	}
	a, _, err := client.GL().ClusterAgents.GetAgent(string(input.ProjectID), input.AgentID, gl.WithContext(ctx))
	if err != nil {
		return AgentItem{}, toolutil.WrapErrWithStatusHint("gitlab_get_cluster_agent", err, http.StatusNotFound,
			"verify agent_id with gitlab_cluster_agents_list; the agent may have been deleted")
	}
	return AgentItem{ID: a.ID, Name: a.Name, CreatedByUserID: a.CreatedByUserID}, nil
}

// RegisterAgent.

// RegisterAgentInput defines parameters for the register agent operation.
type RegisterAgentInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Name      string               `json:"name" jsonschema:"Agent name,required"`
}

// RegisterAgent performs the register agent operation for the clusteragents package.
func RegisterAgent(ctx context.Context, client *gitlabclient.Client, input RegisterAgentInput) (AgentItem, error) {
	opts := &gl.RegisterAgentOptions{Name: new(input.Name)}
	a, _, err := client.GL().ClusterAgents.RegisterAgent(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return AgentItem{}, toolutil.WrapErrWithStatusHint("gitlab_register_cluster_agent", err, http.StatusBadRequest,
			"name must match DNS-1123 label format (lowercase alphanumeric + dashes, max 63 chars) and be unique within the project; requires Maintainer role")
	}
	return AgentItem{ID: a.ID, Name: a.Name, CreatedByUserID: a.CreatedByUserID}, nil
}

// DeleteAgent.

// DeleteAgentInput defines parameters for the delete agent operation.
type DeleteAgentInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID   int64                `json:"agent_id" jsonschema:"Agent ID,required"`
}

// DeleteAgent deletes agent for the clusteragents package.
func DeleteAgent(ctx context.Context, client *gitlabclient.Client, input DeleteAgentInput) error {
	if input.AgentID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_delete_cluster_agent", "agent_id")
	}
	_, err := client.GL().ClusterAgents.DeleteAgent(string(input.ProjectID), input.AgentID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_delete_cluster_agent", err, http.StatusForbidden,
			"deleting cluster agents requires Maintainer role; the agent and all its tokens will be removed")
	}
	return nil
}

// ListAgentTokens.

// ListAgentTokensInput defines parameters for the list agent tokens operation.
type ListAgentTokensInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID   int64                `json:"agent_id" jsonschema:"Agent ID,required"`
	Page      int64                `json:"page" jsonschema:"Page number for pagination"`
	PerPage   int64                `json:"per_page" jsonschema:"Number of items per page"`
}

// AgentTokenItem holds data for clusteragents operations.
type AgentTokenItem struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	AgentID     int64  `json:"agent_id"`
	Status      string `json:"status"`
	Token       string `json:"token,omitempty"`
}

// ListAgentTokensOutput represents the response from the list agent tokens operation.
type ListAgentTokensOutput struct {
	toolutil.HintableOutput
	Tokens     []AgentTokenItem          `json:"tokens"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListAgentTokens lists agent tokens for the clusteragents package.
func ListAgentTokens(ctx context.Context, client *gitlabclient.Client, input ListAgentTokensInput) (ListAgentTokensOutput, error) {
	if input.AgentID <= 0 {
		return ListAgentTokensOutput{}, toolutil.ErrRequiredInt64("gitlab_list_cluster_agent_tokens", "agent_id")
	}
	opts := &gl.ListAgentTokensOptions{}
	if input.Page > 0 || input.PerPage > 0 {
		opts.ListOptions = gl.ListOptions{Page: input.Page, PerPage: input.PerPage}
	}
	tokens, resp, err := client.GL().ClusterAgents.ListAgentTokens(string(input.ProjectID), input.AgentID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListAgentTokensOutput{}, toolutil.WrapErrWithStatusHint("gitlab_list_cluster_agent_tokens", err, http.StatusNotFound,
			"verify agent_id with gitlab_cluster_agents_list; requires Maintainer role")
	}
	items := make([]AgentTokenItem, 0, len(tokens))
	for _, t := range tokens {
		items = append(items, AgentTokenItem{
			ID: t.ID, Name: t.Name, Description: t.Description,
			AgentID: t.AgentID, Status: t.Status, Token: t.Token,
		})
	}
	return ListAgentTokensOutput{Tokens: items, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetAgentToken.

// GetAgentTokenInput defines parameters for the get agent token operation.
type GetAgentTokenInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID   int64                `json:"agent_id" jsonschema:"Agent ID,required"`
	TokenID   int64                `json:"token_id" jsonschema:"Token ID,required"`
}

// GetAgentToken retrieves agent token for the clusteragents package.
func GetAgentToken(ctx context.Context, client *gitlabclient.Client, input GetAgentTokenInput) (AgentTokenItem, error) {
	if input.AgentID <= 0 {
		return AgentTokenItem{}, toolutil.ErrRequiredInt64("gitlab_get_cluster_agent_token", "agent_id")
	}
	if input.TokenID <= 0 {
		return AgentTokenItem{}, toolutil.ErrRequiredInt64("gitlab_get_cluster_agent_token", "token_id")
	}
	t, _, err := client.GL().ClusterAgents.GetAgentToken(string(input.ProjectID), input.AgentID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return AgentTokenItem{}, toolutil.WrapErrWithStatusHint("gitlab_get_cluster_agent_token", err, http.StatusNotFound,
			"verify token_id with gitlab_cluster_agent_tokens_list; the token value is only returned at creation \u2014 stored tokens show metadata only")
	}
	return AgentTokenItem{
		ID: t.ID, Name: t.Name, Description: t.Description,
		AgentID: t.AgentID, Status: t.Status, Token: t.Token,
	}, nil
}

// CreateAgentToken.

// CreateAgentTokenInput defines parameters for the create agent token operation.
type CreateAgentTokenInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID     int64                `json:"agent_id" jsonschema:"Agent ID,required"`
	Name        string               `json:"name" jsonschema:"Token name,required"`
	Description string               `json:"description" jsonschema:"Token description"`
}

// CreateAgentToken creates agent token for the clusteragents package.
func CreateAgentToken(ctx context.Context, client *gitlabclient.Client, input CreateAgentTokenInput) (AgentTokenItem, error) {
	if input.AgentID <= 0 {
		return AgentTokenItem{}, toolutil.ErrRequiredInt64("gitlab_create_cluster_agent_token", "agent_id")
	}
	opts := &gl.CreateAgentTokenOptions{
		Name: new(input.Name),
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	t, _, err := client.GL().ClusterAgents.CreateAgentToken(string(input.ProjectID), input.AgentID, opts, gl.WithContext(ctx))
	if err != nil {
		return AgentTokenItem{}, toolutil.WrapErrWithStatusHint("gitlab_create_cluster_agent_token", err, http.StatusBadRequest,
			"name must be unique within the agent; max 2 active tokens per agent (revoke an existing token first); save the returned token value \u2014 it cannot be retrieved later")
	}
	return AgentTokenItem{
		ID: t.ID, Name: t.Name, Description: t.Description,
		AgentID: t.AgentID, Status: t.Status, Token: t.Token,
	}, nil
}

// RevokeAgentToken.

// RevokeAgentTokenInput defines parameters for the revoke agent token operation.
type RevokeAgentTokenInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AgentID   int64                `json:"agent_id" jsonschema:"Agent ID,required"`
	TokenID   int64                `json:"token_id" jsonschema:"Token ID,required"`
}

// RevokeAgentToken revokes agent token for the clusteragents package.
func RevokeAgentToken(ctx context.Context, client *gitlabclient.Client, input RevokeAgentTokenInput) error {
	if input.AgentID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_revoke_cluster_agent_token", "agent_id")
	}
	if input.TokenID <= 0 {
		return toolutil.ErrRequiredInt64("gitlab_revoke_cluster_agent_token", "token_id")
	}
	_, err := client.GL().ClusterAgents.RevokeAgentToken(string(input.ProjectID), input.AgentID, input.TokenID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_revoke_cluster_agent_token", err, http.StatusForbidden,
			"revoking cluster agent tokens requires Maintainer role; revocation is irreversible \u2014 the token cannot be reactivated")
	}
	return nil
}

// formatters.
