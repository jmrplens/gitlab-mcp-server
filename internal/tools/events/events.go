package events

import (
	"context"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Input types.

// ListContributionEventsInput contains parameters for listing current user contribution events.
type ListContributionEventsInput struct {
	Action     string `json:"action,omitempty" jsonschema:"Filter by action type (created, updated, closed, reopened, pushed, commented, merged, joined, left, destroyed, expired)"`
	TargetType string `json:"target_type,omitempty" jsonschema:"Filter by target type (issue, milestone, merge_request, note, project, snippet, user)"`
	Before     string `json:"before,omitempty" jsonschema:"Return events before this date (YYYY-MM-DD)"`
	After      string `json:"after,omitempty" jsonschema:"Return events after this date (YYYY-MM-DD)"`
	Sort       string `json:"sort,omitempty" jsonschema:"Sort order (asc or desc)"`
	Scope      string `json:"scope,omitempty" jsonschema:"Include events from all projects (all) or only user's projects"`
	OrderBy    string `json:"order_by,omitempty" jsonschema:"Column by which to order keyset-paginated results (id)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output sub-objects (full mirrors of client-go event types).

// UserOutput mirrors gitlab.BasicUser embedded as an event author.
type UserOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// ContributionEventPushDataOutput mirrors gitlab.ContributionEventPushData.
type ContributionEventPushDataOutput struct {
	CommitCount int64  `json:"commit_count"`
	Action      string `json:"action,omitempty"`
	RefType     string `json:"ref_type,omitempty"`
	CommitFrom  string `json:"commit_from,omitempty"`
	CommitTo    string `json:"commit_to,omitempty"`
	Ref         string `json:"ref,omitempty"`
	CommitTitle string `json:"commit_title,omitempty"`
}

// NoteAuthorOutput mirrors gitlab.NoteAuthor / gitlab.NoteResolvedBy.
type NoteAuthorOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	Email     string `json:"email,omitempty"`
	Name      string `json:"name,omitempty"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
}

// LinePositionOutput mirrors gitlab.LinePosition.
type LinePositionOutput struct {
	LineCode string `json:"line_code,omitempty"`
	Type     string `json:"type,omitempty"`
	OldLine  int64  `json:"old_line,omitempty"`
	NewLine  int64  `json:"new_line,omitempty"`
}

// LineRangeOutput mirrors gitlab.LineRange.
type LineRangeOutput struct {
	StartRange *LinePositionOutput `json:"start,omitempty"`
	EndRange   *LinePositionOutput `json:"end,omitempty"`
}

// NotePositionOutput mirrors gitlab.NotePosition.
type NotePositionOutput struct {
	BaseSHA      string           `json:"base_sha,omitempty"`
	StartSHA     string           `json:"start_sha,omitempty"`
	HeadSHA      string           `json:"head_sha,omitempty"`
	PositionType string           `json:"position_type,omitempty"`
	NewPath      string           `json:"new_path,omitempty"`
	NewLine      int64            `json:"new_line,omitempty"`
	OldPath      string           `json:"old_path,omitempty"`
	OldLine      int64            `json:"old_line,omitempty"`
	LineRange    *LineRangeOutput `json:"line_range,omitempty"`
}

// NoteOutput mirrors gitlab.Note as embedded on a ContributionEvent.
type NoteOutput struct {
	ID           int64               `json:"id"`
	Type         string              `json:"type,omitempty"`
	Body         string              `json:"body,omitempty"`
	Attachment   string              `json:"attachment,omitempty"`
	Title        string              `json:"title,omitempty"`
	FileName     string              `json:"file_name,omitempty"`
	Author       *NoteAuthorOutput   `json:"author,omitempty"`
	System       bool                `json:"system"`
	CreatedAt    string              `json:"created_at,omitempty"`
	UpdatedAt    string              `json:"updated_at,omitempty"`
	ExpiresAt    string              `json:"expires_at,omitempty"`
	CommitID     string              `json:"commit_id,omitempty"`
	Position     *NotePositionOutput `json:"position,omitempty"`
	NoteableID   int64               `json:"noteable_id,omitempty"`
	NoteableType string              `json:"noteable_type,omitempty"`
	ProjectID    int64               `json:"project_id,omitempty"`
	NoteableIID  int64               `json:"noteable_iid,omitempty"`
	Resolvable   bool                `json:"resolvable"`
	Resolved     bool                `json:"resolved"`
	ResolvedAt   string              `json:"resolved_at,omitempty"`
	ResolvedBy   *NoteAuthorOutput   `json:"resolved_by,omitempty"`
	Internal     bool                `json:"internal"`
	Confidential bool                `json:"confidential"`
}

// ContributionEventOutput mirrors gitlab.ContributionEvent.
type ContributionEventOutput struct {
	ID             int64                            `json:"id"`
	Title          string                           `json:"title,omitempty"`
	ProjectID      int64                            `json:"project_id"`
	ActionName     string                           `json:"action_name"`
	TargetID       int64                            `json:"target_id,omitempty"`
	TargetIID      int64                            `json:"target_iid,omitempty"`
	TargetType     string                           `json:"target_type,omitempty"`
	TargetURL      string                           `json:"target_url,omitempty"`
	AuthorID       int64                            `json:"author_id"`
	TargetTitle    string                           `json:"target_title,omitempty"`
	CreatedAt      string                           `json:"created_at,omitempty"`
	PushData       *ContributionEventPushDataOutput `json:"push_data,omitempty"`
	Note           *NoteOutput                      `json:"note,omitempty"`
	Author         *UserOutput                      `json:"author,omitempty"`
	AuthorUsername string                           `json:"author_username,omitempty"`
}

// ListContributionEventsOutput holds a paginated list of contribution events.
type ListContributionEventsOutput struct {
	toolutil.HintableOutput
	Events     []ContributionEventOutput `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListCurrentUserContributionEvents returns contribution events for the authenticated user.
func ListCurrentUserContributionEvents(ctx context.Context, client *gitlabclient.Client, input ListContributionEventsInput) (ListContributionEventsOutput, error) {
	opts := &gl.ListContributionEventsOptions{}
	filters := newEventFilters(input.Action, input.TargetType, input.Before, input.After, input.Sort)
	opts.Action = filters.Action
	opts.TargetType = filters.TargetType
	opts.Before = filters.Before
	opts.After = filters.After
	opts.Sort = filters.Sort
	if input.Scope != "" {
		opts.Scope = &input.Scope
	}
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

	events, resp, err := client.GL().Events.ListCurrentUserContributionEvents(opts, gl.WithContext(ctx))
	if err != nil {
		return ListContributionEventsOutput{}, toolutil.WrapErrWithStatusHint("user_contribution_event_list", err, http.StatusForbidden, "verify your token has read_api scope")
	}

	out := ListContributionEventsOutput{
		Events:     make([]ContributionEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toContributionEventOutput(e))
	}

	enrichContributionEventURLs(ctx, client, out.Events)

	return out, nil
}

// toContributionEventOutput converts the GitLab API response to the tool output format.
func toContributionEventOutput(e *gl.ContributionEvent) ContributionEventOutput {
	o := ContributionEventOutput{
		ID:             e.ID,
		Title:          e.Title,
		ProjectID:      e.ProjectID,
		ActionName:     e.ActionName,
		TargetID:       e.TargetID,
		TargetIID:      e.TargetIID,
		TargetType:     e.TargetType,
		AuthorID:       e.AuthorID,
		TargetTitle:    e.TargetTitle,
		AuthorUsername: e.AuthorUsername,
		PushData:       toContributionPushDataOutput(e.PushData),
		Note:           toNoteOutput(e.Note),
		Author:         toBasicUserOutput(e.Author),
	}
	if e.CreatedAt != nil {
		o.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	return o
}

// toContributionPushDataOutput mirrors a non-empty ContributionEventPushData.
func toContributionPushDataOutput(d gl.ContributionEventPushData) *ContributionEventPushDataOutput {
	if d == (gl.ContributionEventPushData{}) {
		return nil
	}
	return &ContributionEventPushDataOutput{
		CommitCount: d.CommitCount,
		Action:      d.Action,
		RefType:     d.RefType,
		CommitFrom:  d.CommitFrom,
		CommitTo:    d.CommitTo,
		Ref:         d.Ref,
		CommitTitle: d.CommitTitle,
	}
}

// toBasicUserOutput mirrors a gitlab.BasicUser, returning nil for the zero value.
func toBasicUserOutput(u gl.BasicUser) *UserOutput {
	if u.ID == 0 && u.Username == "" && u.Name == "" {
		return nil
	}
	out := &UserOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
	if u.CreatedAt != nil {
		out.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// toNoteOutput mirrors a gitlab.Note, returning nil when absent.
func toNoteOutput(n *gl.Note) *NoteOutput {
	if n == nil {
		return nil
	}
	out := &NoteOutput{
		ID:           n.ID,
		Type:         string(n.Type),
		Body:         n.Body,
		Attachment:   n.Attachment,
		Title:        n.Title,
		FileName:     n.FileName,
		Author:       noteAuthorOutput(n.Author.ID, n.Author.Username, n.Author.Email, n.Author.Name, n.Author.State, n.Author.AvatarURL, n.Author.WebURL),
		System:       n.System,
		CommitID:     n.CommitID,
		Position:     toNotePositionOutput(n.Position),
		NoteableID:   n.NoteableID,
		NoteableType: n.NoteableType,
		ProjectID:    n.ProjectID,
		NoteableIID:  n.NoteableIID,
		Resolvable:   n.Resolvable,
		Resolved:     n.Resolved,
		ResolvedBy:   noteAuthorOutput(n.ResolvedBy.ID, n.ResolvedBy.Username, n.ResolvedBy.Email, n.ResolvedBy.Name, n.ResolvedBy.State, n.ResolvedBy.AvatarURL, n.ResolvedBy.WebURL),
		Internal:     n.Internal,
		Confidential: n.Confidential, //nolint:staticcheck // mirrored for 1:1 client-go field coverage
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = n.UpdatedAt.Format(time.RFC3339)
	}
	if n.ExpiresAt != nil {
		out.ExpiresAt = n.ExpiresAt.Format(time.RFC3339)
	}
	if n.ResolvedAt != nil {
		out.ResolvedAt = n.ResolvedAt.Format(time.RFC3339)
	}
	return out
}

// noteAuthorOutput builds a *NoteAuthorOutput from the shared author fields of
// gitlab.NoteAuthor and gitlab.NoteResolvedBy, returning nil for a zero value.
func noteAuthorOutput(id int64, username, email, name, state, avatarURL, webURL string) *NoteAuthorOutput {
	if id == 0 && username == "" && email == "" && name == "" && state == "" && avatarURL == "" && webURL == "" {
		return nil
	}
	return &NoteAuthorOutput{ID: id, Username: username, Email: email, Name: name, State: state, AvatarURL: avatarURL, WebURL: webURL}
}

// toNotePositionOutput mirrors a gitlab.NotePosition, returning nil when absent.
func toNotePositionOutput(p *gl.NotePosition) *NotePositionOutput {
	if p == nil {
		return nil
	}
	return &NotePositionOutput{
		BaseSHA:      p.BaseSHA,
		StartSHA:     p.StartSHA,
		HeadSHA:      p.HeadSHA,
		PositionType: p.PositionType,
		NewPath:      p.NewPath,
		NewLine:      p.NewLine,
		OldPath:      p.OldPath,
		OldLine:      p.OldLine,
		LineRange:    toLineRangeOutput(p.LineRange),
	}
}

// toLineRangeOutput mirrors a gitlab.LineRange, returning nil when absent.
func toLineRangeOutput(r *gl.LineRange) *LineRangeOutput {
	if r == nil {
		return nil
	}
	return &LineRangeOutput{
		StartRange: toLinePositionOutput(r.StartRange),
		EndRange:   toLinePositionOutput(r.EndRange),
	}
}

// toLinePositionOutput mirrors a gitlab.LinePosition, returning nil when absent.
func toLinePositionOutput(p *gl.LinePosition) *LinePositionOutput {
	if p == nil {
		return nil
	}
	return &LinePositionOutput{LineCode: p.LineCode, Type: p.Type, OldLine: p.OldLine, NewLine: p.NewLine}
}

// ListProjectEventsInput contains parameters for listing project visible events.
type ListProjectEventsInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Action     string               `json:"action,omitempty" jsonschema:"Filter by action type (created, updated, closed, reopened, pushed, commented, merged, joined, left, destroyed, expired)"`
	TargetType string               `json:"target_type,omitempty" jsonschema:"Filter by target type (issue, milestone, merge_request, note, project, snippet, user)"`
	Before     string               `json:"before,omitempty" jsonschema:"Return events before this date (YYYY-MM-DD)"`
	After      string               `json:"after,omitempty" jsonschema:"Return events after this date (YYYY-MM-DD)"`
	Sort       string               `json:"sort,omitempty" jsonschema:"Sort order (asc or desc, default desc)"`
	OrderBy    string               `json:"order_by,omitempty" jsonschema:"Column by which to order keyset-paginated results (id)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
}

// Output types.

// CommitStatsOutput mirrors gitlab.CommitStats.
type CommitStatsOutput struct {
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	Total     int64 `json:"total"`
}

// PipelineInfoOutput mirrors gitlab.PipelineInfo.
type PipelineInfoOutput struct {
	ID        int64  `json:"id"`
	IID       int64  `json:"iid"`
	ProjectID int64  `json:"project_id"`
	Status    string `json:"status,omitempty"`
	Source    string `json:"source,omitempty"`
	Ref       string `json:"ref,omitempty"`
	SHA       string `json:"sha,omitempty"`
	Name      string `json:"name,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// CommitOutput mirrors gitlab.Commit as embedded in project event push data.
type CommitOutput struct {
	ID               string              `json:"id"`
	ShortID          string              `json:"short_id,omitempty"`
	Title            string              `json:"title,omitempty"`
	AuthorName       string              `json:"author_name,omitempty"`
	AuthorEmail      string              `json:"author_email,omitempty"`
	AuthoredDate     string              `json:"authored_date,omitempty"`
	CommitterName    string              `json:"committer_name,omitempty"`
	CommitterEmail   string              `json:"committer_email,omitempty"`
	CommittedDate    string              `json:"committed_date,omitempty"`
	CreatedAt        string              `json:"created_at,omitempty"`
	Message          string              `json:"message,omitempty"`
	ParentIDs        []string            `json:"parent_ids,omitempty"`
	Stats            *CommitStatsOutput  `json:"stats,omitempty"`
	Status           string              `json:"status,omitempty"`
	LastPipeline     *PipelineInfoOutput `json:"last_pipeline,omitempty"`
	ProjectID        int64               `json:"project_id,omitempty"`
	Trailers         map[string]string   `json:"trailers,omitempty"`
	ExtendedTrailers map[string]string   `json:"extended_trailers,omitempty"`
	WebURL           string              `json:"web_url,omitempty"`
}

// RepositoryOutput mirrors gitlab.Repository.
type RepositoryOutput struct {
	Name              string `json:"name,omitempty"`
	Description       string `json:"description,omitempty"`
	WebURL            string `json:"web_url,omitempty"`
	AvatarURL         string `json:"avatar_url,omitempty"`
	GitSSHURL         string `json:"git_ssh_url,omitempty"`
	GitHTTPURL        string `json:"git_http_url,omitempty"`
	Namespace         string `json:"namespace,omitempty"`
	Visibility        string `json:"visibility,omitempty"`
	PathWithNamespace string `json:"path_with_namespace,omitempty"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	Homepage          string `json:"homepage,omitempty"`
	URL               string `json:"url,omitempty"`
	SSHURL            string `json:"ssh_url,omitempty"`
	HTTPURL           string `json:"http_url,omitempty"`
}

// ProjectEventDataOutput mirrors gitlab.ProjectEventData.
type ProjectEventDataOutput struct {
	Before            string            `json:"before,omitempty"`
	After             string            `json:"after,omitempty"`
	Ref               string            `json:"ref,omitempty"`
	UserID            int64             `json:"user_id,omitempty"`
	UserName          string            `json:"user_name,omitempty"`
	Repository        *RepositoryOutput `json:"repository,omitempty"`
	Commits           []CommitOutput    `json:"commits,omitempty"`
	TotalCommitsCount int64             `json:"total_commits_count"`
}

// ProjectEventNoteOutput mirrors gitlab.ProjectEventNote.
type ProjectEventNoteOutput struct {
	ID           int64             `json:"id"`
	Body         string            `json:"body,omitempty"`
	Attachment   string            `json:"attachment,omitempty"`
	Author       *NoteAuthorOutput `json:"author,omitempty"`
	CreatedAt    string            `json:"created_at,omitempty"`
	System       bool              `json:"system"`
	NoteableID   int64             `json:"noteable_id,omitempty"`
	NoteableType string            `json:"noteable_type,omitempty"`
	NoteableIID  int64             `json:"noteable_iid,omitempty"`
}

// ProjectEventPushDataOutput mirrors gitlab.ProjectEventPushData.
type ProjectEventPushDataOutput struct {
	CommitCount int64  `json:"commit_count"`
	Action      string `json:"action,omitempty"`
	RefType     string `json:"ref_type,omitempty"`
	CommitFrom  string `json:"commit_from,omitempty"`
	CommitTo    string `json:"commit_to,omitempty"`
	Ref         string `json:"ref,omitempty"`
	CommitTitle string `json:"commit_title,omitempty"`
}

// ProjectEventOutput mirrors gitlab.ProjectEvent.
type ProjectEventOutput struct {
	ID             int64                       `json:"id"`
	Title          string                      `json:"title,omitempty"`
	ProjectID      int64                       `json:"project_id"`
	ActionName     string                      `json:"action_name"`
	TargetID       int64                       `json:"target_id,omitempty"`
	TargetIID      int64                       `json:"target_iid,omitempty"`
	TargetType     string                      `json:"target_type,omitempty"`
	TargetURL      string                      `json:"target_url,omitempty"`
	AuthorID       int64                       `json:"author_id"`
	TargetTitle    string                      `json:"target_title,omitempty"`
	CreatedAt      string                      `json:"created_at,omitempty"`
	Author         *UserOutput                 `json:"author,omitempty"`
	AuthorUsername string                      `json:"author_username,omitempty"`
	Data           *ProjectEventDataOutput     `json:"data,omitempty"`
	Note           *ProjectEventNoteOutput     `json:"note,omitempty"`
	PushData       *ProjectEventPushDataOutput `json:"push_data,omitempty"`
}

// ListProjectEventsOutput holds a paginated list of project events.
type ListProjectEventsOutput struct {
	toolutil.HintableOutput
	Events     []ProjectEventOutput      `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// Handlers.

// ListProjectEvents returns a paginated list of visible events for a project.
func ListProjectEvents(ctx context.Context, client *gitlabclient.Client, input ListProjectEventsInput) (ListProjectEventsOutput, error) {
	if input.ProjectID == "" {
		return ListProjectEventsOutput{}, toolutil.WrapErrWithMessage("project_event_list", toolutil.ErrFieldRequired("project_id"))
	}

	opts := &gl.ListProjectVisibleEventsOptions{}
	filters := newEventFilters(input.Action, input.TargetType, input.Before, input.After, input.Sort)
	opts.Action = filters.Action
	opts.TargetType = filters.TargetType
	opts.Before = filters.Before
	opts.After = filters.After
	opts.Sort = filters.Sort
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)

	events, resp, err := client.GL().Events.ListProjectVisibleEvents(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProjectEventsOutput{}, toolutil.WrapErrWithStatusHint("project_event_list", err, http.StatusNotFound, "verify project_id with gitlab_project_get")
	}

	out := ListProjectEventsOutput{
		Events:     make([]ProjectEventOutput, 0, len(events)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, e := range events {
		out.Events = append(out.Events, toProjectEventOutput(e))
	}

	enrichProjectEventURLs(ctx, client, out.Events)

	return out, nil
}

// Converters.

// toProjectEventOutput converts the GitLab API response to the tool output format.
func toProjectEventOutput(e *gl.ProjectEvent) ProjectEventOutput {
	return ProjectEventOutput{
		ID:             e.ID,
		Title:          e.Title,
		ProjectID:      e.ProjectID,
		ActionName:     e.ActionName,
		TargetID:       e.TargetID,
		TargetIID:      e.TargetIID,
		TargetType:     e.TargetType,
		AuthorID:       e.AuthorID,
		TargetTitle:    e.TargetTitle,
		CreatedAt:      e.CreatedAt,
		Author:         toBasicUserOutput(e.Author),
		AuthorUsername: e.AuthorUsername,
		Data:           toProjectEventDataOutput(e.Data),
		Note:           toProjectEventNoteOutput(e.Note),
		PushData:       toProjectPushDataOutput(e.PushData),
	}
}

// toProjectEventDataOutput mirrors a non-empty ProjectEventData.
func toProjectEventDataOutput(d gl.ProjectEventData) *ProjectEventDataOutput {
	if d.Before == "" && d.After == "" && d.Ref == "" && d.UserID == 0 && d.UserName == "" &&
		d.Repository == nil && len(d.Commits) == 0 && d.TotalCommitsCount == 0 {
		return nil
	}
	out := &ProjectEventDataOutput{
		Before:            d.Before,
		After:             d.After,
		Ref:               d.Ref,
		UserID:            d.UserID,
		UserName:          d.UserName,
		Repository:        toRepositoryOutput(d.Repository),
		TotalCommitsCount: d.TotalCommitsCount,
	}
	if len(d.Commits) > 0 {
		out.Commits = make([]CommitOutput, 0, len(d.Commits))
		for _, c := range d.Commits {
			if c == nil {
				continue
			}
			out.Commits = append(out.Commits, toCommitOutput(c))
		}
	}
	return out
}

// toRepositoryOutput mirrors a gitlab.Repository, returning nil when absent.
func toRepositoryOutput(r *gl.Repository) *RepositoryOutput {
	if r == nil {
		return nil
	}
	return &RepositoryOutput{
		Name:              r.Name,
		Description:       r.Description,
		WebURL:            r.WebURL,
		AvatarURL:         r.AvatarURL,
		GitSSHURL:         r.GitSSHURL,
		GitHTTPURL:        r.GitHTTPURL,
		Namespace:         r.Namespace,
		Visibility:        string(r.Visibility),
		PathWithNamespace: r.PathWithNamespace,
		DefaultBranch:     r.DefaultBranch,
		Homepage:          r.Homepage,
		URL:               r.URL,
		SSHURL:            r.SSHURL,
		HTTPURL:           r.HTTPURL,
	}
}

// toCommitOutput mirrors a gitlab.Commit.
func toCommitOutput(c *gl.Commit) CommitOutput {
	out := CommitOutput{
		ID:               c.ID,
		ShortID:          c.ShortID,
		Title:            c.Title,
		AuthorName:       c.AuthorName,
		AuthorEmail:      c.AuthorEmail,
		CommitterName:    c.CommitterName,
		CommitterEmail:   c.CommitterEmail,
		Message:          c.Message,
		ParentIDs:        c.ParentIDs,
		ProjectID:        c.ProjectID,
		Trailers:         c.Trailers,
		ExtendedTrailers: c.ExtendedTrailers,
		WebURL:           c.WebURL,
		Stats:            toCommitStatsOutput(c.Stats),
		LastPipeline:     toPipelineInfoOutput(c.LastPipeline),
	}
	if c.AuthoredDate != nil {
		out.AuthoredDate = c.AuthoredDate.Format(time.RFC3339)
	}
	if c.CommittedDate != nil {
		out.CommittedDate = c.CommittedDate.Format(time.RFC3339)
	}
	if c.CreatedAt != nil {
		out.CreatedAt = c.CreatedAt.Format(time.RFC3339)
	}
	if c.Status != nil {
		out.Status = string(*c.Status)
	}
	return out
}

// toCommitStatsOutput mirrors a gitlab.CommitStats, returning nil when absent.
func toCommitStatsOutput(s *gl.CommitStats) *CommitStatsOutput {
	if s == nil {
		return nil
	}
	return &CommitStatsOutput{Additions: s.Additions, Deletions: s.Deletions, Total: s.Total}
}

// toPipelineInfoOutput mirrors a gitlab.PipelineInfo, returning nil when absent.
func toPipelineInfoOutput(p *gl.PipelineInfo) *PipelineInfoOutput {
	if p == nil {
		return nil
	}
	out := &PipelineInfoOutput{
		ID:        p.ID,
		IID:       p.IID,
		ProjectID: p.ProjectID,
		Status:    p.Status,
		Source:    p.Source,
		Ref:       p.Ref,
		SHA:       p.SHA,
		Name:      p.Name,
		WebURL:    p.WebURL,
	}
	if p.UpdatedAt != nil {
		out.UpdatedAt = p.UpdatedAt.Format(time.RFC3339)
	}
	if p.CreatedAt != nil {
		out.CreatedAt = p.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// toProjectEventNoteOutput mirrors a non-empty ProjectEventNote.
func toProjectEventNoteOutput(n gl.ProjectEventNote) *ProjectEventNoteOutput {
	if n.ID == 0 && n.Body == "" && n.Attachment == "" && n.Author == (gl.ProjectEventNoteAuthor{}) &&
		n.CreatedAt == nil && !n.System && n.NoteableID == 0 && n.NoteableType == "" && n.NoteableIID == 0 {
		return nil
	}
	out := &ProjectEventNoteOutput{
		ID:           n.ID,
		Body:         n.Body,
		Attachment:   n.Attachment,
		Author:       toProjectEventNoteAuthorOutput(n.Author),
		System:       n.System,
		NoteableID:   n.NoteableID,
		NoteableType: n.NoteableType,
		NoteableIID:  n.NoteableIID,
	}
	if n.CreatedAt != nil {
		out.CreatedAt = n.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// toProjectEventNoteAuthorOutput mirrors a gitlab.ProjectEventNoteAuthor.
func toProjectEventNoteAuthorOutput(a gl.ProjectEventNoteAuthor) *NoteAuthorOutput {
	return noteAuthorOutput(a.ID, a.Username, a.Email, a.Name, a.State, a.AvatarURL, a.WebURL)
}

// toProjectPushDataOutput mirrors a non-empty ProjectEventPushData.
func toProjectPushDataOutput(d gl.ProjectEventPushData) *ProjectEventPushDataOutput {
	if d == (gl.ProjectEventPushData{}) {
		return nil
	}
	return &ProjectEventPushDataOutput{
		CommitCount: d.CommitCount,
		Action:      d.Action,
		RefType:     d.RefType,
		CommitFrom:  d.CommitFrom,
		CommitTo:    d.CommitTo,
		Ref:         d.Ref,
		CommitTitle: d.CommitTitle,
	}
}

type eventFilters struct {
	Action     *gl.EventTypeValue
	TargetType *gl.EventTargetTypeValue
	Before     *gl.ISOTime
	After      *gl.ISOTime
	Sort       *string
}

func newEventFilters(action, targetType, before, after, sort string) eventFilters {
	filters := eventFilters{
		Before: parseEventDate(before),
		After:  parseEventDate(after),
	}
	if action != "" {
		value := gl.EventTypeValue(action)
		filters.Action = &value
	}
	if targetType != "" {
		value := gl.EventTargetTypeValue(targetType)
		filters.TargetType = &value
	}
	if sort != "" {
		filters.Sort = &sort
	}
	return filters
}

func parseEventDate(value string) *gl.ISOTime {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(toolutil.DateFormatISO, value)
	if err != nil {
		return nil
	}
	date := gl.ISOTime(parsed)
	return &date
}

// formatAuthor returns the author username prefixed with @ for Markdown display.
func formatAuthor(username string) string {
	if username == "" {
		return ""
	}
	return "@" + username
}

// resolveProjectWebURLs fetches the web URL for each unique project ID.
// Failures are silently ignored — missing URLs simply produce no links.
func resolveProjectWebURLs(ctx context.Context, client *gitlabclient.Client, projectIDs []int64) map[int64]string {
	seen := make(map[int64]string, len(projectIDs))
	for _, id := range projectIDs {
		if _, ok := seen[id]; ok || id == 0 {
			continue
		}
		proj, _, err := client.GL().Projects.GetProject(id, &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if err != nil || proj == nil {
			seen[id] = ""
			continue
		}
		seen[id] = proj.WebURL
	}
	return seen
}

// enrichContributionEventURLs resolves project web URLs and sets TargetURL on each event.
func enrichContributionEventURLs(ctx context.Context, client *gitlabclient.Client, events []ContributionEventOutput) {
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ProjectID)
	}
	urls := resolveProjectWebURLs(ctx, client, ids)
	for i := range events {
		events[i].TargetURL = toolutil.BuildTargetURL(urls[events[i].ProjectID], events[i].TargetType, events[i].TargetIID)
	}
}

// enrichProjectEventURLs resolves project web URLs and sets TargetURL on each event.
func enrichProjectEventURLs(ctx context.Context, client *gitlabclient.Client, events []ProjectEventOutput) {
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ProjectID)
	}
	urls := resolveProjectWebURLs(ctx, client, ids)
	for i := range events {
		events[i].TargetURL = toolutil.BuildTargetURL(urls[events[i].ProjectID], events[i].TargetType, events[i].TargetIID)
	}
}
