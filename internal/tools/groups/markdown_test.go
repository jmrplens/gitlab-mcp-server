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

// TestFormatProvisionedUsersListMarkdown_Rows covers the populated table branch.
func TestFormatProvisionedUsersListMarkdown_Rows(t *testing.T) {
	md := FormatProvisionedUsersListMarkdown(ProvisionedUsersListOutput{
		Users: []ProvisionedUserOutput{{ID: 7, Username: "scim-user", Name: "SCIM User", State: "active", Email: "s@e.com", WebURL: "https://g/scim-user"}},
	})
	if !strings.Contains(md, "scim-user") || !strings.Contains(md, "https://g/scim-user") {
		t.Fatalf("markdown = %q, want user row with link", md)
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
