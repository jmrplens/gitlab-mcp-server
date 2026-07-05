package toolutil

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// GraphQLNoteMutation describes one work item note mutation call
// (createNote/updateNote) so the shared executor can apply the error
// conventions used by every GraphQL note domain (epic notes, epic
// discussions): transport errors are wrapped with the operation name and
// corrective hint, the first mutation payload error becomes "op: message",
// and a missing note node becomes "op: no note returned".
type GraphQLNoteMutation struct {
	// Op is the operation name used in wrapped errors (e.g. "epicNoteCreate").
	Op string
	// Hint is the corrective hint attached to transport errors.
	Hint string
	// PayloadKey is the mutation payload key in the response data object
	// ("createNote" or "updateNote").
	PayloadKey string
	// Query is the GraphQL mutation document.
	Query string
	// Variables holds the mutation variables.
	Variables map[string]any
}

// graphQLNotePayload is the generic single-note mutation payload: the mutated
// note node plus mutation errors.
type graphQLNotePayload[N any] struct {
	Note   *N       `json:"note"`
	Errors []string `json:"errors"`
}

// ExecGraphQLNoteMutation runs a work item note mutation and returns the
// mutated note node decoded as N, applying the shared error conventions
// described on GraphQLNoteMutation.
func ExecGraphQLNoteMutation[N any](ctx context.Context, gql gl.GraphQLInterface, m GraphQLNoteMutation) (*N, error) {
	var resp struct {
		Data map[string]graphQLNotePayload[N] `json:"data"`
	}
	if _, err := gql.Do(gl.GraphQLQuery{Query: m.Query, Variables: m.Variables}, &resp, gl.WithContext(ctx)); err != nil {
		return nil, WrapErrWithHint(m.Op, err, m.Hint)
	}
	payload := resp.Data[m.PayloadKey]
	if len(payload.Errors) > 0 {
		return nil, fmt.Errorf("%s: %s", m.Op, payload.Errors[0])
	}
	if payload.Note == nil {
		return nil, errors.New(m.Op + ": no note returned")
	}
	return payload.Note, nil
}

// ExecGraphQLDestroyNote runs the destroyNote work item mutation, applying
// the shared error conventions: transport errors are wrapped with op + hint
// and the first mutation payload error becomes "op: message".
func ExecGraphQLDestroyNote(ctx context.Context, gql gl.GraphQLInterface, op, hint, query, noteGID string) error {
	var resp struct {
		Data map[string]struct {
			Errors []string `json:"errors"`
		} `json:"data"`
	}
	if _, err := gql.Do(gl.GraphQLQuery{
		Query:     query,
		Variables: map[string]any{"id": noteGID},
	}, &resp, gl.WithContext(ctx)); err != nil {
		return WrapErrWithHint(op, err, hint)
	}
	if payload := resp.Data["destroyNote"]; len(payload.Errors) > 0 {
		return fmt.Errorf("%s: %s", op, payload.Errors[0])
	}
	return nil
}
