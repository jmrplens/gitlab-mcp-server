// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// board_test.go drives the board builders against the stub: the mutation each
// sends and for which group or project, what a refused or empty answer comes
// back as, and reading the number a REST action needs out of the global ID
// the mutation answers with.

package fixture

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
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

// TestLimitBoardList_Answered_PutsTheCeilingWhereTheMetricCounts checks the
// limit mutation names the column by the global ID GitLab gives a list and
// carries the metric with the ceiling in the field that metric counts, the
// other ceiling sent as zero, which is what GitLab's own board settings panel
// sends (board_settings_wip_limit.vue): a column counted by weight is limited
// by maxIssueWeight, and one counted by issues by maxIssueCount. The document
// and those variables are held to the pinned schema too, since nothing but a
// licensed GitLab runs the mutation otherwise and a field spelled wrong would
// first fail a whole Enterprise run.
func TestLimitBoardList_Answered_PutsTheCeilingWhereTheMetricCounts(t *testing.T) {
	cases := []struct {
		name   string
		metric string
		answer string
		want   map[string]any
	}{
		{
			name:   "by weight",
			metric: "issue_weights",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/5","limitMetric":"issue_weights","maxIssueCount":0,"maxIssueWeight":4},"errors":[]}}}`,
			want:   map[string]any{"listId": "gid://gitlab/List/5", "limitMetric": "issue_weights", "maxIssueCount": float64(0), "maxIssueWeight": float64(4)},
		},
		{
			name:   "by issue count",
			metric: "issue_count",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/5","limitMetric":"issue_count","maxIssueCount":4,"maxIssueWeight":0},"errors":[]}}}`,
			want:   map[string]any{"listId": "gid://gitlab/List/5", "limitMetric": "issue_count", "maxIssueCount": float64(4), "maxIssueWeight": float64(0)},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{testCase.answer} })

			if err := limitBoardList(t.Context(), client, 5, testCase.metric, 4); err != nil {
				t.Fatalf("limitBoardList() error = %v, want nil", err)
			}
			if len(stub.graphqlDocuments) != 1 || stub.graphqlDocuments[0] != boardListLimitMutation {
				t.Fatalf("the stub was sent %q, want exactly the list limit mutation", stub.graphqlDocuments)
			}
			if got := stub.graphqlVariables[0]; !reflect.DeepEqual(got, testCase.want) {
				t.Errorf("the mutation carried %v, want %v", got, testCase.want)
			}
			if err := graphqlschema.Validate(boardListLimitMutation, testCase.want); err != nil {
				t.Errorf("the pinned schema refuses the list limit mutation: %v", err)
			}
		})
	}
}

// TestLimitBoardList_Answers_RefusesALimitThatDidNotApply checks every way the
// limit mutation's answer can fail to say the column is now limited as asked:
// GitLab's refusal inside the payload, a payload with no list, a list counted
// by another metric, a list whose ceiling landed in the other field or not at
// all, and a document GitLab refused outright. Each is an error, since a
// scenario that went on would assert a metric or a ceiling GitLab was never
// told to send.
func TestLimitBoardList_Answers_RefusesALimitThatDidNotApply(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{
			name:   "refused in the payload",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":null,"errors":["List not found"]}}}`,
			want:   "GitLab refused the limit: List not found",
		},
		{name: "no list", answer: `{"data":{"boardListUpdateLimitMetrics":{"list":null,"errors":[]}}}`, want: errBoardListNotAnswered.Error()},
		{
			name:   "another metric",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/5","limitMetric":"all_metrics","maxIssueCount":0,"maxIssueWeight":4},"errors":[]}}}`,
			want:   `GitLab answered the list counted by "all_metrics", want "issue_weights"`,
		},
		{
			name:   "the ceiling on the issue count",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/5","limitMetric":"issue_weights","maxIssueCount":4,"maxIssueWeight":0},"errors":[]}}}`,
			want:   "GitLab answered the list limited to 4 issues and a weight of 0, want 0 and 4",
		},
		{
			name:   "no ceiling",
			answer: `{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/5","limitMetric":"issue_weights","maxIssueCount":0,"maxIssueWeight":0},"errors":[]}}}`,
			want:   "GitLab answered the list limited to 0 issues and a weight of 0, want 0 and 4",
		},
		{name: "document refused", answer: `{"data":null,"errors":[{"message":"The resource that you are attempting to access does not exist"}]}`, want: "GitLab refused the document"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{testCase.answer} })

			err := limitBoardList(t.Context(), client, 5, "issue_weights", 4)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("limitBoardList() error = %v, want it to say %q", err, testCase.want)
			}
		})
	}
}

// TestLimitBoardList_Detached_LimitsTheColumnItWasGiven checks the exported
// builder hands its column, metric and ceiling to the mutation and returns
// when GitLab applied them; its failing branch is
// [TestBuilders_Refused_FailTheirTestNamingWhatTheyWereMaking]'s.
func TestLimitBoardList_Detached_LimitsTheColumnItWasGiven(t *testing.T) {
	stub, e := detachedStub(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{`{"data":{"boardListUpdateLimitMetrics":{"list":{"id":"gid://gitlab/List/8","limitMetric":"issue_count","maxIssueCount":3,"maxIssueWeight":0},"errors":[]}}}`}
	})

	LimitBoardList(e, 8, "issue_count", 3)

	if len(stub.graphqlVariables) != 1 {
		t.Fatalf("the stub was sent %d document(s), want exactly the list limit mutation", len(stub.graphqlVariables))
	}
	want := map[string]any{"listId": "gid://gitlab/List/8", "limitMetric": "issue_count", "maxIssueCount": float64(3), "maxIssueWeight": float64(0)}
	if got := stub.graphqlVariables[0]; !reflect.DeepEqual(got, want) {
		t.Errorf("the mutation carried %v, want %v", got, want)
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
