package geo

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// FormatOutputMarkdown formats a single Geo site as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	var sb strings.Builder
	// A Geo site's name and both URLs are typed by an administrator, and this
	// server's own geo.create and geo.edit set them; GitLab validates that a
	// URL parses, not which characters its path holds.
	fmt.Fprintf(&sb, "## Geo Site: %s\n\n", toolutil.EscapeMdHeading(o.Name))
	sb.WriteString(toolutil.TblFieldValue)
	fmt.Fprintf(&sb, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&sb, "| Name | %s |\n", toolutil.EscapeMdTableCell(o.Name))
	fmt.Fprintf(&sb, "| URL | %s |\n", toolutil.EscapeMdTableCell(o.URL))
	if o.InternalURL != "" {
		fmt.Fprintf(&sb, "| Internal URL | %s |\n", toolutil.EscapeMdTableCell(o.InternalURL))
	}
	fmt.Fprintf(&sb, "| Primary | %t |\n", o.Primary)
	fmt.Fprintf(&sb, "| Enabled | %t |\n", o.Enabled)
	fmt.Fprintf(&sb, "| Current | %t |\n", o.Current)
	fmt.Fprintf(&sb, "| Files Max Capacity | %d |\n", o.FilesMaxCapacity)
	fmt.Fprintf(&sb, "| Repos Max Capacity | %d |\n", o.ReposMaxCapacity)
	fmt.Fprintf(&sb, "| Verification Max Capacity | %d |\n", o.VerificationMaxCapacity)
	fmt.Fprintf(&sb, "| Sync Object Storage | %t |\n", o.SyncObjectStorage)
	if o.SelectiveSyncType != "" {
		//gitlab:allow-unescaped o.SelectiveSyncType: one of the two values GitLab validates this field against, namespaces or shards.
		fmt.Fprintf(&sb, "| Selective Sync Type | %s |\n", o.SelectiveSyncType)
	}
	if o.WebEditURL != "" {
		fmt.Fprintf(&sb, "| Web Edit URL | %s |\n", toolutil.MdTitleLink("Edit", o.WebEditURL))
	}
	return sb.String()
}

// FormatListMarkdown formats a list of Geo sites as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Geo Sites\n\n")
	sb.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "URL", "Primary", "Enabled"))
	for _, s := range o.Sites {
		fmt.Fprintf(&sb, "| %d | %s | %s | %t | %t |\n",
			s.ID, toolutil.EscapeMdTableCell(s.Name), toolutil.EscapeMdTableCell(s.URL), s.Primary, s.Enabled)
	}
	if o.Pagination.Page != 0 {
		fmt.Fprintf(&sb, "\n_Page %d, %d sites shown._\n", o.Pagination.Page, len(o.Sites))
	}
	return sb.String()
}

// FormatStatusMarkdown formats a single Geo site status as a Markdown table.
func FormatStatusMarkdown(o StatusOutput) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Geo Site Status (Node ID: %d)\n\n", o.GeoNodeID)
	sb.WriteString(toolutil.TblFieldValue)
	fmt.Fprintf(&sb, "| Healthy | %t |\n", o.Healthy)
	//gitlab:allow-unescaped o.HealthStatus: the state GitLab derives from the health check, Healthy or Unhealthy, never the message itself.
	fmt.Fprintf(&sb, "| Health Status | %s |\n", o.HealthStatus)
	if o.Health != "" {
		// This is the health check's own output, which GitLab's troubleshooting
		// docs say carries the exception message, so an unhealthy secondary
		// alone puts a newline in this cell.
		fmt.Fprintf(&sb, "| Health | %s |\n", toolutil.EscapeMdTableCell(o.Health))
	}
	fmt.Fprintf(&sb, "| DB Replication Lag | %ds |\n", o.DBReplicationLagSeconds)
	fmt.Fprintf(&sb, "| Missing OAuth App | %t |\n", o.MissingOAuthApplication)
	fmt.Fprintf(&sb, "| Projects Count | %d |\n", o.ProjectsCount)
	//gitlab:allow-unescaped o.LFSObjectsSyncedInPercentage: a percentage GitLab formatted for display, of the shape "100.00%".
	fmt.Fprintf(&sb, "| LFS Synced | %s |\n", o.LFSObjectsSyncedInPercentage)
	//gitlab:allow-unescaped o.JobArtifactsSyncedInPercentage: a percentage GitLab formatted for display, of the shape "100.00%".
	fmt.Fprintf(&sb, "| Job Artifacts Synced | %s |\n", o.JobArtifactsSyncedInPercentage)
	//gitlab:allow-unescaped o.UploadsSyncedInPercentage: a percentage GitLab formatted for display, of the shape "100.00%".
	fmt.Fprintf(&sb, "| Uploads Synced | %s |\n", o.UploadsSyncedInPercentage)
	//gitlab:allow-unescaped o.Version: the GitLab version the site runs, compiled into that instance rather than typed by anyone.
	fmt.Fprintf(&sb, "| Version | %s |\n", o.Version)
	//gitlab:allow-unescaped o.Revision: the short Git revision of the site's build, hexadecimal digits.
	fmt.Fprintf(&sb, "| Revision | %s |\n", o.Revision)
	fmt.Fprintf(&sb, "| Storage Shards Match | %t |\n", o.StorageShardsMatch)
	if !o.UpdatedAt.IsZero() {
		fmt.Fprintf(&sb, "| Updated At | %s |\n", o.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
	return sb.String()
}

// FormatListStatusMarkdown formats a list of Geo site statuses as a Markdown table.
func FormatListStatusMarkdown(o ListStatusOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Geo Site Statuses\n\n")
	sb.WriteString(toolutil.MarkdownTableHeader("Node ID", "Healthy", "Health Status", "DB Lag (s)", "Projects", "Version"))
	for _, s := range o.Statuses {
		fmt.Fprintf(&sb, "| %d | %t | %s | %d | %d | %s |\n",
			//gitlab:allow-unescaped s.HealthStatus: the state GitLab derives from the health check, Healthy or Unhealthy, never the message itself.
			//gitlab:allow-unescaped s.Version: the GitLab version the site runs, compiled into that instance rather than typed by anyone.
			s.GeoNodeID, s.Healthy, s.HealthStatus, s.DBReplicationLagSeconds, s.ProjectsCount, s.Version)
	}
	if o.Pagination.Page != 0 {
		fmt.Fprintf(&sb, "\n_Page %d, %d statuses shown._\n", o.Pagination.Page, len(o.Statuses))
	}
	return sb.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)     // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)       // ListOutput
	toolutil.RegisterMarkdown(FormatStatusMarkdown)     // StatusOutput
	toolutil.RegisterMarkdown(FormatListStatusMarkdown) // ListStatusOutput
}
