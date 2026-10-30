// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// repository_test.go pins the commit retry rules and the content helpers.

package fixture

import (
	"bytes"
	"errors"
	"image/png"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestCommitRetryable_BranchNotReady_NamesAStartBranchThenDropsIt walks the
// two-step dance a commit on a fresh branch goes through: the first refusal
// turns the start branch on, the "already exists" that follows turns it off,
// and a file that already exists with the start branch off is not retried at
// all, since that is the create-or-update fallback's signal.
func TestCommitRetryable_BranchNotReady_NamesAStartBranchThenDropsIt(t *testing.T) {
	needStartBranch := false

	if !commitRetryable(errors.New("you can only create or edit files when you are on a branch"), &needStartBranch) || !needStartBranch {
		t.Fatalf("the branch-not-ready refusal should be retried with a start branch; need=%t", needStartBranch)
	}
	if !commitRetryable(errors.New("a branch with this name already exists"), &needStartBranch) || needStartBranch {
		t.Fatalf("the branch-exists answer should be retried without the start branch; need=%t", needStartBranch)
	}
	if commitRetryable(errors.New("a file with this name already exists"), &needStartBranch) {
		t.Fatal("a file that already exists is the fallback's signal, not a retry")
	}
	if !commitRetryable(errors.New("connection reset by peer"), &needStartBranch) {
		t.Fatal("a transient network error is retried")
	}
	if commitRetryable(errors.New("403 Forbidden"), &needStartBranch) {
		t.Fatal("a refusal is not retried")
	}
}

// TestCommitRetryReason_ByClass_NamesTheStep checks the log line each retry
// carries.
func TestCommitRetryReason_ByClass_NamesTheStep(t *testing.T) {
	cases := []struct {
		name            string
		err             error
		needStartBranch bool
		want            string
	}{
		{name: "not ready", err: errors.New("only create or edit files when you are on a branch"), want: "branch not ready, naming a start branch"},
		{name: "exists with start branch", err: errors.New("already exists"), needStartBranch: true, want: "branch exists now, dropping the start branch"},
		{name: "network", err: errors.New("EOF"), want: "transient network error"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := commitRetryReason(testCase.err, testCase.needStartBranch); got != testCase.want {
				t.Errorf("commitRetryReason() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// branchRefusedThroughTheServer is the branch refusal as a scenario reads it
// off a tool error, the server's words around GitLab's.
const branchRefusedThroughTheServer = "tool_error after 41ms: repository.commit_create: You can only create or edit files when you are on a branch"

// TestRetryWhileBranchSettles_RetriesOnlyTheBranchRefusal checks what a
// scenario's write to a branch it has just created through the server is tried
// again for: the branch refusal, as the tool error carries it, until the write
// goes through, and nothing else, so a refusal of another kind comes back after
// the one attempt that met it.
func TestRetryWhileBranchSettles_RetriesOnlyTheBranchRefusal(t *testing.T) {
	cases := []struct {
		name      string
		answers   []error
		wantCalls int
		wantErr   string
	}{
		{name: "refused once, then written", answers: []error{errors.New(branchRefusedThroughTheServer), nil}, wantCalls: 2},
		{name: "written at once", answers: []error{nil}, wantCalls: 1},
		{
			name:      "another refusal is not retried",
			answers:   []error{errors.New("tool_error after 9ms: repository.commit_create: A file with this name already exists")},
			wantCalls: 1, wantErr: "A file with this name already exists",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, e := detachedStub(t)
			calls := 0
			got, err := RetryWhileBranchSettles(e, "commit create on feature", func() (string, error) {
				answer := testCase.answers[calls]
				calls++
				if answer != nil {
					return "", answer
				}
				return "written", nil
			})
			if calls != testCase.wantCalls {
				t.Errorf("the write was sent %d times, want %d", calls, testCase.wantCalls)
			}
			switch {
			case testCase.wantErr == "" && (err != nil || got != "written"):
				t.Errorf("RetryWhileBranchSettles() = %q, %v; want the write's answer", got, err)
			case testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)):
				t.Errorf("RetryWhileBranchSettles() error = %v, want the refusal %q", err, testCase.wantErr)
			}
		})
	}
}

// TestShortSHA_Lengths_AbbreviatesLikeGitLab checks the eight-character
// short id and that a short input is left alone.
func TestShortSHA_Lengths_AbbreviatesLikeGitLab(t *testing.T) {
	cases := []struct {
		sha  string
		want string
	}{
		{sha: "0123456789abcdef0123456789abcdef01234567", want: "01234567"},
		{sha: "abc", want: "abc"},
		{sha: "", want: ""},
	}
	for _, testCase := range cases {
		t.Run("sha "+testCase.sha, func(t *testing.T) {
			if got := ShortSHA(testCase.sha); got != testCase.want {
				t.Errorf("ShortSHA(%q) = %q, want %q", testCase.sha, got, testCase.want)
			}
		})
	}
}

// TestPNG_Encoded_DecodesAsAOneByOneImage checks the bytes are a PNG of the
// promised size, since GitLab validates an avatar before storing it.
func TestPNG_Encoded_DecodesAsAOneByOneImage(t *testing.T) {
	data, err := PNG()
	if err != nil {
		t.Fatalf("PNG() error = %v", err)
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("the bytes do not decode as a PNG: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != 1 || bounds.Dy() != 1 {
		t.Errorf("image is %dx%d, want 1x1", bounds.Dx(), bounds.Dy())
	}
}

// TestBranchOf_Commit_ReadsTheSHAThroughThePointer checks the one field a
// branch is read through a pointer for.
func TestBranchOf_Commit_ReadsTheSHAThroughThePointer(t *testing.T) {
	got := branchOf(&gl.Branch{Name: "feature", Commit: &gl.Commit{ID: "abc"}})
	if got != (Branch{Name: "feature", SHA: "abc"}) {
		t.Errorf("branchOf() = %+v, want feature at abc", got)
	}
	if bare := branchOf(&gl.Branch{Name: "bare"}); bare.SHA != "" {
		t.Errorf("branchOf(no commit).SHA = %q, want empty", bare.SHA)
	}
}
