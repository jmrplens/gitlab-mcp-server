package integrations

import (
	"context"
	"errors"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GroupDatadogItem is a JSON-serializable view of [gl.GroupDatadogIntegration]
// returned by the group-level Datadog integration tools.
//
// The Datadog configuration has one home here, the nested Properties object,
// which is what GitLab's own record says the endpoint sends
// (APIEntitiesIntegration carries `properties` and no flat Datadog field).
// The flat copies this type published beside it during v2 are gone in v3;
// client-go still declares its own deprecated flat fields, and they are read
// only as the fallback below.
//
// The embedded [gl.Integration] base fields (identity, lifecycle, and the
// full set of event trigger flags) stay flattened; event flags are plain bool
// in client-go and are surfaced unconditionally (no omitempty) so a false
// value is explicit in the output.
type GroupDatadogItem struct {
	ID                             int64  `json:"id"`
	Title                          string `json:"title"`
	Slug                           string `json:"slug"`
	Active                         bool   `json:"active"`
	CreatedAt                      string `json:"created_at,omitempty"`
	UpdatedAt                      string `json:"updated_at,omitempty"`
	AlertEvents                    bool   `json:"alert_events"`
	CommitEvents                   bool   `json:"commit_events"`
	ConfidentialIssuesEvents       bool   `json:"confidential_issues_events"`
	ConfidentialNoteEvents         bool   `json:"confidential_note_events"`
	DeploymentEvents               bool   `json:"deployment_events"`
	GroupConfidentialMentionEvents bool   `json:"group_confidential_mention_events"`
	GroupMentionEvents             bool   `json:"group_mention_events"`
	IncidentEvents                 bool   `json:"incident_events"`
	IssuesEvents                   bool   `json:"issues_events"`
	JobEvents                      bool   `json:"job_events"`
	MergeRequestsEvents            bool   `json:"merge_requests_events"`
	NoteEvents                     bool   `json:"note_events"`
	PipelineEvents                 bool   `json:"pipeline_events"`
	PushEvents                     bool   `json:"push_events"`
	TagPushEvents                  bool   `json:"tag_push_events"`
	VulnerabilityEvents            bool   `json:"vulnerability_events"`
	WikiPageEvents                 bool   `json:"wiki_page_events"`
	CommentOnEventEnabled          bool   `json:"comment_on_event_enabled"`
	Inherited                      bool   `json:"inherited"`

	Properties *GroupDatadogProperties `json:"properties,omitempty" jsonschema:"The Datadog configuration GitLab returns for the integration"`
}

// GroupDatadogProperties mirrors [gl.GroupDatadogIntegrationProperties]
// field for field (same plain types): the nested "properties" object GitLab
// returns with the group Datadog integration. It is the canonical home of the
// Datadog configuration, and the only one [GroupDatadogItem] publishes.
type GroupDatadogProperties struct {
	APIURL              string `json:"api_url"`
	DatadogEnv          string `json:"datadog_env"`
	DatadogService      string `json:"datadog_service"`
	DatadogSite         string `json:"datadog_site"`
	DatadogTags         string `json:"datadog_tags"`
	DatadogCIVisibility bool   `json:"datadog_ci_visibility"`
	ArchiveTraceEvents  bool   `json:"archive_trace_events"`
}

func groupDatadogToItem(g *gl.GroupDatadogIntegration) GroupDatadogItem {
	if g == nil {
		return GroupDatadogItem{}
	}
	item := GroupDatadogItem{
		ID:                             g.ID,
		Title:                          g.Title,
		Slug:                           g.Slug,
		Active:                         g.Active,
		AlertEvents:                    g.AlertEvents,
		CommitEvents:                   g.CommitEvents,
		ConfidentialIssuesEvents:       g.ConfidentialIssuesEvents,
		ConfidentialNoteEvents:         g.ConfidentialNoteEvents,
		DeploymentEvents:               g.DeploymentEvents,
		GroupConfidentialMentionEvents: g.GroupConfidentialMentionEvents,
		GroupMentionEvents:             g.GroupMentionEvents,
		IncidentEvents:                 g.IncidentEvents,
		IssuesEvents:                   g.IssuesEvents,
		JobEvents:                      g.JobEvents,
		MergeRequestsEvents:            g.MergeRequestsEvents,
		NoteEvents:                     g.NoteEvents,
		PipelineEvents:                 g.PipelineEvents,
		PushEvents:                     g.PushEvents,
		TagPushEvents:                  g.TagPushEvents,
		VulnerabilityEvents:            g.VulnerabilityEvents,
		WikiPageEvents:                 g.WikiPageEvents,
		CommentOnEventEnabled:          g.CommentOnEventEnabled,
		Inherited:                      g.Inherited,
	}
	if g.CreatedAt != nil {
		item.CreatedAt = g.CreatedAt.UTC().Format(time.RFC3339)
	}
	if g.UpdatedAt != nil {
		item.UpdatedAt = g.UpdatedAt.UTC().Format(time.RFC3339)
	}
	if p := g.Properties; p != nil {
		item.Properties = &GroupDatadogProperties{
			APIURL:              p.APIURL,
			DatadogEnv:          p.DatadogEnv,
			DatadogService:      p.DatadogService,
			DatadogSite:         p.DatadogSite,
			DatadogTags:         p.DatadogTags,
			DatadogCIVisibility: p.DatadogCIVisibility,
			ArchiveTraceEvents:  p.ArchiveTraceEvents,
		}
		return item
	}
	// An older GitLab omits the nested object and sends the configuration
	// flat, which is what client-go's own deprecated fields decode. Reading
	// them here is what lets this type publish one shape without losing an
	// old server's answer. Such a payload never carried datadog_ci_visibility,
	// so that field reads false because the server said nothing about it.
	item.Properties = &GroupDatadogProperties{
		APIURL:         g.APIURL,         //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
		DatadogEnv:     g.DatadogEnv,     //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
		DatadogService: g.DatadogService, //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
		DatadogSite:    g.DatadogSite,    //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
		DatadogTags:    g.DatadogTags,    //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
	}
	if g.ArchiveTraceEvents != nil { //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
		item.Properties.ArchiveTraceEvents = *g.ArchiveTraceEvents //nolint:staticcheck // SA1019: the flat fields are what an older server sends.
	}
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
// Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com).
func GetGroupDatadog(ctx context.Context, client *gitlabclient.Client, input GetGroupDatadogInput) (GetGroupDatadogOutput, error) {
	integration, _, err := client.GL().Integrations.GetGroupDatadogIntegration(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return GetGroupDatadogOutput{}, toolutil.WrapErrWithStatusHint("get_group_datadog_integration", err, http.StatusNotFound,
			"verify group_id with gitlab_group_get; the Datadog integration must be active on the group; requires Owner role on the group and GitLab Premium/Ultimate")
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
	APIKey               string               `json:"api_key,omitempty" jsonschema:"Datadog API key (write-only. Never returned by the get endpoint)"`
	APIURL               string               `json:"api_url,omitempty" jsonschema:"Datadog API URL (e.g. https://api.datadoghq.com)"`
	DatadogEnv           string               `json:"datadog_env,omitempty" jsonschema:"Datadog env tag forwarded with every log/metric"`
	DatadogService       string               `json:"datadog_service,omitempty" jsonschema:"Datadog service tag forwarded with every log/metric"`
	DatadogSite          string               `json:"datadog_site,omitempty" jsonschema:"Datadog site (e.g. datadoghq.com, datadoghq.eu, us3.datadoghq.com, us5.datadoghq.com, ap1.datadoghq.com)"`
	DatadogTags          string               `json:"datadog_tags,omitempty" jsonschema:"Comma-separated Datadog tags forwarded with every log/metric"`
	DatadogCIVisibility  *bool                `json:"datadog_ci_visibility,omitempty" jsonschema:"Enable Datadog CI Visibility: forward pipeline/job data to Datadog CI Visibility"`
	ArchiveTraceEvents   *bool                `json:"archive_trace_events,omitempty" jsonschema:"Forward CI job trace events to Datadog"`
	UseInheritedSettings *bool                `json:"use_inherited_settings,omitempty" jsonschema:"Inherit the Datadog configuration from the closest ancestor group that has one configured (overrides all per-field settings)"`
}

// SetGroupDatadogOutput is the output of [SetGroupDatadog].
type SetGroupDatadogOutput struct {
	toolutil.HintableOutput
	Integration GroupDatadogItem `json:"integration"`
}

// buildGroupDatadogOptions maps the tool input onto the SDK options struct,
// sending only the fields the caller actually set (empty strings and nil
// pointers are omitted so unset fields are never overwritten server-side).
func buildGroupDatadogOptions(input SetGroupDatadogInput) *gl.GroupDatadogIntegrationOptions {
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
	if input.DatadogCIVisibility != nil {
		opts.DatadogCIVisibility = input.DatadogCIVisibility
	}
	if input.ArchiveTraceEvents != nil {
		opts.ArchiveTraceEvents = input.ArchiveTraceEvents
	}
	if input.UseInheritedSettings != nil {
		opts.UseInheritedSettings = input.UseInheritedSettings
	}
	return opts
}

// SetGroupDatadog creates or updates the Datadog integration for a group.
// Requires Owner role and GitLab Premium/Ultimate (self-managed EE or GitLab.com).
func SetGroupDatadog(ctx context.Context, client *gitlabclient.Client, input SetGroupDatadogInput) (SetGroupDatadogOutput, error) {
	useInherited := input.UseInheritedSettings != nil && *input.UseInheritedSettings
	if !useInherited && input.APIKey == "" && input.APIURL == "" &&
		input.DatadogEnv == "" && input.DatadogService == "" && input.DatadogSite == "" &&
		input.DatadogTags == "" && input.DatadogCIVisibility == nil && input.ArchiveTraceEvents == nil {
		return SetGroupDatadogOutput{}, toolutil.WrapErrWithMessage("set_group_datadog_integration",
			toolutil.ErrFieldRequired("at least one of: api_key, api_url, datadog_env, datadog_service, datadog_site, datadog_tags, datadog_ci_visibility, archive_trace_events, use_inherited_settings=true"))
	}

	integration, _, err := client.GL().Integrations.SetGroupDatadogIntegration(string(input.GroupID), buildGroupDatadogOptions(input), gl.WithContext(ctx))
	if err != nil {
		return SetGroupDatadogOutput{}, toolutil.WrapErrWithStatusHint("set_group_datadog_integration", err, http.StatusForbidden,
			"requires Owner role on the group and GitLab Premium/Ultimate (self-managed EE or GitLab.com); verify group_id with gitlab_group_get; provide at least one of api_key, api_url, datadog_env, datadog_service, datadog_site, datadog_tags, datadog_ci_visibility, archive_trace_events, or use_inherited_settings=true")
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
			"requires Owner role on the group and GitLab Premium/Ultimate (self-managed EE or GitLab.com); verify group_id with gitlab_group_get; deletion is irreversible (the stored API key is cleared)")
	}
	return nil
}
