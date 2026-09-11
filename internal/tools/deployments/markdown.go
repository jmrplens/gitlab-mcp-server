package deployments

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs the hints name. A deployment action is projected
// under the environment domain, so the ID every surface resolves is
// "environment.deployment_get" and not the "deployment.get" the cross-link
// constants in action_specs.go spell.
const (
	hintActionDeploymentGet     = "environment.deployment_get"
	hintActionDeploymentList    = "environment.deployment_list"
	hintActionDeploymentMRs     = "environment.deployment_merge_requests"
	hintActionDeploymentApprove = "environment.deployment_approve_or_reject"
	hintActionEnvironmentGet    = "environment.get"
)

type deploymentNotFoundOutput struct {
	Identifier string
}

func formatDeploymentNotFound(out deploymentNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Deployment", out.Identifier,
		"Use gitlab_deployment_list with project_id to list deployments",
		"Verify the deployment_id is correct for this project",
	)
}

// FormatOutputMarkdown renders one deployment as the card of a single object:
// what was deployed where, then the approvals it is waiting on as a nested
// collection.
func FormatOutputMarkdown(d Output) string {
	if d.ID == 0 {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Deployment #%d", d.ID))
	c.Int("IID", int64(d.IID))
	c.Markdown("Status", statusCell(d.Status))
	c.Field("Ref", d.Ref)
	// A commit SHA, hexadecimal by construction, shown the way GitLab shows it.
	c.Code("SHA", shortSHA(d.SHA))
	if d.Environment != nil {
		c.Field("Environment", d.Environment.Name)
	}
	if d.User != nil {
		c.Markdown("Deployed By", toolutil.MdUserLink(d.User.Username, d.User.WebURL))
	}
	c.Time("Created", d.CreatedAt)
	c.Time("Updated", d.UpdatedAt)
	if d.Deployable != nil && d.Deployable.Pipeline != nil {
		p := d.Deployable.Pipeline
		c.Link("Pipeline", fmt.Sprintf("#%d", p.ID), p.WebURL)
		c.Markdown("Pipeline Status", statusCell(p.Status))
	}
	// A deployment blocked on approvals says nothing about it anywhere else:
	// the card used to render as an ordinary deployment while GitLab was
	// waiting for someone to approve it.
	c.Count("Pending Approvals", d.PendingApprovalCount)
	writeApprovals(c, d.Approvals)
	writeApprovalRules(c, d.ApprovalSummary)
	c.End(deploymentHints(d)...)
	return b.String()
}

// statusCell renders a deployment or pipeline status with the glyph every
// pipeline-shaped status in the tree carries, and nothing when GitLab sent
// none.
func statusCell(status string) string {
	if strings.TrimSpace(status) == "" {
		return ""
	}
	// A deployment status GitLab picks from a fixed set (created, running,
	// success, failed, canceled, blocked).
	return toolutil.PipelineStatusEmoji(status) + " " + toolutil.EscapeMdTableCell(status)
}

// shortSHA renders the first eight characters of a commit SHA, the form GitLab
// itself shows, and leaves a shorter value alone.
func shortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// writeApprovals writes what has been recorded against the deployment so far.
func writeApprovals(c *toolutil.Card, approvals []toolutil.DeploymentApprovalOutput) {
	if len(approvals) == 0 {
		return
	}
	t := c.Table("Approvals", "User", "Status", "When", "Comment")
	for _, a := range approvals {
		user := ""
		if a.User != nil {
			user = toolutil.MdUserLink(a.User.Username, a.User.WebURL)
		}
		t.Row(
			user,
			toolutil.EscapeMdTableCell(a.Status),
			approvalTime(a.CreatedAt),
			toolutil.EscapeMdTableCell(a.Comment),
		)
	}
}

// approvalTime renders when an approval was recorded in the display form every
// other timestamp in a card carries, and nothing when GitLab sent none.
func approvalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return toolutil.FormatTimeValue(*t)
}

// writeApprovalRules writes the rules the deployment is counted against, so a
// reader can tell who is being waited on rather than only how many are left.
func writeApprovalRules(c *toolutil.Card, summary *toolutil.DeploymentApprovalSummaryOutput) {
	if summary == nil || len(summary.Rules) == 0 {
		return
	}
	t := c.Table("Approval Rules", "ID", "Level", "Grantee", "Description", "Required", "Approved")
	for _, r := range summary.Rules {
		t.Row(
			strconv.FormatInt(r.ID, 10),
			levelCell(r),
			granteeCell(r),
			toolutil.EscapeMdTableCell(r.AccessLevelDescription),
			strconv.FormatInt(r.RequiredApprovals, 10),
			strconv.Itoa(approvedCount(r.DeploymentApprovals)),
		)
	}
}

// levelCell renders the role a rule grants, and "-" for a rule that names a
// user or a group instead, whose access_level is not what decides.
func levelCell(r toolutil.DeploymentApprovalRuleOutput) string {
	if granteeCell(r) != "" {
		return "-"
	}
	return toolutil.AccessLevelDescription(gl.AccessLevelValue(r.AccessLevel))
}

// granteeCell names who a granular rule is about, and nothing for a rule that
// grants a role to everyone who holds it.
func granteeCell(r toolutil.DeploymentApprovalRuleOutput) string {
	switch {
	case r.UserID != 0:
		return fmt.Sprintf("user #%d", r.UserID)
	case r.GroupID != 0:
		return fmt.Sprintf("group #%d", r.GroupID)
	default:
		return ""
	}
}

// approvedCount counts the approvals recorded against a rule, rejections
// aside: a rejection is recorded in the same list and is not an approval.
func approvedCount(approvals []toolutil.DeploymentApprovalOutput) int {
	n := 0
	for _, a := range approvals {
		if a.Status == "approved" {
			n++
		}
	}
	return n
}

// deploymentHints names the next steps this deployment allows: a blocked one
// is approved or rejected, and every one of them leads to its environment and
// to the merge requests it shipped.
func deploymentHints(d Output) []string {
	hints := make([]string, 0, 3)
	if d.PendingApprovalCount > 0 {
		hints = append(hints, toolutil.HintAction(hintActionDeploymentApprove, "approve or reject this blocked deployment"))
	}
	hints = append(hints, toolutil.HintAction(hintActionDeploymentMRs, "list the merge requests this deployment shipped"))
	return append(hints, toolutil.HintAction(hintActionEnvironmentGet, "see the environment it deployed to"))
}

// FormatListMarkdown renders a page of a project's deployments as a Markdown
// table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Deployments) == 0 {
		return toolutil.EmptyMessage("deployments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Deployments", len(out.Deployments), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "IID", "Ref", "Status", "Environment", "Deployed By"))
	for _, d := range out.Deployments {
		var envName, user string
		if d.Environment != nil {
			envName = d.Environment.Name
		}
		if d.User != nil {
			user = toolutil.MdUserLink(d.User.Username, d.User.WebURL)
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdTitleLink(strconv.Itoa(d.ID), pipelineURL(d)),
			strconv.Itoa(d.IID),
			toolutil.EscapeMdTableCell(d.Ref),
			statusCell(d.Status),
			toolutil.EscapeMdTableCell(envName),
			user,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(hintActionDeploymentGet, "see one deployment, its approvals and its pipeline"),
		toolutil.HintAction(hintActionDeploymentList, "page through the rest of the project's deployments"),
		toolutil.HintAction(hintActionDeploymentMRs, "list the merge requests a deployment shipped"),
	)
	return b.String()
}

// pipelineURL is the page a deployment row links to: a deployment has no page
// of its own, and the pipeline that ran it is where a reader goes next. Both
// hops are optional, so a deployment with no backing job links nothing.
func pipelineURL(d Output) string {
	if d.Deployable == nil || d.Deployable.Pipeline == nil {
		return ""
	}
	return d.Deployable.Pipeline.WebURL
}

// FormatApproveOrRejectMarkdown renders the approve/reject confirmation, which
// is a one-line result rather than a card: GitLab answers the call with no
// object, and the sentence is the server's own, composed from the deployment
// id and the status the schema validated.
func FormatApproveOrRejectMarkdown(o ApproveOrRejectOutput) string {
	var b strings.Builder
	b.WriteString(toolutil.EmojiSuccess + " " + toolutil.StripControlBytes(o.Message) + "\n")
	toolutil.WriteHints(&b,
		toolutil.HintAction(hintActionDeploymentGet, "read the deployment back with its approvals"),
		toolutil.HintAction(hintActionDeploymentList, "see the other deployments to this environment"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatDeploymentNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatApproveOrRejectMarkdown)
}
