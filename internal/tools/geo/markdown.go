package geo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical catalog action IDs the hints name, the one form every surface
// resolves.
const (
	actionGet        = "geo.get"
	actionEdit       = "geo.edit"
	actionRepair     = "geo.repair"
	actionGetStatus  = "geo.get_status"
	actionListSites  = "geo.list"
	actionListStatus = "geo.list_status"
)

// FormatOutputMarkdown renders one Geo site as a card.
//
// A Geo site's name and both URLs are typed by an administrator, and this
// server's own geo.create and geo.edit set them; GitLab validates that a URL
// parses, not which characters its path holds. Every value below therefore
// goes through the card, which escapes each one for the row it writes.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Geo Site: "+o.Name)
	c.Int("ID", o.ID)
	c.Field("Name", o.Name)
	c.URL(o.URL)
	c.Link("Internal URL", "", o.InternalURL)
	c.Bool("Primary", o.Primary)
	c.Bool("Enabled", o.Enabled)
	c.Bool("Current", o.Current)
	c.Int("Files Max Capacity", o.FilesMaxCapacity)
	c.Int("Repos Max Capacity", o.ReposMaxCapacity)
	c.Int("Verification Max Capacity", o.VerificationMaxCapacity)
	c.Field("Blob Download Timeout", secondsValue(o.BlobDownloadTimeout))
	c.Int("Checksum Mismatch Report Threshold", o.ChecksumMismatchReportThreshold)
	c.Field("Checksum Mismatch Self-Heal Cooldown", strconv.FormatInt(o.ChecksumMismatchSelfHealCooldownMinutes, 10)+" min")
	c.Bool("Sync Object Storage", o.SyncObjectStorage)
	c.Field("Selective Sync Type", o.SelectiveSyncType)
	// The scope the selective sync applies to. Without it the type alone says
	// that the site syncs a subset and never which subset.
	c.Field("Selective Sync Shards", strings.Join(o.SelectiveSyncShards, ", "))
	c.Field("Selective Sync Namespace IDs", joinIDs(o.SelectiveSyncNamespaceIDs))
	c.Link("Web Edit URL", "", o.WebEditURL)
	c.Link("Replication Details", "", o.WebGeoReplicationDetailsURL)
	c.End(
		toolutil.HintAction(actionGetStatus, "read this site's replication status"),
		toolutil.HintAction(actionEdit, "change this site's capacities or selective sync"),
		toolutil.HintAction(actionListSites, "see every Geo site on the instance"),
	)
	return b.String()
}

// FormatListMarkdown renders a page of Geo sites as a Markdown table: a
// collection of objects that share columns.
func FormatListMarkdown(o ListOutput) string {
	if len(o.Sites) == 0 {
		return toolutil.EmptyMessage("Geo sites")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Geo Sites", len(o.Sites), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "URL", "Primary", "Enabled"))
	for _, s := range o.Sites {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(s.ID, 10),
			toolutil.EscapeMdTableCell(s.Name),
			toolutil.MdTitleLink(s.URL, s.URL),
			toolutil.BoolEmoji(s.Primary),
			toolutil.BoolEmoji(s.Enabled),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, true,
		toolutil.HintAction(actionGet, "read one site in full"),
		toolutil.HintAction(actionListStatus, "see the replication status of every site"),
	)
	return b.String()
}

// FormatStatusMarkdown renders one Geo site's replication status as a card.
func FormatStatusMarkdown(o StatusOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Geo Site Status (Node ID: %d)", o.GeoNodeID))
	c.Bool("Healthy", o.Healthy)
	c.Field("Health Status", o.HealthStatus)
	// This is the health check's own output, which GitLab's troubleshooting
	// docs say carries the exception message, so an unhealthy secondary alone
	// puts a newline in it; Text quotes a body that has one.
	c.Text("Health", o.Health)
	c.Field("DB Replication Lag", secondsValue(o.DBReplicationLagSeconds))
	c.Warn("Missing OAuth Application", o.MissingOAuthApplication)
	c.Int("Projects Count", o.ProjectsCount)
	c.Int("Repositories Count", o.RepositoriesCount)
	c.Count("Replicables Tracked", int64(len(o.Replicables)))
	c.Field("Storage Shards", storageShardNames(o.StorageShards))
	c.Field("LFS Synced", o.LFSObjectsSyncedInPercentage)
	c.Field("Job Artifacts Synced", o.JobArtifactsSyncedInPercentage)
	c.Field("Uploads Synced", o.UploadsSyncedInPercentage)
	c.Field("Version", o.Version)
	c.Field("Revision", o.Revision)
	c.Bool("Storage Shards Match", o.StorageShardsMatch)
	c.Time("Updated", toolutil.RFC3339(o.UpdatedAt))
	c.End(
		toolutil.HintAction(actionGet, "read the site this status belongs to"),
		toolutil.HintAction(actionRepair, "repair the site's OAuth application"),
	)
	return b.String()
}

// FormatListStatusMarkdown renders a page of Geo site statuses as a Markdown
// table.
func FormatListStatusMarkdown(o ListStatusOutput) string {
	if len(o.Statuses) == 0 {
		return toolutil.EmptyMessage("Geo site statuses")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Geo Site Statuses", len(o.Statuses), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("Node ID", "Healthy", "Health Status", "DB Lag (s)", "Projects", "Version"))
	for _, s := range o.Statuses {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(s.GeoNodeID, 10),
			toolutil.BoolEmoji(s.Healthy),
			toolutil.EscapeMdTableCell(s.HealthStatus),
			strconv.FormatInt(s.DBReplicationLagSeconds, 10),
			strconv.FormatInt(s.ProjectsCount, 10),
			toolutil.EscapeMdTableCell(s.Version),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction(actionGetStatus, "read one site's status in full"),
	)
	return b.String()
}

// secondsValue renders a duration GitLab counts in whole seconds.
func secondsValue(seconds int64) string {
	return strconv.FormatInt(seconds, 10) + "s"
}

// joinIDs renders a list of numeric IDs as one comma-separated value, for the
// selective-sync scope that is a set of namespace IDs.
func joinIDs(ids []int64) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ", ")
}

// storageShardNames joins the shard names for the one row that reports them,
// the whole shard object being a name and nothing else.
func storageShardNames(shards []StorageShard) string {
	names := make([]string, 0, len(shards))
	for _, shard := range shards {
		names = append(names, shard.Name)
	}
	return strings.Join(names, ", ")
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)     // Output
	toolutil.RegisterMarkdown(FormatListMarkdown)       // ListOutput
	toolutil.RegisterMarkdown(FormatStatusMarkdown)     // StatusOutput
	toolutil.RegisterMarkdown(FormatListStatusMarkdown) // ListStatusOutput
}
