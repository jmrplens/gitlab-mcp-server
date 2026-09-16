//go:build e2e

// group_protection_test.go drives the group protection builders' pure halves
// against the stub: the branch rule and the directory link, each with what its
// create sends, the two endings its removal has, and what its read-back
// answers, including the listing that answers 404 for a group with no link at
// all.

package fixture

import (
	"net/http"
	"testing"
)

// TestProtectGroupBranch_Created_SendsThePatternAndItsAccess checks that the
// builder names the pattern and the access a protection grants.
func TestProtectGroupBranch_Created_SendsThePatternAndItsAccess(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/5/protected_branches", stubCreated(map[string]any{
		"id": 3, "name": "release/*",
	}))

	got, err := protectGroupBranch(t.Context(), client, 5, "release/*")
	if err != nil {
		t.Fatalf("protectGroupBranch() error = %v, want nil", err)
	}
	want := GroupProtectedBranch{ID: 3, Name: "release/*"}
	if got != want {
		t.Errorf("protectGroupBranch() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("protectGroupBranch() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["name"] != "release/*" || requests[0].Body["push_access_level"] == nil {
		t.Errorf("protectGroupBranch() sent %v, want the pattern and its push access", requests[0].Body)
	}
}

// TestUnprotectGroupBranch_Endings_ToleratesOneACaseLifted checks that a rule a
// case already lifted is not a cleanup failure and any other refusal is.
func TestUnprotectGroupBranch_Endings_ToleratesOneACaseLifted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "lifted", answer: stubNoContent()},
		{name: "already lifted", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/groups/5/protected_branches/release/*", tc.answer)

			err := unprotectGroupBranch(t.Context(), client, 5, "release/*")
			if (err != nil) != tc.wantErr {
				t.Errorf("unprotectGroupBranch() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestGroupBranchIsProtected_Answers covers the three answers the read-back
// distinguishes.
func TestGroupBranchIsProtected_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "protected", answer: stubOK(map[string]any{"id": 3, "name": "release/*"}), want: true},
		{name: "lifted", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/groups/5/protected_branches/release/*", tc.answer)

			got, err := GroupBranchIsProtected(t.Context(), client, 5, "release/*")
			if (err != nil) != tc.wantErr {
				t.Fatalf("GroupBranchIsProtected() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GroupBranchIsProtected() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestAddGroupLDAPLink_Created_NamesTheProviderTheStackConfigures checks that
// the builder sends the common name and the provider a link records.
func TestAddGroupLDAPLink_Created_NamesTheProviderTheStackConfigures(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/5/ldap_group_links", stubCreated(map[string]any{
		"cn": "eval-cn", "provider": LDAPProvider, "group_access": 30,
	}))

	got, err := addGroupLDAPLink(t.Context(), client, 5, "eval-cn")
	if err != nil {
		t.Fatalf("addGroupLDAPLink() error = %v, want nil", err)
	}
	want := LDAPLink{CN: "eval-cn", Provider: LDAPProvider}
	if got != want {
		t.Errorf("addGroupLDAPLink() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("addGroupLDAPLink() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["provider"] != LDAPProvider {
		t.Errorf("addGroupLDAPLink() sent %v, want the configured provider", requests[0].Body)
	}
}

// TestDeleteGroupLDAPLink_Endings_ToleratesOneACaseDeleted checks that a link a
// case already deleted is not a cleanup failure and any other refusal is.
func TestDeleteGroupLDAPLink_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/groups/5/ldap_group_links", tc.answer)

			err := deleteGroupLDAPLink(t.Context(), client, 5, "eval-cn", LDAPProvider)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteGroupLDAPLink() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestGroupHasLDAPLink_Answers covers the four answers the read-back
// distinguishes, including GitLab's own: a group with no link at all answers
// the listing with a 404 rather than an empty page, which is a "no" and not a
// broken world.
func TestGroupHasLDAPLink_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{
			name:   "linked",
			answer: stubOK([]map[string]any{{"cn": "eval-cn", "provider": LDAPProvider}}),
			want:   true,
		},
		{name: "another provider", answer: stubOK([]map[string]any{{"cn": "other", "provider": "ldapalt"}})},
		{name: "no link at all", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/groups/5/ldap_group_links", tc.answer)

			got, err := GroupHasLDAPLink(t.Context(), client, 5, LDAPProvider)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GroupHasLDAPLink() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GroupHasLDAPLink() = %t, want %t", got, tc.want)
			}
		})
	}
}
