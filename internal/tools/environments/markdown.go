package environments

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The canonical catalog IDs the hints name that the specs do not already
// spell. A deployment action is projected under the environment domain, so the
// ID every surface resolves is "environment.deployment_list" and not the
// "deployment.list" the cross-link constant in action_specs.go spells.
const (
	actionEnvironmentCreate  = "environment.create"
	actionEnvironmentDelete  = "environment.delete"
	hintActionDeploymentList = "environment.deployment_list"
)

type environmentNotFoundOutput struct {
	Identifier string
}

func formatEnvironmentNotFound(out environmentNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Environment", out.Identifier,
		"Use gitlab_environment_list with project_id to list environments",
		"Verify the environment_id is correct for this project",
	)
}

// FormatOutputMarkdown renders one environment as the card of a single object:
// its own fields, the project and cluster agent as nested objects, and the
// deployment that put the code there as a section of its own.
func FormatOutputMarkdown(e Output) string {
	if e.Name == "" {
		return ""
	}
	var b strings.Builder
	c := toolutil.NewCard(&b, "Environment: "+e.Name)
	c.Int("ID", e.ID)
	c.Field("Slug", e.Slug)
	// An environment state GitLab picks from a fixed set (available, stopping,
	// stopped).
	c.Field("State", e.State)
	c.Field("Tier", e.Tier)
	c.Field("Description", e.Description)
	// The external URL is the address the deployed application answers on, so
	// it is written as the link it is rather than as text a reader has to copy.
	c.Link("External URL", e.ExternalURL, e.ExternalURL)
	c.Field("Auto-Stop Setting", e.AutoStopSetting)
	c.Code("Kubernetes Namespace", e.KubernetesNamespace)
	c.Code("Flux Resource Path", e.FluxResourcePath)
	c.Time("Created", e.CreatedAt)
	c.Time("Updated", e.UpdatedAt)
	c.Time("Auto-Stop At", e.AutoStopAt)
	if e.Project != nil {
		p := c.Sub("Project")
		p.Int("ID", e.Project.ID)
		p.Field("Path", e.Project.PathWithNamespace)
		p.Link("URL", e.Project.WebURL, e.Project.WebURL)
	}
	if e.ClusterAgent != nil {
		a := c.Sub("Cluster Agent")
		a.Int("ID", e.ClusterAgent.ID)
		a.Field("Name", e.ClusterAgent.Name)
	}
	writeLastDeployment(c, e.LastDeployment)
	c.End(stateHints(e.State)...)
	return b.String()
}

// writeLastDeployment writes what is deployed to the environment right now.
// Without it the card answered "what is this environment?" and never "what is
// running on it?", which is the question an environment is looked up for.
func writeLastDeployment(c *toolutil.Card, d *DeploymentOutput) {
	if d == nil {
		return
	}
	s := c.Section("Last Deployment")
	// A deployment's ids start at one, so a zero is GitLab not having sent the
	// field rather than a deployment numbered nought.
	s.Count("ID", d.ID)
	s.Count("IID", d.IID)
	if d.Status != "" {
		// A deployment status GitLab picks from a fixed set (created, running,
		// success, failed, canceled, blocked), rendered with the glyph every
		// pipeline-shaped status in the tree carries.
		s.Markdown("Status", toolutil.PipelineStatusEmoji(d.Status)+" "+toolutil.EscapeMdTableCell(d.Status))
	}
	s.Field("Ref", d.Ref)
	s.Code("SHA", shortSHA(d.SHA))
	s.Time("Created", d.CreatedAt)
	if d.User != nil {
		s.Markdown("Deployed By", toolutil.MdUserLink(d.User.Username, d.User.WebURL))
	}
	if d.Deployable != nil && d.Deployable.Pipeline != nil {
		s.Link("Pipeline", fmt.Sprintf("#%d", d.Deployable.Pipeline.ID), d.Deployable.Pipeline.WebURL)
	}
}

// shortSHA renders the first eight characters of a commit SHA, the form GitLab
// itself shows, and leaves a shorter value alone.
func shortSHA(sha string) string {
	if len(sha) <= 8 {
		return sha
	}
	return sha[:8]
}

// stateHints picks the next step the environment's own state allows: an
// available environment can be stopped, a stopped one can be deleted, and
// offering "stop" for an environment that is already stopped was advice GitLab
// answers with an error.
func stateHints(state string) []string {
	hints := make([]string, 0, 2)
	switch state {
	case "stopped":
		hints = append(hints, toolutil.HintAction(actionEnvironmentDelete, "delete this stopped environment"))
	case "stopping":
	default:
		hints = append(hints, toolutil.HintAction(actionEnvironmentStop, "stop this environment"))
	}
	return append(hints, toolutil.HintAction(hintActionDeploymentList, "see the deployments to this environment"))
}

// FormatListMarkdown renders a page of a project's environments as a Markdown
// table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Environments) == 0 {
		return toolutil.EmptyMessage("environments")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Environments", len(out.Environments), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "State", "Tier", "External URL"))
	for _, e := range out.Environments {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(e.ID, 10),
			toolutil.EscapeMdTableCell(e.Name),
			toolutil.EscapeMdTableCell(e.State),
			toolutil.EscapeMdTableCell(e.Tier),
			toolutil.MdTitleLink(e.ExternalURL, e.ExternalURL),
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, true,
		toolutil.HintAction(actionEnvironmentGet, "see one environment and what is deployed to it"),
		toolutil.HintAction(actionEnvironmentCreate, "add a new environment"),
		toolutil.HintAction(actionEnvironmentList, "page through the rest of the project's environments"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdownResult(formatEnvironmentNotFound)
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
}
