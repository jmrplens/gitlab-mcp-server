package runners

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionRunnerGet          = "runner.get"
	actionRunnerJobs         = "runner.jobs"
	actionRunnerList         = "runner.list"
	actionRunnerUpdate       = "runner.update"
	actionRunnerRemove       = "runner.remove"
	actionRunnerRegister     = "runner.register"
	actionRunnerVerify       = "runner.verify"
	actionRunnerListManagers = "runner.list_managers"
	actionJobGet             = "job.get"
)

// writeRunnerSummary writes the fields a runner carries wherever it is
// rendered, so the summary and the detail card cannot drift apart.
func writeRunnerSummary(c *toolutil.Card, name, description, runnerType, status, jobStatus string, shared, online, paused bool) {
	c.Field("Name", name)
	c.Field("Description", description)
	// A runner type GitLab picks from a fixed set (instance_type, group_type,
	// project_type) and a status it derives from when the runner last
	// contacted it (online, offline, stale, never_contacted).
	c.Field("Type", runnerType)
	c.Field("Status", status)
	c.Field("Job Execution Status", jobStatus)
	c.Bool("Shared", shared)
	c.Bool("Online", online)
	// Paused is the one negative-polarity flag here: a tick against "Paused"
	// reads as a runner in good order, which is the opposite of what it means.
	c.Warn("Paused", paused)
}

// FormatOutputMarkdown renders one runner as the card of a single object.
//
// A runner GitLab has just created or registered answers with the
// authentication token it minted, once and nowhere else, so the card shows it
// as a secret and [toolutil.Card.End] adds the sentence saying it cannot be
// read back. The table this replaced published every other field and dropped
// the token on the floor, which made the register action unusable: the value
// it exists to return was not in its answer.
func FormatOutputMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner #%d", out.ID))
	c.Int("ID", out.ID)
	writeRunnerSummary(c, out.Name, out.Description, out.RunnerType, out.Status, out.JobExecutionStatus,
		out.IsShared, out.Online, out.Paused)
	c.Code("IP Address", out.IPAddress)
	c.Time("Created", out.CreatedAt)
	if out.CreatedBy != nil {
		c.Markdown("Created By", toolutil.MdUserLink(out.CreatedBy.Username, out.CreatedBy.WebURL))
	}
	c.Secret("Token", out.Token)
	c.Time("Token Expires", out.TokenExpiresAt)
	c.End(
		toolutil.HintAction(actionRunnerGet, "see this runner's full configuration"),
		toolutil.HintAction(actionRunnerJobs, "list the jobs it has run"),
	)
	return b.String()
}

// FormatDetailsMarkdown renders one runner in full: its configuration, then
// the projects and groups it serves as nested collections.
func FormatDetailsMarkdown(out DetailsOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Runner #%d: Details", out.ID))
	c.Int("ID", out.ID)
	writeRunnerSummary(c, out.Name, out.Description, out.RunnerType, out.Status, out.JobExecutionStatus,
		out.IsShared, out.Online, out.Paused)
	c.Bool("Locked", out.Locked)
	// A runner access level GitLab picks from a fixed set (not_protected,
	// ref_protected), and refuses any other value on register and update.
	c.Field("Access Level", out.AccessLevel)
	c.Bool("Run Untagged", out.RunUntagged)
	// A tag is free text the runner's owner typed.
	c.Field("Tags", strings.Join(out.TagList, ", "))
	if out.MaximumTimeout > 0 {
		c.Field("Max Timeout", fmt.Sprintf("%ds", out.MaximumTimeout))
	}
	c.Text("Maintenance Note", out.MaintenanceNote)
	c.Field("Version", out.Version)
	c.Field("Platform", out.Platform)
	c.Field("Architecture", out.Architecture)
	c.Code("Revision", out.Revision)
	c.Code("IP Address", out.IPAddress)
	c.Time("Last Contact", out.ContactedAt)
	c.Time("Created", out.CreatedAt)
	if out.CreatedBy != nil {
		c.Markdown("Created By", toolutil.MdUserLink(out.CreatedBy.Username, out.CreatedBy.WebURL))
	}
	writeProjects(c, out.Projects)
	writeGroups(c, out.Groups)
	c.End(
		toolutil.HintAction(actionRunnerUpdate, "change this runner's settings, paused state included"),
		toolutil.HintAction(actionRunnerJobs, "list the jobs it has run"),
		toolutil.HintAction(actionRunnerListManagers, "see the machines running it"),
	)
	return b.String()
}

// writeProjects writes the projects a runner is assigned to. The card used to
// say only how many there were, which answered none of the questions a reader
// opens a runner's details to ask.
func writeProjects(c *toolutil.Card, projects []RunnerDetailsProjectOutput) {
	if len(projects) == 0 {
		return
	}
	t := c.Table(fmt.Sprintf("Projects (%d)", len(projects)), "ID", "Name", "Path")
	for _, p := range projects {
		t.Row(
			strconv.FormatInt(p.ID, 10),
			toolutil.EscapeMdTableCell(p.Name),
			toolutil.EscapeMdTableCell(p.PathWithNamespace),
		)
	}
}

// writeGroups writes the groups a runner serves, each linked to its page.
func writeGroups(c *toolutil.Card, groups []RunnerDetailsGroupOutput) {
	if len(groups) == 0 {
		return
	}
	t := c.Table(fmt.Sprintf("Groups (%d)", len(groups)), "ID", "Name")
	for _, g := range groups {
		t.Row(
			strconv.FormatInt(g.ID, 10),
			toolutil.MdTitleLink(g.Name, g.WebURL),
		)
	}
}

// FormatListMarkdown renders a page of runners as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Runners) == 0 {
		return toolutil.EmptyMessage("runners")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Runners", len(out.Runners), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Type", "Status", "Paused", "Shared"))
	for _, r := range out.Runners {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(r.ID, 10),
			toolutil.EscapeMdTableCell(r.Name),
			toolutil.EscapeMdTableCell(r.RunnerType),
			toolutil.EscapeMdTableCell(r.Status),
			toolutil.BoolEmoji(r.Paused),
			toolutil.BoolEmoji(r.IsShared),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		toolutil.HintAction(actionRunnerGet, "see one runner's full configuration"),
		toolutil.HintAction(actionRunnerList, "page through the rest of the runners"),
		toolutil.HintAction(actionRunnerRemove, "unregister a runner"),
	)
	return b.String()
}

// FormatJobListMarkdown renders the jobs a runner has processed as a Markdown
// table.
func FormatJobListMarkdown(out JobListOutput) string {
	if len(out.Jobs) == 0 {
		return toolutil.EmptyMessage("jobs")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Runner Jobs", len(out.Jobs), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Status", "Stage", "Ref", "Duration"))
	for _, j := range out.Jobs {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.FormatInt(j.ID, 10), j.WebURL),
			toolutil.EscapeMdTableCell(j.Name),
			jobStatusCell(j.Status),
			// The stage name is written in .gitlab-ci.yml.
			toolutil.EscapeMdTableCell(j.Stage),
			toolutil.EscapeMdTableCell(j.Ref),
			fmt.Sprintf("%.1fs", j.Duration),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionJobGet, "see one job in full, with its log"),
		toolutil.HintAction(actionRunnerJobs, "page through the rest of this runner's jobs"),
	)
	return b.String()
}

// jobStatusCell renders a job status with the glyph every pipeline-shaped
// status in the tree carries, and nothing when GitLab sent none.
func jobStatusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	// A job status, GitLab's own build state (created, running, success,
	// failed and the rest).
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// FormatAuthTokenMarkdown renders the authentication token a runner reset
// answered with: the secret GitLab shows once, and when it expires.
func FormatAuthTokenMarkdown(out AuthTokenOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Runner Authentication Token")
	c.Secret("Token", out.Token)
	c.Time("Expires At", out.ExpiresAt)
	c.End(
		toolutil.HintAction(actionRunnerVerify, "check the new token authenticates"),
		toolutil.HintAction(actionRunnerGet, "read the runner back"),
	)
	return b.String()
}

// FormatRegTokenMarkdown renders a registration token reset. It is a formatter
// of its own because the two tokens are different secrets used in different
// places: a registration token registers new runners, an authentication token
// is what one runner authenticates with.
func FormatRegTokenMarkdown(out RegTokenOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Runner Registration Token")
	c.Secret("Token", out.Token)
	c.Time("Expires At", out.ExpiresAt)
	c.Note("Every runner registered with the previous token keeps working; the old token registers no new ones.")
	c.End(toolutil.HintAction(actionRunnerRegister, "register a new runner with this token"))
	return b.String()
}

// FormatManagerListMarkdown renders the managers of one runner as a Markdown
// table.
func FormatManagerListMarkdown(out ManagerListOutput) string {
	if len(out.Managers) == 0 {
		return toolutil.EmptyMessage("runner managers")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Runner Managers", len(out.Managers), toolutil.PaginationOutput{})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "System ID", "Version", "Platform", "Arch", "Status", "Job Status", "IP", "Last Contact"))
	for _, m := range out.Managers {
		// The version, platform and architecture come out of the info payload
		// the runner process posts, which GitLab length-checks and nothing
		// else, and this server's own runner.register writes them.
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(m.ID, 10),
			toolutil.EscapeMdTableCell(m.SystemID),
			toolutil.EscapeMdTableCell(m.Version),
			toolutil.EscapeMdTableCell(m.Platform),
			toolutil.EscapeMdTableCell(m.Architecture),
			toolutil.EscapeMdTableCell(m.Status),
			toolutil.EscapeMdTableCell(m.JobExecutionStatus),
			toolutil.EscapeMdTableCell(m.IPAddress),
			toolutil.FormatTime(m.ContactedAt),
		))
	}
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionRunnerGet, "see the runner these managers belong to"),
	)
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
