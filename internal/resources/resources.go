// Package resources registers MCP resources that expose read-only GitLab
// project data (metadata, branches, merge requests, pipelines) via stable URIs.
package resources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// Resource output structs.

// ProjectResourceOutput is the output for the project resource.
type ProjectResourceOutput struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	PathWithNamespace string `json:"path_with_namespace"`
	Visibility        string `json:"visibility"`
	WebURL            string `json:"web_url"`
	Description       string `json:"description"`
	DefaultBranch     string `json:"default_branch"`
}

// UserResourceOutput is the output for the current user resource.
type UserResourceOutput struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	State    string `json:"state"`
	WebURL   string `json:"web_url"`
	IsAdmin  bool   `json:"is_admin"`
}

// MemberResourceOutput is the output for a project member.
type MemberResourceOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Name        string `json:"name"`
	State       string `json:"state"`
	AccessLevel int    `json:"access_level"`
	WebURL      string `json:"web_url"`
}

// PipelineResourceOutput is the output for a pipeline.
type PipelineResourceOutput struct {
	ID     int64  `json:"id"`
	IID    int64  `json:"iid"`
	Status string `json:"status"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	WebURL string `json:"web_url"`
	Source string `json:"source"`
}

// JobResourceOutput is the output for a pipeline job.
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

// LabelResourceOutput is the output for a project label.
type LabelResourceOutput struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	Color                  string `json:"color"`
	Description            string `json:"description"`
	OpenIssuesCount        int64  `json:"open_issues_count"`
	OpenMergeRequestsCount int64  `json:"open_merge_requests_count"`
}

// MilestoneResourceOutput is the output for a project milestone.
type MilestoneResourceOutput struct {
	ID          int64  `json:"id"`
	IID         int64  `json:"iid"`
	Title       string `json:"title"`
	Description string `json:"description"`
	State       string `json:"state"`
	DueDate     string `json:"due_date,omitempty"`
	WebURL      string `json:"web_url"`
}

// MRResourceOutput is the output for a merge request resource.
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

// BranchResourceOutput is the output for a repository branch.
type BranchResourceOutput struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Merged    bool   `json:"merged"`
	Default   bool   `json:"default"`
	WebURL    string `json:"web_url"`
}

// GroupResourceOutput is the output for a GitLab group.
type GroupResourceOutput struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Path        string `json:"path"`
	FullPath    string `json:"full_path"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	WebURL      string `json:"web_url"`
}

// IssueResourceOutput is the output for a project issue.
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

// ReleaseResourceOutput is the output for a project release.
type ReleaseResourceOutput struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Author      string `json:"author"`
	CreatedAt   string `json:"created_at"`
	ReleasedAt  string `json:"released_at,omitempty"`
}

// TagResourceOutput is the output for a repository tag.
type TagResourceOutput struct {
	Name      string `json:"name"`
	Message   string `json:"message,omitempty"`
	Target    string `json:"target"`
	Protected bool   `json:"protected"`
	CreatedAt string `json:"created_at,omitempty"`
}

// CommitResourceOutput is the output for a single commit resource.
type CommitResourceOutput struct {
	ID            string                 `json:"id"`
	ShortID       string                 `json:"short_id"`
	Title         string                 `json:"title"`
	Message       string                 `json:"message"`
	AuthorName    string                 `json:"author_name"`
	AuthorEmail   string                 `json:"author_email"`
	AuthoredDate  string                 `json:"authored_date,omitempty"`
	CommittedDate string                 `json:"committed_date,omitempty"`
	WebURL        string                 `json:"web_url"`
	ParentIDs     []string               `json:"parent_ids,omitempty"`
	Stats         *CommitStatsOutput     `json:"stats,omitempty"`
}

// CommitStatsOutput holds additions/deletions/total for a commit resource.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// FileBlobResourceOutput is the output for a repository file blob resource.
// Binary content is omitted; only the textual representation is returned.
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

// WikiResourceOutput is the output for a wiki page resource.
type WikiResourceOutput struct {
	Title    string `json:"title"`
	Slug     string `json:"slug"`
	Format   string `json:"format"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

// MRNoteResourceOutput is the output for a single merge-request note inside
// the MR notes resource.
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

// MRDiscussionNoteResourceOutput is the output for a note inside a discussion
// thread of the MR discussions resource.
type MRDiscussionNoteResourceOutput struct {
	ID         int64  `json:"id"`
	Author     string `json:"author"`
	Body       string `json:"body"`
	System     bool   `json:"system"`
	Resolved   bool   `json:"resolved"`
	Resolvable bool   `json:"resolvable"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// MRDiscussionResourceOutput is the output for a single discussion thread.
type MRDiscussionResourceOutput struct {
	ID             string                           `json:"id"`
	IndividualNote bool                             `json:"individual_note"`
	Notes          []MRDiscussionNoteResourceOutput `json:"notes"`
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
	timeFormatISO    = "2006-01-02T15:04:05Z"
)

// wrapErr enriches a GitLab API error with HTTP status classification.
func wrapErr(msg string, err error) error {
	return fmt.Errorf("%s: %s (%w)", msg, toolutil.ClassifyError(err), err)
}

// Register registers all MCP resources (read-only data endpoints).
func Register(server *mcp.Server, client *gitlabclient.Client) {
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
}

// registerCurrentUserResource registers the "gitlab://user/current" static
// resource that returns the authenticated user's profile from the GitLab Users API.
func registerCurrentUserResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResource(&mcp.Resource{
		URI:         "gitlab://user/current",
		Name:        "current_user",
		Title:       "Current User Profile",
		MIMEType:    mimeJSON,
		Description: "Get the currently authenticated GitLab user profile. Returns username, display name, email, state (active/blocked), admin status, and web URL.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerGroupsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResource(&mcp.Resource{
		URI:         "gitlab://groups",
		Name:        "groups",
		Title:       "All Groups",
		MIMEType:    mimeJSON,
		Description: "List all GitLab groups accessible to the authenticated user. Returns each group's ID, name, full path, description, visibility level, and web URL.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		groups, _, err := client.GL().Groups.ListGroups(&gl.ListGroupsOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerProjectResource registers the "gitlab://project/{project_id}" template
// resource that returns basic metadata for a GitLab project.
func registerProjectResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}",
		Name:        "project",
		Title:       "Project Metadata",
		MIMEType:    mimeJSON,
		Description: "Get basic metadata for a GitLab project by numeric ID or URL-encoded path. Returns name, namespace path, visibility, web URL, description, and default branch.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerProjectMembersResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/members",
		Name:        "project_members",
		Title:       "Project Members",
		MIMEType:    mimeJSON,
		Description: "List all members of a GitLab project with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members from parent groups.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/members")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		members, _, err := client.GL().ProjectMembers.ListAllProjectMembers(projectID, &gl.ListProjectMembersOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list project members", err)
		}
		out := make([]MemberResourceOutput, len(members))
		for i, m := range members {
			out[i] = MemberResourceOutput{
				ID:          m.ID,
				Username:    m.Username,
				Name:        m.Name,
				State:       m.State,
				AccessLevel: int(m.AccessLevel),
				WebURL:      m.WebURL,
			}
		}
		return marshalResourceJSON(out)
	})
}

// registerLatestPipelineResource registers the
// "gitlab://project/{project_id}/pipelines/latest" template resource that
// returns the most recent CI/CD pipeline for a GitLab project.
func registerLatestPipelineResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipelines/latest",
		Name:        "latest_pipeline",
		Title:       "Latest Pipeline",
		MIMEType:    mimeJSON,
		Description: "Get the most recent CI/CD pipeline for a GitLab project. Returns pipeline ID, status (running/pending/success/failed/canceled), ref, SHA, source, and web URL.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerPipelineResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipeline/{pipeline_id}",
		Name:        "pipeline",
		Title:       "Pipeline Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific CI/CD pipeline by its numeric ID. Returns pipeline status, ref, SHA, source, and web URL.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconPipeline,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID, pipelineIDStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/pipeline/")
		if projectID == "" || pipelineIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		pipelineID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		p, _, err := client.GL().Pipelines.GetPipeline(projectID, pipelineID, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get pipeline", err)
		}
		out := pipelineToResourceOutput(p)
		return marshalResourceJSON(out)
	})
}

// registerPipelineJobsResource registers the
// "gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs" template
// resource that lists all jobs for a specific CI/CD pipeline.
func registerPipelineJobsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs",
		Name:        "pipeline_jobs",
		Title:       "Pipeline Jobs",
		MIMEType:    mimeJSON,
		Description: "List all jobs for a specific CI/CD pipeline including each job's name, stage, status, duration, failure reason (if failed), and web URL.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconJob,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := strings.TrimSuffix(req.Params.URI, "/jobs")
		projectID, pipelineIDStr := extractTwoParts(uri, uriProjectPrefix, "/pipeline/")
		if projectID == "" || pipelineIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		pipelineID, err := strconv.ParseInt(pipelineIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		jobs, _, err := client.GL().Jobs.ListPipelineJobs(projectID, pipelineID, &gl.ListJobsOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerProjectLabelsResource registers the
// "gitlab://project/{project_id}/labels" template resource that lists all
// labels defined in a GitLab project.
func registerProjectLabelsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/labels",
		Name:        "project_labels",
		Title:       "Project Labels",
		MIMEType:    mimeJSON,
		Description: "List all labels defined in a GitLab project. Returns each label's name, color, description, and counts of open issues and merge requests using the label.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconLabel,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/labels")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		labels, _, err := client.GL().Labels.ListLabels(projectID, &gl.ListLabelsOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerProjectMilestonesResource registers the
// "gitlab://project/{project_id}/milestones" template resource that lists
// all milestones in a GitLab project.
func registerProjectMilestonesResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/milestones",
		Name:        "project_milestones",
		Title:       "Project Milestones",
		MIMEType:    mimeJSON,
		Description: "List all milestones in a GitLab project. Returns each milestone's title, description, state (active/closed), due date, and web URL.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconMilestone,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/milestones")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		milestones, _, err := client.GL().Milestones.ListMilestones(projectID, &gl.ListMilestonesOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerMergeRequestResource registers the
// "gitlab://project/{project_id}/mr/{mr_iid}" template resource that
// returns details of a specific merge request by its project-scoped IID.
func registerMergeRequestResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{mr_iid}",
		Name:        "merge_request",
		Title:       "Merge Request Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific merge request by its IID (project-scoped ID). Returns title, state, source/target branches, author, merge status, and web URL.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerProjectBranchesResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/branches",
		Name:        "project_branches",
		Title:       "Project Branches",
		MIMEType:    mimeJSON,
		Description: "List all branches in a GitLab project. Returns each branch's name, protection status, merge status, default flag, and web URL.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconBranch,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/branches")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		branches, _, err := client.GL().Branches.ListBranches(projectID, &gl.ListBranchesOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerGroupResource registers the "gitlab://group/{group_id}" template
// resource that returns details for a specific GitLab group.
func registerGroupResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}",
		Name:        "group",
		Title:       "Group Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a specific GitLab group by numeric ID or URL-encoded path. Returns name, full path, description, visibility, and web URL.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconGroup,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerGroupMembersResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/members",
		Name:        "group_members",
		Title:       "Group Members",
		MIMEType:    mimeJSON,
		Description: "List all members of a GitLab group with their access levels (10=guest, 20=reporter, 30=developer, 40=maintainer, 50=owner). Includes inherited members.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconUser,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		groupID := extractMiddle(req.Params.URI, uriGroupPrefix, "/members")
		if groupID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		members, _, err := client.GL().Groups.ListAllGroupMembers(groupID, &gl.ListGroupMembersOptions{}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list group members", err)
		}
		out := make([]MemberResourceOutput, len(members))
		for i, m := range members {
			out[i] = MemberResourceOutput{
				ID:          m.ID,
				Username:    m.Username,
				Name:        m.Name,
				State:       m.State,
				AccessLevel: int(m.AccessLevel),
				WebURL:      m.WebURL,
			}
		}
		return marshalResourceJSON(out)
	})
}

// registerGroupProjectsResource registers the
// "gitlab://group/{group_id}/projects" template resource that lists all
// projects within a GitLab group.
func registerGroupProjectsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://group/{group_id}/projects",
		Name:        "group_projects",
		Title:       "Group Projects",
		MIMEType:    mimeJSON,
		Description: "List all projects within a GitLab group. Returns each project's ID, name, namespace path, visibility, web URL, description, and default branch.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconProject,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		groupID := extractMiddle(req.Params.URI, uriGroupPrefix, "/projects")
		if groupID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		projects, _, err := client.GL().Groups.ListGroupProjects(groupID, &gl.ListGroupProjectsOptions{}, gl.WithContext(ctx))
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
		return marshalResourceJSON(out)
	})
}

// registerProjectIssuesResource registers the
// "gitlab://project/{project_id}/issues" template resource that lists
// open issues for a GitLab project.
func registerProjectIssuesResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/issues",
		Name:        "project_issues",
		Title:       "Project Issues",
		MIMEType:    mimeJSON,
		Description: "List open issues for a GitLab project. Returns each issue's IID, title, state, labels, assignees, author, web URL, and creation date.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/issues")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		state := "opened"
		issues, _, err := client.GL().Issues.ListProjectIssues(projectID, &gl.ListProjectIssuesOptions{
			State: &state,
		}, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to list project issues", err)
		}
		out := make([]IssueResourceOutput, len(issues))
		for i, issue := range issues {
			out[i] = issueToResourceOutput(issue)
		}
		return marshalResourceJSON(out)
	})
}

// registerIssueResource registers the
// "gitlab://project/{project_id}/issue/{issue_iid}" template resource that
// returns details of a specific issue by its project-scoped IID.
func registerIssueResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/issue/{issue_iid}",
		Name:        "issue",
		Title:       "Issue Details",
		MIMEType:    mimeJSON,
		Description: "Get details of a specific issue by its IID (project-scoped ID). Returns title, state, labels, assignees, author, web URL, and creation date.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconIssue,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID, issueIIDStr := extractTwoParts(req.Params.URI, uriProjectPrefix, "/issue/")
		if projectID == "" || issueIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		issueIID, err := strconv.ParseInt(issueIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		issue, _, err := client.GL().Issues.GetIssue(projectID, issueIID, gl.WithContext(ctx))
		if err != nil {
			return nil, wrapErr("failed to get issue", err)
		}
		out := issueToResourceOutput(issue)
		return marshalResourceJSON(out)
	})
}

// registerProjectReleasesResource registers the
// "gitlab://project/{project_id}/releases" template resource that lists
// all releases for a GitLab project.
func registerProjectReleasesResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/releases",
		Name:        "project_releases",
		Title:       "Project Releases",
		MIMEType:    mimeJSON,
		Description: "List all releases for a GitLab project. Returns each release's tag name, name, description, author, and creation/release dates.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconRelease,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/releases")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		releases, _, err := client.GL().Releases.ListReleases(projectID, &gl.ListReleasesOptions{}, gl.WithContext(ctx))
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
				ro.CreatedAt = r.CreatedAt.Format(timeFormatISO)
			}
			if r.ReleasedAt != nil {
				ro.ReleasedAt = r.ReleasedAt.Format(timeFormatISO)
			}
			out[i] = ro
		}
		return marshalResourceJSON(out)
	})
}

// registerProjectTagsResource registers the
// "gitlab://project/{project_id}/tags" template resource that lists all
// repository tags for a GitLab project.
func registerProjectTagsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/tags",
		Name:        "project_tags",
		Title:       "Project Tags",
		MIMEType:    mimeJSON,
		Description: "List all repository tags for a GitLab project. Returns each tag's name, message, target commit SHA, protection status, and creation date.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconTag,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		projectID := extractMiddle(req.Params.URI, uriProjectPrefix, "/tags")
		if projectID == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		tags, _, err := client.GL().Tags.ListTags(projectID, &gl.ListTagsOptions{}, gl.WithContext(ctx))
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
				to.CreatedAt = tag.CreatedAt.Format(timeFormatISO)
			}
			out[i] = to
		}
		return marshalResourceJSON(out)
	})
}

// registerCommitResource registers the
// "gitlab://project/{project_id}/commit/{sha}" template resource that returns
// details for a single commit including message, author/committer, parents
// and stats.
func registerCommitResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/commit/{sha}",
		Name:        "commit",
		Title:       "Commit Details",
		MIMEType:    mimeJSON,
		Description: "Get details for a single commit by SHA. Returns short_id, title, message, author, committer, authored/committed dates, parent commits, web URL, and stats (additions/deletions).",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconCommit,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
			out.AuthoredDate = c.AuthoredDate.Format(timeFormatISO)
		}
		if c.CommittedDate != nil {
			out.CommittedDate = c.CommittedDate.Format(timeFormatISO)
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
func registerFileBlobResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/file/{ref}/{+path}",
		Name:        "file_blob",
		Title:       "Repository File",
		MIMEType:    mimeJSON,
		Description: "Get the contents of a repository file at a specific ref (branch, tag, or SHA). Path may include slashes. Files over 1 MiB return metadata only with truncated=true. Binary files return metadata with empty content.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconFile,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
func registerWikiResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/wiki/{slug}",
		Name:        "wiki_page",
		Title:       "Wiki Page",
		MIMEType:    mimeJSON,
		Description: "Get a wiki page by slug. Returns title, slug, format (markdown/rdoc/asciidoc/org), and raw content. Slugs are case-sensitive and use hyphens for spaces.",
		Annotations: toolutil.ContentDetail,
		Icons:       toolutil.IconWiki,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
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
// "gitlab://project/{project_id}/mr/{mr_iid}/notes" template resource that
// returns the flat list of notes (comments) for a merge request.
func registerMergeRequestNotesResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{mr_iid}/notes",
		Name:        "merge_request_notes",
		Title:       "Merge Request Notes",
		MIMEType:    mimeJSON,
		Description: "List notes (comments) on a merge request. Returns each note's id, author username, body, system flag, resolvable/resolved flags, and timestamps.",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := strings.TrimSuffix(req.Params.URI, "/notes")
		projectID, mrIIDStr := extractTwoParts(uri, uriProjectPrefix, "/mr/")
		if projectID == "" || mrIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mrIID, err := strconv.ParseInt(mrIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		notes, _, err := client.GL().Notes.ListMergeRequestNotes(projectID, mrIID, &gl.ListMergeRequestNotesOptions{}, gl.WithContext(ctx))
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
				no.CreatedAt = n.CreatedAt.Format(timeFormatISO)
			}
			if n.UpdatedAt != nil {
				no.UpdatedAt = n.UpdatedAt.Format(timeFormatISO)
			}
			out[i] = no
		}
		return marshalResourceJSON(out)
	})
}

// registerMergeRequestDiscussionsResource registers the
// "gitlab://project/{project_id}/mr/{mr_iid}/discussions" template resource
// that returns the discussion threads for a merge request, each containing
// one or more notes.
func registerMergeRequestDiscussionsResource(server *mcp.Server, client *gitlabclient.Client) {
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "gitlab://project/{project_id}/mr/{mr_iid}/discussions",
		Name:        "merge_request_discussions",
		Title:       "Merge Request Discussions",
		MIMEType:    mimeJSON,
		Description: "List discussion threads on a merge request. Each discussion has an id, individual_note flag, and an array of notes (id, author, body, system, resolved/resolvable, created_at).",
		Annotations: toolutil.ContentList,
		Icons:       toolutil.IconMR,
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		uri := strings.TrimSuffix(req.Params.URI, "/discussions")
		projectID, mrIIDStr := extractTwoParts(uri, uriProjectPrefix, "/mr/")
		if projectID == "" || mrIIDStr == "" {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		mrIID, err := strconv.ParseInt(mrIIDStr, 10, 64)
		if err != nil {
			return nil, mcp.ResourceNotFoundError(req.Params.URI)
		}
		discussions, _, err := client.GL().Discussions.ListMergeRequestDiscussions(projectID, mrIID, &gl.ListMergeRequestDiscussionsOptions{}, gl.WithContext(ctx))
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
					no.CreatedAt = n.CreatedAt.Format(timeFormatISO)
				}
				dout.Notes = append(dout.Notes, no)
			}
			out[i] = dout
		}
		return marshalResourceJSON(out)
	})
}

// extractFileBlobURI splits a "gitlab://project/{id}/file/{ref}/{path}" URI
// into its three components. The path component may contain slashes. Returns
// empty strings if the URI does not match the expected layout.
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
	slash := strings.Index(tail, "/")
	if slash <= 0 || slash == len(tail)-1 {
		return "", "", ""
	}
	ref = tail[:slash]
	filePath = tail[slash+1:]
	return projectID, ref, filePath
}

// decodeFileContent decodes the contents of a [gl.File] returned by the
// RepositoryFiles GitLab API. Base64 encoded payloads are decoded; binary
// content is detected via the file name and replaced with an empty string
// so JSON responses stay textual. Returns the decoded content and a
// human-readable category ("text" or "binary").
func decodeFileContent(f *gl.File) (string, string) {
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
	if toolutil.IsBinaryFile(f.FileName) {
		return "", "binary"
	}
	return string(decoded), "text"
}

// issueToResourceOutput converts a GitLab API [gl.Issue] to the MCP resource
// output format, extracting author username, assignee usernames, and formatting
// the creation timestamp.
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
		out.CreatedAt = issue.CreatedAt.Format(timeFormatISO)
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

// extractSuffix returns the portion of uri after the given prefix.
func extractSuffix(uri, prefix string) string {
	if !strings.HasPrefix(uri, prefix) {
		return ""
	}
	return strings.TrimPrefix(uri, prefix)
}

// extractMiddle returns the portion of uri between prefix and suffix.
func extractMiddle(uri, prefix, suffix string) string {
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return ""
	}
	return uri[len(prefix) : len(uri)-len(suffix)]
}

// extractTwoParts splits a URI into two dynamic segments around a separator.
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

// marshalResourceJSON marshals a value to JSON and wraps it as a ReadResourceResult.
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

// pipelineToResourceOutput converts a GitLab API [gl.Pipeline] to the MCP
// resource output format, mapping all relevant fields including status, ref,
// SHA, and source.
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
