// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// releaseNotesDir is the repository-relative directory the written summary of
// every release is kept in, one file per release named after its tag. The
// release workflow hands the file of the tag it releases to GoReleaser with
// --release-header, so the GitHub release opens with it and continues with
// the commit list GoReleaser generates.
const releaseNotesDir = ".github/release-notes"

// releaseNotesSections are the sections a release's summary carries, in the
// order the release body shows them: what changed, what breaks, what to do
// when upgrading, and which vulnerabilities the release fixes. A section with
// nothing to say says None., so its absence is never a silence a reader has
// to interpret.
var releaseNotesSections = []string{"Summary", "Breaking changes", "Upgrading", "Security"}

// releaseNotesName is the name of one release's notes: its tag, a plain
// vX.Y.Z, and .md.
var releaseNotesName = regexp.MustCompile(`^v\d+\.\d+\.\d+\.md$`)

// releaseNotesFix is the advice every refused character closes with.
const releaseNotesFix = ": nothing published here uses one, so rewrite the line with a comma, a colon, parentheses or a full stop"

// releaseNotesDashes are the characters a line of a summary may not carry,
// each with how a finding names it. They are built from their code points so
// that this file does not carry them either.
var releaseNotesDashes = []struct {
	character string
	name      string
}{
	{character: string(rune(0x2014)), name: "an em dash (U+2014)"},
	{character: string(rune(0x2013)), name: "an en dash (U+2013)"},
}

// skipCommand and skipChecksTrailer are the ways GitHub reads a commit message
// as a request to skip every workflow a push triggers, matched the way
// scripts/check-em-dash.sh matches them in a pull request description: the
// five bracketed commands anywhere, in any case and with any spacing inside
// the brackets, and the trailer at the start of a line only, since prose may
// name it mid-sentence.
var (
	skipCommand       = regexp.MustCompile(`(?i)\[\s*(?:skip\s+ci|ci\s+skip|no\s+ci|skip\s+actions|actions\s+skip)\s*\]`)
	skipChecksTrailer = regexp.MustCompile("(?i)^\\s*`?skip-checks\\s*:\\s*true")
)

// releaseNotesAttribution are the shapes in which a text credits an assistant
// with writing it: a commit trailer, a footer naming the assistant or calling
// itself machine written, and a link to an assistant's session. Naming a
// client or a tool in prose, which a release note does, is none of them.
var releaseNotesAttribution = []struct {
	pattern *regexp.Regexp
	what    string
}{
	{pattern: regexp.MustCompile(`(?i)^\s*co-authored-by\s*:`), what: "a Co-Authored-By trailer"},
	{
		pattern: regexp.MustCompile(`(?i)\b(?:generated|written|created|drafted)\s+(?:with|by|using)\s+\[?(?:claude|copilot|chatgpt|codex|gemini|cursor|an?\s+(?:ai|assistant|llm)\b)`),
		what:    "a credit to an assistant",
	},
	{pattern: regexp.MustCompile(`(?i)\bclaude\.ai/(?:code|chat|share)/`), what: "a link to an assistant's session"},
}

// errNotADirectory is why the audit stops when the notes directory's path
// names something other than a directory.
var errNotADirectory = errors.New("not a directory")

// checkReleaseNotes holds the release notes under root to the seventh
// property: the release version names has notes, and every file of the
// directory is the notes of one release, carrying the four sections and
// nothing the release body may not publish. A directory that does not exist
// is a release with no notes, and one that cannot be read, a path that is
// not a directory, or a file in it that cannot be read, stops the audit.
//
// The path is stated before it is read, because os.ReadDir does not refuse a
// regular file the same way on every system: on windows-latest the audit
// read a file in the directory's place as no notes at all, and passed.
func checkReleaseNotes(root, version string) ([]string, error) {
	directory := filepath.Join(root, filepath.FromSlash(releaseNotesDir))
	info, err := os.Stat(directory)
	if err == nil && !info.IsDir() {
		err = errNotADirectory
	}
	var entries []fs.DirEntry
	if err == nil {
		entries, err = os.ReadDir(directory)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", directory, err)
	}
	current := "v" + strings.TrimSpace(version) + ".md"
	var problems []string
	if !slices.ContainsFunc(entries, func(entry fs.DirEntry) bool { return entry.Name() == current }) {
		problems = append(problems, releaseNotesDir+"/"+current+": missing: every release opens with a written summary "+
			"of what changed, what breaks, how to upgrade and which vulnerabilities it fixes, and the release "+
			"workflow refuses a tag whose notes are not here (docs/development/release-process.md)")
	}
	for _, entry := range entries {
		label := releaseNotesDir + "/" + entry.Name()
		if entry.IsDir() || !releaseNotesName.MatchString(entry.Name()) {
			problems = append(problems, label+": is not the notes of a release: the directory holds one vX.Y.Z.md per release and nothing else")
			continue
		}
		text, readErr := readTextFile(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		problems = append(problems, releaseNotesProblems(label, text)...)
	}
	return problems, nil
}

// releaseNotesProblems judges one release's notes, label naming the file: the
// sections it carries, then every line it publishes.
func releaseNotesProblems(label, text string) []string {
	if strings.TrimSpace(text) == "" {
		return []string{label + ": is empty: write the four sections, None. where there is nothing to say"}
	}
	lines := splitLines(text)
	problems := sectionProblems(label, lines)
	for index, line := range lines {
		problems = append(problems, lineProblems(fmt.Sprintf("%s:%d", label, index+1), line)...)
	}
	return problems
}

// notesSection is one level-two heading of a summary: its title, the line it
// sits on, and whether any text follows it before the next one.
type notesSection struct {
	title string
	line  int
	body  bool
}

// readSections returns the level-two headings of a summary in the order it
// writes them, and the line of the first text written before the first of
// them, or 0. A heading is a line that opens with "## " outside a fenced code
// block; a deeper heading, a list or a fenced block is text of the section it
// sits in.
func readSections(lines []string) (sections []notesSection, preamble int) {
	fence := ""
	for index, line := range lines {
		if fence != "" || fenceRun(line) != "" {
			fence = nextFence(fence, line)
		} else if title, heading := strings.CutPrefix(line, "## "); heading {
			sections = append(sections, notesSection{title: strings.TrimSpace(title), line: index + 1})
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(sections) > 0 {
			sections[len(sections)-1].body = true
		} else if preamble == 0 {
			preamble = index + 1
		}
	}
	return sections, preamble
}

// fenceRun returns the run of three or more backticks or tildes a line opens
// with once its indentation is set aside, which opens or closes a fenced code
// block, and nothing for any other line.
func fenceRun(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, marker := range []string{"`", "~"} {
		if run := len(trimmed) - len(strings.TrimLeft(trimmed, marker)); run >= 3 {
			return trimmed[:run]
		}
	}
	return ""
}

// nextFence returns the fence a summary is inside after line, given the one it
// was inside before it (empty outside any): a line that opens a fence outside
// one enters it, and inside one only a line holding nothing but a run of the
// same character at least as long closes it, as CommonMark has it.
func nextFence(fence, line string) string {
	run := fenceRun(line)
	if fence == "" {
		return run
	}
	if strings.HasPrefix(run, fence) && run == strings.TrimSpace(line) {
		return ""
	}
	return fence
}

// sectionProblems holds a summary's headings to the four sections, label
// naming the file: each once, in order, with something under it, and nothing
// written above the first.
func sectionProblems(label string, lines []string) []string {
	sections, preamble := readSections(lines)
	var problems []string
	if preamble > 0 {
		problems = append(problems, fmt.Sprintf("%s:%d: opens with text before its first section, where the release body has to open with ## %s",
			label, preamble, releaseNotesSections[0]))
	}
	var order []string
	for _, section := range sections {
		where := fmt.Sprintf("%s:%d: ## %s", label, section.line, section.title)
		switch {
		case !slices.Contains(releaseNotesSections, section.title):
			problems = append(problems, where+" is not one of the four sections ("+strings.Join(releaseNotesSections, ", ")+")")
		case slices.Contains(order, section.title):
			problems = append(problems, where+" appears a second time")
		default:
			order = append(order, section.title)
			if !section.body {
				problems = append(problems, where+" is empty: write None. when there is nothing to say")
			}
		}
	}
	var present []string
	for _, title := range releaseNotesSections {
		if slices.Contains(order, title) {
			present = append(present, title)
			continue
		}
		problems = append(problems, label+": has no ## "+title+" section")
	}
	if !slices.Equal(order, present) {
		problems = append(problems, fmt.Sprintf("%s: carries its sections in the order %s, want %s",
			label, strings.Join(order, ", "), strings.Join(releaseNotesSections, ", ")))
	}
	return problems
}

// lineProblems judges one line of a summary, where naming the file and the
// line: a dash nothing here publishes, a command that makes GitHub skip
// workflows, and a credit to an assistant.
func lineProblems(where, line string) []string {
	var problems []string
	for _, dash := range releaseNotesDashes {
		if strings.Contains(line, dash.character) {
			problems = append(problems, where+": carries "+dash.name+releaseNotesFix)
		}
	}
	if skipCommand.MatchString(line) || skipChecksTrailer.MatchString(line) {
		problems = append(problems, where+": carries a command that makes GitHub skip workflows: pasted into the pull request "+
			"that moves VERSION, it would run no CI on the commit the release tag goes on and no release from the tag")
	}
	for _, credit := range releaseNotesAttribution {
		if credit.pattern.MatchString(line) {
			problems = append(problems, where+": carries "+credit.what+
				": the release body is published under the maintainer's name and credits no assistant with it")
		}
	}
	return problems
}
