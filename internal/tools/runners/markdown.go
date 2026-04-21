// markdown.go provides Markdown formatting functions for runner MCP tool output.

package runners

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown renders a runner output as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner #%d\n\n", out.ID)
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	fmt.Fprintf(&b, "| Type | %s |\n", out.RunnerType)
	fmt.Fprintf(&b, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&b, "| Paused | %s |\n", toolutil.BoolEmoji(out.Paused))
	fmt.Fprintf(&b, "| Shared | %s |\n", toolutil.BoolEmoji(out.IsShared))
	fmt.Fprintf(&b, "| Online | %s |\n", toolutil.BoolEmoji(out.Online))
	toolutil.WriteHints(&b,
		"Use action 'get' for full runner configuration",
		"Use action 'jobs' to see jobs executed by this runner",
	)
	return b.String()
}

// FormatDetailsMarkdown renders detailed runner information as Markdown.
func FormatDetailsMarkdown(out DetailsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner #%d — Details\n\n", out.ID)
	b.WriteString("| Field | Value |\n")
	b.WriteString(toolutil.TblSep2Col)
	fmt.Fprintf(&b, "| Name | %s |\n", toolutil.EscapeMdTableCell(out.Name))
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	fmt.Fprintf(&b, "| Type | %s |\n", out.RunnerType)
	fmt.Fprintf(&b, "| Status | %s |\n", out.Status)
	fmt.Fprintf(&b, "| Paused | %s |\n", toolutil.BoolEmoji(out.Paused))
	fmt.Fprintf(&b, "| Shared | %s |\n", toolutil.BoolEmoji(out.IsShared))
	fmt.Fprintf(&b, "| Online | %s |\n", toolutil.BoolEmoji(out.Online))
	fmt.Fprintf(&b, "| Locked | %s |\n", toolutil.BoolEmoji(out.Locked))
	fmt.Fprintf(&b, "| Access Level | %s |\n", out.AccessLevel)
	fmt.Fprintf(&b, "| Run Untagged | %s |\n", toolutil.BoolEmoji(out.RunUntagged))
	if len(out.TagList) > 0 {
		fmt.Fprintf(&b, "| Tags | %s |\n", toolutil.EscapeMdTableCell(strings.Join(out.TagList, ", ")))
	}
	if out.MaximumTimeout > 0 {
		fmt.Fprintf(&b, "| Max Timeout | %ds |\n", out.MaximumTimeout)
	}
	if out.MaintenanceNote != "" {
		fmt.Fprintf(&b, "| Maintenance Note | %s |\n", toolutil.EscapeMdTableCell(out.MaintenanceNote))
	}
	if out.ContactedAt != "" {
		fmt.Fprintf(&b, "| Last Contact | %s |\n", toolutil.FormatTime(out.ContactedAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'update' to change runner settings",
		"Use action 'update' with paused=true to pause or resume this runner",
		"Use action 'jobs' to list jobs for this runner",
	)
	return b.String()
}

// FormatListMarkdown renders a list of runners as Markdown.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Runners) == 0 {
		return "No runners found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Runners (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Runners), out.Pagination)
	b.WriteString("| ID | Name | Type | Status | Paused | Shared |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range out.Runners {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %t | %t |\n",
			r.ID, toolutil.EscapeMdTableCell(r.Name), r.RunnerType, r.Status, r.Paused, r.IsShared)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with runner_id for full configuration",
		"Use action 'remove' to unregister a runner",
	)
	return b.String()
}

// FormatJobListMarkdown renders a list of runner jobs as Markdown.
func FormatJobListMarkdown(out JobListOutput) string {
	if len(out.Jobs) == 0 {
		return "No jobs found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Jobs (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Jobs), out.Pagination)
	b.WriteString("| ID | Name | Status | Stage | Ref | Duration |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- |\n")
	for _, j := range out.Jobs {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %.1fs |\n",
			j.ID, toolutil.EscapeMdTableCell(j.Name), j.Status, j.Stage, toolutil.EscapeMdTableCell(j.Ref), j.Duration)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use gitlab_job action 'get' with job_id for full job details")
	return b.String()
}

// FormatAuthTokenMarkdown renders an auth token as Markdown.
func FormatAuthTokenMarkdown(out AuthTokenOutput) string {
	var b strings.Builder
	b.WriteString("## Runner Authentication Token\n\n")
	fmt.Fprintf(&b, "- **Token**: %s\n", out.Token)
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires At**: %s\n", toolutil.FormatTime(out.ExpiresAt))
	}
	toolutil.WriteHints(&b, "Use action 'register' with this token to register a new runner")
	return b.String()
}

// FormatRegTokenMarkdown renders a registration token as Markdown.
func FormatRegTokenMarkdown(out AuthTokenOutput) string {
	var b strings.Builder
	b.WriteString("## Runner Registration Token\n\n")
	fmt.Fprintf(&b, "- **Token**: %s\n", out.Token)
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "- **Expires At**: %s\n", toolutil.FormatTime(out.ExpiresAt))
	}
	toolutil.WriteHints(&b, "Use action 'register' with this token to register a new runner")
	return b.String()
}

// FormatManagerListMarkdown renders a list of runner managers as Markdown.
func FormatManagerListMarkdown(out ManagerListOutput) string {
	if len(out.Managers) == 0 {
		return "No runner managers found.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner Managers (%d)\n\n", len(out.Managers))
	b.WriteString("| ID | System ID | Version | Platform | Arch | Status | IP |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- |\n")
	for _, m := range out.Managers {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s |\n",
			m.ID, toolutil.EscapeMdTableCell(m.SystemID), m.Version, m.Platform, m.Architecture, m.Status, m.IPAddress)
	}
	toolutil.WriteHints(&b, "Use action 'get' with runner_id for full runner information")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDetailsMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
