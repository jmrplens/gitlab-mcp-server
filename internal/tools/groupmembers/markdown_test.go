// markdown_test.go contains unit tests for the group-member Markdown
// formatters (billable members and billable memberships).
package groupmembers

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestFormatBillableMembersMarkdown covers the populated and empty paths.
func TestFormatBillableMembersMarkdown(t *testing.T) {
	empty := FormatBillableMembersMarkdown(BillableMembersOutput{})
	if !strings.Contains(empty, "No billable members found.") {
		t.Errorf("empty markdown = %q", empty)
	}
	md := FormatBillableMembersMarkdown(BillableMembersOutput{
		Members: []BillableMemberOutput{{
			ID: 10, Username: "dev", Name: "Developer", State: "active",
			WebURL: "https://gl/dev", MembershipType: "group_member",
			Removable: true, LastActivityOn: "2026-06-01",
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{"Billable Group Members", "[dev](https://gl/dev)", "group_member"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}

// TestFormatBillableMembershipsMarkdown covers the populated and empty paths.
func TestFormatBillableMembershipsMarkdown(t *testing.T) {
	empty := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{})
	if !strings.Contains(empty, "No memberships found") {
		t.Errorf("empty markdown = %q", empty)
	}
	md := FormatBillableMembershipsMarkdown(BillableMembershipsOutput{
		Memberships: []BillableMembershipOutput{{
			ID: 99, SourceID: 7, SourceFullName: "Org / Team",
			SourceMembersURL: "https://gl/groups/team/-/group_members",
			ExpiresAt:        "2026-12-31",
			AccessLevel:      &AccessLevelDetailsOutput{IntegerValue: 30, StringValue: "Developer"},
		}},
		Pagination: toolutil.PaginationOutput{TotalItems: 1},
	})
	for _, want := range []string{
		"Billable Member Memberships",
		"[Org / Team](https://gl/groups/team/-/group_members)",
		"Developer",
		"30",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(md, want) {
				t.Errorf("markdown missing %q:\n%s", want, md)
			}
		})
	}
}
