// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package docwalk answers which Markdown and MDX files a documentation root
// holds, for the two commands that read this repository's documentation:
// cmd/audit_doc_tool_names, which holds the tool names and action IDs the
// pages teach to the server, and cmd/audit_doc_links, which holds every local
// link and every link to the documentation site to the file and the heading
// it names (issue 1114, item 12).
//
// The two used to find their files in two ways: the audit walked a list of
// roots on disk, and the link check asked git for every tracked file. One
// walk is one answer to what the documentation is, and it is a walk on disk
// rather than git's index because the checks run where the tree is a copy
// with no .git as well as in a checkout.
//
// # What a walk leaves out
//
// A root is always entered, whatever it is called, so a dot-directory such
// as .github is read when it is named as a root. Below a root, a directory is
// left out when it is not this repository's documentation: everything
// [sourcewalk.SkipDirBelowRoot] prunes (a dot-directory, and another checkout
// such as a worktree the parallel-agent tooling puts under .claude), and the
// directories that hold what the repository ignores rather than what it
// writes: node_modules, the dependencies; dist, every build's output; and
// plan, the untracked working area make check-plan-untracked keeps out of
// git. A file root is itself, whatever its extension, and a root that does
// not exist holds nothing.
package docwalk
