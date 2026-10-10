package epicdiscussions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicworkitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GraphQL queries and mutations for work item discussions.

// noteFields are the fields every document here selects on a note, spelled
// once so the list query and the three mutations cannot select different
// notes. A person on a note is selected by username, which is the shape this
// package publishes its author in. They are a field list rather than a
// selection set in braces, because a constant that opens with a brace is a
// GraphQL document in its own right (the query shorthand), and the document
// inventory would read it as one no request carries. What the schema offers
// on a note and this leaves out is answered in
// cmd/audit_graphql_shapes/sent_declarations.go.
//
// The list query sends this selection for up to a hundred threads, and GitLab
// charges the work item discussions connection six times its contents at that
// page size (complexity_multiplier 0.05), so every field here and on the
// thread costs six in the query that get and list send. GitLab refuses a
// query above 250 from any caller but an administrator, before running it.
// Measured on GitLab.com on 2026-09-26, the list query costs 220 at
// first=100, the page every get sends; the selection issue 968 first widened
// cost exactly 250, with nothing to spare. The package's tests hold the
// figure, so a field added here is measured against that limit before it
// ships.
const noteFields = `
      id
      body
      author { username }
      system
      internal
      imported
      externalAuthor
      createdAt
      updatedAt
      lastEditedAt
      lastEditedBy { username }
      noteableId
      noteableType
      resolvable
      resolved
      resolvedAt
      resolvedBy { username }
      url`

const queryListDiscussions = `
query($fullPath: ID!, $iid: String!, $first: Int, $after: String) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      widgets {
        ... on WorkItemWidgetNotes {
          discussions(first: $first, after: $after) {
            pageInfo {
              hasNextPage
              endCursor
            }
            nodes {
              id
              resolvable
              resolved
              resolvedAt
              resolvedBy { username }
              notes {
                nodes {` + noteFields + `
                }
              }
            }
          }
        }
      }
    }
  }
}
`

// mutationCreateNote opens a thread, so it alone also selects the discussion
// GitLab opened for the note.
const mutationCreateNote = `
mutation($noteableId: NoteableID!, $body: String!) {
  createNote(input: { noteableId: $noteableId, body: $body }) {
    note {` + noteFields + `
      discussion {
        id
      }
    }
    errors
    ` + toolutil.GraphQLQuickActionsStatusSelection + `
  }
}
`

const mutationCreateNoteReply = `
mutation($noteableId: NoteableID!, $body: String!, $discussionId: DiscussionID!) {
  createNote(input: { noteableId: $noteableId, body: $body, discussionId: $discussionId }) {
    note {` + noteFields + `
    }
    errors
    ` + toolutil.GraphQLQuickActionsStatusSelection + `
  }
}
`

const mutationUpdateNote = `
mutation($id: NoteID!, $body: String!) {
  updateNote(input: { id: $id, body: $body }) {
    note {` + noteFields + `
    }
    errors
    ` + toolutil.GraphQLQuickActionsStatusSelection + `
  }
}
`

// mutationDestroyNote selects the errors alone: GitLab's destroyNote answers
// with nothing else, so a note or a status selected here is null on every
// response.
const mutationDestroyNote = `
mutation($id: NoteID!) {
  destroyNote(input: { id: $id }) {
    errors
  }
}
`

// gqlNoteNode represents a note from the GitLab GraphQL API, as every
// document here selects it.
type gqlNoteNode struct {
	ID             string         `json:"id"`
	Body           string         `json:"body"`
	Author         gqlNoteAuthor  `json:"author"`
	System         bool           `json:"system"`
	Internal       bool           `json:"internal"`
	Imported       bool           `json:"imported"`
	ExternalAuthor string         `json:"externalAuthor"`
	CreatedAt      *string        `json:"createdAt"`
	UpdatedAt      *string        `json:"updatedAt"`
	LastEditedAt   string         `json:"lastEditedAt"`
	LastEditedBy   *gqlNoteAuthor `json:"lastEditedBy"`
	NoteableID     int64          `json:"noteableId"`
	NoteableType   string         `json:"noteableType"`
	Resolvable     bool           `json:"resolvable"`
	Resolved       bool           `json:"resolved"`
	ResolvedAt     string         `json:"resolvedAt"`
	ResolvedBy     *gqlNoteAuthor `json:"resolvedBy"`
	URL            string         `json:"url"`
}

// gqlCreatedNoteNode is the note createNote answers with: the note, plus the
// discussion GitLab opened for it, which only that document selects because
// only that caller has no discussion to hand.
type gqlCreatedNoteNode struct {
	gqlNoteNode
	Discussion *gqlDiscussionRef `json:"discussion"`
}

// gqlNoteAuthor represents the author of a note.
type gqlNoteAuthor struct {
	Username string `json:"username"`
}

// gqlDiscussionRef holds a reference to a discussion by its GID.
type gqlDiscussionRef struct {
	ID string `json:"id"`
}

// gqlNoteNodes holds a list of note nodes.
type gqlNoteNodes struct {
	Nodes []gqlNoteNode `json:"nodes"`
}

// gqlDiscussionNode represents a single discussion with its notes.
type gqlDiscussionNode struct {
	ID         string         `json:"id"`
	Resolvable bool           `json:"resolvable"`
	Resolved   bool           `json:"resolved"`
	ResolvedAt string         `json:"resolvedAt"`
	ResolvedBy *gqlNoteAuthor `json:"resolvedBy"`
	Notes      gqlNoteNodes   `json:"notes"`
}

// gqlDiscussionsConnection holds a paginated list of discussion nodes.
type gqlDiscussionsConnection struct {
	PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
	Nodes    []gqlDiscussionNode                `json:"nodes"`
}

// gqlDiscussionsWidget is a work item widget containing discussions.
type gqlDiscussionsWidget struct {
	Discussions *gqlDiscussionsConnection `json:"discussions"`
}

// gqlDiscWorkItem represents a work item with discussion widgets.
type gqlDiscWorkItem struct {
	ID      string                 `json:"id"`
	Widgets []gqlDiscussionsWidget `json:"widgets"`
}

// gqlNamespaceDiscWorkItem wraps a work item inside a namespace for discussion queries.
type gqlNamespaceDiscWorkItem struct {
	WorkItem *gqlDiscWorkItem `json:"workItem"`
}

// gqlDiscussionsResponse is the response struct for work item discussion queries.
type gqlDiscussionsResponse struct {
	Data struct {
		Namespace *gqlNamespaceDiscWorkItem `json:"namespace"`
	} `json:"data"`
	Errors []toolutil.GraphQLError `json:"errors"`
}

// topLevelError reports what GitLab refused, or nil when it refused nothing.
//
// GitLab answers a rejected document with HTTP 200 and a top-level errors
// array, which client-go does not turn into an error, so a query the instance
// refused reaches a handler looking exactly like an epic that is not there.
// Both handlers that run this document need the same answer, which is why it
// lives on the envelope rather than at one call site.
func (r gqlDiscussionsResponse) topLevelError(operation string) error {
	return toolutil.GraphQLTopLevelError(operation, r.Errors)
}

// extractDiscussionHex extracts the hex ID from a Discussion GID.
func extractDiscussionHex(gid string) string {
	if _, hex, found := strings.CutLast(gid, "/"); found {
		return hex
	}
	return gid
}

// formatDiscussionGID constructs a Discussion GID from a hex ID or returns
// the input unchanged if it is already a full GID.
func formatDiscussionGID(id string) string {
	if strings.HasPrefix(id, "gid://") {
		return id
	}
	return "gid://gitlab/Discussion/" + id
}

// username returns the username of a user GitLab may leave null on a note or
// a thread, or "" when nobody holds the role.
func (a *gqlNoteAuthor) username() string {
	if a == nil {
		return ""
	}
	return a.Username
}

// nodeToNoteOutput converts a GraphQL note node to the MCP output format.
func nodeToNoteOutput(n gqlNoteNode) NoteOutput {
	out := NoteOutput{
		Body:           n.Body,
		Author:         n.Author.Username,
		System:         n.System,
		Internal:       n.Internal,
		Imported:       n.Imported,
		ExternalAuthor: n.ExternalAuthor,
		LastEditedAt:   n.LastEditedAt,
		LastEditedBy:   n.LastEditedBy.username(),
		NoteableID:     n.NoteableID,
		NoteableType:   n.NoteableType,
		Resolvable:     n.Resolvable,
		Resolved:       n.Resolved,
		ResolvedAt:     n.ResolvedAt,
		ResolvedBy:     n.ResolvedBy.username(),
		URL:            n.URL,
	}
	if _, id, err := toolutil.ParseGID(n.ID); err == nil {
		out.ID = id
	}
	if n.CreatedAt != nil {
		out.CreatedAt = *n.CreatedAt
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = *n.UpdatedAt
	}
	return out
}

// mutationToNoteOutput converts what a reply or an edit answered with into
// the note output: the note with the quick actions status beside it, or the
// status alone when the body held only quick actions and GitLab kept no note.
func mutationToNoteOutput(result toolutil.GraphQLNoteMutationResult[gqlNoteNode]) NoteOutput {
	var out NoteOutput
	if result.Note != nil {
		out = nodeToNoteOutput(*result.Note)
	}
	out.QuickActionsStatus = result.QuickActions
	return out
}

func nodeToDiscussionOutput(disc gqlDiscussionNode) Output {
	notes := make([]NoteOutput, len(disc.Notes.Nodes))
	for idx := range disc.Notes.Nodes {
		notes[idx] = nodeToNoteOutput(disc.Notes.Nodes[idx])
	}
	return Output{
		ID:         extractDiscussionHex(disc.ID),
		Resolvable: disc.Resolvable,
		Resolved:   disc.Resolved,
		ResolvedAt: disc.ResolvedAt,
		ResolvedBy: disc.ResolvedBy.username(),
		Notes:      notes,
	}
}

// resolveWorkItemGID resolves the GraphQL GID for a work item by namespace path and IID.
func resolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return epicworkitems.ResolveEpicGID(ctx, client, fullPath, iid)
}

// Input types.

// ListInput defines parameters for listing epic discussions.
type ListInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	toolutil.GraphQLPaginationInput
}

// GetInput defines parameters for getting a single epic discussion.
type GetInput struct {
	FullPath     string `json:"full_path"     jsonschema:"Full path of the group (e.g. my-group),required"`
	IID          int64  `json:"epic_iid"           jsonschema:"Epic IID within the group,required"`
	DiscussionID string `json:"discussion_id" jsonschema:"Discussion ID,required"`
}

// CreateInput defines parameters for creating an epic discussion.
type CreateInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	Body     string `json:"body"      jsonschema:"Discussion body (Markdown supported),required"`
}

// AddNoteInput defines parameters for adding a note to an epic discussion.
type AddNoteInput struct {
	FullPath     string `json:"full_path"     jsonschema:"Full path of the group (e.g. my-group),required"`
	IID          int64  `json:"epic_iid"           jsonschema:"Epic IID within the group,required"`
	DiscussionID string `json:"discussion_id" jsonschema:"Discussion ID to reply to,required"`
	Body         string `json:"body"          jsonschema:"Note body (Markdown supported),required"`
}

// UpdateNoteInput defines parameters for updating an epic discussion note.
type UpdateNoteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	NoteID   int64  `json:"note_id"   jsonschema:"Note ID to update,required"`
	Body     string `json:"body"      jsonschema:"Updated note body,required"`
}

// DeleteNoteInput defines parameters for deleting an epic discussion note.
type DeleteNoteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	NoteID   int64  `json:"note_id"   jsonschema:"Note ID to delete,required"`
}

// Output types.

// NoteOutput represents a single note within a discussion.
//
// The keys the REST note entity shares with GraphQL's Note carry the REST
// spelling (internal, imported, noteable_id, noteable_type and the resolution
// keys); a person is published by username, the shape this package's author
// has always taken. quick_actions_status is set by a reply or an edit alone,
// and only when the body carried a quick action: it is GitLab's account of
// what the commands did. A reply whose body held nothing but quick actions
// answers with the status and no note, id 0, since GitLab ran the commands
// and kept none.
type NoteOutput struct {
	toolutil.HintableOutput
	ID                 int64                              `json:"id"`
	Body               string                             `json:"body"`
	Author             string                             `json:"author"`
	CreatedAt          string                             `json:"created_at"`
	UpdatedAt          string                             `json:"updated_at,omitempty"`
	System             bool                               `json:"system"`
	Internal           bool                               `json:"internal"`
	Imported           bool                               `json:"imported"`
	ExternalAuthor     string                             `json:"external_author,omitempty"`
	LastEditedAt       string                             `json:"last_edited_at,omitempty"`
	LastEditedBy       string                             `json:"last_edited_by,omitempty"`
	NoteableID         int64                              `json:"noteable_id,omitempty"`
	NoteableType       string                             `json:"noteable_type,omitempty"`
	Resolvable         bool                               `json:"resolvable,omitempty"`
	Resolved           bool                               `json:"resolved,omitempty"`
	ResolvedAt         string                             `json:"resolved_at,omitempty"`
	ResolvedBy         string                             `json:"resolved_by,omitempty"`
	URL                string                             `json:"url,omitempty"`
	QuickActionsStatus *toolutil.QuickActionsStatusOutput `json:"quick_actions_status,omitempty"`
}

// Output represents a discussion thread.
//
// The resolution keys are the thread's own, which GitLab derives from its
// notes. The thread's creation time is its first note's, published on
// notes[0], and the id a reply names is the thread's id on an epic, so
// neither is repeated here (cmd/audit_graphql_shapes/sent_declarations.go
// says why for each).
// quick_actions_status is set by create alone, when the opening note carried
// a quick action; a body of quick actions alone opens no thread, so the
// output then carries the status, an empty id and no notes.
type Output struct {
	toolutil.HintableOutput
	ID                 string                             `json:"id"`
	Resolvable         bool                               `json:"resolvable,omitempty"`
	Resolved           bool                               `json:"resolved,omitempty"`
	ResolvedAt         string                             `json:"resolved_at,omitempty"`
	ResolvedBy         string                             `json:"resolved_by,omitempty"`
	Notes              []NoteOutput                       `json:"notes"`
	QuickActionsStatus *toolutil.QuickActionsStatusOutput `json:"quick_actions_status,omitempty"`
}

// ListOutput holds a list of epic discussions.
//
// Pagination is forward-only because the work item notes widget is: its
// discussions field accepts first and after alone. Being keyset-paginated it
// does report a previous page and a start cursor from its second page on, and
// that half is dropped here rather than passed on, since no argument on this
// tool could spend the cursor.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                                `json:"discussions"`
	Pagination  toolutil.GraphQLForwardPaginationOutput `json:"pagination"`
}

// Handlers.

// List retrieves discussion threads on an epic via the Work Items GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicDiscussionList: full_path is required. Use group.list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicDiscussionList", "epic_iid")
	}
	return listWith(ctx, client, queryListDiscussions, input)
}

// listWith runs one discussions document against the variables the input
// resolves to.
//
// The document is a parameter rather than read from the package constant so
// that a test can hand it one declaring too little and prove the pagination
// guard refuses it. The alternative, a package-level variable a test reassigns,
// would put a document under a parallel neighbor's feet, and the race detector
// would report it as a data race rather than as this guard.
func listWith(ctx context.Context, client *gitlabclient.Client, query string, input ListInput) (ListOutput, error) {
	vars, err := input.Variables(query)
	if err != nil {
		return ListOutput{}, fmt.Errorf("epicDiscussionList: %w", err)
	}
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)

	var resp gqlDiscussionsResponse
	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicDiscussionList", err,
			"verify full_path (group path) and iid (project-scoped epic IID) with group.epic_list; epics are migrated to Work Items. Premium/Ultimate license required")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		if graphQLErr := resp.topLevelError("epicDiscussionList"); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
		return ListOutput{}, fmt.Errorf("epicDiscussionList: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	var discussions []Output
	var pageInfo toolutil.GraphQLRawForwardPageInfo
	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		pageInfo = w.Discussions.PageInfo
		for _, disc := range w.Discussions.Nodes {
			discussions = append(discussions, nodeToDiscussionOutput(disc))
		}
	}

	return ListOutput{
		Discussions: discussions,
		Pagination:  toolutil.ForwardPageInfoToOutput(pageInfo),
	}, nil
}

// Get retrieves a single discussion thread by querying the notes widget
// and matching by discussion ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicDiscussionGet: full_path is required")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicDiscussionGet", "epic_iid")
	}
	if input.DiscussionID == "" {
		return Output{}, errors.New("epicDiscussionGet: discussion_id is required")
	}

	targetGID := formatDiscussionGID(input.DiscussionID)

	var resp gqlDiscussionsResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryListDiscussions,
		Variables: map[string]any{
			"fullPath": input.FullPath,
			"iid":      strconv.FormatInt(input.IID, 10),
			"first":    toolutil.GraphQLMaxFirst,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("epicDiscussionGet", err,
			"verify full_path + iid with group.epic_list; discussion_id may be hex (e.g. abc123) or full GID; use group.epic_discussion_list to enumerate existing discussions")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		if graphQLErr := resp.topLevelError("epicDiscussionGet"); graphQLErr != nil {
			return Output{}, graphQLErr
		}
		return Output{}, fmt.Errorf("epicDiscussionGet: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		for _, disc := range w.Discussions.Nodes {
			if disc.ID == targetGID {
				return nodeToDiscussionOutput(disc), nil
			}
		}
	}

	return Output{}, fmt.Errorf("epicDiscussionGet: discussion %q not found on epic &%d in group %q", input.DiscussionID, input.IID, input.FullPath)
}

// Create starts a new discussion thread on an epic via the createNote
// GraphQL mutation.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicDiscussionCreate: full_path is required")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicDiscussionCreate", "epic_iid")
	}
	if input.Body == "" {
		return Output{}, errors.New("epicDiscussionCreate: body is required")
	}

	workItemGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("epicDiscussionCreate", err,
			"failed to resolve epic GID; verify full_path + iid with group.epic_list; requires Reporter role on the group")
	}

	created, err := toolutil.ExecGraphQLNoteMutation[gqlCreatedNoteNode](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "epicDiscussionCreate",
		Hint:       "body is rendered as GitLab Flavored Markdown; max 1MB; check Premium/Ultimate license; createNote mutation may fail if the work item is locked or confidential without permission",
		PayloadKey: "createNote",
		Query:      mutationCreateNote,
		Variables: map[string]any{
			"noteableId": workItemGID,
			"body":       toolutil.NormalizeText(input.Body),
		},
	})
	if err != nil {
		return Output{}, err
	}

	return createdToDiscussionOutput(created), nil
}

// createdToDiscussionOutput converts what the thread-opening createNote
// answered with into the thread output: the thread GitLab opened with its
// first note, and the quick actions status when the body carried a quick
// action. A body of quick actions alone opens no thread, so the output is the
// status with no id and no notes.
func createdToDiscussionOutput(created toolutil.GraphQLNoteMutationResult[gqlCreatedNoteNode]) Output {
	out := Output{Notes: []NoteOutput{}, QuickActionsStatus: created.QuickActions}
	if created.Note == nil {
		return out
	}
	out.Notes = []NoteOutput{nodeToNoteOutput(created.Note.gqlNoteNode)}
	if created.Note.Discussion != nil {
		out.ID = extractDiscussionHex(created.Note.Discussion.ID)
	}
	return out
}

// AddNote adds a reply note to an existing discussion thread via the
// createNote GraphQL mutation with a discussionId.
func AddNote(ctx context.Context, client *gitlabclient.Client, input AddNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.FullPath == "" {
		return NoteOutput{}, errors.New("epicDiscussionAddNote: full_path is required")
	}
	if input.IID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epicDiscussionAddNote", "epic_iid")
	}
	if input.DiscussionID == "" {
		return NoteOutput{}, errors.New("epicDiscussionAddNote: discussion_id is required")
	}
	if input.Body == "" {
		return NoteOutput{}, errors.New("epicDiscussionAddNote: body is required")
	}

	workItemGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithHint("epicDiscussionAddNote", err,
			"failed to resolve epic GID; verify full_path + iid with group.epic_list; requires Reporter role")
	}

	result, err := toolutil.ExecGraphQLNoteMutation[gqlNoteNode](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "epicDiscussionAddNote",
		Hint:       "verify discussion_id with group.epic_discussion_list; cannot reply to a system-generated discussion; body is GFM with 1MB max",
		PayloadKey: "createNote",
		Query:      mutationCreateNoteReply,
		Variables: map[string]any{
			"noteableId":   workItemGID,
			"body":         toolutil.NormalizeText(input.Body),
			"discussionId": formatDiscussionGID(input.DiscussionID),
		},
	})
	if err != nil {
		return NoteOutput{}, err
	}

	return mutationToNoteOutput(result), nil
}

// UpdateNote updates an existing epic discussion note via the updateNote
// GraphQL mutation.
func UpdateNote(ctx context.Context, client *gitlabclient.Client, input UpdateNoteInput) (NoteOutput, error) {
	if err := ctx.Err(); err != nil {
		return NoteOutput{}, err
	}
	if input.FullPath == "" {
		return NoteOutput{}, errors.New("epicDiscussionUpdateNote: full_path is required")
	}
	if input.IID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epicDiscussionUpdateNote", "epic_iid")
	}
	if input.NoteID <= 0 {
		return NoteOutput{}, toolutil.ErrRequiredInt64("epicDiscussionUpdateNote", "note_id")
	}
	if input.Body == "" {
		return NoteOutput{}, errors.New("epicDiscussionUpdateNote: body is required")
	}

	result, err := toolutil.ExecGraphQLNoteMutation[gqlNoteNode](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "epicDiscussionUpdateNote",
		Hint:       "only the note author or a Maintainer/Owner can edit; verify note_id with group.epic_discussion_list; body is GFM with 1MB max",
		PayloadKey: "updateNote",
		Query:      mutationUpdateNote,
		Variables: map[string]any{
			"id":   toolutil.FormatGID("Note", input.NoteID),
			"body": toolutil.NormalizeText(input.Body),
		},
		Authority: client.Authority(),
	})
	if err != nil {
		return NoteOutput{}, err
	}

	return mutationToNoteOutput(result), nil
}

// DeleteNote deletes an epic discussion note via the destroyNote GraphQL mutation.
func DeleteNote(ctx context.Context, client *gitlabclient.Client, input DeleteNoteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.FullPath == "" {
		return errors.New("epicDiscussionDeleteNote: full_path is required")
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("epicDiscussionDeleteNote", "epic_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("epicDiscussionDeleteNote", "note_id")
	}

	return toolutil.ExecGraphQLDestroyNote(ctx, client.GL().GraphQL, "epicDiscussionDeleteNote",
		"only the note author or a Maintainer/Owner can delete; verify note_id with group.epic_discussion_list; deletion is irreversible. System-generated notes cannot be removed",
		mutationDestroyNote, toolutil.FormatGID("Note", input.NoteID))
}
