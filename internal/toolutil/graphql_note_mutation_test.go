package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// fakeGraphQL is a stub gl.GraphQLInterface that either fails with err or
// unmarshals body into the caller's response struct.
type fakeGraphQL struct {
	body string
	err  error
}

func (f fakeGraphQL) Do(_ gl.GraphQLQuery, response any, _ ...gl.RequestOptionFunc) (*gl.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return nil, json.Unmarshal([]byte(f.body), response)
}

// testNote is the note node shape used by the mutation helper tests.
type testNote struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

// TestExecGraphQLNoteMutation_Success verifies the happy path: the payload
// under the configured key is decoded and its note node returned.
func TestExecGraphQLNoteMutation_Success(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":{"id":"gid://gitlab/Note/7","body":"hi"},"errors":[]}}}`}
	note, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", Hint: "hint", PayloadKey: "createNote", Query: "mutation {}",
	})
	if err != nil {
		t.Fatalf("ExecGraphQLNoteMutation error = %v, want nil", err)
	}
	if note == nil || note.ID != "gid://gitlab/Note/7" || note.Body != "hi" {
		t.Errorf("note = %+v, want decoded node", note)
	}
}

// TestExecGraphQLNoteMutation_TransportError verifies transport errors are
// wrapped with the operation name and hint.
func TestExecGraphQLNoteMutation_TransportError(t *testing.T) {
	gql := fakeGraphQL{err: errors.New("boom")}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", Hint: "check the license", PayloadKey: "createNote",
	})
	if err == nil || !strings.Contains(err.Error(), "epicNoteCreate") || !strings.Contains(err.Error(), "check the license") {
		t.Errorf("err = %v, want op + hint wrapped", err)
	}
}

// TestExecGraphQLNoteMutation_MutationError verifies the first payload error
// is surfaced as "op: message".
func TestExecGraphQLNoteMutation_MutationError(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"updateNote":{"note":null,"errors":["not allowed","second"]}}}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteUpdate", PayloadKey: "updateNote",
	})
	if err == nil || err.Error() != "epicNoteUpdate: not allowed" {
		t.Errorf("err = %v, want %q", err, "epicNoteUpdate: not allowed")
	}
}

// TestExecGraphQLNoteMutation_NoNote verifies a nil note node with no errors
// becomes "op: no note returned".
func TestExecGraphQLNoteMutation_NoNote(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":null,"errors":[]}}}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicDiscussionCreate", PayloadKey: "createNote",
	})
	if err == nil || err.Error() != "epicDiscussionCreate: no note returned" {
		t.Errorf("err = %v, want no-note error", err)
	}
}

// TestExecGraphQLDestroyNote covers the destroy helper: success, transport
// error wrapping, and mutation payload error surfacing.
func TestExecGraphQLDestroyNote(t *testing.T) {
	ok := fakeGraphQL{body: `{"data":{"destroyNote":{"errors":[]}}}`}
	if err := ExecGraphQLDestroyNote(context.Background(), ok, "epicNoteDelete", "hint", "mutation {}", "gid://gitlab/Note/7"); err != nil {
		t.Errorf("success case err = %v, want nil", err)
	}

	boom := fakeGraphQL{err: errors.New("boom")}
	err := ExecGraphQLDestroyNote(context.Background(), boom, "epicNoteDelete", "verify note_id", "mutation {}", "gid://gitlab/Note/7")
	if err == nil || !strings.Contains(err.Error(), "epicNoteDelete") || !strings.Contains(err.Error(), "verify note_id") {
		t.Errorf("transport err = %v, want op + hint wrapped", err)
	}

	denied := fakeGraphQL{body: `{"data":{"destroyNote":{"errors":["forbidden"]}}}`}
	err = ExecGraphQLDestroyNote(context.Background(), denied, "epicNoteDelete", "hint", "mutation {}", "gid://gitlab/Note/7")
	if err == nil || err.Error() != "epicNoteDelete: forbidden" {
		t.Errorf("mutation err = %v, want %q", err, "epicNoteDelete: forbidden")
	}
}
