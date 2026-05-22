package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------.

// TypeInput defines the optional GitLab search backend selector.
type TypeInput struct {
	SearchType string `json:"search_type,omitempty" jsonschema:"Search backend to request: basic, advanced, or zoekt"`
}

var allowedSearchTypes = []string{
	string(gl.BasicSearch),
	string(gl.AdvancedSearch),
	string(gl.ZoektSearch),
}

func validateSearchType(searchType string) error {
	if searchType == "" {
		return nil
	}
	if slices.Contains(allowedSearchTypes, searchType) {
		return nil
	}
	return fmt.Errorf("invalid search_type %q: use one of basic, advanced, or zoekt", searchType)
}

// searchOpts builds a [gl.SearchOptions] from pagination, ref, and search type.
func searchOpts(page, perPage int, ref, searchType string) (*gl.SearchOptions, error) {
	if err := validateSearchType(searchType); err != nil {
		return nil, err
	}
	opts := &gl.SearchOptions{}
	if ref != "" {
		opts.Ref = &ref
	}
	if searchType != "" {
		typ := gl.SearchType(searchType)
		opts.SearchType = &typ
	}
	if page > 0 {
		opts.Page = int64(page)
	}
	if perPage > 0 {
		opts.PerPage = int64(perPage)
	}
	return opts, nil
}

// wrapSearchErr enriches recoverable GitLab Search API errors with hints that
// help callers adjust query, scope, or backend selection.
func wrapSearchErr(op string, err error) error {
	if toolutil.IsHTTPStatus(err, 400) {
		return toolutil.WrapErrWithHint(op, err,
			"check query and scope parameters; if search_type was supplied, verify that the requested backend is enabled on this GitLab instance or retry without search_type")
	}
	if toolutil.IsHTTPStatus(err, 422) {
		return toolutil.WrapErrWithHint(op, err,
			"check the search query format — GitLab advanced search supports specific scopes and operators; retry without search_type if the selected backend does not support the query")
	}
	return toolutil.WrapErrWithMessage(op, err)
}

type scopedSearchFunc[T any] func(any, string, *gl.SearchOptions, ...gl.RequestOptionFunc) ([]T, *gl.Response, error)

type globalSearchFunc[T any] func(string, *gl.SearchOptions, ...gl.RequestOptionFunc) ([]T, *gl.Response, error)

type scopedSearchArgs[T any] struct {
	query         string
	projectID     toolutil.StringOrInt
	groupID       toolutil.StringOrInt
	page          int
	perPage       int
	searchType    string
	operation     string
	projectSearch scopedSearchFunc[T]
	groupSearch   scopedSearchFunc[T]
	globalSearch  globalSearchFunc[T]
}

func runScopedSearch[T any](ctx context.Context, args scopedSearchArgs[T]) ([]T, *gl.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if args.query == "" {
		return nil, nil, fmt.Errorf("%s: query is required", args.operation)
	}

	opts, err := searchOpts(args.page, args.perPage, "", args.searchType)
	if err != nil {
		return nil, nil, err
	}

	var (
		items []T
		resp  *gl.Response
	)
	switch {
	case args.projectID != "":
		items, resp, err = args.projectSearch(string(args.projectID), args.query, opts, gl.WithContext(ctx))
	case args.groupID != "":
		items, resp, err = args.groupSearch(string(args.groupID), args.query, opts, gl.WithContext(ctx))
	default:
		items, resp, err = args.globalSearch(args.query, opts, gl.WithContext(ctx))
	}
	if err != nil {
		return nil, nil, wrapSearchErr(args.operation, err)
	}
	return items, resp, nil
}

func convertSearchResults[T, O any](items []T, convert func(T) O) []O {
	out := make([]O, len(items))
	for i, item := range items {
		out[i] = convert(item)
	}
	return out
}

func searchPagination(resp *gl.Response, itemCount int) toolutil.PaginationOutput {
	pag := toolutil.PaginationFromResponse(resp)
	toolutil.AdjustPagination(&pag, itemCount)
	return pag
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
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, input.Ref, input.SearchType)
	if err != nil {
		return CodeOutput{}, err
	}

	var (
		blobs []*gl.Blob
		resp  *gl.Response
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
	TypeInput
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
	searchClient := client.GL().Search
	mrs, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.MergeRequest]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchMergeRequests", projectSearch: searchClient.MergeRequestsByProject,
		groupSearch: searchClient.MergeRequestsByGroup, globalSearch: searchClient.MergeRequests,
	})
	if err != nil {
		return MergeRequestsOutput{}, err
	}
	out := convertSearchResults(mrs, mergerequests.ToOutput)
	return MergeRequestsOutput{MergeRequests: out, Pagination: searchPagination(resp, len(out))}, nil
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
	TypeInput
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
	searchClient := client.GL().Search
	foundIssues, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Issue]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchIssues", projectSearch: searchClient.IssuesByProject,
		groupSearch: searchClient.IssuesByGroup, globalSearch: searchClient.Issues,
	})
	if err != nil {
		return IssuesOutput{}, err
	}
	out := convertSearchResults(foundIssues, issues.ToOutput)
	return IssuesOutput{Issues: out, Pagination: searchPagination(resp, len(out))}, nil
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
	TypeInput
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
	searchClient := client.GL().Search
	commitResults, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Commit]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchCommits", projectSearch: searchClient.CommitsByProject,
		groupSearch: searchClient.CommitsByGroup, globalSearch: searchClient.Commits,
	})
	if err != nil {
		return CommitsOutput{}, err
	}
	out := convertSearchResults(commitResults, commits.ToOutput)
	return CommitsOutput{Commits: out, Pagination: searchPagination(resp, len(out))}, nil
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
	TypeInput
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
	searchClient := client.GL().Search
	msList, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Milestone]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchMilestones", projectSearch: searchClient.MilestonesByProject,
		groupSearch: searchClient.MilestonesByGroup, globalSearch: searchClient.Milestones,
	})
	if err != nil {
		return MilestonesOutput{}, err
	}
	out := convertSearchResults(msList, milestones.ToOutput)
	return MilestonesOutput{Milestones: out, Pagination: searchPagination(resp, len(out))}, nil
}

// ---------------------------------------------------------------------------
// Notes (project-scoped only)
// ---------------------------------------------------------------------------.

// NotesInput defines parameters for searching notes within a project.
type NotesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Query     string               `json:"query"      jsonschema:"Search query string,required"`
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, "", input.SearchType)
	if err != nil {
		return NotesOutput{}, err
	}

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
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, "", input.SearchType)
	if err != nil {
		return ProjectsOutput{}, err
	}

	var (
		projs []*gl.Project
		resp  *gl.Response
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
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, "", input.SearchType)
	if err != nil {
		return SnippetsOutput{}, err
	}

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
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, "", input.SearchType)
	if err != nil {
		return UsersOutput{}, err
	}

	var (
		users []*gl.User
		resp  *gl.Response
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
	TypeInput
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

	opts, err := searchOpts(input.Page, input.PerPage, "", input.SearchType)
	if err != nil {
		return WikiOutput{}, err
	}

	var (
		wikis []*gl.Wiki
		resp  *gl.Response
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
