// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package epicworkitems

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const queryResolveWorkItemGID = `
query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
    }
  }
}
`

// ResolveEpicGID resolves an epic-backed work item GID by group path and IID.
func ResolveEpicGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return resolveGID(ctx, client, fullPath, iid, func() error {
		return fmt.Errorf("epic not found in group %q with IID %d", fullPath, iid)
	})
}

// ResolveWorkItemGID resolves a generic work item GID by namespace path and IID.
func ResolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return resolveGID(ctx, client, fullPath, iid, func() error {
		return fmt.Errorf("work item not found in %q with IID %d", fullPath, iid)
	})
}

type gqlWorkItemID struct {
	ID string `json:"id"`
}

type gqlNamespaceWorkItemID struct {
	WorkItem *gqlWorkItemID `json:"workItem"`
}

func resolveGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64, notFound func() error) (string, error) {
	var resp struct {
		Data struct {
			Namespace *gqlNamespaceWorkItemID `json:"namespace"`
		} `json:"data"`
	}

	//gitlab:request mandatory: an epic action resolves the epic's work item ID here and then sends its own request with it, so both run on every call
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
		return "", notFound()
	}

	return resp.Data.Namespace.WorkItem.ID, nil
}

// queryNoteOwner reads what a note is on and where GitLab serves it. GitLab
// serves a note of a group's work item at /groups/<path>/-/work_items/<iid>,
// which names the group and the epic, and answers an epic's note as one of
// two noteable types: an Issue for a note made on the epic's work item, which
// is every note this server makes, and an Epic for one made through the legacy
// epic route. Measured on GitLab 19.4.2 EE, where the address is the same for
// both and the noteable type is never WorkItem.
//
// The note is all it reads. Resolving the epic as well would hold the note to
// it by id, and would withhold the four note writes from every fine-grained
// token, since no grant lets one resolve a group's work item at GitLab 19.4.1,
// while a grant that may update or delete a note at a group may read it there
// (Note declares read_note at the group boundary).
const queryNoteOwner = `
query($id: NoteID!) {
  note(id: $id) {
    noteableId
    noteableType
    url
  }
}
`

// gqlNoteOwner is a note as [queryNoteOwner] selects it.
type gqlNoteOwner struct {
	NoteableID   int64  `json:"noteableId"`
	NoteableType string `json:"noteableType"`
	URL          string `json:"url"`
}

// epicNoteCollections are the collections an epic's address names it under:
// work_items, where GitLab 19.4 serves every epic, and epics, where it served
// them before epics became work items. Either names the epic of that iid in
// that group and nothing else.
var epicNoteCollections = []string{"work_items", "epics"}

// noteOfEpic reports whether address, the URL GitLab serves a note at, is
// that of a note of the epic iid of the group fullPath on an instance served
// under the relative URL root root.
//
// The whole path is compared, root first, because a match on its tail would
// admit a project's work item: a project whose path ends in groups/<path>
// serves its items at /<namespace>/groups/<path>/-/work_items/<iid>, and the
// segment groups is reserved only at the top of an instance. The comparison
// ignores case, as GitLab routes a path whatever its case. An address under
// a root the client was not configured with is refused, which is the safe
// answer for a write that cannot be undone.
func noteOfEpic(address, root, fullPath string, iid int64) bool {
	group := strings.Trim(fullPath, "/")
	if group == "" {
		return false
	}
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	for _, collection := range epicNoteCollections {
		if strings.EqualFold(parsed.Path, root+"/groups/"+group+"/-/"+collection+"/"+strconv.FormatInt(iid, 10)) {
			return true
		}
	}
	return false
}

// instanceRoot is the relative URL root GitLab is served under, read from
// the API base the client was configured with: whatever stands in front of
// /api/v4, which client-go's base URL ends in.
func instanceRoot(base *url.URL) string {
	return strings.TrimSuffix(strings.TrimSuffix(base.Path, "/"), "/api/v4")
}

// belongsTo reports whether the note is a note of the epic iid of the group
// fullPath on an instance served under root: an Issue or an Epic, the two
// types GitLab answers an epic's note with, at the epic's address.
func (n gqlNoteOwner) belongsTo(root, fullPath string, iid int64) bool {
	return (n.NoteableType == "Issue" || n.NoteableType == "Epic") && noteOfEpic(n.URL, root, fullPath, iid)
}

// CheckNoteOnEpic reads the note noteID and returns the address GitLab serves
// it at when it is a note of the epic fullPath and iid name, and an error
// naming where the note is otherwise.
//
// The epic note writes take the note's global ID alone, which GitLab
// resolves whatever it belongs to, so before this an edit or an irreversible
// delete named through an epic reached a note on a project issue, a merge
// request or another epic, and the epic named was never consulted (issue
// 1301). op prefixes every error the way the caller's own do.
func CheckNoteOnEpic(ctx context.Context, client *gitlabclient.Client, op, fullPath string, iid, noteID int64) (string, error) {
	var resp struct {
		Data struct {
			Note *gqlNoteOwner `json:"note"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}
	//gitlab:request mandatory: a note write reads what the note is on and where GitLab serves it before it changes it, on every call
	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryNoteOwner,
		Variables: map[string]any{"id": toolutil.FormatGID("Note", noteID)},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return "", toolutil.WrapErrWithHint(op, err, "verify note_id with group.epic_note_list")
	}
	note := resp.Data.Note
	if note == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError(op, resp.Errors); graphQLErr != nil {
			return "", graphQLErr
		}
		return "", fmt.Errorf("%s: note %d not found, or not one this token can read; find the epic's notes with group.epic_note_list", op, noteID)
	}
	if !note.belongsTo(instanceRoot(client.GL().BaseURL()), fullPath, iid) {
		return "", fmt.Errorf("%s: note %d is not a note of epic &%d in group %q: GitLab answers it as a note on %s %d (%s), so nothing was changed; find the epic's notes with group.epic_note_list",
			op, noteID, iid, fullPath, note.NoteableType, note.NoteableID, note.URL)
	}
	return note.URL, nil
}

// RequireEpic refuses under op what an epic note or discussion action is
// handed when it names no epic: a context that has already ended, no group
// path, or no epic iid. It sends nothing. Every such action but the two
// lists starts with it; a list's refusal of a missing path also says where to
// find one.
func RequireEpic(ctx context.Context, op, fullPath string, iid int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if fullPath == "" {
		return fmt.Errorf("%s: full_path is required", op)
	}
	if iid <= 0 {
		return toolutil.ErrRequiredInt64(op, "epic_iid")
	}
	return nil
}

// ResolveEpicForNote resolves the work item global ID a note opening a
// thread on the epic is sent to, once [RequireEpic] passes and body is not
// empty, and reports a resolution that failed under op with hint.
func ResolveEpicForNote(ctx context.Context, client *gitlabclient.Client, op, fullPath string, iid int64, body, hint string) (string, error) {
	if err := RequireEpic(ctx, op, fullPath, iid); err != nil {
		return "", err
	}
	if body == "" {
		return "", fmt.Errorf("%s: body is required", op)
	}
	workItemGID, err := ResolveEpicGID(ctx, client, fullPath, iid)
	if err != nil {
		return "", toolutil.WrapErrWithHint(op, err, hint)
	}
	return workItemGID, nil
}

// RequireNoteOnEpic is [CheckNoteOnEpic] for a write handed the epic and the
// note alone, a delete: it refuses what [RequireEpic] refuses and a note_id
// that names no note before it reads anything.
func RequireNoteOnEpic(ctx context.Context, client *gitlabclient.Client, op, fullPath string, iid, noteID int64) (string, error) {
	if err := RequireEpic(ctx, op, fullPath, iid); err != nil {
		return "", err
	}
	if noteID <= 0 {
		return "", toolutil.ErrRequiredInt64(op, "note_id")
	}
	return CheckNoteOnEpic(ctx, client, op, fullPath, iid, noteID)
}

// EpicNotAnswered words, under op, an answer to a document reading the epic's
// work item that carries no work item: what GitLab refused when it refused
// the document, and otherwise that the group holds no epic of that iid.
//
// GitLab answers a rejected document with HTTP 200 and a top-level errors
// array, which client-go does not turn into an error, so a query the instance
// refused reaches a handler looking exactly like an epic that is not there.
// The epic note and discussion reads both need that answer, which is why it
// lives here rather than at each call site.
func EpicNotAnswered(op string, errs []toolutil.GraphQLError, fullPath string, iid int64) error {
	if graphQLErr := toolutil.GraphQLTopLevelError(op, errs); graphQLErr != nil {
		return graphQLErr
	}
	return fmt.Errorf("%s: epic not found in group %q with IID %d", op, fullPath, iid)
}
