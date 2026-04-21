// Package search implements GitLab search operations across multiple scopes:
// code (blobs), merge requests, issues, commits, milestones, notes, projects,
// snippet titles, users, and wiki blobs. Each handler supports global, group,
// and/or project-scoped search as available in the GitLab Search API.
package search

import (
	"context"
	"errors"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------.

// searchOpts builds a [gl.SearchOptions] from pagination and optional ref.
func searchOpts(page, perPage int, ref string) *gl.SearchOptions {
	opts := &gl.SearchOptions{}
	if ref != "" {
		opts.Ref = new(ref)
	}
	if page > 0 {
		opts.Page = int64(page)
	}
	if perPage > 0 {
		opts.PerPage = int64(perPage)
	}
	return opts
}

// wrapSearchErr enriches search errors with a 422-specific hint for query syntax
// errors, falling back to WrapErrWithMessage for all other cases.
func wrapSearchErr(op string, err error) error {
	if toolutil.IsHTTPStatus(err, 422) {
		return toolutil.WrapErrWithHint(op, err,
			"check the search query format — GitLab advanced search supports specific scopes and operators")
	}
	return toolutil.WrapErrWithMessage(op, err)
}

// ---------------------------------------------------------------------------
// Code (blobs)
// ---------------------------------------------------------------------------.

// CodeInput defines parameters for searching code blobs.
// When project_id is provided the search is scoped to that project;
// when group_id is provided it is scoped to that group;
// otherwise a global search is performed.
type CodeInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query text (param 'query' not 'search'),required"`
	Ref       string               `json:"ref,omitempty"        jsonschema:"Branch or tag name to search in (default: default branch)"`
	toolutil.PaginationInput
}

// BlobOutput represents a single code search result.
type BlobOutput struct {
	Basename  string `json:"basename"`
	Data      string `json:"data"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	Ref       string `json:"ref"`
	Startline int64  `json:"startline"`
	ProjectID int64  `json:"project_id"`
}

// CodeOutput holds a paginated list of code search results.
type CodeOutput struct {
	toolutil.HintableOutput
	Blobs      []BlobOutput              `json:"blobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Code searches for code (blobs) in GitLab. Scope priority:
// project_id > group_id > global.
func Code(ctx context.Context, client *gitlabclient.Client, input CodeInput) (CodeOutput, error) {
	if err := ctx.Err(); err != nil {
		return CodeOutput{}, err
	}
	if input.Query == "" {
		return CodeOutput{}, errors.New("searchCode: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, input.Ref)

	var (
		blobs []*gl.Blob
		resp  *gl.Response
		err   error
	)

	switch {
	case input.ProjectID != "":
		blobs, resp, err = client.GL().Search.BlobsByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		blobs, resp, err = client.GL().Search.BlobsByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		blobs, resp, err = client.GL().Search.Blobs(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return CodeOutput{}, wrapSearchErr("searchCode", err)
	}

	out := make([]BlobOutput, len(blobs))
	for i, b := range blobs {
		out[i] = BlobOutput{
			Basename:  b.Basename,
			Data:      b.Data,
			Path:      b.Path,
			Filename:  b.Filename,
			Ref:       b.Ref,
			Startline: b.Startline,
			ProjectID: b.ProjectID,
		}
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return CodeOutput{Blobs: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Merge Requests
// ---------------------------------------------------------------------------.

// MergeRequestsInput defines parameters for searching merge requests.
// Scope: project_id > group_id > global.
type MergeRequestsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query text (param 'query' not 'search'),required"`
	toolutil.PaginationInput
}

// MergeRequestsOutput holds a paginated list of merge request search results.
type MergeRequestsOutput struct {
	toolutil.HintableOutput
	MergeRequests []mergerequests.Output    `json:"merge_requests"`
	Pagination    toolutil.PaginationOutput `json:"pagination"`
}

// MergeRequests searches for merge requests in GitLab.
// Scope priority: project_id > group_id > global.
func MergeRequests(ctx context.Context, client *gitlabclient.Client, input MergeRequestsInput) (MergeRequestsOutput, error) {
	if err := ctx.Err(); err != nil {
		return MergeRequestsOutput{}, err
	}
	if input.Query == "" {
		return MergeRequestsOutput{}, errors.New("searchMergeRequests: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		mrs  []*gl.MergeRequest
		resp *gl.Response
		err  error
	)

	switch {
	case input.ProjectID != "":
		mrs, resp, err = client.GL().Search.MergeRequestsByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		mrs, resp, err = client.GL().Search.MergeRequestsByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		mrs, resp, err = client.GL().Search.MergeRequests(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return MergeRequestsOutput{}, wrapSearchErr("searchMergeRequests", err)
	}

	out := make([]mergerequests.Output, len(mrs))
	for i, mr := range mrs {
		out[i] = mergerequests.ToOutput(mr)
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return MergeRequestsOutput{MergeRequests: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Issues
// ---------------------------------------------------------------------------.

// IssuesInput defines parameters for searching issues.
// Scope: project_id > group_id > global.
type IssuesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query text (param 'query' not 'search'),required"`
	toolutil.PaginationInput
}

// IssuesOutput holds a paginated list of issue search results.
type IssuesOutput struct {
	toolutil.HintableOutput
	Issues     []issues.Output           `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Issues searches for issues in GitLab.
// Scope priority: project_id > group_id > global.
func Issues(ctx context.Context, client *gitlabclient.Client, input IssuesInput) (IssuesOutput, error) {
	if err := ctx.Err(); err != nil {
		return IssuesOutput{}, err
	}
	if input.Query == "" {
		return IssuesOutput{}, errors.New("searchIssues: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		foundIssues []*gl.Issue
		resp        *gl.Response
		err         error
	)

	switch {
	case input.ProjectID != "":
		foundIssues, resp, err = client.GL().Search.IssuesByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		foundIssues, resp, err = client.GL().Search.IssuesByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		foundIssues, resp, err = client.GL().Search.Issues(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return IssuesOutput{}, wrapSearchErr("searchIssues", err)
	}

	out := make([]issues.Output, len(foundIssues))
	for i, issue := range foundIssues {
		out[i] = issues.ToOutput(issue)
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return IssuesOutput{Issues: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Commits
// ---------------------------------------------------------------------------.

// CommitsInput defines parameters for searching commits.
// Scope: project_id > group_id > global.
type CommitsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// CommitsOutput holds a paginated list of commit search results.
type CommitsOutput struct {
	toolutil.HintableOutput
	Commits    []commits.Output          `json:"commits"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Commits searches for commits in GitLab.
// Scope priority: project_id > group_id > global.
func Commits(ctx context.Context, client *gitlabclient.Client, input CommitsInput) (CommitsOutput, error) {
	if err := ctx.Err(); err != nil {
		return CommitsOutput{}, err
	}
	if input.Query == "" {
		return CommitsOutput{}, errors.New("searchCommits: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		commitResults []*gl.Commit
		resp          *gl.Response
		err           error
	)

	switch {
	case input.ProjectID != "":
		commitResults, resp, err = client.GL().Search.CommitsByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		commitResults, resp, err = client.GL().Search.CommitsByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		commitResults, resp, err = client.GL().Search.Commits(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return CommitsOutput{}, wrapSearchErr("searchCommits", err)
	}

	out := make([]commits.Output, len(commitResults))
	for i, c := range commitResults {
		out[i] = commits.ToOutput(c)
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return CommitsOutput{Commits: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Milestones
// ---------------------------------------------------------------------------.

// MilestonesInput defines parameters for searching milestones.
// Scope: project_id > group_id > global.
type MilestonesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// MilestonesOutput holds a paginated list of milestone search results.
type MilestonesOutput struct {
	toolutil.HintableOutput
	Milestones []milestones.Output       `json:"milestones"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Milestones searches for milestones in GitLab.
// Scope priority: project_id > group_id > global.
func Milestones(ctx context.Context, client *gitlabclient.Client, input MilestonesInput) (MilestonesOutput, error) {
	if err := ctx.Err(); err != nil {
		return MilestonesOutput{}, err
	}
	if input.Query == "" {
		return MilestonesOutput{}, errors.New("searchMilestones: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		msList []*gl.Milestone
		resp   *gl.Response
		err    error
	)

	switch {
	case input.ProjectID != "":
		msList, resp, err = client.GL().Search.MilestonesByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		msList, resp, err = client.GL().Search.MilestonesByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		msList, resp, err = client.GL().Search.Milestones(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return MilestonesOutput{}, wrapSearchErr("searchMilestones", err)
	}

	out := make([]milestones.Output, len(msList))
	for i, m := range msList {
		out[i] = milestones.ToOutput(m)
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return MilestonesOutput{Milestones: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Notes (project-scoped only)
// ---------------------------------------------------------------------------.

// NotesInput defines parameters for searching notes within a project.
type NotesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query     string               `json:"query"      jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// NoteOutput represents a single note search result.
type NoteOutput struct {
	ID           int64  `json:"id"`
	Body         string `json:"body"`
	Author       string `json:"author"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	NoteableType string `json:"notable_type"`
	NoteableID   int64  `json:"notable_id"`
	NoteableIID  int64  `json:"notable_iid,omitempty"`
	System       bool   `json:"system"`
}

// NotesOutput holds a paginated list of note search results.
type NotesOutput struct {
	toolutil.HintableOutput
	Notes      []NoteOutput              `json:"notes"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Notes searches for notes within a GitLab project.
func Notes(ctx context.Context, client *gitlabclient.Client, input NotesInput) (NotesOutput, error) {
	if err := ctx.Err(); err != nil {
		return NotesOutput{}, err
	}
	if input.ProjectID == "" {
		return NotesOutput{}, errors.New("searchNotes: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	if input.Query == "" {
		return NotesOutput{}, errors.New("searchNotes: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	notes, resp, err := client.GL().Search.NotesByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	if err != nil {
		return NotesOutput{}, wrapSearchErr("searchNotes", err)
	}

	out := make([]NoteOutput, len(notes))
	for i, n := range notes {
		out[i] = NoteOutput{
			ID:           n.ID,
			Body:         n.Body,
			NoteableType: n.NoteableType,
			NoteableID:   n.NoteableID,
			NoteableIID:  n.NoteableIID,
			System:       n.System,
		}
		if n.Author.Username != "" {
			out[i].Author = n.Author.Username
		}
		if n.CreatedAt != nil {
			out[i].CreatedAt = n.CreatedAt.Format(time.RFC3339)
		}
		if n.UpdatedAt != nil {
			out[i].UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
		}
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return NotesOutput{Notes: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Projects
// ---------------------------------------------------------------------------.

// ProjectsInput defines parameters for searching projects.
// Scope: group_id (optional) — omit for global search.
type ProjectsInput struct {
	GroupID toolutil.StringOrInt `json:"group_id,omitempty" jsonschema:"Group ID or URL-encoded path (optional — omit for global search)"`
	Query   string               `json:"query"              jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// ProjectsOutput holds a paginated list of project search results.
type ProjectsOutput struct {
	toolutil.HintableOutput
	Projects   []projects.Output         `json:"projects"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Projects searches for projects in GitLab.
// Scope: group_id > global.
func Projects(ctx context.Context, client *gitlabclient.Client, input ProjectsInput) (ProjectsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProjectsOutput{}, err
	}
	if input.Query == "" {
		return ProjectsOutput{}, errors.New("searchProjects: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		projs []*gl.Project
		resp  *gl.Response
		err   error
	)

	if input.GroupID != "" {
		projs, resp, err = client.GL().Search.ProjectsByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	} else {
		projs, resp, err = client.GL().Search.Projects(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return ProjectsOutput{}, wrapSearchErr("searchProjects", err)
	}

	out := make([]projects.Output, len(projs))
	for i, p := range projs {
		out[i] = projects.ToOutput(p)
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return ProjectsOutput{Projects: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Snippet Titles (global only)
// ---------------------------------------------------------------------------.

// SnippetsInput defines parameters for searching snippet titles.
type SnippetsInput struct {
	Query string `json:"query" jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// SnippetOutput represents a single snippet search result.
type SnippetOutput struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	FileName    string `json:"file_name"`
	Description string `json:"description"`
	Visibility  string `json:"visibility"`
	Author      string `json:"author"`
	WebURL      string `json:"web_url"`
	RawURL      string `json:"raw_url"`
	ProjectID   int64  `json:"project_id,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SnippetsOutput holds a paginated list of snippet search results.
type SnippetsOutput struct {
	toolutil.HintableOutput
	Snippets   []SnippetOutput           `json:"snippets"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Snippets searches for snippet titles globally in GitLab.
func Snippets(ctx context.Context, client *gitlabclient.Client, input SnippetsInput) (SnippetsOutput, error) {
	if err := ctx.Err(); err != nil {
		return SnippetsOutput{}, err
	}
	if input.Query == "" {
		return SnippetsOutput{}, errors.New("searchSnippets: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	snippets, resp, err := client.GL().Search.SnippetTitles(input.Query, opts, gl.WithContext(ctx))
	if err != nil {
		return SnippetsOutput{}, wrapSearchErr("searchSnippets", err)
	}

	out := make([]SnippetOutput, len(snippets))
	for i, s := range snippets {
		out[i] = SnippetOutput{
			ID:          s.ID,
			Title:       s.Title,
			FileName:    s.FileName,
			Description: s.Description,
			Visibility:  s.Visibility,
			WebURL:      s.WebURL,
			RawURL:      s.RawURL,
			ProjectID:   s.ProjectID,
		}
		if s.Author.Username != "" {
			out[i].Author = s.Author.Username
		}
		if s.CreatedAt != nil {
			out[i].CreatedAt = s.CreatedAt.Format(time.RFC3339)
		}
		if s.UpdatedAt != nil {
			out[i].UpdatedAt = s.UpdatedAt.Format(time.RFC3339)
		}
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return SnippetsOutput{Snippets: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------.

// UsersInput defines parameters for searching users.
// Scope: project_id > group_id > global.
type UsersInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// UserOutput represents a single user search result (simplified).
type UserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// UsersOutput holds a paginated list of user search results.
type UsersOutput struct {
	toolutil.HintableOutput
	Users      []UserOutput              `json:"users"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Users searches for users in GitLab.
// Scope priority: project_id > group_id > global.
func Users(ctx context.Context, client *gitlabclient.Client, input UsersInput) (UsersOutput, error) {
	if err := ctx.Err(); err != nil {
		return UsersOutput{}, err
	}
	if input.Query == "" {
		return UsersOutput{}, errors.New("searchUsers: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		users []*gl.User
		resp  *gl.Response
		err   error
	)

	switch {
	case input.ProjectID != "":
		users, resp, err = client.GL().Search.UsersByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		users, resp, err = client.GL().Search.UsersByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		users, resp, err = client.GL().Search.Users(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return UsersOutput{}, wrapSearchErr("searchUsers", err)
	}

	out := make([]UserOutput, len(users))
	for i, u := range users {
		out[i] = UserOutput{
			ID:        u.ID,
			Username:  u.Username,
			Name:      u.Name,
			State:     u.State,
			AvatarURL: u.AvatarURL,
			WebURL:    u.WebURL,
		}
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return UsersOutput{Users: out, Pagination: pag}, nil
}

// ---------------------------------------------------------------------------
// Wiki Blobs
// ---------------------------------------------------------------------------.

// WikiInput defines parameters for searching wiki blobs.
// Scope: project_id > group_id > global.
type WikiInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID   toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query     string               `json:"query"                jsonschema:"Search query string,required"`
	toolutil.PaginationInput
}

// WikiBlobOutput represents a single wiki blob search result.
type WikiBlobOutput struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Format  string `json:"format"`
}

// WikiOutput holds a paginated list of wiki blob search results.
type WikiOutput struct {
	toolutil.HintableOutput
	WikiBlobs  []WikiBlobOutput          `json:"wiki_blobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Wiki searches for wiki blobs in GitLab.
// Scope priority: project_id > group_id > global.
func Wiki(ctx context.Context, client *gitlabclient.Client, input WikiInput) (WikiOutput, error) {
	if err := ctx.Err(); err != nil {
		return WikiOutput{}, err
	}
	if input.Query == "" {
		return WikiOutput{}, errors.New("searchWiki: query is required")
	}

	opts := searchOpts(input.Page, input.PerPage, "")

	var (
		wikis []*gl.Wiki
		resp  *gl.Response
		err   error
	)

	switch {
	case input.ProjectID != "":
		wikis, resp, err = client.GL().Search.WikiBlobsByProject(string(input.ProjectID), input.Query, opts, gl.WithContext(ctx))
	case input.GroupID != "":
		wikis, resp, err = client.GL().Search.WikiBlobsByGroup(string(input.GroupID), input.Query, opts, gl.WithContext(ctx))
	default:
		wikis, resp, err = client.GL().Search.WikiBlobs(input.Query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return WikiOutput{}, wrapSearchErr("searchWiki", err)
	}

	out := make([]WikiBlobOutput, len(wikis))
	for i, w := range wikis {
		out[i] = WikiBlobOutput{
			Slug:    w.Slug,
			Title:   w.Title,
			Content: w.Content,
			Format:  string(w.Format),
		}
	}
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, len(out))
	return WikiOutput{WikiBlobs: out, Pagination: pag}, nil
}
