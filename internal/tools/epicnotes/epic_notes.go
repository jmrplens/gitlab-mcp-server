// Package epicnotes implements GitLab epic note (comment) operations using
// the Work Items GraphQL API. Notes are comments attached to group epics
// and may be system-generated or user-created.
//
// This package was migrated from the deprecated Epics REST API (deprecated
// GitLab 17.0, removal planned 19.0) to the Work Items GraphQL API per
// ADR-0009 (progressive GraphQL migration).
package epicnotes

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// GraphQL queries and mutations for work item notes.

const queryListWorkItemNotes = `
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
	ID        string        `json:"id"`
	Body      string        `json:"body"`
	Author    gqlNoteAuthor `json:"author"`
	System    bool          `json:"system"`
	CreatedAt *string       `json:"createdAt"`
	UpdatedAt *string       `json:"updatedAt"`
}

// gqlNoteAuthor represents the author of a note.
type gqlNoteAuthor struct {
	Username string `json:"username"`
}

// gqlNoteNodes holds a list of note nodes.
type gqlNoteNodes struct {
	Nodes []gqlNoteNode `json:"nodes"`
}

// gqlDiscussionNoteNodes holds the notes within a discussion.
type gqlDiscussionNoteNodes struct {
	Notes gqlNoteNodes `json:"notes"`
}

// gqlDiscussionsConnection holds a paginated list of discussion nodes.
type gqlDiscussionsConnection struct {
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
	Nodes    []gqlDiscussionNoteNodes    `json:"nodes"`
}

// gqlDiscussionsWidget is a work item widget containing discussions.
type gqlDiscussionsWidget struct {
	Discussions *gqlDiscussionsConnection `json:"discussions"`
}

// gqlNotesWorkItem represents a work item with discussions widgets.
type gqlNotesWorkItem struct {
	ID      string                 `json:"id"`
	Widgets []gqlDiscussionsWidget `json:"widgets"`
}

// gqlNamespaceNotesWorkItem wraps a work item inside a namespace for notes queries.
type gqlNamespaceNotesWorkItem struct {
	WorkItem *gqlNotesWorkItem `json:"workItem"`
}

// gqlNotesResponse is the common response struct for work item notes queries.
type gqlNotesResponse struct {
	Data struct {
		Namespace *gqlNamespaceNotesWorkItem `json:"namespace"`
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

// nodeToOutput converts a GraphQL note node to the MCP output format.
func nodeToOutput(n gqlNoteNode) Output {
	out := Output{
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

// gqlResolveWorkItemID holds a resolved work item GID.
type gqlResolveWorkItemID struct {
	ID string `json:"id"`
}

// gqlResolveNamespace wraps a work item ID inside a namespace.
type gqlResolveNamespace struct {
	WorkItem *gqlResolveWorkItemID `json:"workItem"`
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

// ListInput defines parameters for listing epic notes.
type ListInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group or my-group/sub-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	toolutil.GraphQLPaginationInput
}

// GetInput defines parameters for getting a single epic note.
type GetInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	NoteID   int64  `json:"note_id"   jsonschema:"ID of the note to retrieve,required"`
}

// CreateInput defines parameters for creating a note on an epic.
type CreateInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	Body     string `json:"body"      jsonschema:"Note body (Markdown supported),required"`
}

// UpdateInput defines parameters for updating an epic note.
type UpdateInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	NoteID   int64  `json:"note_id"   jsonschema:"ID of the note to update,required"`
	Body     string `json:"body"      jsonschema:"Updated note body (Markdown supported),required"`
}

// DeleteInput defines parameters for deleting an epic note.
type DeleteInput struct {
	FullPath string `json:"full_path" jsonschema:"Full path of the group (e.g. my-group),required"`
	IID      int64  `json:"epic_iid"       jsonschema:"Epic IID within the group,required"`
	NoteID   int64  `json:"note_id"   jsonschema:"ID of the note to delete,required"`
}

// Output represents a note (comment) on an epic.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Body      string `json:"body"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	System    bool   `json:"system"`
}

// ListOutput holds a paginated list of epic notes.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                         `json:"notes"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves notes on an epic via the Work Items GraphQL API.
// Notes are extracted from all discussions in the notes widget.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicNoteList: full_path is required. Use gitlab_group_list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicNoteList", "epic_iid")
	}

	vars := input.GraphQLPaginationInput.Variables()
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)

	var resp gqlNotesResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListWorkItemNotes,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("epicNoteList", err,
			"verify full_path (group path) and iid (epic IID) with gitlab_epic_list; epics are migrated to Work Items \u2014 Premium/Ultimate license required")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return ListOutput{}, fmt.Errorf("epicNoteList: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	var notes []Output
	var pageInfo toolutil.GraphQLRawPageInfo
	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		pageInfo = w.Discussions.PageInfo
		for _, disc := range w.Discussions.Nodes {
			for _, n := range disc.Notes.Nodes {
				notes = append(notes, nodeToOutput(n))
			}
		}
	}

	return ListOutput{
		Notes:      notes,
		Pagination: toolutil.PageInfoToOutput(pageInfo),
	}, nil
}

// Get retrieves a single note on an epic by querying the notes widget
// and matching by note ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicNoteGet: full_path is required")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteGet", "epic_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteGet", "note_id")
	}

	targetGID := toolutil.FormatGID("Note", input.NoteID)

	var resp gqlNotesResponse
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryListWorkItemNotes,
		Variables: map[string]any{
			"fullPath": input.FullPath,
			"iid":      strconv.FormatInt(input.IID, 10),
			"first":    toolutil.GraphQLMaxFirst,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("epicNoteGet", err,
			"verify full_path + iid with gitlab_epic_list; verify note_id (numeric) with gitlab_epic_note_list; system-generated notes may have restricted access")
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return Output{}, fmt.Errorf("epicNoteGet: epic not found in group %q with IID %d", input.FullPath, input.IID)
	}

	for _, w := range resp.Data.Namespace.WorkItem.Widgets {
		if w.Discussions == nil {
			continue
		}
		for _, disc := range w.Discussions.Nodes {
			for _, n := range disc.Notes.Nodes {
				if n.ID == targetGID {
					return nodeToOutput(n), nil
				}
			}
		}
	}

	return Output{}, fmt.Errorf("epicNoteGet: note %d not found on epic &%d in group %q", input.NoteID, input.IID, input.FullPath)
}

// Create adds a new note to an epic via the createNote GraphQL mutation.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicNoteCreate: full_path is required")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteCreate", "epic_iid")
	}
	if input.Body == "" {
		return Output{}, errors.New("epicNoteCreate: body is required")
	}

	workItemGID, err := resolveWorkItemGID(ctx, client, input.FullPath, input.IID)
	if err != nil {
		return Output{}, toolutil.WrapErrWithHint("epicNoteCreate", err,
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
		return Output{}, toolutil.WrapErrWithHint("epicNoteCreate", err,
			"body is rendered as GitLab Flavored Markdown; max 1MB; check Premium/Ultimate license; createNote mutation may fail if work item is locked or confidential")
	}

	if len(resp.Data.CreateNote.Errors) > 0 {
		return Output{}, fmt.Errorf("epicNoteCreate: %s", resp.Data.CreateNote.Errors[0])
	}
	if resp.Data.CreateNote.Note == nil {
		return Output{}, errors.New("epicNoteCreate: no note returned")
	}

	return nodeToOutput(*resp.Data.CreateNote.Note), nil
}

// Update modifies the body of an existing epic note via the updateNote
// GraphQL mutation.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.FullPath == "" {
		return Output{}, errors.New("epicNoteUpdate: full_path is required")
	}
	if input.IID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteUpdate", "epic_iid")
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteUpdate", "note_id")
	}
	if input.Body == "" {
		return Output{}, errors.New("epicNoteUpdate: body is required")
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
		return Output{}, toolutil.WrapErrWithHint("epicNoteUpdate", err,
			"only the note author or a Maintainer/Owner can edit; verify note_id with gitlab_epic_note_list; body is GFM with 1MB max; system notes cannot be edited")
	}

	if len(resp.Data.UpdateNote.Errors) > 0 {
		return Output{}, fmt.Errorf("epicNoteUpdate: %s", resp.Data.UpdateNote.Errors[0])
	}
	if resp.Data.UpdateNote.Note == nil {
		return Output{}, errors.New("epicNoteUpdate: no note returned")
	}

	return nodeToOutput(*resp.Data.UpdateNote.Note), nil
}

// Delete removes a note from an epic via the destroyNote GraphQL mutation.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.FullPath == "" {
		return errors.New("epicNoteDelete: full_path is required")
	}
	if input.IID <= 0 {
		return toolutil.ErrRequiredInt64("epicNoteDelete", "epic_iid")
	}
	if input.NoteID <= 0 {
		return toolutil.ErrRequiredInt64("epicNoteDelete", "note_id")
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
		return toolutil.WrapErrWithHint("epicNoteDelete", err,
			"only the note author or a Maintainer/Owner can delete; verify note_id with gitlab_epic_note_list; deletion is irreversible \u2014 system-generated notes cannot be removed")
	}

	if len(resp.Data.DestroyNote.Errors) > 0 {
		return fmt.Errorf("epicNoteDelete: %s", resp.Data.DestroyNote.Errors[0])
	}

	return nil
}
