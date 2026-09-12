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
