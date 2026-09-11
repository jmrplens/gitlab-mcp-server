package pipelinetriggers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// triggerTokenPrefix is the marker GitLab puts in front of every pipeline
// trigger token it mints.
const triggerTokenPrefix = "glptt-"

// shownTokenChars is how much of the secret part of a trigger token the get
// and list cards reveal: enough to tell two tokens apart at a glance, and far
// too little to run a pipeline with.
const shownTokenChars = 4

// prefixOnlyHint is what a card whose token is masked tells the reader instead
// of pointing them at a value it did not print.
const prefixOnlyHint = "Only the prefix of the token is shown here. Read the full value from this result's structured output, or use the pipeline-trigger create action to mint a new trigger, before calling the pipeline-trigger run action"

// maskTriggerToken renders the prefix of a pipeline trigger token: GitLab's
// own "glptt-" marker when the token carries one, plus four characters of the
// secret after it, and an ellipsis for the rest.
//
// GitLab returns the whole token from the get and the list endpoints as well
// as from the create one, so printing it in full was this server's choice, and
// on a list it repeated a live credential once per row into text a model
// reads, keeps in its context and may quote back. A token too short to have a
// prefix and four characters is withheld entirely rather than revealed by the
// arithmetic.
func maskTriggerToken(token string) string {
	if token == "" {
		return ""
	}
	shown := shownTokenChars
	if strings.HasPrefix(token, triggerTokenPrefix) {
		shown += len(triggerTokenPrefix)
	}
	if len(token) <= shown {
		return toolutil.RedactedPlaceholder
	}
	return token[:shown] + "..."
}

// FormatTriggerMarkdown renders one pipeline trigger as a card.
//
// The token is written in full only on the result that mints it. A get or an
// update answers with the same Go type and prints the prefix alone.
func FormatTriggerMarkdown(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, fmt.Sprintf("Pipeline Trigger #%d", out.ID))
	c.Int("ID", out.ID)
	c.Field("Description", out.Description)
	masked := out.Token != "" && !out.minted
	if masked {
		c.Code("Token", maskTriggerToken(out.Token))
	} else {
		c.Secret("Token", out.Token)
	}
	if out.Owner != nil {
		c.Field("Owner", out.Owner.Name)
	}
	c.Time("Created", out.CreatedAt)
	c.Time("Last Used", out.LastUsed)
	c.Time("Expires At", out.ExpiresAt)

	runHint := "Use the selected tool surface's pipeline-trigger run action with the same project_id, ref, and this token to execute a pipeline"
	if masked {
		runHint = prefixOnlyHint
	}
	c.End(
		"Use the selected tool surface's pipeline-trigger update action with the same project_id and trigger_id to modify this trigger",
		runHint,
		"Use the selected tool surface's pipeline-trigger delete action with the same project_id, trigger_id, and explicit confirm=true to remove this trigger",
	)
	return b.String()
}

// FormatListTriggersMarkdown renders a list of pipeline triggers as a table.
func FormatListTriggersMarkdown(out ListOutput) string {
	if len(out.Triggers) == 0 {
		return toolutil.EmptyMessage("pipeline triggers")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Pipeline Triggers", len(out.Triggers), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Description", "Token", "Owner", "Last Used", "Expires"))
	for _, t := range out.Triggers {
		owner := ""
		if t.Owner != nil {
			owner = t.Owner.Name
		}
		expires := "never"
		if t.ExpiresAt != "" {
			expires = toolutil.FormatTime(t.ExpiresAt)
		}
		// A list answers with every trigger a project has, so the old row wrote
		// one live credential per line into the model's context. GitLab sends
		// them; publishing them was ours.
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(t.ID, 10),
			toolutil.EscapeMdTableCell(t.Description),
			toolutil.MdCodeSpanCell(maskTriggerToken(t.Token)),
			toolutil.EscapeMdTableCell(owner),
			toolutil.FormatTime(t.LastUsed),
			expires,
		))
	}
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"The Token column shows each token's prefix only. Read the full value from this result's structured output when a pipeline-trigger run action needs one",
		"Use the selected tool surface's pipeline-trigger get action with the same project_id and trigger_id for full details",
		"Use the selected tool surface's pipeline-trigger create action with project_id to add a new pipeline trigger",
	)
	return b.String()
}

// FormatRunOutputMarkdown renders the pipeline a trigger started as a card.
func FormatRunOutputMarkdown(out RunOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Pipeline Triggered")
	c.Int("Pipeline ID", out.ID)
	c.Code("SHA", out.SHA)
	c.Field("Ref", out.Ref)
	if out.Status != "" {
		c.Field("Status", toolutil.PipelineStatusEmoji(out.Status)+" "+out.Status)
	}
	c.URL(out.WebURL)
	c.End("Use the selected tool surface's pipeline get action with the returned id to monitor progress")
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTriggerMarkdown)
	toolutil.RegisterMarkdown(FormatListTriggersMarkdown)
	toolutil.RegisterMarkdown(FormatRunOutputMarkdown)
}
