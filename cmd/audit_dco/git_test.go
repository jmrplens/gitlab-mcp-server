// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// git_test.go covers how a range is read from git: the records git log
// prints, the check against the hashes git rev-list lists, which merges bring
// no change of their own, and the answers of a real repository, git's own
// sign-off and git's errors included.
package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// gitFixture runs git in dir as author on behalf of a test and returns what
// it printed on stdout, trimmed. It is isolated from the machine's and the
// user's configuration, so no signing key, hook or template of theirs reaches
// the fixture.
func gitFixture(t *testing.T, dir string, author identity, args ...string) string {
	t.Helper()

	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(context.Background(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+global, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME="+author.name, "GIT_AUTHOR_EMAIL="+author.email,
		"GIT_COMMITTER_NAME="+author.name, "GIT_COMMITTER_EMAIL="+author.email)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// newRepository creates an empty repository in a temporary directory and
// returns it.
func newRepository(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	gitFixture(t, dir, jane, "init", "--quiet")
	return dir
}

// commitAs makes an empty commit in dir as author, with message passed to git
// through a file and kept verbatim so that it reaches the commit byte for
// byte, and returns the commit.
func commitAs(t *testing.T, dir string, author identity, message string) string {
	t.Helper()

	file := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(file, []byte(message), 0o600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, dir, author, "commit", "--quiet", "--allow-empty", "--no-gpg-sign", "--cleanup=verbatim", "--file", file)
	return gitFixture(t, dir, author, "rev-parse", "HEAD")
}

// signedMessage is a commit message whose subject is followed by a
// Signed-off-by trailer naming who.
func signedMessage(subject string, who identity) string {
	return subject + "\n\nSigned-off-by: " + who.String() + "\n"
}

// commitFile writes content to name in dir, stages it and commits it as
// author with message, and returns the commit.
func commitFile(t *testing.T, dir string, author identity, name, content, message string) string {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, dir, author, "add", "--", name)
	return commitAs(t, dir, author, message)
}

// mergeWithChange merges branch into the branch checked out in dir as author,
// adds a file of its own to the merge before committing it with message, and
// returns the merge, which therefore brings a change of its own.
func mergeWithChange(t *testing.T, dir string, author identity, branch, message string) string {
	t.Helper()

	gitFixture(t, dir, author, "merge", "--quiet", "--no-ff", "--no-commit", branch)
	if err := os.WriteFile(filepath.Join(dir, "payload.txt"), []byte("added while merging\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, dir, author, "add", "--", "payload.txt")
	return commitAs(t, dir, author, message)
}

// TestParseLog_Records_ReadEachCommitAndItsSignOffs verifies how the records
// git log prints are read: the hash, the parents counted, the author, the
// subject and every Signed-off-by value in order, with the newline git puts
// between records and the empty field of a commit without trailers ignored.
func TestParseLog_Records_ReadEachCommitAndItsSignOffs(t *testing.T) {
	out := "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x00Jane Doe <jane@example.org>\x1e\n" +
		"bbb\x00aaa ccc\x00John Roe\x00john@example.org\x00Merge\x00\x1e\n" +
		"ccc\x00\x00Jane Doe\x00jane@example.org\x00root\x00John Roe <john@example.org>\x00Jane Doe <jane@example.org>\x1e\n"
	got, err := parseLog(out)
	if err != nil {
		t.Fatalf("parseLog() error = %v", err)
	}
	want := []commit{
		{hash: "aaa", parents: 1, author: jane, subject: "feat: one", signoffs: []string{"Jane Doe <jane@example.org>"}},
		{hash: "bbb", parents: 2, author: john, subject: "Merge"},
		{hash: "ccc", parents: 0, author: jane, subject: "root", signoffs: []string{"John Roe <john@example.org>", "Jane Doe <jane@example.org>"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLog() = %#v\nwant %#v", got, want)
	}
}

// TestParseLog_NoRecords_ReadsNoCommits verifies that an empty range, which
// git log prints nothing for, reads as no commits rather than an error.
func TestParseLog_NoRecords_ReadsNoCommits(t *testing.T) {
	got, err := parseLog("")
	if err != nil || len(got) != 0 {
		t.Errorf("parseLog(\"\") = %#v, %v; want no commits and no error", got, err)
	}
}

// TestParseLog_SeparatorInAField_StaysInTheField verifies that the record
// separator inside a field, in the subject or at the end of the last trailer,
// is read as part of that field: a record ends only where the separator is
// followed by the newline git prints after every commit, and no field git
// prints holds a newline.
func TestParseLog_SeparatorInAField_StaysInTheField(t *testing.T) {
	out := "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x1etwo\x00Jane Doe <jane@example.org>\x1e\x1e\n" +
		"bbb\x00aaa\x00Jane Doe\x00jane@example.org\x00fix: three\x00\x1e\n"
	got, err := parseLog(out)
	if err != nil {
		t.Fatalf("parseLog() error = %v", err)
	}
	want := []commit{
		{hash: "aaa", parents: 1, author: jane, subject: "feat: one\x1etwo", signoffs: []string{"Jane Doe <jane@example.org>\x1e"}},
		{hash: "bbb", parents: 1, author: jane, subject: "fix: three"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLog() = %#v\nwant %#v", got, want)
	}
}

// TestParseLog_ShortRecord_IsRefused verifies that a record with fewer fields
// than every commit has stops the read instead of being taken as a commit.
// Five fields is one short, since the sixth always exists, empty for a commit
// without trailers, and an empty record is one field.
func TestParseLog_ShortRecord_IsRefused(t *testing.T) {
	tests := []struct{ name, out, wantErr string }{
		{name: "five fields", out: "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x1e\n", wantErr: "a record of 5 field(s) where every commit has at least 6"},
		{name: "an empty record", out: "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x00\x1e\n\x1e\n", wantErr: "a record of 1 field(s) where every commit has at least 6"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLog(tc.out)
			if err == nil || got != nil {
				t.Fatalf("parseLog(%q) = %#v, %v; want an error and no commits", tc.out, got, err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to hold %q", err, tc.wantErr)
			}
		})
	}
}

// TestParseLog_UnterminatedRecord_IsRefused verifies that text after the last
// record end, which git never prints since it ends every commit's record with
// a newline, stops the read rather than being taken as a record.
func TestParseLog_UnterminatedRecord_IsRefused(t *testing.T) {
	out := "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x00\x1e\nbbb\x00aaa\x00Jane Doe\x00jane@example.org\x00fix: two\x00\x1e"
	got, err := parseLog(out)
	if err == nil || got != nil {
		t.Fatalf("parseLog(%q) = %#v, %v; want an error and no commits", out, got, err)
	}
	if !strings.Contains(err.Error(), "where every record ends with a record separator and a newline") {
		t.Errorf("error = %q, want it to say how a record ends", err)
	}
}

// TestReadLog_Output_IsHeldToTheHashesGitRevListListed verifies that what git
// log printed is read only when it parses and lists the hashes git rev-list
// listed for the same range: a record that does not parse, or a commit list
// that departs from the hashes, stops the read.
func TestReadLog_Output_IsHeldToTheHashesGitRevListListed(t *testing.T) {
	out := "aaa\x00ppp\x00Jane Doe\x00jane@example.org\x00feat: one\x00Jane Doe <jane@example.org>\x1e\n"
	t.Run("agreeing", func(t *testing.T) {
		got, err := readLog([]string{"aaa"}, out)
		want := []commit{{hash: "aaa", parents: 1, author: jane, subject: "feat: one", signoffs: []string{"Jane Doe <jane@example.org>"}}}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("readLog() = %#v, %v; want %#v and no error", got, err, want)
		}
	})
	tests := []struct {
		name    string
		hashes  []string
		out     string
		wantErr string
	}{
		{name: "a record that does not parse", hashes: []string{"aaa"}, out: "aaa\x1e\n", wantErr: "a record of 1 field(s)"},
		{name: "a commit rev-list did not list", hashes: []string{"bbb"}, out: out, wantErr: "commit 1 is bbb to git rev-list and aaa to git log"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := readLog(tc.hashes, tc.out)
			if err == nil || got != nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("readLog() = %#v, %v; want no commits and an error holding %q", got, err, tc.wantErr)
			}
		})
	}
}

// TestMatchHashes_Lists_AgreeOnlyWhenEveryHashMatchesInOrder verifies the
// check of the commits git log printed against the hashes git rev-list
// listed: the same hashes in the same order agree, and a missing, an extra
// or a different hash does not.
func TestMatchHashes_Lists_AgreeOnlyWhenEveryHashMatchesInOrder(t *testing.T) {
	read := []commit{{hash: "aaa"}, {hash: "bbb"}}
	tests := []struct {
		name    string
		hashes  []string
		wantErr string
	}{
		{name: "same hashes in order", hashes: []string{"aaa", "bbb"}},
		{name: "one more listed", hashes: []string{"aaa", "bbb", "ccc"}, wantErr: "git rev-list lists 3 commit(s) and git log printed 2"},
		{name: "one fewer listed", hashes: []string{"aaa"}, wantErr: "git rev-list lists 1 commit(s) and git log printed 2"},
		{name: "another hash", hashes: []string{"aaa", "ccc"}, wantErr: "commit 2 is ccc to git rev-list and bbb to git log"},
		{name: "another order", hashes: []string{"bbb", "aaa"}, wantErr: "commit 1 is bbb to git rev-list and aaa to git log"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := matchHashes(tc.hashes, read)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("matchHashes(%q) = %v, want nil", tc.hashes, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("matchHashes(%q) = %v, want an error containing %q", tc.hashes, err, tc.wantErr)
			}
		})
	}
}

// TestListCommits_Repository_ReadsTheRangeOldestFirst verifies the read of a
// real repository: the commits after base, oldest first, each with its
// author, its subject, its parents and its Signed-off-by values as git finds
// them, which includes the trailer git commit --signoff writes, a key written
// in lower case and a Signed-off-by line that shares the message's last
// paragraph with a line of prose, and leaves out a Signed-off-by line outside
// that paragraph.
func TestListCommits_Repository_ReadsTheRangeOldestFirst(t *testing.T) {
	dir := newRepository(t)
	base := commitAs(t, dir, jane, "root\n")
	first := commitAs(t, dir, jane, signedMessage("feat: first", jane))
	gitFixture(t, dir, john, "commit", "--quiet", "--allow-empty", "--no-gpg-sign", "--signoff", "--message", "fix: second")
	second := gitFixture(t, dir, john, "rev-parse", "HEAD")
	third := commitAs(t, dir, jane, "docs: third\n\nsigned-off-by: Jane Doe <jane@example.org>\n")
	fourth := commitAs(t, dir, jane, "chore: fourth\n\nSigned-off-by: Jane Doe <jane@example.org> is not a trailer here.\n\nThe last paragraph is prose.\n")
	fifth := commitAs(t, dir, jane, "test: fifth\n\nSigned-off-by: Jane Doe <jane@example.org>\nand a line of prose in the same last paragraph\n")

	got, err := listCommits(context.Background(), dir, base, fifth)
	if err != nil {
		t.Fatalf("listCommits() error = %v", err)
	}
	want := []commit{
		{hash: first, parents: 1, author: jane, subject: "feat: first", signoffs: []string{"Jane Doe <jane@example.org>"}},
		{hash: second, parents: 1, author: john, subject: "fix: second", signoffs: []string{"John Roe <john@example.org>"}},
		{hash: third, parents: 1, author: jane, subject: "docs: third", signoffs: []string{"Jane Doe <jane@example.org>"}},
		{hash: fourth, parents: 1, author: jane, subject: "chore: fourth"},
		{hash: fifth, parents: 1, author: jane, subject: "test: fifth", signoffs: []string{"Jane Doe <jane@example.org>"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listCommits() = %#v\nwant %#v", got, want)
	}
}

// TestListCommits_MergedBranch_CountsTheMergesParents verifies that a merge
// commit is read with both of its parents, and the commit of the merged
// branch with its own one.
func TestListCommits_MergedBranch_CountsTheMergesParents(t *testing.T) {
	dir := newRepository(t)
	base := commitAs(t, dir, jane, "root\n")
	trunk := gitFixture(t, dir, jane, "symbolic-ref", "--short", "HEAD")
	gitFixture(t, dir, jane, "checkout", "--quiet", "-b", "side")
	side := commitAs(t, dir, jane, signedMessage("feat: side", jane))
	gitFixture(t, dir, jane, "checkout", "--quiet", trunk)
	onTrunk := commitAs(t, dir, jane, signedMessage("feat: trunk", jane))
	gitFixture(t, dir, jane, "merge", "--quiet", "--no-ff", "--no-edit", "--no-gpg-sign", "side")
	merged := gitFixture(t, dir, jane, "rev-parse", "HEAD")

	got, err := listCommits(context.Background(), dir, base, merged)
	if err != nil {
		t.Fatalf("listCommits() error = %v", err)
	}
	parents := map[string]int{}
	for _, each := range got {
		parents[each.hash] = each.parents
	}
	if want := map[string]int{side: 1, onTrunk: 1, merged: 2}; !reflect.DeepEqual(parents, want) {
		t.Errorf("parents by commit = %v, want %v", parents, want)
	}
}

// TestListCommits_SeparatorInAMessage_IsReadAsGitReadsIt verifies that a
// message carrying the byte the read separates records with is read the way
// git reads it, every value in its own field and on its own commit: in the
// subject, inside a trailer value, at the end of the last one, and followed by
// trailers that would parse as a record of their own if the byte ended one.
func TestListCommits_SeparatorInAMessage_IsReadAsGitReadsIt(t *testing.T) {
	dir := newRepository(t)
	base := commitAs(t, dir, jane, "root\n")
	message := "feat: one\x1etwo\n\nSigned-off-by: x\x1ey\n" + strings.Repeat("Signed-off-by: Jane Doe <jane@example.org>\n", 5) +
		"Signed-off-by: John Roe <john@example.org>\x1e\n"
	head := commitAs(t, dir, john, message)

	got, err := listCommits(context.Background(), dir, base, head)
	if err != nil {
		t.Fatalf("listCommits() error = %v", err)
	}
	signoffs := append(append([]string{"x\x1ey"}, slices.Repeat([]string{"Jane Doe <jane@example.org>"}, 5)...), "John Roe <john@example.org>\x1e")
	want := []commit{{hash: head, parents: 1, author: john, subject: "feat: one\x1etwo", signoffs: signoffs}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("listCommits() = %#v\nwant %#v", got, want)
	}
}

// TestMarkCleanMerges_Merges_MarkOnlyTheOnesThatBringNoChange verifies which
// commits are marked as merges bringing no change of their own: a merge of
// two parents whose tree is the one git's own merge of them makes is, and a
// merge that adds a change while merging, one that keeps one side and drops
// the other's change, a merge of three parents, which git cannot remerge, and
// every commit that is not a merge are not.
func TestMarkCleanMerges_Merges_MarkOnlyTheOnesThatBringNoChange(t *testing.T) {
	dir := newRepository(t)
	base := commitAs(t, dir, jane, "root\n")
	trunk := gitFixture(t, dir, jane, "symbolic-ref", "--short", "HEAD")
	// commitNamed commits a file of its own on the branch checked out, and
	// branch commits one on a new branch from the trunk and goes back to it.
	commitNamed := func(name string) {
		commitFile(t, dir, jane, name+".txt", name+"\n", signedMessage("feat: "+name, jane))
	}
	branch := func(name string) {
		gitFixture(t, dir, jane, "checkout", "--quiet", "-b", name)
		commitNamed(name)
		gitFixture(t, dir, jane, "checkout", "--quiet", trunk)
	}
	merge := func(args ...string) string {
		gitFixture(t, dir, jane, append([]string{"merge", "--quiet", "--no-ff", "--no-edit", "--no-gpg-sign"}, args...)...)
		return gitFixture(t, dir, jane, "rev-parse", "HEAD")
	}

	branch("clean")
	commitNamed("one")
	clean := merge("clean")
	branch("changed")
	commitNamed("two")
	changed := mergeWithChange(t, dir, jane, "changed", "Merge branch 'changed'\n")
	branch("dropped")
	commitNamed("three")
	dropped := merge("--strategy=ours", "dropped")
	branch("left")
	branch("right")
	commitNamed("four")
	octopus := merge("left", "right")

	commits, err := listCommits(context.Background(), dir, base, octopus)
	if err != nil {
		t.Fatalf("listCommits() error = %v", err)
	}
	if err = markCleanMerges(context.Background(), dir, commits); err != nil {
		t.Fatalf("markCleanMerges() error = %v", err)
	}
	marked := map[string]bool{}
	parents := map[string]int{}
	for _, each := range commits {
		if each.cleanMerge {
			marked[each.hash] = true
		}
		parents[each.hash] = each.parents
	}
	if want := map[string]bool{clean: true}; !reflect.DeepEqual(marked, want) {
		t.Errorf("merges marked as bringing no change = %v, want %v alone", marked, clean)
	}
	if got := []int{parents[clean], parents[changed], parents[dropped], parents[octopus]}; !slices.Equal(got, []int{2, 2, 2, 3}) {
		t.Errorf("parents of the clean, changed, dropped and octopus merges = %v, want [2 2 2 3]", got)
	}
}

// TestMarkCleanMerges_MissingTree_ReturnsWhatGitShowSaid verifies that a merge
// git cannot remerge, because the clone lacks a tree one of its parents
// records, fails with git's own reason, which names the tree, rather than
// being taken for a merge that brings a change, or none.
func TestMarkCleanMerges_MissingTree_ReturnsWhatGitShowSaid(t *testing.T) {
	dir, base, head, tree := mergeMissingATree(t)

	commits, err := listCommits(context.Background(), dir, base, head)
	if err != nil {
		t.Fatalf("listCommits() error = %v", err)
	}
	err = markCleanMerges(context.Background(), dir, commits)
	if err == nil || !strings.HasPrefix(err.Error(), "git show") || !strings.Contains(err.Error(), tree) {
		t.Errorf("markCleanMerges() = %v, want git show's own reason naming the tree %s", err, tree)
	}
}

// mergeMissingATree is a repository whose root is followed by a merge of a
// branch that adds a file, with the tree of that branch's commit removed from
// the object store, and the root, the merge and the tree removed. git
// rev-list and git log read only commits, so they still answer; remerging
// needs the tree.
func mergeMissingATree(t *testing.T) (dir, base, head, tree string) {
	t.Helper()

	dir = newRepository(t)
	base = commitAs(t, dir, jane, "root\n")
	trunk := gitFixture(t, dir, jane, "symbolic-ref", "--short", "HEAD")
	gitFixture(t, dir, jane, "checkout", "--quiet", "-b", "side")
	commitFile(t, dir, jane, "side.txt", "side\n", signedMessage("feat: side", jane))
	tree = gitFixture(t, dir, jane, "rev-parse", "HEAD^{tree}")
	gitFixture(t, dir, jane, "checkout", "--quiet", trunk)
	commitAs(t, dir, jane, signedMessage("feat: trunk", jane))
	gitFixture(t, dir, jane, "merge", "--quiet", "--no-ff", "--no-edit", "--no-gpg-sign", "side")
	head = gitFixture(t, dir, jane, "rev-parse", "HEAD")
	if err := os.Remove(filepath.Join(dir, ".git", "objects", tree[:2], tree[2:])); err != nil {
		t.Fatal(err)
	}
	return dir, base, head, tree
}

// TestListCommits_UnknownRevision_ReturnsWhatGitSaid verifies that a range
// git cannot read fails with git's own reason, from git rev-list first and
// then from git log.
func TestListCommits_UnknownRevision_ReturnsWhatGitSaid(t *testing.T) {
	dir := newRepository(t)
	head := commitAs(t, dir, jane, "root\n")
	missing := strings.Repeat("0", 40)

	got, err := listCommits(context.Background(), dir, missing, head)
	if err == nil || got != nil {
		t.Fatalf("listCommits() = %#v, %v; want an error and no commits", got, err)
	}
	if !strings.HasPrefix(err.Error(), "git rev-list") || !strings.Contains(err.Error(), "fatal:") {
		t.Errorf("error = %q, want git rev-list's own reason", err)
	}
}

// TestListCommits_LogFails_ReturnsWhatGitLogSaid verifies the second read's
// failure: a range git rev-list reads and git log does not. A repository
// whose configuration names an unknown log.showSignature value is one, since
// only git log reads that setting.
func TestListCommits_LogFails_ReturnsWhatGitLogSaid(t *testing.T) {
	dir := newRepository(t)
	base := commitAs(t, dir, jane, "root\n")
	head := commitAs(t, dir, jane, signedMessage("feat: one", jane))
	gitFixture(t, dir, jane, "config", "log.showSignature", "maybe")

	got, err := listCommits(context.Background(), dir, base, head)
	if err == nil || got != nil {
		t.Fatalf("listCommits() = %#v, %v; want an error and no commits", got, err)
	}
	if !strings.HasPrefix(err.Error(), "git log") || !strings.Contains(err.Error(), "fatal:") {
		t.Errorf("error = %q, want git log's own reason", err)
	}
}

// TestResolveCommit_Revisions_NameTheCommitOrSayWhyNot verifies that a
// revision resolves to the full hash of the commit it names, a tag included,
// and that one naming nothing fails with what git said.
func TestResolveCommit_Revisions_NameTheCommitOrSayWhyNot(t *testing.T) {
	dir := newRepository(t)
	head := commitAs(t, dir, jane, "root\n")
	gitFixture(t, dir, jane, "tag", "--annotate", "--no-sign", "--message", "v1", "v1")

	for _, rev := range []string{"HEAD", "v1", head} {
		t.Run(rev, func(t *testing.T) {
			got, err := resolveCommit(context.Background(), dir, rev)
			if err != nil || got != head {
				t.Errorf("resolveCommit(%q) = %q, %v; want %q", rev, got, err, head)
			}
		})
	}
	t.Run("unknown", func(t *testing.T) {
		got, err := resolveCommit(context.Background(), dir, "no-such-branch")
		if err == nil || got != "" || !strings.Contains(err.Error(), "git rev-parse") {
			t.Errorf("resolveCommit(no-such-branch) = %q, %v; want git rev-parse's error", got, err)
		}
	})
}

// TestRunGit_Failure_CarriesWhatGitSaidOnStderr verifies that a failing git
// command is reported with its arguments and the reason git printed on
// stderr, which is the only place git says why.
func TestRunGit_Failure_CarriesWhatGitSaidOnStderr(t *testing.T) {
	out, err := runGit(context.Background(), t.TempDir(), "rev-parse", "--verify", "HEAD")
	if err == nil || out != "" {
		t.Fatalf("runGit() = %q, %v; want an error outside a repository", out, err)
	}
	if !strings.Contains(err.Error(), "git rev-parse --verify HEAD") || !strings.Contains(strings.ToLower(err.Error()), "not a git repository") {
		t.Errorf("error = %q, want the command and git's reason", err)
	}
}

// TestRunGit_NoGitOnPath_IsTheLookupsFailure verifies that git is found on
// PATH before it is run, and that a PATH holding none is reported as the
// lookup's failure rather than as a git that ran and refused.
func TestRunGit_NoGitOnPath_IsTheLookupsFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	if _, err := runGit(context.Background(), t.TempDir(), "--version"); err == nil || !strings.HasPrefix(err.Error(), "find git: ") {
		t.Errorf("runGit() with no git on PATH = %v, want the lookup's failure", err)
	}
}
