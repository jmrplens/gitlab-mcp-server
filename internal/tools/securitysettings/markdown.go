package securitysettings

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionProjectGet    = "project.security_settings_get"
	actionProjectUpdate = "project.security_settings_update"
	actionGroupUpdate   = "group.security_settings_update"
)

// groupScopeNote is what the group card says instead of letting one flag stand
// for the whole group. The endpoint sets secret push protection for the
// projects of a group, minus the ones the request excluded, and answers with
// the value it was asked for plus the refusals it collected: it never says
// which projects now carry it. A card that printed the flag alone read as a
// per-project outcome the response cannot vouch for.
const groupScopeNote = "This is the value GitLab recorded for the group's projects. A project the request excluded, and any project listed under Errors, keeps the setting it had."

// FormatProjectMarkdown renders project security settings as the card of one
// object: the protections that are on, the per-analyzer auto-fix flags, and
// when the settings were created and last changed.
func FormatProjectMarkdown(o ProjectOutput) string {
	if o.ProjectID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Project Security Settings (Project %d)", o.ProjectID))
	c.Int("Project ID", o.ProjectID)
	c.Bool("Secret Push Protection", o.SecretPushProtectionEnabled)
	c.Bool("Continuous Vulnerability Scans", o.ContinuousVulnerabilityScansEnabled)
	c.Bool("Container Scanning for Registry", o.ContainerScanningForRegistryEnabled)
	c.Bool("Auto-fix SAST", o.AutoFixSAST)
	c.Bool("Auto-fix DAST", o.AutoFixDAST)
	c.Bool("Auto-fix Dependency Scanning", o.AutoFixDependencyScanning)
	c.Bool("Auto-fix Container Scanning", o.AutoFixContainerScanning)
	c.Time("Created", o.CreatedAt)
	c.Time("Updated", o.UpdatedAt)
	c.End(
		toolutil.HintAction(actionProjectUpdate, "turn secret push protection on or off for this project"),
		toolutil.HintAction(actionGroupUpdate, "set it for every project in a group at once"),
	)
	return b.String()
}

// FormatGroupMarkdown renders the answer to a group secret push protection
// change as the card of one object: the value GitLab recorded, the projects it
// refused, and the sentence that keeps the flag from reading as a per-project
// result.
func FormatGroupMarkdown(o GroupOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Group Security Settings")
	c.Bool("Secret Push Protection", o.SecretPushProtectionEnabled)
	c.Count("Projects GitLab could not update", int64(len(o.Errors)))
	if len(o.Errors) > 0 {
		errors := c.Table("Errors", "Error")
		for _, e := range o.Errors {
			// GitLab's own message about what it refused.
			errors.Row(toolutil.EscapeMdTableCell(e))
		}
	}
	c.Note(groupScopeNote)
	c.End(
		toolutil.HintAction(actionGroupUpdate, "change the setting for the group again, excluding the projects that failed"),
		toolutil.HintAction(actionProjectGet, "read one project's settings to see what it carries now"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatProjectMarkdown)
	toolutil.RegisterMarkdown(FormatGroupMarkdown)
}
