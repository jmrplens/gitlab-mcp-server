package toolutil

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
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
// under the configured key is decoded and its note node returned, with no
// quick actions status when GitLab sent none, since a body without a quick
// action is answered with a null status.
func TestExecGraphQLNoteMutation_Success(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":{"id":"gid://gitlab/Note/7","body":"hi"},"errors":[],"quickActionsStatus":null}}}`}
	result, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", Hint: "hint", PayloadKey: "createNote", Query: "mutation {}",
	})
	if err != nil {
		t.Fatalf("ExecGraphQLNoteMutation error = %v, want nil", err)
	}
	if result.Note == nil || result.Note.ID != "gid://gitlab/Note/7" || result.Note.Body != "hi" {
		t.Errorf("note = %+v, want decoded node", result.Note)
	}
	if result.QuickActions != nil {
		t.Errorf("quick actions = %+v, want nil for a body that carried none", result.QuickActions)
	}
}

// TestExecGraphQLNoteMutation_QuickActionsBesideTheNote verifies the status
// GitLab reports for a body mixing text and a quick action reaches the
// caller whole, every one of its four fields under its own name, beside the
// note GitLab kept.
func TestExecGraphQLNoteMutation_QuickActionsBesideTheNote(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":{"id":"gid://gitlab/Note/8","body":"text"},"errors":[],` +
		`"quickActionsStatus":{"commandNames":["label","assign"],"commandsOnly":false,"messages":["Added ~bug label."],"errorMessages":["Could not assign."]}}}}`}
	result, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", PayloadKey: "createNote",
	})
	if err != nil {
		t.Fatalf("ExecGraphQLNoteMutation error = %v, want nil", err)
	}
	if result.Note == nil || result.Note.ID != "gid://gitlab/Note/8" {
		t.Errorf("note = %+v, want the note GitLab kept", result.Note)
	}
	want := &QuickActionsStatusOutput{
		CommandNames:  []string{"label", "assign"},
		Messages:      []string{"Added ~bug label."},
		ErrorMessages: []string{"Could not assign."},
	}
	if !reflect.DeepEqual(result.QuickActions, want) {
		t.Errorf("quick actions = %+v, want %+v", result.QuickActions, want)
	}
}

// TestExecGraphQLNoteMutation_CommandsOnly_ReturnsTheStatusAndNoNote verifies
// the case the quick actions status exists for: a body holding only quick
// actions makes createNote run them and keep no note, which is a result and
// not the missing-note failure it used to be reported as.
func TestExecGraphQLNoteMutation_CommandsOnly_ReturnsTheStatusAndNoNote(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":null,"errors":[],` +
		`"quickActionsStatus":{"commandNames":["label"],"commandsOnly":true,"messages":["Added ~bug label."],"errorMessages":null}}}}`}
	result, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", PayloadKey: "createNote",
	})
	if err != nil {
		t.Fatalf("ExecGraphQLNoteMutation error = %v, want nil for a body of quick actions alone", err)
	}
	if result.Note != nil {
		t.Errorf("note = %+v, want nil: GitLab keeps no note for a body of quick actions alone", result.Note)
	}
	want := &QuickActionsStatusOutput{CommandNames: []string{"label"}, CommandsOnly: true, Messages: []string{"Added ~bug label."}}
	if !reflect.DeepEqual(result.QuickActions, want) {
		t.Errorf("quick actions = %+v, want %+v", result.QuickActions, want)
	}
}

// TestExecGraphQLNoteMutation_UpdateToCommandsOnly_SaysTheNoteWasDeleted
// verifies the one payload that carries neither a note nor a status nor an
// error: updateNote deletes a note whose new body holds only quick actions
// and says nothing more, so the error names that rather than claiming no note
// came back for an unknown reason.
func TestExecGraphQLNoteMutation_UpdateToCommandsOnly_SaysTheNoteWasDeleted(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"updateNote":{"note":null,"errors":[],"quickActionsStatus":null}}}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteUpdate", PayloadKey: "updateNote",
	})
	want := "epicNoteUpdate: GitLab deleted the note instead of editing it. " +
		"A new body holding only quick actions is run against the item the note is on and the note is removed, " +
		"and GitLab reports nothing about what the commands did, so read the item again to see what they changed"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// TestExecGraphQLNoteMutation_TopLevelRefusal_IsReportedAsTheRefusal verifies
// a refusal GitLab answers at the top level, with HTTP 200 and a null
// payload, is reported as GitLab's own message. Without it the null payload
// read as an update to quick actions alone, and as a missing note before
// that.
func TestExecGraphQLNoteMutation_TopLevelRefusal_IsReportedAsTheRefusal(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"updateNote":null},"errors":[{"message":"The resource that you are attempting to access does not exist"}]}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteUpdate", PayloadKey: "updateNote",
	})
	want := "epicNoteUpdate GraphQL errors: The resource that you are attempting to access does not exist"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// TestExecGraphQLNoteMutation_NullPayloadWithoutErrors_NamesTheMissingPayload
// verifies an answer with neither a payload nor a top-level error, which no
// GitLab sends on purpose, is reported as that rather than read as anything
// a payload would have meant.
func TestExecGraphQLNoteMutation_NullPayloadWithoutErrors_NamesTheMissingPayload(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{}}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteUpdate", PayloadKey: "updateNote",
	})
	if err == nil || err.Error() != "epicNoteUpdate: GitLab answered with no updateNote payload" {
		t.Errorf("err = %v, want the missing payload named", err)
	}
}

// TestExecGraphQLNoteMutation_KeptNoteWithAFailedCommand_IsAResult verifies
// the one payload error that is not a refusal: createNote saves the note
// before it applies the commands and adds a command's failure to the note's
// errors afterwards, prefixed with the attribute it validated. The note
// exists, so it is returned with the status that says what failed.
func TestExecGraphQLNoteMutation_KeptNoteWithAFailedCommand_IsAResult(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":{"id":"gid://gitlab/Note/9","body":"text"},` +
		`"errors":["Validation Cannot add more than 2 labels to a work item."],` +
		`"quickActionsStatus":{"commandNames":["label"],"commandsOnly":false,"messages":null,"errorMessages":["Cannot add more than 2 labels to a work item."]}}}}`}
	result, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicNoteCreate", PayloadKey: "createNote",
	})
	if err != nil {
		t.Fatalf("ExecGraphQLNoteMutation error = %v, want the kept note returned", err)
	}
	if result.Note == nil || result.Note.ID != "gid://gitlab/Note/9" {
		t.Errorf("note = %+v, want the note GitLab kept", result.Note)
	}
	if result.QuickActions == nil || len(result.QuickActions.ErrorMessages) != 1 {
		t.Errorf("quick actions = %+v, want the failure GitLab reported", result.QuickActions)
	}
}

// TestExecGraphQLNoteMutation_PayloadErrors_AreRefusals verifies every other
// shape of payload error stays the refusal it was: without a note, beside a
// note the status says nothing about, and beside a note where the status
// accounts for one error of two.
func TestExecGraphQLNoteMutation_PayloadErrors_AreRefusals(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "no note",
			body: `{"data":{"updateNote":{"note":null,"errors":["not allowed","second"]}}}`,
			want: "epicNoteUpdate: not allowed",
		},
		{
			name: "a note and no status",
			body: `{"data":{"updateNote":{"note":{"id":"gid://gitlab/Note/1"},"errors":["Note is too long"],"quickActionsStatus":null}}}`,
			want: "epicNoteUpdate: Note is too long",
		},
		{
			name: "a note and a status with no failure",
			body: `{"data":{"updateNote":{"note":{"id":"gid://gitlab/Note/1"},"errors":["Note is too long"],"quickActionsStatus":{"commandsOnly":false,"errorMessages":[]}}}}`,
			want: "epicNoteUpdate: Note is too long",
		},
		{
			name: "a note and a status accounting for one error of two",
			body: `{"data":{"updateNote":{"note":{"id":"gid://gitlab/Note/1"},"errors":["Validation label failed","Note is too long"],` +
				`"quickActionsStatus":{"commandsOnly":false,"errorMessages":["label failed"]}}}}`,
			want: "epicNoteUpdate: Validation label failed",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ExecGraphQLNoteMutation[testNote](context.Background(), fakeGraphQL{body: testCase.body}, GraphQLNoteMutation{
				Op: "epicNoteUpdate", PayloadKey: "updateNote",
			})
			if err == nil || err.Error() != testCase.want {
				t.Errorf("err = %v, want %q", err, testCase.want)
			}
		})
	}
}

// TestQuickActionFailuresOnly_EveryErrorMustBeAFailureTheStatusReports
// verifies the test the kept-note exception rests on, one condition at a
// time: an empty status message is no evidence (containment of the empty
// string is true of everything), and every payload error has to be matched,
// not just one.
func TestQuickActionFailuresOnly_EveryErrorMustBeAFailureTheStatusReports(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		errors []string
		status *QuickActionsStatusOutput
		want   bool
	}{
		{name: "no status", errors: []string{"x"}, status: nil, want: false},
		{name: "a status with no failure", errors: []string{"x"}, status: &QuickActionsStatusOutput{}, want: false},
		{name: "an empty failure matches nothing", errors: []string{"x"}, status: &QuickActionsStatusOutput{ErrorMessages: []string{""}}, want: false},
		{name: "a failure the error does not contain", errors: []string{"Validation x"}, status: &QuickActionsStatusOutput{ErrorMessages: []string{"y"}}, want: false},
		{name: "every error contains a failure", errors: []string{"Validation x", "Validation y"}, status: &QuickActionsStatusOutput{ErrorMessages: []string{"y", "x"}}, want: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := quickActionFailuresOnly(testCase.errors, testCase.status); got != testCase.want {
				t.Errorf("quickActionFailuresOnly(%q, %+v) = %t, want %t", testCase.errors, testCase.status, got, testCase.want)
			}
		})
	}
}

// TestGraphQLQuickActionsStatusSelection_SelectsEveryFieldTheDecoderReads
// verifies the selection and the decoder name the same four fields, so a
// document that concatenates the one never leaves a field of the other
// empty.
func TestGraphQLQuickActionsStatusSelection_SelectsEveryFieldTheDecoderReads(t *testing.T) {
	decoder := reflect.TypeFor[graphQLQuickActionsStatus]()
	for field := range decoder.Fields() {
		name := field.Tag.Get("json")
		if !strings.Contains(GraphQLQuickActionsStatusSelection, " "+name+" ") {
			t.Errorf("GraphQLQuickActionsStatusSelection = %q, which does not select %q", GraphQLQuickActionsStatusSelection, name)
		}
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

// TestExecGraphQLNoteMutation_NoNote verifies a createNote payload with no
// note, no error and no quick actions status, which GitLab sends for no body
// it documents, stays "op: no note returned" rather than borrowing the
// deletion reading that belongs to updateNote alone.
func TestExecGraphQLNoteMutation_NoNote(t *testing.T) {
	gql := fakeGraphQL{body: `{"data":{"createNote":{"note":null,"errors":[]}}}`}
	_, err := ExecGraphQLNoteMutation[testNote](context.Background(), gql, GraphQLNoteMutation{
		Op: "epicDiscussionCreate", PayloadKey: "createNote",
	})
	if err == nil || err.Error() != "epicDiscussionCreate: no note returned" {
		t.Errorf("err = %v, want no-note error", err)
	}
}

// TestExecGraphQLDestroyNote_TopLevelRefusal_IsNotASuccess verifies the
// defect the top-level check closes: a delete GitLab refuses answers HTTP 200
// with a null payload and a top-level error, and the helper used to read the
// absent payload errors as a successful delete.
func TestExecGraphQLDestroyNote_TopLevelRefusal_IsNotASuccess(t *testing.T) {
	refused := fakeGraphQL{body: `{"data":{"destroyNote":null},"errors":[{"message":"The resource that you are attempting to access does not exist"}]}`}
	err := ExecGraphQLDestroyNote(context.Background(), refused, "epicNoteDelete", "hint", "mutation {}", "gid://gitlab/Note/7")
	want := "epicNoteDelete GraphQL errors: The resource that you are attempting to access does not exist"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}

	empty := fakeGraphQL{body: `{"data":{}}`}
	err = ExecGraphQLDestroyNote(context.Background(), empty, "epicNoteDelete", "hint", "mutation {}", "gid://gitlab/Note/7")
	if err == nil || err.Error() != "epicNoteDelete: GitLab answered with no destroyNote payload" {
		t.Errorf("err = %v, want the missing payload named", err)
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
