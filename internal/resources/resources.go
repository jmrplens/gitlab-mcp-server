// Package resources registers MCP resources that expose read-only GitLab
// project data (metadata, branches, merge requests, pipelines) via stable URIs.
package resources

import (
	"context"
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
