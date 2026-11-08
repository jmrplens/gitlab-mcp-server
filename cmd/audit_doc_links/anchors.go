// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// One file is read in one place, and the place decides how its headings are
// named. A file under site/src/content is a page of the documentation site,
// rendered by Starlight; every other file is read in the repository, rendered
// by GitHub. The two agree on almost every heading in this corpus and
// disagree in ways that are easy to hit by accident, so each is taught here
// rather than one being made to stand for both.

// dialect is how one renderer names the headings of a page.
type dialect struct {
	name string
	// pageAnchors are the anchors every page has whatever it holds.
	pageAnchors []string
	// newSlugger returns a function naming the headings of one page in
	// order, which is what tells a repeated heading from the first.
	newSlugger func() func(text string) string
}

// gitHub is the dialect of a file read in the repository. Its anchors come
// from html-pipeline's TableOfContentsFilter: the text is downcased, every
// character outside Ruby's \p{Word} plus hyphen and space is dropped, the
// spaces become hyphens, and a repeat takes "-N" from a counter kept per
// original slug. Ruby's \p{Word} is Alphabetic, Mark, Digit,
// Connector_Punctuation and Join_Control, which is why the two zero-width
// joiners of an emoji sequence survive in a GitHub anchor and nowhere else.
// A page read in the repository has only the anchors its own headings and
// HTML make.
var gitHub = dialect{
	name: "GitHub",
	newSlugger: func() func(string) string {
		counts := map[string]int{}
		return func(text string) string {
			id := slugBase(text, true)
			seen := counts[id]
			counts[id] = seen + 1
			if seen > 0 {
				return id + "-" + strconv.Itoa(seen)
			}
			return id
		}
	},
}

// starlight is the dialect of a page of the documentation site. Its anchors
// come from Astro's rehypeHeadingIds, which is github-slugger: the same shape
// without Join_Control, and a repeat resolved by walking up from "-1" until a
// free name is found rather than by a counter. The two orders part company on
// a page whose headings are "Foo", "Foo" and "Foo 1": GitHub hands the third
// the "foo-1" it already gave the second, github-slugger gives it "foo-1-1".
//
// Starlight gives every page a "_top" anchor, on the <h1> it renders from the
// frontmatter title. That title is not a Markdown heading and never becomes a
// slug, which is why a site page is linked by "#_top" and a repository file
// by the slug of the "# Title" it carries in its body.
var starlight = dialect{
	name:        "Starlight",
	pageAnchors: []string{"_top"},
	newSlugger: func() func(string) string {
		occurrences := map[string]int{}
		return func(text string) string {
			original := slugBase(text, false)
			result := original
			for {
				if _, taken := occurrences[result]; !taken {
					break
				}
				occurrences[original]++
				result = original + "-" + strconv.Itoa(occurrences[original])
			}
			occurrences[result] = 0
			return result
		}
	},
}

// slugBase is the slug of text before any repeat is resolved: downcased,
// every character dropped but a letter, a mark, a decimal or letter number, a
// connector, a hyphen or a space (and, for GitHub, a join control), and each
// space a hyphen.
func slugBase(text string, joinControl bool) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if r == ' ' {
			b.WriteByte('-')
			continue
		}
		if r == '-' || unicode.In(r, unicode.L, unicode.M, unicode.Nd, unicode.Nl, unicode.Pc) ||
			(joinControl && unicode.Is(unicode.Join_Control, r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// atxHeading matches an ATX heading and captures its text.
var atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+(.*?))?[ \t]*$`)

// setextUnderline matches the line that makes the paragraph line above it a
// Setext heading.
var setextUnderline = regexp.MustCompile(`^ {0,3}(=+|-+)[ \t]*$`)

// closingHashes are the hashes an ATX heading may close with.
var closingHashes = regexp.MustCompile(`[ \t]+#+[ \t]*$`)

// htmlComment, image, link, referenceLink and htmlTag are the constructs a
// reader does not see the source of, each with what it shows instead.
var (
	htmlComment   = regexp.MustCompile(`(?s)<!--.*?-->`)
	image         = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	link          = regexp.MustCompile(`\[([^\]]*)\]\((?:[^()]|\([^()]*\))*\)`)
	referenceLink = regexp.MustCompile(`\[([^\]]*)\]\[[^\]]*\]`)
	htmlTag       = regexp.MustCompile(`</?[A-Za-z][A-Za-z0-9-]*(?:\s[^<>]*)?/?>`)
)

// explicitID matches an element that states its own anchor.
var explicitID = regexp.MustCompile(`<[A-Za-z][^<>]*?\s(?:id|name)\s*=\s*["']([^"']+)["'][^<>]*>`)

// blockOpener matches the start of a line that opens a block owning its
// line, which a Setext underline cannot turn into a heading.
var blockOpener = regexp.MustCompile(`^ {0,3}(?:[>#|<]|[-*+](?:[ \t]|$)|\d+[.)](?:[ \t]|$))`)

// collectAnchors returns the anchors a page answers to in its dialect.
func collectAnchors(content string, d dialect) map[string]bool {
	anchors := map[string]bool{}
	for _, anchor := range d.pageAnchors {
		anchors[anchor] = true
	}
	slug := d.newSlugger()
	// The line before the current one, when it could be the text of a
	// heading underlined in the Setext style.
	underlinable, hasUnderlinable := "", false

	eachProseLine(withoutFrontmatter(content), func(line string, _ int) {
		for _, id := range explicitIDs(line) {
			anchors[id] = true
		}
		if atx := atxHeading.FindStringSubmatch(line); atx != nil {
			anchors[slug(headingText(atx[1]))] = true
			hasUnderlinable = false
			return
		}
		// A row of dashes under a line carrying pipes is a table, not a
		// heading; a row of equals signs is never anything else.
		if underline := setextUnderline.FindStringSubmatch(line); underline != nil && hasUnderlinable &&
			(underline[1][0] == '=' || !strings.Contains(underlinable, "|")) {
			anchors[slug(headingText(underlinable))] = true
			hasUnderlinable = false
			return
		}
		underlinable, hasUnderlinable = strings.TrimSpace(line), isParagraphLine(line)
	})
	return anchors
}

// headingText renders a heading's source the way a reader sees it, and only
// as far as the slug can tell the difference. The characters a slugger keeps
// are letters, digits, underscores, hyphens and spaces, so a construct
// matters here exactly when dropping its markup changes one of those: a
// link's URL, an image's URL, a reference label, a tag name and the text of a
// comment all disappear on the page and would otherwise be slugged.
// Backticks, asterisks and tildes need no handling at all, since the slugger
// drops them wherever they stand.
func headingText(raw string) string {
	text := stripRepeatedly(closingHashes.ReplaceAllString(raw, ""), htmlComment)
	text = image.ReplaceAllString(text, "$1")
	text = link.ReplaceAllString(text, "$1")
	text = referenceLink.ReplaceAllString(text, "$1")
	return strings.TrimSpace(withoutUnderscoreEmphasis(stripRepeatedly(text, htmlTag)))
}

// stripRepeatedly removes every match and then looks again, because one pass
// over nested markup leaves a piece of the outer construct behind: "<<b>i>"
// loses the "<b>" and leaves "<i>", and "<!--<!-- -->-->" leaves "-->". What
// is left would go on to be slugged, and a heading whose anchor nobody can
// guess is exactly the failure this command is meant to report rather than
// cause.
func stripRepeatedly(text string, pattern *regexp.Regexp) string {
	for {
		stripped := pattern.ReplaceAllString(text, "")
		if stripped == text {
			return text
		}
		text = stripped
	}
}

// withoutUnderscoreEmphasis drops emphasis written with underscores while
// keeping the ones inside a name like audit_md_escaping, which both sluggers
// keep: only a pair on a word boundary is emphasis. It is the script's
// expression (?<![\p{L}\p{N}_])__?([^_]+?)__?(?![\p{L}\p{N}_]) replaced by its
// group, written out because Go's expressions have no lookaround: at each
// position not preceded by a letter, a number or an underscore, one or two
// underscores, then text with none up to the next underscore, then one or two
// more, followed by none of the three.
func withoutUnderscoreEmphasis(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if content, end, ok := emphasisAt(text, i); ok {
			b.WriteString(content)
			i = end
			continue
		}
		_, size := utf8.DecodeRuneInString(text[i:])
		b.WriteString(text[i : i+size])
		i += size
	}
	return b.String()
}

// emphasisAt reports whether underscore emphasis opens at byte i of text,
// with what it shows and where it ends.
func emphasisAt(text string, i int) (content string, end int, ok bool) {
	if text[i] != '_' {
		return "", 0, false
	}
	// At the start of the text there is no rune before, which decodes as
	// utf8.RuneError, no word rune.
	if before, _ := utf8.DecodeLastRuneInString(text[:i]); isWordRune(before) {
		return "", 0, false
	}
	for _, open := range []int{2, 1} {
		start := i + open
		if strings.Count(text[i:min(start, len(text))], "_") != open {
			continue
		}
		closing := strings.IndexByte(text[start:], '_')
		if closing <= 0 {
			continue
		}
		closing += start
		for _, width := range []int{2, 1} {
			after := closing + width
			if after > len(text) || strings.Count(text[closing:after], "_") != width {
				continue
			}
			// At the end of the text the rune after decodes as
			// utf8.RuneError, no word rune either.
			if next, _ := utf8.DecodeRuneInString(text[after:]); isWordRune(next) {
				continue
			}
			return text[start:closing], after, true
		}
	}
	return "", 0, false
}

// isWordRune reports whether r is a letter, a number or an underscore, what
// stops an underscore from opening or closing emphasis.
func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// explicitIDs collects the anchors a line states rather than derives. Both
// renderers honor one: Astro keeps an id a heading or component already
// carries instead of slugging it, and GitHub resolves a fragment to the
// "<a id>" or "<a name>" it prefixed.
func explicitIDs(line string) []string {
	var ids []string
	for _, match := range explicitID.FindAllStringSubmatch(line, -1) {
		ids = append(ids, match[1])
	}
	return ids
}

// isParagraphLine answers whether a line could be the text of a Setext
// heading, which is to say an ordinary paragraph: not a blank, not indented
// code, and not the opening of a block that owns its line. A bullet with
// nothing after it is one of those blocks, which is how the two empty list
// items of .github/pull_request_template.md read as a heading underlined by
// the second.
func isParagraphLine(line string) bool {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "    ") {
		return false
	}
	return !blockOpener.MatchString(line)
}
