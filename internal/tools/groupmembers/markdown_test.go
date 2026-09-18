// markdown_test.go contains unit tests for the group-member Markdown
// formatters (billable members and billable memberships).
package groupmembers

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestFormatBillableMembersMarkdown verifies the whole document a page of
// billable members renders: the heading counting the total GitLab reported,
// the table with both flags through the emoji and the activity date through
// the display layout, and the guidance last.
func TestFormatBillableMembersMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got, want := FormatBillableMembersMarkdown(BillableMembersOutput{}), "No billable members found.\n"; got != want {
			t.Errorf("FormatBillableMembersMarkdown =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("one member", func(t *testing.T) {
		got := FormatBillableMembersMarkdown(BillableMembersOutput{
			Members: []BillableMemberOutput{{
				ID: 10, Username: "dev", Name: "Developer", State: "active",
				WebURL: "https://gl/dev", MembershipType: "group_member",
				Removable: true, LastActivityOn: "2026-06-01",
			}},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		})
		want := "## Billable Group Members (1)\n\n" +
			"| Username | Name | State | Membership Type | Locked | Removable | Last Activity |\n" +
			"| --- | --- | --- | --- | --- | --- | --- |\n" +
			"| [@dev](https://gl/dev) | Developer | active | group_member | " +
			toolutil.BoolEmoji(false) + " | " + toolutil.BoolEmoji(true) + " | 1 Jun 2026 |\n" +
			"\n1 items total\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- " + toolutil.HintPreserveLinks + "\n" +
			"- Use action 'group.group_billable_member_memberships_list' to see why a member is billable\n" +
			"- Use action 'group.group_billable_member_remove' to remove a removable billable member\n"
		if got != want {
			t.Errorf("FormatBillableMembersMarkdown =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestFormatBillableMembershipsMarkdown verifies the whole document a page of
// a billable member's memberships renders, the expiry through the display
// layout rather than as GitLab spelled it.
func TestFormatBillableMembershipsMarkdown(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got, want := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{}), "No memberships found.\n"; got != want {
			t.Errorf("FormatBillableMembershipsMarkdown =\n%q\nwant\n%q", got, want)
		}
	})
	t.Run("one membership", func(t *testing.T) {
		got := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{
			Memberships: []BillableMembershipOutput{{
				ID: 99, SourceID: 7, SourceFullName: "Org / Team",
				SourceMembersURL: "https://gl/groups/team/-/group_members",
				ExpiresAt:        "2026-12-31",
				AccessLevel:      &AccessLevelDetailsOutput{IntegerValue: 30, StringValue: "Developer"},
			}},
			Pagination: toolutil.PaginationOutput{TotalItems: 1},
		})
		want := "## Billable Member Memberships (1)\n\n" +
			"| Source | Access Level | Expires |\n| --- | --- | --- |\n" +
			"| [Org / Team](https://gl/groups/team/-/group_members) | Developer (30) | 31 Dec 2026 |\n" +
			"\n1 items total\n" +
			"\n---\n💡 **Next steps:**\n" +
			"- " + toolutil.HintPreserveLinks + "\n" +
			"- Use action 'group.members' to inspect the source group's membership\n"
		if got != want {
			t.Errorf("FormatBillableMembershipsMarkdown =\n%q\nwant\n%q", got, want)
		}
	})
}

// TestFormatBillableMembersMarkdown_ALinkedRowFollowedByALinklessOne_KeepsTheHint
// verifies that the link flag accumulates over the whole page rather than being
// decided by the last row: the first member is linked, the second is not, and
// the instruction to preserve the table's links survives.
//
// Why it matters: the flag is what tells [toolutil.WriteListFooter] whether to
// keep [toolutil.HintPreserveLinks], and a page mixing the two shapes is the
// ordinary case — GitLab sends no web_url for a member the caller may not see.
// Written as `linked = m.WebURL != ""` instead of `linked = linked || …` the
// last row alone would decide, so one linkless member at the bottom of a page
// would strip the instruction from every clickable username above it, and a
// model would drop the links when it relayed the table. The one-row tests
// beside this cannot see that: with a single member the accumulator is only
// ever read as false, which is exactly what condition coverage reported.
func TestFormatBillableMembersMarkdown_ALinkedRowFollowedByALinklessOne_KeepsTheHint(t *testing.T) {
	got := FormatBillableMembersMarkdown(BillableMembersOutput{
		Members: []BillableMemberOutput{
			{ID: 10, Username: "dev", Name: "Developer", State: "active", WebURL: "https://gl/dev"},
			{ID: 11, Username: "ghost", Name: "Ghost", State: "active"},
		},
	})
	want := "## Billable Group Members (2)\n\n" +
		"| Username | Name | State | Membership Type | Locked | Removable | Last Activity |\n" +
		"| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| [@dev](https://gl/dev) | Developer | active |  | " +
		toolutil.BoolEmoji(false) + " | " + toolutil.BoolEmoji(false) + " |  |\n" +
		"| @ghost | Ghost | active |  | " +
		toolutil.BoolEmoji(false) + " | " + toolutil.BoolEmoji(false) + " |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'group.group_billable_member_memberships_list' to see why a member is billable\n" +
		"- Use action 'group.group_billable_member_remove' to remove a removable billable member\n"
	if got != want {
		t.Errorf("FormatBillableMembersMarkdown =\n%q\nwant\n%q", got, want)
	}
}

// TestFormatBillableMembershipsMarkdown_ALinkedRowFollowedByALinklessOne_KeepsTheHint
// is the same property for a billable member's memberships: a source group the
// caller can open followed by one they cannot must still end with the
// instruction to preserve the links.
//
// Why it matters: source_members_url is sent only for a source whose member
// list the caller may read, so a member billed through one visible and one
// invisible group is the common mixed page. A flag reset each iteration would
// hide the surviving link from the model, and no single-row fixture can tell
// the two spellings apart.
func TestFormatBillableMembershipsMarkdown_ALinkedRowFollowedByALinklessOne_KeepsTheHint(t *testing.T) {
	got := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{
		Memberships: []BillableMembershipOutput{
			{
				ID: 99, SourceID: 7, SourceFullName: "Org / Team",
				SourceMembersURL: "https://gl/groups/team/-/group_members",
			},
			{ID: 100, SourceID: 8, SourceFullName: "Org / Hidden"},
		},
	})
	want := "## Billable Member Memberships (2)\n\n" +
		"| Source | Access Level | Expires |\n| --- | --- | --- |\n" +
		"| [Org / Team](https://gl/groups/team/-/group_members) |  |  |\n" +
		"| Org / Hidden |  |  |\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- " + toolutil.HintPreserveLinks + "\n" +
		"- Use action 'group.members' to inspect the source group's membership\n"
	if got != want {
		t.Errorf("FormatBillableMembershipsMarkdown =\n%q\nwant\n%q", got, want)
	}
}
