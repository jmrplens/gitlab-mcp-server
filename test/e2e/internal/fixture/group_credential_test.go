//go:build e2e

// group_credential_test.go drives the SSH certificate, member role and group
// wiki builders' pure halves against the stub: what each create sends, the two
// endings each removal has, and what each read-back answers.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateGroupSSHCertificate_Created_SendsTheKeyAndItsTitle checks that the
// builder sends the generated key rather than a constant, which is what keeps
// two attempts from colliding on a fingerprint.
func TestCreateGroupSSHCertificate_Created_SendsTheKeyAndItsTitle(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/9/ssh_certificates", stubCreated(map[string]any{
		"id": 14, "title": "eval-cert",
	}))

	got, err := createGroupSSHCertificate(t.Context(), client, 9, "eval-cert", "ssh-ed25519 AAAA")
	if err != nil {
		t.Fatalf("createGroupSSHCertificate() error = %v, want nil", err)
	}
	want := SSHCertificate{ID: 14, Title: "eval-cert"}
	if got != want {
		t.Errorf("createGroupSSHCertificate() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 || requests[0].Body["key"] != "ssh-ed25519 AAAA" {
		t.Errorf("createGroupSSHCertificate() sent %v, want the key it was given", requests)
	}
}

// TestDeleteGroupSSHCertificate_Endings_ToleratesOneACaseDeleted checks that a
// certificate a case already deleted is not a cleanup failure.
func TestDeleteGroupSSHCertificate_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/groups/9/ssh_certificates/14", tc.answer)

			err := deleteGroupSSHCertificate(t.Context(), client, 9, 14)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteGroupSSHCertificate() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestGroupHasSSHCertificate_ReadsTheListing checks the read-back GitLab
// leaves no single-certificate read for.
func TestGroupHasSSHCertificate_ReadsTheListing(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "there", answer: stubOK([]map[string]any{{"id": 14, "title": "eval-cert"}}), want: true},
		{name: "gone", answer: stubOK([]map[string]any{})},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/groups/9/ssh_certificates", tc.answer)

			got, err := GroupHasSSHCertificate(t.Context(), client, 9, 14)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GroupHasSSHCertificate() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GroupHasSSHCertificate() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestCreateInstanceMemberRole_Created_SendsTheBaseAccessLevel checks that the
// role is built on the level GitLab demands with the name.
func TestCreateInstanceMemberRole_Created_SendsTheBaseAccessLevel(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/member_roles", stubCreated(map[string]any{
		"id": 6, "name": "eval-role", "base_access_level": 10,
	}))

	got, err := createInstanceMemberRole(t.Context(), client, "eval-role")
	if err != nil {
		t.Fatalf("createInstanceMemberRole() error = %v, want nil", err)
	}
	want := MemberRole{ID: 6, Name: "eval-role"}
	if got != want {
		t.Errorf("createInstanceMemberRole() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 || requests[0].Body["base_access_level"] == nil {
		t.Errorf("createInstanceMemberRole() sent %v, want a base access level", requests)
	}
}

// TestInstanceHasMemberRole_ReadsTheListing checks the read-back a case that
// deletes a role is judged on.
func TestInstanceHasMemberRole_ReadsTheListing(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "there", answer: stubOK([]map[string]any{{"id": 6, "name": "eval-role"}}), want: true},
		{name: "gone", answer: stubOK([]map[string]any{{"id": 7}})},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/member_roles", tc.answer)

			got, err := InstanceHasMemberRole(t.Context(), client, 6)
			if (err != nil) != tc.wantErr {
				t.Fatalf("InstanceHasMemberRole() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("InstanceHasMemberRole() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestDeleteInstanceMemberRole_Endings_ToleratesOneACaseDeleted checks that a
// role a case already deleted is not a cleanup failure.
func TestDeleteInstanceMemberRole_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/member_roles/6", tc.answer)

			err := deleteInstanceMemberRole(t.Context(), client, 6)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteInstanceMemberRole() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestCreateGroupWikiPage_Created_ReadsTheSlugGitLabChose checks that the
// builder hands back the slug rather than one computed from the title, which
// GitLab derives and does not always echo unchanged.
func TestCreateGroupWikiPage_Created_ReadsTheSlugGitLabChose(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/groups/9/wikis", stubCreated(map[string]any{
		"slug": "eval-group-wiki", "title": "Eval Group Wiki", "content": wikiContent,
	}))

	got, err := createGroupWikiPage(t.Context(), client, 9, "Eval Group Wiki")
	if err != nil {
		t.Fatalf("createGroupWikiPage() error = %v, want nil", err)
	}
	if got.Slug != "eval-group-wiki" || got.Title != "Eval Group Wiki" {
		t.Errorf("createGroupWikiPage() = %+v, want the slug and title GitLab answered", got)
	}
}

// TestDeleteGroupWikiPage_Endings_ToleratesOneACaseDeleted checks that a page a
// case already deleted is not a cleanup failure.
func TestDeleteGroupWikiPage_Endings_ToleratesOneACaseDeleted(t *testing.T) {
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
			stub.answers(http.MethodDelete, "/api/v4/groups/9/wikis/eval-group-wiki", tc.answer)

			err := deleteGroupWikiPage(t.Context(), client, 9, "eval-group-wiki")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteGroupWikiPage() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestGroupWikiPageExists_Answers covers the three answers the read-back
// distinguishes.
func TestGroupWikiPageExists_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		want    bool
		wantErr bool
	}{
		{name: "there", answer: stubOK(map[string]any{"slug": "eval-group-wiki"}), want: true},
		{name: "gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodGet, "/api/v4/groups/9/wikis/eval-group-wiki", tc.answer)

			got, err := GroupWikiPageExists(t.Context(), client, 9, "eval-group-wiki")
			if (err != nil) != tc.wantErr {
				t.Fatalf("GroupWikiPageExists() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("GroupWikiPageExists() = %t, want %t", got, tc.want)
			}
		})
	}
}
