package attestations

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a single attestation as the card of one
// object: the identity and status rows, the predicate it carries, the subject
// it is about, and the two timestamps in the display form.
func FormatOutputMarkdown(o Output) string {
	if o.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Attestation #%d (IID %d)", o.ID, o.IID))
	c.Int("Project ID", o.ProjectID)
	c.Int("Build ID", o.BuildID)
	c.Field("Status", o.Status)
	c.Field("Predicate Kind", o.PredicateKind)
	// The predicate type is a URI the attestation's producer chose, such as
	// https://slsa.dev/provenance/v1, not a set GitLab constrains.
	c.Field("Predicate Type", o.PredicateType)
	c.Code("Subject Digest", o.SubjectDigest)
	c.Link("Download URL", o.DownloadURL, o.DownloadURL)
	c.Time("Created", o.CreatedAt)
	c.Time("Expires", o.ExpireAt)
	c.End(
		"Use `gitlab_download_attestation` to download this attestation's content",
		"Use `gitlab_list_attestations` to view all attestations for the project",
	)
	return b.String()
}

// FormatListMarkdown renders a list of attestations as a Markdown table.
//
// The endpoint sends no pagination headers, so the heading counts what is
// shown; the empty pagination is what says so rather than a zero total.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Attestations) == 0 {
		return toolutil.EmptyMessage("attestations")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Attestations", len(out.Attestations), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Build", "Status", "Predicate Kind", "Created"))
	for _, a := range out.Attestations {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(a.ID, 10),
			strconv.FormatInt(a.IID, 10),
			strconv.FormatInt(a.BuildID, 10),
			toolutil.EscapeMdTableCell(a.Status),
			toolutil.EscapeMdTableCell(a.PredicateKind),
			toolutil.FormatTime(a.CreatedAt),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		"Use `gitlab_download_attestation` with an IID from the table to fetch one attestation's bundle")
	return b.String()
}

// FormatDownloadMarkdown renders a download result as the card of one object:
// how large the content is and where the bytes themselves are.
func FormatDownloadMarkdown(o DownloadOutput) string {
	if o.AttestationIID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Attestation Download (IID %d)", o.AttestationIID))
	c.Field("Size", fmt.Sprintf("%d bytes", o.Size))
	c.Markdown("Content", "Base64-encoded in the `content_base64` field")
	c.End("Use `gitlab_list_attestations` to view all attestations for the project")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
}
