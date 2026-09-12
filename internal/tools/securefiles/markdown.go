package securefiles

import (
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// expiryOrDash formats an optional expiry timestamp in the display form every
// other table in this server shows, or "-" when GitLab sent none. The dash is
// used in table cells to signal "no expiry set" rather than an empty cell.
func expiryOrDash(t *time.Time) string {
	if t == nil {
		return "-"
	}
	if shown := toolutil.FormatTimeValue(*t); shown != "" {
		return shown
	}
	return "-"
}

// FormatListMarkdown formats secure files as markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Files) == 0 {
		return toolutil.EmptyMessage("secure files")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Secure Files", len(out.Files), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Checksum Algorithm", "Expires At"))
	for _, f := range out.Files {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(f.ID, 10),
			toolutil.EscapeMdTableCell(f.Name),
			toolutil.EscapeMdTableCell(f.ChecksumAlgorithm),
			expiryOrDash(f.ExpiresAt),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		"Use `gitlab_show_secure_file` to view details of a specific file")
	return sb.String()
}

// FormatShowMarkdown renders one secure file as the card of one object: the
// identity, the digest, the two timestamps, and the parsed certificate or
// provisioning-profile metadata as a section of its own.
//
// The download hint this card used to carry named `gitlab_show_secure_file`,
// which is the action that produced the card and downloads nothing: GitLab
// serves a secure file's contents only to a CI job, and this server exposes
// no such route. A hint pointing at a route nobody implements is worse than
// no hint, so it is gone.
func FormatShowMarkdown(f SecureFileItem) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, showHeading(f.Name))
	c.Int("ID", f.ID)
	c.Field("Name", f.Name)
	// The digest and the algorithm name are GitLab's own; the digest is a code
	// span because a reader compares it character by character.
	c.Code("Checksum", f.Checksum)
	c.Field("Algorithm", f.ChecksumAlgorithm)
	c.Time("Created At", toolutil.RFC3339Ptr(f.CreatedAt))
	c.Time("Expires At", toolutil.RFC3339Ptr(f.ExpiresAt))
	if f.FileExtension != "" {
		// GitLab derives the extension from the uploaded file's name.
		c.Field("File Extension", f.FileExtension)
	}
	writeMetadataSection(c, f)
	c.End(
		"Use `gitlab_list_secure_files` to see the other secure files in this project",
		"Use `gitlab_remove_secure_file` to delete it",
	)
	return b.String()
}

// writeMetadataSection writes the parsed metadata GitLab extracts from a
// recognized certificate or provisioning profile, and writes nothing at all
// when the object carries no value a reader would see: the section used to be
// opened whenever the key was present, which printed a heading over four
// labels with nothing after them.
func writeMetadataSection(c *toolutil.Card, f SecureFileItem) {
	m := f.Metadata
	if m == nil || !metadataHasContent(m) {
		return
	}
	// Everything below is read out of the certificate whoever uploaded the
	// file supplied, so its common names are whatever they put in it.
	s := c.Section(metadataSectionTitle(f.FileExtension))
	s.Field("ID", m.ID)
	s.Time("Expires At", toolutil.RFC3339Ptr(m.ExpiresAt))
	s.Field("Issuer CN", m.Issuer.CN)
	s.Field("Subject CN", m.Subject.CN)
}

// metadataHasContent reports whether the parsed metadata holds anything the
// card would render.
func metadataHasContent(m *SecureFileMetadata) bool {
	return strings.TrimSpace(m.ID) != "" ||
		m.ExpiresAt != nil ||
		strings.TrimSpace(m.Issuer.CN) != "" ||
		strings.TrimSpace(m.Subject.CN) != ""
}

// metadataSectionTitle names the section after the kind of file the metadata
// was parsed out of. GitLab parses two, and the extension is what says which:
// calling a provisioning profile's metadata a certificate's misnames every
// field under the heading.
func metadataSectionTitle(fileExtension string) string {
	switch strings.ToLower(strings.TrimPrefix(fileExtension, ".")) {
	case "mobileprovision", "provisionprofile":
		return "Provisioning Profile Metadata"
	default:
		return "Certificate Metadata"
	}
}

// showHeading names the card after the file when GitLab sent a name, and
// after the resource alone when it did not, so the heading never ends in a
// colon with nothing behind it.
func showHeading(name string) string {
	if strings.TrimSpace(name) == "" {
		return "Secure File"
	}
	return "Secure File: " + name
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatShowMarkdown)
}
