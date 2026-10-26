// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/fileheader"
)

// kind is how one sort of source file carries the header: the text each of
// its two comment lines opens and closes with, how many of the file's first
// lines have to stay above it, and whether it lives inside a frontmatter
// fence, which a file without one is given.
type kind struct {
	open, close string
	preamble    func(lines []string) int
	fenced      bool
}

// fence opens and closes an Astro component's frontmatter, the one part of
// the file whose comments are not rendered into the page.
const fence = "---"

// The kinds, by the comment syntax and the preamble each one keeps above the
// header.
var (
	goKind         = kind{open: "// ", preamble: noPreamble}
	scriptKind     = kind{open: "# ", preamble: shebang}
	encodedKind    = kind{open: "# ", preamble: shebangAndEncoding}
	dockerfileKind = kind{open: "# ", preamble: parserDirectives}
	javascriptKind = kind{open: "// ", preamble: shebang}
	cssKind        = kind{open: "/* ", close: " */", preamble: charset}
	astroKind      = kind{open: "// ", preamble: frontmatter, fenced: true}
)

// kindsByName are the source files a base name alone identifies.
var kindsByName = map[string]kind{
	"Makefile":   scriptKind,
	"Dockerfile": dockerfileKind,
}

// kindsByExtension are the source files their extension identifies.
var kindsByExtension = map[string]kind{
	".go":    goKind,
	".sh":    scriptKind,
	".bash":  scriptKind,
	".ps1":   scriptKind,
	".psm1":  scriptKind,
	".mk":    scriptKind,
	".jq":    scriptKind,
	".py":    encodedKind,
	".rb":    encodedKind,
	".js":    javascriptKind,
	".mjs":   javascriptKind,
	".cjs":   javascriptKind,
	".jsx":   javascriptKind,
	".ts":    javascriptKind,
	".mts":   javascriptKind,
	".cts":   javascriptKind,
	".tsx":   javascriptKind,
	".css":   cssKind,
	".astro": astroKind,
}

// kindOf returns the kind of the file at the slash-separated path name, and
// false for a file the rule does not judge: documentation, data,
// configuration and anything else that is not source.
func kindOf(name string) (kind, bool) {
	base := path.Base(name)
	if k, ok := kindsByName[base]; ok {
		return k, true
	}
	k, ok := kindsByExtension[path.Ext(base)]
	return k, ok
}

// header returns the two lines of the header in the kind's comment syntax.
func (k kind) header() []string {
	return []string{k.open + fileheader.CopyrightStatement + k.close, k.open + fileheader.LicenseStatement + k.close}
}

// noPreamble keeps nothing above the header: a Go file opens with it, above
// any build constraint, as the Go project's own files do.
func noPreamble([]string) int {
	return 0
}

// shebang keeps an interpreter line above the header, since the kernel reads
// it only as a file's first line.
func shebang(lines []string) int {
	if strings.HasPrefix(lines[0], "#!") {
		return 1
	}
	return 0
}

// encoding is the declaration of a source encoding that Python (PEP 263) and
// Ruby read only on a file's first or second line.
var encoding = regexp.MustCompile(`^[ \t\f]*#.*?coding[:=]`)

// shebangAndEncoding keeps an interpreter line and an encoding declaration
// above the header, the declaration only where it is still read.
func shebangAndEncoding(lines []string) int {
	kept := shebang(lines)
	for i := kept; i < 2 && i < len(lines); i++ {
		if encoding.MatchString(lines[i]) {
			return i + 1
		}
	}
	return kept
}

// directive is a Dockerfile parser directive, which BuildKit reads only above
// the file's first comment, blank line or instruction.
var directive = regexp.MustCompile(`(?i)^#\s*(syntax|escape|check)\s*=`)

// parserDirectives keeps a Dockerfile's parser directives above the header.
func parserDirectives(lines []string) int {
	kept := 0
	for kept < len(lines) && directive.MatchString(lines[kept]) {
		kept++
	}
	return kept
}

// charset keeps a stylesheet's @charset rule above the header, since a
// browser honors it only as the file's first bytes.
func charset(lines []string) int {
	if strings.HasPrefix(lines[0], "@charset ") {
		return 1
	}
	return 0
}

// frontmatter keeps the fence that opens an Astro component's frontmatter
// above the header, so the header is a comment of the frontmatter's script
// and not text of the page.
func frontmatter(lines []string) int {
	if lines[0] == fence {
		return 1
	}
	return 0
}

// verdict is what the header rule finds in one file: whether the header is
// missing from where its kind puts it, and the lines, counted from one, that
// carry a copyright or license notice other than that header.
type verdict struct {
	missing bool
	notices []int
}

// judge holds the content of a file of kind k to the header rule. The header
// counts only as its two lines, in order, exactly where k puts them, and for
// an Astro component only inside a frontmatter. Every other comment line
// opening with a copyright or license statement is a notice, a copy of the
// header out of place included.
func judge(k kind, content string) verdict {
	lines := strings.Split(content, "\n")
	at := k.preamble(lines)
	header := k.header()
	present := (!k.fenced || at != 0) && at+1 < len(lines) && lines[at] == header[0] && lines[at+1] == header[1]
	v := verdict{missing: !present}
	for i, line := range lines {
		if present && (i == at || i == at+1) {
			continue
		}
		if isNotice(line) {
			v.notices = append(v.notices, i+1)
		}
	}
	return v
}

// fix returns content with the header written where k puts it: after the
// preamble, followed by a blank line unless the file already has one there,
// and for an Astro component without a frontmatter inside a new one. Every
// byte of content is kept, a missing final newline included. It is called
// for a file judge found missing the header and carrying no other notice.
func fix(k kind, content string) string {
	lines := strings.Split(content, "\n")
	at := k.preamble(lines)
	block := k.header()
	if k.fenced && at == 0 {
		block = slices.Concat([]string{fence}, block, []string{fence})
	} else if at < len(lines) && lines[at] != "" {
		block = append(block, "")
	}
	return strings.Join(slices.Concat(lines[:at], block, lines[at:]), "\n")
}

// commentOpeners are what a comment line opens with in the kinds above: a
// line comment, a hash comment, a block comment and the asterisk a block
// comment's continuation lines carry.
var commentOpeners = []string{"//", "#", "/*", "*"}

// isNotice reports whether line is a comment whose text opens with the word
// "Copyright" or with an SPDX tag, which is what a notice looks like in every
// comment syntax the rule reads. The word has to end there, so a doc comment
// opening with an identifier that merely starts with it is not a notice; nor
// is a comment quoting one inside its text, or a line of code, a string on it
// included. The line is read alone, without the file's syntax around it, so a
// line of a multi-line string or a heredoc that opens with a comment marker is
// read as a comment.
func isNotice(line string) bool {
	text := strings.TrimSpace(line)
	for _, opener := range commentOpeners {
		if rest, ok := strings.CutPrefix(text, opener); ok {
			rest = strings.TrimSpace(rest)
			if after, found := strings.CutPrefix(rest, "Copyright"); found {
				next, _ := utf8.DecodeRuneInString(after)
				return !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_'
			}
			return strings.HasPrefix(rest, "SPDX-")
		}
	}
	return false
}
