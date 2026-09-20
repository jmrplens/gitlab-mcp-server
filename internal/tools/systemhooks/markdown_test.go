// markdown_test.go asserts the whole Markdown document each system hook
// formatter writes: the list table with the delivery status GitLab records, the
// hook card with its redacted key tables, and the test result, which reports
// what was sent and never how it was received.
package systemhooks

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// hookCardHints is the guidance section every hook card closes with.
const hookCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.system_hook_test' to send GitLab's sample payload to this hook\n" +
	"- Use action 'admin.system_hook_edit' to change its URL or the events it fires on\n" +
	"- Use action 'admin.system_hook_delete' to remove it\n"

// hookListHints is the guidance section every hook table closes with.
const hookListHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'admin.system_hook_get' to read one hook with its filters and headers\n" +
	"- Use action 'admin.system_hook_add' to register another system hook\n"

// hookTableHeader is the column row of the hook table and its separator.
const hookTableHeader = "| ID | Name | URL | Status | Push | Tag Push | MR | Repo Update | SSL |\n" +
	"| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n"

// hookCardFlags pairs each flag row the card always states, in the order it
// writes them, with the HookItem field that fills it.
var hookCardFlags = []struct {
	label string
	set   func(*HookItem)
}{
	{label: "Push Events", set: func(h *HookItem) { h.PushEvents = true }},
	{label: "Tag Push Events", set: func(h *HookItem) { h.TagPushEvents = true }},
	{label: "MR Events", set: func(h *HookItem) { h.MergeRequestsEvents = true }},
	{label: "Repo Update Events", set: func(h *HookItem) { h.RepositoryUpdateEvents = true }},
	{label: "SSL Verification", set: func(h *HookItem) { h.EnableSSLVerification = true }},
	{label: "Token Present", set: func(h *HookItem) { h.TokenPresent = true }},
	{label: "Signing Token Present", set: func(h *HookItem) { h.SigningTokenPresent = true }},
}

// hookCardFlagRows renders those seven rows with a tick against the one named
// and a cross against the other six.
func hookCardFlagRows(on string) string {
	var sb strings.Builder
	for _, flag := range hookCardFlags {
		mark := "❌"
		if flag.label == on {
			mark = "✅"
		}
		sb.WriteString("- **" + flag.label + "**: " + mark + "\n")
	}
	return sb.String()
}

// hookRowFlags renders the five flag cells of a table row with a tick in the
// column named and a cross in the other four.
func hookRowFlags(on int) string {
	marks := make([]string, len(hookListFlags))
	for i := range marks {
		marks[i] = "❌"
	}
	marks[on] = "✅"
	return strings.Join(marks, " | ")
}

// hookListFlags pairs each flag column of the hook table, in the order it is
// written, with the HookItem field that fills it.
var hookListFlags = []struct {
	name string
	set  func(*HookItem)
}{
	{name: "Push", set: func(h *HookItem) { h.PushEvents = true }},
	{name: "Tag Push", set: func(h *HookItem) { h.TagPushEvents = true }},
	{name: "MR", set: func(h *HookItem) { h.MergeRequestsEvents = true }},
	{name: "Repo Update", set: func(h *HookItem) { h.RepositoryUpdateEvents = true }},
	{name: "SSL", set: func(h *HookItem) { h.EnableSSLVerification = true }},
}

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// hookText unwraps the single text block a registered formatter produces.
func hookText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("expected non-nil markdown result")
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want 1", len(result.Content))
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

// TestFormatListMarkdown verifies the hook table, including the status column
// without which a hook GitLab had switched off after repeated failures looked
// exactly like a healthy one.
func TestFormatListMarkdown(t *testing.T) {
	t.Run("with hooks", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatListMarkdown(ListOutput{
			Hooks: []HookItem{
				{ID: 1, URL: "https://example.com/hook", Name: "My Hook", PushEvents: true, EnableSSLVerification: true},
				{
					ID: 2, URL: "https://example.com/broken", Name: "Broken",
					AlertStatus: "temporarily_disabled", DisabledUntil: "2026-02-03T04:05:06Z",
				},
			},
		})),
			"## System Hooks (2)\n\n"+
				hookTableHeader+
				"| 1 | My Hook | https://example.com/hook | - | ✅ | ❌ | ❌ | ❌ | ✅ |\n"+
				"| 2 | Broken | https://example.com/broken | temporarily_disabled until 3 Feb 2026 04:05 UTC | ❌ | ❌ | ❌ | ❌ | ❌ |\n"+
				hookListHints)
	})
	t.Run("a status GitLab set no expiry on", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatListMarkdown(ListOutput{
			Hooks: []HookItem{{ID: 3, URL: testHookURL, Name: "Watched", AlertStatus: "executable"}},
		})),
			"## System Hooks (1)\n\n"+
				hookTableHeader+
				"| 3 | Watched | "+testHookURL+" | executable | ❌ | ❌ | ❌ | ❌ | ❌ |\n"+
				hookListHints)
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatListMarkdown(ListOutput{})), "No system hooks found.\n")
	})
}

// TestFormatListMarkdown_OneFlagAtATime verifies each event flag renders in its
// own column of the table. Rendered together the ticks are interchangeable: the
// table above sets two flags and leaves three clear, so a column reading the
// wrong field draws the same row.
func TestFormatListMarkdown_OneFlagAtATime(t *testing.T) {
	for column, flag := range hookListFlags {
		t.Run(flag.name, func(t *testing.T) {
			item := HookItem{ID: 1, URL: testHookURL, Name: "Hook"}
			flag.set(&item)
			assertRendered(t, hookText(t, FormatListMarkdown(ListOutput{Hooks: []HookItem{item}})),
				"## System Hooks (1)\n\n"+
					hookTableHeader+
					"| 1 | Hook | "+testHookURL+" | - | "+hookRowFlags(column)+" |\n"+
					hookListHints)
		})
	}
}

// TestFormatHookMarkdown verifies the hook card: the flags every hook states,
// the optional rows dropped when GitLab sent nothing, and the created time in
// the display form.
func TestFormatHookMarkdown(t *testing.T) {
	t.Run("name and description", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatHookMarkdown(HookItem{
			ID: 1, URL: "https://example.com/hook", Name: "My Hook",
			Description: "A test hook", PushEvents: true,
		})),
			"## System Hook #1\n\n"+
				"- **ID**: 1\n"+
				"- **Name**: My Hook\n"+
				"- **Description**: A test hook\n"+
				"- **URL**: https://example.com/hook\n"+
				hookCardFlagRows("Push Events")+
				hookCardHints)
	})
	t.Run("with created at", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatHookMarkdown(HookItem{
			ID: 1, URL: "https://example.com/hook", CreatedAt: "2026-01-01T00:00:00Z",
		})),
			"## System Hook #1\n\n"+
				"- **ID**: 1\n"+
				"- **URL**: https://example.com/hook\n"+
				hookCardFlagRows("")+
				"- **Created At**: 1 Jan 2026 00:00 UTC\n"+
				hookCardHints)
	})
}

// TestFormatHookMarkdown_OneFlagAtATime verifies each of the seven booleans a
// hook card always states renders against its own row. A card that sets several
// of them cannot tell the rows apart, since a row reading the wrong field draws
// the same tick, so each is rendered on its own and the whole card is held to
// carrying exactly one.
func TestFormatHookMarkdown_OneFlagAtATime(t *testing.T) {
	for _, flag := range hookCardFlags {
		t.Run(flag.label, func(t *testing.T) {
			item := HookItem{ID: 1, URL: testHookURL}
			flag.set(&item)
			assertRendered(t, hookText(t, FormatHookMarkdown(item)),
				"## System Hook #1\n\n"+
					"- **ID**: 1\n"+
					"- **URL**: "+testHookURL+"\n"+
					hookCardFlagRows(flag.label)+
					hookCardHints)
		})
	}
}

// TestFormatHookMarkdown_SentFields verifies the hook card names the fields
// read off the captured response, with every secret value redacted, and leaves
// each of them out of a hook that carries none.
func TestFormatHookMarkdown_SentFields(t *testing.T) {
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, hookText(t, FormatHookMarkdown(HookItem{
			ID:                     1,
			URL:                    "https://example.com/hook",
			PushEventsBranchFilter: "release/*",
			BranchFilterStrategy:   "wildcard",
			AlertStatus:            "temporarily_disabled",
			DisabledUntil:          "2026-02-03T04:05:06Z",
			CustomWebhookTemplate:  `{"event":"push"}`,
			URLVariables:           []HookURLVariable{{Key: "token"}},
			CustomHeaders:          []HookCustomHeader{{Key: "X-Env"}},
			OrganizationID:         7,
		})),
			"## System Hook #1\n\n"+
				"- **ID**: 1\n"+
				"- **URL**: https://example.com/hook\n"+
				"- **Alert Status**: temporarily_disabled\n"+
				"- **Disabled Until**: 3 Feb 2026 04:05 UTC\n"+
				"- **Push Events**: ❌\n"+
				"- **Push Events Branch Filter**: release/*\n"+
				"- **Branch Filter Strategy**: wildcard\n"+
				"- **Tag Push Events**: ❌\n"+
				"- **MR Events**: ❌\n"+
				"- **Repo Update Events**: ❌\n"+
				"- **SSL Verification**: ❌\n"+
				`- **Custom Webhook Template**: {"event":"push"}`+"\n"+
				"- **Organization ID**: 7\n"+
				"- **Token Present**: ❌\n"+
				"- **Signing Token Present**: ❌\n\n"+
				"### URL Variables\n\n"+
				"| Key | Value |\n"+
				"| --- | --- |\n"+
				"| token | "+toolutil.RedactedSecretValue+" |\n\n"+
				"### Custom Headers\n\n"+
				"| Key | Value |\n"+
				"| --- | --- |\n"+
				"| X-Env | "+toolutil.RedactedSecretValue+" |\n"+
				hookCardHints)
	})
	t.Run("a hook carrying none shows none", func(t *testing.T) {
		bare := hookText(t, FormatHookMarkdown(HookItem{ID: 1, URL: "https://example.com/hook"}))
		for _, unwanted := range []string{
			"Push Events Branch Filter", "Branch Filter Strategy", "Alert Status",
			"Disabled Until", "Custom Webhook Template", "Custom Headers", "Organization ID",
			"URL Variables",
		} {
			t.Run("omits "+unwanted, func(t *testing.T) {
				if strings.Contains(bare, unwanted) {
					t.Errorf("the card shows %q for a hook that has none:\n%s", unwanted, bare)
				}
			})
		}
	})
}

// TestFormatTestMarkdown verifies the test result says what GitLab sent and
// refuses to imply what the receiver answered, which the response does not
// carry.
func TestFormatTestMarkdown(t *testing.T) {
	assertRendered(t, hookText(t, FormatTestMarkdown(TestOutput{
		Event: HookEventItem{
			EventName: "project_create", Name: "test", Path: "group/test",
			ProjectID: 42, OwnerName: "Alice", OwnerEmail: "alice@example.com",
		},
	})),
		"## Test Delivery Triggered\n\n"+
			"### Sample Payload GitLab Sent\n\n"+
			"- **Event Name**: project_create\n"+
			"- **Name**: test\n"+
			"- **Path**: group/test\n"+
			"- **Project ID**: 42\n"+
			"- **Owner Name**: Alice\n"+
			"- **Owner Email**: alice@example.com\n\n"+
			"GitLab sent its own fixed sample payload to the hook URL. This response carries that payload, not the receiver's status code, so it does not say whether the delivery arrived.\n"+
			"\n---\n💡 **Next steps:**\n"+
			"- Use action 'admin.system_hook_get' to read the hook's alert status, which does record repeated delivery failures\n"+
			"- Use action 'admin.system_hook_list' to see every system hook on the instance\n")
}

// TestSystemHookFormattersAreRegistered verifies every output type resolves
// through the shared Markdown registry, the get, add and edit delegators
// included.
func TestSystemHookFormattersAreRegistered(t *testing.T) {
	hook := HookItem{ID: 7, URL: "https://example.com/hook"}
	card := hookText(t, FormatHookMarkdown(hook))
	list := ListOutput{Hooks: []HookItem{hook}}
	test := TestOutput{Event: HookEventItem{EventName: "project_create"}}
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "ListOutput", output: list, rendered: hookText(t, FormatListMarkdown(list))},
		{name: "HookItem", output: hook, rendered: card},
		{name: "GetOutput", output: GetOutput{Hook: hook}, rendered: card},
		{name: "AddOutput", output: AddOutput{Hook: hook}, rendered: card},
		{name: "EditOutput", output: EditOutput{Hook: hook}, rendered: card},
		{name: "TestOutput", output: test, rendered: hookText(t, FormatTestMarkdown(test))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRendered(t, hookText(t, toolutil.MarkdownForResult(tc.output)), tc.rendered)
		})
	}
}
