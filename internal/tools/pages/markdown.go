package pages

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const hintDomainGet = "Use `gitlab_pages_domain_get` to view details of a specific domain"

// FormatPagesMarkdown formats Pages settings for display.
func FormatPagesMarkdown(out Output) string {
	var sb strings.Builder
	sb.WriteString("## Pages Settings\n\n")
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| URL | %s |\n", out.URL)
	fmt.Fprintf(&sb, "| Unique Domain | %v |\n", out.IsUniqueDomainEnabled)
	fmt.Fprintf(&sb, "| Force HTTPS | %v |\n", out.ForceHTTPS)
	fmt.Fprintf(&sb, "| Primary Domain | %s |\n", out.PrimaryDomain)
	if len(out.Deployments) > 0 {
		sb.WriteString("\n### Deployments\n\n| URL | Created | Path Prefix | Root Dir |\n|---|---|---|---|\n")
		for _, d := range out.Deployments {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s |\n", toolutil.MdTitleLink(d.URL, d.URL), toolutil.FormatTime(d.CreatedAt), d.PathPrefix, d.RootDirectory)
		}
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		hintDomainGet,
	)
	return sb.String()
}

// FormatDomainMarkdown formats a single Pages domain for display.
func FormatDomainMarkdown(out DomainOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Pages Domain: %s\n\n", out.Domain)
	sb.WriteString("| Property | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| URL | %s |\n", toolutil.MdTitleLink(out.URL, out.URL))
	fmt.Fprintf(&sb, "| Project | %s |\n", projectDisplay(out.ProjectPath, out.ProjectID))
	fmt.Fprintf(&sb, "| Verified | %v |\n", out.Verified)
	fmt.Fprintf(&sb, "| Auto SSL | %v |\n", out.AutoSslEnabled)
	if out.EnabledUntil != "" {
		fmt.Fprintf(&sb, "| Enabled Until | %s |\n", out.EnabledUntil)
	}
	if out.Certificate.Subject != "" {
		fmt.Fprintf(&sb, "| Cert Subject | %s |\n", out.Certificate.Subject)
		fmt.Fprintf(&sb, "| Cert Expired | %v |\n", out.Certificate.Expired)
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		"Use `gitlab_pages_domain_update` to modify domain settings",
	)
	return sb.String()
}

// FormatDomainListMarkdown formats a list of Pages domains.
func FormatDomainListMarkdown(out ListDomainsOutput) string {
	if len(out.Domains) == 0 {
		return "No Pages domains found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## Pages Domains\n\n| Domain | URL | Verified | Auto SSL | Project |\n|---|---|---|---|---|\n")
	for _, d := range out.Domains {
		fmt.Fprintf(&sb, "| %s | %s | %v | %v | %s |\n", d.Domain, toolutil.MdTitleLink(d.URL, d.URL), d.Verified, d.AutoSslEnabled, projectDisplay(d.ProjectPath, d.ProjectID))
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		hintDomainGet,
	)
	return sb.String()
}

// FormatAllDomainsMarkdown formats a list of all Pages domains.
func FormatAllDomainsMarkdown(out ListAllDomainsOutput) string {
	if len(out.Domains) == 0 {
		return "No Pages domains found.\n"
	}
	var sb strings.Builder
	sb.WriteString("## All Pages Domains\n\n| Domain | URL | Verified | Auto SSL | Project |\n|---|---|---|---|---|\n")
	for _, d := range out.Domains {
		fmt.Fprintf(&sb, "| %s | %s | %v | %v | %s |\n", d.Domain, toolutil.MdTitleLink(d.URL, d.URL), d.Verified, d.AutoSslEnabled, projectDisplay(d.ProjectPath, d.ProjectID))
	}
	toolutil.WriteHints(&sb,
		toolutil.HintPreserveLinks,
		hintDomainGet,
	)
	return sb.String()
}

// FormatDeleteMarkdown returns a confirmation for domain deletion.
func FormatDeleteMarkdown(domain string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Pages domain `%s` deleted successfully.", domain)
	toolutil.WriteHints(&b, "Use `gitlab_pages_domain_list` to verify deletion")
	return b.String()
}

// FormatUnpublishMarkdown returns a confirmation for pages unpublish.
func FormatUnpublishMarkdown() string {
	var b strings.Builder
	b.WriteString("Pages unpublished successfully.")
	toolutil.WriteHints(&b, "Use `gitlab_pages_domain_list` to see remaining domains")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatPagesMarkdown)
	toolutil.RegisterMarkdown(FormatDomainMarkdown)
	toolutil.RegisterMarkdown(FormatDomainListMarkdown)
	toolutil.RegisterMarkdown(FormatAllDomainsMarkdown)
}
