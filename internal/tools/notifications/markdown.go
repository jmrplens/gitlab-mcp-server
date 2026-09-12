package notifications

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FormatMarkdown formats notification settings as a Markdown CallToolResult.
func FormatMarkdown(out Output) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(FormatMarkdownString(out))
}

// FormatMarkdownString renders notification settings as the card of one
// object: the level and the email, then the per-event flags as a section when
// the level is custom and GitLab sends them.
//
// The event flags used to be written as "- ✅ Close Issue", which reads as a
// list of enabled events rather than as fields of the settings object. Each is
// a field, so each is a row whose value is the flag.
func FormatMarkdownString(out Output) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Notification Settings")
	c.Field("Level", out.Level)
	c.Field("Email", out.NotificationEmail)
	if out.Events != nil {
		writeEventRows(c.Section("Custom Events"), *out.Events)
	}
	c.End(
		toolutil.HintAction("user.notification_global_update", "change the account-wide preferences"),
		toolutil.HintAction("user.notification_project_update", "override them for one project"),
		toolutil.HintAction("user.notification_group_update", "override them for one group"),
	)
	return b.String()
}

// writeEventRows writes one row per event flag. Every flag is written whatever
// its value: false is the answer "this event does not notify you", not an
// absence.
func writeEventRows(c *toolutil.Card, events EventOutput) {
	c.Bool("Close Issue", events.CloseIssue)
	c.Bool("Close MR", events.CloseMergeRequest)
	c.Bool("Failed Pipeline", events.FailedPipeline)
	c.Bool("Fixed Pipeline", events.FixedPipeline)
	c.Bool("Issue Due", events.IssueDue)
	c.Bool("Merge MR", events.MergeMergeRequest)
	c.Bool("Merge When Pipeline Succeeds", events.MergeWhenPipelineSucceeds)
	c.Bool("Moved Project", events.MovedProject)
	c.Bool("New Issue", events.NewIssue)
	c.Bool("New MR", events.NewMergeRequest)
	c.Bool("New Epic", events.NewEpic)
	c.Bool("New Note", events.NewNote)
	c.Bool("Push to MR", events.PushToMergeRequest)
	c.Bool("Reassign Issue", events.ReassignIssue)
	c.Bool("Reassign MR", events.ReassignMergeRequest)
	c.Bool("Reopen Issue", events.ReopenIssue)
	c.Bool("Reopen MR", events.ReopenMergeRequest)
	c.Bool("Success Pipeline", events.SuccessPipeline)
}

func init() {
	toolutil.RegisterMarkdown(FormatMarkdownString)
}
