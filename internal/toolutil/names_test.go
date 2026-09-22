// names_test.go pins the tool-name heuristics that annotation derivation and
// the surface-quality audit share, so the two keep agreeing on what a name
// says about the operation behind it.
package toolutil

import "testing"

// TestIsReadToolName_Suffixes_OnlyAWholeReadSuffixCounts verifies that every
// listed suffix marks a name as a read, and that a name merely containing one
// of the words, or ending in a longer word that starts like one, does not.
func TestIsReadToolName_Suffixes_OnlyAWholeReadSuffixCounts(t *testing.T) {
	for _, suffix := range ReadOnlyNameSuffixes {
		t.Run("gitlab_thing"+suffix, func(t *testing.T) {
			if !IsReadToolName("gitlab_thing" + suffix) {
				t.Errorf("IsReadToolName(%q) = false, want true for the listed suffix %q", "gitlab_thing"+suffix, suffix)
			}
		})
	}
	notReads := []string{
		"gitlab_project_create",
		"gitlab_list_projects_now",
		"gitlab_project_getter",
		"",
	}
	for _, name := range notReads {
		t.Run("not a read: "+name, func(t *testing.T) {
			if IsReadToolName(name) {
				t.Errorf("IsReadToolName(%q) = true, want false", name)
			}
		})
	}
}

// TestIsDeleteToolName_Forms_ASegmentOrTheSuffixCounts verifies that a delete
// is recognized as the trailing _delete or as a whole underscore-separated
// segment, and that a word which only contains "delete" is not one.
func TestIsDeleteToolName_Forms_ASegmentOrTheSuffixCounts(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "gitlab_project_delete", want: true},
		{name: "gitlab_delete_branch", want: true},
		{name: "delete", want: true},
		{name: "gitlab_undelete_project", want: false},
		{name: "gitlab_deleted_items_list", want: false},
		{name: "gitlab_project_get", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDeleteToolName(tt.name); got != tt.want {
				t.Errorf("IsDeleteToolName(%q) = %t, want %t", tt.name, got, tt.want)
			}
		})
	}
}
