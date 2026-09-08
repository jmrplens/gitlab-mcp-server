package toolutil

import (
	"errors"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// TestCapturedMember_ReadsWhatTheSDKDoesNotModel verifies the single-member
// reader: the two fields GitLab sends on every member, the five it sends
// under a condition, each on the key lib/api/entities/member.rb spells, and
// a capture nothing ran under reported as such.
func TestCapturedMember_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	capture := gitlabclient.CapturedBody([]byte(`{"id":1,"locked":true,"public_email":"alice@public.example",` +
		`"membership_state":"awaiting","two_factor_enabled":true,"override":false,` +
		`"group_saml_identity":{"extern_uid":"u1","provider":"group_saml","saml_provider_id":3},` +
		`"group_scim_identity":{"extern_uid":"s1","group_id":9,"active":true}}`))

	got, err := CapturedMember(capture)
	if err != nil {
		t.Fatalf("CapturedMember() error = %v", err)
	}
	if !got.Locked || got.PublicEmail != "alice@public.example" || got.MembershipState != "awaiting" ||
		got.TwoFactorEnabled == nil || !*got.TwoFactorEnabled || got.Override == nil || *got.Override ||
		got.GroupSAMLIdentity == nil || got.GroupSAMLIdentity.SAMLProviderID != 3 ||
		got.GroupSCIMIdentity == nil || got.GroupSCIMIdentity.GroupID != 9 || !got.GroupSCIMIdentity.Active {
		t.Errorf("CapturedMember() = %+v, want every captured field", got)
	}
	_, untouched := gitlabclient.WithResponseCapture(t.Context())
	_, err = CapturedMember(untouched)
	if !errors.Is(err, gitlabclient.ErrNoResponseCaptured) {
		t.Errorf("CapturedMember() on a capture nothing ran under = %v, want ErrNoResponseCaptured", err)
	}
}

// TestCapturedMember_AConditionalFieldNotSent_StaysNil verifies that a
// member GitLab sent without the conditional fields converts to nil pointers
// rather than false values, so an output cannot claim a member has no second
// factor when GitLab said nothing about it.
func TestCapturedMember_AConditionalFieldNotSent_StaysNil(t *testing.T) {
	got, err := CapturedMember(gitlabclient.CapturedBody([]byte(`{"id":1,"locked":false}`)))
	if err != nil {
		t.Fatalf("CapturedMember() error = %v", err)
	}
	if got.Locked || got.TwoFactorEnabled != nil || got.Override != nil || got.GroupSAMLIdentity != nil || got.GroupSCIMIdentity != nil {
		t.Errorf("CapturedMember() = %+v, want the conditional fields nil", got)
	}
}

// TestCapturedMembers_HoldsTheCountToTheSDKs verifies the list reader: one
// extra per member in order, a count other than the SDK's refused with both
// numbers, and a body that is not a list refused as a decode.
func TestCapturedMembers_HoldsTheCountToTheSDKs(t *testing.T) {
	capture := gitlabclient.CapturedBody([]byte(`[{"id":1,"locked":true},{"id":2,"membership_state":"active"}]`))

	got, err := CapturedMembers(capture, 2)
	if err != nil {
		t.Fatalf("CapturedMembers() error = %v", err)
	}
	if len(got) != 2 || !got[0].Locked || got[1].MembershipState != "active" {
		t.Errorf("CapturedMembers() = %+v, want two extras in order", got)
	}

	_, err = CapturedMembers(capture, 1)
	if err == nil || !strings.Contains(err.Error(), "holds 2 members and the SDK decoded 1") {
		t.Errorf("CapturedMembers() with another count = %v, want the two numbers", err)
	}
	_, err = CapturedMembers(gitlabclient.CapturedBody([]byte(`{"id":1}`)), 1)
	if err == nil || !strings.Contains(err.Error(), "decode the captured response") {
		t.Errorf("CapturedMembers() on an object = %v, want a decode error", err)
	}
}
