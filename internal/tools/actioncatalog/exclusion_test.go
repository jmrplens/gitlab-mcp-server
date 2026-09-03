package actioncatalog

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// exclusionTestCatalog builds a two-group catalog whose actions carry both an
// individual tool name and a canonical action ID, which is the shape every
// production catalog has and the shape the exclusion matcher must handle.
//
// gitlab_issue holds list (read) and delete (write); gitlab_project holds get.
func exclusionTestCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog := NewCatalog()
	issues := NewGroup(GroupOptions{ToolName: "gitlab_issue", BaseDomain: "issue"})
	issues.SetAction(Action{
		Name:           "list",
		Route:          testRoute(false),
		ReadOnly:       true,
		Aliases:        []string{"issues_list"},
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_issue_list"},
	})
	issues.SetAction(Action{
		Name:           "delete",
		Route:          testRoute(false),
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_issue_delete"},
	})
	projects := NewGroup(GroupOptions{ToolName: "gitlab_project", BaseDomain: "project"})
	projects.SetAction(Action{
		Name:           "get",
		Route:          testRoute(false),
		ReadOnly:       true,
		IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_project_get"},
	})
	if err := catalog.AddGroup(issues); err != nil {
		t.Fatalf("AddGroup(gitlab_issue) error = %v", err)
	}
	if err := catalog.AddGroup(projects); err != nil {
		t.Fatalf("AddGroup(gitlab_project) error = %v", err)
	}
	return catalog
}

// catalogActionIDs returns every canonical action ID in the catalog, sorted,
// so a subtest can compare the survivors as one value.
func catalogActionIDs(catalog *Catalog) []string {
	if catalog == nil {
		return nil
	}
	ids := make([]string, 0, catalog.CountActions())
	for _, action := range catalog.Actions() {
		ids = append(ids, string(action.ID))
	}
	slices.Sort(ids)
	return ids
}

// TestFilterExcludedToolNames_MatchesEveryNameASurfaceUses verifies that an
// --exclude-tools entry removes the action it names whichever of the three
// names the operator wrote: the meta-tool group name, the individual tool
// name, or the canonical action ID.
//
// Before this, only the group name matched. On the default dynamic surface the
// two visible tools reach every action by canonical ID, so excluding
// gitlab_issue_delete removed nothing and reported nothing: the operator saw a
// clean startup and a server that still executed the action. Each subtest
// asserts the surviving action IDs exactly, so an entry that removes too much
// fails as loudly as one that removes too little.
func TestFilterExcludedToolNames_MatchesEveryNameASurfaceUses(t *testing.T) {
	tests := []struct {
		name          string
		exclude       []string
		wantActions   []string
		wantGroups    int
		wantUnmatched []string
	}{
		{
			name:        "group tool name removes the whole group",
			exclude:     []string{"gitlab_issue"},
			wantActions: []string{"project.get"},
			wantGroups:  1,
		},
		{
			name:        "individual tool name removes only that action",
			exclude:     []string{"gitlab_issue_delete"},
			wantActions: []string{"issue.list", "project.get"},
			wantGroups:  2,
		},
		{
			name:        "canonical action ID removes only that action",
			exclude:     []string{"issue.delete"},
			wantActions: []string{"issue.list", "project.get"},
			wantGroups:  2,
		},
		{
			name:        "excluding every action drops the empty group",
			exclude:     []string{"gitlab_issue_list", "issue.delete"},
			wantActions: []string{"project.get"},
			wantGroups:  1,
		},
		{
			name:        "mixed group and action entries compose",
			exclude:     []string{"gitlab_project", "issue.delete"},
			wantActions: []string{"issue.list"},
			wantGroups:  1,
		},
		{
			name:          "an alias is not a name any surface accepts",
			exclude:       []string{"issues_list"},
			wantActions:   []string{"issue.delete", "issue.list", "project.get"},
			wantGroups:    2,
			wantUnmatched: []string{"issues_list"},
		},
		{
			name:          "an entry matching nothing is reported",
			exclude:       []string{"gitlab_not_a_tool", "issue.delete"},
			wantActions:   []string{"issue.list", "project.get"},
			wantGroups:    2,
			wantUnmatched: []string{"gitlab_not_a_tool"},
		},
		{
			name:        "blank entries are ignored",
			exclude:     []string{"", "   "},
			wantActions: []string{"issue.delete", "issue.list", "project.get"},
			wantGroups:  2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := exclusionTestCatalog(t)
			filtered, unmatched := catalog.FilterExcludedToolNames(tt.exclude)
			if got := catalogActionIDs(filtered); !slices.Equal(got, tt.wantActions) {
				t.Errorf("FilterExcludedToolNames(%v) actions = %v, want %v", tt.exclude, got, tt.wantActions)
			}
			if got := filtered.CountGroups(); got != tt.wantGroups {
				t.Errorf("FilterExcludedToolNames(%v) CountGroups() = %d, want %d", tt.exclude, got, tt.wantGroups)
			}
			if !slices.Equal(unmatched, tt.wantUnmatched) {
				t.Errorf("FilterExcludedToolNames(%v) unmatched = %v, want %v", tt.exclude, unmatched, tt.wantUnmatched)
			}
			if got := catalog.CountActions(); got != 3 {
				t.Errorf("source catalog CountActions() = %d, want 3 (filter must not mutate its input)", got)
			}
		})
	}
}

// TestFilterExcludedToolNames_KeepsGroupMetadataAndSchemas verifies that a
// group which loses one action keeps every other property intact: the
// remaining action is still executable, still indexed by its canonical ID, and
// the group still carries its own metadata.
//
// Removing an action rebuilds the group, so this pins the rebuild against
// silently dropping the group description, base domain or read-only flag,
// which downstream surfaces read to derive tool annotations.
func TestFilterExcludedToolNames_KeepsGroupMetadataAndSchemas(t *testing.T) {
	catalog := NewCatalog()
	group := NewGroup(GroupOptions{
		ToolName:               "gitlab_issue",
		Title:                  "Issues",
		Description:            "Issue operations.",
		BaseDomain:             "issue",
		ReadOnly:               false,
		OwnerPackage:           "issues",
		CapabilityRequirements: []string{"elicitation"},
	})
	group.SetAction(Action{Name: "list", Route: testRoute(false), ReadOnly: true, IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_issue_list"}})
	group.SetAction(Action{Name: "delete", Route: testRoute(false), IndividualTool: toolutil.IndividualToolSpec{Name: "gitlab_issue_delete"}})
	if err := catalog.AddGroup(group); err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	filtered, unmatched := catalog.FilterExcludedToolNames([]string{"gitlab_issue_delete"})
	if len(unmatched) != 0 {
		t.Fatalf("unmatched = %v, want none", unmatched)
	}

	kept, ok := filtered.Group("gitlab_issue")
	if !ok {
		t.Fatal("Group(gitlab_issue) missing after excluding one of its actions")
	}

	t.Run("group metadata survives", func(t *testing.T) {
		if kept.Title != "Issues" || kept.Description != "Issue operations." {
			t.Errorf("group title/description = %q/%q, want %q/%q", kept.Title, kept.Description, "Issues", "Issue operations.")
		}
		if kept.BaseDomain != "issue" || kept.OwnerPackage != "issues" {
			t.Errorf("group base domain/owner = %q/%q, want %q/%q", kept.BaseDomain, kept.OwnerPackage, "issue", "issues")
		}
		if !slices.Equal(kept.CapabilityRequirements, []string{"elicitation"}) {
			t.Errorf("group capability requirements = %v, want [elicitation]", kept.CapabilityRequirements)
		}
	})

	t.Run("surviving action stays executable", func(t *testing.T) {
		action, found := kept.Actions["list"]
		if !found {
			t.Fatalf("group actions = %v, want list to survive", kept.ActionOrder)
		}
		if action.Route.Handler == nil {
			t.Error("surviving action lost its handler")
		}
	})

	t.Run("removed action leaves the action index", func(t *testing.T) {
		if _, found := filtered.Action("issue.delete"); found {
			t.Error("Action(issue.delete) still resolves after exclusion")
		}
		if _, found := filtered.Action("issue.list"); !found {
			t.Error("Action(issue.list) no longer resolves after excluding a sibling")
		}
	})
}

// TestFilterExcludedTools_DelegatesToNameMatching verifies that the existing
// FilterExcludedTools entry point, which the server calls, now applies the same
// action-level matching, and that its documented edge cases still hold.
func TestFilterExcludedTools_DelegatesToNameMatching(t *testing.T) {
	tests := []struct {
		name        string
		catalog     func(t *testing.T) *Catalog
		exclude     []string
		wantNil     bool
		wantActions []string
	}{
		{
			name:        "individual tool name is honored",
			catalog:     exclusionTestCatalog,
			exclude:     []string{"gitlab_project_get"},
			wantActions: []string{"issue.delete", "issue.list"},
		},
		{
			name:        "empty exclusion list clones the catalog",
			catalog:     exclusionTestCatalog,
			exclude:     nil,
			wantActions: []string{"issue.delete", "issue.list", "project.get"},
		},
		{
			name:    "nil catalog stays nil",
			catalog: func(*testing.T) *Catalog { return nil },
			exclude: []string{"gitlab_issue"},
			wantNil: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := tt.catalog(t).FilterExcludedTools(tt.exclude)
			if tt.wantNil {
				if filtered != nil {
					t.Fatalf("FilterExcludedTools() = %v, want nil", filtered)
				}
				return
			}
			if got := catalogActionIDs(filtered); !slices.Equal(got, tt.wantActions) {
				t.Errorf("FilterExcludedTools(%v) actions = %v, want %v", tt.exclude, got, tt.wantActions)
			}
		})
	}
}

// TestFilterExcludedToolNames_EntriesNamingAnAlreadyRemovedAction_AreNotReported
// verifies that an entry naming an action some other entry already removed is
// not reported as naming nothing.
//
// Two entries reach the same action routinely: a configuration merged from two
// sources carries both spellings of it, and a file that excludes a whole group
// often also lists the member action that motivated the exclusion. Reporting
// the second one as unmatched accuses a correct configuration, and the warning
// is the operator's only signal that an entry really is wrong, so a false
// positive teaches them to stop reading it.
func TestFilterExcludedToolNames_EntriesNamingAnAlreadyRemovedAction_AreNotReported(t *testing.T) {
	tests := []struct {
		name        string
		exclude     []string
		wantActions []string
		wantGroups  int
	}{
		{
			name:        "individual tool name then canonical ID",
			exclude:     []string{"gitlab_issue_delete", "issue.delete"},
			wantActions: []string{"issue.list", "project.get"},
			wantGroups:  2,
		},
		{
			name:        "canonical ID then individual tool name",
			exclude:     []string{"issue.delete", "gitlab_issue_delete"},
			wantActions: []string{"issue.list", "project.get"},
			wantGroups:  2,
		},
		{
			name:        "group name and a member individual tool name",
			exclude:     []string{"gitlab_issue", "gitlab_issue_delete"},
			wantActions: []string{"project.get"},
			wantGroups:  1,
		},
		{
			name:        "group name and a member canonical ID",
			exclude:     []string{"gitlab_issue", "issue.delete"},
			wantActions: []string{"project.get"},
			wantGroups:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := exclusionTestCatalog(t)
			filtered, unmatched := catalog.FilterExcludedToolNames(tt.exclude)
			if len(unmatched) != 0 {
				t.Errorf("FilterExcludedToolNames(%v) unmatched = %v, want none", tt.exclude, unmatched)
			}
			if got := catalogActionIDs(filtered); !slices.Equal(got, tt.wantActions) {
				t.Errorf("FilterExcludedToolNames(%v) actions = %v, want %v", tt.exclude, got, tt.wantActions)
			}
			if got := filtered.CountGroups(); got != tt.wantGroups {
				t.Errorf("FilterExcludedToolNames(%v) CountGroups() = %d, want %d", tt.exclude, got, tt.wantGroups)
			}
		})
	}
}

// TestUnmatchedExcludedPatterns_ReportsInputOrderWithoutDuplicates verifies the
// reporting helper feeding the startup warning: an operator reading it must see
// the entries in the order they wrote them, once each, with blanks dropped.
func TestUnmatchedExcludedPatterns_ReportsInputOrderWithoutDuplicates(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		matched []string
		want    []string
	}{
		{name: "input order preserved", input: []string{"b_tool", "a_tool"}, want: []string{"b_tool", "a_tool"}},
		{name: "duplicates collapse", input: []string{"a_tool", "a_tool"}, want: []string{"a_tool"}},
		{name: "matched entries are omitted", input: []string{"a_tool", "b_tool"}, matched: []string{"a_tool"}, want: []string{"b_tool"}},
		{name: "blanks are dropped", input: []string{" ", ""}, want: nil},
		{name: "surrounding space is trimmed", input: []string{"  a_tool  "}, want: []string{"a_tool"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := make(map[string]struct{}, len(tt.matched))
			for _, entry := range tt.matched {
				matched[entry] = struct{}{}
			}
			got := unmatchedExcludedPatterns(tt.input, matched)
			if !slices.Equal(got, tt.want) {
				t.Errorf("unmatchedExcludedPatterns(%v, %v) = %v, want %v", tt.input, tt.matched, got, tt.want)
			}
			for _, entry := range got {
				if strings.TrimSpace(entry) != entry {
					t.Errorf("unmatchedExcludedPatterns() returned untrimmed entry %q", entry)
				}
			}
		})
	}
}

// TestExcludedActionIDs_ResolvesEverySpellingToCanonicalIDs verifies the
// resolution the other request paths depend on.
//
// A resource template returns the same GitLab object as a tool and knows
// nothing about tool names, so it can only apply the operator's exclusion if
// something resolves "gitlab_issue", "gitlab_issue_delete" and "issue.delete"
// to the same canonical ID first. This is that something, and the sorted result
// is what makes it comparable in a caller's test.
func TestExcludedActionIDs_ResolvesEverySpellingToCanonicalIDs(t *testing.T) {
	tests := []struct {
		name    string
		exclude []string
		want    []string
	}{
		{name: "nothing excluded", exclude: nil, want: nil},
		{name: "group name resolves to all its actions", exclude: []string{"gitlab_issue"}, want: []string{"issue.delete", "issue.list"}},
		{name: "individual tool name resolves to one action", exclude: []string{"gitlab_issue_delete"}, want: []string{"issue.delete"}},
		{name: "canonical ID resolves to itself", exclude: []string{"issue.delete"}, want: []string{"issue.delete"}},
		{name: "unknown entries resolve to nothing", exclude: []string{"gitlab_not_a_tool"}, want: nil},
		{name: "entries compose and the result is sorted", exclude: []string{"project.get", "gitlab_issue_list"}, want: []string{"issue.list", "project.get"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exclusionTestCatalog(t).ExcludedActionIDs(tt.exclude)
			if !slices.Equal(got, tt.want) {
				t.Errorf("ExcludedActionIDs(%v) = %v, want %v", tt.exclude, got, tt.want)
			}
		})
	}
	t.Run("nil catalog resolves to nothing", func(t *testing.T) {
		var nilCatalog *Catalog
		if got := nilCatalog.ExcludedActionIDs([]string{"gitlab_issue"}); got != nil {
			t.Errorf("nil catalog ExcludedActionIDs() = %v, want nil", got)
		}
	})
}
