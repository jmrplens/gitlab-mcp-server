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
	Page       int64  `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage    int64  `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// EventOutput represents a single GitLab event shared by event list tools.
type EventOutput struct {
	ID             int64  `json:"id"`
	Title          string `json:"title,omitempty"`
	ProjectID      int64  `json:"project_id"`
	ActionName     string `json:"action_name"`
	TargetID       int64  `json:"target_id,omitempty"`
	TargetIID      int64  `json:"target_iid,omitempty"`
	TargetType     string `json:"target_type,omitempty"`
	TargetURL      string `json:"target_url,omitempty"`
	AuthorID       int64  `json:"author_id"`
	TargetTitle    string `json:"target_title,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	AuthorUsername string `json:"author_username,omitempty"`
}

// ContributionEventOutput represents a single contribution event.
type ContributionEventOutput = EventOutput

// ListContributionEventsOutput holds a paginated list of contribution events.
type ListContributionEventsOutput struct {
	toolutil.HintableOutput
	Events     []ContributionEventOutput `json:"events"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListCurrentUserContributionEvents returns contribution events for the authenticated user.
func ListCurrentUserContributionEvents(ctx context.Context, client *gitlabclient.Client, input ListContributionEventsInput) (ListContributionEventsOutput, error) {
	opts := &gl.ListContributionEventsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	filters := newEventFilters(input.Action, input.TargetType, input.Before, input.After, input.Sort)
	opts.Action = filters.Action
	opts.TargetType = filters.TargetType
	opts.Before = filters.Before
	opts.After = filters.After
	opts.Sort = filters.Sort
	if input.Scope != "" {
		opts.Scope = &input.Scope
	}

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
	}
	if e.CreatedAt != nil {
		o.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	return o
}

// ListProjectEventsInput contains parameters for listing project visible events.
type ListProjectEventsInput struct {
	ProjectID  toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Action     string               `json:"action,omitempty" jsonschema:"Filter by action type (created, updated, closed, reopened, pushed, commented, merged, joined, left, destroyed, expired)"`
	TargetType string               `json:"target_type,omitempty" jsonschema:"Filter by target type (issue, milestone, merge_request, note, project, snippet, user)"`
	Before     string               `json:"before,omitempty" jsonschema:"Return events before this date (YYYY-MM-DD)"`
	After      string               `json:"after,omitempty" jsonschema:"Return events after this date (YYYY-MM-DD)"`
	Sort       string               `json:"sort,omitempty" jsonschema:"Sort order (asc or desc, default desc)"`
	Page       int64                `json:"page,omitempty" jsonschema:"Page number for pagination (default 1)"`
	PerPage    int64                `json:"per_page,omitempty" jsonschema:"Number of items per page (default 20, max 100)"`
}

// Output types.

// ProjectEventOutput represents a single project event.
type ProjectEventOutput = EventOutput

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

	opts := &gl.ListProjectVisibleEventsOptions{
		ListOptions: gl.ListOptions{
			Page:    input.Page,
			PerPage: input.PerPage,
		},
	}
	filters := newEventFilters(input.Action, input.TargetType, input.Before, input.After, input.Sort)
	opts.Action = filters.Action
	opts.TargetType = filters.TargetType
	opts.Before = filters.Before
	opts.After = filters.After
	opts.Sort = filters.Sort

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
	return EventOutput{
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
		AuthorUsername: e.AuthorUsername,
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
	enrichEventURLs(ctx, client, events)
}

// enrichProjectEventURLs resolves project web URLs and sets TargetURL on each event.
func enrichProjectEventURLs(ctx context.Context, client *gitlabclient.Client, events []ProjectEventOutput) {
	enrichEventURLs(ctx, client, events)
}

func enrichEventURLs(ctx context.Context, client *gitlabclient.Client, events []EventOutput) {
	ids := make([]int64, 0, len(events))
	for i := range events {
		ids = append(ids, events[i].ProjectID)
	}
	urls := resolveProjectWebURLs(ctx, client, ids)
	for i := range events {
		events[i].TargetURL = toolutil.BuildTargetURL(urls[events[i].ProjectID], events[i].TargetType, events[i].TargetIID)
	}
}
