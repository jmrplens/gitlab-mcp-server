// action_specs_test.go contains unit tests for the canonical action metadata of
// the work item saved view domain: the action set, its read/write/destructive
// classification, the discovery metadata every action carries, and the
// WorkItemSort enum override.
package workitemsavedviews

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// specsByName builds the action set once for a test, keyed by action name.
func specsByName(t *testing.T) map[string]toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	if len(byName) != len(specs) {
		t.Fatalf("ActionSpecs() has duplicate names: %d specs, %d distinct", len(specs), len(byName))
	}
	return byName
}

// TestActionSpecs_ActionSet verifies that the domain exposes exactly the seven
// SDK methods, no more and no fewer, so a future upstream addition shows up
// here rather than silently going unexposed.
func TestActionSpecs_ActionSet(t *testing.T) {
	byName := specsByName(t)
	want := []string{
		"work_item_saved_view_create",
		"work_item_saved_view_delete",
		"work_item_saved_view_get",
		"work_item_saved_view_list",
		"work_item_saved_view_subscribe",
		"work_item_saved_view_unsubscribe",
		"work_item_saved_view_update",
	}
	got := make([]string, 0, len(byName))
	for name := range byName {
		got = append(got, name)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("ActionSpecs() names = %v, want %v", got, want)
	}
}

// TestActionSpecs_Classification verifies that reads are read-only, the delete
// is destructive, and no other action is.
func TestActionSpecs_Classification(t *testing.T) {
	byName := specsByName(t)
	cases := []struct {
		name            string
		wantReadOnly    bool
		wantDestructive bool
	}{
		{name: "work_item_saved_view_get", wantReadOnly: true},
		{name: "work_item_saved_view_list", wantReadOnly: true},
		{name: "work_item_saved_view_create"},
		{name: "work_item_saved_view_update"},
		{name: "work_item_saved_view_delete", wantDestructive: true},
		{name: "work_item_saved_view_subscribe"},
		{name: "work_item_saved_view_unsubscribe"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := byName[tc.name]
			if !ok {
				t.Fatalf("action %q missing", tc.name)
			}
			if spec.ReadOnly != tc.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v", spec.ReadOnly, tc.wantReadOnly)
			}
			if spec.Destructive != tc.wantDestructive {
				t.Errorf("Destructive = %v, want %v", spec.Destructive, tc.wantDestructive)
			}
		})
	}
}

// TestActionSpecs_Ownership verifies that every action declares its owner
// package, its individual tool identity, and no edition gate: saved views are
// available on Free as well as Premium and Ultimate.
func TestActionSpecs_Ownership(t *testing.T) {
	for name, spec := range specsByName(t) {
		t.Run(name, func(t *testing.T) {
			if spec.OwnerPackage != "workitemsavedviews" {
				t.Errorf("OwnerPackage = %q, want workitemsavedviews", spec.OwnerPackage)
			}
			if spec.Edition != "" {
				t.Errorf("Edition = %q, want it unset: saved views are available on Free", spec.Edition)
			}
			if spec.IndividualTool.Name != "gitlab_"+name {
				t.Errorf("IndividualTool.Name = %q, want gitlab_%s", spec.IndividualTool.Name, name)
			}
			if spec.IndividualTool.Title == "" {
				t.Error("IndividualTool.Title is empty")
			}
		})
	}
}

// TestActionSpecs_Descriptions verifies that every individual-tool description
// states what it returns and carries the experimental notice, and that no action
// was left with the placeholder usage string.
func TestActionSpecs_Descriptions(t *testing.T) {
	for name, spec := range specsByName(t) {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(spec.IndividualTool.Description, "Returns:") {
				t.Errorf("IndividualTool.Description = %q, want a Returns: clause", spec.IndividualTool.Description)
			}
			if !strings.Contains(spec.IndividualTool.Description, "Experimental") {
				t.Errorf("IndividualTool.Description = %q, want the experimental notice", spec.IndividualTool.Description)
			}
			if strings.Contains(spec.Usage, "domain action") {
				t.Errorf("Usage = %q, want action-specific guidance rather than the placeholder", spec.Usage)
			}
		})
	}
}

// TestActionSpecs_DiscoveryMetadata verifies the metadata the find surface
// matches against: natural-language aliases beyond the tool name, related
// actions spelled as canonical issue-domain IDs, and the domain tag.
func TestActionSpecs_DiscoveryMetadata(t *testing.T) {
	for name, spec := range specsByName(t) {
		t.Run(name, func(t *testing.T) {
			naturalAliases := 0
			for _, alias := range spec.Aliases {
				if !strings.HasPrefix(alias, "gitlab_") {
					naturalAliases++
				}
			}
			if naturalAliases < 3 {
				t.Errorf("Aliases = %v, want at least three natural-language entries", spec.Aliases)
			}
			if len(spec.RelatedActions) == 0 {
				t.Error("RelatedActions is empty")
			}
			for _, related := range spec.RelatedActions {
				if !strings.HasPrefix(related, "issue.") {
					t.Errorf("RelatedActions entry %q is not a canonical issue-domain action ID", related)
				}
			}
			if !slices.Contains(spec.Tags, "saved_view") {
				t.Errorf("Tags = %v, want the saved_view tag", spec.Tags)
			}
		})
	}
}

// TestActionSpecs_SortEnum verifies that create and update constrain sort to the
// WorkItemSort enum, and that no other action does.
func TestActionSpecs_SortEnum(t *testing.T) {
	byName := specsByName(t)
	for name, spec := range byName {
		wantOverride := name == "work_item_saved_view_create" || name == "work_item_saved_view_update"
		t.Run(name, func(t *testing.T) {
			has := false
			for _, override := range spec.InputSchemaOverrides {
				if override.PropertyPath == "sort" {
					has = true
					if enum, ok := override.Values["enum"].([]any); !ok || len(enum) != len(workItemSortValues) {
						t.Errorf("sort enum = %v, want the full WorkItemSort list", override.Values["enum"])
					}
				}
			}
			if has != wantOverride {
				t.Errorf("sort override present = %v, want %v", has, wantOverride)
			}
		})
	}
}

// TestActionSpecs_FilterEnums verifies that create and update constrain every
// enum-typed filter field to exactly the values its GraphQL enum documents, and
// that no other action carries a filter override.
func TestActionSpecs_FilterEnums(t *testing.T) {
	want := map[string][]any{
		"filters.assignee_wildcard_id":                 {"ANY", "ME", "NONE"},
		"filters.health_status_filter":                 {"ANY", "NONE", "atRisk", "needsAttention", "onTrack"},
		"filters.hierarchy_filters.parent_wildcard_id": {"ANY", "NONE"},
		"filters.iteration_wildcard_id":                {"ANY", "CURRENT", "NONE"},
		"filters.milestone_wildcard_id":                {"ANY", "NONE", "STARTED", "UPCOMING"},
		"filters.state":                                {"all", "closed", "locked", "opened"},
		"filters.subscribed":                           {"EXPLICITLY_SUBSCRIBED", "EXPLICITLY_UNSUBSCRIBED"},
		"filters.weight_wildcard_id":                   {"ANY", "NONE"},
	}
	for name, spec := range specsByName(t) {
		wantOverrides := name == "work_item_saved_view_create" || name == "work_item_saved_view_update"
		t.Run(name, func(t *testing.T) {
			got := collectFilterEnums(t, spec)
			if !wantOverrides {
				if len(got) != 0 {
					t.Errorf("filter overrides = %v, want none", got)
				}
				return
			}
			if len(got) != len(want) {
				t.Errorf("filter override paths = %d, want %d: %v", len(got), len(want), got)
			}
			for path, values := range want {
				t.Run(path, func(t *testing.T) {
					if !slices.Equal(got[path], values) {
						t.Errorf("enum = %v, want %v", got[path], values)
					}
				})
			}
		})
	}
}

// collectFilterEnums returns the enum carried by every filters.* override on
// spec, keyed by property path, and fails the test for an override that
// carries none.
func collectFilterEnums(t *testing.T, spec toolutil.ActionSpec) map[string][]any {
	t.Helper()
	got := map[string][]any{}
	for _, override := range spec.InputSchemaOverrides {
		if !strings.HasPrefix(override.PropertyPath, "filters.") {
			continue
		}
		enum, ok := override.Values["enum"].([]any)
		if !ok {
			t.Errorf("override %q carries no enum: %v", override.PropertyPath, override.Values)
			continue
		}
		got[override.PropertyPath] = enum
	}
	return got
}

// TestWorkItemSortValues_Shape verifies that the enum list holds only uppercase
// strings, since the deprecated lowercase aliases are deliberately excluded.
func TestWorkItemSortValues_Shape(t *testing.T) {
	if len(workItemSortValues) == 0 {
		t.Fatal("workItemSortValues is empty")
	}
	for _, value := range workItemSortValues {
		str, ok := value.(string)
		if !ok {
			t.Errorf("enum value %#v is not a string", value)
			continue
		}
		t.Run(str, func(t *testing.T) {
			if str != strings.ToUpper(str) {
				t.Errorf("enum value %q is not uppercase", str)
			}
		})
	}
}
