package attestations

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
	//gitlab:allow-unescaped o.Status: an attestation status GitLab picks from a fixed set (pending, success, failed).
	fmt.Fprintf(&b, "- **Status**: %s\n", o.Status)
	if o.PredicateKind != "" {
		fmt.Fprintf(&b, "- **Predicate Kind**: %s\n", toolutil.EscapeMdTableCell(o.PredicateKind))
	}
	if o.PredicateType != "" {
		// The predicate type is a URI the attestation's producer chose, such as
		// https://slsa.dev/provenance/v1, not a set GitLab constrains.
		fmt.Fprintf(&b, "- **Predicate Type**: %s\n", toolutil.EscapeMdTableCell(o.PredicateType))
	}
	if o.SubjectDigest != "" {
		//gitlab:allow-unescaped o.SubjectDigest: a subject digest, hexadecimal digits after an algorithm name.
		fmt.Fprintf(&b, "- **Subject Digest**: `%s`\n", o.SubjectDigest)
	}
	if o.DownloadURL != "" {
		fmt.Fprintf(&b, "- **Download URL**: %s\n", toolutil.EscapeMdTableCell(o.DownloadURL))
	}
	if o.CreatedAt != "" {
		//gitlab:allow-unescaped o.CreatedAt: a timestamp this package formatted itself, with time.Time.Format as RFC 3339.
		fmt.Fprintf(&b, toolutil.FmtMdCreated, o.CreatedAt)
	}
	if o.ExpireAt != "" {
		//gitlab:allow-unescaped o.ExpireAt: a timestamp this package formatted itself, with time.Time.Format as RFC 3339.
		fmt.Fprintf(&b, "- **Expires**: %s\n", o.ExpireAt)
	}
	toolutil.WriteHints(
		&b,
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
		fmt.Fprintf(
			&b, "| %d | %d | %d | %s | %s | %s |\n",
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
	toolutil.WriteHints(
		&b,
		"Use `gitlab_list_attestations` to view all attestations for the project",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
}
