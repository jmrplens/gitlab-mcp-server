package toolutil

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// GraphQLQuickActionsStatusSelection is the selection every createNote and
// updateNote document sends beside its note, spelled once so that a document
// cannot ask for part of it: GitLab's account of the quick actions the body
// carried, which is the only place a caller learns whether "/label ~bug" was
// applied, refused, or was the whole of a body GitLab therefore kept no note
// for. The documents concatenate it as a constant, so the schema and shape
// gates judge the string GitLab receives.
const GraphQLQuickActionsStatusSelection = "quickActionsStatus { commandNames commandsOnly errorMessages messages }"

// updateNotePayloadKey is the payload key of the one note mutation that
// deletes the note it was asked to edit when the new body holds nothing but
// quick actions.
const updateNotePayloadKey = "updateNote"

// GraphQLNoteMutation describes one work item note mutation call
// (createNote/updateNote) so the shared executor can apply the error
// conventions used by every GraphQL note domain (epic notes, epic
// discussions): transport errors are wrapped with the operation name and
// corrective hint, a refusal GitLab answers at the top level is reported as
// that refusal, the first mutation payload error becomes "op: message", and
// a missing note node is reported for what GitLab means by it.
type GraphQLNoteMutation struct {
	// Op is the operation name used in wrapped errors (e.g. "epicNoteCreate").
	Op string
	// Hint is the corrective hint attached to transport errors.
	Hint string
	// PayloadKey is the mutation payload key in the response data object
	// ("createNote" or "updateNote").
	PayloadKey string
	// Query is the GraphQL mutation document. It selects
	// [GraphQLQuickActionsStatusSelection] beside the note.
	Query string
	// Variables holds the mutation variables.
	Variables map[string]any
}

// QuickActionsStatusOutput mirrors GitLab's QuickActionsStatus, what a note
// mutation reports about the quick actions its body carried: the commands it
// recognized, whether the body held nothing else (in which case GitLab keeps
// no note), what the commands did, and what they failed to do. A nil pointer
// on an output is a body that carried none: GitLab answers a status for every
// note on an item that supports quick actions, an empty one when the body
// named no command, and the decoder reads the empty one as none.
type QuickActionsStatusOutput struct {
	CommandNames  []string `json:"command_names,omitempty"`
	CommandsOnly  bool     `json:"commands_only"`
	Messages      []string `json:"messages,omitempty"`
	ErrorMessages []string `json:"error_messages,omitempty"`
}

// graphQLQuickActionsStatus decodes the QuickActionsStatus object as the
// schema spells it.
type graphQLQuickActionsStatus struct {
	CommandNames  []string `json:"commandNames"`
	CommandsOnly  bool     `json:"commandsOnly"`
	ErrorMessages []string `json:"errorMessages"`
	Messages      []string `json:"messages"`
}

// output converts the decoded status to the published one, nil when GitLab
// sent none or sent the empty one it answers for a body that named no
// command.
//
// Notes::CreateService and Notes::UpdateService build a QuickActionsStatus for
// every note on a work item, an issue, a merge request or a commit, whether or
// not the body named a command, and createNote and updateNote answer with it
// whenever it exists. For a body without a command it carries no command, no
// message, no failure and commandsOnly false, which says nothing, and
// publishing it would put a quick actions section on every note a caller
// writes. Any one of the four set is a report worth publishing.
func (s *graphQLQuickActionsStatus) output() *QuickActionsStatusOutput {
	if s == nil {
		return nil
	}
	if len(s.CommandNames) == 0 && !s.CommandsOnly && len(s.Messages) == 0 && len(s.ErrorMessages) == 0 {
		return nil
	}
	return &QuickActionsStatusOutput{
		CommandNames:  s.CommandNames,
		CommandsOnly:  s.CommandsOnly,
		Messages:      s.Messages,
		ErrorMessages: s.ErrorMessages,
	}
}

// graphQLNotePayload is the generic single-note mutation payload: the mutated
// note node, the mutation errors, and the quick actions status.
type graphQLNotePayload[N any] struct {
	Note               *N                         `json:"note"`
	Errors             []string                   `json:"errors"`
	QuickActionsStatus *graphQLQuickActionsStatus `json:"quickActionsStatus"`
}

// GraphQLNoteMutationResult is what a note mutation answered with.
type GraphQLNoteMutationResult[N any] struct {
	// Note is the note the mutation created or edited. It is nil when the
	// body held only quick actions: createNote then runs them and keeps no
	// note, and QuickActions says what they did.
	Note *N
	// QuickActions is GitLab's account of the quick actions the body
	// carried, nil when it carried none.
	QuickActions *QuickActionsStatusOutput
}

// ExecGraphQLNoteMutation runs a work item note mutation and returns the
// mutated note node decoded as N with the quick actions status beside it,
// applying the shared error conventions described on GraphQLNoteMutation.
//
// A payload without a note is not one condition. A body holding only quick
// actions makes createNote run them and keep no note, and it says so in the
// status, which is a result and not a failure. The same body makes updateNote
// delete the note it was asked to edit and answer with neither note nor status
// (Mutations::Notes::Update::Base in GitLab), which is reported as an error
// that says what happened, since the call cannot answer with the edited note
// it promised and a caller told only that no note came back would read the
// commands as not applied. A refusal GitLab answers at the top level leaves
// the payload null, and is checked first so that neither reading is ever
// given to a refusal.
//
// A payload error is a refusal, with one exception: a note GitLab kept whose
// every error is a quick action failure the status reports too. createNote
// saves the note before it applies the commands and adds a command's failure
// to the note's errors afterwards, so reporting that as a refusal would hide
// a note that exists and invite the caller to post it twice. A body of
// commands alone whose update failed answers the same error and status with
// no note, and stays a refusal: GitLab ran nothing and kept nothing.
func ExecGraphQLNoteMutation[N any](ctx context.Context, gql gl.GraphQLInterface, m GraphQLNoteMutation) (GraphQLNoteMutationResult[N], error) {
	var resp struct {
		Data   map[string]*graphQLNotePayload[N] `json:"data"`
		Errors []GraphQLError                    `json:"errors"`
	}
	if _, err := gql.Do(gl.GraphQLQuery{Query: m.Query, Variables: m.Variables}, &resp, gl.WithContext(ctx)); err != nil {
		return GraphQLNoteMutationResult[N]{}, WrapErrWithHint(m.Op, err, m.Hint)
	}
	payload := resp.Data[m.PayloadKey]
	if payload == nil {
		if graphQLErr := GraphQLTopLevelError(m.Op, resp.Errors); graphQLErr != nil {
			return GraphQLNoteMutationResult[N]{}, graphQLErr
		}
		return GraphQLNoteMutationResult[N]{}, errors.New(m.Op + ": GitLab answered with no " + m.PayloadKey + " payload")
	}
	status := payload.QuickActionsStatus.output()
	if len(payload.Errors) > 0 && (payload.Note == nil || !quickActionFailuresOnly(payload.Errors, status)) {
		return GraphQLNoteMutationResult[N]{}, fmt.Errorf("%s: %s", m.Op, payload.Errors[0])
	}
	if payload.Note == nil {
		if status != nil {
			return GraphQLNoteMutationResult[N]{QuickActions: status}, nil
		}
		if m.PayloadKey == updateNotePayloadKey {
			return GraphQLNoteMutationResult[N]{}, errors.New(m.Op + ": GitLab deleted the note instead of editing it. " +
				"A new body holding only quick actions is run against the item the note is on and the note is removed, " +
				"and GitLab reports nothing about what the commands did, so read the item again to see what they changed")
		}
		return GraphQLNoteMutationResult[N]{}, errors.New(m.Op + ": no note returned")
	}
	return GraphQLNoteMutationResult[N]{Note: payload.Note, QuickActions: status}, nil
}

// quickActionFailuresOnly reports whether every payload error is a quick
// action failure the status also reports. GitLab adds such a failure to the
// note's errors under the attribute it validates, so the payload spells it
// with that attribute's name in front ("Validation Cannot add more than 10
// labels"), which is why the test is containment rather than equality.
func quickActionFailuresOnly(payloadErrors []string, status *QuickActionsStatusOutput) bool {
	if status == nil || len(status.ErrorMessages) == 0 {
		return false
	}
	for _, payloadError := range payloadErrors {
		if !slices.ContainsFunc(status.ErrorMessages, func(message string) bool {
			return message != "" && strings.Contains(payloadError, message)
		}) {
			return false
		}
	}
	return true
}

// ExecGraphQLDestroyNote runs the destroyNote work item mutation, applying
// the shared error conventions: transport errors are wrapped with op + hint,
// a refusal GitLab answers at the top level is reported as that refusal, and
// the first mutation payload error becomes "op: message".
//
// The document selects the errors alone. The payload type offers a note and a
// quick actions status as well, and Mutations::Notes::Destroy answers with
// neither: its resolver returns the errors and nothing else, so both are null
// on every response.
//
// The top-level check is what keeps a refused delete from reading as a
// successful one. A note that is gone or that the token may not delete is
// refused with HTTP 200, a top-level error and a null payload, which
// client-go does not turn into an error, and a null payload carries no
// payload error either.
func ExecGraphQLDestroyNote(ctx context.Context, gql gl.GraphQLInterface, op, hint, query, noteGID string) error {
	var resp struct {
		Data map[string]*struct {
			Errors []string `json:"errors"`
		} `json:"data"`
		Errors []GraphQLError `json:"errors"`
	}
	if _, err := gql.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: map[string]any{"id": noteGID},
	}, &resp, gl.WithContext(ctx)); err != nil {
		return WrapErrWithHint(op, err, hint)
	}
	payload := resp.Data["destroyNote"]
	if payload == nil {
		if graphQLErr := GraphQLTopLevelError(op, resp.Errors); graphQLErr != nil {
			return graphQLErr
		}
		return errors.New(op + ": GitLab answered with no destroyNote payload")
	}
	if len(payload.Errors) > 0 {
		return fmt.Errorf("%s: %s", op, payload.Errors[0])
	}
	return nil
}
