// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_doc_links holds every link of this repository's
// documentation to what it names: a relative link to a file that exists and
// to the heading its anchor names there, and a link to the documentation
// site to the page under site/src/content/docs that serves it, its heading,
// or the file the site serves. It is item 12 of issue 1114,
// scripts/check-doc-links.mjs moved into Go, and its tests hold it to what
// that script answered (testdata/corpus.txtar and corpus.golden) and to the
// cases its suite held it to. make check-doc-links runs it.
//
// # What it reads
//
// Every Markdown and MDX file of the repository that cmd/internal/docwalk
// finds below the root and below .github and .claude, the two dot-directories
// that hold tracked documentation: the walk leaves out dependencies, build
// output, the untracked plan area, every other dot-directory and every other
// checkout, worktrees included. The script asked git for the tracked files;
// a walk on disk reads the same set in a checkout, which a test holds against
// git ls-files, and also runs where the tree is a copy with no .git.
//
// Only prose is read: a line inside a fenced block is code, and so is the
// text of a code span, where a link is quoted rather than linked.
//
// # The two dialects
//
// A file under site/src/content is a page of the site, rendered by
// Starlight, and its headings are named the way github-slugger names them; a
// file anywhere else is read in the repository, rendered by GitHub, whose
// TableOfContentsFilter names them another way. The two agree on almost every
// heading and part company on a repeated heading, a join control and the
// _top every site page has (anchors.go says how), so a link is judged in the
// dialect of the page it points into.
//
// # What it skips
//
// A link with a scheme is somebody else's server, the site's own address
// aside; a root-relative link is a route the site validates at build time;
// a placeholder word or a template names nothing; and a fragment on a file
// that is not Markdown is a line reference nothing here can read.
package main
