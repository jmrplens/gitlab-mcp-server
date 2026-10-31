// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_dco holds the commits of a branch to the Developer Certificate
// of Origin (https://developercertificate.org/): every commit it judges has to
// carry a Signed-off-by trailer naming its author, which is how the author
// certifies the certificate for that commit.
//
//	audit_dco [-base <rev>] [-head <rev>] [-opened-by <login>] [-dir <repository>] [-annotate]
//
// # What it judges
//
// The commits reachable from -head and not from -base, which are the ones a
// pull request brings: origin/main..HEAD by default, and in CI the pull
// request's base and head commits as the event names them. The event of a
// stacked pull request names the head of the layer below as its base, so each
// layer's pull request judges that layer's commits and no others, and a layer
// below is judged only by its own pull request's run of ci.yml.
//
// Each commit is read from git with the trailers git finds in its message
// (git log's %(trailers) placeholder, which applies the rules of git
// interpret-trailers), so a sign-off counts exactly when git, and every tool
// that reads trailers through git, finds one. A commit passes when one of its
// Signed-off-by trailers names its author, the name and the email each
// compared without regard to case. git commit --signoff writes that trailer
// with the committer's identity, which is the author's for a commit one makes
// oneself.
//
// A commit with no Signed-off-by trailer fails, and so does one whose trailers
// name only other people: a sign-off certifies the commit for the person it
// names, and the certificate is the author's to give. A trailer that is not a
// name followed by an email in angle brackets names nobody.
//
// git log prints the fields of each commit between separator bytes, and a
// message can carry them. A record ends only where its separator is followed
// by a newline, which no field git prints holds, so such a message is read as
// git reads it: the byte stays in its field, and a trailer it ends names
// nobody. The commits are also listed by git rev-list, whose hashes carry no
// text a commit's author chose, and a run in which the two lists differ stops
// rather than judging either, so a sign-off can never move onto another
// commit.
//
// # What it does not judge
//
// A merge of two parents that brings no change of its own is reported and not
// judged: its tree is the one git's own merge of its parents makes, which git
// show --remerge-diff prints no difference for, and the commits it brings in
// that the base does not hold are judged in their own right. GitHub's Update
// branch button writes one. Every other merge is judged like any other
// commit: one that resolves a conflict, adds or drops a change while merging,
// or joins more than two parents, which git does not remerge.
//
// The commits of a declared bot are exempt in a pull request that bot opened.
// A bot is an app, not a person, so it can certify nothing, and what it
// commits is generated. Each bot is declared in bots.go with the author
// identity its commits carry, the GitHub login that opens its pull requests
// and the reason it is exempt, and the exemption needs both to agree: a
// commit carrying a bot's identity in a pull request somebody else opened is
// judged like any other, so writing a bot's name into a commit exempts
// nothing. Today the one declared bot is Dependabot. A workflow that starts
// opening pull requests as another app fails here until that app is declared.
//
// The release workflow's version stamp is not declared, because it never sits
// in a pull request: the release job pushes it to main with its deploy key
// after a release, as GOVERNANCE.md describes, and the commits a pull request
// brings are the only ones this command reads. A fix developed in a security
// advisory's temporary private fork reaches main from a pull request of that
// fork, merged from the advisory, and GitHub runs no check on it there, so CI
// never runs this command on its commits: make check-dco on the fork's
// branch, before the advisory is merged, is what can judge them.
//
// # The squash commit
//
// main gains one squash commit per pull request, whose message is, by
// default, the pull request description, and the description is not judged.
// With GitHub's default squash message, GitHub copies the Signed-off-by
// trailers of the pull request's commits into the squash commit itself, once
// each, after the description: the squash commits of pull requests 1512 and
// 1529 carry the trailer their descriptions did not, and so do the squash
// commits of Dependabot's pull requests. A description that carried its own
// would repeat it. A message written by hand at the merge carries only the
// trailers it holds, so it should end with the pull request's Signed-off-by
// lines.
//
// # Exit codes
//
// 0 when every judged commit is signed off by its author, or there is none to
// judge; 1 when a commit is not (said on stdout, with how to sign it off) or
// git could not answer (said on stderr); 2 for arguments it cannot use.
package main
