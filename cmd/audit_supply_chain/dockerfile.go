// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"strconv"
	"strings"
)

// dockerfileDirective is a parser directive, which BuildKit reads only from
// the lines at the top of a Dockerfile, up to the first line that is not one:
// a comment, an empty line or an instruction (moby/buildkit,
// frontend/dockerfile/parser). Its key is read in any case.
var dockerfileDirective = regexp.MustCompile(`^#[ \t]*([a-zA-Z][a-zA-Z0-9]*)[ \t]*=[ \t]*(.+?)[ \t]*$`)

// dockerfileHeredoc is a word of a RUN, COPY or ADD instruction that opens a
// heredoc, as BuildKit reads one: an optional file descriptor, <<, an optional
// - that strips the leading tabs of the body's lines, and the word, quoted or
// not, that ends the body on a line of its own. A here-string, <<<, is not
// one, and neither is a << that no word follows.
var dockerfileHeredoc = regexp.MustCompile(`^\d*<<(-?)([^<]*)$`)

// dockerfileArgName is the name a $NAME reference reads.
var dockerfileArgName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*`)

// dockerfileHeredocWords are the instructions BuildKit reads a heredoc on.
var dockerfileHeredocWords = map[string]bool{"ADD": true, "COPY": true, "RUN": true}

// dockerfileInstruction is one instruction of a Dockerfile: the line it starts
// on, its keyword in upper case, and the words after it, its continuation
// lines joined.
type dockerfileInstruction struct {
	keyword string
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

// dockerfileArg is an ARG declared before the first FROM: its default, and
// whether it has one this audit could resolve.
type dockerfileArg struct {
	value string
	set   bool
}

// readDockerfile splits a Dockerfile into its instructions as BuildKit's
// parser does. The parser directives at the top come first: escape changes
// the character that continues a line, and syntax names the image the build
// runs as its frontend, which is returned as the first image the build pulls.
// Then each instruction, its keyword in any case: a line that ends in the
// escape character, white space after it included, goes on on the next, a
// comment line or an empty one is dropped wherever it is, and the body of every
// heredoc a RUN, COPY or ADD opens is skipped, since none of it is an
// instruction. unterminated is the line of an instruction whose heredoc no
// line ends, which BuildKit refuses to build, and 0 when there is none.
func readDockerfile(text string) (instructions []dockerfileInstruction, syntax *dockerfileImage, unterminated int) {
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\xef\xbb\xbf"), "\r\n", "\n"), "\n")
	escape := `\`
	index := 0
	for ; index < len(lines); index++ {
		directive := dockerfileDirective.FindStringSubmatch(lines[index])
		if directive == nil {
			break
		}
		switch strings.ToLower(directive[1]) {
		case "escape":
			if directive[2] == "`" {
				escape = "`"
			}
		case "syntax":
			syntax = &dockerfileImage{instruction: "# syntax=", reference: directive[2], line: index + 1, resolved: true}
		}
	}
	var pending []string
	first := 0
	for ; index < len(lines); index++ {
		line := lines[index]
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if pending == nil {
			first = index + 1
		}
		if body := strings.TrimRight(line, " \t"); strings.HasSuffix(body, escape) {
			pending = append(pending, strings.TrimSuffix(body, escape))
			continue
		}
		instructions = appendDockerfileInstruction(instructions, first, append(pending, line))
		pending = nil
		for _, heredoc := range instructions[len(instructions)-1].heredocs() {
			if index = skipHeredoc(lines, index, heredoc); index == len(lines) {
				return instructions, syntax, first
			}
		}
	}
	if pending != nil {
		instructions = appendDockerfileInstruction(instructions, first, pending)
	}
	return instructions, syntax, 0
}

// appendDockerfileInstruction joins the lines of one instruction, as BuildKit
// joins a continued line, with nothing between them, and appends it split
// into its keyword and its words. Lines that hold nothing but the escape
// character leave an instruction with no keyword, which names nothing and
// opens no heredoc.
func appendDockerfileInstruction(instructions []dockerfileInstruction, line int, parts []string) []dockerfileInstruction {
	words := strings.Fields(strings.Join(parts, ""))
	if len(words) == 0 {
		return append(instructions, dockerfileInstruction{line: line})
	}
	return append(instructions, dockerfileInstruction{keyword: strings.ToUpper(words[0]), words: words[1:], line: line})
}

// dockerfileHeredocEnd is the line that ends one heredoc's body, and whether
// the body's lines are read without their leading tabs.
type dockerfileHeredocEnd struct {
	word  string
	chomp bool
}

// heredocs are the heredocs an instruction opens, in the order its words
// name them, which is the order their bodies follow it in.
func (i dockerfileInstruction) heredocs() []dockerfileHeredocEnd {
	if !dockerfileHeredocWords[i.keyword] {
		return nil
	}
	var ends []dockerfileHeredocEnd
	for _, word := range i.words {
		if match := dockerfileHeredoc.FindStringSubmatch(word); len(match) == 3 && match[2] != "" {
			ends = append(ends, dockerfileHeredocEnd{word: strings.Trim(match[2], `"'`), chomp: match[1] == "-"})
		}
	}
	return ends
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
// order it names them, and the line of a heredoc no line ends, 0 when there is
// none: the syntax directive's frontend, the image of each FROM that is not
// scratch or a stage, and each COPY --from and RUN --mount from= that names
// neither a stage by its name, in any case, nor one by its index.
//
// A FROM line sees the ARGs declared before the first FROM, and the runner
// builds with no --build-arg, so their defaults are what is substituted; a
// stage's own ARGs reach its COPY and RUN, and a variable there is one only
// the build knows. An ONBUILD instruction runs in a build that uses the image
// as its base, not in this one, and is passed over.
func dockerfileImages(text string) (images []dockerfileImage, unterminated int) {
	instructions, syntax, unterminated := readDockerfile(text)
	if syntax != nil {
		images = append(images, *syntax)
	}
	stages := dockerfileStages(instructions)
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
			images = appendDockerfileBase(images, instruction.line, args, globals, stages)
		case "COPY", "RUN":
			for _, from := range dockerfileSources(instruction.keyword, flags) {
				images = appendDockerfileSource(images, instruction, from, stages)
			}
		}
	}
	return images, unterminated
}

// dockerfileStages are the names a Dockerfile's FROM ... AS lines give their
// stages, in lower case, as BuildKit compares them. A stage can be named after
// the line that refers to it, so they are all collected first.
func dockerfileStages(instructions []dockerfileInstruction) map[string]bool {
	stages := map[string]bool{}
	for _, instruction := range instructions {
		_, args := dockerfileFlags(instruction.words)
		if instruction.keyword == "FROM" && len(args) == 3 && strings.EqualFold(args[1], "as") {
			stages[strings.ToLower(args[2])] = true
		}
	}
	return stages
}

// appendDockerfileBase appends the image a FROM line builds from, unless it
// names none, scratch, which is empty, or a stage.
func appendDockerfileBase(images []dockerfileImage, line int, args []string, globals map[string]dockerfileArg, stages map[string]bool) []dockerfileImage {
	if len(args) == 0 {
		return images
	}
	reference, resolved := substituteDockerfileArgs(args[0], globals)
	if !resolved {
		reference = args[0]
	} else if reference == "scratch" || stages[strings.ToLower(reference)] {
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
// from= names, unless it names a stage, by its name or by its index. A value
// holding a variable is one only the build knows.
func appendDockerfileSource(images []dockerfileImage, instruction dockerfileInstruction, from string, stages map[string]bool) []dockerfileImage {
	if _, notIndex := strconv.Atoi(from); notIndex == nil || stages[strings.ToLower(from)] {
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
		if strings.HasPrefix(rest, "{") {
			end := strings.IndexByte(rest, '}')
			if end < 0 {
				return "", false
			}
			name = rest[1:end]
			index += end + 1
			if dockerfileArgName.FindString(name) != name {
				return "", false
			}
		}
		arg := globals[name]
		if name == "" || !arg.set {
			return "", false
		}
		substituted.WriteString(arg.value)
	}
	return substituted.String(), true
}
