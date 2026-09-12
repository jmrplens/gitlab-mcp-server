//go:build e2e

// repository.go builds what lives inside a project's repository: branches,
// commits, and the bytes a test needs to upload.
//
// A commit is the one write GitLab is slowest to admit it is ready for. A
// branch the API has just created is not yet one the commits API will write
// to, and the refusal it gives ("you can only create or edit files when you
// are on a branch") is answered here by naming a start branch and trying
// again, which is what the suite this replaces learned to do.

package fixture

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// Branch is a branch a builder created.
type Branch struct {
	// Name is the branch name.
	Name string
	// SHA is the commit it pointed at when it was created.
	SHA string
}

// Commit is a commit a builder made, with the file it wrote.
type Commit struct {
	// SHA is the full commit identifier.
	SHA string
	// ShortID is the abbreviated identifier GitLab shows.
	ShortID string
	// Branch is the branch it was made on.
	Branch string
	// FilePath is the file it wrote.
	FilePath string
}

// NewBranch creates a branch from the project's default branch and registers
// nothing: the branch goes with the project.
func NewBranch(e *harness.Env, project Project, name string) Branch {
	e.T.Helper()
	return NewBranchFrom(e, project, name, project.DefaultBranch)
}

// NewBranchFrom creates a branch from ref, retrying while the ref it starts
// from is still becoming visible.
func NewBranchFrom(e *harness.Env, project Project, name, ref string) Branch {
	e.T.Helper()

	branch, err := retryTransient(e, "create branch "+name, createRetries, func() (Branch, error) {
		created, _, err := e.Client().GL().Branches.CreateBranch(project.ID, &gl.CreateBranchOptions{
			Branch: new(name),
			Ref:    new(ref),
		}, gl.WithContext(e.Ctx))
		if err != nil {
			return Branch{}, err
		}
		return branchOf(created), nil
	})
	if err != nil {
		e.T.Fatalf("creating branch %q from %q in project %d: %v", name, ref, project.ID, err)
	}
	return branch
}

// branchOf reads what a test needs out of what GitLab returned.
func branchOf(b *gl.Branch) Branch {
	branch := Branch{Name: b.Name}
	if b.Commit != nil {
		branch.SHA = b.Commit.ID
	}
	return branch
}

// CommitFile writes content to path on branch, creating the file or updating
// it, and returns the commit. A file that already holds exactly that content
// returns the commit that wrote it rather than an empty one.
//
// Create-or-update rather than create, because the same fixture is asked for
// by more than one test against the World, and because GitLab refuses an
// update that changes nothing ("has not been changed") just as it refuses a
// create of a file that exists.
func CommitFile(e *harness.Env, project Project, branch, path, content, message string) Commit {
	e.T.Helper()

	commit, err := commitAction(e, project, branch, path, content, message, gl.FileCreate)
	if err == nil {
		return commit
	}
	if !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		e.T.Fatalf("committing %s on %q in project %d: %v", path, branch, project.ID, err)
	}

	existing, existingContent, err := readFile(e, project, branch, path)
	if err != nil {
		e.T.Fatalf("reading the existing %s on %q in project %d: %v", path, branch, project.ID, err)
	}
	if existingContent == content {
		return existing
	}

	commit, err = commitAction(e, project, branch, path, content, message, gl.FileUpdate)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "not been changed") {
		return existing
	}
	if err != nil {
		e.T.Fatalf("updating %s on %q in project %d: %v", path, branch, project.ID, err)
	}
	return commit
}

// commitAction makes one commit carrying one file action, retrying the two
// answers GitLab gives while a branch is still settling.
//
// The first, "only create or edit files when you are on a branch", means the
// commits API cannot see the branch yet; naming the default branch as the
// start branch gets the commit accepted, and is dropped again on the next
// attempt if GitLab then answers that the branch "already exists", which is
// its way of saying the start branch is no longer needed.
func commitAction(e *harness.Env, project Project, branch, path, content, message string, action gl.FileActionValue) (Commit, error) {
	e.T.Helper()

	needStartBranch := false
	return harness.Retry(e.Ctx, e.T, "commit "+path, commitRetries, retryBaseDelay, func(int) (Commit, bool, string, error) {
		opts := &gl.CreateCommitOptions{
			Branch:        new(branch),
			CommitMessage: new(message),
			Actions: []*gl.CommitActionOptions{
				{Action: new(action), FilePath: new(path), Content: new(content)},
			},
		}
		if needStartBranch && branch != project.DefaultBranch {
			opts.StartBranch = new(project.DefaultBranch)
		}
		created, _, err := e.Client().GL().Commits.CreateCommit(project.ID, opts, gl.WithContext(e.Ctx))
		if err == nil {
			return Commit{SHA: created.ID, ShortID: created.ShortID, Branch: branch, FilePath: path}, false, "", nil
		}
		return Commit{}, commitRetryable(err, &needStartBranch), commitRetryReason(err, needStartBranch), err
	})
}

// commitRetryable classifies a commit failure and flips the start-branch
// switch the next attempt reads.
func commitRetryable(err error, needStartBranch *bool) bool {
	message := err.Error()
	switch {
	case strings.Contains(message, "only create or edit files when you are on a branch"):
		*needStartBranch = true
		return true
	case *needStartBranch && strings.Contains(message, "already exists"):
		*needStartBranch = false
		return true
	default:
		return IsTransientNetwork(err)
	}
}

// commitRetryReason names a retried commit failure for the log.
func commitRetryReason(err error, needStartBranch bool) string {
	switch {
	case strings.Contains(err.Error(), "only create or edit files when you are on a branch"):
		return "branch not ready, naming a start branch"
	case needStartBranch:
		return "branch exists now, dropping the start branch"
	default:
		return "transient network error"
	}
}

// readFile returns the commit that last wrote path on branch and the file's
// decoded content.
func readFile(e *harness.Env, project Project, branch, path string) (Commit, string, error) {
	e.T.Helper()

	file, _, err := e.Client().GL().RepositoryFiles.GetFile(project.ID, path, &gl.GetFileOptions{Ref: new(branch)}, gl.WithContext(e.Ctx))
	if err != nil {
		return Commit{}, "", err
	}
	content, err := base64.StdEncoding.DecodeString(file.Content)
	if err != nil {
		return Commit{}, "", fmt.Errorf("decoding %s: %w", path, err)
	}
	return Commit{SHA: file.LastCommitID, ShortID: ShortSHA(file.LastCommitID), Branch: branch, FilePath: path}, string(content), nil
}

// ShortSHA abbreviates a commit identifier the way GitLab's short_id does.
func ShortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// PNG returns a freshly encoded 1x1 PNG, for an avatar or an upload, so no
// binary fixture has to live on disk.
func PNG() ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		return nil, fmt.Errorf("encoding a 1x1 png: %w", err)
	}
	return buf.Bytes(), nil
}

// PNGBase64 is PNG encoded for a content_base64 input, failing the test if
// the encoder ever refuses a 1x1 image.
func PNGBase64(e *harness.Env) string {
	e.T.Helper()
	data, err := PNG()
	if err != nil {
		e.T.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(data)
}
