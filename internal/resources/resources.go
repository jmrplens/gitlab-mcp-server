package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Resource output structs.

// ProjectResourceOutput is the JSON payload returned by the
// "gitlab://project/{project_id}" resource. It contains the project's
// identifying fields, namespace path, visibility, web URL,
// description, and default branch — enough for clients to render a
// project card without an extra API call.
type ProjectResourceOutput struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	Description       string `json:"description"`
	DefaultBranch     string `json:"default_branch"`
}

// UserResourceOutput is the JSON payload returned by the
// "gitlab://user/current" resource. It contains the authenticated
// user's profile fields: ID, username, display name, email, account
// state, web URL, and admin status.
type UserResourceOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
	IsAdmin  bool   `json:"is_admin"`
}

// MemberResourceOutput is the JSON payload for a single member entry
// in the "gitlab://{project|group}/{id}/members" resources. It
// includes the member's ID, username, display name, account state,
// numeric access level (10=guest, 20=reporter, 30=developer,
// 40=maintainer, 50=owner), and web URL.
type MemberResourceOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	AccessLevel int    `json:"access_level"`
	WebURL      string `json:"web_url"`
}

func memberResourceOutput(id int64, username, name, state string, accessLevel gl.AccessLevelValue, webURL string) MemberResourceOutput {
	return MemberResourceOutput{
		ID:          id,
		Username:    username,
		Name:        name,
		State:       state,
		AccessLevel: int(accessLevel),
		WebURL:      webURL,
	}
}

func projectMemberResourceOutput(member *gl.ProjectMember) MemberResourceOutput {
	return memberResourceOutput(member.ID, member.Username, member.Name, member.State, member.AccessLevel, member.WebURL)
}

func groupMemberResourceOutput(member *gl.GroupMember) MemberResourceOutput {
	return memberResourceOutput(member.ID, member.Username, member.Name, member.State, member.AccessLevel, member.WebURL)
}

// PipelineResourceOutput is the JSON payload returned by the pipeline
// detail resources ("gitlab://project/{id}/pipelines/latest" and
// "gitlab://project/{id}/pipeline/{pipeline_id}"). It contains the
// pipeline's ID, IID, status, ref, SHA, web URL, and source
// ("push", "web", etc.).
type PipelineResourceOutput struct {
	ID     int64  `json:"id"`
	IID    int64  `json:"iid"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	WebURL string `json:"web_url"`
	Source string `json:"source"`
}

// JobResourceOutput is the JSON payload for a single pipeline job
// returned by the "gitlab://project/{id}/pipeline/{id}/jobs" and
// "gitlab://project/{id}/job/{job_id}" resources. It contains the
// job's ID, name, stage, status, ref, duration in seconds,
// failure reason (omitted when empty), and web URL.
type JobResourceOutput struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Stage         string  `json:"stage"`
	Status        string  `json:"status"`
	Ref           string  `json:"ref"`
	Duration      float64 `json:"duration"`
	FailureReason string  `json:"failure_reason,omitempty"`
	WebURL        string  `json:"web_url"`
}

// LabelResourceOutput is the JSON payload for a single project or
// group label. It includes the label's ID, name, color (hex), optional
// description, and the current open issue and open MR counts (used by
// the label detail and listing resources).
type LabelResourceOutput struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	Description            string `json:"description"`
	OpenIssuesCount        int64  `json:"open_issues_count"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count"`
}

// MilestoneResourceOutput is the JSON payload for a single project or
// group milestone. It includes the milestone's ID, IID, title,
// description, state (active/closed), optional due date, and web URL.
type MilestoneResourceOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	DueDate     string `json:"due_date,omitempty"`
	WebURL      string `json:"web_url"`
}

// MRResourceOutput is the JSON payload returned by the
// "gitlab://project/{id}/mr/{iid}" resource. It contains the MR's ID,
// IID, title, state, source and target branches, author username,
// web URL, and detailed merge status.
type MRResourceOutput struct {
	ID           int64  `json:"id"`
	IID          int64  `json:"iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Author       string `json:"author"`
	WebURL       string `json:"web_url"`
	MergeStatus  string `json:"merge_status"`
}

// BranchResourceOutput is the JSON payload for a single repository
// branch (used by both the branch listing and the branch detail
// resources). It includes the branch's name, protection flag, merge
// status, default flag, and web URL.
type BranchResourceOutput struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Merged    bool   `json:"merged"`
	Default   bool   `json:"default"`
	WebURL    string `json:"web_url"`
}

// GroupResourceOutput is the JSON payload for a GitLab group
// (used by both the groups listing and the group detail resources).
// It contains the group's ID, name, path, full path, description,
// visibility, and web URL.
type GroupResourceOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	FullPath    string `json:"full_path"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	WebURL      string `json:"web_url"`
}

// IssueResourceOutput is the JSON payload for a single project issue
// (used by both the issues listing and the issue detail resources).
// It contains the issue's ID, IID, title, state, labels, assignees
// (usernames), author username, web URL, and creation timestamp.
type IssueResourceOutput struct {
	ID        int64    `json:"id"`
	IID       int64    `json:"iid"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	Labels    []string `json:"labels"`
	Assignees []string `json:"assignees"`
	Author    string   `json:"author"`
	WebURL    string   `json:"web_url"`
	CreatedAt string   `json:"created_at"`
}

// ReleaseResourceOutput is the JSON payload for a single project
// release (used by both the releases listing and the release detail
// resources). It contains the tag name, release name, description,
// author username, creation timestamp, and optional release timestamp.
type ReleaseResourceOutput struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	CreatedAt   string `json:"created_at"`
	ReleasedAt  string `json:"released_at,omitempty"`
}

// TagResourceOutput is the JSON payload for a single repository tag
// (used by both the tags listing and the tag detail resources). It
// contains the tag name, optional annotation message, target commit
// SHA, protection status, and optional creation timestamp.
type TagResourceOutput struct {
	Name      string `json:"name"`
	Message   string `json:"message,omitempty"`
	Target    string `json:"target"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at,omitempty"`
}

// CommitResourceOutput is the JSON payload returned by the
// "gitlab://project/{id}/commit/{sha}" resource. It contains the
// commit's full ID, short ID, title, full message, author name and
// email, authored/committed timestamps, parent commit IDs, web URL,
// and optional [CommitStatsOutput] with addition/deletion totals.
type CommitResourceOutput struct {
	ID            string             `json:"id"`
	ShortID       string             `json:"short_id"`
	Title         string             `json:"title"`
	Message       string             `json:"message"`
	AuthorName    string             `json:"author_name"`
	AuthorEmail   string             `json:"author_email"`
	AuthoredDate  string             `json:"authored_date,omitempty"`
	CommittedDate string             `json:"committed_date,omitempty"`
	WebURL        string             `json:"web_url"`
	ParentIDs     []string           `json:"parent_ids,omitempty"`
	Stats         *CommitStatsOutput `json:"stats,omitempty"`
}

// CommitStatsOutput holds line addition and deletion totals for a
// single commit, returned as a sub-object of [CommitResourceOutput].
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// FileBlobResourceOutput is the JSON payload returned by the
// "gitlab://project/{id}/file/{ref}/{path}" resource. Binary content
// is omitted; only the textual representation is returned (see
// [decodeFileContent]). Files larger than [fileBlobMaxBytes] are
// truncated to their metadata with Truncated=true and
// ContentCategory="truncated".
type FileBlobResourceOutput struct {
	FileName        string `json:"file_name"`
	FilePath        string `json:"file_path"`
	Size            int64  `json:"size"`
	Encoding        string `json:"encoding,omitempty"`
	Ref             string `json:"ref"`
	BlobID          string `json:"blob_id"`
	CommitID        string `json:"commit_id"`
	LastCommitID    string `json:"last_commit_id"`
	Content         string `json:"content,omitempty"`
	ContentCategory string `json:"content_category"`
	Truncated       bool   `json:"truncated,omitempty"`
}

// WikiResourceOutput is the JSON payload returned by the
// "gitlab://project/{id}/wiki/{slug}" resource. It contains the page
// title, slug, format (markdown, rdoc, asciidoc, or org), raw content,
// and encoding (omitted when the API does not return one).
type WikiResourceOutput struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Format   string `json:"format"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// MRNoteResourceOutput is the JSON payload for a single merge-request
// note inside the flat "gitlab://project/{id}/mr/{iid}/notes" list
// resource. It contains the note's ID, author username, body,
// system flag, resolvable and resolved flags (omitted when not
// resolvable), and optional creation and update timestamps.
type MRNoteResourceOutput struct {
	ID         int64  `json:"id"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	System     bool   `json:"system"`
	Resolvable bool   `json:"resolvable,omitempty"`
	Resolved   bool   `json:"resolved,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// MRDiscussionNoteResourceOutput is the JSON payload for a single
// note inside a discussion thread, returned as part of the
// "gitlab://project/{id}/mr/{iid}/discussions" resource. It contains
// the note's ID, author username, body, system flag, resolved and
// resolvable flags, and optional creation timestamp.
type MRDiscussionNoteResourceOutput struct {
	ID         int64  `json:"id"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	System     bool   `json:"system"`
	Resolved   bool   `json:"resolved"`
	Resolvable bool   `json:"resolvable"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// MRDiscussionResourceOutput is the JSON payload for a single
// discussion thread on a merge request. It bundles a thread ID, the
// individual_note flag (true for non-threaded comments), and the
// ordered list of notes that make up the thread.
type MRDiscussionResourceOutput struct {
	ID             string                           `json:"id"`
	IndividualNote bool                             `json:"individual_note"`
	Notes          []MRDiscussionNoteResourceOutput `json:"notes"`
}

// DeploymentResourceOutput is the JSON payload for a single project
// deployment, returned by the
// "gitlab://project/{id}/deployment/{deployment_id}" resource. It
// contains the deployment's ID, IID, ref, SHA, status, and optional
// environment name.
type DeploymentResourceOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Ref         string `json:"ref"`
	SHA         string `json:"sha"`
	Status      string `json:"status"`
	Environment string `json:"environment,omitempty"`
}

// EnvironmentResourceOutput is the JSON payload for a single project
// environment, returned by the
// "gitlab://project/{id}/environment/{environment_id}" resource. It
// contains the environment's ID, name, slug, state, and optional
// tier ("production", "staging", "testing", "development", or other).
type EnvironmentResourceOutput struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	State string `json:"state"`
	Tier  string `json:"tier,omitempty"`
}

// SnippetResourceOutput is the JSON payload for a personal (global) or
// project snippet, returned by the
// "gitlab://snippet/{snippet_id}" and
// "gitlab://project/{id}/snippet/{snippet_id}" resources. It contains
// the snippet's ID, title, file name, description, visibility, and
// web URL.
type SnippetResourceOutput struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	FileName    string `json:"file_name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	WebURL      string `json:"web_url"`
}

// FeatureFlagResourceOutput is the JSON payload for a single project
// feature flag, returned by the
// "gitlab://project/{id}/feature_flag/{name}" resource. It contains
// the flag's name, optional description, active flag, and the
// strategy version ("legacy" or "new").
type FeatureFlagResourceOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Active      bool   `json:"active"`
	Version     string `json:"version"`
}

// DeployKeyResourceOutput is the JSON payload for a single project
// deploy key, returned by the
// "gitlab://project/{id}/deploy_key/{deploy_key_id}" resource. It
// contains the key's ID, title, public key text, and optional SHA256
// fingerprint.
type DeployKeyResourceOutput struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// BoardResourceOutput is the JSON payload for a single project issue
// board, returned by the "gitlab://project/{id}/board/{board_id}"
// resource. It contains the board's ID and name.
type BoardResourceOutput struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Maximum size (in bytes) of file content returned by the file blob resource.
// Files exceeding this limit return their metadata with content omitted and
// truncated=true to keep responses small for LLM context windows.
const fileBlobMaxBytes = 1 << 20 // 1 MiB

// Internal constants for the JSON MIME type and URI scheme prefixes
// used to route MCP resource requests to the correct GitLab API endpoints.
const (
	mimeJSON         = "application/json"
	uriProjectPrefix = "gitlab://project/"
	uriGroupPrefix   = "gitlab://group/"
	uriSnippetPrefix = "gitlab://snippet/"
	timeFormatISO    = "2006-01-02T15:04:05Z"
)

// wrapErr enriches a GitLab API error with HTTP status classification, and
// marks it as a JSON-RPC internal error.
//
// Without the code it reached the client as 0, which is not an error code. The
// cause stays wrapped, so toolutil.IsHTTPStatus still classifies it and
// subscriptions.TranslateReadError still recognizes a 401/403/404; and a
// resource that is genuinely absent keeps its own code, because those handlers
// return mcp.ResourceNotFoundError directly rather than passing through here.
func wrapErr(msg string, err error) error {
	return toolutil.InternalError(fmt.Errorf("%s: %s (%w)", msg, toolutil.ClassifyError(err), err))
}

// Register adds every GitLab-backed MCP resource to the given server.
// The full set of URI shapes exposed by Register is:
//
//	gitlab://user/current
//	gitlab://groups
//	gitlab://group/{group_id}
//	gitlab://group/{group_id}/members
//	gitlab://group/{group_id}/projects
//	gitlab://group/{group_id}/milestone/{milestone_iid}
//	gitlab://group/{group_id}/label/{label_id}
//	gitlab://project/{project_id}
//	gitlab://project/{project_id}/members
//	gitlab://project/{project_id}/issues
//	gitlab://project/{project_id}/issue/{issue_iid}
//	gitlab://project/{project_id}/pipelines/latest
//	gitlab://project/{project_id}/pipeline/{pipeline_id}
//	gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs
//	gitlab://project/{project_id}/labels
//	gitlab://project/{project_id}/label/{label_id}
//	gitlab://project/{project_id}/milestones
//	gitlab://project/{project_id}/milestone/{milestone_iid}
//	gitlab://project/{project_id}/mr/{merge_request_iid}
//	gitlab://project/{project_id}/mr/{merge_request_iid}/notes
//	gitlab://project/{project_id}/mr/{merge_request_iid}/discussions
//	gitlab://project/{project_id}/branches
//	gitlab://project/{project_id}/branch/{branch}
//	gitlab://project/{project_id}/releases
//	gitlab://project/{project_id}/release/{tag_name}
//	gitlab://project/{project_id}/tags
//	gitlab://project/{project_id}/tag/{tag_name}
//	gitlab://project/{project_id}/commit/{sha}
//	gitlab://project/{project_id}/file/{ref}/{+path}
//	gitlab://project/{project_id}/wiki/{slug}
//	gitlab://project/{project_id}/deployment/{deployment_id}
//	gitlab://project/{project_id}/environment/{environment_id}
//	gitlab://project/{project_id}/job/{job_id}
//	gitlab://project/{project_id}/snippet/{snippet_id}
//	gitlab://project/{project_id}/feature_flag/{name}
//	gitlab://project/{project_id}/deploy_key/{deploy_key_id}
//	gitlab://project/{project_id}/board/{board_id}
//	gitlab://snippet/{snippet_id}
//
// Tool-manifest, workflow-guide, and meta/dynamic
// schema resources are registered separately by their dedicated
// Register* helpers (see doc.go for the full family breakdown).
// Register also returns a [HandlerIndex] keyed by URI template, so a caller
// can re-read a resource through the same handler the MCP router dispatches
// to. Callers that only need registration can ignore it.
// An optional [RegisterOptions] narrows the surface: a resource whose data an
// excluded action also served is neither registered on the server nor placed in
// the returned index, so it cannot be read and cannot be subscribed to either.
func Register(server *mcp.Server, client *gitlabclient.Client, opts ...RegisterOptions) HandlerIndex {
	rec := &recorder{server: server, index: make(HandlerIndex)}
	registerAll(registrarFor(rec, opts), client)
	return rec.index
}

// registerAll performs every registration against a registrar.
//
// Every handler registered here resolves the GitLab client it uses from the
// request context, with base.For(ctx), rather than from the client captured
// at registration. One mcp.Server is shared by all the credentials whose
// configuration hashes to the same shape, so the captured client is the
// credential-less one that refuses every request, and the per-request binding
// the HTTP layer installs is what makes a read reach the caller's own
// instance: registration decides which resources exist, the request decides
// whose GitLab answers them. On stdio, and in every test that registers with a
// real client, nothing is ever bound and For returns the captured client, so
// the resolution is invisible there.
func registerAll(server registrar, client *gitlabclient.Client) {
	registerCurrentUserResource(server, client)
	registerGroupsResource(server, client)
	registerGroupResource(server, client)
	registerGroupMembersResource(server, client)
	registerGroupProjectsResource(server, client)
	registerProjectResource(server, client)
	registerProjectMembersResource(server, client)
	registerProjectIssuesResource(server, client)
	registerIssueResource(server, client)
	registerLatestPipelineResource(server, client)
	registerPipelineResource(server, client)
	registerPipelineJobsResource(server, client)
	registerProjectLabelsResource(server, client)
	registerProjectMilestonesResource(server, client)
	registerMergeRequestResource(server, client)
	registerProjectBranchesResource(server, client)
	registerProjectReleasesResource(server, client)
	registerProjectTagsResource(server, client)
	registerCommitResource(server, client)
	registerFileBlobResource(server, client)
	registerWikiResource(server, client)
	registerMergeRequestNotesResource(server, client)
	registerMergeRequestDiscussionsResource(server, client)
	registerReleaseResource(server, client)
	registerBranchResource(server, client)
	registerTagResource(server, client)
	registerLabelResource(server, client)
	registerMilestoneResource(server, client)
	registerDeploymentResource(server, client)
	registerEnvironmentResource(server, client)
	registerJobResource(server, client)
	registerSnippetResource(server, client)
	registerProjectSnippetResource(server, client)
	registerFeatureFlagResource(server, client)
	registerDeployKeyResource(server, client)
	registerBoardResource(server, client)
	registerGroupMilestoneResource(server, client)
	registerGroupLabelResource(server, client)
}

// registerCurrentUserResource registers the "gitlab://user/current" static
// resource that returns the authenticated user's profile from the GitLab Users API.
func registerCurrentUserResource(server registrar, base *gitlabclient.Client) {
	server.AddResource(&mcp.Resource{
		URI:         "gitlab://user/current",
		Name:        "current_user",
		Title:       "Current User Profile",
		MIMEType:    mimeJSON,
		Description: "Get the currently authenticated GitLab user profile. Returns username, display name, email, state (active/blocked), admin status, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		u, _, err := client.GL().Users.CurrentUser(gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get current user", err)
		}
		out := UserResourceOutput{
			ID:       u.ID,
			Username: u.Username,
			Name:     u.Name,
			Email:    u.Email,
			State:    u.State,
			WebURL:   u.WebURL,
			IsAdmin:  u.IsAdmin,
		}
		return marshalResourceJSON(out)
	})
}

// registerGroupsResource registers the "gitlab://groups" static resource
// that lists all GitLab groups accessible to the authenticated user.
func registerGroupsResource(server registrar, base *gitlabclient.Client) {
	server.AddResource(&mcp.Resource{
		URI:         "gitlab://groups",
		Name:        "groups",
		Title:       "All Groups",
		MIMEType:    mimeJSON,
		Description: "Groups accessible to the authenticated user, up to one page (100). Returns each group's ID, name, full path, description, visibility level, and web URL.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		groups, resp, err := client.GL().Groups.ListGroups(&gl.ListGroupsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list groups", err)
		}
		out := make([]GroupResourceOutput, len(groups))
		for i, g := range groups {
			out[i] = GroupResourceOutput{
				ID:          g.ID,
				Name:        g.Name,
				Path:        g.Path,
				FullPath:    g.FullPath,
				Description: g.Description,
				Visibility:  string(g.Visibility),
				WebURL:      g.WebURL,
			}
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerProjectResource registers the "gitlab://project/{project_id}" template
// resource that returns basic metadata for a GitLab project.
func registerProjectResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}",
		Name:        "project",
		Title:       "Project Metadata",
		MIMEType:    mimeJSON,
		Description: "Get basic metadata for a GitLab project by numeric ID or URL-encoded path. Returns name, namespace path, visibility, web URL, description, and default branch.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractSuffix(req.Params.URI, uriProjectPrefix)
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError("gitlab://project/{project_id}")
		}
		p, _, err := client.GL().Projects.GetProject(projectID, &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out := ProjectResourceOutput{
			ID:                p.ID,
			Name:              p.Name,
			PathWithNamespace: p.PathWithNamespace,
			Visibility:        string(p.Visibility),
			WebURL:            p.WebURL,
			Description:       p.Description,
			DefaultBranch:     p.DefaultBranch,
		}
		return marshalResourceJSON(out)
	})
}

// registerProjectMembersResource registers the "gitlab://project/{project_id}/members"
// template resource that lists all members of a GitLab project, including
// inherited members from parent groups.
func registerProjectMembersResource(server registrar, base *gitlabclient.Client) {
	registerMembersResource(server, &mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/members",
		Name:        "project_members",
		Title:       "Project Members",
		Description: "Members of a GitLab project, up to one page (100), with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members from parent groups.",
	}, uriProjectPrefix, "failed to list project members", func(ctx context.Context, projectID string) ([]MemberResourceOutput, *gl.Response, error) {
		client := base.For(ctx)
		members, resp, err := client.GL().ProjectMembers.ListAllProjectMembers(projectID, &gl.ListProjectMembersOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, nil, err
		}
		out := make([]MemberResourceOutput, len(members))
		for i, m := range members {
			out[i] = projectMemberResourceOutput(m)
		}
		return out, resp, nil
	})
}

// registerLatestPipelineResource registers the
// "gitlab://project/{project_id}/pipelines/latest" template resource that
// returns the most recent CI/CD pipeline for a GitLab project.
func registerLatestPipelineResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipelines/latest",
		Name:        "latest_pipeline",
		Title:       "Latest Pipeline",
		MIMEType:    mimeJSON,
		Description: "Get the most recent CI/CD pipeline for a GitLab project. Returns pipeline ID, status (running/pending/success/failed/canceled), ref, SHA, source, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/pipelines/latest")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		p, _, err := client.GL().Pipelines.GetLatestPipeline(projectID, &gl.GetLatestPipelineOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get latest pipeline", err)
		}
		out := pipelineToResourceOutput(p)
		return marshalResourceJSON(out)
	})
}

// registerPipelineResource registers the
// "gitlab://project/{project_id}/pipeline/{pipeline_id}" template resource
// that returns details of a specific CI/CD pipeline by its numeric ID.
func registerPipelineResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipeline/{pipeline_id}",
		Name:        "pipeline",
		Title:       "Pipeline Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific CI/CD pipeline by its numeric ID. Returns pipeline status, ref, SHA, source, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		return readProjectIntResource(ctx, req, "/pipeline/", "failed to get pipeline",
			func(projectID string, pipelineID int64) (PipelineResourceOutput, error) {
				p, _, err := client.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(ctx))
				if err != nil {
					return PipelineResourceOutput{}, err
				}
				return pipelineToResourceOutput(p), nil
			})
	})
}

// registerPipelineJobsResource registers the
// "gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs" template
// resource that lists all jobs for a specific CI/CD pipeline.
func registerPipelineJobsResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs",
		Name:        "pipeline_jobs",
		Title:       "Pipeline Jobs",
		MIMEType:    mimeJSON,
		Description: "Jobs for a specific CI/CD pipeline, up to one page (100), including each job's name, stage, status, duration, failure reason (if failed), and web URL.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		uri := strings.TrimSuffix(req.Params.URI, "/jobs")
		projectID, pipelineIDStr := extractTwoParts(uri, uriProjectPrefix, "/pipeline/")
		if projectID == "" || pipelineIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		pipelineID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		jobs, resp, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, &gl.ListJobsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list pipeline jobs", err)
		}
		out := make([]JobResourceOutput, len(jobs))
		for i, j := range jobs {
			out[i] = JobResourceOutput{
				ID:            j.ID,
				Name:          j.Name,
				Stage:         j.Stage,
				Status:        j.Status,
				Ref:           j.Ref,
				Duration:      j.Duration,
				FailureReason: j.FailureReason,
				WebURL:        j.WebURL,
			}
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerProjectLabelsResource registers the
// "gitlab://project/{project_id}/labels" template resource that lists all
// labels defined in a GitLab project.
func registerProjectLabelsResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/labels",
		Name:        "project_labels",
		Title:       "Project Labels",
		MIMEType:    mimeJSON,
		Description: "Labels defined in a GitLab project, up to one page (100). Returns each label's name, color, description, and counts of open issues and merge requests using the label.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/labels")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		labels, resp, err := client.GL().Labels.ListLabels(projectID, &gl.ListLabelsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list labels", err)
		}
		out := make([]LabelResourceOutput, len(labels))
		for i, l := range labels {
			out[i] = LabelResourceOutput{
				ID:                     l.ID,
				Name:                   l.Name,
				Color:                  l.Color,
				Description:            l.Description,
				OpenIssuesCount:        l.OpenIssuesCount,
				OpenMergeRequestsCount: l.OpenMergeRequestsCount,
			}
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerProjectMilestonesResource registers the
// "gitlab://project/{project_id}/milestones" template resource that lists
// all milestones in a GitLab project.
func registerProjectMilestonesResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/milestones",
		Name:        "project_milestones",
		Title:       "Project Milestones",
		MIMEType:    mimeJSON,
		Description: "Milestones in a GitLab project, up to one page (100). Returns each milestone's title, description, state (active/closed), due date, and web URL.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/milestones")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		milestones, resp, err := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list milestones", err)
		}
		out := make([]MilestoneResourceOutput, len(milestones))
		for i, m := range milestones {
			ms := MilestoneResourceOutput{
				ID:          m.ID,
				IID:         m.IID,
				Title:       m.Title,
				Description: m.Description,
				State:       m.State,
				WebURL:      m.WebURL,
			}
			if m.DueDate != nil {
				ms.DueDate = m.DueDate.String()
			}
			out[i] = ms
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerMergeRequestResource registers the
// "gitlab://project/{project_id}/mr/{merge_request_iid}" template resource that
// returns details of a specific merge request by its project-scoped IID.
func registerMergeRequestResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{merge_request_iid}",
		Name:        "merge_request",
		Title:       "Merge Request Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific merge request by its IID (project-scoped ID). Returns title, state, source/target branches, author, merge status, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, mrIIDStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/mr/")
		if projectID == "" || mrIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mrIID, err := strconv.ParseInt(mrIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mr, _, err := client.GL().MergeRequests.GetMergeRequest(projectID, mrIID, &gl.GetMergeRequestsOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get merge request", err)
		}
		author := ""
		if mr.Author != nil {
			author = mr.Author.Username
		}
		out := MRResourceOutput{
			ID:           mr.ID,
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			Author:       author,
			WebURL:       mr.WebURL,
			MergeStatus:  mr.DetailedMergeStatus,
		}
		return marshalResourceJSON(out)
	})
}

// registerProjectBranchesResource registers the
// "gitlab://project/{project_id}/branches" template resource that lists
// all branches in a GitLab project repository.
func registerProjectBranchesResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/branches",
		Name:        "project_branches",
		Title:       "Project Branches",
		MIMEType:    mimeJSON,
		Description: "Branches in a GitLab project, up to one page (100). Returns each branch's name, protection status, merge status, default flag, and web URL.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/branches")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		branches, resp, err := client.GL().Branches.ListBranches(projectID, &gl.ListBranchesOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list branches", err)
		}
		out := make([]BranchResourceOutput, len(branches))
		for i, b := range branches {
			out[i] = BranchResourceOutput{
				Name:      b.Name,
				Protected: b.Protected,
				Merged:    b.Merged,
				Default:   b.Default,
				WebURL:    b.WebURL,
			}
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerGroupResource registers the "gitlab://group/{group_id}" template
// resource that returns details for a specific GitLab group.
func registerGroupResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}",
		Name:        "group",
		Title:       "Group Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a specific GitLab group by numeric ID or URL-encoded path. Returns name, full path, description, visibility, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		groupID := extractSuffix(req.Params.URI, uriGroupPrefix)
		if groupID == "" {
			return nil, mcp.ResourceNotFoundError("gitlab://group/{group_id}")
		}
		g, _, err := client.GL().Groups.GetGroup(groupID, &gl.GetGroupOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out := GroupResourceOutput{
			ID:          g.ID,
			Name:        g.Name,
			Path:        g.Path,
			FullPath:    g.FullPath,
			Description: g.Description,
			Visibility:  string(g.Visibility),
			WebURL:      g.WebURL,
		}
		return marshalResourceJSON(out)
	})
}

// registerGroupMembersResource registers the
// "gitlab://group/{group_id}/members" template resource that lists all
// members of a GitLab group, including inherited members.
func registerGroupMembersResource(server registrar, base *gitlabclient.Client) {
	registerMembersResource(server, &mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/members",
		Name:        "group_members",
		Title:       "Group Members",
		Description: "Members of a GitLab group, up to one page (100), with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members.",
	}, uriGroupPrefix, "failed to list group members", func(ctx context.Context, groupID string) ([]MemberResourceOutput, *gl.Response, error) {
		client := base.For(ctx)
		members, resp, err := client.GL().Groups.ListAllGroupMembers(groupID, &gl.ListGroupMembersOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, nil, err
		}
		out := make([]MemberResourceOutput, len(members))
		for i, m := range members {
			out[i] = groupMemberResourceOutput(m)
		}
		return out, resp, nil
	})
}

// The list callback returns GitLab's response alongside the members so the
// read can disclose whether it is the whole membership. See [listPageMetaKey].
func registerMembersResource(server registrar, tmpl *mcp.ResourceTemplate, uriPrefix, operation string, list func(context.Context, string) ([]MemberResourceOutput, *gl.Response, error)) {
	tmpl.MIMEType = mimeJSON
	tmpl.Annotations = toolutil.ResourceList
	tmpl.Icons = toolutil.IconUser
	server.AddResourceTemplate(tmpl, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		scopeID := extractMiddle(req.Params.URI, uriPrefix, "/members")
		if scopeID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out, resp, err := list(ctx, scopeID)
		if err != nil {
			return nil, wrapErr(operation, err)
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerGroupProjectsResource registers the
// "gitlab://group/{group_id}/projects" template resource that lists all
// projects within a GitLab group.
func registerGroupProjectsResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/projects",
		Name:        "group_projects",
		Title:       "Group Projects",
		MIMEType:    mimeJSON,
		Description: "Projects within a GitLab group, up to one page (100). Returns each project's ID, name, namespace path, visibility, web URL, description, and default branch.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		groupID := extractMiddle(req.Params.URI, uriGroupPrefix, "/projects")
		if groupID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		projects, resp, err := client.GL().Groups.ListGroupProjects(groupID, &gl.ListGroupProjectsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list group projects", err)
		}
		out := make([]ProjectResourceOutput, len(projects))
		for i, p := range projects {
			out[i] = ProjectResourceOutput{
				ID:                p.ID,
				Name:              p.Name,
				PathWithNamespace: p.PathWithNamespace,
				Visibility:        string(p.Visibility),
				WebURL:            p.WebURL,
				Description:       p.Description,
				DefaultBranch:     p.DefaultBranch,
			}
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerProjectIssuesResource registers the
// "gitlab://project/{project_id}/issues" template resource that lists
// open issues for a GitLab project.
func registerProjectIssuesResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/issues",
		Name:        "project_issues",
		Title:       "Project Issues",
		MIMEType:    mimeJSON,
		Description: "Open issues for a GitLab project, up to one page (100). Returns each issue's IID, title, state, labels, assignees, author, web URL, and creation date.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/issues")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		state := "opened"
		issues, resp, err := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
			PerPage: resourcePerPage,
			State:   &state,
		}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list project issues", err)
		}
		out := make([]IssueResourceOutput, len(issues))
		for i, issue := range issues {
			out[i] = issueToResourceOutput(issue)
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerIssueResource registers the
// "gitlab://project/{project_id}/issue/{issue_iid}" template resource that
// returns details of a specific issue by its project-scoped IID.
func registerIssueResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/issue/{issue_iid}",
		Name:        "issue",
		Title:       "Issue Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific issue by its IID (project-scoped ID). Returns title, state, labels, assignees, author, web URL, and creation date.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		return readProjectIntResource(ctx, req, "/issue/", "failed to get issue",
			func(projectID string, issueIID int64) (IssueResourceOutput, error) {
				issue, _, err := client.GL().Issues.GetIssue(projectID, issueIID, gl.WithContext(ctx))
				if err != nil {
					return IssueResourceOutput{}, err
				}
				return issueToResourceOutput(issue), nil
			})
	})
}

// registerProjectReleasesResource registers the
// "gitlab://project/{project_id}/releases" template resource that lists
// all releases for a GitLab project.
func registerProjectReleasesResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/releases",
		Name:        "project_releases",
		Title:       "Project Releases",
		MIMEType:    mimeJSON,
		Description: "Releases for a GitLab project, up to one page (100). Returns each release's tag name, name, description, author, and creation/release dates.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/releases")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		releases, resp, err := client.GL().Releases.ListReleases(projectID, &gl.ListReleasesOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list releases", err)
		}
		out := make([]ReleaseResourceOutput, len(releases))
		for i, r := range releases {
			ro := ReleaseResourceOutput{
				TagName:     r.TagName,
				Name:        r.Name,
				Description: r.Description,
				Author:      r.Author.Username,
			}
			if r.CreatedAt != nil {
				ro.CreatedAt = r.CreatedAt.UTC().Format(timeFormatISO)
			}
			if r.ReleasedAt != nil {
				ro.ReleasedAt = r.ReleasedAt.UTC().Format(timeFormatISO)
			}
			out[i] = ro
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerProjectTagsResource registers the
// "gitlab://project/{project_id}/tags" template resource that lists all
// repository tags for a GitLab project.
func registerProjectTagsResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/tags",
		Name:        "project_tags",
		Title:       "Project Tags",
		MIMEType:    mimeJSON,
		Description: "Repository tags for a GitLab project, up to one page (100). Returns each tag's name, message, target commit SHA, protection status, and creation date.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/tags")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		tags, resp, err := client.GL().Tags.ListTags(projectID, &gl.ListTagsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list tags", err)
		}
		out := make([]TagResourceOutput, len(tags))
		for i, tag := range tags {
			to := TagResourceOutput{
				Name:      tag.Name,
				Message:   tag.Message,
				Target:    tag.Target,
				Protected: tag.Protected,
			}
			if tag.CreatedAt != nil {
				to.CreatedAt = tag.CreatedAt.UTC().Format(timeFormatISO)
			}
			out[i] = to
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerCommitResource registers the
// "gitlab://project/{project_id}/commit/{sha}" template resource that returns
// details for a single commit including message, author/committer, parents
// and stats.
func registerCommitResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/commit/{sha}",
		Name:        "commit",
		Title:       "Commit Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single commit by SHA. Returns short_id, title, message, author, committer, authored/committed dates, parent commits, web URL, and stats (additions, deletions and their total).",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, sha := extractTwoParts(req.Params.URI, uriProjectPrefix, "/commit/")
		if projectID == "" || sha == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		c, _, err := client.GL().Commits.GetCommit(projectID, sha, nil, gl.WithContext(ctx))
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out := CommitResourceOutput{
			ID:          c.ID,
			ShortID:     c.ShortID,
			Title:       c.Title,
			Message:     c.Message,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			WebURL:      c.WebURL,
			ParentIDs:   c.ParentIDs,
		}
		if c.AuthoredDate != nil {
			out.AuthoredDate = c.AuthoredDate.UTC().Format(timeFormatISO)
		}
		if c.CommittedDate != nil {
			out.CommittedDate = c.CommittedDate.UTC().Format(timeFormatISO)
		}
		if c.Stats != nil {
			out.Stats = &CommitStatsOutput{
				Additions: c.Stats.Additions,
				Deletions: c.Stats.Deletions,
				Total:     c.Stats.Total,
			}
		}
		return marshalResourceJSON(out)
	})
}

// registerFileBlobResource registers the
// "gitlab://project/{project_id}/file/{ref}/{path}" template resource that
// returns the textual contents of a repository file. Files larger than
// fileBlobMaxBytes return metadata with content omitted and truncated=true.
// Binary content is omitted (only metadata returned).
func registerFileBlobResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/file/{ref}/{+path}",
		Name:        "file_blob",
		Title:       "Repository File",
		MIMEType:    mimeJSON,
		Description: "Get the contents of a repository file at a specific ref (branch, tag, or SHA). Path may include slashes. Files over 1 MiB return metadata only with truncated=true. Binary files return metadata with empty content.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, ref, filePath := extractFileBlobURI(req.Params.URI)
		if projectID == "" || ref == "" || filePath == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		opts := &gl.GetFileOptions{Ref: &ref}
		f, _, err := client.GL().RepositoryFiles.GetFile(projectID, filePath, opts, gl.WithContext(ctx))
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out := FileBlobResourceOutput{
			FileName:     f.FileName,
			FilePath:     f.FilePath,
			Size:         f.Size,
			Encoding:     f.Encoding,
			Ref:          f.Ref,
			BlobID:       f.BlobID,
			CommitID:     f.CommitID,
			LastCommitID: f.LastCommitID,
		}
		if f.Size > fileBlobMaxBytes {
			out.Truncated = true
			out.ContentCategory = "truncated"
			return marshalResourceJSON(out)
		}
		content, category := decodeFileContent(f)
		out.Content = content
		out.ContentCategory = category
		return marshalResourceJSON(out)
	})
}

// registerWikiResource registers the
// "gitlab://project/{project_id}/wiki/{slug}" template resource that returns
// a single wiki page by slug.
func registerWikiResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/wiki/{slug}",
		Name:        "wiki_page",
		Title:       "Wiki Page",
		MIMEType:    mimeJSON,
		Description: "Get a wiki page by slug. Returns title, slug, format (markdown/rdoc/asciidoc/org), and raw content. Slugs are case-sensitive and use hyphens for spaces.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, slug := extractTwoParts(req.Params.URI, uriProjectPrefix, "/wiki/")
		if projectID == "" || slug == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		w, _, err := client.GL().Wikis.GetWikiPage(projectID, slug, &gl.GetWikiPageOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		out := WikiResourceOutput{
			Title:    w.Title,
			Slug:     w.Slug,
			Format:   string(w.Format),
			Content:  w.Content,
			Encoding: w.Encoding,
		}
		return marshalResourceJSON(out)
	})
}

// registerMergeRequestNotesResource registers the
// "gitlab://project/{project_id}/mr/{merge_request_iid}/notes" template resource that
// returns the flat list of notes (comments) for a merge request.
func registerMergeRequestNotesResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{merge_request_iid}/notes",
		Name:        "merge_request_notes",
		Title:       "Merge Request Notes",
		MIMEType:    mimeJSON,
		Description: "List notes (comments) on a merge request. Returns up to one page of 100 notes, newest ordering as GitLab returns it. A busier merge request has more than this resource shows. Each note carries id, author username, body, system flag, resolvable/resolved flags, and timestamps.",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		uri := strings.TrimSuffix(req.Params.URI, "/notes")
		projectID, mrIIDStr := extractTwoParts(uri, uriProjectPrefix, "/mr/")
		if projectID == "" || mrIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mrIID, err := strconv.ParseInt(mrIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		notes, resp, err := client.GL().Notes.ListMergeRequestNotes(projectID, mrIID, &gl.ListMergeRequestNotesOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list merge request notes", err)
		}
		out := make([]MRNoteResourceOutput, len(notes))
		for i, n := range notes {
			no := MRNoteResourceOutput{
				ID:         n.ID,
				Body:       n.Body,
				System:     n.System,
				Resolvable: n.Resolvable,
				Resolved:   n.Resolved,
			}
			if n.Author.Username != "" {
				no.Author = n.Author.Username
			}
			if n.CreatedAt != nil {
				no.CreatedAt = n.CreatedAt.UTC().Format(timeFormatISO)
			}
			if n.UpdatedAt != nil {
				no.UpdatedAt = n.UpdatedAt.UTC().Format(timeFormatISO)
			}
			out[i] = no
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerMergeRequestDiscussionsResource registers the
// "gitlab://project/{project_id}/mr/{merge_request_iid}/discussions" template resource
// that returns the discussion threads for a merge request, each containing
// one or more notes.
func registerMergeRequestDiscussionsResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{merge_request_iid}/discussions",
		Name:        "merge_request_discussions",
		Title:       "Merge Request Discussions",
		MIMEType:    mimeJSON,
		Description: "List discussion threads on a merge request. Returns up to one page of 100 discussions. A busier merge request has more than this resource shows. Each discussion has an id, individual_note flag, and an array of notes (id, author, body, system, resolved/resolvable, created_at).",
		Annotations: toolutil.ResourceList,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		uri := strings.TrimSuffix(req.Params.URI, "/discussions")
		projectID, mrIIDStr := extractTwoParts(uri, uriProjectPrefix, "/mr/")
		if projectID == "" || mrIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mrIID, err := strconv.ParseInt(mrIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		discussions, resp, err := client.GL().Discussions.ListMergeRequestDiscussions(projectID, mrIID, &gl.ListMergeRequestDiscussionsOptions{PerPage: resourcePerPage}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list merge request discussions", err)
		}
		out := make([]MRDiscussionResourceOutput, len(discussions))
		for i, d := range discussions {
			dout := MRDiscussionResourceOutput{
				ID:             d.ID,
				IndividualNote: d.IndividualNote,
				Notes:          make([]MRDiscussionNoteResourceOutput, 0, len(d.Notes)),
			}
			for _, n := range d.Notes {
				if n == nil {
					continue
				}
				no := MRDiscussionNoteResourceOutput{
					ID:         n.ID,
					Body:       n.Body,
					System:     n.System,
					Resolved:   n.Resolved,
					Resolvable: n.Resolvable,
				}
				if n.Author.Username != "" {
					no.Author = n.Author.Username
				}
				if n.CreatedAt != nil {
					no.CreatedAt = n.CreatedAt.UTC().Format(timeFormatISO)
				}
				dout.Notes = append(dout.Notes, no)
			}
			out[i] = dout
		}
		return marshalResourceList(out, pageOf(len(out), resp))
	})
}

// registerReleaseResource registers the
// "gitlab://project/{project_id}/release/{tag_name}" template resource that
// returns details for a single release identified by its Git tag.
func registerReleaseResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/release/{tag_name}",
		Name:        "release",
		Title:       "Release Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single GitLab release by tag name. Returns tag_name, name, description, author, creation/release dates.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, tagName := extractTwoParts(req.Params.URI, uriProjectPrefix, "/release/")
		if projectID == "" || tagName == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		r, _, err := client.GL().Releases.GetRelease(projectID, tagName, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get release", err)
		}
		out := ReleaseResourceOutput{
			TagName:     r.TagName,
			Name:        r.Name,
			Description: r.Description,
			Author:      r.Author.Username,
		}
		if r.CreatedAt != nil {
			out.CreatedAt = r.CreatedAt.UTC().Format(timeFormatISO)
		}
		if r.ReleasedAt != nil {
			out.ReleasedAt = r.ReleasedAt.UTC().Format(timeFormatISO)
		}
		return marshalResourceJSON(out)
	})
}

// registerBranchResource registers the
// "gitlab://project/{project_id}/branch/{branch}" template resource,
// which returns details for a single repository branch.
func registerBranchResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/branch/{branch}",
		Name:        "branch",
		Title:       "Branch Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single repository branch. Returns name, protection status, merge status, default flag, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, branch := extractTwoParts(req.Params.URI, uriProjectPrefix, "/branch/")
		if projectID == "" || branch == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		b, _, err := client.GL().Branches.GetBranch(projectID, branch, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get branch", err)
		}
		out := BranchResourceOutput{
			Name:      b.Name,
			Protected: b.Protected,
			Merged:    b.Merged,
			Default:   b.Default,
			WebURL:    b.WebURL,
		}
		return marshalResourceJSON(out)
	})
}

// registerTagResource registers the
// "gitlab://project/{project_id}/tag/{tag_name}" template resource,
// which returns details for a single Git tag.
func registerTagResource(server registrar, base *gitlabclient.Client) {
	registerProjectNamedResource(server, base, &mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/tag/{tag_name}",
		Name:        "tag",
		Title:       "Tag Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single Git tag. Returns name, target commit SHA, annotation message, and protection status.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconTag,
	}, "/tag/", "failed to get tag",
		func(ctx context.Context, client *gitlabclient.Client, projectID, tagName string) (TagResourceOutput, error) {
			t, _, err := client.GL().Tags.GetTag(projectID, tagName, gl.WithContext(ctx))
			if err != nil {
				return TagResourceOutput{}, err
			}
			return TagResourceOutput{Name: t.Name, Message: t.Message, Target: t.Target, Protected: t.Protected}, nil
		})
}

// registerProjectNamedResource registers a template whose handler reads one
// named child of a project, resolving the request's client first.
//
// The two handlers that take this shape were identical down to the token once
// the client resolution was added to each, which is what makes the helper worth
// having rather than two copies with a lint exemption on them.
func registerProjectNamedResource[O any](
	server registrar,
	base *gitlabclient.Client,
	template *mcp.ResourceTemplate,
	separator, operation string,
	read func(ctx context.Context, client *gitlabclient.Client, projectID, name string) (O, error),
) {
	server.AddResourceTemplate(template, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		return readProjectNamedResource(ctx, req, separator, operation,
			func(projectID, name string) (O, error) {
				return read(ctx, client, projectID, name)
			})
	})
}

// registerLabelResource registers the
// "gitlab://project/{project_id}/label/{label_id}" template resource,
// which returns details for a single project label by numeric ID or
// label name.
func registerLabelResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/label/{label_id}",
		Name:        "label",
		Title:       "Label Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project label by numeric ID or label name. Returns id, name, color, description, and open issue/MR counts.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, labelID := extractTwoParts(req.Params.URI, uriProjectPrefix, "/label/")
		if projectID == "" || labelID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		l, _, err := client.GL().Labels.GetLabel(projectID, labelID, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get label", err)
		}
		out := LabelResourceOutput{
			ID:                     l.ID,
			Name:                   l.Name,
			Color:                  l.Color,
			Description:            l.Description,
			OpenIssuesCount:        l.OpenIssuesCount,
			OpenMergeRequestsCount: l.OpenMergeRequestsCount,
		}
		return marshalResourceJSON(out)
	})
}

// registerMilestoneResource registers the
// "gitlab://project/{project_id}/milestone/{milestone_iid}" template
// resource, which returns details for a single project milestone
// identified by its project-scoped IID. Internally, it lists
// milestones filtered by IID because the GitLab Milestones API
// exposes only a list endpoint.
func registerMilestoneResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/milestone/{milestone_iid}",
		Name:        "milestone",
		Title:       "Milestone Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project milestone by IID. Returns id, iid, title, description, state, due date, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, iidStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/milestone/")
		if projectID == "" || iidStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		iid, err := strconv.ParseInt(iidStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		iids := []int64{iid}
		ms, _, err := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{IIDs: &iids}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to resolve milestone IID", err)
		}
		if len(ms) == 0 {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		m := ms[0]
		out := MilestoneResourceOutput{
			ID:          m.ID,
			IID:         m.IID,
			Title:       m.Title,
			Description: m.Description,
			State:       m.State,
			WebURL:      m.WebURL,
		}
		if m.DueDate != nil {
			out.DueDate = m.DueDate.String()
		}
		return marshalResourceJSON(out)
	})
}

// extractFileBlobURI splits a "gitlab://project/{id}/file/{ref}/{path}"
// URI into its three components. The path component may contain slashes.
// Returns empty strings if the URI does not match the expected layout.
//
// Limitation: when the ref itself contains a slash (e.g. "feature/new-ui"),
// the URI is ambiguous because both segments use "/" as a separator. This
// helper assumes refs are slash-free. Callers that need to address files
// on branches with slashes should URL-encode the ref before constructing
// the URI.
func extractFileBlobURI(uri string) (projectID, ref, filePath string) {
	rest := extractSuffix(uri, uriProjectPrefix)
	if rest == "" {
		return "", "", ""
	}
	idx := strings.Index(rest, "/file/")
	if idx <= 0 {
		return "", "", ""
	}
	projectID = rest[:idx]
	tail := rest[idx+len("/file/"):]
	var ok bool
	ref, filePath, ok = strings.Cut(tail, "/")
	if !ok || ref == "" || filePath == "" {
		return "", "", ""
	}
	return projectID, ref, filePath
}

// decodeFileContent decodes the contents of a [gl.File] returned by the
// RepositoryFiles GitLab API. Base64 encoded payloads are decoded;
// binary content is detected via the file name (or via invalid UTF-8
// after decoding) and replaced with an empty string so JSON responses
// stay textual. Returns the decoded content and a human-readable
// category ("text" or "binary").
func decodeFileContent(f *gl.File) (content, category string) {
	if f == nil {
		return "", "binary"
	}
	if f.Encoding != "base64" {
		if toolutil.IsBinaryFile(f.FileName) {
			return "", "binary"
		}
		return f.Content, "text"
	}
	decoded, err := base64.StdEncoding.DecodeString(f.Content)
	if err != nil {
		return "", "binary"
	}
	if toolutil.IsBinaryFile(f.FileName) || !utf8.Valid(decoded) {
		return "", "binary"
	}
	return string(decoded), "text"
}

// issueToResourceOutput converts a GitLab API [gl.Issue] to the MCP
// resource output format, extracting the author username, the list of
// assignee usernames, and formatting the creation timestamp.
func issueToResourceOutput(issue *gl.Issue) IssueResourceOutput {
	out := IssueResourceOutput{
		ID:     issue.ID,
		IID:    issue.IID,
		Title:  issue.Title,
		State:  issue.State,
		Labels: issue.Labels,
		WebURL: issue.WebURL,
	}
	if issue.Author != nil {
		out.Author = issue.Author.Username
	}
	if issue.CreatedAt != nil {
		out.CreatedAt = issue.CreatedAt.UTC().Format(timeFormatISO)
	}
	assignees := make([]string, 0, len(issue.Assignees))
	for _, a := range issue.Assignees {
		if a != nil {
			assignees = append(assignees, a.Username)
		}
	}
	out.Assignees = assignees
	return out
}

// URI parsing helpers.

// extractSuffix returns the portion of uri after the given prefix, or
// the empty string when uri does not start with prefix.
func extractSuffix(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return strings.TrimPrefix(uri, prefix)
}

// extractMiddle returns the substring of uri between prefix and suffix.
// Both prefix and suffix must match exactly; otherwise the empty string
// is returned.
func extractMiddle(uri, prefix, suffix string) string {
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return ""
	}
	return uri[len(prefix) : len(uri)-len(suffix)]
}

// extractTwoParts splits a URI into two dynamic segments around
// separator. Both segments must be non-empty for the split to succeed;
// the empty string is returned for both when the URI does not match
// the expected layout.
func extractTwoParts(uri, prefix, separator string) (first, second string) {
	rest := extractSuffix(uri, prefix)
	if rest == "" {
		return "", ""
	}
	parts := strings.SplitN(rest, separator, 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", ""
	}
	return parts[0], parts[1]
}

// readProjectIntResource is the shared handler for resources that
// extract a (projectID, int64) pair from the URI and dispatch to a
// GitLab API call. The supplied separator and operation label are used
// to build the resource-not-found and error responses.
func readProjectIntResource[O any](_ context.Context, req *mcp.ReadResourceRequest, separator, operation string, read func(string, int64) (O, error)) (*mcp.ReadResourceResult, error) {
	projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, separator)
	if projectID == "" || idStr == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	out, err := read(projectID, id)
	if err != nil {
		return nil, wrapErr(operation, err)
	}
	return marshalResourceJSON(out)
}

// readProjectNamedResource is the shared handler for resources that
// extract a (projectID, name) pair from the URI and dispatch to a
// GitLab API call. It mirrors [readProjectIntResource] but takes a
// string for the second segment (e.g. branch name, tag name, feature
// flag name).
func readProjectNamedResource[O any](_ context.Context, req *mcp.ReadResourceRequest, separator, operation string, read func(string, string) (O, error)) (*mcp.ReadResourceResult, error) {
	projectID, name := extractTwoParts(req.Params.URI, uriProjectPrefix, separator)
	if projectID == "" || name == "" {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	out, err := read(projectID, name)
	if err != nil {
		return nil, wrapErr(operation, err)
	}
	return marshalResourceJSON(out)
}

// marshalResourceJSON marshals a value to JSON and wraps it in a
// [*mcp.ReadResourceResult] as a single text contents block with
// [mimeJSON]. The marshal error is returned wrapped in a
// "failed to marshal resource" prefix.
func marshalResourceJSON(v any) (*mcp.ReadResourceResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal resource: %w", err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			MIMEType: mimeJSON,
			Text:     string(data),
		}},
	}, nil
}

// resourcePerPage is the page size every collection resource asks GitLab for.
//
// It is GitLab's maximum. The default is 20, which is what these resources were
// silently getting: gitlab://groups answered 20 of 137 with nothing to say so.
const resourcePerPage = 100

// listPageMetaKey carries the completeness of a collection read.
//
// resources/read has no continuation mechanism. The transport specification
// scopes pagination to the list operations (resources/list,
// resources/templates/list, tools/list, prompts/list) and gives read none, so
// there is no cursor a client could follow and no partial-result shape in the
// schema. Disclosure is all a server can offer, and it belongs in _meta rather
// than in the payload: the resource body is what a consumer parses, it has no
// negotiated shape, and wrapping the array in an object would break every
// reader of it.
//
// Reverse-DNS keyed, matching [subscribableMetaKey], which is the extension
// point the specification sanctions for exactly this.
const listPageMetaKey = "io.github.jmrplens/pageInfo"

// listPage is what a resource read knows about the collection it returned.
type listPage struct {
	// Returned is how many items are in the body.
	Returned int `json:"returned"`
	// Total is how many exist, when GitLab said. Omitted when it did not:
	// keyset-paginated and unindexed endpoints send no X-Total.
	Total int `json:"total,omitempty"`
	// Complete reports whether the body is the whole collection. False means a
	// consumer needing completeness must use the tool surface, which paginates
	// properly, rather than reading this resource again.
	Complete bool `json:"complete"`
}

// pageOf reads the completeness of a response GitLab has just answered.
func pageOf(returned int, resp *gl.Response) listPage {
	page := listPage{Returned: returned, Complete: true}
	if resp == nil {
		return page
	}
	page.Total = int(resp.TotalItems)
	// NextPage is the honest signal: X-Total is absent on endpoints GitLab
	// paginates by keyset or declines to count, and a missing total is not a
	// statement that there is nothing more.
	if resp.NextPage > 0 {
		page.Complete = false
	}
	return page
}

// marshalResourceList is [marshalResourceJSON] for a collection, adding the
// _meta that says whether the collection is all of it.
func marshalResourceList(v any, page listPage) (*mcp.ReadResourceResult, error) {
	result, err := marshalResourceJSON(v)
	if err != nil {
		return nil, err
	}
	result.Meta = mcp.Meta{listPageMetaKey: page}
	return result, nil
}

// pipelineToResourceOutput converts a GitLab API [gl.Pipeline] to the
// MCP resource output format, mapping the ID, IID, status, ref, SHA,
// web URL, and source.
func pipelineToResourceOutput(p *gl.Pipeline) PipelineResourceOutput {
	return PipelineResourceOutput{
		ID:     p.ID,
		IID:    p.IID,
		Status: p.Status,
		Ref:    p.Ref,
		SHA:    p.SHA,
		WebURL: p.WebURL,
		Source: string(p.Source),
	}
}

// extractGroupTwoParts splits a "gitlab://group/{group_id}/{kind}/{value}"
// URI into its (group_id, value) components. The kind argument is
// interpolated into the separator so the helper is reusable across
// group milestone, label, and similar lookups.
func extractGroupTwoParts(uri, kind string) (groupID, value string) {
	return extractTwoParts(uri, uriGroupPrefix, "/"+kind+"/")
}

// registerDeploymentResource registers the
// "gitlab://project/{project_id}/deployment/{deployment_id}" template
// resource, which returns details for a single project deployment by
// numeric ID.
func registerDeploymentResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/deployment/{deployment_id}",
		Name:        "deployment",
		Title:       "Deployment Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project deployment by numeric ID. Returns id, iid, ref, sha, status, and environment name.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconDeploy,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/deployment/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		d, _, err := client.GL().Deployments.GetProjectDeployment(projectID, id, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get deployment", err)
		}
		out := DeploymentResourceOutput{
			ID:     d.ID,
			IID:    d.IID,
			Ref:    d.Ref,
			SHA:    d.SHA,
			Status: d.Status,
		}
		if d.Environment != nil {
			out.Environment = d.Environment.Name
		}
		return marshalResourceJSON(out)
	})
}

// registerEnvironmentResource registers the
// "gitlab://project/{project_id}/environment/{environment_id}" template
// resource, which returns details for a single project environment by
// numeric ID.
func registerEnvironmentResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/environment/{environment_id}",
		Name:        "environment",
		Title:       "Environment Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project environment by numeric ID. Returns id, name, slug, state, and tier.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconEnvironment,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/environment/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		env, _, err := client.GL().Environments.GetEnvironment(projectID, int64(id), gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get environment", err)
		}
		out := EnvironmentResourceOutput{
			ID:    env.ID,
			Name:  env.Name,
			Slug:  env.Slug,
			State: env.State,
			Tier:  env.Tier,
		}
		return marshalResourceJSON(out)
	})
}

// registerJobResource registers the
// "gitlab://project/{project_id}/job/{job_id}" template resource, which
// returns details for a single CI job by numeric ID.
func registerJobResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/job/{job_id}",
		Name:        "job",
		Title:       "Job Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single CI job by numeric ID. Returns id, name, stage, status, ref, duration, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/job/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		j, _, err := client.GL().Jobs.GetJob(projectID, id, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get job", err)
		}
		out := JobResourceOutput{
			ID:            j.ID,
			Name:          j.Name,
			Stage:         j.Stage,
			Status:        j.Status,
			Ref:           j.Ref,
			Duration:      j.Duration,
			FailureReason: j.FailureReason,
			WebURL:        j.WebURL,
		}
		return marshalResourceJSON(out)
	})
}

// registerSnippetResource registers the "gitlab://snippet/{snippet_id}"
// template resource, which returns details for a personal (global) snippet
// by numeric ID.
func registerSnippetResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://snippet/{snippet_id}",
		Name:        "snippet",
		Title:       "Snippet Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single personal/global snippet by numeric ID. Returns id, title, file_name, description, visibility, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		idStr := extractSuffix(req.Params.URI, uriSnippetPrefix)
		if idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		s, _, err := client.GL().Snippets.GetSnippet(int64(id), gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get snippet", err)
		}
		out := SnippetResourceOutput{
			ID:          s.ID,
			Title:       s.Title,
			FileName:    s.FileName,
			Description: s.Description,
			Visibility:  s.Visibility,
			WebURL:      s.WebURL,
		}
		return marshalResourceJSON(out)
	})
}

// registerProjectSnippetResource registers the
// "gitlab://project/{project_id}/snippet/{snippet_id}" template
// resource, which returns details for a single project snippet by
// numeric ID.
func registerProjectSnippetResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/snippet/{snippet_id}",
		Name:        "project_snippet",
		Title:       "Project Snippet Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project snippet by numeric ID. Returns id, title, file_name, description, visibility, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconSnippet,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/snippet/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		s, _, err := client.GL().ProjectSnippets.GetSnippet(projectID, int64(id), gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get project snippet", err)
		}
		out := SnippetResourceOutput{
			ID:          s.ID,
			Title:       s.Title,
			FileName:    s.FileName,
			Description: s.Description,
			Visibility:  s.Visibility,
			WebURL:      s.WebURL,
		}
		return marshalResourceJSON(out)
	})
}

// registerFeatureFlagResource registers the
// "gitlab://project/{project_id}/feature_flag/{name}" template
// resource, which returns details for a single project feature flag
// by name.
func registerFeatureFlagResource(server registrar, base *gitlabclient.Client) {
	registerProjectNamedResource(server, base, &mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/feature_flag/{name}",
		Name:        "feature_flag",
		Title:       "Feature Flag Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project feature flag by name. Returns name, description, active, and version.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconConfig,
	}, "/feature_flag/", "failed to get feature flag",
		func(ctx context.Context, client *gitlabclient.Client, projectID, name string) (FeatureFlagResourceOutput, error) {
			f, _, err := client.GL().ProjectFeatureFlags.GetProjectFeatureFlag(projectID, name, gl.WithContext(ctx))
			if err != nil {
				return FeatureFlagResourceOutput{}, err
			}
			return FeatureFlagResourceOutput{Name: f.Name, Description: f.Description, Active: f.Active, Version: f.Version}, nil
		})
}

// registerDeployKeyResource registers the
// "gitlab://project/{project_id}/deploy_key/{deploy_key_id}" template
// resource, which returns details for a single project deploy key by
// numeric ID.
func registerDeployKeyResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/deploy_key/{deploy_key_id}",
		Name:        "deploy_key",
		Title:       "Deploy Key Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project deploy key by numeric ID. Returns id, title, key, and fingerprint.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconKey,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/deploy_key/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		k, _, err := client.GL().DeployKeys.GetDeployKey(projectID, id, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get deploy key", err)
		}
		out := DeployKeyResourceOutput{
			ID:          k.ID,
			Title:       k.Title,
			Key:         k.Key,
			Fingerprint: k.Fingerprint,
		}
		return marshalResourceJSON(out)
	})
}

// registerBoardResource registers the
// "gitlab://project/{project_id}/board/{board_id}" template resource,
// which returns details for a single project issue board by numeric
// ID.
func registerBoardResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/board/{board_id}",
		Name:        "board",
		Title:       "Board Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single project issue board by numeric ID. Returns id and name.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconBoard,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		projectID, idStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/board/")
		if projectID == "" || idStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		b, _, err := client.GL().Boards.GetIssueBoard(projectID, int64(id), gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get board", err)
		}
		out := BoardResourceOutput{ID: b.ID, Name: b.Name}
		return marshalResourceJSON(out)
	})
}

// registerGroupMilestoneResource registers the
// "gitlab://group/{group_id}/milestone/{milestone_iid}" template
// resource, which returns details for a single group milestone by IID.
func registerGroupMilestoneResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/milestone/{milestone_iid}",
		Name:        "group_milestone",
		Title:       "Group Milestone Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single group milestone by IID. Returns id, iid, title, description, state, due date, and web URL.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		groupID, iidStr := extractGroupTwoParts(req.Params.URI, "milestone")
		if groupID == "" || iidStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		iid, err := strconv.ParseInt(iidStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		iids := []int64{iid}
		ms, _, err := client.GL().GroupMilestones.ListGroupMilestones(groupID, &gl.ListGroupMilestonesOptions{IIDs: &iids}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to resolve group milestone IID", err)
		}
		if len(ms) == 0 {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		m := ms[0]
		out := MilestoneResourceOutput{
			ID:          m.ID,
			IID:         m.IID,
			Title:       m.Title,
			Description: m.Description,
			State:       m.State,
		}
		if m.DueDate != nil {
			out.DueDate = m.DueDate.String()
		}
		return marshalResourceJSON(out)
	})
}

// registerGroupLabelResource registers the
// "gitlab://group/{group_id}/label/{label_id}" template resource,
// which returns details for a single group label by numeric ID or
// name.
func registerGroupLabelResource(server registrar, base *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/label/{label_id}",
		Name:        "group_label",
		Title:       "Group Label Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single group label by numeric ID or name. Returns id, name, color, description, and open issue/MR counts.",
		Annotations: toolutil.ResourceDetail,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		client := base.For(ctx)
		groupID, labelID := extractGroupTwoParts(req.Params.URI, "label")
		if groupID == "" || labelID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		l, _, err := client.GL().GroupLabels.GetGroupLabel(groupID, labelID, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get group label", err)
		}
		out := LabelResourceOutput{
			ID:                     l.ID,
			Name:                   l.Name,
			Color:                  l.Color,
			Description:            l.Description,
			OpenIssuesCount:        l.OpenIssuesCount,
			OpenMergeRequestsCount: l.OpenMergeRequestsCount,
		}
		return marshalResourceJSON(out)
	})
}
