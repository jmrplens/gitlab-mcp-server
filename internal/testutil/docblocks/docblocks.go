// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package docblocks

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The ways a page can fail to give a test the block it asked for. Each is
// wrapped with the page, the key and the lines involved, and matched with
// [errors.Is].
var (
	// ErrUnknownKey is a key no fence of the English page carries.
	ErrUnknownKey = errors.New("no fence carries the key")
	// ErrDuplicateKey is a key two fences of one page carry.
	ErrDuplicateKey = errors.New("more than one fence carries the key")
	// ErrNotInTwin is a key the English page carries and its Spanish twin
	// does not.
	ErrNotInTwin = errors.New("the Spanish twin carries no fence with the key")
	// ErrUnclosedFence is a fence the page never closes.
	ErrUnclosedFence = errors.New("the fence is never closed")
	// ErrBadKey is an e2e option whose value is not a key, or a fence that
	// names two.
	ErrBadKey = errors.New("the fence's e2e option is not one key")
)

// keyOption is the fence option that names a block for a test.
const keyOption = "e2e="

// keyPattern is what a key may be: lowercase words joined by hyphens, the
// shape a case name already has, so a key never needs quoting in a fence.
var keyPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// openFence and closeFence are the CommonMark fence lines, at any
// indentation, as check-i18n-code.mjs matches them.
var (
	openFence  = regexp.MustCompile("^([ \t]*)(`{3,}|~{3,})(.*)$")
	closeFence = regexp.MustCompile("^[ \t]*(`{3,}|~{3,})[ \t]*$")
)

// Block is one fenced code block of a page.
type Block struct {
	// Lang is the first word of the fence's info string.
	Lang string
	// Info is the info string after the language, as the page wrote it.
	Info string
	// Key is the value of the fence's e2e option, empty when it has none.
	Key string
	// Body is the content between the fences, each line ending in a newline
	// and stripped of the opening fence's indentation.
	Body string
	// Line is the 1-based line of the opening fence.
	Line int
}

// Page is an English page of the site, read together with its Spanish twin.
type Page struct {
	// Path is the English page's file.
	Path string
	// Twin is the Spanish page's file.
	Twin string

	blocks []Block
	twin   []Block
}

// Parse returns the fenced code blocks of a page's source, in page order.
//
// A page written with carriage returns is read as one without them, which is
// what a renderer makes of it.
func Parse(src []byte) ([]Block, error) {
	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	var blocks []Block
	for i := afterFrontmatter(lines); i < len(lines); i++ {
		open := openFence.FindStringSubmatch(lines[i])
		if open == nil {
			continue
		}
		indent, fence, rest := open[1], open[2], open[3]
		if fence[0] == '`' && strings.Contains(rest, "`") {
			continue
		}
		block, err := fenceInfo(strings.TrimSpace(rest), i+1)
		if err != nil {
			return nil, err
		}
		body, closing, closed := fenceBody(lines[i+1:], indent, fence)
		if !closed {
			return nil, fmt.Errorf("the %s fence at line %d: %w", fence, i+1, ErrUnclosedFence)
		}
		block.Body = body
		blocks = append(blocks, block)
		i += 1 + closing
	}
	return blocks, nil
}

// afterFrontmatter returns the index of the first line after a page's
// frontmatter, or 0 when the page has none: a fence spelled inside a
// frontmatter value is text.
func afterFrontmatter(lines []string) int {
	if lines[0] != "---" {
		return 0
	}
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return i + 1
		}
	}
	return 0
}

// fenceBody reads the lines after an opening fence up to the one that closes
// it, and returns the body together with the closing line's index in lines.
func fenceBody(lines []string, indent, fence string) (body string, closing int, closed bool) {
	var b strings.Builder
	for j, line := range lines {
		if closes(line, fence) {
			return b.String(), j, true
		}
		if after, ok := strings.CutPrefix(line, indent); ok {
			line = after
		} else {
			line = strings.TrimLeft(line, " \t")
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return "", 0, false
}

// closes reports whether line closes a block opened by fence: a run of the
// same character at least as long, and nothing else on the line.
func closes(line, fence string) bool {
	c := closeFence.FindStringSubmatch(line)
	return len(c) == 2 && c[1][0] == fence[0] && len(c[1]) >= len(fence)
}

// fenceInfo reads a fence's info string into a block with no body yet.
//
// The language ends at the first space or tab, as CommonMark ends an info
// string's first word and check-i18n-code.mjs splits it, so a tab after the
// language does not fold the options into it.
func fenceInfo(info string, line int) (Block, error) {
	options := strings.TrimLeftFunc(info, func(r rune) bool { return r != ' ' && r != '\t' })
	block := Block{Lang: info[:len(info)-len(options)], Info: strings.TrimSpace(options), Line: line}
	for _, option := range splitOptions(block.Info) {
		value, ok := strings.CutPrefix(option, keyOption)
		if !ok {
			continue
		}
		if block.Key != "" {
			return Block{}, fmt.Errorf("the fence at line %d names %q and %q: %w", line, block.Key, value, ErrBadKey)
		}
		if !keyPattern.MatchString(value) {
			return Block{}, fmt.Errorf("the fence at line %d has %s%s, and a key is lowercase words joined by hyphens: %w", line, keyOption, value, ErrBadKey)
		}
		block.Key = value
	}
	return block, nil
}

// splitOptions splits a fence's options on the spaces outside quotes, so a
// title="…" holding a space, or holding the text e2e=, stays one option.
func splitOptions(options string) []string {
	var (
		out   []string
		cur   strings.Builder
		quote rune
	)
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range options {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			cur.WriteRune(r)
			continue
		}
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
		}
		cur.WriteRune(r)
	}
	flush()
	return out
}

// Open reads the English page rel under docsDir and its twin under
// docsDir/es, and parses both.
func Open(docsDir, rel string) (*Page, error) {
	page := &Page{
		Path: filepath.Join(docsDir, filepath.FromSlash(rel)),
		Twin: filepath.Join(docsDir, "es", filepath.FromSlash(rel)),
	}
	var err error
	if page.blocks, err = parseFile(page.Path); err != nil {
		return nil, err
	}
	if page.twin, err = parseFile(page.Twin); err != nil {
		return nil, err
	}
	return page, nil
}

// parseFile reads and parses one page, naming it in any error.
func parseFile(path string) ([]Block, error) {
	src, err := os.ReadFile(path) //#nosec G304 -- a documentation page of this repository, named by the test that reads it
	if err != nil {
		return nil, fmt.Errorf("reading the page: %w", err)
	}
	blocks, err := Parse(src)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return blocks, nil
}

// Block returns the one block of the English page whose fence carries
// e2e=key, after checking that exactly one fence of the Spanish twin carries
// it too.
func (p *Page) Block(key string) (Block, error) {
	found, err := only(p.blocks, p.Path, key)
	if err != nil {
		return Block{}, err
	}
	if len(found) == 0 {
		return Block{}, fmt.Errorf("%s: e2e=%s: %w", p.Path, key, ErrUnknownKey)
	}
	inTwin, err := only(p.twin, p.Twin, key)
	if err != nil {
		return Block{}, err
	}
	if len(inTwin) == 0 {
		return Block{}, fmt.Errorf("%s: e2e=%s is on line %d of the English page: %w", p.Twin, key, found[0].Line, ErrNotInTwin)
	}
	return found[0], nil
}

// only returns the blocks of one page that carry key, refusing more than one.
// A fence with no e2e option carries no key, so it never matches, the empty
// key included.
func only(blocks []Block, path, key string) ([]Block, error) {
	var found []Block
	for _, block := range blocks {
		if block.Key != "" && block.Key == key {
			found = append(found, block)
		}
	}
	if len(found) > 1 {
		return nil, fmt.Errorf("%s: e2e=%s is on lines %d and %d: %w", path, key, found[0].Line, found[1].Line, ErrDuplicateKey)
	}
	return found, nil
}
