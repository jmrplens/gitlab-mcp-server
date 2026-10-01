package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// TestDenialText_WordsEveryCause verifies the page says, for each reason no
// fine-grained token reaches an action, what GitLab declares and what it does
// to the request: refused before anything runs, or written and answered null.
func TestDenialText_WordsEveryCause(t *testing.T) {
	cases := []struct {
		denial finegrained.Denial
		want   string
	}{
		{
			denial: finegrained.Denial{Cause: finegrained.CauseMutationUndeclared, Element: "attach"},
			want:   "GitLab declares no fine-grained permission for the mutation `attach`, and refuses it",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseTypeUndeclared, Element: "Namespace"},
			want:   "GitLab declares no fine-grained permission for `Namespace`, which the answer is made of",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CausePayloadUndeclared, Element: "Label"},
			want:   "GitLab declares no fine-grained permission for `Label` in the answer, so the write commits and its answer is lost",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "workItemUpdate", Effect: finegrained.EffectRefused},
			want:   "`workItemUpdate` is declared at a boundary the object the action reaches never resolves to, so GitLab refuses the write before it runs",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "WorkItem", Effect: finegrained.EffectCommittedThenNull},
			want:   "`WorkItem` is declared at a boundary the object the action reaches never resolves to, so the write commits and its answer is lost",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseBoundaryUnresolvable, Element: "WorkItem", Effect: finegrained.EffectNullOrEmpty},
			want:   "`WorkItem` is declared at a boundary the object the action reaches never resolves to, so GitLab answers it as null or leaves it out of the list",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseRESTTodo, Element: "POST /policies"},
			want:   "GitLab marks `POST /policies` as not yet supported for fine-grained tokens",
		},
		{
			denial: finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"},
			want:   "GitLab declares no fine-grained permission for `GET /x`",
		},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.denial.Cause)+" "+string(testCase.denial.Effect), func(t *testing.T) {
			if got := denialText(&testCase.denial); got != testCase.want {
				t.Errorf("denialText = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestNeedsText_WordsEachWayOfRunningTheAction verifies what an action needs
// is worded way by way: one line for one way, "one of" for several, a request
// GitLab leaves to itself named as such, and a way that needs nothing said so
// rather than left blank.
func TestNeedsText_WordsEachWayOfRunningTheAction(t *testing.T) {
	read := finegrained.Need{Permissions: []string{"Project: Read"}, At: []string{"project", "group"}}
	write := finegrained.Need{Permissions: []string{"Issue: Create", "Label: Create"}, At: []string{"project"}}
	cases := []struct {
		name        string
		description finegrained.Description
		want        string
	}{
		{
			name: "denied", description: finegrained.Description{Denied: &finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"}},
			want: "Not reachable at this release: GitLab declares no fine-grained permission for `GET /x`",
		},
		{
			name: "one way", description: finegrained.Description{AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read, write}}}},
			want: "Project: Read at project or group; Issue: Create, Label: Create at project",
		},
		{
			name: "several ways", description: finegrained.Description{AnyOf: []finegrained.Way{{Needs: []finegrained.Need{read}}, {NotJudged: true}}},
			want: "one of: Project: Read at project or group **or** a request GitLab does not judge by the grant",
		},
		{name: "nothing needed", description: finegrained.Description{AnyOf: []finegrained.Way{{}}}, want: "no permission"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := needsText(&testCase.description); got != testCase.want {
				t.Errorf("needsText = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestServedEmptyText_NamesWhatAGrantStillLeavesOut verifies the parts of an
// answer a fine-grained token is served empty are named, each as the GraphQL
// selection that reaches it: always, or unless the grant also holds what
// follows them.
func TestServedEmptyText_NamesWhatAGrantStillLeavesOut(t *testing.T) {
	description := finegrained.Description{
		AlwaysEmpty: []string{"project.branchRules.nodes", "vulnerability"},
		EmptyWithout: []finegrained.Position{{
			Path: "issue.author", Needs: []finegrained.Need{{Permissions: []string{"User: Read"}, At: []string{"user"}}},
		}},
	}
	want := "`project { branchRules { nodes } }` (always), `vulnerability` (always), `issue { author }` (without User: Read at user)"
	if got := servedEmptyText(&description); got != want {
		t.Errorf("servedEmptyText = %q, want %q", got, want)
	}
	if got := servedEmptyText(&finegrained.Description{}); got != "" {
		t.Errorf("servedEmptyText of nothing = %q, want empty", got)
	}
}

// TestRenderReference_OneTablePerDomain verifies the page names the release
// it describes and holds one table per domain, in order, each action on its
// own row.
func TestRenderReference_OneTablePerDomain(t *testing.T) {
	table := &finegrained.Table{
		Version:     "19.4.1-ee",
		Permissions: []string{"read_issue"},
		Display:     []string{"Issue: Read"},
		Groups:      []finegrained.Group{{Perms: []uint16{0}, Any: finegrained.BoundaryProject}},
		Operations:  []finegrained.Operation{{Name: "GET /projects/:id/issues", Groups: []uint32{0}}},
		Actions: []finegrained.Requirement{
			{ID: "issue.get", Paths: [][]uint32{{0}}},
			{ID: "issue.list", Paths: [][]uint32{{0}}},
			{ID: "admin.thing", Denied: &finegrained.Denial{Cause: finegrained.CauseRESTUndeclared, Element: "GET /x"}},
		},
	}
	page := string(renderReference(table))
	if !strings.Contains(page, "as GitLab 19.4.1 declares it") {
		t.Error("the page does not name the release it describes")
	}
	admin, issue := strings.Index(page, "## `admin`"), strings.Index(page, "## `issue`")
	if admin < 0 || issue < admin {
		t.Errorf("the domains are not one table each in order:\n%s", page)
	}
	for _, row := range []string{"| `issue.get`  | Issue: Read at project |", "| `issue.list` | Issue: Read at project |", "Not reachable at this release"} {
		t.Run(row, func(t *testing.T) {
			if !strings.Contains(page, row) {
				t.Errorf("the page holds no %q:\n%s", row, page)
			}
		})
	}
}
