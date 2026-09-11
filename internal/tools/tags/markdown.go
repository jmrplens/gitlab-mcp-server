package tags

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name that the action specs do not already
// spell, the one form every surface resolves.
const (
	actionTagCreate       = "tag.create"
	actionTagDelete       = "tag.delete"
	actionTagGetProtected = "tag.get_protected"
	actionReleaseCreate   = "release.create"
)

type tagNotFoundOutput struct {
	Identifier string
}

func formatTagNotFound(out tagNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Tag", out.Identifier,
		"Use gitlab_tag_list with project_id to list tags",
		"Verify the tag name is spelled correctly (case-sensitive)",
	)
}

// FormatOutputMarkdownString renders one tag as a card: the tag's own fields,
// then the commit it points at as a nested object.
//
// The tag object's own id ("target") is shown only when it differs from the
// commit's: for a lightweight tag the two are the same string, and printing it
// twice under two labels invited a reader to treat the tag object as a second
// commit.
func FormatOutputMarkdownString(t Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Tag: "+t.Name)
	c.Bool("Protected", t.Protected)
	if t.Commit == nil || t.Target != t.Commit.ID {
		c.Code("Tag Object", t.Target)
	}
	// The annotated tag message is what whoever tagged typed, and this server's
	// own tag.create writes it: a message of several lines is quoted rather than
	// flattened onto the row.
	c.Text("Message", t.Message)
	c.Time("Created", t.CreatedAt)
	if t.Commit != nil {
		commit := c.Sub("Commit")
		commit.Code("SHA", t.Commit.ID)
		commit.Field("Title", t.Commit.Title)
		commit.Field("Author", t.Commit.AuthorName)
		commit.Time("Committed", t.Commit.CommittedDate)
	}
	if t.Release != nil {
		c.Text("Release", t.Release.Description)
	}
	c.End(
		toolutil.HintAction(actionTagDelete, "remove this tag"),
		toolutil.HintAction(actionReleaseCreate, "create a release from this tag"),
	)
	return b.String()
}

// FormatOutputMarkdown renders a single tag as an MCP CallToolResult.
func FormatOutputMarkdown(t Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatOutputMarkdownString(t))
}

// FormatListMarkdownString renders a page of tags as a Markdown table.
//
// The commit column is the commit the tag resolves to, not the tag object's
// own id: for an annotated tag those differ, and the column that named the tag
// object was a SHA no other action accepts.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Tags) == 0 {
		return toolutil.EmptyMessage("tags")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Tags", len(out.Tags), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Commit", "Protected"))
	for _, t := range out.Tags {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.MdCodeSpanCell(tagCommitSHA(t)),
			toolutil.BoolEmoji(t.Protected),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionTagGet, "see one tag in full"),
		toolutil.HintAction(actionTagCreate, "create a new tag"),
	)
	return b.String()
}

// tagCommitSHA is the commit a tag resolves to: the commit object's id when
// GitLab sent one, and the tag object's target otherwise.
func tagCommitSHA(t Output) string {
	if t.Commit != nil && t.Commit.ID != "" {
		return t.Commit.ID
	}
	return t.Target
}

// FormatListMarkdown renders a list of tags as an MCP CallToolResult.
func FormatListMarkdown(out ListOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListMarkdownString(out))
}

// FormatSignatureMarkdownString renders a tag's X.509 signature as a card with
// the certificate and its issuer as sections of their own.
func FormatSignatureMarkdownString(out SignatureOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Tag Signature")
	c.Field("Signature Type", out.SignatureType)
	c.Field("Verification Status", out.VerificationStatus)
	cert := out.X509Certificate
	// A section is opened only where GitLab sent something to put under it: a
	// heading with nothing beneath reads as content that failed to render.
	if cert != (X509CertificateOutput{}) {
		certificate := c.Section("X.509 Certificate")
		// The subject and the email are read out of the signer's own
		// certificate, which GitLab stores as parsed rather than validating.
		certificate.Field("Subject", cert.Subject)
		certificate.Field("Email", cert.Email)
		certificate.Field("Status", cert.CertificateStatus)
		certificate.Code("Serial Number", cert.SerialNumber)
	}
	if cert.X509Issuer != (X509IssuerOutput{}) {
		issuer := c.Section("Issuer")
		issuer.Field("Subject", cert.X509Issuer.Subject)
		issuer.Link("CRL URL", cert.X509Issuer.CrlURL, cert.X509Issuer.CrlURL)
	}
	c.End(
		toolutil.HintAction(actionTagGet, "see the tag this signature belongs to"),
		toolutil.HintAction(actionTagList, "browse all tags"),
	)
	return b.String()
}

// FormatSignatureMarkdown renders a tag signature as an MCP CallToolResult.
func FormatSignatureMarkdown(out SignatureOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatSignatureMarkdownString(out))
}

// FormatProtectedTagMarkdownString renders one protected tag rule as a card
// whose create access levels are the nested collection they are.
func FormatProtectedTagMarkdownString(out ProtectedTagOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Tag: "+out.Name)
	if len(out.CreateAccessLevels) == 0 {
		c.Note("No create access levels are defined, so no one may create this tag.")
	} else {
		t := c.Table("Create Access Levels", "ID", "Access Level", "Description", "User ID", "Group ID", "Deploy Key ID")
		for _, al := range out.CreateAccessLevels {
			t.Row(
				formatIDCell(al.ID),
				// The role the number stands for: the number alone was a lookup
				// table a reader does not have.
				toolutil.EscapeMdTableCell(toolutil.AccessLevelDescription(gl.AccessLevelValue(al.AccessLevel))),
				// GitLab's own description, which names a role for a plain rule
				// and a person, group or deploy key for a granular one.
				toolutil.EscapeMdTableCell(al.AccessLevelDescription),
				formatIDCell(al.UserID),
				formatIDCell(al.GroupID),
				formatIDCell(al.DeployKeyID),
			)
		}
	}
	c.End(
		toolutil.HintAction(actionTagListProtected, "see all protected tags"),
		toolutil.HintAction(actionTagUnprotect, "remove tag protection"),
	)
	return b.String()
}

// FormatProtectedTagMarkdown renders a protected tag as an MCP CallToolResult.
func FormatProtectedTagMarkdown(out ProtectedTagOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatProtectedTagMarkdownString(out))
}

// FormatListProtectedTagsMarkdownString renders a page of protected tags as a
// Markdown table.
func FormatListProtectedTagsMarkdownString(out ListProtectedTagsOutput) string {
	if len(out.Tags) == 0 {
		return toolutil.EmptyMessage("protected tags")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Protected Tags", len(out.Tags), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Name", "Create Access Levels"))
	for _, t := range out.Tags {
		levels := make([]string, len(t.CreateAccessLevels))
		for i, al := range t.CreateAccessLevels {
			levels[i] = formatAccessLevelSummary(al)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(t.Name),
			toolutil.EscapeMdTableCell(strings.Join(levels, ", ")),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionTagGetProtected, "see one rule in full"),
		toolutil.HintAction(actionTagProtect, "add a new protected tag"),
	)
	return b.String()
}

// FormatListProtectedTagsMarkdown renders a list of protected tags as an MCP CallToolResult.
func FormatListProtectedTagsMarkdown(out ListProtectedTagsOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatListProtectedTagsMarkdownString(out))
}

func init() {
	toolutil.RegisterMarkdownResult(formatTagNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdownString)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
	toolutil.RegisterMarkdown(FormatSignatureMarkdownString)
	toolutil.RegisterMarkdown(FormatProtectedTagMarkdownString)
	toolutil.RegisterMarkdown(FormatListProtectedTagsMarkdownString)
}
