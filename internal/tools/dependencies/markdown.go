package dependencies

import (
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatListMarkdown renders a paginated list of dependencies as a Markdown
// table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Dependencies) == 0 {
		return toolutil.EmptyMessage("dependencies")
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "Project Dependencies", len(out.Dependencies), out.Pagination)
	sb.WriteString(toolutil.MarkdownTableHeader("Name", "Version", "Package Manager", "Vulns", "Licenses", "Malware"))
	for _, d := range out.Dependencies {
		sb.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(d.Name),
			toolutil.EscapeMdTableCell(d.Version),
			toolutil.EscapeMdTableCell(d.PackageManager),
			strconv.Itoa(len(d.Vulnerabilities)),
			strconv.Itoa(len(d.Licenses)),
			malwareCell(d.Malware),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&sb, out.Pagination, false,
		"Use `gitlab_create_dependency_list_export` to export the whole list as a CycloneDX SBOM")
	return sb.String()
}

// malwareCell renders the three answers GitLab gives about malware, and never
// as a bare tick: [toolutil.BoolEmoji] maps true to a tick, which under the
// heading "Malware" reads as "this package is fine". A detection therefore
// carries the warning sign, a cleared package the tick, and a package no scan
// covered a dash, since an absent flag means the scan did not run rather than
// that the package is clean.
func malwareCell(malware *bool) string {
	switch {
	case malware == nil:
		return "-"
	case *malware:
		return toolutil.EmojiWarning + " detected"
	default:
		return toolutil.EmojiSuccess + " clear"
	}
}

// FormatExportMarkdown renders a dependency list export status as the card of
// one object.
func FormatExportMarkdown(e ExportOutput) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, "Dependency List Export")
	c.Int("ID", e.ID)
	c.Bool("Finished", e.HasFinished)
	c.Field("Self", e.Self)
	c.Field("Download", e.Download)
	c.End("Use `gitlab_download_dependency_list_export` once the export has finished")
	return sb.String()
}

// FormatDownloadMarkdown renders the downloaded SBOM content as Markdown.
func FormatDownloadMarkdown(d DownloadOutput) string {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, "Dependency List Export (CycloneDX SBOM)")
	// The SBOM names every component of the project, and those names come out
	// of the project's own dependency files. JSON escaping leaves a backtick
	// alone, so a dependency named with a run of three would close a
	// three-backtick fence and put the rest of the document at the top level of
	// the response; the card sizes the fence to the body.
	if strings.TrimSpace(d.Content) == "" {
		c.Note("GitLab returned no SBOM content for this export.")
		return sb.String()
	}
	c.Fence("", "json", d.Content)
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatExportMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
}
