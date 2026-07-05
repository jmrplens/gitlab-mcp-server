package commits

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Action specifies a single file operation within a commit. It mirrors
// gl.CommitActionOptions field-for-field (C-IMPORTS).
type Action struct {
	Action          string `json:"action"    jsonschema:"Action to perform on the file (create, update, delete, move, chmod),required"`
	FilePath        string `json:"file_path" jsonschema:"Full path of the file (e.g. src/main.go),required"`
	PreviousPath    string `json:"previous_path,omitempty" jsonschema:"Original path when action is move"`
	Content         string `json:"content,omitempty"    jsonschema:"File content (required for create and update)"`
	Encoding        string `json:"encoding,omitempty"   jsonschema:"Content encoding: 'text' (default) or 'base64'"`
	LastCommitID    string `json:"last_commit_id,omitempty" jsonschema:"Last known commit ID of this file for conflict detection"`
	ExecuteFilemode bool   `json:"execute_filemode,omitempty" jsonschema:"When true, enable or disable the executable flag on the file"`
}

// CreateInput defines parameters for creating a commit with file actions.
// It mirrors gl.CreateCommitOptions (C-IMPORTS).
type CreateInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	Branch        string               `json:"branch"          jsonschema:"Target branch name,required"`
	CommitMessage string               `json:"commit_message"  jsonschema:"Commit message,required"`
	StartBranch   string               `json:"start_branch,omitempty" jsonschema:"Branch to start from if target branch does not exist"`
	StartSHA      string               `json:"start_sha,omitempty"    jsonschema:"SHA to start from if target branch does not exist (alternative to start_branch)"`
	StartProject  string               `json:"start_project,omitempty" jsonschema:"Project ID or path of the source project to start the branch from (for cross-project commits)"`
	Actions       []Action             `json:"actions"         jsonschema:"List of file actions (create, update, delete, move, chmod),required"`
	AuthorEmail   string               `json:"author_email,omitempty" jsonschema:"Custom author email"`
	AuthorName    string               `json:"author_name,omitempty"  jsonschema:"Custom author name"`
	Stats         bool                 `json:"stats,omitempty"        jsonschema:"Include commit stats (additions, deletions, total) in the response"`
	Force         bool                 `json:"force,omitempty"        jsonschema:"When true, force-overwrite the target branch even if a conflict exists"`
}

// Output represents a created commit. It mirrors the full gl.Commit payload
// (C-IMPORTS), surfacing trailers, the last associated pipeline, and stats.
type Output struct {
	toolutil.HintableOutput
	ID               string              `json:"id"`
	ShortID          string              `json:"short_id"`
	Title            string              `json:"title"`
	Message          string              `json:"message,omitempty"`
	AuthorName       string              `json:"author_name"`
	AuthorEmail      string              `json:"author_email"`
	AuthoredDate     string              `json:"authored_date,omitempty"`
	CommitterName    string              `json:"committer_name"`
	CommitterEmail   string              `json:"committer_email"`
	CommittedDate    string              `json:"committed_date"`
	CreatedAt        string              `json:"created_at,omitempty"`
	WebURL           string              `json:"web_url"`
	ParentIDs        []string            `json:"parent_ids,omitempty"`
	Status           string              `json:"status,omitempty"`
	ProjectID        int64               `json:"project_id,omitempty"`
	Trailers         map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string   `json:"extended_trailers,omitempty"`
	LastPipeline     *LastPipelineOutput `json:"last_pipeline,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
}

// CommitStatsOutput mirrors gl.CommitStats: line additions/deletions/total
// for a commit.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// LastPipelineOutput mirrors gl.PipelineInfo, the pipeline summary embedded in
// a commit payload as last_pipeline. Canonical shape shared via toolutil.
type LastPipelineOutput = toolutil.LastPipelineOutput

// BasicUserOutput mirrors gl.Author, the user object returned on commit
// comments and commit statuses.
type BasicUserOutput struct {
	ID        int64  `json:"id,omitempty"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	State     string `json:"state,omitempty"`
	Blocked   bool   `json:"blocked,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// authorToOutput maps gl.Author to *BasicUserOutput, or nil when the author is
// empty (zero-valued).
func authorToOutput(a gl.Author) *BasicUserOutput {
	if a.ID == 0 && a.Username == "" && a.Name == "" && a.Email == "" {
		return nil
	}
	out := &BasicUserOutput{
		ID:       a.ID,
		Username: a.Username,
		Name:     a.Name,
		Email:    a.Email,
		State:    a.State,
		Blocked:  a.Blocked,
	}
	if a.CreatedAt != nil {
		out.CreatedAt = a.CreatedAt.String()
	}
	return out
}

// Create creates a new commit in the specified GitLab project with one
// or more file actions. Supports optional start branch (to create a new branch
// from an existing one) and custom author metadata. Returns the created commit
// details or an error if the API call fails.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("commitCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	actions := make([]*gl.CommitActionOptions, len(input.Actions))
	for i, a := range input.Actions {
		action := &gl.CommitActionOptions{
			Action:   new(gl.FileActionValue(a.Action)),
			FilePath: new(a.FilePath),
		}
		if a.PreviousPath != "" {
			action.PreviousPath = new(a.PreviousPath)
		}
		if a.Content != "" {
			action.Content = new(toolutil.NormalizeText(a.Content))
		}
		if a.Encoding != "" {
			action.Encoding = new(a.Encoding)
		}
		if a.LastCommitID != "" {
			action.LastCommitID = new(a.LastCommitID)
		}
		if a.ExecuteFilemode {
			action.ExecuteFilemode = new(true)
		}
		actions[i] = action
	}

	opts := &gl.CreateCommitOptions{
		Branch:        new(input.Branch),
		CommitMessage: new(toolutil.NormalizeText(input.CommitMessage)),
		Actions:       actions,
	}
	if input.StartBranch != "" {
		opts.StartBranch = new(input.StartBranch)
	}
	if input.StartSHA != "" {
		opts.StartSHA = new(input.StartSHA)
	}
	if input.StartProject != "" {
		opts.StartProject = new(input.StartProject)
	}
	if input.AuthorEmail != "" {
		opts.AuthorEmail = new(input.AuthorEmail)
	}
	if input.AuthorName != "" {
		opts.AuthorName = new(input.AuthorName)
	}
	if input.Stats {
		opts.Stats = new(true)
	}
	if input.Force {
		opts.Force = new(true)
	}

	c, _, err := client.GL().Commits.CreateCommit(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return Output{}, toolutil.WrapErrWithHint("commitCreate", err, "check that the branch exists, file paths are valid, and required content is provided for create/update actions")
		}
		return Output{}, toolutil.WrapErrWithMessage("commitCreate", err)
	}
	return ToOutput(c), nil
}

// commitFields holds the gl.Commit values shared by Output and DetailOutput.
// Centralizing the mapping keeps the two typed constructors from diverging and
// avoids duplicated conversion logic.
type commitFields struct {
	id, shortID, title, message                    string
	authorName, authorEmail                        string
	committerName, committerEmail                  string
	authoredDate, committedDate, createdAt, webURL string
	status                                         string
	projectID                                      int64
	parentIDs                                      []string
	trailers, extendedTrailers                     map[string]string
	lastPipeline                                   *LastPipelineOutput
	stats                                          *CommitStatsOutput
}

// commitToFields maps a gl.Commit into the shared commitFields intermediate,
// stringifying timestamps and projecting nested objects.
func commitToFields(c *gl.Commit) commitFields {
	f := commitFields{
		id:               c.ID,
		shortID:          c.ShortID,
		title:            c.Title,
		message:          c.Message,
		authorName:       c.AuthorName,
		authorEmail:      c.AuthorEmail,
		committerName:    c.CommitterName,
		committerEmail:   c.CommitterEmail,
		webURL:           c.WebURL,
		parentIDs:        c.ParentIDs,
		projectID:        c.ProjectID,
		trailers:         c.Trailers,
		extendedTrailers: c.ExtendedTrailers,
		lastPipeline:     pipelineInfoToOutput(c.LastPipeline),
	}
	if c.CommittedDate != nil {
		f.committedDate = c.CommittedDate.String()
	}
	if c.AuthoredDate != nil {
		f.authoredDate = c.AuthoredDate.String()
	}
	if c.CreatedAt != nil {
		f.createdAt = c.CreatedAt.String()
	}
	if c.Status != nil {
		f.status = string(*c.Status)
	}
	if c.Stats != nil {
		f.stats = &CommitStatsOutput{
			Additions: c.Stats.Additions,
			Deletions: c.Stats.Deletions,
			Total:     c.Stats.Total,
		}
	}
	return f
}

// ToOutput converts a GitLab API [gl.Commit] to the list/summary MCP tool
// output format, mirroring the full commit payload.
func ToOutput(c *gl.Commit) Output {
	f := commitToFields(c)
	return Output{
		ID:               f.id,
		ShortID:          f.shortID,
		Title:            f.title,
		Message:          f.message,
		AuthorName:       f.authorName,
		AuthorEmail:      f.authorEmail,
		AuthoredDate:     f.authoredDate,
		CommitterName:    f.committerName,
		CommitterEmail:   f.committerEmail,
		CommittedDate:    f.committedDate,
		CreatedAt:        f.createdAt,
		WebURL:           f.webURL,
		ParentIDs:        f.parentIDs,
		Status:           f.status,
		ProjectID:        f.projectID,
		Trailers:         f.trailers,
		ExtendedTrailers: f.extendedTrailers,
		LastPipeline:     f.lastPipeline,
		Stats:            f.stats,
	}
}

// ListInput defines parameters for listing commits in a GitLab project.
// It mirrors gl.ListCommitsOptions (C-IMPORTS) and supports offset and keyset
// pagination via order_by/sort/page_token.
type ListInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"            jsonschema:"Project ID or URL-encoded path,required"`
	RefName     string               `json:"ref_name,omitempty"    jsonschema:"Branch name, tag, or commit SHA to list commits from (default: default branch)"`
	Since       string               `json:"since,omitempty"       jsonschema:"Return commits after this date (ISO 8601 format, e.g. 2025-01-01T00:00:00Z)"`
	Until       string               `json:"until,omitempty"       jsonschema:"Return commits before this date (ISO 8601 format, e.g. 2025-12-31T23:59:59Z)"`
	Path        string               `json:"path,omitempty"        jsonschema:"File path to filter commits by (only commits touching this path)"`
	Author      string               `json:"author,omitempty"      jsonschema:"Filter by commit author name or email"`
	All         bool                 `json:"all,omitempty"         jsonschema:"Retrieve every commit from the whole repository (across all branches)"`
	WithStats   bool                 `json:"with_stats,omitempty"  jsonschema:"Include commit stats (additions, deletions, total)"`
	FirstParent bool                 `json:"first_parent,omitempty" jsonschema:"Follow only the first parent commit upon seeing a merge commit"`
	Trailers    bool                 `json:"trailers,omitempty"    jsonschema:"Parse and include Git trailers for every commit"`
	OrderBy     string               `json:"order_by,omitempty"    jsonschema:"For keyset pagination, the column to order results by"`
	Sort        string               `json:"sort,omitempty"        jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// ListOutput holds a paginated list of commits.
type ListOutput struct {
	toolutil.HintableOutput
	Commits    []Output                  `json:"commits"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List retrieves a paginated list of commits for a GitLab project.
// Supports filtering by branch/tag, date range, file path, and author.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("commitList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.ListCommitsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.RefName != "" {
		opts.RefName = new(input.RefName)
	}
	opts.Since = toolutil.ParseOptionalTime(input.Since)
	opts.Until = toolutil.ParseOptionalTime(input.Until)
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Author != "" {
		opts.Author = new(input.Author)
	}
	if input.All {
		opts.All = new(true)
	}
	if input.WithStats {
		opts.WithStats = new(true)
	}
	if input.FirstParent {
		opts.FirstParent = new(true)
	}
	if input.Trailers {
		opts.Trailers = new(true)
	}

	commits, resp, err := client.GL().Commits.ListCommits(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("commitList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get and ref_name (branch/tag/SHA) with gitlab_branch_list or gitlab_tag_list")
	}

	out := make([]Output, len(commits))
	for i, c := range commits {
		out[i] = ToOutput(c)
	}
	return ListOutput{Commits: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for retrieving a single commit. It mirrors
// gl.GetCommitOptions (C-IMPORTS).
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA hash to retrieve,required"`
	Stats     bool                 `json:"stats,omitempty" jsonschema:"Include commit stats (additions, deletions, total); defaults to true on GitLab"`
}

// DetailOutput represents a single commit with full details. It mirrors the
// full gl.Commit payload (C-IMPORTS).
type DetailOutput struct {
	toolutil.HintableOutput
	ID               string              `json:"id"`
	ShortID          string              `json:"short_id"`
	Title            string              `json:"title"`
	Message          string              `json:"message"`
	AuthorName       string              `json:"author_name"`
	AuthorEmail      string              `json:"author_email"`
	AuthoredDate     string              `json:"authored_date,omitempty"`
	CommitterName    string              `json:"committer_name"`
	CommitterEmail   string              `json:"committer_email"`
	CommittedDate    string              `json:"committed_date"`
	CreatedAt        string              `json:"created_at,omitempty"`
	WebURL           string              `json:"web_url"`
	ParentIDs        []string            `json:"parent_ids"`
	Status           string              `json:"status,omitempty"`
	ProjectID        int64               `json:"project_id,omitempty"`
	Trailers         map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string   `json:"extended_trailers,omitempty"`
	LastPipeline     *LastPipelineOutput `json:"last_pipeline,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
}

// Get retrieves a single commit by SHA from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (DetailOutput, error) {
	if err := ctx.Err(); err != nil {
		return DetailOutput{}, err
	}
	if input.ProjectID == "" {
		return DetailOutput{}, errors.New("commitGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	var opts *gl.GetCommitOptions
	if input.Stats {
		opts = &gl.GetCommitOptions{Stats: new(true)}
	}
	c, _, err := client.GL().Commits.GetCommit(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		return DetailOutput{}, toolutil.WrapErrWithStatusHint("commitGet", err, http.StatusNotFound,
			"verify SHA exists in this project (use full or short SHA, branch name, or tag name)")
	}
	return detailToOutput(c), nil
}

// detailToOutput converts a GitLab API [gl.Commit] to the detailed MCP
// output format, mirroring the full commit payload including trailers,
// last pipeline, and optional stats.
func detailToOutput(c *gl.Commit) DetailOutput {
	f := commitToFields(c)
	return DetailOutput{
		ID:               f.id,
		ShortID:          f.shortID,
		Title:            f.title,
		Message:          f.message,
		AuthorName:       f.authorName,
		AuthorEmail:      f.authorEmail,
		AuthoredDate:     f.authoredDate,
		CommitterName:    f.committerName,
		CommitterEmail:   f.committerEmail,
		CommittedDate:    f.committedDate,
		CreatedAt:        f.createdAt,
		WebURL:           f.webURL,
		ParentIDs:        f.parentIDs,
		Status:           f.status,
		ProjectID:        f.projectID,
		Trailers:         f.trailers,
		ExtendedTrailers: f.extendedTrailers,
		LastPipeline:     f.lastPipeline,
		Stats:            f.stats,
	}
}

// DiffInput defines parameters for retrieving diffs of a commit. It mirrors
// gl.GetCommitDiffOptions (C-IMPORTS) and supports offset and keyset
// pagination via order_by/sort/page_token.
type DiffInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA hash to get diffs for,required"`
	Unidiff   bool                 `json:"unidiff,omitempty" jsonschema:"Return diffs in unified diff format"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// DiffOutput holds the list of file diffs for a commit.
type DiffOutput struct {
	toolutil.HintableOutput
	Diffs      []toolutil.DiffOutput     `json:"diffs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Diff retrieves the file diffs for a specific commit.
func Diff(ctx context.Context, client *gitlabclient.Client, input DiffInput) (DiffOutput, error) {
	if err := ctx.Err(); err != nil {
		return DiffOutput{}, err
	}
	if input.ProjectID == "" {
		return DiffOutput{}, errors.New("commitDiff: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	opts := &gl.GetCommitDiffOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Unidiff {
		opts.Unidiff = new(true)
	}

	diffs, resp, err := client.GL().Commits.GetCommitDiff(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		return DiffOutput{}, toolutil.WrapErrWithStatusHint("commitDiff", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get; large diffs may be truncated by GitLab \u2014 use unidiff=true for git-compatible format")
	}

	out := make([]toolutil.DiffOutput, len(diffs))
	for i, d := range diffs {
		out[i] = toolutil.DiffToOutput(d)
	}
	return DiffOutput{Diffs: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// GetCommitRefs — branches/tags referencing a commit
// ---------------------------------------------------------------------------.

// RefsInput defines parameters for retrieving branches/tags referencing a
// commit. It mirrors gl.GetCommitRefsOptions (C-IMPORTS) and supports offset
// and keyset pagination via order_by/sort/page_token.
type RefsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA to look up,required"`
	Type      string               `json:"type,omitempty" jsonschema:"Filter by ref type: branch, tag, or all (default: all)"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// RefOutput represents a branch or tag referencing a commit.
type RefOutput struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// RefsOutput holds a paginated list of refs referencing a commit.
type RefsOutput struct {
	toolutil.HintableOutput
	Refs       []RefOutput               `json:"refs"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetRefs retrieves branches and tags a commit is pushed to.
func GetRefs(ctx context.Context, client *gitlabclient.Client, input RefsInput) (RefsOutput, error) {
	if err := ctx.Err(); err != nil {
		return RefsOutput{}, err
	}
	if input.ProjectID == "" {
		return RefsOutput{}, errors.New("getCommitRefs: project_id is required")
	}
	opts := &gl.GetCommitRefsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Type != "" {
		opts.Type = new(input.Type)
	}
	refs, resp, err := client.GL().Commits.GetCommitRefs(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		return RefsOutput{}, toolutil.WrapErrWithStatusHint("getCommitRefs", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get \u2014 refs lists branches/tags containing the commit")
	}
	out := make([]RefOutput, len(refs))
	for i, r := range refs {
		out[i] = RefOutput{Type: r.Type, Name: r.Name}
	}
	return RefsOutput{Refs: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// ---------------------------------------------------------------------------
// GetCommitComments / PostCommitComment
// ---------------------------------------------------------------------------.

// CommentsInput defines parameters for listing comments on a commit. It mirrors
// gl.GetCommitCommentsOptions (C-IMPORTS) and supports offset and keyset
// pagination via order_by/sort/page_token.
type CommentsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"For keyset pagination, the column to order results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// CommentOutput represents a single commit comment. It mirrors gl.CommitComment
// (C-IMPORTS), surfacing the author as a full user object.
type CommentOutput struct {
	toolutil.HintableOutput
	Note     string           `json:"note"`
	Path     string           `json:"path,omitempty"`
	Line     int64            `json:"line,omitempty"`
	LineType string           `json:"line_type,omitempty"`
	Author   *BasicUserOutput `json:"author,omitempty"`
}

// CommentsOutput holds a paginated list of commit comments.
type CommentsOutput struct {
	toolutil.HintableOutput
	Comments   []CommentOutput           `json:"comments"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetComments retrieves the comments on a commit.
func GetComments(ctx context.Context, client *gitlabclient.Client, input CommentsInput) (CommentsOutput, error) {
	if err := ctx.Err(); err != nil {
		return CommentsOutput{}, err
	}
	if input.ProjectID == "" {
		return CommentsOutput{}, errors.New("getCommitComments: project_id is required")
	}
	opts := &gl.GetCommitCommentsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	comments, resp, err := client.GL().Commits.GetCommitComments(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		return CommentsOutput{}, toolutil.WrapErrWithStatusHint("getCommitComments", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get")
	}
	out := make([]CommentOutput, len(comments))
	for i, c := range comments {
		out[i] = commentToOutput(c)
	}
	return CommentsOutput{Comments: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// commentToOutput converts the GitLab API response to the tool output format,
// mirroring the full author user object.
func commentToOutput(c *gl.CommitComment) CommentOutput {
	return CommentOutput{
		Note:     c.Note,
		Path:     c.Path,
		Line:     c.Line,
		LineType: c.LineType,
		Author:   authorToOutput(c.Author),
	}
}

// PostCommentInput defines parameters for posting a comment on a commit.
type PostCommentInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA,required"`
	Note      string               `json:"note"       jsonschema:"Comment text,required"`
	Path      string               `json:"path,omitempty"      jsonschema:"File path to comment on (for inline comments)"`
	Line      int64                `json:"line,omitempty"      jsonschema:"Line number to comment on"`
	LineType  string               `json:"line_type,omitempty" jsonschema:"Line type: new or old (default: new)"`
}

// PostComment creates a comment on a commit.
func PostComment(ctx context.Context, client *gitlabclient.Client, input PostCommentInput) (CommentOutput, error) {
	if err := ctx.Err(); err != nil {
		return CommentOutput{}, err
	}
	if input.ProjectID == "" {
		return CommentOutput{}, errors.New("postCommitComment: project_id is required")
	}
	opts := &gl.PostCommitCommentOptions{
		Note: new(input.Note),
	}
	if input.Path != "" {
		opts.Path = new(input.Path)
	}
	if input.Line > 0 {
		opts.Line = new(input.Line)
	}
	if input.LineType != "" {
		opts.LineType = new(input.LineType)
	}
	c, _, err := client.GL().Commits.PostCommitComment(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return CommentOutput{}, toolutil.WrapErrWithHint("postCommitComment", err,
				"when line is set, line_type must be 'new' or 'old' and path must point to a file changed in the commit")
		}
		return CommentOutput{}, toolutil.WrapErrWithStatusHint("postCommitComment", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get; commenting requires Reporter+ role")
	}
	return commentToOutput(c), nil
}

// ---------------------------------------------------------------------------
// GetCommitStatuses / SetCommitStatus
// ---------------------------------------------------------------------------.

// StatusesInput defines parameters for listing pipeline statuses of a commit.
// It mirrors gl.GetCommitStatusesOptions (C-IMPORTS) and supports offset and
// keyset pagination via order_by/sort/page_token.
type StatusesInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id"  jsonschema:"Project ID or URL-encoded path,required"`
	SHA        string               `json:"sha"         jsonschema:"Commit SHA,required"`
	Ref        string               `json:"ref,omitempty"        jsonschema:"Branch or tag name filter"`
	Stage      string               `json:"stage,omitempty"      jsonschema:"Stage name filter"`
	Name       string               `json:"name,omitempty"       jsonschema:"Status name filter"`
	PipelineID int64                `json:"pipeline_id,omitempty" jsonschema:"Pipeline ID filter"`
	All        bool                 `json:"all,omitempty"        jsonschema:"Return all statuses including retries"`
	OrderBy    string               `json:"order_by,omitempty"   jsonschema:"For keyset pagination, the column to order results by"`
	Sort       string               `json:"sort,omitempty"       jsonschema:"Sort order for keyset pagination: 'asc' or 'desc'"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// StatusOutput represents a single commit pipeline status. It mirrors
// gl.CommitStatus (C-IMPORTS), surfacing the author as a full user object.
type StatusOutput struct {
	toolutil.HintableOutput
	ID           int64            `json:"id"`
	SHA          string           `json:"sha"`
	Ref          string           `json:"ref"`
	Status       string           `json:"status"`
	Name         string           `json:"name"`
	TargetURL    string           `json:"target_url,omitempty"`
	Description  string           `json:"description,omitempty"`
	Coverage     float64          `json:"coverage,omitempty"`
	PipelineID   int64            `json:"pipeline_id,omitempty"`
	AllowFailure bool             `json:"allow_failure,omitempty"`
	CreatedAt    string           `json:"created_at,omitempty"`
	StartedAt    string           `json:"started_at,omitempty"`
	FinishedAt   string           `json:"finished_at,omitempty"`
	Author       *BasicUserOutput `json:"author,omitempty"`
}

// StatusesOutput holds a paginated list of commit statuses.
type StatusesOutput struct {
	toolutil.HintableOutput
	Statuses   []StatusOutput            `json:"statuses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetStatuses retrieves the pipeline statuses of a commit.
func GetStatuses(ctx context.Context, client *gitlabclient.Client, input StatusesInput) (StatusesOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusesOutput{}, err
	}
	if input.ProjectID == "" {
		return StatusesOutput{}, errors.New("getCommitStatuses: project_id is required")
	}
	opts := &gl.GetCommitStatusesOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if input.Stage != "" {
		opts.Stage = new(input.Stage)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.PipelineID > 0 {
		opts.PipelineID = new(input.PipelineID)
	}
	if input.All {
		opts.All = new(true)
	}
	statuses, resp, err := client.GL().Commits.GetCommitStatuses(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		return StatusesOutput{}, toolutil.WrapErrWithStatusHint("getCommitStatuses", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get \u2014 statuses are populated by CI jobs and external integrations")
	}
	out := make([]StatusOutput, len(statuses))
	for i, s := range statuses {
		out[i] = statusToOutput(s)
	}
	return StatusesOutput{Statuses: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// statusToOutput converts the GitLab API response to the tool output format.
func statusToOutput(s *gl.CommitStatus) StatusOutput {
	out := StatusOutput{
		ID:           s.ID,
		SHA:          s.SHA,
		Ref:          s.Ref,
		Status:       s.Status,
		Name:         s.Name,
		TargetURL:    s.TargetURL,
		Description:  s.Description,
		Coverage:     s.Coverage,
		PipelineID:   s.PipelineID,
		AllowFailure: s.AllowFailure,
	}
	if s.CreatedAt != nil {
		out.CreatedAt = s.CreatedAt.String()
	}
	if s.StartedAt != nil {
		out.StartedAt = s.StartedAt.String()
	}
	if s.FinishedAt != nil {
		out.FinishedAt = s.FinishedAt.String()
	}
	out.Author = authorToOutput(s.Author)
	return out
}

// SetStatusInput defines parameters for setting a commit pipeline status.
type SetStatusInput struct {
	ProjectID   toolutil.StringOrInt `json:"project_id"   jsonschema:"Project ID or URL-encoded path,required"`
	SHA         string               `json:"sha"          jsonschema:"Commit SHA,required"`
	State       string               `json:"state"        jsonschema:"Status state: pending, running, success, failed, canceled,required"`
	Ref         string               `json:"ref,omitempty"         jsonschema:"Branch or tag name"`
	Name        string               `json:"name,omitempty"        jsonschema:"Status name / context"`
	Context     string               `json:"context,omitempty"     jsonschema:"Status context label (overrides name)"`
	TargetURL   string               `json:"target_url,omitempty"  jsonschema:"URL to link from the status"`
	Description string               `json:"description,omitempty" jsonschema:"Short description of the status"`
	Coverage    float64              `json:"coverage,omitempty"    jsonschema:"Code coverage percentage"`
	PipelineID  int64                `json:"pipeline_id,omitempty" jsonschema:"Pipeline ID to associate the status with"`
}

// SetStatus sets the pipeline status of a commit.
func SetStatus(ctx context.Context, client *gitlabclient.Client, input SetStatusInput) (StatusOutput, error) {
	if err := ctx.Err(); err != nil {
		return StatusOutput{}, err
	}
	if input.ProjectID == "" {
		return StatusOutput{}, errors.New("setCommitStatus: project_id is required")
	}
	opts := &gl.SetCommitStatusOptions{
		State: gl.BuildStateValue(input.State),
	}
	if input.Ref != "" {
		opts.Ref = new(input.Ref)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Context != "" {
		opts.Context = new(input.Context)
	}
	if input.TargetURL != "" {
		opts.TargetURL = new(input.TargetURL)
	}
	if input.Description != "" {
		opts.Description = new(input.Description)
	}
	if input.Coverage > 0 {
		opts.Coverage = new(input.Coverage)
	}
	if input.PipelineID > 0 {
		opts.PipelineID = new(input.PipelineID)
	}
	s, _, err := client.GL().Commits.SetCommitStatus(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusForbidden) {
			return StatusOutput{}, toolutil.WrapErrWithHint("setCommitStatus", err,
				"setting commit status requires Developer+ role and a CI/CD-enabled project")
		}
		if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return StatusOutput{}, toolutil.WrapErrWithHint("setCommitStatus", err,
				"state must be one of: pending, running, success, failed, canceled, skipped \u2014 status names are case-sensitive")
		}
		return StatusOutput{}, toolutil.WrapErrWithStatusHint("setCommitStatus", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get")
	}
	return statusToOutput(s), nil
}

// ---------------------------------------------------------------------------
// ListMergeRequestsByCommit
// ---------------------------------------------------------------------------.

// MRsByCommitInput defines parameters for listing merge requests associated with a commit.
type MRsByCommitInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA,required"`
}

// BasicMROutput represents a basic merge request associated with a commit.
type BasicMROutput struct {
	ID           int64  `json:"id"`
	IID          int64  `json:"merge_request_iid"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
	Author       string `json:"author,omitempty"`
}

// MRsByCommitOutput holds the list of merge requests for a commit.
type MRsByCommitOutput struct {
	toolutil.HintableOutput
	MergeRequests []BasicMROutput `json:"merge_requests"`
}

// ListMRsByCommit retrieves merge requests associated with a commit.
func ListMRsByCommit(ctx context.Context, client *gitlabclient.Client, input MRsByCommitInput) (MRsByCommitOutput, error) {
	if err := ctx.Err(); err != nil {
		return MRsByCommitOutput{}, err
	}
	if input.ProjectID == "" {
		return MRsByCommitOutput{}, errors.New("listMergeRequestsByCommit: project_id is required")
	}
	mrs, _, err := client.GL().Commits.ListMergeRequestsByCommit(string(input.ProjectID), input.SHA, gl.WithContext(ctx))
	if err != nil {
		return MRsByCommitOutput{}, toolutil.WrapErrWithStatusHint("listMergeRequestsByCommit", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get \u2014 returns MRs that include this commit")
	}
	out := make([]BasicMROutput, len(mrs))
	for i, mr := range mrs {
		o := BasicMROutput{
			ID:           mr.ID,
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			WebURL:       mr.WebURL,
		}
		if mr.Author != nil {
			o.Author = mr.Author.Username
		}
		out[i] = o
	}
	return MRsByCommitOutput{MergeRequests: out}, nil
}

// ---------------------------------------------------------------------------
// CherryPickCommit
// ---------------------------------------------------------------------------.

// CherryPickInput defines parameters for cherry-picking a commit.
type CherryPickInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA to cherry-pick"`
	Branch    string               `json:"branch"     jsonschema:"Target branch name,required"`
	DryRun    bool                 `json:"dry_run,omitempty"  jsonschema:"If true, does not create the commit but checks for conflicts"`
	Message   string               `json:"message,omitempty"  jsonschema:"Custom commit message (defaults to original)"`
}

// CherryPick cherry-picks a commit to a target branch.
func CherryPick(ctx context.Context, client *gitlabclient.Client, input CherryPickInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("cherryPickCommit: project_id is required")
	}
	opts := &gl.CherryPickCommitOptions{
		Branch: new(input.Branch),
	}
	if input.DryRun {
		opts.DryRun = new(true)
	}
	if input.Message != "" {
		opts.Message = new(input.Message)
	}
	c, _, err := client.GL().Commits.CherryPickCommit(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("cherryPickCommit", err, "the commit may produce an empty cherry-pick or the branch may not exist")
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("cherryPickCommit", err, "cherry-pick has merge conflicts — resolve manually or cherry-pick to a different branch")
		default:
			return Output{}, toolutil.WrapErrWithMessage("cherryPickCommit", err)
		}
	}
	return ToOutput(c), nil
}

// ---------------------------------------------------------------------------
// RevertCommit
// ---------------------------------------------------------------------------.

// RevertInput defines parameters for reverting a commit.
type RevertInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA to revert"`
	Branch    string               `json:"branch"     jsonschema:"Target branch name,required"`
}

// Revert reverts a commit on a target branch.
func Revert(ctx context.Context, client *gitlabclient.Client, input RevertInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("revertCommit: project_id is required")
	}
	opts := &gl.RevertCommitOptions{
		Branch: new(input.Branch),
	}
	c, _, err := client.GL().Commits.RevertCommit(string(input.ProjectID), input.SHA, opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.IsHTTPStatus(err, http.StatusBadRequest):
			return Output{}, toolutil.WrapErrWithHint("revertCommit", err, "the commit may already be reverted or the branch may not exist")
		case toolutil.IsHTTPStatus(err, http.StatusConflict):
			return Output{}, toolutil.WrapErrWithHint("revertCommit", err, "revert has merge conflicts — resolve manually or revert on a different branch")
		default:
			return Output{}, toolutil.WrapErrWithMessage("revertCommit", err)
		}
	}
	return ToOutput(c), nil
}

// ---------------------------------------------------------------------------
// GetGPGSignature
// ---------------------------------------------------------------------------.

// GPGSignatureInput defines parameters for retrieving a commit's GPG signature.
type GPGSignatureInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	SHA       string               `json:"sha"        jsonschema:"Commit SHA,required"`
}

// GPGSignatureOutput represents a commit's cryptographic signature. It mirrors
// the documented "Get the signature of a commit" response, which is a superset
// of gl.GPGSignature: the SDK struct only exposes the PGP (gpg_key_*) fields and
// verification_status, while the documented response also returns signature_type,
// commit_source, and the SSH (key) and X.509 (x509_certificate) sub-objects.
// Those documented-but-not-in-SDK fields are surfaced via a raw REST fetch
// (client.GL().NewRequest+Do, per doc/api/commits.md#get-the-signature-of-a-commit).
type GPGSignatureOutput struct {
	toolutil.HintableOutput
	SignatureType      string                 `json:"signature_type,omitempty"`
	VerificationStatus string                 `json:"verification_status"`
	CommitSource       string                 `json:"commit_source,omitempty"`
	KeyID              int64                  `json:"gpg_key_id"`
	KeyPrimaryKeyID    string                 `json:"gpg_key_primary_keyid"`
	KeyUserName        string                 `json:"gpg_key_user_name"`
	KeyUserEmail       string                 `json:"gpg_key_user_email"`
	KeySubkeyID        int64                  `json:"gpg_key_subkey_id,omitempty"`
	Key                *SSHSignatureKey       `json:"key,omitempty"`
	X509Certificate    *X509CertificateOutput `json:"x509_certificate,omitempty"`
}

// SSHSignatureKey mirrors the documented `key` object on an SSH-signed commit
// signature (doc/api/commits.md#get-the-signature-of-a-commit). The SDK
// gl.GPGSignature lacks this object, so it is decoded via the raw REST superset.
type SSHSignatureKey struct {
	ID        int64  `json:"id"`
	Title     string `json:"title,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Key       string `json:"key,omitempty"`
	UsageType string `json:"usage_type,omitempty"`
}

// X509CertificateOutput mirrors the documented `x509_certificate` object on an
// X.509-signed commit signature (doc/api/commits.md#get-the-signature-of-a-commit).
// The SDK gl.GPGSignature lacks this object, so it is decoded via the raw REST
// superset.
type X509CertificateOutput struct {
	ID                   int64             `json:"id"`
	Subject              string            `json:"subject,omitempty"`
	SubjectKeyIdentifier string            `json:"subject_key_identifier,omitempty"`
	Email                string            `json:"email,omitempty"`
	SerialNumber         json.Number       `json:"serial_number,omitempty"`
	CertificateStatus    string            `json:"certificate_status,omitempty"`
	X509Issuer           *X509IssuerOutput `json:"x509_issuer,omitempty"`
}

// X509IssuerOutput mirrors the documented nested `x509_issuer` object of an X.509
// certificate (doc/api/commits.md#get-the-signature-of-a-commit).
type X509IssuerOutput struct {
	ID                   int64  `json:"id"`
	Subject              string `json:"subject,omitempty"`
	SubjectKeyIdentifier string `json:"subject_key_identifier,omitempty"`
	CRLURL               string `json:"crl_url,omitempty"`
}

// gpgSignatureAPI is the raw-fetch superset decoded from the documented
// signature response. It is intentionally tolerant: GitLab versions that omit the
// newer signature_type/commit_source/key/x509_certificate fields decode them as
// zero values, which are then dropped by omitempty.
type gpgSignatureAPI struct {
	SignatureType      string                 `json:"signature_type"`
	VerificationStatus string                 `json:"verification_status"`
	CommitSource       string                 `json:"commit_source"`
	KeyID              int64                  `json:"gpg_key_id"`
	KeyPrimaryKeyID    string                 `json:"gpg_key_primary_keyid"`
	KeyUserName        string                 `json:"gpg_key_user_name"`
	KeyUserEmail       string                 `json:"gpg_key_user_email"`
	KeySubkeyID        int64                  `json:"gpg_key_subkey_id"`
	Key                *SSHSignatureKey       `json:"key"`
	X509Certificate    *X509CertificateOutput `json:"x509_certificate"`
}

// rawGetGPGSignature issues a raw REST GET against the commit signature path,
// decoding the full documented response (including the SDK-missing signature_type,
// commit_source, key, and x509_certificate fields) into a [gpgSignatureAPI]. The
// unmarshal is naturally tolerant of older instances that omit these fields.
func rawGetGPGSignature(ctx context.Context, client *gitlabclient.Client, projectID toolutil.StringOrInt, sha string) (*gpgSignatureAPI, *gl.Response, error) {
	path := fmt.Sprintf("projects/%s/repository/commits/%s/signature",
		gl.PathEscape(string(projectID)), gl.PathEscape(sha))
	req, err := client.GL().NewRequest(http.MethodGet, path, nil, []gl.RequestOptionFunc{gl.WithContext(ctx)})
	if err != nil {
		return nil, nil, err
	}
	var sig gpgSignatureAPI
	resp, err := client.GL().Do(req, &sig)
	return &sig, resp, err
}

// GetGPGSignature retrieves the cryptographic signature of a commit. It uses a
// raw REST fetch to surface the full documented signature superset (PGP, SSH, and
// X.509 variants); the SDK's gl.GPGSignature only exposes the PGP subset.
func GetGPGSignature(ctx context.Context, client *gitlabclient.Client, input GPGSignatureInput) (GPGSignatureOutput, error) {
	if err := ctx.Err(); err != nil {
		return GPGSignatureOutput{}, err
	}
	if input.ProjectID == "" {
		return GPGSignatureOutput{}, errors.New("getGPGSignature: project_id is required")
	}
	sig, _, err := rawGetGPGSignature(ctx, client, input.ProjectID, input.SHA)
	if err != nil {
		return GPGSignatureOutput{}, toolutil.WrapErrWithStatusHint("getGPGSignature", err, http.StatusNotFound,
			"verify SHA with gitlab_commit_get \u2014 404 also returned for unsigned commits or unsupported signature types")
	}
	return GPGSignatureOutput{
		SignatureType:      sig.SignatureType,
		VerificationStatus: sig.VerificationStatus,
		CommitSource:       sig.CommitSource,
		KeyID:              sig.KeyID,
		KeyPrimaryKeyID:    sig.KeyPrimaryKeyID,
		KeyUserName:        sig.KeyUserName,
		KeyUserEmail:       sig.KeyUserEmail,
		KeySubkeyID:        sig.KeySubkeyID,
		Key:                sig.Key,
		X509Certificate:    sig.X509Certificate,
	}, nil
}

// pipelineInfoToOutput maps gl.PipelineInfo to *LastPipelineOutput, or nil when
// the commit has no associated pipeline.
func pipelineInfoToOutput(p *gl.PipelineInfo) *LastPipelineOutput {
	return toolutil.NewLastPipelineOutput(p)
}
