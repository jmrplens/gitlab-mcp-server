// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the sixth property: every script in another language than Go the
// repository holds is declared with the reason it has to stay, so a new one
// fails the gate the day it is added.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// undeclaredScript renders the finding a script no declaration names carries.
func undeclaredScript(script string) string {
	return script + ": a script that is not Go, which the table in cmd/audit_supply_chain/declarations.go does not " +
		"declare: repository tooling is a Go command under cmd/, so port it, or declare why it has to stay"
}

// TestAudit_ScriptsOutsideGo_AreHeldToTheTable verifies that the audit finds
// every script in the tree that is not Go, by its extension or, for a file
// with none, by the #! it opens with, and reports each one the table does not
// declare.
//
// It reads the tree the way the other walkers here do, except that the
// dot-directories at the root this repository tracks are read too: .github,
// .claude but the worktrees Claude Code checks out there, .plugin and .vscode,
// any of which a script can be added to and run from. Every other
// dot-directory is a tool's state and another checkout is not this
// repository's source, and the site's own build steps, a node_modules, a dist,
// a testdata, the untracked plan/ and the npm packages the release assembles
// are not tooling this rule is about. Five scripts arrived in the first half
// of October 2026 from the very batches issue 1114 ports scripts away from,
// and nothing said so.
func TestAudit_ScriptsOutsideGo_AreHeldToTheTable(t *testing.T) {
	t.Parallel()

	problems, err := audit(scriptTree(t), tables{})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	want := []string{
		".github/workflows/ci.yml:5: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA",
		undeclaredScript(".claude/hooks/hook.sh"),
		undeclaredScript(".github/scripts/verdict.sh"),
		undeclaredScript(".plugin/hook.sh"),
		undeclaredScript(".vscode/task.sh"),
		undeclaredScript("scripts/install.ps1"),
		undeclaredScript("scripts/install.sh"),
		undeclaredScript("scripts/scratch.sh"),
		undeclaredScript("tools/stamp"),
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// scriptTree is brokenRepository with a script in every place the script
// rule reads and in every place it does not, and files that are not scripts
// beside them.
func scriptTree(t *testing.T) string {
	t.Helper()

	root := brokenRepository(t)
	for relative, body := range map[string]string{
		"scripts/scratch.sh":                   "echo scratch\n",
		".github/scripts/verdict.sh":           "echo verdict\n",
		".github/workflows/notes.md":           "not a script\n",
		"tools/stamp":                          "#!/usr/bin/env python3\nprint('stamp')\n",
		"tools/README":                         "#not a shebang\n",
		"tools/x":                              "#",
		"tools/empty":                          "",
		"LICENSE":                              "MIT\n",
		"docs/notes.txt":                       "#!/not/a/script/by/its/extension\n",
		"site/scripts/build.mjs":               "console.log('site')\n",
		"site/node_modules/a/i.js":             "module.exports = 1\n",
		"npm/launcher/node_modules/x/index.js": "module.exports = 1\n",
		"npm/packages/linux/bin.js":            "module.exports = 1\n",
		"plan/draft.py":                        "print('draft')\n",
		"dist/stage/run.sh":                    "echo dist\n",
		"cmd/tool/testdata/fixture.sh":         "echo fixture\n",
		".claude/hooks/hook.sh":                "echo hook\n",
		".claude/worktrees/other/run.sh":       "echo worktree\n",
		".cache/tool/run.sh":                   "echo cache\n",
		".plugin/hook.sh":                      "echo plugin\n",
		".vscode/task.sh":                      "echo task\n",
		"scratch/.git":                         "gitdir: /elsewhere\n",
		"scratch/copy.sh":                      "echo copy\n",
	} {
		writeFile(t, root, relative, body)
	}
	// A symbolic link is not followed, whatever its name says. A platform
	// that refuses to make one leaves nothing there to report either way.
	_ = os.Symlink("stamp", filepath.Join(root, "tools", "link.sh"))
	return root
}

// TestScriptProblems_Declarations_AreHeldToTheTree verifies the table's own
// rules: a script is declared by its path or by the nearest directory that
// holds it, and a declaration is reported when it is not a repository-relative
// path, gives a category this command does not define, gives no reason where
// one is asked for, names an item of issue 1114 a pending port does not or a
// port without one, or names no script; an item of the ports table no pending
// script names any longer has landed and is reported too.
func TestScriptProblems_Declarations_AreHeldToTheTree(t *testing.T) {
	t.Parallel()

	found := []string{"scripts/kept.sh", "scripts/pending.py", "test/fixtures/a/b.py", "test/fixtures/a/c/d.py", "test/fixtures/z.py", "tools/x.sh"}
	declared := tables{
		scripts: map[string]scriptDeclaration{
			"scripts/kept.sh":        {category: scriptJobGlue, reason: "glue"},
			"scripts/pending.py":     {category: scriptPortPending, item: 4},
			"test/fixtures/":         {category: scriptFixtureContent, reason: "fixtures"},
			"test/fixtures/a/":       {category: scriptFixtureContent, reason: "nearer fixtures"},
			"test/fixtures/a/c/d.py": {category: scriptFixtureContent, reason: "its own"},
			"tools/":                 {category: scriptFixtureContent, reason: "covers tools/x.sh"},
			"tools/x.sh":             {category: scriptJobGlue, reason: "its own wins"},
			"scripts/gone.sh":        {category: scriptJobGlue, reason: "names nothing"},
			"scripts/blank.sh":       {category: scriptJobGlue, reason: "  "},
			"scripts/itemized.sh":    {category: scriptJobGlue, reason: "an item it may not name", item: 4},
			"scripts/unported.sh":    {category: scriptPortPending, item: 9},
			"scripts/trust.sh":       {category: "trust-me", reason: "fine"},
			"../outside.sh":          {category: scriptJobGlue, reason: "outside"},
			"/absolute.sh":           {category: scriptJobGlue, reason: "absolute"},
			"scripts//double.sh":     {category: scriptJobGlue, reason: "unclean"},
			"/":                      {category: scriptFixtureContent, reason: "the root"},
			"./":                     {category: scriptFixtureContent, reason: "the whole tree"},
			".":                      {category: scriptFixtureContent, reason: "the whole tree"},
		},
		ports: map[int]string{4: "the coverage command", 12: "the link check"},
	}

	got := scriptProblems(found, declared)
	const label = "cmd/audit_supply_chain/declarations.go: "
	want := []string{
		label + `"." is not a repository-relative path, or a directory of one ending in /`,
		label + `"../outside.sh" is not a repository-relative path, or a directory of one ending in /`,
		label + `"./" is not a repository-relative path, or a directory of one ending in /`,
		label + `"/" is not a repository-relative path, or a directory of one ending in /`,
		label + `"/absolute.sh" is not a repository-relative path, or a directory of one ending in /`,
		label + `"scripts//double.sh" is not a repository-relative path, or a directory of one ending in /`,
		label + `"scripts/blank.sh" gives no reason`,
		label + `"scripts/blank.sh" names no script in the tree`,
		label + `"scripts/gone.sh" names no script in the tree`,
		label + `"scripts/itemized.sh" names item 4 of issue 1114, which only a script pending its port does`,
		label + `"scripts/itemized.sh" names no script in the tree`,
		label + `"scripts/trust.sh": the category "trust-me" is not one this command defines for a script`,
		label + `"scripts/trust.sh" names no script in the tree`,
		label + `"scripts/unported.sh" is pending a port and names no item of issue 1114 that ports it`,
		label + `"scripts/unported.sh" names no script in the tree`,
		label + `"tools/" names no script in the tree`,
		label + "item 12 of issue 1114 is named by no script pending its port, so it has landed and its line goes",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("scriptProblems() =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestIsScript_UnopenableFile_IsNotAScript verifies that a file with no
// extension this audit cannot open is passed over rather than stopping the
// audit: the extension rule has already judged every file that names its
// language, and a file the walk cannot read is one no job can run either.
// It swaps the package's open function, so it does not run in parallel.
func TestIsScript_UnopenableFile_IsNotAScript(t *testing.T) {
	previous := openFile
	t.Cleanup(func() { openFile = previous })
	openFile = func(string) (*os.File, error) { return nil, errors.New("refused") }

	root := t.TempDir()
	writeFile(t, root, "tools/stamp", "#!/bin/sh\n")
	if isScript(filepath.Join(root, "tools", "stamp"), "stamp") {
		t.Error("isScript() = true for a file that could not be opened, want false")
	}
}
