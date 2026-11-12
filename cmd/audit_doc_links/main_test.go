// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// main_test.go holds the command to what the script answered: over a corpus
// written to cover every construct the check reads, the answer
// scripts/check-doc-links.mjs printed is the answer this command prints,
// byte for byte, and over this repository it reads every tracked page.
package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// layOut writes files, keyed by their slash path, below a new directory and
// returns it.
func layOut(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //#nosec G703 -- a path below the test's own t.TempDir
			t.Fatal(err)
		}
	}
	return root
}

// readCorpus reads testdata/corpus.txtar into its files: each starts after a
// "-- path --" line and runs to the next, and the lines before the first are
// a comment.
func readCorpus(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "corpus.txtar"))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	name := ""
	for line := range strings.Lines(string(data)) {
		if strings.HasPrefix(line, "-- ") && strings.HasSuffix(line, " --\n") {
			name = strings.TrimSuffix(strings.TrimPrefix(line, "-- "), " --\n")
			files[name] = ""
			continue
		}
		if name != "" {
			files[name] += line
		}
	}
	return files
}

// check runs the command over the repository at root.
func check(root string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(root, &out, &errOut)
	return code, out.String(), errOut.String()
}

// TestRun_TheCorpus_AnswersWhatTheScriptAnswered holds the port to the
// record of the script it replaces: every heading dialect, fence, code span,
// reference, relative link, site address and served file of the corpus is
// answered as scripts/check-doc-links.mjs answered it, in the same order and
// the same words.
func TestRun_TheCorpus_AnswersWhatTheScriptAnswered(t *testing.T) {
	t.Parallel()

	golden, err := os.ReadFile(filepath.Join("testdata", "corpus.golden"))
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := check(layOut(t, readCorpus(t)))
	if code != 1 || stdout != "" || stderr != string(golden) {
		t.Errorf("exit %d, stdout %q, stderr\n%s\nwant 1, nothing and\n%s", code, stdout, stderr, golden)
	}
}

// TestRun_ACleanTree_SaysWhatItChecked verifies the passing answer, which
// counts the files and the anchors it judged rather than only saying nothing
// was wrong.
func TestRun_ACleanTree_SaysWhatItChecked(t *testing.T) {
	t.Parallel()

	root := layOut(t, map[string]string{
		"README.md":     "# Readme\n\n[guide](docs/guide.md#usage) [self](#readme)\n",
		"docs/guide.md": "# Guide\n\n## Usage\n",
	})
	code, stdout, stderr := check(root)
	want := "Checked 2 Markdown/MDX files; local links are valid, including 2 anchors.\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Errorf("exit %d, stdout %q, stderr %q, want 0 and %q", code, stdout, stderr, want)
	}
}

// TestRun_ADocumentationTreeThatCannotBeRead_Fails verifies that a walk that
// cannot list the tree, and a page that cannot be read, stop the run with
// the reason rather than passing what was never read.
func TestRun_ADocumentationTreeThatCannotBeRead_Fails(t *testing.T) {
	t.Parallel()

	t.Run("an unstattable root", func(t *testing.T) {
		t.Parallel()

		if code, _, stderr := check("docs\x00"); code != 1 || !strings.HasPrefix(stderr, "ERROR: list the documentation: ") {
			t.Errorf("exit %d, stderr %q, want 1 and the walk's error", code, stderr)
		}
	})
	t.Run("a dangling page", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		if err := os.Symlink(filepath.Join(root, "absent.md"), filepath.Join(root, "broken.md")); err != nil {
			t.Skipf("this platform does not let the test make a link: %v", err)
		}
		if code, _, stderr := check(root); code != 1 || !strings.HasPrefix(stderr, "ERROR: read broken.md: ") {
			t.Errorf("exit %d, stderr %q, want 1 and the read error", code, stderr)
		}
	})
}

// TestDocuments_TheRoots_AreTheRepositoryAndItsTwoDocumentationDotDirectories
// verifies which files the check reads: the repository's own pages, those of
// .github and .claude, and nothing from another dot-directory, the untracked
// plan area, a build's output or a worktree, sorted as git sorts paths.
func TestDocuments_TheRoots_AreTheRepositoryAndItsTwoDocumentationDotDirectories(t *testing.T) {
	t.Parallel()

	root := layOut(t, map[string]string{
		"README.md":                        "",
		"a/b.md":                           "",
		"a.mdx":                            "",
		".github/skills/x/SKILL.md":        "",
		".claude/agents/a.md":              "",
		".claude/worktrees/one/.git":       "gitdir: elsewhere",
		".claude/worktrees/one/README.md":  "",
		".cache/gitlab-api-docs/issues.md": "",
		"plan/draft.md":                    "",
		"dist/modeleval/notes.md":          "",
		"site/node_modules/pkg/README.md":  "",
		"site/src/content/docs/index.mdx":  "",
		"notes.txt":                        "",
	})
	files, err := documents(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".claude/agents/a.md", ".github/skills/x/SKILL.md", "README.md", "a.mdx", "a/b.md", "site/src/content/docs/index.mdx"}
	if !slices.Equal(files, want) {
		t.Errorf("documents %q, want %q", files, want)
	}
}

// TestDocuments_ThisRepository_ReadsEveryTrackedPage holds the roots to the
// repository: every Markdown and MDX file git tracks outside plan/ is one the
// check reads, so a page added under a directory the roots miss fails here
// rather than going unchecked. It needs git and a checkout, which CI has; off
// CI, a copy of the tree with no .git skips it with the reason.
func TestDocuments_ThisRepository_ReadsEveryTrackedPage(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	out, err := exec.CommandContext(t.Context(), "git", "-C", root, "ls-files", "-z", "*.md", "*.mdx").Output()
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("git ls-files: %v, and CI must run this check rather than skip it", err)
		}
		t.Skipf("not a checkout git can list (%v), so there is no tracked set to hold the walk to", err)
	}
	files, err := documents(root)
	if err != nil {
		t.Fatal(err)
	}
	for tracked := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if strings.HasPrefix(tracked, "plan/") {
			continue
		}
		if _, found := slices.BinarySearch(files, tracked); !found {
			t.Errorf("%s is tracked and the check does not read it", tracked)
		}
	}
}

// TestMain_HandsTheExitCodeToTheProcess covers main, which checks the
// working directory: a tree holding a broken link fails. It changes the
// working directory and the exit seam, so it does not run in parallel.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	previousExit := exitProcess
	t.Cleanup(func() { exitProcess = previousExit })
	t.Chdir(layOut(t, map[string]string{"README.md": "[x](missing.md)\n"}))

	code := -1
	exitProcess = func(c int) { code = c }
	main()
	if code != 1 {
		t.Errorf("main exited %d over a broken link, want 1", code)
	}
}
