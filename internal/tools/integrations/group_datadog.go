package integrations

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// GroupDatadogItem is a JSON-serializable view of [gl.GroupDatadogIntegration]
// returned by the group-level Datadog integration tools. The struct flattens
// the embedded [gl.Integration] fields and the Datadog-specific fields so the
// LLM-facing schema stays stable even when client-go adds new embedded types.
type GroupDatadogItem struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	Slug               string `json:"slug"`
	Active             bool   `json:"active"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
	APIURL             string `json:"api_url,omitempty"`
	DatadogEnv         string `json:"datadog_env,omitempty"`
	DatadogService     string `json:"datadog_service,omitempty"`
	DatadogSite        string `json:"datadog_site,omitempty"`
	DatadogTags        string `json:"datadog_tags,omitempty"`
	ArchiveTraceEvents *bool  `json:"archive_trace_events,omitempty"`
}

func groupDatadogToItem(g *gl.GroupDatadogIntegration) GroupDatadogItem {
	if g == nil {
		return GroupDatadogItem{}
	}
	item := GroupDatadogItem{
		ID:             g.ID,
		Title:          g.Title,
		Slug:           g.Slug,
		Active:         g.Active,
		APIURL:         g.APIURL,
		DatadogEnv:     g.DatadogEnv,
		DatadogService: g.DatadogService,
		DatadogSite:    g.DatadogSite,
		DatadogTags:    g.DatadogTags,
	}
	if g.CreatedAt != nil {
		item.CreatedAt = g.CreatedAt.String()
	}
	if g.UpdatedAt != nil {
		item.UpdatedAt = g.UpdatedAt.String()
	}
	item.ArchiveTraceEvents = g.ArchiveTraceEvents
	return item
}

// GetGroupDatadog (read).

// GetGroupDatadogInput is the input for fetching the Datadog integration of a group.
type GetGroupDatadogInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// GetGroupDatadogOutput is the output of [GetGroupDatadog].
type GetGroupDatadogOutput struct {
	toolutil.HintableOutput
	Integration GroupDatadogItem `json:"integration"`
}

// GetGroupDatadog retrieves the Datadog integration configured for a group.
// Requires Owner role on the group; the endpoint is GitLab.com Enterprise/Premium only.
func GetGroupDatadog(ctx context.Context, client *gitlabclient.Client, input GetGroupDatadogInput) (GetGroupDatadogOutput, error) {
	integration, _, err := client.GL().Integrations.GetGroupDatadogIntegration(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return GetGroupDatadogOutput{}, toolutil.WrapErrWithStatusHint("get_group_datadog_integration", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; the Datadog integration must be active on the group; this endpoint requires GitLab.com Premium/Ultimate")
	}
	if integration == nil {
		return GetGroupDatadogOutput{}, toolutil.WrapErrWithMessage("get_group_datadog_integration",
			errors.New("get_group_datadog_integration: response integration was nil"))
	}
	return GetGroupDatadogOutput{Integration: groupDatadogToItem(integration)}, nil
}

// SetGroupDatadog (mutate).

// SetGroupDatadogInput is the input for creating or updating the Datadog
// integration of a group. At least one Datadog field must be set, or
// UseInheritedSettings=true to inherit settings from an ancestor group.
type SetGroupDatadogInput struct {
	GroupID              toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	APIKey               string               `json:"api_key,omitempty" jsonschema:"Datadog API key (write-only; never returned by the get endpoint)"`
	APIURL               string               `json:"api_url,omitempty" jsonschema:"Datadog API URL (e.g. https://api.datadoghq.com)"`
	DatadogEnv           string               `json:"datadog_env,omitempty" jsonschema:"Datadog env tag forwarded with every log/metric"`
	DatadogService       string               `json:"datadog_service,omitempty" jsonschema:"Datadog service tag forwarded with every log/metric"`
	DatadogSite          string               `json:"datadog_site,omitempty" jsonschema:"Datadog site (e.g. datadoghq.com, datadoghq.eu, us3.datadoghq.com, us5.datadoghq.com, ap1.datadoghq.com)"`
	DatadogTags          string               `json:"datadog_tags,omitempty" jsonschema:"Comma-separated Datadog tags forwarded with every log/metric"`
	ArchiveTraceEvents   *bool                `json:"archive_trace_events,omitempty" jsonschema:"Forward CI job trace events to Datadog"`
	UseInheritedSettings *bool                `json:"use_inherited_settings,omitempty" jsonschema:"Inherit the Datadog configuration from the closest ancestor group that has one configured (overrides all per-field settings)"`
}

// SetGroupDatadogOutput is the output of [SetGroupDatadog].
type SetGroupDatadogOutput struct {
	toolutil.HintableOutput
	Integration GroupDatadogItem `json:"integration"`
}

// SetGroupDatadog creates or updates the Datadog integration for a group.
// Requires Owner role; requires GitLab.com Premium/Ultimate.
func SetGroupDatadog(ctx context.Context, client *gitlabclient.Client, input SetGroupDatadogInput) (SetGroupDatadogOutput, error) {
	if input.UseInheritedSettings == nil && input.APIKey == "" && input.APIURL == "" &&
		input.DatadogEnv == "" && input.DatadogService == "" && input.DatadogSite == "" &&
		input.DatadogTags == "" && input.ArchiveTraceEvents == nil {
		return SetGroupDatadogOutput{}, toolutil.WrapErrWithMessage("set_group_datadog_integration",
			toolutil.ErrFieldRequired("at least one of: api_key, api_url, datadog_env, datadog_service, datadog_site, datadog_tags, archive_trace_events, use_inherited_settings"))
	}

	opts := &gl.GroupDatadogIntegrationOptions{}
	if input.APIKey != "" {
		opts.APIKey = new(input.APIKey)
	}
	if input.APIURL != "" {
		opts.APIURL = new(input.APIURL)
	}
	if input.DatadogEnv != "" {
		opts.DatadogEnv = new(input.DatadogEnv)
	}
	if input.DatadogService != "" {
		opts.DatadogService = new(input.DatadogService)
	}
	if input.DatadogSite != "" {
		opts.DatadogSite = new(input.DatadogSite)
	}
	if input.DatadogTags != "" {
		opts.DatadogTags = new(input.DatadogTags)
	}
	if input.ArchiveTraceEvents != nil {
		opts.ArchiveTraceEvents = input.ArchiveTraceEvents
	}
	if input.UseInheritedSettings != nil {
		opts.UseInheritedSettings = input.UseInheritedSettings
	}

	integration, _, err := client.GL().Integrations.SetGroupDatadogIntegration(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return SetGroupDatadogOutput{}, toolutil.WrapErrWithStatusHint("set_group_datadog_integration", err, http.StatusForbidden,
			"requires Owner role on the group and GitLab.com Premium/Ultimate; verify group_id with gitlab_group_get; api_key is mandatory unless use_inherited_settings=true")
	}
	if integration == nil {
		return SetGroupDatadogOutput{}, toolutil.WrapErrWithMessage("set_group_datadog_integration",
			errors.New("set_group_datadog_integration: response integration was nil"))
	}
	return SetGroupDatadogOutput{Integration: groupDatadogToItem(integration)}, nil
}

// DeleteGroupDatadog (destructive).

// DeleteGroupDatadogInput is the input for removing the Datadog integration from a group.
type DeleteGroupDatadogInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// DeleteGroupDatadog removes the Datadog integration configuration from a group.
// Requires Owner role on the group; deletion is irreversible and clears the
// stored API key.
func DeleteGroupDatadog(ctx context.Context, client *gitlabclient.Client, input DeleteGroupDatadogInput) error {
	if _, err := client.GL().Integrations.DeleteGroupDatadogIntegration(string(input.GroupID), gl.WithContext(ctx)); err != nil {
		return toolutil.WrapErrWithStatusHint("delete_group_datadog_integration", err, http.StatusForbidden,
			"requires Owner role on the group and GitLab.com Premium/Ultimate; verify group_id with gitlab_group_get; deletion is irreversible (the stored API key is cleared)")
	}
	return nil
}
