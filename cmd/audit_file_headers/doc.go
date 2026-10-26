// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_file_headers holds every source file of this repository to
// the two lines it opens with: the copyright statement and the SPDX
// identifier of the license, the ones cmd/internal/fileheader spells.
//
//	audit_file_headers [-dir <repository>] [-fix]
//
// It is the gate behind the OpenSSF Best Practices criteria copyright_per_file
// and license_per_file, which ask every source file to name its copyright
// holder and its license, the second in the SPDX form near the file's top.
//
// # Which files
//
// The files git holds in its index and the untracked ones it would add, read
// with git ls-files from -dir. The list is git's and not a walk of the tree,
// because the question is which files the repository publishes, and git
// answers it exactly: a walk would read what .gitignore keeps out of the
// repository (plan/, dist/, site/node_modules and its thousands of files
// somebody else wrote) unless it named each of them again, and the name rule
// of internal/sourcewalk, which leaves every dot-directory out, would leave
// out the scripts under .github/scripts that are this repository's own. Files
// of a checkout nested in the tree are not listed, which is what
// internal/sourcewalk's marker rule answers for a walk. The command therefore
// runs in a checkout, not in a copy of the tree without its .git.
//
// Of those, it judges the source files: Go (testdata included), shell,
// PowerShell, Python, Ruby, jq, JavaScript and TypeScript in each of their
// extensions, CSS, Astro components, Makefiles and the Dockerfile.
// Documentation, data and configuration (Markdown, MDX, JSON, YAML and the
// workflows written in it, TOML, SVG and images) are not source and are not
// judged. A path whose file the working tree no longer has, a symbolic link
// and a nested checkout's directory are skipped.
//
// # Where the header goes
//
// The two lines are written as comments of the file's language: line
// comments in Go, JavaScript, TypeScript and the frontmatter of an Astro
// component, hash comments in the scripts, the jq programs, the Makefile and
// the Dockerfile, and a block comment per line in CSS. They are the file's
// first lines, after what has to stay above them to keep working:
//
//   - a shebang, which the kernel reads only as the first line;
//   - in Python and Ruby, an encoding declaration on the first or second
//     line, the only lines either reads one from;
//   - a Dockerfile's parser directives, which BuildKit stops reading at the
//     first comment;
//   - a stylesheet's @charset rule, honored only as the file's first bytes;
//   - the fence that opens an Astro component's frontmatter, so the header is
//     a comment of the component's script and not text of the page.
//
// In a Go file the header comes before any build constraint, as in the Go
// project's own files, and a blank line keeps it apart from what follows:
// without it the header would be read as the package's doc comment, which
// cmd/godoc_tool refuses, or would join the "Code generated" comment of a
// generated file.
//
// # What fails
//
// A file fails when its header is not those two lines, in that order, exactly
// there. It also fails at every line that carries a copyright or license
// notice other than that header, which is any comment line whose text opens
// with the word "Copyright" or with an SPDX tag ("SPDX-License-Identifier:",
// "SPDX-FileCopyrightText:"): a second copy of the header, a header written
// below the package doc comment, and the notice of a file somebody else
// wrote. The word has to end there, so the doc comment of an identifier that
// starts with it, such as fileheader.CopyrightStatement, is not a notice. A
// comment that mentions a copyright inside its text is not a notice either,
// nor is a line of code, a string on it included. Each line is read alone,
// without the syntax of the file around it, so a line of a multi-line string
// or a heredoc that opens with a comment marker is read as a comment. The
// rule errs towards reporting: a line of this repository's own that it reads
// as a notice is reworded, since a notice the rule missed would be a file
// somebody else wrote stamped with this one.
//
// # The fix mode
//
// -fix writes the header into every file that does not open with it and
// carries no other notice, followed by a blank line unless the file has one
// there already, keeping every other byte and the file's mode. An Astro
// component without a frontmatter is given one around the header. A file with
// another notice is never rewritten: one this repository did not write keeps
// its own notice and is not stamped with this one, and one it did is moved to
// a single header at its top by hand.
//
// The fix mode is how the header reached every file the first time, as a
// mechanical rewrite verified by the build, the tests and the linters
// afterwards. A generated file carries the header because its generator
// writes it, and the gate then holds the generator to that as it holds any
// file.
//
// # Exit codes
//
// 0 when every source file opens with the header, or -fix wrote it into every
// one that did not; 1 when a file still fails or could not be listed, stated,
// read or written (the finding on stdout with how to fix it, the error on
// stderr); 2 for arguments it cannot use.
package main
