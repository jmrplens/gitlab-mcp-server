// Package projects implements GitLab project operations including create, get,
// list, delete, update, restore, fork, star, unstar, archive, unarchive,
// transfer, list forks, get languages, webhook management (list, get, add,
// edit, delete, trigger test), user/group/starrer listings, share/unshare
// with groups, invited groups, push rules (get, add, edit, delete), and
// user contributed/starred project listings. It exposes typed input/output
// structs and handler functions registered as MCP tools.
package projects

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// boolToAccessLevel converts a bool pointer to an AccessControlValue pointer
// for bridging legacy bool-based tool inputs to the modern AccessLevel API.
func boolToAccessLevel(b *bool) *gl.AccessControlValue {
	if b == nil {
		return nil
	}
	if *b {
		return new(gl.EnabledAccessControl)
	}
	return new(gl.DisabledAccessControl)
}

// accessLevelEnabled returns true if the access level indicates the feature is not disabled.
func accessLevelEnabled(v gl.AccessControlValue) bool {
	return v != "" && v != gl.DisabledAccessControl
}

// CreateInput defines parameters for creating a GitLab project.
type CreateInput struct {
	// Basic metadata
	Name                 string   `json:"name" jsonschema:"Project name,required"`
	Path                 string   `json:"path,omitempty" jsonschema:"Project path slug (defaults from name)"`
	NamespaceID          int      `json:"namespace_id,omitempty" jsonschema:"Namespace ID (defaults to personal namespace)"`
	Description          string   `json:"description,omitempty" jsonschema:"Project description"`
	Visibility           string   `json:"visibility,omitempty" jsonschema:"Visibility level (private, internal, public)"`
	InitializeWithReadme bool     `json:"initialize_with_readme,omitempty" jsonschema:"Initialize with a README"`
	DefaultBranch        string   `json:"default_branch,omitempty" jsonschema:"Default branch name"`
	Topics               []string `json:"topics,omitempty" jsonschema:"Topic tags for the project"`
	ImportURL            string   `json:"import_url,omitempty" jsonschema:"URL to import repository from"`

	// Merge settings
	MergeMethod                               string `json:"merge_method,omitempty" jsonschema:"Merge method (merge, rebase_merge, ff)"`
	SquashOption                              string `json:"squash_option,omitempty" jsonschema:"Squash option (never, always, default_on, default_off)"`
	OnlyAllowMergeIfPipelineSucceeds          bool   `json:"only_allow_merge_if_pipeline_succeeds,omitempty" jsonschema:"Only allow merge when pipeline succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool   `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Only allow merge when all discussions are resolved"`
	AllowMergeOnSkippedPipeline               *bool  `json:"allow_merge_on_skipped_pipeline,omitempty" jsonschema:"Allow merge when pipeline is skipped"`
	RemoveSourceBranchAfterMerge              *bool  `json:"remove_source_branch_after_merge,omitempty" jsonschema:"Remove source branch after merge by default"`
	AutocloseReferencedIssues                 *bool  `json:"autoclose_referenced_issues,omitempty" jsonschema:"Auto-close referenced issues on merge"`
	SuggestionCommitMessage                   string `json:"suggestion_commit_message,omitempty" jsonschema:"Default commit message for suggestions"`

	// Feature toggles
	IssuesEnabled              *bool  `json:"issues_enabled,omitempty" jsonschema:"Enable issues feature"`
	MergeRequestsEnabled       *bool  `json:"merge_requests_enabled,omitempty" jsonschema:"Enable merge requests feature"`
	WikiEnabled                *bool  `json:"wiki_enabled,omitempty" jsonschema:"Enable wiki feature"`
	JobsEnabled                *bool  `json:"jobs_enabled,omitempty" jsonschema:"Enable CI/CD jobs"`
	LFSEnabled                 *bool  `json:"lfs_enabled,omitempty" jsonschema:"Enable Git LFS"`
	PackagesEnabled            *bool  `json:"packages_enabled,omitempty" jsonschema:"Enable packages feature (deprecated: use package_registry_access_level)"`
	PackageRegistryAccessLevel string `json:"package_registry_access_level,omitempty" jsonschema:"Package registry access level (disabled, private, enabled)"`

	// CI/CD settings
	CIConfigPath               string `json:"ci_config_path,omitempty" jsonschema:"Custom CI/CD configuration file path"`
	BuildTimeout               int64  `json:"build_timeout,omitempty" jsonschema:"Build timeout in seconds"`
	CIForwardDeploymentEnabled *bool  `json:"ci_forward_deployment_enabled,omitempty" jsonschema:"Enable CI/CD forward deployment"`
	SharedRunnersEnabled       *bool  `json:"shared_runners_enabled,omitempty" jsonschema:"Enable shared runners"`
	PublicBuilds               *bool  `json:"public_builds,omitempty" jsonschema:"Enable public access to pipelines"`

	// Access control
	RequestAccessEnabled         *bool  `json:"request_access_enabled,omitempty" jsonschema:"Allow users to request access"`
	PagesAccessLevel             string `json:"pages_access_level,omitempty" jsonschema:"Pages access level (disabled, private, enabled, public)"`
	ContainerRegistryAccessLevel string `json:"container_registry_access_level,omitempty" jsonschema:"Container registry access level (disabled, private, enabled)"`
	SnippetsAccessLevel          string `json:"snippets_access_level,omitempty" jsonschema:"Snippets access level (disabled, private, enabled)"`
}

// Output is the common output for project operations.
type Output struct {
	toolutil.HintableOutput
	ID                                        int64    `json:"id"`
	Name                                      string   `json:"name"`
	Path                                      string   `json:"path"`
	PathWithNamespace                         string   `json:"path_with_namespace"`
	NameWithNamespace                         string   `json:"name_with_namespace,omitempty"`
	Visibility                                string   `json:"visibility"`
	DefaultBranch                             string   `json:"default_branch"`
	WebURL                                    string   `json:"web_url"`
	Description                               string   `json:"description"`
	Archived                                  bool     `json:"archived"`
	EmptyRepo                                 bool     `json:"empty_repo,omitempty"`
	ForksCount                                int64    `json:"forks_count,omitempty"`
	StarCount                                 int64    `json:"star_count,omitempty"`
	OpenIssuesCount                           int64    `json:"open_issues_count,omitempty"`
	HTTPURLToRepo                             string   `json:"http_url_to_repo,omitempty"`
	SSHURLToRepo                              string   `json:"ssh_url_to_repo,omitempty"`
	Namespace                                 string   `json:"namespace,omitempty"`
	Topics                                    []string `json:"topics"`
	MergeMethod                               string   `json:"merge_method,omitempty"`
	SquashOption                              string   `json:"squash_option,omitempty"`
	OnlyAllowMergeIfPipelineSucceeds          bool     `json:"only_allow_merge_if_pipeline_succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved bool     `json:"only_allow_merge_if_all_discussions_are_resolved"`
	RemoveSourceBranchAfterMerge              bool     `json:"remove_source_branch_after_merge"`
	ForkedFromProject                         string   `json:"forked_from_project,omitempty"`
	MarkedForDeletionOn                       string   `json:"marked_for_deletion_on,omitempty"`
	CreatedAt                                 string   `json:"created_at"`
	UpdatedAt                                 string   `json:"updated_at,omitempty"`
	LastActivityAt                            string   `json:"last_activity_at,omitempty"`
	ReadmeURL                                 string   `json:"readme_url,omitempty"`
	AvatarURL                                 string   `json:"avatar_url,omitempty"`
	CreatorID                                 int64    `json:"creator_id,omitempty"`
	RequestAccessEnabled                      bool     `json:"request_access_enabled"`
	IssuesEnabled                             bool     `json:"issues_enabled"`
	MergeRequestsEnabled                      bool     `json:"merge_requests_enabled"`
	WikiEnabled                               bool     `json:"wiki_enabled"`
	JobsEnabled                               bool     `json:"jobs_enabled"`
	LFSEnabled                                bool     `json:"lfs_enabled"`
	CIConfigPath                              string   `json:"ci_config_path,omitempty"`
	AllowMergeOnSkippedPipeline               bool     `json:"allow_merge_on_skipped_pipeline"`
	MergePipelinesEnabled                     bool     `json:"merge_pipelines_enabled"`
	MergeTrainsEnabled                        bool     `json:"merge_trains_enabled"`
	MergeCommitTemplate                       string   `json:"merge_commit_template,omitempty"`
	SquashCommitTemplate                      string   `json:"squash_commit_template,omitempty"`
	AutocloseReferencedIssues                 bool     `json:"autoclose_referenced_issues"`
	ApprovalsBeforeMerge                      int64    `json:"approvals_before_merge,omitempty"`
	ResolveOutdatedDiffDiscussions            bool     `json:"resolve_outdated_diff_discussions"`
	ContainerRegistryEnabled                  bool     `json:"container_registry_enabled,omitempty"`
	SharedRunnersEnabled                      bool     `json:"shared_runners_enabled,omitempty"`
	PublicBuilds                              bool     `json:"public_builds,omitempty"`
	SnippetsEnabled                           bool     `json:"snippets_enabled,omitempty"`
	PackagesEnabled                           bool     `json:"packages_enabled,omitempty"`
	PackageRegistryAccessLevel                string   `json:"package_registry_access_level,omitempty"`
	BuildTimeout                              int64    `json:"build_timeout,omitempty"`
	SuggestionCommitMessage                   string   `json:"suggestion_commit_message,omitempty"`
	ComplianceFrameworks                      []string `json:"compliance_frameworks,omitempty"`
	ImportURL                                 string   `json:"import_url,omitempty"`
	MergeRequestTitleRegex                    string   `json:"merge_request_title_regex,omitempty"`
	MergeRequestTitleRegexDescription         string   `json:"merge_request_title_regex_description,omitempty"`
}

// GetInput defines parameters for retrieving a project.
type GetInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path (e.g. 'user/repo' or 42),required"`
	Statistics           *bool                `json:"statistics,omitempty" jsonschema:"Include project statistics (commit count, storage sizes)"`
	License              *bool                `json:"license,omitempty" jsonschema:"Include license information in response"`
	WithCustomAttributes *bool                `json:"with_custom_attributes,omitempty" jsonschema:"Include custom attributes in response"`
}

// ListInput defines filters for listing projects.
type ListInput struct {
	Owned                    bool   `json:"owned,omitempty"      jsonschema:"Limit to projects explicitly owned by the current user"`
	Search                   string `json:"search,omitempty"     jsonschema:"Search query for project name"`
	Visibility               string `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	Archived                 *bool  `json:"archived,omitempty"   jsonschema:"Filter by archived status (true=only archived, false=only active)"`
	OrderBy                  string `json:"order_by,omitempty"   jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort                     string `json:"sort,omitempty"       jsonschema:"Sort direction (asc, desc)"`
	Topic                    string `json:"topic,omitempty"      jsonschema:"Filter by topic name"`
	Simple                   bool   `json:"simple,omitempty"     jsonschema:"Return only limited fields (faster for large result sets)"`
	MinAccessLevel           int    `json:"min_access_level,omitempty" jsonschema:"Filter by minimum access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner)"`
	LastActivityAfter        string `json:"last_activity_after,omitempty"  jsonschema:"Return projects with last activity after date (ISO 8601 format)"`
	LastActivityBefore       string `json:"last_activity_before,omitempty" jsonschema:"Return projects with last activity before date (ISO 8601 format)"`
	Starred                  *bool  `json:"starred,omitempty"              jsonschema:"Limit to projects starred by the current user"`
	Membership               *bool  `json:"membership,omitempty"           jsonschema:"Limit to projects where current user is a member"`
	WithIssuesEnabled        *bool  `json:"with_issues_enabled,omitempty"  jsonschema:"Filter by projects with issues feature enabled"`
	WithMergeRequestsEnabled *bool  `json:"with_merge_requests_enabled,omitempty" jsonschema:"Filter by projects with merge requests enabled"`
	SearchNamespaces         *bool  `json:"search_namespaces,omitempty"    jsonschema:"Include namespace in search"`
	Statistics               *bool  `json:"statistics,omitempty"           jsonschema:"Include project statistics in response"`
	WithProgrammingLanguage  string `json:"with_programming_language,omitempty" jsonschema:"Filter by programming language name"`
	IncludePendingDelete     *bool  `json:"include_pending_delete,omitempty"    jsonschema:"Include projects that are marked/scheduled for deletion. Default false."`
	IncludeHidden            *bool  `json:"include_hidden,omitempty"            jsonschema:"Include hidden projects in results"`
	IDAfter                  int64  `json:"id_after,omitempty"                  jsonschema:"Return projects with ID greater than this value (keyset pagination)"`
	IDBefore                 int64  `json:"id_before,omitempty"                 jsonschema:"Return projects with ID less than this value (keyset pagination)"`
	toolutil.PaginationInput
}

// ListOutput holds a paginated list of projects.
type ListOutput struct {
	toolutil.HintableOutput
	Projects   []Output                  `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// DeleteInput defines parameters for deleting a project.
type DeleteInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PermanentlyRemove bool                 `json:"permanently_remove,omitempty" jsonschema:"If true, immediately and permanently delete the project bypassing delayed deletion. Requires admin permissions on some GitLab instances."`
	FullPath          string               `json:"full_path,omitempty" jsonschema:"Full path of the project to confirm permanent removal (required when permanently_remove is true)"`
}

// DeleteOutput holds the result of a project deletion request.
type DeleteOutput struct {
	toolutil.HintableOutput
	Status              string `json:"status"`
	Message             string `json:"message"`
	MarkedForDeletionOn string `json:"marked_for_deletion_on,omitempty"`
	PermanentlyRemoved  bool   `json:"permanently_removed"`
}

// RestoreInput defines parameters for restoring a project marked for deletion.
type RestoreInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path of the project marked for deletion,required"`
}

// UpdateInput defines parameters for updating project settings.
type UpdateInput struct {
	ProjectID                                 toolutil.StringOrInt `json:"project_id"     jsonschema:"Project ID or URL-encoded path,required"`
	Name                                      string               `json:"name,omitempty"          jsonschema:"New project name"`
	Description                               string               `json:"description,omitempty"   jsonschema:"New description"`
	Visibility                                string               `json:"visibility,omitempty"    jsonschema:"Visibility level (private, internal, public)"`
	DefaultBranch                             string               `json:"default_branch,omitempty" jsonschema:"New default branch"`
	MergeMethod                               string               `json:"merge_method,omitempty"  jsonschema:"Merge method (merge, rebase_merge, ff)"`
	Topics                                    []string             `json:"topics,omitempty"        jsonschema:"Topic tags for the project"`
	SquashOption                              string               `json:"squash_option,omitempty" jsonschema:"Squash option (never, always, default_on, default_off)"`
	OnlyAllowMergeIfPipelineSucceeds          *bool                `json:"only_allow_merge_if_pipeline_succeeds,omitempty" jsonschema:"Only allow merge when pipeline succeeds"`
	OnlyAllowMergeIfAllDiscussionsAreResolved *bool                `json:"only_allow_merge_if_all_discussions_are_resolved,omitempty" jsonschema:"Only allow merge when all discussions are resolved"`
	IssuesEnabled                             *bool                `json:"issues_enabled,omitempty"           jsonschema:"Enable/disable issues feature (use 'issues_enabled' not 'issues_access_level')"`
	MergeRequestsEnabled                      *bool                `json:"merge_requests_enabled,omitempty"   jsonschema:"Enable merge requests feature"`
	WikiEnabled                               *bool                `json:"wiki_enabled,omitempty"             jsonschema:"Enable wiki feature"`
	JobsEnabled                               *bool                `json:"jobs_enabled,omitempty"             jsonschema:"Enable CI/CD jobs"`
	LFSEnabled                                *bool                `json:"lfs_enabled,omitempty"              jsonschema:"Enable Git LFS"`
	RequestAccessEnabled                      *bool                `json:"request_access_enabled,omitempty"   jsonschema:"Allow users to request access"`
	SharedRunnersEnabled                      *bool                `json:"shared_runners_enabled,omitempty"   jsonschema:"Enable shared runners"`
	PublicBuilds                              *bool                `json:"public_builds,omitempty"            jsonschema:"Enable public access to pipelines"`
	PackagesEnabled                           *bool                `json:"packages_enabled,omitempty"         jsonschema:"Enable packages feature (deprecated: use package_registry_access_level)"`
	PackageRegistryAccessLevel                string               `json:"package_registry_access_level,omitempty" jsonschema:"Package registry access level (disabled, private, enabled)"`
	PagesAccessLevel                          string               `json:"pages_access_level,omitempty"       jsonschema:"Pages access level (disabled, private, enabled, public)"`
	ContainerRegistryAccessLevel              string               `json:"container_registry_access_level,omitempty" jsonschema:"Container registry access level (disabled, private, enabled)"`
	SnippetsAccessLevel                       string               `json:"snippets_access_level,omitempty"    jsonschema:"Snippets access level (disabled, private, enabled)"`
	CIConfigPath                              string               `json:"ci_config_path,omitempty"           jsonschema:"Custom CI/CD configuration file path"`
	AllowMergeOnSkippedPipeline               *bool                `json:"allow_merge_on_skipped_pipeline,omitempty" jsonschema:"Allow merge when pipeline is skipped"`
	RemoveSourceBranchAfterMerge              *bool                `json:"remove_source_branch_after_merge,omitempty" jsonschema:"Remove source branch after merge by default"`
	AutocloseReferencedIssues                 *bool                `json:"autoclose_referenced_issues,omitempty" jsonschema:"Auto-close referenced issues on merge"`
	MergeCommitTemplate                       string               `json:"merge_commit_template,omitempty"    jsonschema:"Template for merge commit messages"`
	SquashCommitTemplate                      string               `json:"squash_commit_template,omitempty"   jsonschema:"Template for squash commit messages"`
	MergePipelinesEnabled                     *bool                `json:"merge_pipelines_enabled,omitempty"  jsonschema:"Enable merged results pipelines"`
	MergeTrainsEnabled                        *bool                `json:"merge_trains_enabled,omitempty"     jsonschema:"Enable merge trains"`
	ResolveOutdatedDiffDiscussions            *bool                `json:"resolve_outdated_diff_discussions,omitempty" jsonschema:"Auto-resolve outdated diff discussions"`
	ApprovalsBeforeMerge                      int64                `json:"approvals_before_merge,omitempty"   jsonschema:"Number of approvals required before merge"`
	MergeRequestTitleRegex                    string               `json:"merge_request_title_regex,omitempty" jsonschema:"Regex that MR titles must match"`
	MergeRequestTitleRegexDescription         string               `json:"merge_request_title_regex_description,omitempty" jsonschema:"Human-readable description for the MR title regex"`
}

// ToOutput converts a GitLab API [gl.Project] to the MCP tool output
// format, mapping visibility to its string representation.
func ToOutput(p *gl.Project) Output {
	out := Output{
		ID:                               p.ID,
		Name:                             p.Name,
		Path:                             p.Path,
		PathWithNamespace:                p.PathWithNamespace,
		NameWithNamespace:                p.NameWithNamespace,
		Visibility:                       string(p.Visibility),
		DefaultBranch:                    p.DefaultBranch,
		WebURL:                           p.WebURL,
		Description:                      p.Description,
		Archived:                         p.Archived,
		EmptyRepo:                        p.EmptyRepo,
		ForksCount:                       p.ForksCount,
		StarCount:                        p.StarCount,
		OpenIssuesCount:                  p.OpenIssuesCount,
		HTTPURLToRepo:                    p.HTTPURLToRepo,
		SSHURLToRepo:                     p.SSHURLToRepo,
		Topics:                           p.Topics,
		MergeMethod:                      string(p.MergeMethod),
		SquashOption:                     string(p.SquashOption),
		OnlyAllowMergeIfPipelineSucceeds: p.OnlyAllowMergeIfPipelineSucceeds,
		OnlyAllowMergeIfAllDiscussionsAreResolved: p.OnlyAllowMergeIfAllDiscussionsAreResolved,
		RemoveSourceBranchAfterMerge:              p.RemoveSourceBranchAfterMerge,
		ReadmeURL:                                 p.ReadmeURL,
		AvatarURL:                                 p.AvatarURL,
		CreatorID:                                 p.CreatorID,
		RequestAccessEnabled:                      p.RequestAccessEnabled,
		IssuesEnabled:                             accessLevelEnabled(p.IssuesAccessLevel),
		MergeRequestsEnabled:                      accessLevelEnabled(p.MergeRequestsAccessLevel),
		WikiEnabled:                               accessLevelEnabled(p.WikiAccessLevel),
		JobsEnabled:                               accessLevelEnabled(p.BuildsAccessLevel),
		LFSEnabled:                                p.LFSEnabled,
		CIConfigPath:                              p.CIConfigPath,
		AllowMergeOnSkippedPipeline:               p.AllowMergeOnSkippedPipeline,
		MergePipelinesEnabled:                     p.MergePipelinesEnabled,
		MergeTrainsEnabled:                        p.MergeTrainsEnabled,
		MergeCommitTemplate:                       p.MergeCommitTemplate,
		SquashCommitTemplate:                      p.SquashCommitTemplate,
		AutocloseReferencedIssues:                 p.AutocloseReferencedIssues,
		//lint:ignore SA1019 no replacement field on Project struct
		ApprovalsBeforeMerge:           p.ApprovalsBeforeMerge, //nolint:staticcheck // SA1019: no replacement field
		ResolveOutdatedDiffDiscussions: p.ResolveOutdatedDiffDiscussions,
		ContainerRegistryEnabled:       accessLevelEnabled(p.ContainerRegistryAccessLevel),
		SharedRunnersEnabled:           p.SharedRunnersEnabled,
		PublicBuilds:                   p.PublicJobs,
		SnippetsEnabled:                accessLevelEnabled(p.SnippetsAccessLevel),
		//lint:ignore SA1019 backward compat with PackagesEnabled field
		PackagesEnabled:                   p.PackagesEnabled, //nolint:staticcheck // SA1019: use PackageRegistryAccessLevel
		PackageRegistryAccessLevel:        string(p.PackageRegistryAccessLevel),
		BuildTimeout:                      p.BuildTimeout,
		SuggestionCommitMessage:           p.SuggestionCommitMessage,
		ComplianceFrameworks:              p.ComplianceFrameworks,
		ImportURL:                         p.ImportURL,
		MergeRequestTitleRegex:            p.MergeRequestTitleRegex,
		MergeRequestTitleRegexDescription: p.MergeRequestTitleRegexDescription,
	}
	if out.Topics == nil {
		out.Topics = []string{}
	}
	if p.Namespace != nil {
		out.Namespace = p.Namespace.FullPath
	}
	if p.ForkedFromProject != nil {
		out.ForkedFromProject = p.ForkedFromProject.PathWithNamespace
	}
	if p.MarkedForDeletionOn != nil {
		out.MarkedForDeletionOn = time.Time(*p.MarkedForDeletionOn).Format("2006-01-02")
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = p.UpdatedAt.Format(time.RFC3339)
	}
	if p.LastActivityAt != nil {
		out.LastActivityAt = p.LastActivityAt.Format(time.RFC3339)
	}
	return out
}

// buildCreateOpts maps CreateInput fields to the GitLab API create options.
func buildCreateOpts(input CreateInput) *gl.CreateProjectOptions {
	opts := &gl.CreateProjectOptions{Name: new(input.Name)}
	if input.NamespaceID != 0 {
		opts.NamespaceID = new(int64(input.NamespaceID))
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.InitializeWithReadme {
		opts.InitializeWithReadme = new(true)
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if len(input.Topics) > 0 {
		opts.Topics = &input.Topics
	}
	if input.MergeMethod != "" {
		opts.MergeMethod = new(gl.MergeMethodValue(input.MergeMethod))
	}
	if input.SquashOption != "" {
		opts.SquashOption = new(gl.SquashOptionValue(input.SquashOption))
	}
	if input.OnlyAllowMergeIfPipelineSucceeds {
		opts.OnlyAllowMergeIfPipelineSucceeds = new(true)
	}
	if input.OnlyAllowMergeIfAllDiscussionsAreResolved {
		opts.OnlyAllowMergeIfAllDiscussionsAreResolved = new(true)
	}
	applyCreateFeatureOpts(opts, input)
	return opts
}

// applyCreateFeatureOpts sets optional feature toggles on the create options.
func applyCreateFeatureOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	applyCreateFeatureToggles(opts, input)
	applyCreateBuildOpts(opts, input)
	applyCreateAccessLevels(opts, input)
}

// applyCreateFeatureToggles is an internal helper for the projects package.
func applyCreateFeatureToggles(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.IssuesEnabled != nil {
		opts.IssuesAccessLevel = boolToAccessLevel(input.IssuesEnabled)
	}
	if input.MergeRequestsEnabled != nil {
		opts.MergeRequestsAccessLevel = boolToAccessLevel(input.MergeRequestsEnabled)
	}
	if input.WikiEnabled != nil {
		opts.WikiAccessLevel = boolToAccessLevel(input.WikiEnabled)
	}
	if input.JobsEnabled != nil {
		opts.BuildsAccessLevel = boolToAccessLevel(input.JobsEnabled)
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.CIConfigPath != "" {
		opts.CIConfigPath = new(input.CIConfigPath)
	}
	if input.AllowMergeOnSkippedPipeline != nil {
		opts.AllowMergeOnSkippedPipeline = input.AllowMergeOnSkippedPipeline
	}
	if input.RemoveSourceBranchAfterMerge != nil {
		opts.RemoveSourceBranchAfterMerge = input.RemoveSourceBranchAfterMerge
	}
	if input.AutocloseReferencedIssues != nil {
		opts.AutocloseReferencedIssues = input.AutocloseReferencedIssues
	}
}

// applyCreateBuildOpts is an internal helper for the projects package.
func applyCreateBuildOpts(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.ImportURL != "" {
		opts.ImportURL = new(input.ImportURL)
	}
	if input.BuildTimeout > 0 {
		opts.BuildTimeout = new(input.BuildTimeout)
	}
	if input.SharedRunnersEnabled != nil {
		opts.SharedRunnersEnabled = input.SharedRunnersEnabled
	}
	if input.PublicBuilds != nil {
		//lint:ignore SA1019 CreateProjectOptions lacks PublicJobs field
		opts.PublicBuilds = input.PublicBuilds //nolint:staticcheck // SA1019: no PublicJobs field
	}
	if input.PackagesEnabled != nil {
		//lint:ignore SA1019 backward compat with PackagesEnabled field
		opts.PackagesEnabled = input.PackagesEnabled //nolint:staticcheck // SA1019: use PackageRegistryAccessLevel
	}
	if input.PackageRegistryAccessLevel != "" {
		opts.PackageRegistryAccessLevel = new(gl.AccessControlValue(input.PackageRegistryAccessLevel))
	}
	if input.SuggestionCommitMessage != "" {
		opts.SuggestionCommitMessage = new(input.SuggestionCommitMessage)
	}
}

// applyCreateAccessLevels is an internal helper for the projects package.
func applyCreateAccessLevels(opts *gl.CreateProjectOptions, input CreateInput) {
	if input.PagesAccessLevel != "" {
		opts.PagesAccessLevel = new(gl.AccessControlValue(input.PagesAccessLevel))
	}
	if input.ContainerRegistryAccessLevel != "" {
		opts.ContainerRegistryAccessLevel = new(gl.AccessControlValue(input.ContainerRegistryAccessLevel))
	}
	if input.SnippetsAccessLevel != "" {
		opts.SnippetsAccessLevel = new(gl.AccessControlValue(input.SnippetsAccessLevel))
	}
}

// Create creates a new GitLab project with the specified settings.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	opts := buildCreateOpts(input)
	p, _, err := client.GL().Projects.CreateProject(opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("projectCreate", err, "check that the project name/path is unique in the target namespace and all required fields are valid")
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("projectCreate", err, "a project with this name already exists in the namespace — use gitlab_project_list to verify")
		default:
			return Output{}, toolutil.WrapErrWithMessage("projectCreate", err)
		}
	}
	return ToOutput(p), nil
}

// Get retrieves a single GitLab project by its ID or URL-encoded path.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.GetProjectOptions{}
	if input.Statistics != nil {
		opts.Statistics = input.Statistics
	}
	if input.License != nil {
		opts.License = input.License
	}
	if input.WithCustomAttributes != nil {
		opts.WithCustomAttributes = input.WithCustomAttributes
	}
	p, _, err := client.GL().Projects.GetProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return Output{}, toolutil.WrapErrWithHint("projectGet", err,
				"verify project_id (numeric ID or URL-encoded full path like 'group%2Fsubgroup%2Fproject'); use gitlab_project_list with a search term to discover the correct ID")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectGet", err)
	}
	return ToOutput(p), nil
}

// buildListOpts maps ListInput fields to the GitLab API list options.
func buildListOpts(input ListInput) *gl.ListProjectsOptions {
	opts := &gl.ListProjectsOptions{}
	if input.Owned {
		opts.Owned = new(true)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.Archived != nil {
		opts.Archived = input.Archived
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Topic != "" {
		opts.Topic = new(input.Topic)
	}
	if input.Simple {
		opts.Simple = new(true)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	opts.LastActivityAfter = toolutil.ParseOptionalTime(input.LastActivityAfter)
	opts.LastActivityBefore = toolutil.ParseOptionalTime(input.LastActivityBefore)
	applyListFilterOpts(opts, input)
	return opts
}

// applyListFilterOpts sets optional boolean-pointer filters and pagination.
func applyListFilterOpts(opts *gl.ListProjectsOptions, input ListInput) {
	if input.Starred != nil {
		opts.Starred = input.Starred
	}
	if input.Membership != nil {
		opts.Membership = input.Membership
	}
	if input.WithIssuesEnabled != nil {
		opts.WithIssuesEnabled = input.WithIssuesEnabled
	}
	if input.WithMergeRequestsEnabled != nil {
		opts.WithMergeRequestsEnabled = input.WithMergeRequestsEnabled
	}
	if input.SearchNamespaces != nil {
		opts.SearchNamespaces = input.SearchNamespaces
	}
	if input.Statistics != nil {
		opts.Statistics = input.Statistics
	}
	if input.WithProgrammingLanguage != "" {
		opts.WithProgrammingLanguage = new(input.WithProgrammingLanguage)
	}
	if input.IncludePendingDelete != nil {
		opts.IncludePendingDelete = input.IncludePendingDelete
	}
	if input.IncludeHidden != nil {
		opts.IncludeHidden = input.IncludeHidden
	}
	if input.IDAfter > 0 {
		opts.IDAfter = new(input.IDAfter)
	}
	if input.IDBefore > 0 {
		opts.IDBefore = new(input.IDBefore)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
}

// List retrieves a paginated list of GitLab projects.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := buildListOpts(input)
	projects, resp, err := client.GL().Projects.ListProjects(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectList", err)
	}
	out := make([]Output, len(projects))
	for i, p := range projects {
		out[i] = ToOutput(p)
	}
	return ListOutput{Projects: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// Delete deletes a GitLab project by its ID or URL-encoded path.
// When the GitLab instance has delayed deletion enabled, the project is marked
// for deletion rather than removed immediately. When permanently_remove is true
// and the instance requires a two-step process (mark-then-remove), the handler
// performs both steps automatically.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	if input.ProjectID == "" {
		return DeleteOutput{}, errors.New("projectDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.DeleteProjectOptions{}
	if input.PermanentlyRemove {
		opts.PermanentlyRemove = new(true)
		if input.FullPath != "" {
			opts.FullPath = new(input.FullPath)
		}
	}

	_, err := client.GL().Projects.DeleteProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.ContainsAny(err, "already been marked for deletion", "already marked for deletion") {
			return DeleteOutput{
				Status:             "already_scheduled",
				Message:            fmt.Sprintf("Project %s is already marked for deletion. Use permanently_remove=true with full_path to delete immediately, or use gitlab_project_restore to cancel the deletion.", input.ProjectID),
				PermanentlyRemoved: false,
			}, nil
		}

		// GitLab CE may require marking for deletion before permanent removal.
		if input.PermanentlyRemove && toolutil.ContainsAny(err, "must be marked for deletion first", "marked for deletion first") {
			return deleteTwoStep(ctx, client, input)
		}

		return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete", err)
	}

	if input.PermanentlyRemove {
		return DeleteOutput{
			Status:             "success",
			Message:            fmt.Sprintf("Project %s has been permanently deleted.", input.ProjectID),
			PermanentlyRemoved: true,
		}, nil
	}

	// Check if the project still exists (marked for delayed deletion)
	p, _, getErr := client.GL().Projects.GetProject(string(input.ProjectID), &gl.GetProjectOptions{}, gl.WithContext(ctx))
	if getErr == nil && p.MarkedForDeletionOn != nil {
		deletionDate := time.Time(*p.MarkedForDeletionOn).Format("2006-01-02")
		return DeleteOutput{
			Status:              "scheduled",
			Message:             fmt.Sprintf("Project %s is marked for deletion on %s. Use gitlab_project_delete with permanently_remove=true and full_path to delete immediately, or use gitlab_project_restore to cancel the deletion.", input.ProjectID, deletionDate),
			MarkedForDeletionOn: deletionDate,
			PermanentlyRemoved:  false,
		}, nil
	}

	return DeleteOutput{
		Status:             "success",
		Message:            fmt.Sprintf("Project %s has been deleted.", input.ProjectID),
		PermanentlyRemoved: true,
	}, nil
}

// deleteTwoStep handles GitLab instances that require marking a project
// for deletion before it can be permanently removed (e.g. CE with delayed deletion).
func deleteTwoStep(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	// Step 1: Mark for deletion (no options).
	_, err := client.GL().Projects.DeleteProject(string(input.ProjectID), nil, gl.WithContext(ctx))
	if err != nil {
		if !toolutil.ContainsAny(err, "already been marked for deletion", "already marked for deletion") {
			return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete (mark)", err)
		}
	}

	// The project path changes after marking for deletion (GitLab appends "-deletion_scheduled-{ID}").
	// Re-fetch the current path.
	fullPath := input.FullPath
	if fullPath != "" {
		p, _, getErr := client.GL().Projects.GetProject(string(input.ProjectID), &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if getErr == nil {
			fullPath = p.PathWithNamespace
		}
	}

	// Step 2: Permanently remove.
	permOpts := &gl.DeleteProjectOptions{
		PermanentlyRemove: new(true),
	}
	if fullPath != "" {
		permOpts.FullPath = &fullPath
	}
	_, err = client.GL().Projects.DeleteProject(string(input.ProjectID), permOpts, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithMessage("projectDelete (permanent)", err)
	}
	return DeleteOutput{
		Status:             "success",
		Message:            fmt.Sprintf("Project %s has been permanently deleted (two-step: marked then removed).", input.ProjectID),
		PermanentlyRemoved: true,
	}, nil
}

// Restore restores a GitLab project that has been marked for deletion.
func Restore(ctx context.Context, client *gitlabclient.Client, input RestoreInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectRestore: project_id is required. Use gitlab_project_list with include_pending_delete=true to find projects marked for deletion")
	}
	p, _, err := client.GL().Projects.RestoreProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectRestore", err)
	}
	return ToOutput(p), nil
}

// buildUpdateOpts maps UpdateInput fields to the GitLab API edit options.
func buildUpdateOpts(input UpdateInput) *gl.EditProjectOptions {
	opts := &gl.EditProjectOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(toolutil.NormalizeText(input.Description))
	}
	if input.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(input.Visibility))
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = new(input.DefaultBranch)
	}
	if input.MergeMethod != "" {
		opts.MergeMethod = new(gl.MergeMethodValue(input.MergeMethod))
	}
	if len(input.Topics) > 0 {
		opts.Topics = &input.Topics
	}
	if input.SquashOption != "" {
		opts.SquashOption = new(gl.SquashOptionValue(input.SquashOption))
	}
	if input.OnlyAllowMergeIfPipelineSucceeds != nil {
		opts.OnlyAllowMergeIfPipelineSucceeds = input.OnlyAllowMergeIfPipelineSucceeds
	}
	if input.OnlyAllowMergeIfAllDiscussionsAreResolved != nil {
		opts.OnlyAllowMergeIfAllDiscussionsAreResolved = input.OnlyAllowMergeIfAllDiscussionsAreResolved
	}
	if input.IssuesEnabled != nil {
		opts.IssuesAccessLevel = boolToAccessLevel(input.IssuesEnabled)
	}
	if input.MergeRequestsEnabled != nil {
		opts.MergeRequestsAccessLevel = boolToAccessLevel(input.MergeRequestsEnabled)
	}
	if input.WikiEnabled != nil {
		opts.WikiAccessLevel = boolToAccessLevel(input.WikiEnabled)
	}
	if input.JobsEnabled != nil {
		opts.BuildsAccessLevel = boolToAccessLevel(input.JobsEnabled)
	}
	applyUpdateFeatureOpts(opts, input)
	return opts
}

// applyUpdateFeatureOpts sets optional feature toggles and advanced merge settings.
func applyUpdateFeatureOpts(opts *gl.EditProjectOptions, input UpdateInput) {
	if input.CIConfigPath != "" {
		opts.CIConfigPath = new(input.CIConfigPath)
	}
	if input.AllowMergeOnSkippedPipeline != nil {
		opts.AllowMergeOnSkippedPipeline = input.AllowMergeOnSkippedPipeline
	}
	if input.RemoveSourceBranchAfterMerge != nil {
		opts.RemoveSourceBranchAfterMerge = input.RemoveSourceBranchAfterMerge
	}
	if input.AutocloseReferencedIssues != nil {
		opts.AutocloseReferencedIssues = input.AutocloseReferencedIssues
	}
	if input.MergeCommitTemplate != "" {
		opts.MergeCommitTemplate = new(input.MergeCommitTemplate)
	}
	if input.SquashCommitTemplate != "" {
		opts.SquashCommitTemplate = new(input.SquashCommitTemplate)
	}
	if input.MergePipelinesEnabled != nil {
		opts.MergePipelinesEnabled = input.MergePipelinesEnabled
	}
	if input.MergeTrainsEnabled != nil {
		opts.MergeTrainsEnabled = input.MergeTrainsEnabled
	}
	if input.ResolveOutdatedDiffDiscussions != nil {
		opts.ResolveOutdatedDiffDiscussions = input.ResolveOutdatedDiffDiscussions
	}
	if input.ApprovalsBeforeMerge > 0 {
		//lint:ignore SA1019 no replacement field, needs Merge Request Approvals API
		opts.ApprovalsBeforeMerge = new(input.ApprovalsBeforeMerge) //nolint:staticcheck // SA1019: no replacement field
	}
	if input.LFSEnabled != nil {
		opts.LFSEnabled = input.LFSEnabled
	}
	if input.RequestAccessEnabled != nil {
		opts.RequestAccessEnabled = input.RequestAccessEnabled
	}
	if input.SharedRunnersEnabled != nil {
		opts.SharedRunnersEnabled = input.SharedRunnersEnabled
	}
	if input.PublicBuilds != nil {
		opts.PublicJobs = input.PublicBuilds
	}
	if input.PackagesEnabled != nil {
		//lint:ignore SA1019 backward compat with PackagesEnabled field
		opts.PackagesEnabled = input.PackagesEnabled //nolint:staticcheck // SA1019: use PackageRegistryAccessLevel
	}
	if input.PackageRegistryAccessLevel != "" {
		opts.PackageRegistryAccessLevel = new(gl.AccessControlValue(input.PackageRegistryAccessLevel))
	}
	if input.PagesAccessLevel != "" {
		opts.PagesAccessLevel = new(gl.AccessControlValue(input.PagesAccessLevel))
	}
	if input.ContainerRegistryAccessLevel != "" {
		opts.ContainerRegistryAccessLevel = new(gl.AccessControlValue(input.ContainerRegistryAccessLevel))
	}
	if input.SnippetsAccessLevel != "" {
		opts.SnippetsAccessLevel = new(gl.AccessControlValue(input.SnippetsAccessLevel))
	}
	if input.MergeRequestTitleRegex != "" {
		opts.MergeRequestTitleRegex = new(input.MergeRequestTitleRegex)
	}
	if input.MergeRequestTitleRegexDescription != "" {
		opts.MergeRequestTitleRegexDescription = new(input.MergeRequestTitleRegexDescription)
	}
}

// Update modifies settings on an existing GitLab project.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUpdate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := buildUpdateOpts(input)
	p, _, err := client.GL().Projects.EditProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("projectUpdate", err, "check that the project settings are valid — name/path must be unique in the namespace")
		case toolutil.IsHTTPStatus(err, http.StatusForbidden):
			return Output{}, toolutil.WrapErrWithHint("projectUpdate", err, "you need at least Maintainer role to update project settings")
		default:
			return Output{}, toolutil.WrapErrWithMessage("projectUpdate", err)
		}
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Fork
// ---------------------------------------------------------------------------.

// ForkInput defines parameters for forking a project.
type ForkInput struct {
	ProjectID                     toolutil.StringOrInt `json:"project_id" jsonschema:"Source project ID or URL-encoded path,required"`
	Name                          string               `json:"name,omitempty" jsonschema:"Name for the forked project"`
	Path                          string               `json:"path,omitempty" jsonschema:"Path slug for the forked project"`
	NamespaceID                   int64                `json:"namespace_id,omitempty" jsonschema:"Namespace ID to fork into"`
	NamespacePath                 string               `json:"namespace_path,omitempty" jsonschema:"Namespace path to fork into"`
	Description                   string               `json:"description,omitempty" jsonschema:"Description for the forked project"`
	Visibility                    string               `json:"visibility,omitempty" jsonschema:"Visibility level (private, internal, public)"`
	Branches                      string               `json:"branches,omitempty" jsonschema:"Branches to fork (empty=all)"`
	MergeRequestDefaultTargetSelf *bool                `json:"mr_default_target_self,omitempty" jsonschema:"MR default target is the fork itself instead of upstream"`
}

// Fork creates a fork of an existing GitLab project.
func Fork(ctx context.Context, client *gitlabclient.Client, input ForkInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectFork: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ForkProjectOptions{}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.NamespaceID > 0 {
		opts.NamespaceID = new(input.NamespaceID)
	}
	if input.NamespacePath != "" {
		opts.NamespacePath = new(input.NamespacePath)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if input.Branches != "" {
		opts.Branches = new(input.Branches)
	}
	if input.MergeRequestDefaultTargetSelf != nil {
		opts.MergeRequestDefaultTargetSelf = input.MergeRequestDefaultTargetSelf
	}
	p, _, err := client.GL().Projects.ForkProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return Output{}, toolutil.WrapErrWithHint("projectFork", err, "a fork of this project already exists in your namespace — use gitlab_project_list to find it")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectFork", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Star / Unstar
// ---------------------------------------------------------------------------.

// StarInput defines parameters for starring a project.
type StarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Star adds a star to a project for the authenticated user.
func Star(ctx context.Context, client *gitlabclient.Client, input StarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectStar: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.StarProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectStar", err)
	}
	return ToOutput(p), nil
}

// UnstarInput defines parameters for unstarring a project.
type UnstarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Unstar removes the star from a project for the authenticated user.
func Unstar(ctx context.Context, client *gitlabclient.Client, input UnstarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUnstar: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.UnstarProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectUnstar", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Archive / Unarchive
// ---------------------------------------------------------------------------.

// ArchiveInput defines parameters for archiving a project.
type ArchiveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Archive sets a project to archived (read-only) state.
func Archive(ctx context.Context, client *gitlabclient.Client, input ArchiveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectArchive: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.ArchiveProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectArchive", err, "only project owners can archive projects")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectArchive", err)
	}
	return ToOutput(p), nil
}

// UnarchiveInput defines parameters for unarchiving a project.
type UnarchiveInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// Unarchive removes the archived (read-only) state from a project.
func Unarchive(ctx context.Context, client *gitlabclient.Client, input UnarchiveInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUnarchive: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	p, _, err := client.GL().Projects.UnarchiveProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return Output{}, toolutil.WrapErrWithHint("projectUnarchive", err, "only project owners can unarchive projects")
		}
		return Output{}, toolutil.WrapErrWithMessage("projectUnarchive", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// Transfer
// ---------------------------------------------------------------------------.

// TransferInput defines parameters for transferring a project to another namespace.
type TransferInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Namespace string               `json:"namespace" jsonschema:"Target namespace ID or path,required"`
}

// Transfer moves a project to a different namespace.
func Transfer(ctx context.Context, client *gitlabclient.Client, input TransferInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectTransfer: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.Namespace == "" {
		return Output{}, errors.New("projectTransfer: namespace is required. Provide the target namespace ID or path (e.g. 'my-group' or '42')")
	}
	opts := &gl.TransferProjectOptions{
		Namespace: input.Namespace,
	}
	p, _, err := client.GL().Projects.TransferProject(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectTransfer", err)
	}
	return ToOutput(p), nil
}

// ---------------------------------------------------------------------------
// List Forks
// ---------------------------------------------------------------------------.

// ListForksInput defines parameters for listing project forks.
type ListForksInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Owned      bool                 `json:"owned,omitempty" jsonschema:"Limit to forks owned by the current user"`
	Search     string               `json:"search,omitempty" jsonschema:"Search query for fork name"`
	Visibility string               `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	OrderBy    string               `json:"order_by,omitempty" jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort       string               `json:"sort,omitempty" jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListForksOutput holds a paginated list of project forks.
type ListForksOutput struct {
	toolutil.HintableOutput
	Forks      []Output                  `json:"forks"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListForks retrieves a paginated list of forks for a project.
func ListForks(ctx context.Context, client *gitlabclient.Client, input ListForksInput) (ListForksOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListForksOutput{}, err
	}
	if input.ProjectID == "" {
		return ListForksOutput{}, errors.New("projectListForks: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}
	if input.Owned {
		opts.Owned = new(input.Owned)
	}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	forks, resp, err := client.GL().Projects.ListProjectForks(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListForksOutput{}, toolutil.WrapErrWithMessage("projectListForks", err)
	}
	out := make([]Output, 0, len(forks))
	for _, f := range forks {
		out = append(out, ToOutput(f))
	}
	return ListForksOutput{
		Forks:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// Get Languages
// ---------------------------------------------------------------------------.

// GetLanguagesInput defines parameters for retrieving project languages.
type GetLanguagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// LanguageEntry represents a single programming language and its percentage.
type LanguageEntry struct {
	Name       string  `json:"name"`
	Percentage float32 `json:"percentage"`
}

// LanguagesOutput holds the programming languages detected in a project.
type LanguagesOutput struct {
	toolutil.HintableOutput
	Languages []LanguageEntry `json:"languages"`
}

// GetLanguages retrieves the programming languages used in a project with percentages.
func GetLanguages(ctx context.Context, client *gitlabclient.Client, input GetLanguagesInput) (LanguagesOutput, error) {
	if err := ctx.Err(); err != nil {
		return LanguagesOutput{}, err
	}
	if input.ProjectID == "" {
		return LanguagesOutput{}, errors.New("projectGetLanguages: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	langs, _, err := client.GL().Projects.GetProjectLanguages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return LanguagesOutput{}, toolutil.WrapErrWithMessage("projectGetLanguages", err)
	}
	entries := make([]LanguageEntry, 0, len(*langs))
	for name, pct := range *langs {
		entries = append(entries, LanguageEntry{Name: name, Percentage: pct})
	}
	return LanguagesOutput{Languages: entries}, nil
}

// ---------------------------------------------------------------------------
// Project Hooks (Webhooks)
// ---------------------------------------------------------------------------.

// HookOutput represents a project webhook.
type HookOutput struct {
	toolutil.HintableOutput
	ID                        int64     `json:"id"`
	URL                       string    `json:"url"`
	Name                      string    `json:"name,omitempty"`
	Description               string    `json:"description,omitempty"`
	ProjectID                 int64     `json:"project_id"`
	PushEvents                bool      `json:"push_events"`
	PushEventsBranchFilter    string    `json:"push_events_branch_filter,omitempty"`
	IssuesEvents              bool      `json:"issues_events"`
	ConfidentialIssuesEvents  bool      `json:"confidential_issues_events"`
	MergeRequestsEvents       bool      `json:"merge_requests_events"`
	TagPushEvents             bool      `json:"tag_push_events"`
	NoteEvents                bool      `json:"note_events"`
	ConfidentialNoteEvents    bool      `json:"confidential_note_events"`
	JobEvents                 bool      `json:"job_events"`
	PipelineEvents            bool      `json:"pipeline_events"`
	WikiPageEvents            bool      `json:"wiki_page_events"`
	DeploymentEvents          bool      `json:"deployment_events"`
	ReleasesEvents            bool      `json:"releases_events"`
	MilestoneEvents           bool      `json:"milestone_events"`
	FeatureFlagEvents         bool      `json:"feature_flag_events"`
	EmojiEvents               bool      `json:"emoji_events"`
	EnableSSLVerification     bool      `json:"enable_ssl_verification"`
	RepositoryUpdateEvents    bool      `json:"repository_update_events"`
	ResourceAccessTokenEvents bool      `json:"resource_access_token_events"`
	AlertStatus               string    `json:"alert_status,omitempty"`
	BranchFilterStrategy      string    `json:"branch_filter_strategy,omitempty"`
	CustomWebhookTemplate     string    `json:"custom_webhook_template,omitempty"`
	CreatedAt                 time.Time `json:"created_at"`
}

// hookOutputFromGL is an internal helper for the projects package.
func hookOutputFromGL(h *gl.ProjectHook) HookOutput {
	out := HookOutput{
		ID:                        h.ID,
		URL:                       h.URL,
		Name:                      h.Name,
		Description:               h.Description,
		ProjectID:                 h.ProjectID,
		PushEvents:                h.PushEvents,
		PushEventsBranchFilter:    h.PushEventsBranchFilter,
		IssuesEvents:              h.IssuesEvents,
		ConfidentialIssuesEvents:  h.ConfidentialIssuesEvents,
		MergeRequestsEvents:       h.MergeRequestsEvents,
		TagPushEvents:             h.TagPushEvents,
		NoteEvents:                h.NoteEvents,
		ConfidentialNoteEvents:    h.ConfidentialNoteEvents,
		JobEvents:                 h.JobEvents,
		PipelineEvents:            h.PipelineEvents,
		WikiPageEvents:            h.WikiPageEvents,
		DeploymentEvents:          h.DeploymentEvents,
		ReleasesEvents:            h.ReleasesEvents,
		MilestoneEvents:           h.MilestoneEvents,
		FeatureFlagEvents:         h.FeatureFlagEvents,
		EmojiEvents:               h.EmojiEvents,
		EnableSSLVerification:     h.EnableSSLVerification,
		RepositoryUpdateEvents:    h.RepositoryUpdateEvents,
		ResourceAccessTokenEvents: h.ResourceAccessTokenEvents,
		AlertStatus:               h.AlertStatus,
		BranchFilterStrategy:      h.BranchFilterStrategy,
		CustomWebhookTemplate:     h.CustomWebhookTemplate,
	}
	if h.CreatedAt != nil {
		out.CreatedAt = *h.CreatedAt
	}
	return out
}

// ListHooksInput defines parameters for listing project webhooks.
type ListHooksInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ListHooksOutput holds a paginated list of project webhooks.
type ListHooksOutput struct {
	toolutil.HintableOutput
	Hooks      []HookOutput              `json:"hooks"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListHooks retrieves all webhooks for a project.
func ListHooks(ctx context.Context, client *gitlabclient.Client, input ListHooksInput) (ListHooksOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListHooksOutput{}, err
	}
	if input.ProjectID == "" {
		return ListHooksOutput{}, errors.New("projectListHooks: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectHooksOptions{
		ListOptions: gl.ListOptions{Page: int64(input.Page), PerPage: int64(input.PerPage)},
	}
	hooks, resp, err := client.GL().Projects.ListProjectHooks(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListHooksOutput{}, toolutil.WrapErrWithMessage("projectListHooks", err)
	}
	out := make([]HookOutput, 0, len(hooks))
	for _, h := range hooks {
		out = append(out, hookOutputFromGL(h))
	}
	return ListHooksOutput{Hooks: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetHookInput defines parameters for getting a single project webhook.
type GetHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
}

// GetHook retrieves a single project webhook by ID.
func GetHook(ctx context.Context, client *gitlabclient.Client, input GetHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectGetHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return HookOutput{}, errors.New("projectGetHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	h, _, err := client.GL().Projects.GetProjectHook(string(input.ProjectID), input.HookID, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithMessage("projectGetHook", err)
	}
	return hookOutputFromGL(h), nil
}

// AddHookInput defines parameters for adding a webhook to a project.
type AddHookInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	URL                       string               `json:"url" jsonschema:"Webhook URL,required"`
	Name                      string               `json:"name,omitempty" jsonschema:"Webhook name"`
	Description               string               `json:"description,omitempty" jsonschema:"Webhook description"`
	Token                     string               `json:"token,omitempty" jsonschema:"Secret token for validation"`
	PushEvents                *bool                `json:"push_events,omitempty" jsonschema:"Trigger on push events"`
	PushEventsBranchFilter    string               `json:"push_events_branch_filter,omitempty" jsonschema:"Branch filter for push events"`
	IssuesEvents              *bool                `json:"issues_events,omitempty" jsonschema:"Trigger on issue events"`
	ConfidentialIssuesEvents  *bool                `json:"confidential_issues_events,omitempty" jsonschema:"Trigger on confidential issue events"`
	MergeRequestsEvents       *bool                `json:"merge_requests_events,omitempty" jsonschema:"Trigger on merge request events"`
	TagPushEvents             *bool                `json:"tag_push_events,omitempty" jsonschema:"Trigger on tag push events"`
	NoteEvents                *bool                `json:"note_events,omitempty" jsonschema:"Trigger on note/comment events"`
	ConfidentialNoteEvents    *bool                `json:"confidential_note_events,omitempty" jsonschema:"Trigger on confidential note events"`
	JobEvents                 *bool                `json:"job_events,omitempty" jsonschema:"Trigger on CI job events"`
	PipelineEvents            *bool                `json:"pipeline_events,omitempty" jsonschema:"Trigger on pipeline events"`
	WikiPageEvents            *bool                `json:"wiki_page_events,omitempty" jsonschema:"Trigger on wiki page events"`
	DeploymentEvents          *bool                `json:"deployment_events,omitempty" jsonschema:"Trigger on deployment events"`
	ReleasesEvents            *bool                `json:"releases_events,omitempty" jsonschema:"Trigger on release events"`
	EmojiEvents               *bool                `json:"emoji_events,omitempty" jsonschema:"Trigger on emoji events"`
	ResourceAccessTokenEvents *bool                `json:"resource_access_token_events,omitempty" jsonschema:"Trigger on resource access token events"`
	EnableSSLVerification     *bool                `json:"enable_ssl_verification,omitempty" jsonschema:"Enable SSL verification for webhook"`
	CustomWebhookTemplate     string               `json:"custom_webhook_template,omitempty" jsonschema:"Custom webhook payload template"`
	BranchFilterStrategy      string               `json:"branch_filter_strategy,omitempty" jsonschema:"Branch filter strategy (wildcard, regex, all_branches)"`
}

// AddHook adds a webhook to a project.
func AddHook(ctx context.Context, client *gitlabclient.Client, input AddHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectAddHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.URL == "" {
		return HookOutput{}, errors.New("projectAddHook: url is required. Provide the URL that will receive webhook HTTP POST requests")
	}
	opts := &gl.AddProjectHookOptions{
		URL: new(input.URL),
	}
	applyAddHookIdentity(input, opts)
	applyAddHookEvents(input, opts)
	h, _, err := client.GL().Projects.AddProjectHook(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithMessage("projectAddHook", err)
	}
	return hookOutputFromGL(h), nil
}

// applyAddHookIdentity is an internal helper for the projects package.
func applyAddHookIdentity(input AddHookInput, opts *gl.AddProjectHookOptions) {
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Token != "" {
		opts.Token = new(input.Token)
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
	if input.CustomWebhookTemplate != "" {
		opts.CustomWebhookTemplate = new(input.CustomWebhookTemplate)
	}
	if input.BranchFilterStrategy != "" {
		opts.BranchFilterStrategy = new(input.BranchFilterStrategy)
	}
}

// applyAddHookEvents is an internal helper for the projects package.
func applyAddHookEvents(input AddHookInput, opts *gl.AddProjectHookOptions) {
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.IssuesEvents != nil {
		opts.IssuesEvents = input.IssuesEvents
	}
	if input.ConfidentialIssuesEvents != nil {
		opts.ConfidentialIssuesEvents = input.ConfidentialIssuesEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.NoteEvents != nil {
		opts.NoteEvents = input.NoteEvents
	}
	if input.ConfidentialNoteEvents != nil {
		opts.ConfidentialNoteEvents = input.ConfidentialNoteEvents
	}
	if input.JobEvents != nil {
		opts.JobEvents = input.JobEvents
	}
	if input.PipelineEvents != nil {
		opts.PipelineEvents = input.PipelineEvents
	}
	if input.WikiPageEvents != nil {
		opts.WikiPageEvents = input.WikiPageEvents
	}
	if input.DeploymentEvents != nil {
		opts.DeploymentEvents = input.DeploymentEvents
	}
	if input.ReleasesEvents != nil {
		opts.ReleasesEvents = input.ReleasesEvents
	}
	if input.EmojiEvents != nil {
		opts.EmojiEvents = input.EmojiEvents
	}
	if input.ResourceAccessTokenEvents != nil {
		opts.ResourceAccessTokenEvents = input.ResourceAccessTokenEvents
	}
}

// EditHookInput defines parameters for editing a project webhook.
type EditHookInput struct {
	ProjectID                 toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID                    int64                `json:"hook_id" jsonschema:"Webhook ID to edit,required"`
	URL                       string               `json:"url,omitempty" jsonschema:"Updated webhook URL"`
	Name                      string               `json:"name,omitempty" jsonschema:"Updated webhook name"`
	Description               string               `json:"description,omitempty" jsonschema:"Updated webhook description"`
	Token                     string               `json:"token,omitempty" jsonschema:"Updated secret token"`
	PushEvents                *bool                `json:"push_events,omitempty" jsonschema:"Trigger on push events"`
	PushEventsBranchFilter    string               `json:"push_events_branch_filter,omitempty" jsonschema:"Branch filter for push events"`
	IssuesEvents              *bool                `json:"issues_events,omitempty" jsonschema:"Trigger on issue events"`
	ConfidentialIssuesEvents  *bool                `json:"confidential_issues_events,omitempty" jsonschema:"Trigger on confidential issue events"`
	MergeRequestsEvents       *bool                `json:"merge_requests_events,omitempty" jsonschema:"Trigger on merge request events"`
	TagPushEvents             *bool                `json:"tag_push_events,omitempty" jsonschema:"Trigger on tag push events"`
	NoteEvents                *bool                `json:"note_events,omitempty" jsonschema:"Trigger on note/comment events"`
	ConfidentialNoteEvents    *bool                `json:"confidential_note_events,omitempty" jsonschema:"Trigger on confidential note events"`
	JobEvents                 *bool                `json:"job_events,omitempty" jsonschema:"Trigger on CI job events"`
	PipelineEvents            *bool                `json:"pipeline_events,omitempty" jsonschema:"Trigger on pipeline events"`
	WikiPageEvents            *bool                `json:"wiki_page_events,omitempty" jsonschema:"Trigger on wiki page events"`
	DeploymentEvents          *bool                `json:"deployment_events,omitempty" jsonschema:"Trigger on deployment events"`
	ReleasesEvents            *bool                `json:"releases_events,omitempty" jsonschema:"Trigger on release events"`
	EmojiEvents               *bool                `json:"emoji_events,omitempty" jsonschema:"Trigger on emoji events"`
	ResourceAccessTokenEvents *bool                `json:"resource_access_token_events,omitempty" jsonschema:"Trigger on resource access token events"`
	EnableSSLVerification     *bool                `json:"enable_ssl_verification,omitempty" jsonschema:"Enable SSL verification"`
	CustomWebhookTemplate     string               `json:"custom_webhook_template,omitempty" jsonschema:"Custom webhook payload template"`
	BranchFilterStrategy      string               `json:"branch_filter_strategy,omitempty" jsonschema:"Branch filter strategy (wildcard, regex, all_branches)"`
}

// EditHook edits an existing project webhook.
func EditHook(ctx context.Context, client *gitlabclient.Client, input EditHookInput) (HookOutput, error) {
	if err := ctx.Err(); err != nil {
		return HookOutput{}, err
	}
	if input.ProjectID == "" {
		return HookOutput{}, errors.New("projectEditHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return HookOutput{}, errors.New("projectEditHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	opts := &gl.EditProjectHookOptions{}
	applyEditHookIdentity(input, opts)
	applyEditHookEvents(input, opts)
	h, _, err := client.GL().Projects.EditProjectHook(string(input.ProjectID), input.HookID, opts, gl.WithContext(ctx))
	if err != nil {
		return HookOutput{}, toolutil.WrapErrWithMessage("projectEditHook", err)
	}
	return hookOutputFromGL(h), nil
}

// applyEditHookIdentity is an internal helper for the projects package.
func applyEditHookIdentity(input EditHookInput, opts *gl.EditProjectHookOptions) {
	if input.URL != "" {
		opts.URL = new(input.URL)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Token != "" {
		opts.Token = new(input.Token)
	}
	if input.EnableSSLVerification != nil {
		opts.EnableSSLVerification = input.EnableSSLVerification
	}
	if input.PushEventsBranchFilter != "" {
		opts.PushEventsBranchFilter = new(input.PushEventsBranchFilter)
	}
	if input.CustomWebhookTemplate != "" {
		opts.CustomWebhookTemplate = new(input.CustomWebhookTemplate)
	}
	if input.BranchFilterStrategy != "" {
		opts.BranchFilterStrategy = new(input.BranchFilterStrategy)
	}
}

// applyEditHookEvents is an internal helper for the projects package.
func applyEditHookEvents(input EditHookInput, opts *gl.EditProjectHookOptions) {
	if input.PushEvents != nil {
		opts.PushEvents = input.PushEvents
	}
	if input.IssuesEvents != nil {
		opts.IssuesEvents = input.IssuesEvents
	}
	if input.ConfidentialIssuesEvents != nil {
		opts.ConfidentialIssuesEvents = input.ConfidentialIssuesEvents
	}
	if input.MergeRequestsEvents != nil {
		opts.MergeRequestsEvents = input.MergeRequestsEvents
	}
	if input.TagPushEvents != nil {
		opts.TagPushEvents = input.TagPushEvents
	}
	if input.NoteEvents != nil {
		opts.NoteEvents = input.NoteEvents
	}
	if input.ConfidentialNoteEvents != nil {
		opts.ConfidentialNoteEvents = input.ConfidentialNoteEvents
	}
	if input.JobEvents != nil {
		opts.JobEvents = input.JobEvents
	}
	if input.PipelineEvents != nil {
		opts.PipelineEvents = input.PipelineEvents
	}
	if input.WikiPageEvents != nil {
		opts.WikiPageEvents = input.WikiPageEvents
	}
	if input.DeploymentEvents != nil {
		opts.DeploymentEvents = input.DeploymentEvents
	}
	if input.ReleasesEvents != nil {
		opts.ReleasesEvents = input.ReleasesEvents
	}
	if input.EmojiEvents != nil {
		opts.EmojiEvents = input.EmojiEvents
	}
	if input.ResourceAccessTokenEvents != nil {
		opts.ResourceAccessTokenEvents = input.ResourceAccessTokenEvents
	}
}

// DeleteHookInput defines parameters for deleting a project webhook.
type DeleteHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID to delete,required"`
}

// DeleteHook deletes a project webhook.
func DeleteHook(ctx context.Context, client *gitlabclient.Client, input DeleteHookInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	_, err := client.GL().Projects.DeleteProjectHook(string(input.ProjectID), input.HookID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteHook", err)
	}
	return nil
}

// TriggerTestHookInput defines parameters for triggering a test webhook event.
type TriggerTestHookInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID to test,required"`
	Event     string               `json:"event" jsonschema:"Event type to trigger (push_events, issues_events, merge_requests_events, tag_push_events, note_events, job_events, pipeline_events, wiki_page_events, releases_events, emoji_events),required"`
}

// TriggerTestHookOutput holds the result of triggering a test webhook.
type TriggerTestHookOutput struct {
	toolutil.HintableOutput
	Message string `json:"message"`
}

// TriggerTestHook triggers a test event for a project webhook.
func TriggerTestHook(ctx context.Context, client *gitlabclient.Client, input TriggerTestHookInput) (TriggerTestHookOutput, error) {
	if err := ctx.Err(); err != nil {
		return TriggerTestHookOutput{}, err
	}
	if input.ProjectID == "" {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.HookID == 0 {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: hook_id is required. Use gitlab_project_hook_list to find webhook IDs for the project")
	}
	if input.Event == "" {
		return TriggerTestHookOutput{}, errors.New("projectTriggerTestHook: event is required. Valid events: push_events, tag_push_events, merge_requests_events, note_events, issues_events, job_events, pipeline_events, wiki_page_events, releases_events, emoji_events")
	}
	_, err := client.GL().Projects.TriggerTestProjectHook(string(input.ProjectID), input.HookID, gl.ProjectHookEvent(input.Event), gl.WithContext(ctx))
	if err != nil {
		return TriggerTestHookOutput{}, toolutil.WrapErrWithMessage("projectTriggerTestHook", err)
	}
	return TriggerTestHookOutput{Message: fmt.Sprintf("Test event '%s' triggered for hook %d", input.Event, input.HookID)}, nil
}

// boolIcon is an internal helper for the projects package.
//
// Deprecated: Use toolutil.BoolEmoji instead. Retained as alias.
func boolIcon(v bool) string {
	return toolutil.BoolEmoji(v)
}

// ---------------------------------------------------------------------------
// ListUserProjects — list projects owned by a user
// ---------------------------------------------------------------------------.

// ListUserProjectsInput defines parameters for listing projects owned by a specific user.
type ListUserProjectsInput struct {
	UserID     toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	Search     string               `json:"search,omitempty"     jsonschema:"Search query for project name"`
	Visibility string               `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	Archived   *bool                `json:"archived,omitempty"   jsonschema:"Filter by archived status"`
	OrderBy    string               `json:"order_by,omitempty"   jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort       string               `json:"sort,omitempty"       jsonschema:"Sort direction (asc, desc)"`
	Simple     bool                 `json:"simple,omitempty"     jsonschema:"Return only limited fields (faster)"`
	toolutil.PaginationInput
}

// ListUserProjects lists projects owned by the given user.
func ListUserProjects(ctx context.Context, client *gitlabclient.Client, input ListUserProjectsInput) (ListOutput, error) {
	if input.UserID == "" {
		return ListOutput{}, errors.New("projectListUserProjects: user_id is required. Use gitlab_get_user to find the user ID")
	}
	opts := buildUserProjectOpts(userProjectFilter{
		Search: input.Search, Visibility: input.Visibility, Archived: input.Archived,
		OrderBy: input.OrderBy, Sort: input.Sort, Simple: input.Simple,
		Page: input.Page, PerPage: input.PerPage,
	})
	projects, resp, err := client.GL().Projects.ListUserProjects(string(input.UserID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectListUserProjects", err)
	}
	out := make([]Output, len(projects))
	for i, p := range projects {
		out[i] = ToOutput(p)
	}
	return ListOutput{
		Projects:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListProjectUsers — list users of a project
// ---------------------------------------------------------------------------.

// ProjectUserOutput represents a project user.
type ProjectUserOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// projectUserOutputFromGL is an internal helper for the projects package.
func projectUserOutputFromGL(u *gl.ProjectUser) ProjectUserOutput {
	return ProjectUserOutput{
		ID:        u.ID,
		Name:      u.Name,
		Username:  u.Username,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
}

// ListProjectUsersInput defines parameters for listing users of a project.
type ListProjectUsersInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty" jsonschema:"Search by name or username"`
	toolutil.PaginationInput
}

// ListProjectUsersOutput holds a paginated list of project users.
type ListProjectUsersOutput struct {
	toolutil.HintableOutput
	Users      []ProjectUserOutput       `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectUsers lists users who are members of the given project.
func ListProjectUsers(ctx context.Context, client *gitlabclient.Client, input ListProjectUsersInput) (ListProjectUsersOutput, error) {
	if input.ProjectID == "" {
		return ListProjectUsersOutput{}, errors.New("projectListUsers: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectUserOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	users, resp, err := client.GL().Projects.ListProjectsUsers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectUsersOutput{}, toolutil.WrapErrWithMessage("projectListUsers", err)
	}
	out := make([]ProjectUserOutput, len(users))
	for i, u := range users {
		out[i] = projectUserOutputFromGL(u)
	}
	return ListProjectUsersOutput{
		Users:      out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListProjectGroups — list ancestor groups of a project
// ---------------------------------------------------------------------------.

// ProjectGroupOutput represents a group associated with a project.
type ProjectGroupOutput struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	FullName  string `json:"full_name"`
	FullPath  string `json:"full_path"`
}

// projectGroupOutputFromGL is an internal helper for the projects package.
func projectGroupOutputFromGL(g *gl.ProjectGroup) ProjectGroupOutput {
	return ProjectGroupOutput{
		ID:        g.ID,
		Name:      g.Name,
		AvatarURL: g.AvatarURL,
		WebURL:    g.WebURL,
		FullName:  g.FullName,
		FullPath:  g.FullPath,
	}
}

// ListProjectGroupsInput defines parameters for listing a project's ancestor groups.
type ListProjectGroupsInput struct {
	ProjectID            toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search               string               `json:"search,omitempty"              jsonschema:"Search by group name"`
	WithShared           *bool                `json:"with_shared,omitempty"         jsonschema:"Include shared groups (default true)"`
	SharedVisibleOnly    *bool                `json:"shared_visible_only,omitempty" jsonschema:"Only show shared groups visible to the current user"`
	SkipGroups           []int64              `json:"skip_groups,omitempty"         jsonschema:"Array of group IDs to exclude"`
	SharedMinAccessLevel int                  `json:"shared_min_access_level,omitempty" jsonschema:"Filter by minimum access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner)"`
	toolutil.PaginationInput
}

// ListProjectGroupsOutput holds a paginated list of project groups.
type ListProjectGroupsOutput struct {
	toolutil.HintableOutput
	Groups     []ProjectGroupOutput      `json:"groups"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectGroups lists the ancestor groups of the given project.
func ListProjectGroups(ctx context.Context, client *gitlabclient.Client, input ListProjectGroupsInput) (ListProjectGroupsOutput, error) {
	if input.ProjectID == "" {
		return ListProjectGroupsOutput{}, errors.New("projectListGroups: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectGroupOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.WithShared != nil {
		opts.WithShared = input.WithShared
	}
	if input.SharedVisibleOnly != nil {
		opts.SharedVisibleOnly = input.SharedVisibleOnly
	}
	if len(input.SkipGroups) > 0 {
		opts.SkipGroups = new(input.SkipGroups)
	}
	if input.SharedMinAccessLevel > 0 {
		opts.SharedMinAccessLevel = new(gl.AccessLevelValue(input.SharedMinAccessLevel))
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	groups, resp, err := client.GL().Projects.ListProjectsGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectGroupsOutput{}, toolutil.WrapErrWithMessage("projectListGroups", err)
	}
	out := make([]ProjectGroupOutput, len(groups))
	for i, g := range groups {
		out[i] = projectGroupOutputFromGL(g)
	}
	return ListProjectGroupsOutput{
		Groups:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListProjectStarrers — list users who starred a project
// ---------------------------------------------------------------------------.

// StarrerOutput represents a user who starred a project.
type StarrerOutput struct {
	StarredSince string            `json:"starred_since"`
	User         ProjectUserOutput `json:"user"`
}

// ListProjectStarrersInput defines parameters for listing project starrers.
type ListProjectStarrersInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty" jsonschema:"Search by name or username"`
	toolutil.PaginationInput
}

// ListProjectStarrersOutput holds a paginated list of project starrers.
type ListProjectStarrersOutput struct {
	toolutil.HintableOutput
	Starrers   []StarrerOutput           `json:"starrers"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListProjectStarrers lists users who have starred the given project.
func ListProjectStarrers(ctx context.Context, client *gitlabclient.Client, input ListProjectStarrersInput) (ListProjectStarrersOutput, error) {
	if input.ProjectID == "" {
		return ListProjectStarrersOutput{}, errors.New("projectListStarrers: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectStarrersOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	starrers, resp, err := client.GL().Projects.ListProjectStarrers(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectStarrersOutput{}, toolutil.WrapErrWithMessage("projectListStarrers", err)
	}
	out := make([]StarrerOutput, len(starrers))
	for i, s := range starrers {
		out[i] = StarrerOutput{
			StarredSince: s.StarredSince.Format(time.RFC3339),
			User:         projectUserOutputFromGL(&s.User),
		}
	}
	return ListProjectStarrersOutput{
		Starrers:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ShareProjectWithGroup
// ---------------------------------------------------------------------------.

// ShareProjectInput defines parameters for sharing a project with a group.
type ShareProjectInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	GroupID     int64                `json:"group_id"     jsonschema:"Group ID to share with,required"`
	GroupAccess int                  `json:"group_access" jsonschema:"Access level for the group (10=Guest 20=Reporter 30=Developer 40=Maintainer),required"`
	ExpiresAt   string               `json:"expires_at,omitempty" jsonschema:"Expiration date for the share (YYYY-MM-DD)"`
}

// ShareProjectOutput holds the result of sharing a project.
type ShareProjectOutput struct {
	toolutil.HintableOutput
	Message     string `json:"message"`
	GroupID     int64  `json:"group_id,omitempty"`
	GroupAccess int    `json:"group_access,omitempty"`
	AccessRole  string `json:"access_role,omitempty"`
}

// accessLevelName returns the human-readable name for a GitLab access level.
func accessLevelName(level int) string {
	names := map[int]string{
		10: "Guest",
		20: "Reporter",
		30: "Developer",
		40: "Maintainer",
		50: "Owner",
	}
	if name, ok := names[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}

// ShareProjectWithGroup shares a project with the given group.
func ShareProjectWithGroup(ctx context.Context, client *gitlabclient.Client, input ShareProjectInput) (ShareProjectOutput, error) {
	if input.ProjectID == "" {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.GroupID == 0 {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: group_id is required. Use gitlab_group_list to find the group ID")
	}
	if input.GroupAccess == 0 {
		return ShareProjectOutput{}, errors.New("projectShareWithGroup: group_access is required. Valid levels: 10 (Guest), 20 (Reporter), 30 (Developer), 40 (Maintainer)")
	}
	opts := &gl.ShareWithGroupOptions{
		GroupID:     new(input.GroupID),
		GroupAccess: new(gl.AccessLevelValue(input.GroupAccess)),
	}
	if input.ExpiresAt != "" {
		opts.ExpiresAt = new(input.ExpiresAt)
	}
	_, err := client.GL().Projects.ShareProjectWithGroup(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ShareProjectOutput{}, toolutil.WrapErrWithMessage("projectShareWithGroup", err)
	}
	roleName := accessLevelName(input.GroupAccess)
	return ShareProjectOutput{
		Message:     fmt.Sprintf("Project %s shared with group %d as %s", input.ProjectID, input.GroupID, roleName),
		GroupID:     input.GroupID,
		GroupAccess: input.GroupAccess,
		AccessRole:  roleName,
	}, nil
}

// ---------------------------------------------------------------------------
// DeleteSharedProjectFromGroup
// ---------------------------------------------------------------------------.

// DeleteSharedGroupInput defines parameters for removing a group share from a project.
type DeleteSharedGroupInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	GroupID   int64                `json:"group_id"   jsonschema:"Group ID to remove from project sharing,required"`
}

// DeleteSharedProjectFromGroup removes a shared group link from a project.
func DeleteSharedProjectFromGroup(ctx context.Context, client *gitlabclient.Client, input DeleteSharedGroupInput) error {
	if input.ProjectID == "" {
		return errors.New("projectDeleteSharedGroup: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	if input.GroupID == 0 {
		return errors.New("projectDeleteSharedGroup: group_id is required. Use gitlab_project_list_groups to find shared group IDs")
	}
	_, err := client.GL().Projects.DeleteSharedProjectFromGroup(string(input.ProjectID), input.GroupID, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteSharedGroup", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// ListInvitedGroups — list groups invited to a project
// ---------------------------------------------------------------------------.

// ListInvitedGroupsInput defines parameters for listing groups invited to a project.
type ListInvitedGroupsInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search         string               `json:"search,omitempty"          jsonschema:"Search by group name"`
	MinAccessLevel int                  `json:"min_access_level,omitempty" jsonschema:"Filter by minimum access level (10=Guest 20=Reporter 30=Developer 40=Maintainer 50=Owner)"`
	toolutil.PaginationInput
}

// ListInvitedGroups lists groups that have been invited to the given project.
func ListInvitedGroups(ctx context.Context, client *gitlabclient.Client, input ListInvitedGroupsInput) (ListProjectGroupsOutput, error) {
	if input.ProjectID == "" {
		return ListProjectGroupsOutput{}, errors.New("projectListInvitedGroups: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.ListProjectInvitedGroupOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.MinAccessLevel > 0 {
		opts.MinAccessLevel = new(gl.AccessLevelValue(input.MinAccessLevel))
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	groups, resp, err := client.GL().Projects.ListProjectsInvitedGroups(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectGroupsOutput{}, toolutil.WrapErrWithMessage("projectListInvitedGroups", err)
	}
	out := make([]ProjectGroupOutput, len(groups))
	for i, g := range groups {
		out[i] = projectGroupOutputFromGL(g)
	}
	return ListProjectGroupsOutput{
		Groups:     out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListUserContributedProjects — list projects a user has contributed to
// ---------------------------------------------------------------------------.

// ListUserContributedProjectsInput defines parameters for listing contributed projects.
type ListUserContributedProjectsInput struct {
	UserID     toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	Search     string               `json:"search,omitempty"     jsonschema:"Search query for project name"`
	Visibility string               `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	Archived   *bool                `json:"archived,omitempty"   jsonschema:"Filter by archived status"`
	OrderBy    string               `json:"order_by,omitempty"   jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort       string               `json:"sort,omitempty"       jsonschema:"Sort direction (asc, desc)"`
	Simple     bool                 `json:"simple,omitempty"     jsonschema:"Return only limited fields (faster)"`
	toolutil.PaginationInput
}

// ListUserContributedProjects lists projects a specific user has contributed to.
func ListUserContributedProjects(ctx context.Context, client *gitlabclient.Client, input ListUserContributedProjectsInput) (ListOutput, error) {
	if input.UserID == "" {
		return ListOutput{}, errors.New("projectListUserContributed: user_id is required. Use gitlab_get_user to find the user ID")
	}
	opts := buildUserProjectOpts(userProjectFilter{
		Search: input.Search, Visibility: input.Visibility, Archived: input.Archived,
		OrderBy: input.OrderBy, Sort: input.Sort, Simple: input.Simple,
		Page: input.Page, PerPage: input.PerPage,
	})
	projects, resp, err := client.GL().Projects.ListUserContributedProjects(string(input.UserID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectListUserContributed", err)
	}
	out := make([]Output, len(projects))
	for i, p := range projects {
		out[i] = ToOutput(p)
	}
	return ListOutput{
		Projects:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// ---------------------------------------------------------------------------
// ListUserStarredProjects — list projects a user has starred
// ---------------------------------------------------------------------------.

// ListUserStarredProjectsInput defines parameters for listing starred projects.
type ListUserStarredProjectsInput struct {
	UserID     toolutil.StringOrInt `json:"user_id" jsonschema:"User ID or username,required"`
	Search     string               `json:"search,omitempty"     jsonschema:"Search query for project name"`
	Visibility string               `json:"visibility,omitempty" jsonschema:"Filter by visibility (private, internal, public)"`
	Archived   *bool                `json:"archived,omitempty"   jsonschema:"Filter by archived status"`
	OrderBy    string               `json:"order_by,omitempty"   jsonschema:"Order by field (id, name, path, created_at, updated_at, last_activity_at)"`
	Sort       string               `json:"sort,omitempty"       jsonschema:"Sort direction (asc, desc)"`
	Simple     bool                 `json:"simple,omitempty"     jsonschema:"Return only limited fields (faster)"`
	toolutil.PaginationInput
}

// ListUserStarredProjects lists projects starred by a specific user.
func ListUserStarredProjects(ctx context.Context, client *gitlabclient.Client, input ListUserStarredProjectsInput) (ListOutput, error) {
	if input.UserID == "" {
		return ListOutput{}, errors.New("projectListUserStarred: user_id is required. Use gitlab_get_user to find the user ID")
	}
	opts := buildUserProjectOpts(userProjectFilter{
		Search: input.Search, Visibility: input.Visibility, Archived: input.Archived,
		OrderBy: input.OrderBy, Sort: input.Sort, Simple: input.Simple,
		Page: input.Page, PerPage: input.PerPage,
	})
	projects, resp, err := client.GL().Projects.ListUserStarredProjects(string(input.UserID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("projectListUserStarred", err)
	}
	out := make([]Output, len(projects))
	for i, p := range projects {
		out[i] = ToOutput(p)
	}
	return ListOutput{
		Projects:   out,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// userProjectFilter holds the filter parameters for user-scoped project listings.
type userProjectFilter struct {
	Search     string
	Visibility string
	Archived   *bool
	OrderBy    string
	Sort       string
	Simple     bool
	Page       int
	PerPage    int
}

// buildUserProjectOpts centralizes option building for user-scoped project listings.
func buildUserProjectOpts(f userProjectFilter) *gl.ListProjectsOptions {
	opts := &gl.ListProjectsOptions{}
	if f.Search != "" {
		opts.Search = new(f.Search)
	}
	if f.Visibility != "" {
		opts.Visibility = new(gl.VisibilityValue(f.Visibility))
	}
	if f.Archived != nil {
		opts.Archived = f.Archived
	}
	if f.OrderBy != "" {
		opts.OrderBy = new(f.OrderBy)
	}
	if f.Sort != "" {
		opts.Sort = new(f.Sort)
	}
	if f.Simple {
		opts.Simple = new(true)
	}
	if f.Page > 0 {
		opts.Page = int64(f.Page)
	}
	if f.PerPage > 0 {
		opts.PerPage = int64(f.PerPage)
	}
	return opts
}

// ---------------------------------------------------------------------------
// Push Rules — get, add, edit, delete project push rule configuration
// ---------------------------------------------------------------------------.

// PushRuleOutput represents a project's push rule configuration.
type PushRuleOutput struct {
	toolutil.HintableOutput
	ID                         int64  `json:"id"`
	ProjectID                  int64  `json:"project_id"`
	CommitMessageRegex         string `json:"commit_message_regex,omitempty"`
	CommitMessageNegativeRegex string `json:"commit_message_negative_regex,omitempty"`
	BranchNameRegex            string `json:"branch_name_regex,omitempty"`
	DenyDeleteTag              bool   `json:"deny_delete_tag"`
	MemberCheck                bool   `json:"member_check"`
	PreventSecrets             bool   `json:"prevent_secrets"`
	AuthorEmailRegex           string `json:"author_email_regex,omitempty"`
	FileNameRegex              string `json:"file_name_regex,omitempty"`
	MaxFileSize                int64  `json:"max_file_size"`
	CommitCommitterCheck       bool   `json:"commit_committer_check"`
	CommitCommitterNameCheck   bool   `json:"commit_committer_name_check"`
	RejectUnsignedCommits      bool   `json:"reject_unsigned_commits"`
	RejectNonDCOCommits        bool   `json:"reject_non_dco_commits"`
	CreatedAt                  string `json:"created_at,omitempty"`
}

// pushRuleOutputFromGL is an internal helper for the projects package.
func pushRuleOutputFromGL(r *gl.ProjectPushRules) PushRuleOutput {
	out := PushRuleOutput{
		ID:                         r.ID,
		ProjectID:                  r.ProjectID,
		CommitMessageRegex:         r.CommitMessageRegex,
		CommitMessageNegativeRegex: r.CommitMessageNegativeRegex,
		BranchNameRegex:            r.BranchNameRegex,
		DenyDeleteTag:              r.DenyDeleteTag,
		MemberCheck:                r.MemberCheck,
		PreventSecrets:             r.PreventSecrets,
		AuthorEmailRegex:           r.AuthorEmailRegex,
		FileNameRegex:              r.FileNameRegex,
		MaxFileSize:                r.MaxFileSize,
		CommitCommitterCheck:       r.CommitCommitterCheck,
		CommitCommitterNameCheck:   r.CommitCommitterNameCheck,
		RejectUnsignedCommits:      r.RejectUnsignedCommits,
		RejectNonDCOCommits:        r.RejectNonDCOCommits,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// GetPushRulesInput defines parameters for getting project push rules.
type GetPushRulesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// GetPushRules retrieves the push rule configuration for a project.
func GetPushRules(ctx context.Context, client *gitlabclient.Client, input GetPushRulesInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectGetPushRules: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	rule, _, err := client.GL().Projects.GetProjectPushRules(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return PushRuleOutput{}, toolutil.WrapErrWithMessage("projectGetPushRules", err)
	}
	return pushRuleOutputFromGL(rule), nil
}

// AddPushRuleInput defines parameters for adding push rules to a project.
type AddPushRuleInput struct {
	ProjectID                  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AuthorEmailRegex           string               `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            string               `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a project member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex string               `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         string               `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              string               `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from project members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

// AddPushRule adds push rule configuration to a project.
func AddPushRule(ctx context.Context, client *gitlabclient.Client, input AddPushRuleInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectAddPushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.AddProjectPushRuleOptions{}
	if input.AuthorEmailRegex != "" {
		opts.AuthorEmailRegex = new(input.AuthorEmailRegex)
	}
	if input.BranchNameRegex != "" {
		opts.BranchNameRegex = new(input.BranchNameRegex)
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != "" {
		opts.CommitMessageNegativeRegex = new(input.CommitMessageNegativeRegex)
	}
	if input.CommitMessageRegex != "" {
		opts.CommitMessageRegex = new(input.CommitMessageRegex)
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != "" {
		opts.FileNameRegex = new(input.FileNameRegex)
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
	rule, _, err := client.GL().Projects.AddProjectPushRule(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return PushRuleOutput{}, toolutil.WrapErrWithMessage("projectAddPushRule", err)
	}
	return pushRuleOutputFromGL(rule), nil
}

// EditPushRuleInput defines parameters for editing push rules on a project.
type EditPushRuleInput struct {
	ProjectID                  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	AuthorEmailRegex           *string              `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            *string              `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a project member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex *string              `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         *string              `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              *string              `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from project members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

// EditPushRule modifies the push rule configuration for a project.
func EditPushRule(ctx context.Context, client *gitlabclient.Client, input EditPushRuleInput) (PushRuleOutput, error) {
	if input.ProjectID == "" {
		return PushRuleOutput{}, errors.New("projectEditPushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	opts := &gl.EditProjectPushRuleOptions{}
	if input.AuthorEmailRegex != nil {
		opts.AuthorEmailRegex = input.AuthorEmailRegex
	}
	if input.BranchNameRegex != nil {
		opts.BranchNameRegex = input.BranchNameRegex
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != nil {
		opts.CommitMessageNegativeRegex = input.CommitMessageNegativeRegex
	}
	if input.CommitMessageRegex != nil {
		opts.CommitMessageRegex = input.CommitMessageRegex
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != nil {
		opts.FileNameRegex = input.FileNameRegex
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
	rule, _, err := client.GL().Projects.EditProjectPushRule(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return PushRuleOutput{}, toolutil.WrapErrWithMessage("projectEditPushRule", err)
	}
	return pushRuleOutputFromGL(rule), nil
}

// DeletePushRuleInput defines parameters for deleting push rules from a project.
type DeletePushRuleInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeletePushRule deletes the push rule configuration from a project.
func DeletePushRule(ctx context.Context, client *gitlabclient.Client, input DeletePushRuleInput) error {
	if input.ProjectID == "" {
		return errors.New("projectDeletePushRule: project_id is required. Use gitlab_project_list to find the ID, then pass it as project_id")
	}
	_, err := client.GL().Projects.DeleteProjectPushRule(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeletePushRule", err)
	}
	return nil
}

// SetCustomHeaderInput defines parameters for setting a custom header on a webhook.
type SetCustomHeaderInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"Custom header key name,required"`
	Value     string               `json:"value" jsonschema:"Custom header value,required"`
}

// SetCustomHeader sets a custom header on a project webhook.
func SetCustomHeader(ctx context.Context, client *gitlabclient.Client, input SetCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectSetCustomHeader: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectSetCustomHeader: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectSetCustomHeader: key is required")
	}
	opts := &gl.SetHookCustomHeaderOptions{
		Value: &input.Value,
	}
	_, err := client.GL().Projects.SetProjectCustomHeader(string(input.ProjectID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectSetCustomHeader", err)
	}
	return nil
}

// DeleteCustomHeaderInput defines parameters for deleting a custom header from a webhook.
type DeleteCustomHeaderInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"Custom header key name to delete,required"`
}

// DeleteCustomHeader deletes a custom header from a project webhook.
func DeleteCustomHeader(ctx context.Context, client *gitlabclient.Client, input DeleteCustomHeaderInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteCustomHeader: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteCustomHeader: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectDeleteCustomHeader: key is required")
	}
	_, err := client.GL().Projects.DeleteProjectCustomHeader(string(input.ProjectID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteCustomHeader", err)
	}
	return nil
}

// SetWebhookURLVariableInput defines parameters for setting a URL variable on a webhook.
type SetWebhookURLVariableInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"URL variable key name,required"`
	Value     string               `json:"value" jsonschema:"URL variable value,required"`
}

// SetWebhookURLVariable sets a URL variable on a project webhook.
func SetWebhookURLVariable(ctx context.Context, client *gitlabclient.Client, input SetWebhookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectSetWebhookURLVariable: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectSetWebhookURLVariable: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectSetWebhookURLVariable: key is required")
	}
	opts := &gl.SetProjectWebhookURLVariableOptions{
		Value: &input.Value,
	}
	_, err := client.GL().Projects.SetProjectWebhookURLVariable(string(input.ProjectID), input.HookID, input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectSetWebhookURLVariable", err)
	}
	return nil
}

// DeleteWebhookURLVariableInput defines parameters for deleting a URL variable from a webhook.
type DeleteWebhookURLVariableInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	HookID    int64                `json:"hook_id" jsonschema:"Webhook ID,required"`
	Key       string               `json:"key" jsonschema:"URL variable key name to delete,required"`
}

// DeleteWebhookURLVariable deletes a URL variable from a project webhook.
func DeleteWebhookURLVariable(ctx context.Context, client *gitlabclient.Client, input DeleteWebhookURLVariableInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteWebhookURLVariable: project_id is required")
	}
	if input.HookID == 0 {
		return errors.New("projectDeleteWebhookURLVariable: hook_id is required")
	}
	if input.Key == "" {
		return errors.New("projectDeleteWebhookURLVariable: key is required")
	}
	_, err := client.GL().Projects.DeleteProjectWebhookURLVariable(string(input.ProjectID), input.HookID, input.Key, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteWebhookURLVariable", err)
	}
	return nil
}

// CreateForkRelationInput defines parameters for creating a fork relation.
type CreateForkRelationInput struct {
	ProjectID    toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path of the forked project,required"`
	ForkedFromID int64                `json:"forked_from_id" jsonschema:"ID of the project to set as the fork source,required"`
}

// ForkRelationOutput holds the result of a fork relation operation.
type ForkRelationOutput struct {
	toolutil.HintableOutput
	ID                  int64  `json:"id"`
	ForkedToProjectID   int64  `json:"forked_to_project_id"`
	ForkedFromProjectID int64  `json:"forked_from_project_id"`
	CreatedAt           string `json:"created_at,omitempty"`
	UpdatedAt           string `json:"updated_at,omitempty"`
}

func forkRelationToOutput(r *gl.ProjectForkRelation) ForkRelationOutput {
	out := ForkRelationOutput{
		ID:                  r.ID,
		ForkedToProjectID:   r.ForkedToProjectID,
		ForkedFromProjectID: r.ForkedFromProjectID,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	if r.UpdatedAt != nil {
		out.UpdatedAt = r.UpdatedAt.Format(time.RFC3339)
	}
	return out
}

// CreateForkRelation creates a fork relation between two projects.
func CreateForkRelation(ctx context.Context, client *gitlabclient.Client, input CreateForkRelationInput) (ForkRelationOutput, error) {
	if err := ctx.Err(); err != nil {
		return ForkRelationOutput{}, err
	}
	if input.ProjectID == "" {
		return ForkRelationOutput{}, errors.New("projectCreateForkRelation: project_id is required")
	}
	if input.ForkedFromID == 0 {
		return ForkRelationOutput{}, errors.New("projectCreateForkRelation: forked_from_id is required")
	}
	rel, _, err := client.GL().Projects.CreateProjectForkRelation(string(input.ProjectID), input.ForkedFromID, gl.WithContext(ctx))
	if err != nil {
		return ForkRelationOutput{}, toolutil.WrapErrWithMessage("projectCreateForkRelation", err)
	}
	return forkRelationToOutput(rel), nil
}

// DeleteForkRelationInput defines parameters for deleting a fork relation.
type DeleteForkRelationInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DeleteForkRelation removes the fork relationship from a project.
func DeleteForkRelation(ctx context.Context, client *gitlabclient.Client, input DeleteForkRelationInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectDeleteForkRelation: project_id is required")
	}
	_, err := client.GL().Projects.DeleteProjectForkRelation(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectDeleteForkRelation", err)
	}
	return nil
}

// UploadAvatarInput defines parameters for uploading a project avatar.
// Exactly one of FilePath or ContentBase64 must be provided.
type UploadAvatarInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Filename      string               `json:"filename" jsonschema:"Avatar filename (e.g. avatar.png),required"`
	FilePath      string               `json:"file_path,omitempty" jsonschema:"Absolute path to a local image file on the MCP server filesystem. Alternative to content_base64 for files too large to base64-encode. Only one of file_path or content_base64 should be provided."`
	ContentBase64 string               `json:"content_base64,omitempty" jsonschema:"Base64-encoded image content. Only one of file_path or content_base64 should be provided."`
}

// UploadAvatar uploads or replaces the avatar for a project.
// Accepts either file_path (local file) or content_base64 (base64-encoded string).
func UploadAvatar(ctx context.Context, client *gitlabclient.Client, input UploadAvatarInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("projectUploadAvatar: project_id is required")
	}
	if input.Filename == "" {
		return Output{}, errors.New("projectUploadAvatar: filename is required")
	}

	hasFilePath := input.FilePath != ""
	hasBase64 := input.ContentBase64 != ""

	if hasFilePath && hasBase64 {
		return Output{}, errors.New("projectUploadAvatar: provide either file_path or content_base64, not both")
	}
	if !hasFilePath && !hasBase64 {
		return Output{}, errors.New("projectUploadAvatar: either file_path or content_base64 is required")
	}

	var reader *bytes.Reader

	if hasFilePath {
		cfg := toolutil.GetUploadConfig()
		f, info, err := toolutil.OpenAndValidateFile(input.FilePath, cfg.MaxFileSize)
		if err != nil {
			return Output{}, fmt.Errorf("projectUploadAvatar: %w", err)
		}
		defer f.Close()

		data := make([]byte, info.Size())
		if _, err = io.ReadFull(f, data); err != nil {
			return Output{}, fmt.Errorf("projectUploadAvatar: reading file: %w", err)
		}
		reader = bytes.NewReader(data)
	} else {
		decoded, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return Output{}, fmt.Errorf("projectUploadAvatar: invalid base64 content: %w", err)
		}
		reader = bytes.NewReader(decoded)
	}

	p, _, err := client.GL().Projects.UploadAvatar(string(input.ProjectID), reader, input.Filename, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectUploadAvatar", err)
	}
	return ToOutput(p), nil
}

// DownloadAvatarInput defines parameters for downloading a project avatar.
type DownloadAvatarInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// DownloadAvatarOutput holds the result of downloading a project avatar.
type DownloadAvatarOutput struct {
	toolutil.HintableOutput
	ContentBase64 string `json:"content_base64"`
	SizeBytes     int    `json:"size_bytes"`
}

// DownloadAvatar downloads the avatar image for a project as base64-encoded data.
func DownloadAvatar(ctx context.Context, client *gitlabclient.Client, input DownloadAvatarInput) (DownloadAvatarOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadAvatarOutput{}, err
	}
	if input.ProjectID == "" {
		return DownloadAvatarOutput{}, errors.New("projectDownloadAvatar: project_id is required")
	}
	reader, _, err := client.GL().Projects.DownloadAvatar(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return DownloadAvatarOutput{}, toolutil.WrapErrWithMessage("projectDownloadAvatar", err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return DownloadAvatarOutput{}, fmt.Errorf("projectDownloadAvatar: reading response: %w", err)
	}
	return DownloadAvatarOutput{
		ContentBase64: base64.StdEncoding.EncodeToString(data),
		SizeBytes:     len(data),
	}, nil
}

// StartHousekeepingInput defines parameters for triggering project housekeeping.
type StartHousekeepingInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// StartHousekeeping triggers housekeeping (git gc/repack) for a project.
func StartHousekeeping(ctx context.Context, client *gitlabclient.Client, input StartHousekeepingInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("projectStartHousekeeping: project_id is required")
	}
	_, err := client.GL().Projects.StartHousekeepingProject(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("projectStartHousekeeping", err)
	}
	return nil
}

// GetRepositoryStorageInput defines parameters for getting repository storage info.
type GetRepositoryStorageInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// RepositoryStorageOutput holds repository storage information.
type RepositoryStorageOutput struct {
	toolutil.HintableOutput
	ProjectID         int64  `json:"project_id"`
	DiskPath          string `json:"disk_path"`
	RepositoryStorage string `json:"repository_storage"`
	CreatedAt         string `json:"created_at,omitempty"`
}

// GetRepositoryStorage retrieves repository storage information for a project.
func GetRepositoryStorage(ctx context.Context, client *gitlabclient.Client, input GetRepositoryStorageInput) (RepositoryStorageOutput, error) {
	if err := ctx.Err(); err != nil {
		return RepositoryStorageOutput{}, err
	}
	if input.ProjectID == "" {
		return RepositoryStorageOutput{}, errors.New("projectGetRepositoryStorage: project_id is required")
	}
	storage, _, err := client.GL().Projects.GetRepositoryStorage(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return RepositoryStorageOutput{}, toolutil.WrapErrWithMessage("projectGetRepositoryStorage", err)
	}
	out := RepositoryStorageOutput{
		ProjectID:         storage.ProjectID,
		DiskPath:          storage.DiskPath,
		RepositoryStorage: storage.RepositoryStorage,
	}
	if storage.CreatedAt != nil {
		out.CreatedAt = storage.CreatedAt.Format(time.RFC3339)
	}
	return out, nil
}

// CreateForUserInput defines parameters for creating a project on behalf of a user.
type CreateForUserInput struct {
	UserID               int64    `json:"user_id" jsonschema:"Target user ID who will own the project,required"`
	Name                 string   `json:"name" jsonschema:"Project name,required"`
	Path                 string   `json:"path,omitempty" jsonschema:"Project path slug (defaults from name)"`
	NamespaceID          int      `json:"namespace_id,omitempty" jsonschema:"Namespace ID (defaults to user personal namespace)"`
	Description          string   `json:"description,omitempty" jsonschema:"Project description"`
	Visibility           string   `json:"visibility,omitempty" jsonschema:"Visibility level (private, internal, public)"`
	InitializeWithReadme bool     `json:"initialize_with_readme,omitempty" jsonschema:"Initialize with a README"`
	DefaultBranch        string   `json:"default_branch,omitempty" jsonschema:"Default branch name"`
	Topics               []string `json:"topics,omitempty" jsonschema:"Topic tags for the project"`
	IssuesEnabled        *bool    `json:"issues_enabled,omitempty" jsonschema:"Enable issues feature"`
	MergeRequestsEnabled *bool    `json:"merge_requests_enabled,omitempty" jsonschema:"Enable merge requests feature"`
	WikiEnabled          *bool    `json:"wiki_enabled,omitempty" jsonschema:"Enable wiki feature"`
	JobsEnabled          *bool    `json:"jobs_enabled,omitempty" jsonschema:"Enable CI/CD jobs"`
}

// CreateForUser creates a new project owned by the specified user (admin operation).
func CreateForUser(ctx context.Context, client *gitlabclient.Client, input CreateForUserInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.UserID == 0 {
		return Output{}, errors.New("projectCreateForUser: user_id is required")
	}
	if input.Name == "" {
		return Output{}, errors.New("projectCreateForUser: name is required")
	}
	opts := &gl.CreateProjectForUserOptions{Name: &input.Name}
	if input.Path != "" {
		opts.Path = &input.Path
	}
	if input.NamespaceID != 0 {
		opts.NamespaceID = new(int64(input.NamespaceID))
	}
	if input.Description != "" {
		d := toolutil.NormalizeText(input.Description)
		opts.Description = &d
	}
	if input.Visibility != "" {
		v := gl.VisibilityValue(input.Visibility)
		opts.Visibility = &v
	}
	if input.InitializeWithReadme {
		opts.InitializeWithReadme = new(true)
	}
	if input.DefaultBranch != "" {
		opts.DefaultBranch = &input.DefaultBranch
	}
	if len(input.Topics) > 0 {
		opts.Topics = &input.Topics
	}
	if input.IssuesEnabled != nil {
		opts.IssuesAccessLevel = boolToAccessLevel(input.IssuesEnabled)
	}
	if input.MergeRequestsEnabled != nil {
		opts.MergeRequestsAccessLevel = boolToAccessLevel(input.MergeRequestsEnabled)
	}
	if input.WikiEnabled != nil {
		opts.WikiAccessLevel = boolToAccessLevel(input.WikiEnabled)
	}
	if input.JobsEnabled != nil {
		opts.BuildsAccessLevel = boolToAccessLevel(input.JobsEnabled)
	}
	p, _, err := client.GL().Projects.CreateProjectForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("projectCreateForUser", err)
	}
	return ToOutput(p), nil
}
