//go:build e2e

// repository_test.go pins the commit retry rules and the content helpers.

package fixture

import (
	"bytes"
	"errors"
	"image/png"
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
