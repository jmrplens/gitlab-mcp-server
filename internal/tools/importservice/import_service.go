package importservice

import (
	"context"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The labels an import card repeats, and the canonical catalog action IDs its
// hints name. The import actions are routes on the admin catalog group, so
// their domain is "admin"; the project the import creates is read through the
// project group.
const (
	labelFullPath     = "Full Path"
	labelImportSource = "Import Source"
	labelImportStatus = "Import Status"
	labelStatusName   = "Status Name"
	labelProviderLink = "Provider Link"

	actionProjectGet     = "project.get"
	actionCancelImport   = "admin.import_cancel_github"
	actionImportGitHub   = "admin.import_github"
	hintPollImportStatus = "poll import_status until the import finishes"
)

// Import from GitHub.

// ImportFromGitHubInput represents input for importing a repository from GitHub.
type ImportFromGitHubInput struct {
	PersonalAccessToken string `json:"personal_access_token" jsonschema:"GitHub personal access token. Treat as secret: do not log or store it, and redact it in telemetry/errors,required"`
	RepoID              int64  `json:"repo_id" jsonschema:"GitHub repository ID,required"`
	NewName             string `json:"new_name,omitempty" jsonschema:"New name for the imported project"`
	TargetNamespace     string `json:"target_namespace" jsonschema:"Target namespace for the imported project,required"`
	GitHubHostname      string `json:"github_hostname,omitempty" jsonschema:"GitHub hostname for GitHub Enterprise"`
	// OptionalStages selectively enables the slower, optional import stages.
	// Each flag defaults to disabled when omitted, matching the GitLab API.
	OptionalStages  *GitHubOptionalStagesInput `json:"optional_stages,omitempty" jsonschema:"Optional import stages to enable (each defaults to disabled when omitted)"`
	TimeoutStrategy string                     `json:"timeout_strategy,omitempty" jsonschema:"Timeout strategy (optimistic or pessimistic)"`
}

// GitHubOptionalStagesInput mirrors the GitLab GitHub-import optional_stages
// object, toggling the slower, opt-in import stages.
type GitHubOptionalStagesInput struct {
	SingleEndpointNotesImport *bool `json:"single_endpoint_notes_import,omitempty" jsonschema:"Import notes (comments) using the single-endpoint strategy for large repositories"`
	AttachmentsImport         *bool `json:"attachments_import,omitempty" jsonschema:"Import Markdown attachments (images, files) referenced in descriptions and comments"`
	CollaboratorsImport       *bool `json:"collaborators_import,omitempty" jsonschema:"Import collaborators (project members) from the GitHub repository"`
}

// GitHubImportOutput represents the output of a GitHub import operation.
type GitHubImportOutput struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	FullPath              string `json:"full_path"`
	FullName              string `json:"full_name"`
	RefsURL               string `json:"refs_url,omitempty"`
	ImportSource          string `json:"import_source"`
	ImportStatus          string `json:"import_status"`
	HumanImportStatusName string `json:"human_import_status_name,omitempty"`
	ProviderLink          string `json:"provider_link,omitempty"`
	RelationType          string `json:"relation_type,omitempty"`
	ImportWarning         string `json:"import_warning,omitempty"`
}

// ImportFromGitHub imports a repository from GitHub into GitLab.
func ImportFromGitHub(ctx context.Context, client *gitlabclient.Client, input ImportFromGitHubInput) (*GitHubImportOutput, error) {
	if input.RepoID <= 0 {
		return nil, toolutil.ErrRequiredInt64("gitlab_import_from_github", "repo_id")
	}
	opts := &gl.ImportRepositoryFromGitHubOptions{
		PersonalAccessToken: new(input.PersonalAccessToken),
		RepoID:              new(input.RepoID),
		TargetNamespace:     new(input.TargetNamespace),
	}
	if input.NewName != "" {
		opts.NewName = new(input.NewName)
	}
	if input.GitHubHostname != "" {
		opts.GitHubHostname = new(input.GitHubHostname)
	}
	if input.TimeoutStrategy != "" {
		opts.TimeoutStrategy = new(input.TimeoutStrategy)
	}
	if input.OptionalStages != nil {
		opts.OptionalStages = gl.ImportRepositoryFromGitHubOptionalStagesOptions{
			SingleEndpointNotesImport: input.OptionalStages.SingleEndpointNotesImport,
			AttachmentsImport:         input.OptionalStages.AttachmentsImport,
			CollaboratorsImport:       input.OptionalStages.CollaboratorsImport,
		}
	}
	result, _, err := client.GL().Import.ImportRepositoryFromGitHub(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_import_from_github", err, http.StatusBadRequest,
			"personal_access_token must be a valid GitHub PAT with repo scope; repo_id is the GitHub numeric repo ID; target_namespace must exist in GitLab. Import is async, poll status with gitlab_project_get")
	}
	return &GitHubImportOutput{
		ID:                    result.ID,
		Name:                  result.Name,
		FullPath:              result.FullPath,
		FullName:              result.FullName,
		RefsURL:               result.RefsURL,
		ImportSource:          result.ImportSource,
		ImportStatus:          result.ImportStatus,
		HumanImportStatusName: result.HumanImportStatusName,
		ProviderLink:          result.ProviderLink,
		RelationType:          result.RelationType,
		ImportWarning:         result.ImportWarning,
	}, nil
}

// Cancel GitHub Import.

// CancelGitHubImportInput represents input for canceling a GitHub import.
type CancelGitHubImportInput struct {
	ProjectID int64 `json:"project_id" jsonschema:"The GitLab project ID of the import to cancel,required"`
}

// CancelledImportOutput represents the output of a canceled GitHub import.
type CancelledImportOutput struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	FullPath              string `json:"full_path"`
	FullName              string `json:"full_name"`
	ImportSource          string `json:"import_source"`
	ImportStatus          string `json:"import_status"`
	HumanImportStatusName string `json:"human_import_status_name,omitempty"`
	ProviderLink          string `json:"provider_link,omitempty"`
}

// CancelGitHubImport cancels an ongoing GitHub import.
func CancelGitHubImport(ctx context.Context, client *gitlabclient.Client, input CancelGitHubImportInput) (*CancelledImportOutput, error) {
	if input.ProjectID <= 0 {
		return nil, toolutil.ErrRequiredInt64("gitlab_cancel_github_import", "project_id")
	}
	opts := &gl.CancelGitHubProjectImportOptions{
		ProjectID: new(input.ProjectID),
	}
	result, _, err := client.GL().Import.CancelGitHubProjectImport(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_cancel_github_import", err, http.StatusBadRequest,
			"verify project_id with gitlab_project_list; cancellation only works while import is in progress (status=started); completed/failed imports cannot be cancelled")
	}
	return &CancelledImportOutput{
		ID:                    result.ID,
		Name:                  result.Name,
		FullPath:              result.FullPath,
		FullName:              result.FullName,
		ImportSource:          result.ImportSource,
		ImportStatus:          result.ImportStatus,
		HumanImportStatusName: result.HumanImportStatusName,
		ProviderLink:          result.ProviderLink,
	}, nil
}

// Import GitHub Gists.

// ImportGistsInput represents input for importing GitHub gists as GitLab snippets.
type ImportGistsInput struct {
	PersonalAccessToken string `json:"personal_access_token" jsonschema:"GitHub personal access token. Treat as secret: do not log or store it, and redact it in telemetry/errors,required"`
}

// ImportGists imports GitHub gists into GitLab snippets.
func ImportGists(ctx context.Context, client *gitlabclient.Client, input ImportGistsInput) error {
	opts := &gl.ImportGitHubGistsIntoGitLabSnippetsOptions{
		PersonalAccessToken: new(input.PersonalAccessToken),
	}
	_, err := client.GL().Import.ImportGitHubGistsIntoGitLabSnippets(opts, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_import_github_gists", err, http.StatusBadRequest,
			"personal_access_token must have gist scope; gists are imported as personal snippets for the authenticated user; import is async")
	}
	return nil
}

// Import from Bitbucket Cloud.

// ImportFromBitbucketCloudInput represents input for importing from Bitbucket Cloud.
type ImportFromBitbucketCloudInput struct {
	BitbucketUsername    string `json:"bitbucket_username" jsonschema:"Bitbucket Cloud username,required"`
	BitbucketAppPassword string `json:"bitbucket_app_password,omitempty" jsonschema:"Bitbucket Cloud app password (legacy auth). Treat as secret: do not log or store it, and redact it in telemetry/errors. Provide this OR bitbucket_api_token + bitbucket_email"`
	BitbucketAPIToken    string `json:"bitbucket_api_token,omitempty" jsonschema:"Bitbucket Cloud API token (replaces app passwords). Treat as secret: do not log or store it, and redact it in telemetry/errors. Requires bitbucket_email"`
	BitbucketEmail       string `json:"bitbucket_email,omitempty" jsonschema:"Atlassian account email associated with the API token (required when using bitbucket_api_token)"`
	RepoPath             string `json:"repo_path" jsonschema:"Bitbucket repository path (e.g. owner/repo),required"`
	TargetNamespace      string `json:"target_namespace" jsonschema:"Target namespace for the imported project,required"`
	NewName              string `json:"new_name,omitempty" jsonschema:"New name for the imported project"`
}

// BitbucketCloudImportOutput represents the output of a Bitbucket Cloud import.
type BitbucketCloudImportOutput struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	FullPath              string `json:"full_path"`
	FullName              string `json:"full_name"`
	ImportSource          string `json:"import_source"`
	ImportStatus          string `json:"import_status"`
	HumanImportStatusName string `json:"human_import_status_name,omitempty"`
	ProviderLink          string `json:"provider_link,omitempty"`
}

// ImportFromBitbucketCloud imports a repository from Bitbucket Cloud into GitLab.
func ImportFromBitbucketCloud(ctx context.Context, client *gitlabclient.Client, input ImportFromBitbucketCloudInput) (*BitbucketCloudImportOutput, error) {
	if input.BitbucketUsername == "" {
		return nil, toolutil.ErrFieldRequired("bitbucket_username")
	}
	if input.RepoPath == "" {
		return nil, toolutil.ErrFieldRequired("repo_path")
	}
	if input.TargetNamespace == "" {
		return nil, toolutil.ErrFieldRequired("target_namespace")
	}
	// API-token auth requires the associated Atlassian account email.
	if input.BitbucketAPIToken != "" && input.BitbucketEmail == "" {
		return nil, toolutil.ErrFieldRequired("bitbucket_email")
	}

	opts := &gl.ImportRepositoryFromBitbucketCloudOptions{
		BitbucketUsername: new(input.BitbucketUsername),
		RepoPath:          new(input.RepoPath),
		TargetNamespace:   new(input.TargetNamespace),
	}
	if input.BitbucketAppPassword != "" {
		opts.BitbucketAppPassword = new(input.BitbucketAppPassword)
	}
	if input.BitbucketAPIToken != "" {
		opts.BitbucketAPIToken = new(input.BitbucketAPIToken)
	}
	if input.BitbucketEmail != "" {
		opts.BitbucketEmail = new(input.BitbucketEmail)
	}
	if input.NewName != "" {
		opts.NewName = new(input.NewName)
	}
	result, _, err := client.GL().Import.ImportRepositoryFromBitbucketCloud(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_import_from_bitbucket_cloud", err, http.StatusBadRequest,
			"authenticate with bitbucket_username + bitbucket_app_password (legacy) OR bitbucket_api_token + bitbucket_email (NOT the account password); repo_path is workspace/repo; target_namespace must exist; import is async")
	}
	return &BitbucketCloudImportOutput{
		ID:                    result.ID,
		Name:                  result.Name,
		FullPath:              result.FullPath,
		FullName:              result.FullName,
		ImportSource:          result.ImportSource,
		ImportStatus:          result.ImportStatus,
		HumanImportStatusName: result.HumanImportStatusName,
		ProviderLink:          result.ProviderLink,
	}, nil
}

// Import from Bitbucket Server.

// ImportFromBitbucketServerInput represents input for importing from Bitbucket Server.
type ImportFromBitbucketServerInput struct {
	BitbucketServerURL      string `json:"bitbucket_server_url" jsonschema:"Bitbucket Server URL,required"`
	BitbucketServerUsername string `json:"bitbucket_server_username" jsonschema:"Bitbucket Server username,required"`
	PersonalAccessToken     string `json:"personal_access_token" jsonschema:"Bitbucket Server personal access token. Treat as secret: do not log or store it, and redact it in telemetry/errors,required"`
	BitbucketServerProject  string `json:"bitbucket_server_project" jsonschema:"Bitbucket Server project key,required"`
	BitbucketServerRepo     string `json:"bitbucket_server_repo" jsonschema:"Bitbucket Server repository slug,required"`
	NewName                 string `json:"new_name,omitempty" jsonschema:"New name for the imported project"`
	NewNamespace            string `json:"new_namespace,omitempty" jsonschema:"Target namespace for the imported project"`
	TimeoutStrategy         string `json:"timeout_strategy,omitempty" jsonschema:"Timeout strategy (optimistic or pessimistic)"`
}

// BitbucketServerImportOutput represents the output of a Bitbucket Server import.
type BitbucketServerImportOutput struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
	FullName string `json:"full_name"`
}

// ImportFromBitbucketServer imports a repository from Bitbucket Server into GitLab.
func ImportFromBitbucketServer(ctx context.Context, client *gitlabclient.Client, input ImportFromBitbucketServerInput) (*BitbucketServerImportOutput, error) {
	opts := &gl.ImportRepositoryFromBitbucketServerOptions{
		BitbucketServerURL:      new(input.BitbucketServerURL),
		BitbucketServerUsername: new(input.BitbucketServerUsername),
		PersonalAccessToken:     new(input.PersonalAccessToken),
		BitbucketServerProject:  new(input.BitbucketServerProject),
		BitbucketServerRepo:     new(input.BitbucketServerRepo),
	}
	if input.NewName != "" {
		opts.NewName = new(input.NewName)
	}
	if input.NewNamespace != "" {
		opts.NewNamespace = new(input.NewNamespace)
	}
	if input.TimeoutStrategy != "" {
		opts.TimeoutStrategy = new(input.TimeoutStrategy)
	}
	result, _, err := client.GL().Import.ImportRepositoryFromBitbucketServer(opts, gl.WithContext(ctx))
	if err != nil {
		return nil, toolutil.WrapErrWithStatusHint("gitlab_import_from_bitbucket_server", err, http.StatusBadRequest,
			"bitbucket_server_url must be the base URL (no trailing path); bitbucket_server_username + personal_access_token; project_key + repo_slug from Bitbucket Server; import is async")
	}
	return &BitbucketServerImportOutput{
		ID:       result.ID,
		Name:     result.Name,
		FullPath: result.FullPath,
		FullName: result.FullName,
	}, nil
}

// Markdown Formatters.

// FormatGitHubImport renders a GitHub import as a card.
//
// The imported project's name is whatever the source repository was called, so
// every value here is one a person typed; the card escapes each for the row it
// writes. It used to drop the relation type and both addresses GitLab sends —
// the source repository and the refs endpoint — which left a reader of a
// mirror import unable to tell what was mirrored or from where.
func FormatGitHubImport(out *GitHubImportOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "GitHub Import: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field(labelFullPath, out.FullPath)
	c.Field(labelImportSource, out.ImportSource)
	c.Field(labelImportStatus, out.ImportStatus)
	c.Field(labelStatusName, out.HumanImportStatusName)
	c.Field("Relation Type", out.RelationType)
	c.Link(labelProviderLink, "", out.ProviderLink)
	c.Link("Refs URL", "", out.RefsURL)
	c.Text("Import Warning", out.ImportWarning)
	c.End(
		toolutil.HintAction(actionProjectGet, hintPollImportStatus),
		toolutil.HintAction(actionCancelImport, "cancel the import while it runs"),
	)
	return b.String()
}

// FormatCancelledImport renders a canceled GitHub import as a card.
func FormatCancelledImport(out *CancelledImportOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Canceled Import: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field(labelFullPath, out.FullPath)
	c.Field(labelImportSource, out.ImportSource)
	c.Field(labelImportStatus, out.ImportStatus)
	c.Field(labelStatusName, out.HumanImportStatusName)
	c.Link(labelProviderLink, "", out.ProviderLink)
	c.End(toolutil.HintAction(actionImportGitHub, "start a new import if one is still wanted"))
	return b.String()
}

// FormatBitbucketCloudImport renders a Bitbucket Cloud import as a card.
func FormatBitbucketCloudImport(out *BitbucketCloudImportOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Bitbucket Cloud Import: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field(labelFullPath, out.FullPath)
	c.Field(labelImportSource, out.ImportSource)
	c.Field(labelImportStatus, out.ImportStatus)
	c.Field(labelStatusName, out.HumanImportStatusName)
	c.Link(labelProviderLink, "", out.ProviderLink)
	c.End(toolutil.HintAction(actionProjectGet, hintPollImportStatus))
	return b.String()
}

// FormatBitbucketServerImport renders a Bitbucket Server import as a card.
// The endpoint answers with the created project alone: no import status, no
// provider link.
func FormatBitbucketServerImport(out *BitbucketServerImportOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Bitbucket Server Import: "+out.Name)
	c.Int("ID", out.ID)
	c.Field("Name", out.Name)
	c.Field(labelFullPath, out.FullPath)
	c.Field("Full Name", out.FullName)
	c.End(toolutil.HintAction(actionProjectGet, hintPollImportStatus))
	return b.String()
}

// init registers value-type Markdown formatters so the registry resolves the
// pointer the import handlers return: MarkdownForResult dereferences a pointer
// whose element type has a formatter, so the registrations below use value
// signatures.
func init() {
	toolutil.RegisterMarkdown(func(out GitHubImportOutput) string { return FormatGitHubImport(&out) })
	toolutil.RegisterMarkdown(func(out CancelledImportOutput) string { return FormatCancelledImport(&out) })
	toolutil.RegisterMarkdown(func(out BitbucketCloudImportOutput) string { return FormatBitbucketCloudImport(&out) })
	toolutil.RegisterMarkdown(func(out BitbucketServerImportOutput) string { return FormatBitbucketServerImport(&out) })
}
