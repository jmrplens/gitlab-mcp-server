package systemhooks

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet    = "admin.system_hook_get"
	actionList   = "admin.system_hook_list"
	actionAdd    = "admin.system_hook_add"
	actionEdit   = "admin.system_hook_edit"
	actionTest   = "admin.system_hook_test"
	actionDelete = "admin.system_hook_delete"
)

// hookStatusCell says whether GitLab is still delivering to the hook. GitLab
// disables a hook that keeps failing, temporarily or permanently, and the list
// used to show only the event flags — so a hook that had been switched off
// looked exactly like a healthy one.
func hookStatusCell(h HookItem) string {
	if h.AlertStatus == "" {
		// An instance older than the field sends nothing; a dash says the
		// response did not say rather than leaving the cell to read as empty.
		return "-"
	}
	status := toolutil.EscapeMdTableCell(h.AlertStatus)
	if h.DisabledUntil == "" {
		return status
	}
	return status + " until " + toolutil.FormatTime(h.DisabledUntil)
}

// FormatListMarkdown renders the instance's system hooks as a Markdown table:
// a collection of objects that share columns.
func FormatListMarkdown(output ListOutput) *mcp.CallToolResult {
	if len(output.Hooks) == 0 {
		return toolutil.ToolResultWithMarkdown(toolutil.EmptyMessage("system hooks"))
	}
	var sb strings.Builder
	toolutil.WriteListHeading(&sb, "System Hooks", len(output.Hooks), toolutil.PaginationOutput{})
	sb.WriteString(toolutil.MarkdownTableHeader(
		"ID", "Name", "URL", "Status", "Push", "Tag Push", "MR", "Repo Update", "SSL",
	))
	for _, h := range output.Hooks {
		sb.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(h.ID, 10),
			toolutil.EscapeMdTableCell(h.Name),
			toolutil.EscapeMdTableCell(h.URL),
			hookStatusCell(h),
			toolutil.BoolEmoji(h.PushEvents),
			toolutil.BoolEmoji(h.TagPushEvents),
			toolutil.BoolEmoji(h.MergeRequestsEvents),
			toolutil.BoolEmoji(h.RepositoryUpdateEvents),
			toolutil.BoolEmoji(h.EnableSSLVerification),
		))
	}
	toolutil.WriteListFooter(&sb, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionGet, "read one hook with its filters and headers"),
		toolutil.HintAction(actionAdd, "register another system hook"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatHookMarkdown renders a single system hook as the card of one object,
// with its URL variables and custom headers as the nested collections they
// are. Both carry keys only: GitLab never sends either value back.
func FormatHookMarkdown(item HookItem) *mcp.CallToolResult {
	var sb strings.Builder
	c := toolutil.NewCard(&sb, fmt.Sprintf("System Hook #%d", item.ID))
	c.Int("ID", item.ID)
	c.Field("Name", item.Name)
	// The description is whatever the administrator who registered the hook
	// typed against it.
	c.Text("Description", item.Description)
	c.Field("URL", item.URL)
	c.Field("Alert Status", item.AlertStatus)
	c.Time("Disabled Until", item.DisabledUntil)
	c.Bool("Push Events", item.PushEvents)
	c.Field("Push Events Branch Filter", item.PushEventsBranchFilter)
	c.Field("Branch Filter Strategy", item.BranchFilterStrategy)
	c.Bool("Tag Push Events", item.TagPushEvents)
	c.Bool("MR Events", item.MergeRequestsEvents)
	c.Bool("Repo Update Events", item.RepositoryUpdateEvents)
	c.Bool("SSL Verification", item.EnableSSLVerification)
	c.Field("Custom Webhook Template", item.CustomWebhookTemplate)
	c.Count("Organization ID", item.OrganizationID)
	c.Bool("Token Present", item.TokenPresent)
	c.Bool("Signing Token Present", item.SigningTokenPresent)
	c.Time("Created At", item.CreatedAt)
	writeRedactedKeys(c, "URL Variables", urlVariableKeys(item.URLVariables))
	writeRedactedKeys(c, "Custom Headers", customHeaderKeys(item.CustomHeaders))
	c.End(
		toolutil.HintAction(actionTest, "send GitLab's sample payload to this hook"),
		toolutil.HintAction(actionEdit, "change its URL or the events it fires on"),
		toolutil.HintAction(actionDelete, "remove it"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// writeRedactedKeys writes one nested collection of configured keys, every
// value redacted.
func writeRedactedKeys(c *toolutil.Card, title string, keys []string) {
	if len(keys) == 0 {
		return
	}
	table := c.Table(title, "Key", "Value")
	for _, key := range keys {
		table.Row(toolutil.EscapeMdTableCell(key), toolutil.RedactedSecretValue)
	}
}

func urlVariableKeys(variables []HookURLVariable) []string {
	keys := make([]string, 0, len(variables))
	for _, variable := range variables {
		keys = append(keys, variable.Key)
	}
	return keys
}

func customHeaderKeys(headers []HookCustomHeader) []string {
	keys := make([]string, 0, len(headers))
	for _, header := range headers {
		keys = append(keys, header.Key)
	}
	return keys
}

// FormatTestMarkdown renders the result of asking GitLab to deliver a test
// event to a system hook.
//
// It reports what was sent and never how it was received. GitLab's test
// endpoint fires its own fixed sample payload — a project_create event about a
// stand-in project — and answers with that payload, not with the receiver's
// response, so the previous "Hook Test Event" heading over a table of the
// sample's fields read as a delivery outcome the response does not carry.
func FormatTestMarkdown(output TestOutput) *mcp.CallToolResult {
	e := output.Event
	var sb strings.Builder
	c := toolutil.NewCard(&sb, "Test Delivery Triggered")
	section := c.Section("Sample Payload GitLab Sent")
	// A system hook event carries the name and path of whatever it fired about,
	// both of which a person chose.
	section.Field("Event Name", e.EventName)
	section.Field("Name", e.Name)
	section.Field("Path", e.Path)
	section.Count("Project ID", e.ProjectID)
	section.Field("Owner Name", e.OwnerName)
	section.Field("Owner Email", e.OwnerEmail)
	c.Note("GitLab sent its own fixed sample payload to the hook URL. This response carries that payload, not the receiver's status code, so it does not say whether the delivery arrived.")
	c.End(
		toolutil.HintAction(actionGet, "read the hook's alert status, which does record repeated delivery failures"),
		toolutil.HintAction(actionList, "see every system hook on the instance"),
	)
	return toolutil.ToolResultWithMarkdown(sb.String())
}

// FormatGetMarkdown formats a single system_hook_get response.
func FormatGetMarkdown(output GetOutput) *mcp.CallToolResult {
	return FormatHookMarkdown(output.Hook)
}

// FormatAddMarkdown formats a single system_hook_add response.
func FormatAddMarkdown(output AddOutput) *mcp.CallToolResult {
	return FormatHookMarkdown(output.Hook)
}

// FormatEditMarkdown formats a single system_hook_edit response.
func FormatEditMarkdown(output EditOutput) *mcp.CallToolResult {
	return FormatHookMarkdown(output.Hook)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatListMarkdown)
	toolutil.RegisterMarkdownResult(FormatHookMarkdown)
	toolutil.RegisterMarkdownResult(FormatTestMarkdown)
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatAddMarkdown)
	toolutil.RegisterMarkdownResult(FormatEditMarkdown)
}
