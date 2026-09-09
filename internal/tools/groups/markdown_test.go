// markdown_test.go contains unit tests for the group Markdown formatters
// covering provisioned-user lists and transfer-location lists.
package groups

import (
	"strings"
	"testing"
)

// TestFormatProvisionedUsersListMarkdown_Empty covers the empty-list branch.
func TestFormatProvisionedUsersListMarkdown_Empty(t *testing.T) {
	md := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{})
	if !strings.Contains(md, "No provisioned users found.") {
		t.Fatalf("markdown = %q, want empty message", md)
	}
}

// TestGroupMarkdown_RowsLinkOnlyWhatGitLabGaveAURLFor verifies the three
// tables that turn a name into a link do so only when the row carries a URL,
// and print the plain name otherwise, rather than an empty link.
func TestGroupMarkdown_RowsLinkOnlyWhatGitLabGaveAURLFor(t *testing.T) {
	locations := FormatTransferLocationsListMarkdown(TransferLocationsListOutput{
		Locations: []TransferLocationOutput{
			{ID: 1, Name: "linked", FullPath: "g/linked", WebURL: "https://gl/linked"},
			{ID: 2, Name: "plain", FullPath: "g/plain"},
		},
	})
	if !strings.Contains(locations, "[linked](https://gl/linked)") {
		t.Errorf("location with a URL was not linked:\n%s", locations)
	}
	if strings.Contains(locations, "[plain](") {
		t.Errorf("location with no URL was linked:\n%s", locations)
	}

	users := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{
		Users: []ProvisionedUserOutput{
			{ID: 1, Username: "linked", WebURL: "https://gl/linked"},
			{ID: 2, Username: "plain"},
		},
	})
	if !strings.Contains(users, "[linked](https://gl/linked)") {
		t.Errorf("user with a URL was not linked:\n%s", users)
	}
	if strings.Contains(users, "[plain](") {
		t.Errorf("user with no URL was linked:\n%s", users)
	}
}

// TestFormatHookMarkdown_URLVariablesSectionAppearsOnlyWhenThereAreAny
// verifies the masked URL-variable section is written for a hook that has
// variables and left out entirely for one that has none, so an empty table
// header never stands on its own.
func TestFormatHookMarkdown_URLVariablesSectionAppearsOnlyWhenThereAreAny(t *testing.T) {
	with := FormatHookMarkdown(HookOutput{
		ID: 1, URL: "https://example.com/hook",
		URLVariables: []HookURLVariable{{Key: "env"}},
	})
	if !strings.Contains(with, "### URL Variables") || !strings.Contains(with, "env") {
		t.Errorf("markdown missing the URL variables section:\n%s", with)
	}
	without := FormatHookMarkdown(HookOutput{ID: 1, URL: "https://example.com/hook"})
	if strings.Contains(without, "### URL Variables") {
		t.Errorf("markdown carries a URL variables section for a hook with none:\n%s", without)
	}
}

// TestFormatProvisionedUsersListMarkdown_Rows covers the populated table branch.
func TestFormatProvisionedUsersListMarkdown_Rows(t *testing.T) {
	md := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{
		Users: []ProvisionedUserOutput{{ID: 7, Username: "scim-user", Name: "SCIM User", State: "active", Email: "s@e.com", WebURL: "https://g/scim-user"}},
	})
	if !strings.Contains(md, "[scim-user](https://g/scim-user)") {
		t.Fatalf("markdown = %q, want a clickable user link", md)
	}
}

// TestFormatTransferLocationsListMarkdown verifies the transfer-locations markdown formatter.
// The test exercises rendering of a populated location list.
// It asserts the rendered Markdown contains a clickable name link.
func TestFormatTransferLocationsListMarkdown(t *testing.T) {
	out := TransferLocationsListOutput{Locations: []TransferLocationOutput{
		{ID: 99, Name: "Target", FullPath: "target", WebURL: "https://gitlab.example.com/groups/target"},
	}}
	md := FormatTransferLocationsListMarkdown(out)
	if !strings.Contains(md, "[Target](https://gitlab.example.com/groups/target)") {
		t.Errorf("expected clickable link in markdown, got: %s", md)
	}
}

// TestFormatTransferLocationsListMarkdown_Empty verifies the empty-state rendering.
// The test exercises rendering of an empty location list.
// It asserts the empty-state message is present.
func TestFormatTransferLocationsListMarkdown_Empty(t *testing.T) {
	md := FormatTransferLocationsListMarkdown(TransferLocationsListOutput{})
	if !strings.Contains(md, "No transfer locations available") {
		t.Errorf("expected empty-state message, got: %s", md)
	}
}

// ---------------------------------------------------------------------------
// Create/Update new options (client-go v2.41.0)
// ---------------------------------------------------------------------------.
