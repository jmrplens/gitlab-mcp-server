package runners

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatOutputMarkdown renders a runner output as Markdown.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner #%d\n\n", out.ID)
	toolutil.WriteMdField(&b, "Name", out.Name)
	toolutil.WriteMdField(&b, "Description", out.Description)
	//gitlab:allow-unescaped out.RunnerType: a runner type GitLab picks from a fixed set (instance_type, group_type, project_type).
	toolutil.WriteMdFieldRendered(&b, "Type", out.RunnerType)
	//gitlab:allow-unescaped out.Status: a runner status GitLab derives from when the runner last contacted it (online, offline, stale, never_contacted).
	toolutil.WriteMdFieldRendered(&b, "Status", out.Status)
	toolutil.WriteMdFieldBool(&b, "Paused", out.Paused)
	toolutil.WriteMdFieldBool(&b, "Shared", out.IsShared)
	toolutil.WriteMdFieldBool(&b, "Online", out.Online)
	toolutil.WriteHints(
		&b,
		"Use action 'get' for full runner configuration",
		"Use action 'jobs' to see jobs executed by this runner",
	)
	return b.String()
}

// FormatDetailsMarkdown renders detailed runner information as Markdown.
func FormatDetailsMarkdown(out DetailsOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Runner #%d: Details\n\n", out.ID)
	toolutil.WriteMdField(&b, "Name", out.Name)
	toolutil.WriteMdField(&b, "Description", out.Description)
	//gitlab:allow-unescaped out.RunnerType: a runner type GitLab picks from a fixed set (instance_type, group_type, project_type).
	toolutil.WriteMdFieldRendered(&b, "Type", out.RunnerType)
	//gitlab:allow-unescaped out.Status: a runner status GitLab derives from when the runner last contacted it (online, offline, stale, never_contacted).
	toolutil.WriteMdFieldRendered(&b, "Status", out.Status)
	toolutil.WriteMdFieldBool(&b, "Paused", out.Paused)
	toolutil.WriteMdFieldBool(&b, "Shared", out.IsShared)
	toolutil.WriteMdFieldBool(&b, "Online", out.Online)
	toolutil.WriteMdFieldBool(&b, "Locked", out.Locked)
	//gitlab:allow-unescaped out.AccessLevel: a runner access level GitLab picks from a fixed set (not_protected, ref_protected), and refuses any other value on register and update.
	toolutil.WriteMdFieldRendered(&b, "Access Level", out.AccessLevel)
	toolutil.WriteMdFieldBool(&b, "Run Untagged", out.RunUntagged)
	if len(out.TagList) > 0 {
		toolutil.WriteMdField(&b, "Tags", strings.Join(out.TagList, ", "))
	}
	if out.MaximumTimeout > 0 {
		toolutil.WriteMdFieldRendered(&b, "Max Timeout", fmt.Sprintf("%ds", out.MaximumTimeout))
	}
	toolutil.WriteMdFieldIf(&b, "Maintenance Note", out.MaintenanceNote)
	toolutil.WriteMdFieldTime(&b, "Last Contact", out.ContactedAt)
	if len(out.Projects) > 0 {
		toolutil.WriteMdFieldInt(&b, "Projects", int64(len(out.Projects)))
	}
	if len(out.Groups) > 0 {
		toolutil.WriteMdFieldInt(&b, "Groups", int64(len(out.Groups)))
	}
	toolutil.WriteHints(
		&b,
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
			//gitlab:allow-unescaped r.RunnerType: the list rows carry the same runner type enum the single-runner table writes.
			//gitlab:allow-unescaped r.Status: the list rows carry the same status enum the single-runner table writes.
			r.ID, toolutil.EscapeMdTableCell(r.Name), r.RunnerType, r.Status, r.Paused, r.IsShared)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
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
			//gitlab:allow-unescaped j.Status: a job status, GitLab's own build state (created, running, success, failed and the rest).
			// The stage name is written in .gitlab-ci.yml, and the jobs package
			// escapes the same field in both of its tables.
			j.ID, toolutil.EscapeMdTableCell(j.Name), j.Status, toolutil.EscapeMdTableCell(j.Stage), toolutil.EscapeMdTableCell(j.Ref), j.Duration)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b, "Use gitlab_job action 'get' with job_id for full job details")
	return b.String()
}

// FormatAuthTokenMarkdown renders an auth token as Markdown.
func FormatAuthTokenMarkdown(out AuthTokenOutput) string {
	var b strings.Builder
	b.WriteString("## Runner Authentication Token\n\n")
	//gitlab:allow-unescaped out.Token: a token GitLab minted, a fixed prefix and URL-safe characters, which the reader has to copy back verbatim.
	toolutil.WriteMdFieldRendered(&b, "Token", out.Token)
	toolutil.WriteMdFieldTime(&b, "Expires At", out.ExpiresAt)
	toolutil.WriteHints(&b, "Use action 'register' with this token to register a new runner")
	return b.String()
}

// FormatRegTokenMarkdown renders a registration token as Markdown.
func FormatRegTokenMarkdown(out AuthTokenOutput) string {
	var b strings.Builder
	b.WriteString("## Runner Registration Token\n\n")
	//gitlab:allow-unescaped out.Token: a token GitLab minted, a fixed prefix and URL-safe characters, which the reader has to copy back verbatim.
	toolutil.WriteMdFieldRendered(&b, "Token", out.Token)
	toolutil.WriteMdFieldTime(&b, "Expires At", out.ExpiresAt)
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
	b.WriteString("| ID | System ID | Version | Platform | Arch | Status | Job Status | IP |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, m := range out.Managers {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			// The version, platform and architecture come out of the info
			// payload the runner process posts, which GitLab length-checks and
			// nothing else, and this server's own runner.register writes them.
			//gitlab:allow-unescaped m.Status: a manager status GitLab derives from when the manager last contacted it (online, offline, stale, never_contacted).
			//gitlab:allow-unescaped m.JobExecutionStatus: a job execution status GitLab derives from the manager's running builds (active, idle).
			//gitlab:allow-unescaped m.IPAddress: GitLab fills this from the address the manager connected from, never from the info payload beside it.
			m.ID, toolutil.EscapeMdTableCell(m.SystemID), toolutil.EscapeMdTableCell(m.Version),
			toolutil.EscapeMdTableCell(m.Platform), toolutil.EscapeMdTableCell(m.Architecture), m.Status,
			m.JobExecutionStatus, m.IPAddress)
	}
	toolutil.WriteHints(&b, "Use action 'get' with runner_id for full runner information")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatDetailsMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatJobListMarkdown)
	toolutil.RegisterMarkdown(FormatAuthTokenMarkdown)
	toolutil.RegisterMarkdown(FormatRegTokenMarkdown)
	toolutil.RegisterMarkdown(FormatManagerListMarkdown)
}
