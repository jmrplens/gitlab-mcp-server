// markdown_test.go tests the Markdown the project and the group protected
// environment formatters share: the rules a card carries and the table a page
// of environments is listed in.
package protectedenvdata

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// rulesCard renders the rules of one environment the way both formatters do,
// under a heading and with no hints, so a test sees exactly what
// [WriteRules] writes.
func rulesCard(requiredApprovalCount int64, deployAccessLevels []AccessLevel, approvalRules []ApprovalRule) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Protected Environment: production")
	WriteRules(c, requiredApprovalCount, deployAccessLevels, approvalRules)
	return b.String()
}

// The heading rulesCard opens every card with, and the two table headers
// [WriteRules] writes.
const (
	cardHeading         = "## Protected Environment: production\n\n"
	deployTableHeader   = "\n### Deploy Access Levels\n\n| ID | Level | Grantee | Description | Inheritance |\n| --- | --- | --- | --- | --- |\n"
	approvalTableHeader = "\n### Approval Rules\n\n| ID | Level | Grantee | Description | Required | Inheritance |\n| --- | --- | --- | --- | --- | --- |\n"
)

// TestWriteRules_RendersEachKindOfRule pins the rules of a card, case by
// case: the headline defers to the per-rule counts whenever an approval rule
// is present and is the count GitLab sent otherwise, zero included; each table
// appears only when GitLab sent an entry for it; and every row says which role
// it grants, to whom, and whether a group rule reaches inherited members.
// GitLab's zero means "direct" for a group rule and "not applicable" for a
// rule that is not about a group, and a description is GitLab's text, so a
// pipe in it is escaped rather than splitting the row.
func TestWriteRules_RendersEachKindOfRule(t *testing.T) {
	cases := []struct {
		name     string
		required int64
		deploy   []AccessLevel
		rules    []ApprovalRule
		want     string
	}{
		{
			name:     "no rule at all",
			required: 0,
			want:     cardHeading + "- **Required Approvals**: 0\n",
		},
		{
			name:     "deploy access levels of every kind",
			required: 1,
			deploy: []AccessLevel{
				{ID: 1, AccessLevel: 40, AccessLevelDescription: "Maintainers"},
				{ID: 2, AccessLevel: 30, AccessLevelDescription: "Sam Bauch", UserID: 123},
				{ID: 3, AccessLevelDescription: "Platform | Ops", GroupID: 77},
				{ID: 4, AccessLevelDescription: "Release managers", GroupID: 55, GroupInheritanceType: 1},
			},
			want: cardHeading + "- **Required Approvals**: 1\n" + deployTableHeader +
				"| 1 | Maintainer |  | Maintainers |  |\n" +
				"| 2 | - | user #123 | Sam Bauch |  |\n" +
				"| 3 | - | group #77 | Platform &#124; Ops | direct |\n" +
				"| 4 | - | group #55 | Release managers | inherited |\n",
		},
		{
			name:     "approval rules of every kind",
			required: 2,
			rules: []ApprovalRule{
				{ID: 10, AccessLevel: 30, AccessLevelDescription: "Developers", RequiredApprovalCount: 1},
				{ID: 11, AccessLevelDescription: "Sam Bauch", UserID: 123, RequiredApprovalCount: 2},
				{ID: 12, AccessLevelDescription: "Platform | Ops", GroupID: 77, RequiredApprovalCount: 3},
				{ID: 13, AccessLevelDescription: "Release managers", GroupID: 55, GroupInheritanceType: 1, RequiredApprovalCount: 5},
			},
			want: cardHeading + "- **Required Approvals**: per approval rule (see below)\n" + approvalTableHeader +
				"| 10 | Developer |  | Developers | 1 |  |\n" +
				"| 11 | - | user #123 | Sam Bauch | 2 |  |\n" +
				"| 12 | - | group #77 | Platform &#124; Ops | 3 | direct |\n" +
				"| 13 | - | group #55 | Release managers | 5 | inherited |\n",
		},
		{
			name:     "both tables",
			required: 2,
			deploy:   []AccessLevel{{ID: 1, AccessLevelDescription: "Sam Bauch", UserID: 123}},
			rules:    []ApprovalRule{{ID: 10, AccessLevel: 40, AccessLevelDescription: "Maintainers", RequiredApprovalCount: 1}},
			want: cardHeading + "- **Required Approvals**: per approval rule (see below)\n" +
				deployTableHeader + "| 1 | - | user #123 | Sam Bauch |  |\n" +
				approvalTableHeader + "| 10 | Maintainer |  | Maintainers | 1 |  |\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rulesCard(tc.required, tc.deploy, tc.rules); got != tc.want {
				t.Errorf("WriteRules() wrote:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// TestListHeader_NamesTheFourColumns pins the header of the table a page of
// protected environments is listed in.
func TestListHeader_NamesTheFourColumns(t *testing.T) {
	want := "| Name | Required Approvals | Deploy Access Levels | Approval Rules |\n| --- | --- | --- | --- |\n"
	if got := ListHeader(); got != want {
		t.Errorf("ListHeader() = %q, want %q", got, want)
	}
}

// TestListRow_CountsTheRulesOfOneEnvironment pins one environment's row: the
// name escaped as GitLab's text, the count GitLab sent, and how many deploy
// access levels and approval rules the environment carries.
func TestListRow_CountsTheRulesOfOneEnvironment(t *testing.T) {
	cases := []struct {
		name     string
		envName  string
		required int64
		deploy   []AccessLevel
		rules    []ApprovalRule
		want     string
	}{
		{name: "no rule", envName: "staging", want: "| staging | 0 | 0 | 0 |\n"},
		{
			name:     "two deploy access levels and three approval rules",
			envName:  "production",
			required: 2,
			deploy:   []AccessLevel{{ID: 1}, {ID: 2}},
			rules:    []ApprovalRule{{ID: 5}, {ID: 6}, {ID: 7}},
			want:     "| production | 2 | 2 | 3 |\n",
		},
		{name: "a name carrying a pipe", envName: "review|*", required: 1, deploy: []AccessLevel{{ID: 1}}, want: "| review&#124;* | 1 | 1 | 0 |\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ListRow(tc.envName, tc.required, tc.deploy, tc.rules); got != tc.want {
				t.Errorf("ListRow() = %q, want %q", got, tc.want)
			}
		})
	}
}
