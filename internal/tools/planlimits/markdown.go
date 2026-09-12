package planlimits

import (
	"strconv"
	"strings"

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
// package format, each carrying the unit.
//
// Every limit GitLab sends here is a byte count, and the rows used to print the
// number alone, so "5368709120" said neither what it counted nor how big it is.
func formatPlanLimitsMarkdown(title string, limits PlanLimitItem, hint string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, title)
	c.Field("Conan Max File Size", fileSize(limits.ConanMaxFileSize))
	c.Field("Generic Packages Max File Size", fileSize(limits.GenericPackagesMaxFileSize))
	c.Field("Helm Max File Size", fileSize(limits.HelmMaxFileSize))
	c.Field("Maven Max File Size", fileSize(limits.MavenMaxFileSize))
	c.Field("NPM Max File Size", fileSize(limits.NPMMaxFileSize))
	c.Field("NuGet Max File Size", fileSize(limits.NugetMaxFileSize))
	c.Field("PyPI Max File Size", fileSize(limits.PyPiMaxFileSize))
	c.Field("Terraform Module Max File Size", fileSize(limits.TerraformModuleMaxFileSize))
	c.End(hint)
	return b.String()
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
