package pages

import (
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name. Pages is a set of routes on the
// project catalog group, so their domain is "project".
const (
	actionPagesGet      = "project.pages_get"
	actionDomainGet     = "project.pages_domain_get"
	actionDomainList    = "project.pages_domain_list"
	actionDomainUpdate  = "project.pages_domain_update"
	actionDomainListAll = "project.pages_domain_list_all"
)

// FormatPagesMarkdown renders a project's Pages settings as a card, with the
// deployments as a nested collection under a heading of their own.
func FormatPagesMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pages Settings")
	c.URL(out.URL)
	c.Bool("Unique Domain", out.IsUniqueDomainEnabled)
	c.Bool("Force HTTPS", out.ForceHTTPS)
	c.Field("Primary Domain", out.PrimaryDomain)
	if len(out.Deployments) > 0 {
		t := c.Table("Deployments", "URL", "Created", "Path Prefix", "Root Dir")
		for _, d := range out.Deployments {
			t.Row(
				toolutil.MdTitleLink(d.URL, d.URL),
				toolutil.FormatTime(d.CreatedAt),
				toolutil.EscapeMdTableCell(d.PathPrefix),
				toolutil.EscapeMdTableCell(d.RootDirectory),
			)
		}
	}
	c.End(toolutil.HintAction(actionDomainList, "see the project's custom Pages domains"))
	return b.String()
}

// FormatDomainMarkdown renders one Pages custom domain as a card.
//
// The verification code is shown only while the domain is unverified, which is
// the one moment a reader needs it: it is the TXT record GitLab checks for.
func FormatDomainMarkdown(out DomainOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pages Domain: "+out.Domain)
	c.URL(out.URL)
	c.Field("Project ID", projectDisplay(out.ProjectID))
	c.Bool("Verified", out.Verified)
	if !out.Verified {
		c.Code("Verification Code", out.VerificationCode)
	}
	c.Bool("Auto SSL", out.AutoSslEnabled)
	c.Time("Enabled Until", out.EnabledUntil)
	if out.Certificate.Subject != "" {
		cert := c.Sub("Certificate")
		// The subject is read out of a certificate whoever configured the
		// domain supplied, so its common name is whatever they put in it.
		cert.Field("Subject", out.Certificate.Subject)
		cert.Warn("Expired", out.Certificate.Expired)
	}
	c.End(
		toolutil.HintAction(actionDomainUpdate, "change this domain's auto-SSL flag or certificate"),
		toolutil.HintAction(actionPagesGet, "read the project's Pages settings"),
	)
	return b.String()
}

// FormatDomainListMarkdown renders a project's Pages domains as a table.
func FormatDomainListMarkdown(out ListDomainsOutput) string {
	if len(out.Domains) == 0 {
		return toolutil.EmptyMessage("Pages domains")
	}
	return domainTable("Pages Domains", out.Domains, out.Pagination,
		toolutil.HintAction(actionDomainGet, "read one domain in full"),
	)
}

// FormatAllDomainsMarkdown renders every Pages domain on the instance as a
// table. The endpoint is not paginated, so the heading counts what it sent.
func FormatAllDomainsMarkdown(out ListAllDomainsOutput) string {
	if len(out.Domains) == 0 {
		return toolutil.EmptyMessage("Pages domains")
	}
	return domainTable("All Pages Domains", out.Domains, toolutil.PaginationOutput{},
		toolutil.HintAction(actionDomainGet, "read one domain in full"),
		toolutil.HintAction(actionDomainListAll, "list every Pages domain on the instance again"),
	)
}

// domainTable renders the one table shape the two domain listings share.
func domainTable(title string, domains []DomainOutput, pagination toolutil.PaginationOutput, hints ...string) string {
	var b strings.Builder
	toolutil.WriteListHeading(&b, title, len(domains), pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Domain", "URL", "Verified", "Auto SSL", "Project ID"))
	for _, d := range domains {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(d.Domain),
			toolutil.MdTitleLink(d.URL, d.URL),
			toolutil.BoolEmoji(d.Verified),
			toolutil.BoolEmoji(d.AutoSslEnabled),
			projectDisplay(d.ProjectID),
		))
	}
	toolutil.WriteListFooter(&b, pagination, true, hints...)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatPagesMarkdown)
	toolutil.RegisterMarkdown(FormatDomainMarkdown)
	toolutil.RegisterMarkdown(FormatDomainListMarkdown)
	toolutil.RegisterMarkdown(FormatAllDomainsMarkdown)
}
