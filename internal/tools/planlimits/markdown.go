package planlimits

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet    = "admin.plan_limits_get"
	actionChange = "admin.plan_limits_change"
)

// FormatGetMarkdown renders one plan's limits as a card.
func FormatGetMarkdown(out GetOutput) string {
	return formatPlanLimitsMarkdown("Plan Limits", out.PlanLimitItem,
		toolutil.HintAction(actionChange, "raise or lower one of these limits"))
}

// FormatChangeMarkdown renders the limits a change returned as the same card.
func FormatChangeMarkdown(out ChangeOutput) string {
	return formatPlanLimitsMarkdown("Updated Plan Limits", out.PlanLimitItem,
		toolutil.HintAction(actionGet, "read the plan's limits back"))
}

// formatPlanLimitsMarkdown renders one plan's limits as a card: one row per
// limit, the unit GitLab counts it in carried by the value for a byte count and
// by the label otherwise, then the change history when GitLab kept one.
//
// The rows used to print the number alone, so "5368709120" said neither what
// it counted nor how big it is. A limit the instance sent no key for writes no
// row, since it is one the instance does not have rather than one set to zero.
func formatPlanLimitsMarkdown(title string, limits PlanLimitItem, hint string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, title)
	c.Field("Cargo Max File Size", optionalFileSize(limits.CargoMaxFileSize))
	c.Field("Conan Max File Size", fileSize(limits.ConanMaxFileSize))
	c.Field("Generic Packages Max File Size", fileSize(limits.GenericPackagesMaxFileSize))
	c.Field("Helm Max File Size", fileSize(limits.HelmMaxFileSize))
	c.Field("Maven Max File Size", fileSize(limits.MavenMaxFileSize))
	c.Field("NPM Max File Size", fileSize(limits.NPMMaxFileSize))
	c.Field("NuGet Max File Size", fileSize(limits.NugetMaxFileSize))
	c.Field("PyPI Max File Size", fileSize(limits.PyPiMaxFileSize))
	c.Field("Terraform Module Max File Size", fileSize(limits.TerraformModuleMaxFileSize))
	c.Field("CI Instance-Level Variables", amount(limits.CIInstanceLevelVariables))
	c.Field("CI Pipeline Size (jobs in one pipeline)", amount(limits.CIPipelineSize))
	c.Field("CI Active Jobs (jobs in active pipelines)", amount(limits.CIActiveJobs))
	c.Field("CI Project Subscriptions", amount(limits.CIProjectSubscriptions))
	c.Field("CI Pipeline Schedules", amount(limits.CIPipelineSchedules))
	c.Field("CI Needs Size Limit (needs per job)", amount(limits.CINeedsSizeLimit))
	c.Field("CI Registered Group Runners (per group, past seven days)", amount(limits.CIRegisteredGroupRunners))
	c.Field("CI Registered Project Runners (per project, past seven days)", amount(limits.CIRegisteredProjectRunners))
	c.Field("Pipeline Hierarchy Size (downstream pipelines)", amount(limits.PipelineHierarchySize))
	c.Field("Max Pipelines per Merge Train", amount(limits.MaxPipelinesPerMergeTrain))
	c.Field("Dotenv Variables (per artifact)", amount(limits.DotenvVariables))
	c.Field("Dotenv Size", optionalFileSize(limits.DotenvSize))
	c.Field("Storage Size Limit (MiB)", amount(limits.StorageSizeLimit))
	c.Field("Enforcement Limit (MiB)", amount(limits.EnforcementLimit))
	c.Field("Notification Limit (MiB)", amount(limits.NotificationLimit))
	c.Field("Webhook Calls (per minute, per top-level namespace)", rate(limits.WebHookCalls))
	c.Field("Webhook Calls Low (per minute, per top-level namespace)", rate(limits.WebHookCallsLow))
	c.Field("Webhook Calls Mid (per minute, per top-level namespace)", rate(limits.WebHookCallsMid))
	c.Field("Service Desk Outbound Emails per Hour (per top-level namespace)", rate(limits.ServiceDeskOutboundEmailsPerHour))
	c.Field("Service Desk Outbound Emails per Day (per top-level namespace)", rate(limits.ServiceDeskOutboundEmailsPerDay))
	writeLimitsHistory(c, limits.LimitsHistory)
	c.End(hint)
	return b.String()
}

// writeLimitsHistory writes every recorded change, one row each, the limits in
// name order and each limit's changes in the order GitLab kept them.
func writeLimitsHistory(c *toolutil.Card, history map[string][]LimitChange) {
	if len(history) == 0 {
		return
	}
	t := c.Table("Limits History", "Limit", "Value", "Changed By", "Changed At")
	for _, name := range slices.Sorted(maps.Keys(history)) {
		for _, change := range history[name] {
			t.Row(
				toolutil.EscapeMdTableCell(name),
				strconv.FormatInt(change.Value, 10),
				toolutil.EscapeMdTableCell(change.Username),
				toolutil.FormatTimeValue(time.Unix(change.Timestamp, 0)),
			)
		}
	}
}

// amount renders a limit that counts something, whose unit its row's label
// names, or nothing when the instance sent no key for it.
func amount(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

// rate renders a limit GitLab documents as unlimited at zero, and says so: a
// zero rate would otherwise read as nothing allowed.
func rate(v *int64) string {
	if v != nil && *v == 0 {
		return "unlimited (0)"
	}
	return amount(v)
}

// optionalFileSize renders a byte limit the way [fileSize] does, or nothing
// when the instance sent no key for it.
func optionalFileSize(v *int64) string {
	if v == nil {
		return ""
	}
	return fileSize(*v)
}

// fileSize renders a byte count as the row shows it: the exact number of bytes
// always, preceded by the binary-prefix size once the value passes a kibibyte
// and a reader can judge it at a glance.
//
// Both halves are there on purpose. The prefix is what a person reads, and the
// exact count is what the change action takes back, so a limit read here can be
// written there unchanged.
func fileSize(bytes int64) string {
	exact := strconv.FormatInt(bytes, 10)
	if bytes < 1024 {
		return exact + " bytes"
	}
	value := float64(bytes)
	unit := ""
	for _, prefix := range []string{"KiB", "MiB", "GiB", "TiB", "PiB", "EiB"} {
		value /= 1024
		unit = prefix
		if value < 1024 {
			break
		}
	}
	rounded := strings.TrimSuffix(strconv.FormatFloat(value, 'f', 1, 64), ".0")
	return rounded + " " + unit + " (" + exact + " bytes)"
}

func init() {
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatChangeMarkdown)
}
