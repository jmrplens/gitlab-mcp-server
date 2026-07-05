package toolutil

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewMemberUserOutput pins the created_by conversion: nil SDK input maps
// to nil, and a populated gl.MemberCreatedBy maps every field onto the shared
// output shape.
func TestNewMemberUserOutput(t *testing.T) {
	if got := NewMemberUserOutput(nil); got != nil {
		t.Fatalf("NewMemberUserOutput(nil) = %+v, want nil", got)
	}

	in := &gl.MemberCreatedBy{
		ID:        7,
		Username:  "jdoe",
		Name:      "John Doe",
		State:     "active",
		AvatarURL: "https://example.com/avatar.png",
		WebURL:    "https://example.com/jdoe",
	}
	got := NewMemberUserOutput(in)
	if got == nil {
		t.Fatal("NewMemberUserOutput returned nil for non-nil input")
	}
	if got.ID != 7 || got.Username != "jdoe" || got.Name != "John Doe" ||
		got.State != "active" || got.AvatarURL != in.AvatarURL || got.WebURL != in.WebURL {
		t.Errorf("NewMemberUserOutput = %+v, want mirror of %+v", got, in)
	}
}

// TestNewSAMLIdentityOutput pins the group_saml_identity conversion:
// nil-on-nil plus a full field mirror for populated input.
func TestNewSAMLIdentityOutput(t *testing.T) {
	if got := NewSAMLIdentityOutput(nil); got != nil {
		t.Fatalf("NewSAMLIdentityOutput(nil) = %+v, want nil", got)
	}

	in := &gl.GroupMemberSAMLIdentity{
		ExternUID:      "uid-1",
		Provider:       "group_saml",
		SAMLProviderID: 12,
	}
	got := NewSAMLIdentityOutput(in)
	if got == nil {
		t.Fatal("NewSAMLIdentityOutput returned nil for non-nil input")
	}
	if got.ExternUID != "uid-1" || got.Provider != "group_saml" || got.SAMLProviderID != 12 {
		t.Errorf("NewSAMLIdentityOutput = %+v, want mirror of %+v", got, in)
	}
}

// TestNewMemberRoleOutput pins the member_role conversion: nil-on-nil plus a
// full mirror of the identifying fields and a sample of permission flags
// (all flags are copied by the same one-line assignments; the sample guards
// against dropped wiring).
func TestNewMemberRoleOutput(t *testing.T) {
	if got := NewMemberRoleOutput(nil); got != nil {
		t.Fatalf("NewMemberRoleOutput(nil) = %+v, want nil", got)
	}

	in := &gl.MemberRole{
		ID:                        3,
		Name:                      "custom-dev",
		Description:               "custom role",
		GroupID:                   42,
		BaseAccessLevel:           gl.DeveloperPermissions,
		AdminCICDVariables:        true,
		AdminMergeRequests:        true,
		ManageProjectAccessTokens: true,
		ReadCode:                  true,
		RemoveProject:             true,
	}
	got := NewMemberRoleOutput(in)
	if got == nil {
		t.Fatal("NewMemberRoleOutput returned nil for non-nil input")
	}
	if got.ID != 3 || got.Name != "custom-dev" || got.Description != "custom role" ||
		got.GroupID != 42 || got.BaseAccessLevel != int(gl.DeveloperPermissions) {
		t.Errorf("NewMemberRoleOutput identity fields = %+v, want mirror of %+v", got, in)
	}
	if !got.AdminCICDVariables || !got.AdminMergeRequests || !got.ManageProjectAccessTokens ||
		!got.ReadCode || !got.RemoveProject {
		t.Errorf("NewMemberRoleOutput permission flags = %+v, want true flags mirrored from %+v", got, in)
	}
	if got.AdminPushRules || got.RemoveGroup {
		t.Errorf("NewMemberRoleOutput unset flags = %+v, want false flags preserved", got)
	}
}
