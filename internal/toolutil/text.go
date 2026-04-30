// text.go provides text normalization and escaping utilities for tool
// input processing and safe Markdown output generation.
package toolutil

import (
	"fmt"
	"strings"
)

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

// EscapeMdTableCell escapes characters in s that would break a Markdown table row.
// Pipes are replaced with the HTML entity &#124; and newlines/carriage-returns are
// replaced with a space so the cell stays on a single row.
func EscapeMdTableCell(s string) string {
	s = strings.ReplaceAll(s, "|", "&#124;")
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

// MdTitleLink returns the title as a Markdown link if url is non-empty,
// otherwise returns the escaped title. Suitable for table cells.
func MdTitleLink(title, url string) string {
	escaped := EscapeMdTableCell(title)
	if url == "" {
		return escaped
	}
	return fmt.Sprintf("[%s](%s)", escaped, url)
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
func WrapGFMBody(body string) string {
	if body == "" {
		return ""
	}
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
	return fmt.Sprintf("\n> **Contains**: %s — [view in GitLab](%s) for full rendering.\n", features, webURL)
}

// EscapeMdHeading sanitizes a user-controlled string that will be interpolated
// into a Markdown heading (e.g. `## Project: {name}`). It strips leading '#'
// characters that could promote/demote the heading level and collapses newlines
// into spaces so the heading stays on one line.
func EscapeMdHeading(s string) string {
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

// TitleFromName generates a human-readable UI title from a snake_case MCP tool
// name by stripping the "gitlab_" prefix and converting to Title Case.
//
//	TitleFromName("gitlab_list_projects") // returns "List Projects"
func TitleFromName(name string) string {
	s := strings.TrimPrefix(name, "gitlab_")
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		// Capitalize well-known acronyms
		switch strings.ToLower(p) {
		case "mr", "ci", "ssh", "api", "url", "id", "iid", "gpg", "ssl", "ip", "yaml", "ui":
			parts[i] = strings.ToUpper(p)
		default:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}
