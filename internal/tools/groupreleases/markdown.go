package groupreleases

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// releaseWebURL derives the release page URL from the _links object, preferring
// the self link and falling back to the edit_url with its trailing "/edit"
// segment trimmed. Returns "" when no links are present.
func releaseWebURL(r Output) string {
	if r.Links == nil {
		return ""
	}
	if r.Links.Self != "" {
		return r.Links.Self
	}
	if r.Links.EditURL != "" {
		return strings.TrimSuffix(r.Links.EditURL, "/edit")
	}
	return ""
}

// releaseAuthor returns the author username for compact rendering, or "" when
// no author is associated with the release.
func releaseAuthor(r Output) string {
	if r.Author == nil {
		return ""
	}
	return r.Author.Username
}

// FormatListMarkdown renders a paginated list of group releases as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Releases) == 0 {
		return "No group releases found.\n"
	}
	var b strings.Builder
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	toolutil.WriteListSummary(&b, len(out.Releases), out.Pagination)
	b.WriteString("| Tag | Name | Released | Author |\n| --- | --- | --- | --- |\n")
	for _, r := range out.Releases {
		tag := toolutil.EscapeMdTableCell(r.TagName)
		if webURL := releaseWebURL(r); webURL != "" {
			tag = fmt.Sprintf("[%s](%s)", tag, webURL)
		}
		fmt.Fprintf(
			&b, "| %s | %s | %s | %s |\n",
			tag,
			toolutil.EscapeMdTableCell(r.Name),
			r.ReleasedAt,
			releaseAuthor(r),
		)
	}
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
