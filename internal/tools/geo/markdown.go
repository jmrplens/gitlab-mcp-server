package geo

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single Geo site as a Markdown table.
func FormatOutputMarkdown(o Output) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Geo Site: %s\n\n", o.Name)
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&sb, "| Name | %s |\n", o.Name)
	fmt.Fprintf(&sb, "| URL | %s |\n", o.URL)
	if o.InternalURL != "" {
		fmt.Fprintf(&sb, "| Internal URL | %s |\n", o.InternalURL)
	}
	fmt.Fprintf(&sb, "| Primary | %t |\n", o.Primary)
	fmt.Fprintf(&sb, "| Enabled | %t |\n", o.Enabled)
	fmt.Fprintf(&sb, "| Current | %t |\n", o.Current)
	fmt.Fprintf(&sb, "| Files Max Capacity | %d |\n", o.FilesMaxCapacity)
	fmt.Fprintf(&sb, "| Repos Max Capacity | %d |\n", o.ReposMaxCapacity)
	fmt.Fprintf(&sb, "| Verification Max Capacity | %d |\n", o.VerificationMaxCapacity)
	fmt.Fprintf(&sb, "| Sync Object Storage | %t |\n", o.SyncObjectStorage)
	if o.SelectiveSyncType != "" {
		fmt.Fprintf(&sb, "| Selective Sync Type | %s |\n", o.SelectiveSyncType)
	}
	if o.WebEditURL != "" {
		fmt.Fprintf(&sb, "| Web Edit URL | [Edit](%s) |\n", o.WebEditURL)
	}
	return sb.String()
}

// FormatListMarkdown formats a list of Geo sites as a Markdown table.
func FormatListMarkdown(o ListOutput) string {
	var sb strings.Builder
	toolutil.WriteHints(&sb, toolutil.HintPreserveLinks)
	sb.WriteString("## Geo Sites\n\n")
	sb.WriteString("| ID | Name | URL | Primary | Enabled |\n|---|---|---|---|---|\n")
	for _, s := range o.Sites {
		fmt.Fprintf(&sb, "| %d | %s | %s | %t | %t |\n",
			s.ID, s.Name, s.URL, s.Primary, s.Enabled)
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
	sb.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Healthy | %t |\n", o.Healthy)
	fmt.Fprintf(&sb, "| Health Status | %s |\n", o.HealthStatus)
	if o.Health != "" {
		fmt.Fprintf(&sb, "| Health | %s |\n", o.Health)
	}
	fmt.Fprintf(&sb, "| DB Replication Lag | %ds |\n", o.DBReplicationLagSeconds)
	fmt.Fprintf(&sb, "| Missing OAuth App | %t |\n", o.MissingOAuthApplication)
	fmt.Fprintf(&sb, "| Projects Count | %d |\n", o.ProjectsCount)
	fmt.Fprintf(&sb, "| LFS Synced | %s |\n", o.LFSObjectsSyncedInPercentage)
	fmt.Fprintf(&sb, "| Job Artifacts Synced | %s |\n", o.JobArtifactsSyncedInPercentage)
	fmt.Fprintf(&sb, "| Uploads Synced | %s |\n", o.UploadsSyncedInPercentage)
	fmt.Fprintf(&sb, "| Version | %s |\n", o.Version)
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
	sb.WriteString("| Node ID | Healthy | Health Status | DB Lag (s) | Projects | Version |\n|---|---|---|---|---|---|\n")
	for _, s := range o.Statuses {
		fmt.Fprintf(&sb, "| %d | %t | %s | %d | %d | %s |\n",
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
