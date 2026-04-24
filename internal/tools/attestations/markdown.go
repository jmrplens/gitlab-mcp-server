// markdown.go provides Markdown formatting functions for attestation
// MCP tool output.
package attestations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a single attestation as Markdown.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Attestation #%d (IID %d)\n\n", o.ID, o.IID)
	fmt.Fprintf(&b, "- **Project ID**: %d\n", o.ProjectID)
	fmt.Fprintf(&b, "- **Build ID**: %d\n", o.BuildID)
	fmt.Fprintf(&b, "- **Status**: %s\n", o.Status)
	if o.PredicateKind != "" {
		fmt.Fprintf(&b, "- **Predicate Kind**: %s\n", o.PredicateKind)
	}
	if o.PredicateType != "" {
		fmt.Fprintf(&b, "- **Predicate Type**: %s\n", o.PredicateType)
	}
	if o.SubjectDigest != "" {
		fmt.Fprintf(&b, "- **Subject Digest**: `%s`\n", o.SubjectDigest)
	}
	if o.DownloadURL != "" {
		fmt.Fprintf(&b, "- **Download URL**: %s\n", o.DownloadURL)
	}
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, o.CreatedAt)
	}
	if o.ExpireAt != "" {
		fmt.Fprintf(&b, "- **Expires**: %s\n", o.ExpireAt)
	}
	toolutil.WriteHints(&b,
		"Use `gitlab_download_attestation` to download this attestation's content",
		"Use `gitlab_list_attestations` to view all attestations for the project",
	)
	return b.String()
}

// FormatListMarkdown renders a list of attestations as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Attestations) == 0 {
		return "No attestations found."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Attestations (%d)\n\n", len(out.Attestations))
	b.WriteString("| ID | IID | Build | Status | Predicate Kind | Created |\n")
	b.WriteString("| --: | --: | ----: | ------ | -------------- | ------- |\n")
	for _, a := range out.Attestations {
		fmt.Fprintf(&b, "| %d | %d | %d | %s | %s | %s |\n",
			a.ID,
			a.IID,
			a.BuildID,
			toolutil.EscapeMdTableCell(a.Status),
			toolutil.EscapeMdTableCell(a.PredicateKind),
			toolutil.EscapeMdTableCell(a.CreatedAt),
		)
	}
	toolutil.WriteHints(&b, toolutil.HintPreserveLinks)
	return b.String()
}

// FormatDownloadMarkdown renders a download result as Markdown.
func FormatDownloadMarkdown(o DownloadOutput) string {
	if o.AttestationIID == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Attestation Download (IID %d)\n\n", o.AttestationIID)
	fmt.Fprintf(&b, "- **Size**: %d bytes\n", o.Size)
	b.WriteString("- **Content**: Base64-encoded in the `content_base64` field\n")
	toolutil.WriteHints(&b,
		"Use `gitlab_list_attestations` to view all attestations for the project",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
}
