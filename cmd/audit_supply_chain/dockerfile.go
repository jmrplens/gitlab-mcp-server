// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// dockerfileDirective is a parser directive once the comment marker in front
// of it, and the white space after that, are set aside, as BuildKit reads one
// (moby/buildkit, frontend/dockerfile/parser/directives.go): a name, read in
// any case, an = and a value, white space allowed around each.
var dockerfileDirective = regexp.MustCompile(`^([a-zA-Z][a-zA-Z0-9]*)\s*=\s*(.+?)\s*$`)

// dockerfileDirectiveNames are the parser directives BuildKit knows. A line
// naming any other is a comment, which ends the directives.
var dockerfileDirectiveNames = map[string]bool{"syntax": true, "escape": true, "check": true}

// dockerfileHeredoc is a word of a RUN, COPY or ADD instruction that opens a
// heredoc, as BuildKit's ParseHeredoc reads one: an optional file descriptor,
// <<, an optional - that strips the leading tabs of the body's lines, and the
// word that ends the body on a line of its own. A here-string, <<<, is not
// one, and neither is a << that no word follows in the same word.
var dockerfileHeredoc = regexp.MustCompile(`^\d*<<(-?)([^<]*)$`)

// dockerfileHeredocName is the word of a heredoc this audit can name the end
// of: one with no quote, escape, variable or white space in it, which
// BuildKit's shell lexer reads apart from the line that ends the body, alone
// or inside one pair of quotes around all of it.
var dockerfileHeredocName = regexp.MustCompile(`^(?:'([^'"\\$\s]+)'|"([^'"\\$\s]+)"|([^'"\\$\s]+))$`)

// dockerfileArgName is the name a $NAME reference reads.
var dockerfileArgName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*`)

// dockerfileHeredocWords are the instructions BuildKit reads a heredoc on.
var dockerfileHeredocWords = map[string]bool{"ADD": true, "COPY": true, "RUN": true}

// The reasons a Dockerfile stops being one this audit can read as BuildKit
// does, after the line of the instruction they are about.
const (
	heredocUnterminated = "opens a heredoc no line ends, which BuildKit refuses"
	heredocUnread       = "opens a heredoc in a form this audit does not read (a quote left open, a quote, an escape, a variable or a space in its word, a variable beside <<, a << another word follows, or one on ONBUILD), so which lines are its body is not known"
)

// dockerfileInstruction is one instruction of a Dockerfile: the line it starts
// on, its keyword in upper case, the words after it, and its text, its
// continuation lines joined.
type dockerfileInstruction struct {
	keyword string
	text    string
	words   []string
	line    int
}

// dockerfileImage is an image a Dockerfile has the build pull: the line that
// names it, how it is named there (written to sit right before the
// reference), and the reference with the defaults of the
// global ARGs substituted. resolved is false when a variable in it is one no
// default sets, or is written in a form this audit does not substitute, and
// the reference is then the one written.
type dockerfileImage struct {
	instruction string
	reference   string
	line        int
	resolved    bool
}

// dockerfileUnreadable is where a Dockerfile stops being one this audit can
// read as BuildKit does: the line of the instruction it stops at and why. Its
// line is 0 when the file can be read to its end.
type dockerfileUnreadable struct {
	reason string
	line   int
}

// dockerfileArg is an ARG declared before the first FROM: its default, and
// whether it has one this audit could resolve.
type dockerfileArg struct {
	value string
	set   bool
}

// dockerfileDirectiveLine reads one line as a parser directive written behind
// marker: its name in lower case, its value, and whether the line is one.
func dockerfileDirectiveLine(line, marker string) (name, value string, ok bool) {
	rest, marked := strings.CutPrefix(line, marker)
	if !marked {
		return "", "", false
	}
	match := dockerfileDirective.FindStringSubmatch(strings.TrimLeftFunc(rest, unicode.IsSpace))
	if match == nil {
		return "", "", false
	}
	return strings.ToLower(match[1]), match[2], true
}

// dockerfileEscape is the character that continues a line of a Dockerfile, as
// BuildKit's parser reads it from the escape directive (parser.go): only among
// the directives at the top, each a line that opens with # once its leading
// white space is set aside and names a directive BuildKit knows. The first
// line that is not one ends them, so an escape directive after an empty line,
// a #! line, a comment, a directive BuildKit does not know or an instruction
// is a comment. A value other than ` or \ is one BuildKit refuses.
func dockerfileEscape(lines []string) string {
	escape := `\`
	for _, line := range lines {
		name, value, ok := dockerfileDirectiveLine(strings.TrimLeftFunc(line, unicode.IsSpace), "#")
		if !ok || !dockerfileDirectiveNames[name] {
			return escape
		}
		if name == "escape" && value == "`" {
			escape = "`"
		}
	}
	return escape
}

// dockerfileSyntax returns the frontend images a Dockerfile's syntax
// directive names, which BuildKit pulls and builds with in place of its own
// parser. BuildKit looks for the directive (DetectSyntax) past a first line
// that opens with #!: among the directives behind # at the top of the file,
// then among those behind //, taking the directive's first word, then in a
// file that is one JSON object, as its syntax member. The directives are read
// here behind either marker, past white space in front of one and past one
// BuildKit does not know, which finds every directive BuildKit takes and
// perhaps more: one the build ignores is a finding to set aside, while one
// this audit missed would be an image that passes unread.
func dockerfileSyntax(lines []string) []dockerfileImage {
	start := 0
	if strings.HasPrefix(lines[0], "#!") {
		start = 1
	}
	var images []dockerfileImage
	for index := start; index < len(lines); index++ {
		trimmed := strings.TrimLeftFunc(lines[index], unicode.IsSpace)
		marker := "#"
		if strings.HasPrefix(trimmed, "//") {
			marker = "//"
		}
		name, value, ok := dockerfileDirectiveLine(trimmed, marker)
		if !ok {
			break
		}
		if name == "syntax" {
			reference, _, _ := strings.Cut(value, " ")
			images = append(images, dockerfileImage{instruction: marker + " syntax=", reference: reference, line: index + 1, resolved: true})
		}
	}
	var document map[string]any
	if json.Unmarshal([]byte(strings.Join(lines[start:], "\n")), &document) == nil {
		if reference, named := document["syntax"].(string); named {
			images = append(images, dockerfileImage{instruction: `"syntax": `, reference: reference, line: start + 1, resolved: true})
		}
	}
	return images
}

// readDockerfile splits a Dockerfile's lines into its instructions as
// BuildKit's parser does (parser.go). Each instruction's keyword is read in
// any case; a line that ends in the escape character, white space after it
// included, goes on on the next, unless the character before it is the
// escape character too; a comment line or an empty one is dropped wherever it
// is; and the body of every heredoc an instruction opens is skipped, since
// none of it is an instruction. unreadable is where the file stops being one
// this audit can read, a heredoc no line ends or one in a form it does not
// read, and has the line 0 when there is none.
func readDockerfile(lines []string) (instructions []dockerfileInstruction, unreadable dockerfileUnreadable) {
	escape := dockerfileEscape(lines)
	var pending []string
	first := 0
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if pending == nil {
			first = index + 1
		}
		if body, continued := dockerfileContinued(line, escape); continued {
			pending = append(pending, body)
			continue
		}
		instruction := newDockerfileInstruction(first, append(pending, line))
		instructions = append(instructions, instruction)
		pending = nil
		heredocs, readable := instruction.heredocs()
		if !readable {
			return instructions, dockerfileUnreadable{reason: heredocUnread, line: first}
		}
		for _, heredoc := range heredocs {
			if index = skipHeredoc(lines, index, heredoc); index == len(lines) {
				return instructions, dockerfileUnreadable{reason: heredocUnterminated, line: first}
			}
		}
	}
	if pending != nil {
		instructions = append(instructions, newDockerfileInstruction(first, pending))
	}
	return instructions, dockerfileUnreadable{}
}

// dockerfileContinued reports whether a line goes on on the next, as the
// lineContinuationRegex of BuildKit's parser reads it, and returns the line
// without the escape character then: it ends in the escape character, white
// space after it aside, and the character before that is not the escape
// character as well, which makes the pair an escaped one that ends the line.
func dockerfileContinued(line, escape string) (string, bool) {
	body, ends := strings.CutSuffix(strings.TrimRight(line, " \t"), escape)
	if !ends || strings.HasSuffix(body, escape) {
		return line, false
	}
	return body, true
}

// newDockerfileInstruction joins the lines of one instruction, as BuildKit
// joins a continued line, with nothing between them, and splits it into its
// keyword and its words. Lines that hold nothing but the escape character
// leave an instruction with no keyword, which names nothing and opens no
// heredoc.
func newDockerfileInstruction(line int, parts []string) dockerfileInstruction {
	text := strings.Join(parts, "")
	words := strings.Fields(text)
	if len(words) == 0 {
		return dockerfileInstruction{line: line}
	}
	return dockerfileInstruction{keyword: strings.ToUpper(words[0]), text: text, words: words[1:], line: line}
}

// dockerfileHeredocEnd is the line that ends one heredoc's body, and whether
// the body's lines are read without their leading tabs.
type dockerfileHeredocEnd struct {
	word  string
	chomp bool
}

// heredocs are the heredocs an instruction opens, in the order its words name
// them, which is the order their bodies follow it in, and false when it opens
// one this audit cannot find the end of.
//
// BuildKit reads a heredoc on a RUN, a COPY or an ADD, or on one an ONBUILD
// names, whose text holds << (heredocsFromLine): the text is split into words
// as its shell lexer splits it with the quotes and escapes kept, and a word
// that is a heredoc names the line that ends the body. A heredoc this audit
// cannot name the end of is unreadable rather than guessed at, since a guess
// that ends the body too early or too late reads lines the build does not, or
// skips lines it does. That is a quote the text leaves open, a word with a
// quote, an escape, a variable or a space in it, and the forms the release of
// BuildKit decides: a variable beside << (a release that expands an unset
// variable to nothing reads $X<<EOF as a heredoc), a << another word follows
// (a later release reads << EOF as one), and one on ONBUILD (which only some
// releases read there).
func (i dockerfileInstruction) heredocs() ([]dockerfileHeredocEnd, bool) {
	keyword := i.keyword
	if keyword == "ONBUILD" && len(i.words) > 0 {
		keyword = strings.ToUpper(i.words[0])
	}
	if !dockerfileHeredocWords[keyword] || !strings.Contains(i.text, "<<") {
		return nil, true
	}
	words, closed := dockerfileWords(i.text)
	if !closed {
		return nil, false
	}
	var ends []dockerfileHeredocEnd
	for index, word := range words {
		match := dockerfileHeredoc.FindStringSubmatch(word)
		if match == nil {
			if strings.Contains(word, "$") && strings.Contains(word, "<<") {
				return nil, false
			}
			continue
		}
		if match[2] == "" {
			if match[1] == "" && index+1 < len(words) {
				return nil, false
			}
			continue
		}
		name := dockerfileHeredocName.FindStringSubmatch(match[2])
		if name == nil {
			return nil, false
		}
		ends = append(ends, dockerfileHeredocEnd{word: name[1] + name[2] + name[3], chomp: match[1] == "-"})
	}
	if len(ends) > 0 && i.keyword == "ONBUILD" {
		return nil, false
	}
	return ends, true
}

// dockerfileWords splits an instruction's text into words as the shell lexer
// of BuildKit's heredocsFromLine splits it, with the quotes and the escapes
// kept: at white space outside quotes, a backslash keeping the character
// after it in the word, a single quote running to the next one, and a double
// quote to the next one no backslash escapes. closed is false when the text
// leaves a quote open, which BuildKit reads as a line that opens no heredoc.
func dockerfileWords(text string) (words []string, closed bool) {
	var word strings.Builder
	quote := rune(0)
	escaped := false
	for _, character := range text {
		if escaped {
			word.WriteRune(character)
			escaped = false
			continue
		}
		if quote == 0 && unicode.IsSpace(character) {
			if word.Len() > 0 {
				words = append(words, word.String())
				word.Reset()
			}
			continue
		}
		word.WriteRune(character)
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote == 0 && (character == '\'' || character == '"') {
			quote = character
			continue
		}
		if character == quote {
			quote = 0
		}
	}
	if word.Len() > 0 {
		words = append(words, word.String())
	}
	return words, quote == 0
}

// skipHeredoc returns the index of the line that ends a heredoc whose body
// starts after lines[index], or len(lines) when none does.
func skipHeredoc(lines []string, index int, heredoc dockerfileHeredocEnd) int {
	for index++; index < len(lines); index++ {
		candidate := lines[index]
		if heredoc.chomp {
			candidate = strings.TrimLeft(candidate, "\t")
		}
		if candidate == heredoc.word {
			return index
		}
	}
	return index
}

// dockerfileImages returns every image a Dockerfile has the build pull, in the
// order it names them, and where the file stops being one this audit can read:
// the syntax directive's frontend, the image of each FROM that names neither
// scratch nor a stage an earlier FROM declares, each COPY --from that names
// neither a stage of the file nor one by its index, and each RUN --mount
// from= that names no stage of the file.
//
// BuildKit binds a FROM to a stage only when an earlier FROM declares it,
// comparing the name as written with the stage's name in lower case
// (dispatchStates.addState in dockerfile2llb), so a FROM naming the stage it
// declares, or one declared after it, pulls the image of that name; a COPY
// --from and a RUN --mount from= look a stage up in the whole file, in any
// case, and only COPY takes a stage's index. A FROM line sees the ARGs
// declared before the first FROM, and the runner builds with no --build-arg,
// so their defaults are what is substituted; a stage's own ARGs reach its
// COPY and RUN, and a variable there is one only the build knows. An ONBUILD
// instruction runs in a build that uses the image as its base, not in this
// one, and is passed over.
func dockerfileImages(text string) (images []dockerfileImage, unreadable dockerfileUnreadable) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\xef\xbb\xbf"), "\r\n", "\n"), "\n")
	images = dockerfileSyntax(lines)
	instructions, unreadable := readDockerfile(lines)
	stages := dockerfileStages(instructions)
	declared := map[string]bool{}
	globals := map[string]dockerfileArg{}
	staged := false
	for _, instruction := range instructions {
		flags, args := dockerfileFlags(instruction.words)
		switch instruction.keyword {
		case "ARG":
			if !staged {
				declareDockerfileArgs(globals, args)
			}
		case "FROM":
			staged = true
			images = appendDockerfileBase(images, instruction.line, args, globals, declared)
			if name := dockerfileStageName(args); name != "" {
				declared[name] = true
			}
		case "COPY", "RUN":
			for _, from := range dockerfileSources(instruction.keyword, flags) {
				images = appendDockerfileSource(images, instruction, from, stages)
			}
		}
	}
	return images, unreadable
}

// dockerfileStageName is the name a FROM line's words (its flags set aside)
// give its stage, in lower case as BuildKit keeps it, or nothing.
func dockerfileStageName(args []string) string {
	if len(args) == 3 && strings.EqualFold(args[1], "as") {
		return strings.ToLower(args[2])
	}
	return ""
}

// dockerfileStages are the names every FROM ... AS line of a Dockerfile gives
// its stage, which a COPY --from or a RUN --mount from= can name wherever the
// stage is declared.
func dockerfileStages(instructions []dockerfileInstruction) map[string]bool {
	stages := map[string]bool{}
	for _, instruction := range instructions {
		_, args := dockerfileFlags(instruction.words)
		if name := dockerfileStageName(args); instruction.keyword == "FROM" && name != "" {
			stages[name] = true
		}
	}
	return stages
}

// appendDockerfileBase appends the image a FROM line builds from, unless it
// names none, scratch, which is empty, or a stage an earlier FROM declares,
// compared as written with the stage's name in lower case.
func appendDockerfileBase(images []dockerfileImage, line int, args []string, globals map[string]dockerfileArg, declared map[string]bool) []dockerfileImage {
	if len(args) == 0 {
		return images
	}
	reference, resolved := substituteDockerfileArgs(args[0], globals)
	if !resolved {
		reference = args[0]
	} else if reference == "scratch" || declared[reference] {
		return images
	}
	return append(images, dockerfileImage{instruction: "FROM ", reference: reference, line: line, resolved: resolved})
}

// dockerfileSources are the values a COPY's --from flags, or a RUN's --mount
// flags' from= options, name a source by, a mount option's key read in any
// case and its quotes dropped.
func dockerfileSources(keyword string, flags []string) []string {
	var sources []string
	for _, flag := range flags {
		if from, ok := strings.CutPrefix(flag, "--from="); ok && keyword == "COPY" {
			sources = append(sources, from)
		}
		mount, ok := strings.CutPrefix(flag, "--mount=")
		if !ok || keyword != "RUN" {
			continue
		}
		for option := range strings.SplitSeq(mount, ",") {
			if key, from, _ := strings.Cut(strings.Trim(option, `"`), "="); strings.EqualFold(key, "from") {
				sources = append(sources, from)
			}
		}
	}
	return sources
}

// dockerfileFlags splits an instruction's words into the flags in front of
// them, each a word that starts with --, and the rest.
func dockerfileFlags(words []string) (flags, rest []string) {
	for index, word := range words {
		if !strings.HasPrefix(word, "--") {
			return words[:index], words[index:]
		}
	}
	return words, nil
}

// appendDockerfileSource appends the image a COPY --from or a RUN --mount
// from= names, unless it names a stage of the file, in any case, or, on COPY,
// one by its index. A value holding a variable is one only the build knows.
func appendDockerfileSource(images []dockerfileImage, instruction dockerfileInstruction, from string, stages map[string]bool) []dockerfileImage {
	_, notIndex := strconv.Atoi(from)
	if (instruction.keyword == "COPY" && notIndex == nil) || stages[strings.ToLower(from)] {
		return images
	}
	named := "COPY --from="
	if instruction.keyword == "RUN" {
		named = "RUN --mount from="
	}
	return append(images, dockerfileImage{instruction: named, reference: from, line: instruction.line, resolved: !strings.Contains(from, "$")})
}

// declareDockerfileArgs records the ARGs one instruction declares, each NAME
// or NAME=default, a default stripped of the quotes it is written in and
// substituted with the ARGs declared before it. A NAME alone has no default,
// so a variable naming it is one only the build knows.
func declareDockerfileArgs(globals map[string]dockerfileArg, words []string) {
	for _, word := range words {
		name, written, hasDefault := strings.Cut(word, "=")
		value, resolved := substituteDockerfileArgs(unquote(written), globals)
		globals[name] = dockerfileArg{value: value, set: hasDefault && resolved}
	}
}

// substituteDockerfileArgs replaces each $NAME and ${NAME} in word with the
// default of the ARG of that name, and reports false when one names an ARG
// with no default, or none at all, or is written in a form this audit does not
// substitute: ${NAME:-word} and the other modifiers, an unclosed brace, or a $
// with no name after it.
func substituteDockerfileArgs(word string, globals map[string]dockerfileArg) (string, bool) {
	var substituted strings.Builder
	for index := 0; index < len(word); index++ {
		if word[index] != '$' {
			substituted.WriteByte(word[index])
			continue
		}
		rest := word[index+1:]
		name := dockerfileArgName.FindString(rest)
		index += len(name)
		if braced, ok := strings.CutPrefix(rest, "{"); ok {
			inner, _, closed := strings.Cut(braced, "}")
			if !closed || dockerfileArgName.FindString(inner) != inner {
				return "", false
			}
			name = inner
			index += len(inner) + 2
		}
		arg := globals[name]
		if name == "" || !arg.set {
			return "", false
		}
		substituted.WriteString(arg.value)
	}
	return substituted.String(), true
}
