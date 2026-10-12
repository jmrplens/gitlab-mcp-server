package epicnotes

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/epicworkitems"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GraphQL queries and mutations for work item notes.

// noteAuthorFields are the fields a note's author is selected with, the full
// object the canonical author key has always carried.
const noteAuthorFields = "id name username webUrl avatarUrl"

// noteUserRefFields are the fields whoever last edited a note and whoever
// resolved it are selected with: enough to name the person, and three fields
// fewer than the author, which is what keeps the list query under GitLab's
// complexity limit (see [noteFields]).
const noteUserRefFields = "id username"

// noteFields are the fields of the note every document here decodes into
// [gqlNoteNode], spelled once so the list query and the two mutations cannot
// select different notes. They are a field list rather than a selection set
// in braces, because a constant that opens with a brace is a GraphQL document
// in its own right (the query shorthand), and the document inventory would
// read it as one no request carries. What the schema offers on a note and this
// leaves out is answered in cmd/audit_graphql_shapes/sent_declarations.go.
//
// The list query sends this selection for up to a hundred threads, and GitLab
// charges the work item discussions connection six times its contents at that
// page size (complexity_multiplier 0.05), so every field here costs six in the
// query that get and list send. GitLab refuses a query above 250 from any
// caller but an administrator, before running it. Measured on GitLab.com on
// 2026-09-26, the list query costs 220 at first=100, the page every get sends;
// the selection issue 968 first widened cost 274 and was refused on every
// call. The package's tests hold the figure, so a field added here is measured
// against that limit before it ships.
const noteFields = `
      id
      body
      author { ` + noteAuthorFields + ` }
      system
      internal
      imported
      externalAuthor
      createdAt
      updatedAt
      lastEditedAt
      lastEditedBy { ` + noteUserRefFields + ` }
      noteableId
      noteableType
      resolvable
      resolved
      resolvedAt
      resolvedBy { ` + noteUserRefFields + ` }
      url`

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
              endCursor
            }
            nodes {
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

const mutationCreateNote = `
mutation($noteableId: NoteableID!, $body: String!) {
  createNote(input: { noteableId: $noteableId, body: $body }) {
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

// gqlNoteNode represents a note from the GitLab GraphQL API, as [noteFields]
// selects it.
type gqlNoteNode struct {
	ID             string          `json:"id"`
	Body           string          `json:"body"`
	Author         gqlNoteAuthor   `json:"author"`
	System         bool            `json:"system"`
	Internal       bool            `json:"internal"`
	Imported       bool            `json:"imported"`
	ExternalAuthor string          `json:"externalAuthor"`
	CreatedAt      *string         `json:"createdAt"`
	UpdatedAt      *string         `json:"updatedAt"`
	LastEditedAt   string          `json:"lastEditedAt"`
	LastEditedBy   *gqlNoteUserRef `json:"lastEditedBy"`
	NoteableID     int64           `json:"noteableId"`
	NoteableType   string          `json:"noteableType"`
	Resolvable     bool            `json:"resolvable"`
	Resolved       bool            `json:"resolved"`
	ResolvedAt     string          `json:"resolvedAt"`
	ResolvedBy     *gqlNoteUserRef `json:"resolvedBy"`
	URL            string          `json:"url"`
}

// gqlNoteAuthor represents the author of a note as selected from the GraphQL
// User type. Per the 1:1 audit policy every selected field is surfaced on the
// canonical author output object.
type gqlNoteAuthor struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	WebURL    string `json:"webUrl"`
	AvatarURL string `json:"avatarUrl"`
}

// gqlNoteUserRef represents whoever last edited a note or resolved it, as
// [noteUserRefFields] selects them.
type gqlNoteUserRef struct {
	ID       string `json:"id"`
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
	PageInfo toolutil.GraphQLRawForwardPageInfo `json:"pageInfo"`
	Nodes    []gqlDiscussionNoteNodes           `json:"nodes"`
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
	Errors []toolutil.GraphQLError `json:"errors"`
}

// notesRead is how a read of the epic's notes widget reports a failure: under
// op, a request that failed with hint naming what to verify, and an answer
// carrying no work item with the epic the caller named by fullPath and iid.
type notesRead struct {
	op, hint string
	fullPath string
	iid      int64
}

// queryNotes sends query, a document reading the epic's notes widget, with
// vars and answers with the epic's work item, reporting a failure as read
// says and an answer carrying no work item as
// [epicworkitems.EpicNotAnswered] words it. The list and the read both run
// the notes document through it.
func queryNotes(ctx context.Context, client *gitlabclient.Client, query string, vars map[string]any, read notesRead) (*gqlNotesWorkItem, error) {
	var resp gqlNotesResponse
	if _, err := client.GL().GraphQL.Do(gl.GraphQLQuery{Query: query, Variables: vars}, &resp, gl.WithContext(ctx)); err != nil {
		return nil, toolutil.WrapErrWithHint(read.op, err, read.hint)
	}
	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return nil, epicworkitems.EpicNotAnswered(read.op, resp.Errors, read.fullPath, read.iid)
	}
	return resp.Data.Namespace.WorkItem, nil
}

// nodeToOutput converts a GraphQL note node to the MCP output format. Per the
// locked canonical-key convention it surfaces the full author object on the
// canonical `author` key.
func nodeToOutput(n gqlNoteNode) Output {
	out := Output{
		Body:           n.Body,
		Author:         noteAuthorOutput(n.Author),
		System:         n.System,
		Internal:       n.Internal,
		Imported:       n.Imported,
		ExternalAuthor: n.ExternalAuthor,
		LastEditedAt:   n.LastEditedAt,
		LastEditedBy:   optionalNoteUserOutput(n.LastEditedBy),
		NoteableID:     n.NoteableID,
		NoteableType:   n.NoteableType,
		Resolvable:     n.Resolvable,
		Resolved:       n.Resolved,
		ResolvedAt:     n.ResolvedAt,
		ResolvedBy:     optionalNoteUserOutput(n.ResolvedBy),
		URL:            n.URL,
		GID:            n.ID,
	}
	out.ID, out.ResourceEvent = toolutil.NoteIDFromGID(n.ID)
	if n.CreatedAt != nil {
		out.CreatedAt = *n.CreatedAt
	}
	if n.UpdatedAt != nil {
		out.UpdatedAt = *n.UpdatedAt
	}
	return out
}

// mutationToOutput converts what a createNote or updateNote mutation answered
// with into the output: the note with the quick actions status beside it, or
// the status alone when the body held only quick actions and GitLab kept no
// note.
func mutationToOutput(result toolutil.GraphQLNoteMutationResult[gqlNoteNode]) Output {
	var out Output
	if result.Note != nil {
		out = nodeToOutput(*result.Note)
	}
	out.QuickActionsStatus = result.QuickActions
	return out
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

// Output represents a note (comment) on an epic. Per the locked canonical-key
// convention the full *NoteUserOutput author object is surfaced on the canonical
// `author` key. Only fields the Work Items GraphQL notes widget actually returns
// are populated (no invented output scalars per the 1:1 audit policy).
//
// The keys the REST note entity shares with GraphQL's Note carry the REST
// spelling (internal, imported, noteable_id, noteable_type and the resolution
// keys), so an epic note reads like every other note this server answers
// with. last_edited_by and resolved_by are the author's object carrying the
// id and the username alone, which is what the list query can afford to
// select for a hundred threads. quick_actions_status is set by create and
// update alone, and only when the body carried a quick action: it is GitLab's
// account of what the commands did. A create whose body held nothing but
// quick actions answers with the status and no note, id 0, since GitLab ran
// the commands and kept none.
//
// gid is the note's GraphQL global ID as GitLab answered it, on every note.
// It is the only identifier of a note GitLab synthesizes from a resource
// event (a label, state, milestone, iteration or weight change), whose global
// ID ends in a hexadecimal discussion ID rather than a number: such a note
// carries id 0 and resource_event true, and no note action can address it.
type Output struct {
	toolutil.HintableOutput
	ID                 int64                              `json:"id"`
	GID                string                             `json:"gid,omitempty"`
	ResourceEvent      bool                               `json:"resource_event,omitempty"`
	Body               string                             `json:"body"`
	Author             *NoteUserOutput                    `json:"author,omitempty"`
	CreatedAt          string                             `json:"created_at"`
	UpdatedAt          string                             `json:"updated_at,omitempty"`
	System             bool                               `json:"system"`
	Internal           bool                               `json:"internal"`
	Imported           bool                               `json:"imported"`
	ExternalAuthor     string                             `json:"external_author,omitempty"`
	LastEditedAt       string                             `json:"last_edited_at,omitempty"`
	LastEditedBy       *NoteUserOutput                    `json:"last_edited_by,omitempty"`
	NoteableID         int64                              `json:"noteable_id,omitempty"`
	NoteableType       string                             `json:"noteable_type,omitempty"`
	Resolvable         bool                               `json:"resolvable,omitempty"`
	Resolved           bool                               `json:"resolved,omitempty"`
	ResolvedAt         string                             `json:"resolved_at,omitempty"`
	ResolvedBy         *NoteUserOutput                    `json:"resolved_by,omitempty"`
	URL                string                             `json:"url,omitempty"`
	QuickActionsStatus *toolutil.QuickActionsStatusOutput `json:"quick_actions_status,omitempty"`
}

// ListOutput holds a paginated list of epic notes.
//
// Pagination is forward-only because the work item notes widget is: its
// discussions field accepts first and after alone. Being keyset-paginated it
// does report a previous page and a start cursor from its second page on, and
// that half is dropped here rather than passed on, since no argument on this
// tool could spend the cursor.
type ListOutput struct {
	toolutil.HintableOutput
	Notes      []Output                                `json:"notes"`
	Pagination toolutil.GraphQLForwardPaginationOutput `json:"pagination"`
}

// List retrieves notes on an epic via the Work Items GraphQL API.
// Notes are extracted from all discussions in the notes widget.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.FullPath == "" {
		return ListOutput{}, errors.New("epicNoteList: full_path is required. Use group.list to find the group path first")
	}
	if input.IID <= 0 {
		return ListOutput{}, toolutil.ErrRequiredInt64("epicNoteList", "epic_iid")
	}
	return listWith(ctx, client, queryListWorkItemNotes, input)
}

// listWith runs one notes document against the variables the input resolves to.
//
// The document is a parameter rather than read from the package constant so
// that a test can hand it one declaring too little and prove the pagination
// guard refuses it. The alternative, a package-level variable a test reassigns,
// would put a document under a parallel neighbor's feet, and the race detector
// would report it as a data race rather than as this guard.
func listWith(ctx context.Context, client *gitlabclient.Client, query string, input ListInput) (ListOutput, error) {
	vars, err := input.Variables(query)
	if err != nil {
		return ListOutput{}, fmt.Errorf("epicNoteList: %w", err)
	}
	vars["fullPath"] = input.FullPath
	vars["iid"] = strconv.FormatInt(input.IID, 10)
	workItem, err := queryNotes(ctx, client, query, vars, notesRead{
		op:       "epicNoteList",
		hint:     "verify full_path (group path) and iid (epic IID) with group.epic_list; epics are migrated to Work Items. Premium/Ultimate license required",
		fullPath: input.FullPath,
		iid:      input.IID,
	})
	if err != nil {
		return ListOutput{}, err
	}

	var notes []Output
	var pageInfo toolutil.GraphQLRawForwardPageInfo
	for _, w := range workItem.Widgets {
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
		Pagination: toolutil.ForwardPageInfoToOutput(pageInfo),
	}, nil
}

// Get retrieves a single note on an epic by querying the notes widget
// and matching by note ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := epicworkitems.RequireEpic(ctx, "epicNoteGet", input.FullPath, input.IID); err != nil {
		return Output{}, err
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteGet", "note_id")
	}

	targetGID := toolutil.FormatGID("Note", input.NoteID)
	workItem, err := queryNotes(ctx, client, queryListWorkItemNotes, map[string]any{
		"fullPath": input.FullPath,
		"iid":      strconv.FormatInt(input.IID, 10),
		"first":    toolutil.GraphQLMaxFirst,
	}, notesRead{
		op:       "epicNoteGet",
		hint:     "verify full_path + iid with group.epic_list; verify note_id (numeric) with group.epic_note_list; system-generated notes may have restricted access",
		fullPath: input.FullPath,
		iid:      input.IID,
	})
	if err != nil {
		return Output{}, err
	}

	for _, w := range workItem.Widgets {
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
	workItemGID, err := epicworkitems.ResolveEpicForNote(ctx, client, "epicNoteCreate", input.FullPath, input.IID, input.Body,
		"failed to resolve epic GID; verify full_path + iid with group.epic_list; requires Reporter role on the group")
	if err != nil {
		return Output{}, err
	}

	result, err := toolutil.ExecGraphQLNoteMutation[gqlNoteNode](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "epicNoteCreate",
		Hint:       "body is rendered as GitLab Flavored Markdown; max 1MB; check Premium/Ultimate license; createNote mutation may fail if work item is locked or confidential",
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

	return mutationToOutput(result), nil
}

// Update modifies the body of an existing epic note via the updateNote
// GraphQL mutation.
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := epicworkitems.RequireEpic(ctx, "epicNoteUpdate", input.FullPath, input.IID); err != nil {
		return Output{}, err
	}
	if input.NoteID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("epicNoteUpdate", "note_id")
	}
	if input.Body == "" {
		return Output{}, errors.New("epicNoteUpdate: body is required")
	}
	if _, err := epicworkitems.CheckNoteOnEpic(ctx, client, "epicNoteUpdate", input.FullPath, input.IID, input.NoteID); err != nil {
		return Output{}, err
	}

	result, err := toolutil.ExecGraphQLNoteMutation[gqlNoteNode](ctx, client.GL().GraphQL, toolutil.GraphQLNoteMutation{
		Op:         "epicNoteUpdate",
		Hint:       "only the note author or a Maintainer/Owner can edit; verify note_id with group.epic_note_list; body is GFM with 1MB max; system notes cannot be edited",
		PayloadKey: "updateNote",
		Query:      mutationUpdateNote,
		Variables: map[string]any{
			"id":   toolutil.FormatGID("Note", input.NoteID),
			"body": toolutil.NormalizeText(input.Body),
		},
		Authority: client.Authority(),
	})
	if err != nil {
		return Output{}, err
	}

	return mutationToOutput(result), nil
}

// Delete removes a note from an epic via the destroyNote GraphQL mutation,
// once the note is known to be one of the epic's.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	_, err := deleteChecked(ctx, client, input)
	return err
}

// deleteChecked is [Delete], answering with the address GitLab served the
// note at, which the note was held to and the confirmation names.
func deleteChecked(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (string, error) {
	address, err := epicworkitems.RequireNoteOnEpic(ctx, client, "epicNoteDelete", input.FullPath, input.IID, input.NoteID)
	if err != nil {
		return "", err
	}

	return address, toolutil.ExecGraphQLDestroyNote(ctx, client.GL().GraphQL, "epicNoteDelete",
		"only the note author or a Maintainer/Owner can delete; verify note_id with group.epic_note_list; deletion is irreversible. System-generated notes cannot be removed",
		mutationDestroyNote, toolutil.FormatGID("Note", input.NoteID))
}
