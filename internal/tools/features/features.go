// Package features implements MCP tools for GitLab Features (feature flags) API.
package features

import (
	"context"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Types.

// GateItem represents a gate on a feature flag.
type GateItem struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

// DefinitionItem represents a feature definition.
type DefinitionItem struct {
	Name            string `json:"name"`
	IntroducedByURL string `json:"introduced_by_url,omitempty"`
	RolloutIssueURL string `json:"rollout_issue_url,omitempty"`
	Milestone       string `json:"milestone,omitempty"`
	LogStateChanges bool   `json:"log_state_changes"`
	Type            string `json:"type,omitempty"`
	Group           string `json:"group,omitempty"`
	DefaultEnabled  bool   `json:"default_enabled"`
}

// FeatureItem represents a feature flag.
type FeatureItem struct {
	Name       string          `json:"name"`
	State      string          `json:"state"`
	Gates      []GateItem      `json:"gates,omitempty"`
	Definition *DefinitionItem `json:"definition,omitempty"`
}

// ListInput is the input for listing features.
type ListInput struct{}

// ListOutput is the output for listing features.
type ListOutput struct {
	toolutil.HintableOutput
	Features []FeatureItem `json:"features"`
}

// ListDefinitionsInput is the input for listing feature definitions.
type ListDefinitionsInput struct{}

// ListDefinitionsOutput is the output for listing feature definitions.
type ListDefinitionsOutput struct {
	toolutil.HintableOutput
	Definitions []DefinitionItem `json:"definitions"`
}

// SetInput is the input for setting a feature flag.
type SetInput struct {
	Name         string `json:"name"          jsonschema:"Feature flag name,required"`
	Value        any    `json:"value"         jsonschema:"Value to set (true, false, integer percentage, or string),required"`
	Key          string `json:"key,omitempty"            jsonschema:"Gate key (percentage_of_actors or percentage_of_time)"`
	FeatureGroup string `json:"feature_group,omitempty"  jsonschema:"Feature group name"`
	User         string `json:"user,omitempty"           jsonschema:"GitLab username"`
	Group        string `json:"group,omitempty"          jsonschema:"GitLab group path"`
	Namespace    string `json:"namespace,omitempty"      jsonschema:"GitLab namespace path"`
	Project      string `json:"project,omitempty"        jsonschema:"GitLab project path (namespace/project)"`
	Repository   string `json:"repository,omitempty"     jsonschema:"GitLab repository path"`
	Force        bool   `json:"force,omitempty"          jsonschema:"Force the change even if the flag is read-only"`
}

// SetOutput is the output for setting a feature flag.
type SetOutput struct {
	toolutil.HintableOutput
	Feature FeatureItem `json:"feature"`
}

// DeleteInput is the input for deleting a feature flag.
type DeleteInput struct {
	Name string `json:"name" jsonschema:"Feature flag name to delete,required"`
}

// Helpers.

// toGateItem converts the GitLab API response to the tool output format.
func toGateItem(g gl.Gate) GateItem {
	return GateItem{Key: g.Key, Value: g.Value}
}

// toDefinitionItem converts the GitLab API response to the tool output format.
func toDefinitionItem(d *gl.FeatureDefinition) *DefinitionItem {
	if d == nil {
		return nil
	}
	return &DefinitionItem{
		Name:            d.Name,
		IntroducedByURL: d.IntroducedByURL,
		RolloutIssueURL: d.RolloutIssueURL,
		Milestone:       d.Milestone,
		LogStateChanges: d.LogStateChanges,
		Type:            d.Type,
		Group:           d.Group,
		DefaultEnabled:  d.DefaultEnabled,
	}
}

// toFeatureItem converts the GitLab API response to the tool output format.
func toFeatureItem(f *gl.Feature) FeatureItem {
	gates := make([]GateItem, 0, len(f.Gates))
	for _, g := range f.Gates {
		gates = append(gates, toGateItem(g))
	}
	return FeatureItem{
		Name:       f.Name,
		State:      f.State,
		Gates:      gates,
		Definition: toDefinitionItem(f.Definition),
	}
}

// Handlers.

// List retrieves all feature flags.
func List(ctx context.Context, client *gitlabclient.Client, _ ListInput) (ListOutput, error) {
	features, _, err := client.GL().Features.ListFeatures(gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("feature_list", err)
	}

	items := make([]FeatureItem, 0, len(features))
	for _, f := range features {
		items = append(items, toFeatureItem(f))
	}
	return ListOutput{Features: items}, nil
}

// ListDefinitions retrieves all feature definitions.
func ListDefinitions(ctx context.Context, client *gitlabclient.Client, _ ListDefinitionsInput) (ListDefinitionsOutput, error) {
	defs, _, err := client.GL().Features.ListFeatureDefinitions(gl.WithContext(ctx))
	if err != nil {
		return ListDefinitionsOutput{}, toolutil.WrapErrWithMessage("feature_list_definitions", err)
	}

	items := make([]DefinitionItem, 0, len(defs))
	for _, d := range defs {
		items = append(items, DefinitionItem{
			Name:            d.Name,
			IntroducedByURL: d.IntroducedByURL,
			RolloutIssueURL: d.RolloutIssueURL,
			Milestone:       d.Milestone,
			LogStateChanges: d.LogStateChanges,
			Type:            d.Type,
			Group:           d.Group,
			DefaultEnabled:  d.DefaultEnabled,
		})
	}
	return ListDefinitionsOutput{Definitions: items}, nil
}

// Set creates or updates a feature flag.
// Uses a raw HTTP request to work around upstream client-go issue where
// SetFeatureFlagOptions fields lack omitempty, causing GitLab to reject
// the request with "mutually exclusive" errors for empty string fields.
func Set(ctx context.Context, client *gitlabclient.Client, input SetInput) (SetOutput, error) {
	body := map[string]any{"value": input.Value}
	if input.Force {
		body["force"] = true
	}
	if input.Key != "" {
		body["key"] = input.Key
	}
	if input.FeatureGroup != "" {
		body["feature_group"] = input.FeatureGroup
	}
	if input.User != "" {
		body["user"] = input.User
	}
	if input.Group != "" {
		body["group"] = input.Group
	}
	if input.Namespace != "" {
		body["namespace"] = input.Namespace
	}
	if input.Project != "" {
		body["project"] = input.Project
	}
	if input.Repository != "" {
		body["repository"] = input.Repository
	}

	path := fmt.Sprintf("features/%s", gl.PathEscape(input.Name))
	req, err := client.GL().NewRequest("POST", path, body, nil)
	if err != nil {
		return SetOutput{}, toolutil.WrapErrWithMessage("feature_set", err)
	}
	req = req.WithContext(ctx)

	var feature gl.Feature
	if _, err = client.GL().Do(req, &feature); err != nil {
		return SetOutput{}, toolutil.WrapErrWithMessage("feature_set", err)
	}
	return SetOutput{Feature: toFeatureItem(&feature)}, nil
}

// Delete removes a feature flag.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	_, err := client.GL().Features.DeleteFeatureFlag(input.Name, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("feature_delete", err)
	}
	return nil
}

// Markdown formatters.

// formatGates renders the result as a formatted string.
func formatGates(gates []GateItem) string {
	if len(gates) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(gates))
	for _, g := range gates {
		parts = append(parts, fmt.Sprintf("%s=%v", g.Key, g.Value))
	}
	return strings.Join(parts, ", ")
}
