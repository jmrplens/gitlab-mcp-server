package main

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// TestCommittedRecord_ReadsWhatHEADHolds verifies the three answers the
// comparison can be given: the record HEAD holds, decoded; git's refusal,
// passed on as it came; and bytes HEAD holds that are not a record of this
// build, refused by the decoder rather than compared.
func TestCommittedRecord_ReadsWhatHEADHolds(t *testing.T) {
	doc := orbitrecord.Canonical(orbitrecord.Document{SchemaVersion: orbitrecord.SchemaVersion, Source: orbitrecord.Source{Namespace: "plens1"}})
	answering := func(raw []byte, err error) genRun {
		return genRun{dir: "docs/development", git: func(_ context.Context, dir string, args ...string) ([]byte, error) {
			if dir != "docs/development" || !reflect.DeepEqual(args, []string{"show", "HEAD:./orbit-responses.json"}) {
				t.Errorf("git ran in %q with %q, want show of the record at HEAD in the record's directory", dir, args)
			}
			return raw, err
		}}
	}

	got, err := committedRecord(t.Context(), answering(orbitrecord.Encode(doc), nil))
	if err != nil || !reflect.DeepEqual(got, doc) {
		t.Errorf("committedRecord() = %+v, %v, want the committed record", got, err)
	}
	refusal := errors.New("fatal: path does not exist in 'HEAD'")
	if _, err = committedRecord(t.Context(), answering(nil, refusal)); !errors.Is(err, refusal) {
		t.Errorf("committedRecord() error = %v, want git's refusal", err)
	}
	if _, err = committedRecord(t.Context(), answering([]byte("{"), nil)); err == nil || !strings.Contains(err.Error(), "decoding the Orbit response record") {
		t.Errorf("committedRecord() of bytes that are no record = %v, want the decoder's refusal", err)
	}
}

// TestRunGit_SaysWhatGitAnsweredOrWhyItCouldNotRun verifies the runner against
// the real git, in the four states a recording can meet: git answers, git runs
// and refuses (outside any repository, which is how a -dir outside the
// checkout reads), git cannot be started in a directory that does not exist,
// and there is no git at all. No commit is made: a repository with one would
// need an identity this test has no business setting.
func TestRunGit_SaysWhatGitAnsweredOrWhyItCouldNotRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()

	output, err := runGit(t.Context(), dir, "--version")
	if err != nil || !strings.HasPrefix(string(output), "git version") {
		t.Errorf("runGit(--version) = %q, %v, want git's version", output, err)
	}
	if _, err = runGit(t.Context(), dir, "rev-parse", "--git-dir"); err == nil || !strings.Contains(err.Error(), "git rev-parse --git-dir: fatal: not a git repository") {
		t.Errorf("runGit() outside a repository = %v, want git's refusal with its message", err)
	}
	if _, err = runGit(t.Context(), filepath.Join(dir, "missing"), "--version"); err == nil || !strings.HasPrefix(err.Error(), "run git: ") {
		t.Errorf("runGit() in a directory that does not exist = %v, want the failure to start", err)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err = runGit(t.Context(), dir, "--version"); err == nil || !strings.HasPrefix(err.Error(), "find git: ") {
		t.Errorf("runGit() with no git on PATH = %v, want the lookup's failure", err)
	}
}
