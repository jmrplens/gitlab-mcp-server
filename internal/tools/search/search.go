// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/mergerequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/milestones"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projects"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	// A page or size of zero or less sends nothing, since the options omit a
	// zero and GitLab then applies its own default.
	opts.Page = int64(max(page, 0))
	opts.PerPage = int64(max(perPage, 0))
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
			"check the search query format. GitLab advanced search supports specific scopes and operators; retry without search_type if the selected backend does not support the query")
	}
	return toolutil.WrapErrWithMessage(op, err)
}

type scopedSearchFunc[T any] func(any, string, *gl.SearchOptions, ...gl.RequestOptionFunc) ([]T, *gl.Response, error)

type globalSearchFunc[T any] func(string, *gl.SearchOptions, ...gl.RequestOptionFunc) ([]T, *gl.Response, error)

type scopedSearchArgs[T any] struct {
	query         string
	projectID     toolutil.StringOrInt
	groupID       toolutil.StringOrInt
	ref           string
	page          int
	perPage       int
	searchType    string
	operation     string
	projectSearch scopedSearchFunc[T]
	groupSearch   scopedSearchFunc[T]
	globalSearch  globalSearchFunc[T]
	// options are request options sent after the context, for parameters
	// client-go's SearchOptions does not model.
	options []gl.RequestOptionFunc
	// refusalHint is the hint for a 400 the search can explain from what it
	// sent, and the empty string for one it cannot, which keeps the hint
	// every search shares. Nil for a search that explains none.
	refusalHint func(error) string
	// capturesPage marks a caller that reads the page from the captured
	// response (ADR-0021) rather than from what client-go decoded. client-go
	// failing to decode an answer GitLab gave ([commits.MisreadByClientGo])
	// is then not the search's failure: the error is dropped and the response
	// kept, so the page's headers still reach the output.
	capturesPage bool
}

func runScopedSearch[T any](ctx context.Context, args scopedSearchArgs[T]) ([]T, *gl.Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if args.query == "" {
		return nil, nil, fmt.Errorf("%s: query is required", args.operation)
	}

	opts, err := searchOpts(args.page, args.perPage, args.ref, args.searchType)
	if err != nil {
		return nil, nil, err
	}

	items, resp, err := args.search(ctx, opts)
	if err != nil && (!args.capturesPage || !commits.MisreadByClientGo(err)) {
		return nil, nil, args.wrap(err)
	}
	return items, resp, nil
}

// wrap is the error a failed search returns: a 400 the search's own
// refusalHint explains with that hint, and anything else with the hints
// every search shares.
func (args scopedSearchArgs[T]) wrap(err error) error {
	if args.refusalHint != nil && toolutil.IsHTTPStatus(err, 400) {
		if hint := args.refusalHint(err); hint != "" {
			return toolutil.WrapErrWithHint(args.operation, err, hint)
		}
	}
	return wrapSearchErr(args.operation, err)
}

// search sends the query to the narrowest scope the caller named: the
// project, else the group, else the whole instance.
func (args scopedSearchArgs[T]) search(ctx context.Context, opts *gl.SearchOptions) ([]T, *gl.Response, error) {
	options := append([]gl.RequestOptionFunc{gl.WithContext(ctx)}, args.options...)
	if args.projectID != "" {
		return args.projectSearch(string(args.projectID), args.query, opts, options...)
	}
	if args.groupID != "" {
		return args.groupSearch(string(args.groupID), args.query, opts, options...)
	}
	return args.globalSearch(args.query, opts, options...)
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
	ProjectID       toolutil.StringOrInt `json:"project_id,omitempty" jsonschema:"Project ID or URL-encoded path (optional)"`
	GroupID         toolutil.StringOrInt `json:"group_id,omitempty"   jsonschema:"Group ID or URL-encoded path (optional)"`
	Query           string               `json:"query"                jsonschema:"Search query text (param 'query' not 'search'),required"`
	Ref             string               `json:"ref,omitempty"        jsonschema:"Branch or tag name to search in (default: default branch)"`
	NumContextLines *int64               `json:"num_context_lines,omitempty" jsonschema:"Lines of context GitLab sends around a code match, up to this many on each side, 0 to 20. Honored by advanced and exact code search (search_type advanced or zoekt), which send 2 and 1 by default, though exact search only from GitLab 19.5. GitLab Enterprise 18.11 and later refuses it on basic search, which a project search of a ref other than the default branch always runs, unless the token carries the mcp scope. Earlier releases and Community Edition ignore it"`
	Regex           *bool                `json:"regex,omitempty"             jsonschema:"Search the query as a regular expression (true) or as literal text (false). Honored by exact code search alone (search_type zoekt), which reads the query as a regular expression when this is left out. GitLab Enterprise 18.9 and later refuses it on any other backend, false included, and so on a project search of a ref other than the default branch, which always runs basic search, unless the token carries the mcp scope. Earlier releases and Community Edition ignore it"`
	TypeInput
	toolutil.PaginationInput
}

// BlobOutput represents a single code search result, which GitLab renders as
// API::Entities::Blob.
//
// ID tells the two kinds of basic-search result apart. Basic search sets the
// blob id only on a file that matched by its path (Gitlab::FileFinder's
// find_by_path, whose Gitlab::Search::FoundBlob is filled by fetch_blob), and
// answers such a file with its first 1024 bytes and start line 1; a match on
// the file's content is parsed from git grep output with no id, and GitLab
// answers null. A path match whose blob fetch_blob could not read answers
// null for the id, the snippet and the start line alike, and so reads as a
// content match with nothing in it. Advanced and exact search build every
// result without an id, so there a missing id says nothing about why the file
// matched. Exact search tells a match on a file's name apart another way: it
// sends the file's path as the snippet, with start line 1 (0 on GitLab 19.4),
// because the API reads the line zoekt gives such a match and not the mark
// saying the line is the file's name.
type BlobOutput struct {
	Basename  string `json:"basename"`
	Data      string `json:"data"       jsonschema:"The snippet GitLab sent: the lines around a match, or for a file that matched by its path the start of the file on basic search and the file's path itself on exact search"`
	Path      string `json:"path"`
	Filename  string `json:"filename"`
	ID        string `json:"id,omitempty" jsonschema:"Git blob ID of a file that matched the query by its path, set by basic search only. Such a result carries the start of the file and startline 1 rather than the lines around a match. Absent for a match on the file's content, for a path match whose file GitLab could not read (which then carries no snippet either), and on every result of advanced and exact search"`
	Ref       string `json:"ref"`
	Startline int64  `json:"startline"  jsonschema:"Line the snippet starts on. GitLab sends no line number for the match itself. Exact search answers 1 for a file that matched by its name, and on GitLab 19.4 answers 0 for that and for a snippet starting on the first line. 0 also stands for the null basic search answers for a path match whose file it could not read"`
	ProjectID int64  `json:"project_id"`
}

// CodeOutput holds a paginated list of code search results.
type CodeOutput struct {
	toolutil.HintableOutput
	Blobs      []BlobOutput              `json:"blobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`

	// snippets is how GitLab cut the snippets of this page: the backend that
	// answered and the context the request asked for, which together say
	// where in each snippet the match is. The Markdown reads it to show the
	// lines around the match. It is unexported so that nothing about the
	// published shape moves: GitLab sends the backend in a header and no
	// entity field, and the context is the caller's own input.
	snippets snippetContext
}

// Code searches for code (blobs) in GitLab. Scope priority:
// project_id > group_id > global.
func Code(ctx context.Context, client *gitlabclient.Client, input CodeInput) (CodeOutput, error) {
	if err := validateCodeSearchParams(input.NumContextLines); err != nil {
		return CodeOutput{}, err
	}
	searchClient := client.GL().Search
	//gitlab:request alternatives: the search goes to exactly one of the project, group and instance routes, the narrowest scope the caller named, and refusalHint, a function in the same literal, sends nothing and only explains a refusal
	blobs, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Blob]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, ref: input.Ref, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchCode", projectSearch: searchClient.BlobsByProject,
		groupSearch: searchClient.BlobsByGroup, globalSearch: searchClient.Blobs,
		options:     []gl.RequestOptionFunc{codeSearchParams(input.NumContextLines, input.Regex)},
		refusalHint: newCodeRefusal(input).hint,
	})
	if err != nil {
		return CodeOutput{}, err
	}

	out := make([]BlobOutput, len(blobs))
	for i, b := range blobs {
		out[i] = BlobOutput{
			Basename:  b.Basename,
			Data:      b.Data,
			Path:      b.Path,
			Filename:  b.Filename,
			ID:        b.ID,
			Ref:       b.Ref,
			Startline: b.Startline,
			ProjectID: b.ProjectID,
		}
	}
	return CodeOutput{
		Blobs: out, Pagination: searchPagination(resp, len(out)),
		snippets: snippetContext{backend: searchBackend(resp), contextLines: input.NumContextLines},
	}, nil
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
	// A search result is a whole merge request, so it carries the keys no
	// client-go merge request struct models and the shared output type
	// publishes; without the capture they would be schema fields this handler
	// alone never fills (ADR-0021).
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	mrs, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.MergeRequest]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchMergeRequests", projectSearch: searchClient.MergeRequestsByProject,
		groupSearch: searchClient.MergeRequestsByGroup, globalSearch: searchClient.MergeRequests,
	})
	if err != nil {
		return MergeRequestsOutput{}, err
	}
	extras, err := toolutil.CapturedMergeRequests(captured, len(mrs))
	if err != nil {
		return MergeRequestsOutput{}, toolutil.WrapErr("searchMergeRequests", err)
	}
	out := make([]mergerequests.Output, len(mrs))
	for i, mr := range mrs {
		out[i] = mergerequests.ToOutput(mr, extras[i])
	}
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
//
// Search renders API::Entities::IssueBasic (lib/api/search.rb), so the rows
// are the basic issue and not the full one: what only the issues API adds is
// not sent here.
type IssuesOutput struct {
	toolutil.HintableOutput
	Issues     []issues.BasicOutput      `json:"issues"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Issues searches for issues in GitLab.
// Scope priority: project_id > group_id > global.
func Issues(ctx context.Context, client *gitlabclient.Client, input IssuesInput) (IssuesOutput, error) {
	searchClient := client.GL().Search
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	foundIssues, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Issue]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchIssues", projectSearch: searchClient.IssuesByProject,
		groupSearch: searchClient.IssuesByGroup, globalSearch: searchClient.Issues,
	})
	if err != nil {
		return IssuesOutput{}, err
	}
	out, err := issues.ToBasicOutputs(foundIssues, captured)
	if err != nil {
		return IssuesOutput{}, toolutil.WrapErr("searchIssues", err)
	}
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
	Ref       string               `json:"ref,omitempty"        jsonschema:"Branch or tag whose commits a project search reads (default: the default branch). Group and instance searches do not take it"`
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
//
// The page is read from the captured response (ADR-0021), as
// [commits.List] reads its own: a commit whose trailers GitLab parsed carries
// extended_trailers as lists, which client-go's Commit cannot decode, and
// client-go then returns no page at all. Its failure to read an answer GitLab
// gave is passed over ([commits.MisreadByClientGo]) and the capture decoded
// instead; an answer the capture cannot hold either is reported.
func Commits(ctx context.Context, client *gitlabclient.Client, input CommitsInput) (CommitsOutput, error) {
	searchClient := client.GL().Search
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Commit]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, ref: input.Ref, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchCommits", projectSearch: searchClient.CommitsByProject,
		groupSearch: searchClient.CommitsByGroup, globalSearch: searchClient.Commits,
		capturesPage: true,
	})
	if err != nil {
		return CommitsOutput{}, err
	}
	out, err := commits.CapturedOutputs(captured)
	if err != nil {
		return CommitsOutput{}, toolutil.WrapErr("searchCommits", err)
	}
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
	NoteableType string `json:"noteable_type"`
	NoteableID   int64  `json:"noteable_id"`
	NoteableIID  int64  `json:"noteable_iid,omitempty"`
	System       bool   `json:"system"`
	Internal     bool   `json:"internal" jsonschema:"Whether the note is internal, which GitLab shows only to project members allowed to read internal notes"`
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
		return NotesOutput{}, errors.New("searchNotes: project_id is required. Use project.list to find the ID first, then pass it as project_id")
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
			Internal:     n.Internal,
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
	GroupID toolutil.StringOrInt `json:"group_id,omitempty" jsonschema:"Group ID or URL-encoded path. Omit for a global search"`
	Query   string               `json:"query"              jsonschema:"Search query string,required"`
	TypeInput
	toolutil.PaginationInput
}

// ProjectsOutput holds a paginated list of project search results.
//
// Search renders API::Entities::BasicProjectDetails (lib/api/search.rb), so
// the rows are the basic project and not the full one: none of what only the
// project entity adds is sent here.
type ProjectsOutput struct {
	toolutil.HintableOutput
	Projects   []projects.BasicOutput    `json:"projects"`
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

	out := make([]projects.BasicOutput, len(projs))
	for i, p := range projs {
		out[i] = projects.ToBasicOutput(p)
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
	searchClient := client.GL().Search
	users, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.User]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchUsers", projectSearch: searchClient.UsersByProject,
		groupSearch: searchClient.UsersByGroup, globalSearch: searchClient.Users,
	})
	if err != nil {
		return UsersOutput{}, err
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
	return UsersOutput{Users: out, Pagination: searchPagination(resp, len(out))}, nil
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
	Ref       string               `json:"ref,omitempty"        jsonschema:"Branch or tag of the wiki repository a project search reads (default: the default branch). Group and instance searches do not take it"`
	TypeInput
	toolutil.PaginationInput
}

// WikiBlobOutput represents a single wiki search result. GitLab answers the
// wiki_blobs scope with API::Entities::Blob, the entity code search answers
// with (lib/api/search.rb, SCOPE_ENTITY), each result a page's file in the
// wiki repository with the lines that matched, and adds group_id for a wiki
// page (lib/api/entities/blob.rb). It used to publish a wiki page's slug,
// title, content and format, which no search answers with, and so every
// field of every result was empty: client-go decodes this scope into its
// Wiki struct, so the rows are read from the captured response instead
// (ADR-0021).
type WikiBlobOutput struct {
	Basename  string `json:"basename"`
	Data      string `json:"data"                 jsonschema:"The snippet GitLab sent: the lines around a match, or the start of the page for a page whose file matched by its path"`
	Path      string `json:"path"                 jsonschema:"Path of the page's file in the wiki repository. The page's slug is this path without its extension"`
	Filename  string `json:"filename"`
	ID        string `json:"id,omitempty"         jsonschema:"Git blob ID of a page whose file matched the query by its path, set by basic search only. Such a result carries the start of the page and startline 1 rather than the lines around a match. Absent for a match on the page's content, for a path match whose file GitLab could not read (which then carries no snippet either), and on every result of advanced search"`
	Ref       string `json:"ref"`
	Startline int64  `json:"startline"            jsonschema:"Line the snippet starts on. GitLab sends no line number for the match itself. 0 stands for the null basic search answers for a path match whose file it could not read"`
	ProjectID int64  `json:"project_id,omitempty" jsonschema:"Project whose wiki holds the page. Absent for a page of a group wiki"`
	GroupID   int64  `json:"group_id,omitempty"   jsonschema:"Group of the wiki, set by advanced search only: the group whose wiki holds the page, or the group of the project whose wiki does. Absent for a project in a user namespace"`
}

// WikiOutput holds a paginated list of wiki blob search results.
type WikiOutput struct {
	toolutil.HintableOutput
	WikiBlobs  []WikiBlobOutput          `json:"wiki_blobs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Wiki searches for wiki blobs in GitLab.
// Scope priority: project_id > group_id > global.
//
// The page is read from the captured response, whatever client-go made of
// it: GitLab's blob keys share none with client-go's Wiki struct, so client-go
// decodes each result into an empty Wiki and reports no error. capturesPage
// is defensive, for an answer whose key client-go's Wiki declares with
// another type, which would otherwise fail a page GitLab answered.
func Wiki(ctx context.Context, client *gitlabclient.Client, input WikiInput) (WikiOutput, error) {
	searchClient := client.GL().Search
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	_, resp, err := runScopedSearch(ctx, scopedSearchArgs[*gl.Wiki]{
		query: input.Query, projectID: input.ProjectID, groupID: input.GroupID, ref: input.Ref, page: input.Page, perPage: input.PerPage,
		searchType: input.SearchType, operation: "searchWiki", projectSearch: searchClient.WikiBlobsByProject,
		groupSearch: searchClient.WikiBlobsByGroup, globalSearch: searchClient.WikiBlobs,
		capturesPage: true,
	})
	if err != nil {
		return WikiOutput{}, err
	}
	var out []WikiBlobOutput
	if err = captured.Decode(&out); err != nil {
		return WikiOutput{}, toolutil.WrapErr("searchWiki", err)
	}
	return WikiOutput{WikiBlobs: out, Pagination: searchPagination(resp, len(out))}, nil
}
