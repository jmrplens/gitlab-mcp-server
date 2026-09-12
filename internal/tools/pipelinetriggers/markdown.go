package pipelinetriggers

import (
	"fmt"
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

// FormatTriggerMarkdown formats a single pipeline trigger as markdown.
//
// The token is written in full only on the result that mints it. A get or an
// update answers with the same Go type and prints the prefix alone.
func FormatTriggerMarkdown(out Output) string {
	var b strings.Builder
	b.WriteString("## Pipeline Trigger\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", out.ID)
	fmt.Fprintf(&b, "| Description | %s |\n", toolutil.EscapeMdTableCell(out.Description))
	if out.Token != "" {
		token := out.Token
		if !out.minted {
			token = maskTriggerToken(out.Token)
		}
		//gitlab:allow-unescaped token: a token GitLab minted, a fixed prefix and URL-safe characters, inside a code span so the reader can copy it back verbatim.
		fmt.Fprintf(&b, "| Token | `%s` |\n", token)
	}
	if out.Owner != nil && out.Owner.Name != "" {
		fmt.Fprintf(&b, "| Owner | %s |\n", toolutil.EscapeMdTableCell(out.Owner.Name))
	}
	if out.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(out.CreatedAt))
	}
	if out.LastUsed != "" {
		fmt.Fprintf(&b, "| Last Used | %s |\n", toolutil.FormatTime(out.LastUsed))
	}
	if out.ExpiresAt != "" {
		fmt.Fprintf(&b, "| Expires At | %s |\n", toolutil.FormatTime(out.ExpiresAt))
	}
	hints := []string{
		"Use the selected tool surface's pipeline-trigger update action with the same project_id and trigger_id to modify this trigger",
		"Use the selected tool surface's pipeline-trigger run action with the same project_id, ref, and this token to execute a pipeline",
		"Use the selected tool surface's pipeline-trigger delete action with the same project_id, trigger_id, and explicit confirm=true to remove this trigger",
	}
	if !out.minted && out.Token != "" {
		hints[1] = "Only the prefix of the token is shown here. Read the full value from this result's structured output, or use the pipeline-trigger create action to mint a new trigger, before calling the pipeline-trigger run action"
	}
	toolutil.WriteHints(&b, hints...)
	return b.String()
}

// FormatListTriggersMarkdown formats a list of pipeline triggers as markdown.
func FormatListTriggersMarkdown(out ListOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Triggers\n\n")
	toolutil.WriteListSummary(&b, len(out.Triggers), out.Pagination)
	if len(out.Triggers) == 0 {
		b.WriteString("No pipeline triggers found.\n")
		return b.String()
	}
	b.WriteString("| ID | Description | Token | Owner | Last Used |\n|---|---|---|---|---|\n")
	for _, t := range out.Triggers {
		owner := ""
		if t.Owner != nil {
			owner = t.Owner.Name
		}
		// A list answers with every trigger a project has, so the old row wrote
		// one live credential per line into the model's context. GitLab sends
		// them; publishing them was ours.
		//gitlab:allow-unescaped maskTriggerToken(t.Token): the prefix of a token GitLab minted, a fixed marker and URL-safe characters.
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			t.ID,
			toolutil.EscapeMdTableCell(t.Description),
			maskTriggerToken(t.Token),
			toolutil.EscapeMdTableCell(owner),
			toolutil.FormatTime(t.LastUsed))
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"The Token column shows each token's prefix only. Read the full value from this result's structured output when a pipeline-trigger run action needs one",
		"Use the selected tool surface's pipeline-trigger get action with the same project_id and trigger_id for full details",
		"Use the selected tool surface's pipeline-trigger create action with project_id to add a new pipeline trigger",
	)
	return b.String()
}

// FormatRunOutputMarkdown formats the result of triggering a pipeline as markdown.
func FormatRunOutputMarkdown(out RunOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Triggered\n\n")
	b.WriteString("| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| Pipeline ID | %d |\n", out.ID)
	//gitlab:allow-unescaped out.SHA: a commit SHA, hexadecimal by construction.
	fmt.Fprintf(&b, "| SHA | %s |\n", out.SHA)
	fmt.Fprintf(&b, "| Ref | %s |\n", toolutil.EscapeMdTableCell(out.Ref))
	//gitlab:allow-unescaped out.Status: a pipeline status, one of GitLab's fixed set (created, running, success, failed and the rest).
	fmt.Fprintf(&b, "| Status | %s |\n", out.Status)
	if out.WebURL != "" {
		fmt.Fprintf(&b, "| URL | %s |\n", toolutil.MdTitleLink(fmt.Sprintf("Pipeline #%d", out.ID), out.WebURL))
	}
	toolutil.WriteHints(
		&b,
		toolutil.HintPreserveLinks,
		"Use the selected tool surface's pipeline get action with the returned id to monitor progress",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatTriggerMarkdown)
	toolutil.RegisterMarkdown(FormatListTriggersMarkdown)
	toolutil.RegisterMarkdown(FormatRunOutputMarkdown)
}
