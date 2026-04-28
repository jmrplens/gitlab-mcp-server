// Package epicdiscussions implements MCP tools for GitLab epic discussion
// operations using the Work Items GraphQL API. Discussions are threaded
// conversations on group epics, each containing one or more notes.
//
// This package was migrated from the deprecated Epics REST API (deprecated
// GitLab 17.0, removal planned 19.0) to the Work Items GraphQL API per
// ADR-0009 (progressive GraphQL migration).
package epicdiscussions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GraphQL queries and mutations for work item discussions.

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
              hasPreviousPage
              endCursor
              startCursor
            }
            nodes {
              id
              notes {
                nodes {
                  id
                  body
                  author { username }
                  system
                  createdAt
                  updatedAt
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

const queryResolveWorkItemGID = `
query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
    }
  }
}
`

const mutationCreateNote = `
mutation($noteableId: NoteableID!, $body: String!) {
  createNote(input: { noteableId: $noteableId, body: $body }) {
    note {
      id
      body
      author { username }
      system
      createdAt
      updatedAt
      discussion {
        id
      }
    }
    errors
  }
}
`

const mutationCreateNoteReply = `
mutation($noteableId: NoteableID!, $body: String!, $discussionId: DiscussionID!) {
  createNote(input: { noteableId: $noteableId, body: $body, discussionId: $discussionId }) {
    note {
      id
      body
      author { username }
      system
      createdAt
      updatedAt
    }
    errors
  }
}
`

const mutationUpdateNote = `
mutation($id: NoteID!, $body: String!) {
  updateNote(input: { id: $id, body: $body }) {
    note {
      id
      body
      author { username }
      system
      createdAt
      updatedAt
    }
    errors
  }
}
`

const mutationDestroyNote = `
mutation($id: NoteID!) {
  destroyNote(input: { id: $id }) {
    note {
      id
    }
    errors
  }
}
`

// gqlNoteNode represents a note from the GitLab GraphQL API.
type gqlNoteNode struct {
	ID         string            `json:"id"`
	Body       string            `json:"body"`
	Author     gqlNoteAuthor     `json:"author"`
	System     bool              `json:"system"`
	CreatedAt  *string           `json:"createdAt"`
	UpdatedAt  *string           `json:"updatedAt"`
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
	ID    string       `json:"id"`
	Notes gqlNoteNodes `json:"notes"`
}

// gqlDiscussionsConnection holds a paginated list of discussion nodes.
type gqlDiscussionsConnection struct {
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
	Nodes    []gqlDiscussionNode         `json:"nodes"`
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
}

// gqlCreateNotePayload is the response payload for creating a note.
type gqlCreateNotePayload struct {
	Note   *gqlNoteNode `json:"note"`
	Errors []string     `json:"errors"`
}

// gqlUpdateNotePayload is the response payload for updating a note.
type gqlUpdateNotePayload struct {
	Note   *gqlNoteNode `json:"note"`
	Errors []string     `json:"errors"`
}

// gqlDestroyNotePayload is the response payload for deleting a note.
type gqlDestroyNotePayload struct {
	Errors []string `json:"errors"`
}

// gqlResolveWorkItemID holds a resolved work item GID.
type gqlResolveWorkItemID struct {
	ID string `json:"id"`
}

// gqlResolveNamespace wraps a work item ID inside a namespace.
type gqlResolveNamespace struct {
	WorkItem *gqlResolveWorkItemID `json:"workItem"`
}

// extractDiscussionHex extracts the hex ID from a Discussion GID.
func extractDiscussionHex(gid string) string {
	if idx := strings.LastIndex(gid, "/"); idx >= 0 {
		return gid[idx+1:]
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

// nodeToNoteOutput converts a GraphQL note node to the MCP output format.
func nodeToNoteOutput(n gqlNoteNode) NoteOutput {
	out := NoteOutput{
		Body:   n.Body,
		Author: n.Author.Username,
		System: n.System,
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

// resolveWorkItemGID resolves the GraphQL GID for a work item by namespace path and IID.
func resolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	var resp struct {
		Data struct {
			Namespace *gqlResolveNamespace `json:"namespace"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryResolveWorkItemGID,
		Variables: map[string]any{
			"fullPath": fullPath,
			"iid":      strconv.FormatInt(iid, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return "", err
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return "", fmt.Errorf("epic not found in group %q with IID %d", fullPath, iid)
	}

	return resp.Data.Namespace.WorkItem.ID, nil
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
type NoteOutput struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	System    bool   `json:"system"`
}

// Output represents a discussion thread.
type Output struct {
	toolutil.HintableOutput
	ID    string       `json:"id"`
	Notes []NoteOutput `json:"notes"`
}

// ListOutput holds a list of epic discussions.
type ListOutput struct {
	toolutil.HintableOutput
	Discussions []Output                         `json:"discussions"`
	Pagination  toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// Handlers.

// List retrieves discussion threads on an epic via the Work Items GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicDiscussionList: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicDiscussionList", "epic_iid")
	}

	vars := input.GraphQLPaginationInput.Variables()
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)

	var resp gqlDiscussionsResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListDiscussions,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicDiscussionList", err,
			"verify full_path (group path) and iid (project-scoped epic IID) with gitlab_epic_list; epics are migrated to Work Items \u2014 Premium/Ultimate license required")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return ListOutput{}, fmt.Errorf("epicDiscussionList: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	var discussions []Output
	var pageInfo toolutil.GraphQLRawPageInfo
	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		pageInfo = w.Discussions.PageInfo
		for _, disc := range w.Discussions.Nodes {
			d := Output{
				ID:    extractDiscussionHex(disc.ID),
				Notes: make([]NoteOutput, 0, len(disc.Notes.Nodes)),
			}
			for _, n := range disc.Notes.Nodes {
				d.Notes = append(d.Notes, nodeToNoteOutput(n))
			}
			discussions = append(discussions, d)
		}
	}

	return ListOutput{
		Discussions: discussions,
		Pagination:  toolutil.PageInfoToOutput(pageInfo),
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
			"verify full_path + iid with gitlab_epic_list; discussion_id may be hex (e.g. abc123) or full GID; use gitlab_list_epic_discussions to enumerate existing discussions")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return Output{}, fmt.Errorf("epicDiscussionGet: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		for _, disc := range w.Discussions.Nodes {
			if disc.ID == targetGID {
				d := Output{
					ID:    extractDiscussionHex(disc.ID),
					Notes: make([]NoteOutput, 0, len(disc.Notes.Nodes)),
				}
				for _, n := range disc.Notes.Nodes {
					d.Notes = append(d.Notes, nodeToNoteOutput(n))
				}
				return d, nil
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
			"failed to resolve epic GID; verify full_path + iid with gitlab_epic_list; requires Reporter role on the group")
	}

	body := toolutil.NormalizeText(input.Body)
	var resp struct {
		Data struct {
			CreateNote gqlCreateNotePayload `json:"createNote"`
		} `json:"data"`
	}

	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationCreateNote,
		Variables: map[string]any{
			"noteableId": workItemGID,
			"body":       body,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("epicDiscussionCreate", err,
			"body is rendered as GitLab Flavored Markdown; max 1MB; check Premium/Ultimate license; createNote mutation may fail if the work item is locked or confidential without permission")
	}

	if len(resp.Data.CreateNote.Errors) > 0 {
		return Output{}, fmt.Errorf("epicDiscussionCreate: %s", resp.Data.CreateNote.Errors[0])
	}
	if resp.Data.CreateNote.Note == nil {
		return Output{}, errors.New("epicDiscussionCreate: no note returned")
	}

	note := nodeToNoteOutput(*resp.Data.CreateNote.Note)
	discussionID := ""
	if resp.Data.CreateNote.Note.Discussion != nil {
		discussionID = extractDiscussionHex(resp.Data.CreateNote.Note.Discussion.ID)
	}

	return Output{
		ID:    discussionID,
		Notes: []NoteOutput{note},
	}, nil
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
			"failed to resolve epic GID; verify full_path + iid with gitlab_epic_list; requires Reporter role")
	}

	body := toolutil.NormalizeText(input.Body)
	discussionGID := formatDiscussionGID(input.DiscussionID)

	var resp struct {
		Data struct {
			CreateNote gqlCreateNotePayload `json:"createNote"`
		} `json:"data"`
	}

	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationCreateNoteReply,
		Variables: map[string]any{
			"noteableId":   workItemGID,
			"body":         body,
			"discussionId": discussionGID,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithHint("epicDiscussionAddNote", err,
			"verify discussion_id with gitlab_list_epic_discussions; cannot reply to a system-generated discussion; body is GFM with 1MB max")
	}

	if len(resp.Data.CreateNote.Errors) > 0 {
		return NoteOutput{}, fmt.Errorf("epicDiscussionAddNote: %s", resp.Data.CreateNote.Errors[0])
	}
	if resp.Data.CreateNote.Note == nil {
		return NoteOutput{}, errors.New("epicDiscussionAddNote: no note returned")
	}

	return nodeToNoteOutput(*resp.Data.CreateNote.Note), nil
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

	body := toolutil.NormalizeText(input.Body)
	noteGID := toolutil.FormatGID("Note", input.NoteID)

	var resp struct {
		Data struct {
			UpdateNote gqlUpdateNotePayload `json:"updateNote"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationUpdateNote,
		Variables: map[string]any{
			"id":   noteGID,
			"body": body,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return NoteOutput{}, toolutil.WrapErrWithHint("epicDiscussionUpdateNote", err,
			"only the note author or a Maintainer/Owner can edit; verify note_id with gitlab_list_epic_discussions; body is GFM with 1MB max")
	}

	if len(resp.Data.UpdateNote.Errors) > 0 {
		return NoteOutput{}, fmt.Errorf("epicDiscussionUpdateNote: %s", resp.Data.UpdateNote.Errors[0])
	}
	if resp.Data.UpdateNote.Note == nil {
		return NoteOutput{}, errors.New("epicDiscussionUpdateNote: no note returned")
	}

	return nodeToNoteOutput(*resp.Data.UpdateNote.Note), nil
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

	noteGID := toolutil.FormatGID("Note", input.NoteID)

	var resp struct {
		Data struct {
			DestroyNote gqlDestroyNotePayload `json:"destroyNote"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: mutationDestroyNote,
		Variables: map[string]any{
			"id": noteGID,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithHint("epicDiscussionDeleteNote", err,
			"only the note author or a Maintainer/Owner can delete; verify note_id with gitlab_list_epic_discussions; deletion is irreversible \u2014 system-generated notes cannot be removed")
	}

	if len(resp.Data.DestroyNote.Errors) > 0 {
		return fmt.Errorf("epicDiscussionDeleteNote: %s", resp.Data.DestroyNote.Errors[0])
	}

	return nil
}
