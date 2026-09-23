//go:build e2e

// board_test.go drives the board builders against the stub: the mutation each
// sends and for which group or project, what a refused or empty answer comes
// back as, and reading the number a REST action needs out of the global ID
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

// TestCreateProjectBoard_Created_NamesTheProjectAndReadsTheNumber checks the
// project board creator sends the project's document with the project's path,
// and reads the number the board resource and the REST actions take out of
// the global ID GitLab answers with.
func TestCreateProjectBoard_Created_NamesTheProjectAndReadsTheNumber(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"createBoard":{"board":{"id":"gid://gitlab/Board/12"},"errors":[]}}}`}
	})

	got, err := createProjectBoard(t.Context(), client, "group/project", "board-run")
	if err != nil {
		t.Fatalf("createProjectBoard() error = %v, want nil", err)
	}
	if want := (ProjectBoard{ID: 12, Name: "board-run"}); got != want {
		t.Errorf("createProjectBoard() = %+v, want %+v", got, want)
	}
	if len(stub.graphqlDocuments) != 1 || stub.graphqlDocuments[0] != projectBoardMutation {
		t.Errorf("the stub was sent %q, want exactly the project board mutation", stub.graphqlDocuments)
	}
}

// TestNewGroupBoard_Detached_AsksForTheGroupsBoardUnderTheRun checks the
// builder whole: the group mutation, carrying the group's path and a name
// scoped to the run, and the board handed back with the number and the global
// ID GitLab answered.
func TestNewGroupBoard_Detached_AsksForTheGroupsBoardUnderTheRun(t *testing.T) {
	stub, e := detachedStub(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"createBoard":{"board":{"id":"gid://gitlab/Board/9"},"errors":[]}}}`}
	})

	board := NewGroupBoard(e, Group{ID: 1, Path: "e2e-group"}, "board")

	if board.ID != 9 || board.GID != "gid://gitlab/Board/9" || !isScopedName(board.Name, "board", e) {
		t.Errorf("NewGroupBoard() = %+v, want board 9 named under the run", board)
	}
	if len(stub.graphqlDocuments) != 1 || stub.graphqlDocuments[0] != groupBoardMutation {
		t.Fatalf("the stub was sent %q, want exactly the group board mutation", stub.graphqlDocuments)
	}
	if sent := stub.graphqlVariables[0]; sent["groupPath"] != "e2e-group" || sent["name"] != board.Name {
		t.Errorf("the mutation carried %v, want the group's path and the board's name", sent)
	}
}

// TestCreateBoard_Answers_RefusesWhatNamesNoBoard checks every way a board
// mutation's answer can fail to name a board: GitLab's refusal inside the
// payload, a payload with no board, a global ID of something else, and a
// document GitLab refused outright. Each is an error, since each would
// otherwise hand a caller a board numbered zero.
func TestCreateBoard_Answers_RefusesWhatNamesNoBoard(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{
			name:   "refused in the payload",
			answer: `{"data":{"createBoard":{"board":null,"errors":["Multiple boards are not available"]}}}`,
			want:   "GitLab refused the board: Multiple boards are not available",
		},
		{name: "no board", answer: `{"data":{"createBoard":{"board":null,"errors":[]}}}`, want: errBoardWithoutID.Error()},
		{name: "no id", answer: `{"data":{"createBoard":{"board":{"id":""},"errors":[]}}}`, want: errBoardWithoutID.Error()},
		{name: "another object", answer: `{"data":{"createBoard":{"board":{"id":"gid://gitlab/Issue/3"},"errors":[]}}}`, want: "does not start with"},
		{name: "document refused", answer: `{"data":null,"errors":[{"message":"Argument 'projectPath' is invalid"}]}`, want: "GitLab refused the document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{testCase.answer} })

			got, err := createProjectBoard(t.Context(), client, "group/project", "board-run")
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("createProjectBoard() error = %v, want it to say %q", err, testCase.want)
			}
			if got != (ProjectBoard{}) {
				t.Errorf("createProjectBoard() = %+v on a failure, want nothing", got)
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
	err := runGraphQL(t.Context(), client, groupBoardMutation, map[string]any{"groupPath": "g", "name": "b"}, &got)
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
