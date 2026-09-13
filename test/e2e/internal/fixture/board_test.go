//go:build e2e

// board_test.go pins the one piece of the board builder that is not a call
// to GitLab: reading the number a REST action needs out of the global ID
// the mutation answers with.

package fixture

import (
	"strings"
	"testing"
)

// TestBoardIDFromGID_Answers_ReadsTheNumberOrRefuses checks the global ID
// parser on the shape GitLab answers and on the shapes it must refuse: a
// foreign prefix, a missing number, a zero, since a zero board id would be
// sent on and refused by GitLab with a message about the wrong thing.
func TestBoardIDFromGID_Answers_ReadsTheNumberOrRefuses(t *testing.T) {
	cases := []struct {
		name    string
		gid     string
		want    int64
		refused string
	}{
		{name: "a board", gid: "gid://gitlab/Board/42", want: 42},
		{name: "another object", gid: "gid://gitlab/Issue/42", refused: "does not start with"},
		{name: "no number", gid: "gid://gitlab/Board/", refused: "no positive number"},
		{name: "not a number", gid: "gid://gitlab/Board/x", refused: "no positive number"},
		{name: "zero", gid: "gid://gitlab/Board/0", refused: "no positive number"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := boardIDFromGID(testCase.gid)
			if testCase.refused != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.refused) {
					t.Fatalf("boardIDFromGID(%q) = %d, %v; want an error mentioning %q", testCase.gid, got, err, testCase.refused)
				}
				return
			}
			if err != nil || got != testCase.want {
				t.Errorf("boardIDFromGID(%q) = %d, %v; want %d", testCase.gid, got, err, testCase.want)
			}
		})
	}
}

// TestRunGraphQL_BoardMutation_DecodesTheBoard checks the board mutation's
// answer decodes into the shape the builder reads, through the same sender
// the iteration builder uses, so a renamed payload field fails here rather
// than as a board with no ID against a live GitLab.
func TestRunGraphQL_BoardMutation_DecodesTheBoard(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"createBoard":{"board":{"id":"gid://gitlab/Board/7"},"errors":[]}}}`}
	})

	var got struct {
		CreateBoard struct {
			Board *struct {
				ID string `json:"id"`
			} `json:"board"`
			Errors []string `json:"errors"`
		} `json:"createBoard"`
	}
	err := runGraphQL(t.Context(), client, boardMutation, map[string]any{"groupPath": "g", "name": "b"}, &got)
	if err != nil {
		t.Fatalf("runGraphQL() error = %v, want nil", err)
	}
	if got.CreateBoard.Board == nil || got.CreateBoard.Board.ID != "gid://gitlab/Board/7" {
		t.Errorf("decoded %+v, want the board the stub answered", got.CreateBoard)
	}
	if sent := len(stub.graphqlDocuments); sent != 1 || !strings.Contains(stub.graphqlDocuments[0], "createBoard") {
		t.Errorf("the stub was sent %d document(s), want exactly the board mutation", sent)
	}
}
