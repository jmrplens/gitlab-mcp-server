// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_pr_text refuses an em dash (U+2014) in the text a pull
// request lands in main, and refuses a review bot's generated block or a skip
// command in its description. It is item 11 of issue 1114,
// scripts/check-em-dash.sh moved into Go, and its tests hold it to the cases
// the script's suite held the description half to.
//
//	audit_pr_text diff [BASE_REF]
//	audit_pr_text description
//
// Two surfaces, two subcommands, because each becomes permanent in a
// different way and only one of them can still be cleaned by a later commit.
//
// # diff
//
// The lines this branch adds, judged net over the branch rather than per
// commit, so a branch that introduces an em dash and removes it again at its
// tip passes. A whole-tree check is not an option: the tree already carries
// some five thousand of them in prose written before the rule, so it would be
// red on its first run and switched off on its second. BASE_REF defaults to
// $EM_DASH_BASE, then to origin/main, and the comparison is the three-dot
// form, so only what this branch added on top of their merge base is judged.
// make check-em-dash runs it, and so does the Em dash job of ci.yml.
//
// git is asked for the diff with explicit prefixes, no external diff driver,
// no textconv filter and rename detection stated, so a developer's diff
// configuration cannot hand the parser a shape it cannot read: a gate that
// reads nothing passes everything. A few paths are not judged, because they
// are not this branch's prose ([excludedPaths] says which and why), and an
// entry there that names no file is a finding of its own.
//
// # description
//
// The pull request title and body. This repository squash merges, so the
// description is the commit message that lands on main, and no later commit
// can edit it out of the history. The description is read from the API at
// the moment this runs, through gh, rather than from the workflow event
// payload: the payload is a snapshot taken when the run was queued, and a
// review bot that reinjects its generated block while the run is in flight
// does not appear in it. The pull request is $PR_NUMBER, or the one open for
// the current branch, in $GITHUB_REPOSITORY or the repository gh is in.
// PR_TITLE_FILE and PR_BODY_FILE judge text from disk instead, which needs no
// gh and is how the gate is rehearsed. make check-pr-description runs it, and
// so does the Checks job of ci.yml, last before the verdict.
//
// Besides an em dash anywhere, it refuses the markers a review bot writes
// around the block it injects, and the ways GitHub reads a commit message as
// a request to skip every workflow a push triggers: the five bracketed
// commands anywhere, and a skip-checks trailer at the start of a line, whose
// value is the whole word true. Those two are defined once, in
// cmd/internal/committext, for this command and for the release notes rule
// of cmd/audit_supply_chain alike.
//
// It also refuses a reference GitLab resolves to an item of another project
// or group (issue 1477): a project's path, nested groups included, followed
// by !N; a path of one segment or more followed by #N, since GitLab reads a
// top-level group's name alone before #N as that group's work item; a
// group's path followed by &N for an epic; any of them opened by the / of an
// absolute path; the bracketed [issue:], [work_item:] and [epic:] forms with
// a path; and a full gitlab.com URL to a merge request, an issue, an
// incident, a work item, an epic or a commit, with or without the /-/ before
// the route and at a group's address too. The GitLab mirror of this
// repository reads the squash commit like any commit pushed to it, and GitLab
// adds a "mentioned in commit" note to every item a message names that way,
// on somebody else's project; main cannot be rewritten to take it back. A
// description names the row of docs/development/upstream-bugs.md that records
// the item instead. An item of another forge written the same way, such as a
// GitHub issue as owner/repo#N or repo#N, is refused too, and its github.com
// URL passes. A bare #N names this repository's own items and passes, and so
// does a project of the mirror's own namespace named alone before !N. A
// commit named as group/project@sha passes on purpose, although GitLab reads
// it too, because that is how a workflow pins a GitHub action. The rule reads
// the whole title and body, code spans included, since GitLab reads a commit
// message as plain text when it extracts mentions. File content is out of its
// scope: pages cite upstream items however they need to.
//
// A pull request Dependabot opens usually fails the rule, since its
// description copies each dependency's release notes, changelog and commit
// list, which name that dependency's own items. Dependabot is not exempt,
// because those are exactly the references a squash merge would land in
// main; the failure says how the description is cut before the merge, and
// that the failed jobs are re-run afterwards, since editing a description
// starts no run.
//
// A branch with no pull request open passes, having no description to land.
// The pull request open for it is looked up with gh pr list, which answers
// none with an empty list, so only that answer reads as none: a lookup that
// fails, an answer that is not a list, null included, one with an entry that
// names no pull request or does not say whether it is from a fork, and a
// HEAD on no branch fail the run like a gh that is missing and a
// description that cannot be read, and say so, because a check that cannot
// read what it judges must not report that it judged it. Only a pull
// request from a branch of the repository itself is taken, since the
// listing also holds a fork's pull request from a branch of the same name,
// which is somebody else's description: a branch whose only open pull
// requests are forks' passes as one with none, and a pull request opened
// from a fork, like a branch pushed under another name, is named with
// PR_NUMBER, which CI always sets. The listing asks for a hundred, where gh
// lists 30 unless told, and one that holds a hundred with none from a
// branch of the repository fails as well, since the one to judge may be
// past them.
//
// # What changed in the port
//
// The script's description half passed a pull request it could not read: a
// gh call that failed inside its reader returned an empty title and body, and
// the empty text carried nothing to refuse. Here every such failure stops the
// run. A title or body file that cannot be read does too, where the script
// judged it as empty, and so does a failed lookup of the branch's pull
// request, which the script read as there being none. The skip-checks value
// is the whole word true, where the script matched it as a prefix and
// refused "skip-checks: trueish".
package main
