package toolutil

import (
	"fmt"
	"regexp"
	"strings"
)

// RedactedSecretValue is the standard placeholder for sensitive values that
// must not be exposed through structured output or Markdown renderers.
const RedactedSecretValue = "REDACTED"

// NormalizeText replaces literal escape sequences with real characters.
// MCP clients may send text with literal backslash-n instead of real newlines
// when the JSON transport double-escapes the input.
//
// Replacement order matters to avoid cascading conversions:
//  1. `\\` -> `\`   (double-escaped backslash first, so `\\n` becomes `\` + literal n, not a newline)
//  2. `\r\n` -> `\n` (CRLF before individual CR/LF to avoid double-replacement)
//  3. `\r` -> `\n`   (standalone carriage return)
//  4. `\n` -> newline  (the most common case)
//  5. `\t` -> tab
func NormalizeText(s string) string {
	s = strings.ReplaceAll(s, `\\`, "\\")
	s = strings.ReplaceAll(s, `\r\n`, "\n")
	s = strings.ReplaceAll(s, `\r`, "\n")
	s = strings.ReplaceAll(s, `\n`, "\n")
	s = strings.ReplaceAll(s, `\t`, "\t")
	return s
}

// StripControlBytes removes the C0 and C1 control ranges and DEL from s,
// keeping the three controls Markdown actually uses: tab, newline and carriage
// return.
//
// Everything this package renders is GitLab-authored: an issue title, a file, a
// job log, a note somebody left on a merge request. None of it is written by
// the caller, and a good deal of it is written by whoever can open an issue or
// push a branch, which on a public project is anybody. It reaches the model as
// text and frequently reaches a person's terminal unchanged, where an escape
// sequence is not text but an instruction: ESC[2J clears the screen, ESC]0;…BEL
// renames the window. Nothing downstream filters them, so they are dropped
// here, at the point the text becomes part of a response.
//
// Dropping rather than escaping is deliberate. A rendered escape would still
// have to be un-rendered by something to be read, and the sequences that matter
// carry no information a reader loses by not seeing them: what survives of
// ESC[2J is "[2J", which says plainly that the content tried something.
//
// This covers the text channel and not structuredContent, and that is a
// decision rather than an oversight. structuredContent is marshaled by the SDK
// from the typed value each handler returns, so there is no funnel in this
// package to apply it at: closing it means either a sweep over every output
// struct in the 175 handler packages, or a result middleware in the server that
// walks each value reflectively on the way out. The second is the tempting one
// and it is the wrong trade. encoding/json already escapes every byte in these
// ranges, so what reaches a client is the six characters of a \u001b sequence,
// which no terminal acts on: a viewer would have to parse the JSON and print a
// string field raw to be affected, and such a client is showing the text
// channel too, which is sanitized here. Paying a reflective walk of every
// result for that is not worth it. If the shape of the risk changes, the place
// to add it is one middleware at the server boundary, not this function.
func StripControlBytes(s string) string {
	if strings.IndexFunc(s, isRenderControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isRenderControl(r) {
			return -1
		}
		return r
	}, s)
}

// isRenderControl reports whether r is a control character with no place in
// rendered Markdown.
func isRenderControl(r rune) bool {
	switch r {
	case '\t', '\n', '\r':
		return false
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// EscapeMdTableCell escapes characters in s that would break a Markdown table row.
// Pipes are replaced with the HTML entity &#124; and newlines/carriage-returns are
// replaced with a space so the cell stays on a single row. Control characters are
// dropped by [StripControlBytes].
//
// The opening angle bracket is an entity too, for the reason
// [mdLinkLabelEscaper] gives: a renderer takes raw HTML ahead of whatever
// surrounds it, so a title of
//
//	<a href="http://attacker.invalid/x">Fix login</a>
//
// arrives as a working link to a host that is not GitLab. [MdTitleLink] closed
// that for the link case and returns this escaper's output untouched when the
// item has no URL, which is the ordinary shape for something with no page of
// its own, and around 266 formatters call this one directly.
//
// An entity rather than the backslash the label escaper uses, because
// MdTitleLink pipes a cell through that escaper: a backslash arriving there
// is escaped again into a visible one, where &lt; passes through untouched and
// renders as the character it always was.
//
// Only '<' is escaped. Everything a renderer obeys instead of reading opens
// with it (a tag, a comment, an autolink), so that is the whole of the
// containment, and '>' is left alone because callers compose cells of their
// own: a merge request's branch cell reads "feature/fix -> develop", and
// entity-encoding the arrow the server itself wrote damages output that no
// untrusted text ever touched.
func EscapeMdTableCell(s string) string {
	s = StripControlBytes(s)
	s = strings.ReplaceAll(s, "|", "&#124;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// mdLinkLabelEscaper backslash-escapes the characters that let text inside a
// link label end the label, or open a construct of its own.
//
// The angle brackets are there because CommonMark gives autolinks and raw HTML
// precedence over the link brackets around them: a label holding
// <a href="http://attacker.invalid"> opens an anchor of its own inside the one
// this package wrote, and an HTML-rendering client closes the outer link at it.
// Both are backslash-escapable ASCII punctuation, so the visible text is what
// it always was.
var mdLinkLabelEscaper = strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "`", "\\`", "<", `\<`, ">", `\>`)

// EscapeMdLinkLabel renders s as the visible text of a Markdown link.
//
// A label is not a cell: escaping the pipe keeps the row intact and does
// nothing about the bracket. An issue titled
//
//	Fix login](http://attacker.invalid/x)
//
// closed the label after "Fix login" and opened a destination of its own, so a
// reader saw the issue's title linking to a host that is not GitLab — on this
// server's own instruction, since [HintPreserveLinks] tells the model to keep
// the clickable links so the user can navigate to GitLab.
func EscapeMdLinkLabel(s string) string {
	return mdLinkLabelEscaper.Replace(StripControlBytes(s))
}

// mdLinkDestEscaper percent-encodes the characters that would end a link
// destination early or split it in two. Each encoding resolves to the same
// resource, so the link still works.
var mdLinkDestEscaper = strings.NewReplacer("(", "%28", ")", "%29", "<", "%3C", ">", "%3E", " ", "%20", `"`, "%22")

// EscapeMdLinkDestination renders url as the destination of a Markdown link.
func EscapeMdLinkDestination(url string) string {
	return mdLinkDestEscaper.Replace(StripControlBytes(url))
}

// MdTitleLink returns the title as a Markdown link if url is non-empty,
// otherwise returns the escaped title. Suitable for table cells. Both halves
// are escaped, so neither the title nor the URL can end the link they are in.
func MdTitleLink(title, url string) string {
	escaped := EscapeMdTableCell(title)
	if url == "" {
		return escaped
	}
	return fmt.Sprintf("[%s](%s)", EscapeMdLinkLabel(escaped), EscapeMdLinkDestination(url))
}

// BuildTargetURL constructs a GitLab web URL for a target resource.
// Returns "" when the project web URL is empty, the IID is zero, or the target
// type has no known URL segment.
//
// Supported target types: Issue, MergeRequest, Milestone.
func BuildTargetURL(projectWebURL, targetType string, targetIID int64) string {
	if projectWebURL == "" || targetIID == 0 {
		return ""
	}
	var segment string
	switch targetType {
	case "Issue":
		segment = "issues"
	case "MergeRequest":
		segment = "merge_requests"
	case "Milestone":
		segment = "milestones"
	default:
		return ""
	}
	return fmt.Sprintf("%s/-/%s/%d", projectWebURL, segment, targetIID)
}

// FormatTarget builds a Markdown table cell for a typed target resource.
// When targetURL is non-empty, the result is a clickable link like
// [Issue #42](url). When empty, the label is returned as plain text.
// Returns "" if there is nothing to display.
func FormatTarget(targetType string, targetIID int64, targetTitle, targetURL string) string {
	label := EscapeMdTableCell(targetTitle)
	if label == "" && targetIID != 0 {
		label = fmt.Sprintf("%s #%d", targetType, targetIID)
	}
	if label == "" {
		return ""
	}
	if targetURL != "" {
		return MdTitleLink(label, targetURL)
	}
	return label
}

// WrapGFMBody wraps user-generated GFM content in a Markdown blockquote to prevent
// heading hierarchy conflicts and structural breaks in the formatted output.
// Empty bodies return an empty string.
//
// The quote is the containment: every line of the body is a line of a quote, so
// the body cannot add a heading, a list item or a section to the document it
// sits in. Two things happen before it. Control characters are dropped, and the
// server's own guidance heading is defused, so a body carrying that heading is
// shown as the text it is rather than parsed back out as the server's
// suggestions. See [DefuseHintsHeading].
func WrapGFMBody(body string) string {
	if body == "" {
		return ""
	}
	body = DefuseHintsHeading(StripControlBytes(body))
	// CommonMark counts a CRLF and a bare CR as line endings too (spec 2.1), so
	// splitting on LF alone leaves whatever follows a CR outside the quote. A
	// heading or a list item there is structure rather than a lazy paragraph
	// continuation, which is precisely what the quote exists to prevent, and
	// GitLab stores a note body as it was posted.
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + line
		}
	}
	return strings.Join(lines, "\n")
}

// DetectRichContent scans a GFM body for non-portable features that may not
// render correctly outside GitLab (mermaid diagrams, math blocks, raw HTML).
// Returns a comma-separated list of detected features or an empty string.
func DetectRichContent(body string) string {
	var features []string
	if strings.Contains(body, "```mermaid") {
		features = append(features, "mermaid")
	}
	if strings.Contains(body, "$$") {
		features = append(features, "math")
	}
	if strings.Contains(body, "<details") || strings.Contains(body, "<table") || strings.Contains(body, "<img") {
		features = append(features, "HTML")
	}
	return strings.Join(features, ", ")
}

// RichContentHint returns an informational note directing users to the GitLab
// web URL for full rendering when non-portable GFM features are detected.
// Returns an empty string when features or webURL is empty.
func RichContentHint(features, webURL string) string {
	if features == "" || webURL == "" {
		return ""
	}
	return fmt.Sprintf("\n> **Contains**: %s. %s for full rendering.\n",
		features, MdTitleLink("View in GitLab", webURL))
}

// EscapeMdHeading sanitizes a user-controlled string that will be interpolated
// into a Markdown heading (e.g. `## Project: {name}`). It strips leading '#'
// characters that could promote/demote the heading level and collapses newlines
// into spaces so the heading stays on one line.
//
// The opening angle bracket is escaped for the reason [EscapeMdTableCell]
// gives, and in the same entity form: a heading is the other place a formatter
// puts a GitLab-authored name, all 44 call sites interpolate one value into a
// heading the server wrote, and the same name should not turn into a live tag
// in a heading after being neutralized in a cell.
func EscapeMdHeading(s string) string {
	s = StripControlBytes(s)
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimLeft(s, "# ")
	return s
}

// IsImageFile returns true when the filename has an image extension.
// Comparison is case-insensitive. Returns false for empty strings.
func IsImageFile(filename string) bool {
	return ImageMIMEType(filename) != ""
}

// ImageMIMEType returns the MIME type for image file extensions.
// Returns an empty string for non-image files.
func ImageMIMEType(filename string) string {
	if filename == "" {
		return ""
	}
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	} else {
		return ""
	}
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".ico":
		return "image/x-icon"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// IsBinaryFile returns true when the filename has a known binary extension
// that is not an image. Returns false for text and image files.
func IsBinaryFile(filename string) bool {
	if filename == "" {
		return false
	}
	ext := strings.ToLower(filename)
	if idx := strings.LastIndex(ext, "."); idx >= 0 {
		ext = ext[idx:]
	} else {
		return false
	}
	switch ext {
	case ".pdf", ".zip", ".gz", ".tar", ".bz2", ".xz", ".7z", ".rar",
		".exe", ".dll", ".so", ".dylib", ".bin",
		".woff", ".woff2", ".ttf", ".otf", ".eot",
		".mp3", ".mp4", ".avi", ".mov", ".mkv", ".flac", ".wav", ".ogg",
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".class", ".jar", ".pyc", ".o", ".a", ".lib",
		".sqlite", ".db":
		return true
	default:
		return false
	}
}

// titleAcronyms are the name segments that render fully capitalized in a title.
// A segment also matches in its plural form, which keeps the "s" lowercase:
// "mrs" becomes "MRs", not "MRS".
var titleAcronyms = map[string]bool{
	"mr": true, "ci": true, "ssh": true, "api": true, "url": true, "id": true,
	"iid": true, "gpg": true, "ssl": true, "ip": true, "yaml": true, "ui": true,
}

// TitleFromName generates a human-readable UI title from a snake_case MCP tool
// name by stripping the "gitlab_" prefix and converting to Title Case.
//
//	TitleFromName("gitlab_list_projects") // returns "List Projects"
//	TitleFromName("my_open_mrs")          // returns "My Open MRs"
func TitleFromName(name string) string {
	s := strings.TrimPrefix(name, "gitlab_")
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = titleSegment(p)
	}
	return strings.Join(parts, " ")
}

// titleSegment capitalizes one underscore-separated name segment: a well-known
// acronym in full, a plural acronym with a lowercase "s", anything else as a
// leading capital.
func titleSegment(segment string) string {
	lower := strings.ToLower(segment)
	if titleAcronyms[lower] {
		return strings.ToUpper(segment)
	}
	if stem, ok := strings.CutSuffix(lower, "s"); ok && titleAcronyms[stem] {
		return strings.ToUpper(segment[:len(segment)-1]) + "s"
	}
	return strings.ToUpper(segment[:1]) + segment[1:]
}

// consentURLScheme matches the scheme of a URL a client might render as
// clickable inside an elicitation form.
var consentURLScheme = regexp.MustCompile(`(?i)\b(https?)://`)

// EscapeConsentValue renders caller-controlled text for a consent dialog.
//
// "MCP servers requesting elicitation SHOULD NOT include URLs intended to be
// clickable in any field of a form mode elicitation request." The dialog is
// where a person decides whether to allow an action, so what it shows has to be
// the server's own words plus data that cannot pass for them. Interpolating raw
// values did not meet that: a project path of
//
//	demo/proj\n\n**SECURITY NOTICE** Verify at https://evil.example/verify
//
// reached the user as bold text and a link inside the question they were being
// asked, all of it attacker-supplied by way of the model.
//
// Three things happen here. Line breaks collapse, so a value cannot add
// paragraphs to the dialog's structure. URL schemes are defanged to https[:]//,
// which stays legible while leaving nothing for a client to linkify. And the
// result is fenced in backticks, so emphasis, headings and link syntax inside
// it are shown rather than rendered — using a fence longer than any run of
// backticks in the value, the same rule Markdown itself uses for nesting code.
//
// This is escaping, not sanitizing: the text still reaches the user, and should,
// since a project path they cannot read is no use to them deciding. What it can
// no longer do is impersonate the server asking the question.
func EscapeConsentValue(s string) string {
	s = StripControlBytes(s)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = consentURLScheme.ReplaceAllString(s, "$1[:]//")

	if s == "" {
		// Two bare backticks read as an unclosed span. A code span holding one
		// space is the closest markdown gets to "empty", and a real
		// single-space value rendering identically is an accepted collision.
		return "` `"
	}

	fence := strings.Repeat("`", longestBacktickRun(s)+1)
	// A value that starts or ends with a backtick needs padding, or the fence
	// and the value run together and the span does not close where it should.
	pad := ""
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		pad = " "
	}
	return fence + pad + s + pad + fence
}

// longestBacktickRun returns the length of the longest unbroken run of
// backticks in s, which is what a fence has to exceed to contain it.
func longestBacktickRun(s string) int {
	longest, current := 0, 0
	for _, r := range s {
		if r == '`' {
			current++
			longest = max(longest, current)
			continue
		}
		current = 0
	}
	return longest
}
