package finegrained

import (
	"reflect"
	"testing"
)

// TestDecodeGrant_ReadsEveryNamespaceKind verifies each access level GitLab
// writes is read with the namespace it names: a personal project scope at the
// user, a selected membership at the project or group whose id it carries, and
// the others at no namespace.
func TestDecodeGrant_ReadsEveryNamespaceKind(t *testing.T) {
	body := `{"granular":true,"granular_scopes":[
		{"access":"personal_projects","permissions":["read_project"]},
		{"access":"selected_memberships","permissions":["read_issue"],"project_id":7},
		{"access":"selected_memberships","permissions":["read_issue"],"group_id":9},
		{"access":"all_memberships","permissions":["read_merge_request"]},
		{"access":"user","permissions":["read_user"]},
		{"access":"instance","permissions":["read_metadata"]}
	]}`
	grant, reason := DecodeGrant([]byte(body), 10)
	if reason != FallbackNone {
		t.Fatalf("reason = %q", reason)
	}
	want := []Scope{
		{Access: AccessPersonalProjects, Namespace: NamespaceUser, Permissions: []string{"read_project"}},
		{Access: AccessSelectedMemberships, Namespace: NamespaceProject, NamespaceID: 7, Permissions: []string{"read_issue"}},
		{Access: AccessSelectedMemberships, Namespace: NamespaceGroup, NamespaceID: 9, Permissions: []string{"read_issue"}},
		{Access: AccessAllMemberships, Namespace: NamespaceNone, Permissions: []string{"read_merge_request"}},
		{Access: AccessUser, Namespace: NamespaceNone, Permissions: []string{"read_user"}},
		{Access: AccessInstance, Namespace: NamespaceNone, Permissions: []string{"read_metadata"}},
	}
	if !reflect.DeepEqual(grant.Scopes, want) {
		t.Errorf("scopes =\n%+v\nwant\n%+v", grant.Scopes, want)
	}
}

// TestDecodeGrant_UnionsARepeatedNamespaceAndScopesWithNone verifies two
// scopes naming one namespace, or one access level with no namespace, are
// read as one scope holding the permissions of both, each once and in the
// order they first appeared.
func TestDecodeGrant_UnionsARepeatedNamespaceAndScopesWithNone(t *testing.T) {
	body := `{"granular_scopes":[
		{"access":"selected_memberships","permissions":["read_issue","update_issue"],"project_id":7},
		{"access":"selected_memberships","permissions":["update_issue","create_note"],"project_id":7},
		{"access":"user","permissions":["read_user"]},
		{"access":"instance","permissions":["read_metadata"]},
		{"access":"user","permissions":["read_personal_access_token"]}
	]}`
	grant, reason := DecodeGrant([]byte(body), 10)
	if reason != FallbackNone {
		t.Fatalf("reason = %q", reason)
	}
	want := []Scope{
		{Access: AccessSelectedMemberships, Namespace: NamespaceProject, NamespaceID: 7, Permissions: []string{"read_issue", "update_issue", "create_note"}},
		{Access: AccessUser, Permissions: []string{"read_user", "read_personal_access_token"}},
		{Access: AccessInstance, Permissions: []string{"read_metadata"}},
	}
	if !reflect.DeepEqual(grant.Scopes, want) {
		t.Errorf("scopes =\n%+v\nwant\n%+v", grant.Scopes, want)
	}
}

// TestDecodeGrant_AnEmptyListIsAGrantOfNothing verifies an empty list of
// scopes is read as a grant that holds nothing, not as a grant that could not
// be read.
func TestDecodeGrant_AnEmptyListIsAGrantOfNothing(t *testing.T) {
	grant, reason := DecodeGrant([]byte(`{"granular":true,"granular_scopes":[]}`), 10)
	if reason != FallbackNone || grant.Scopes == nil || len(grant.Scopes) != 0 {
		t.Errorf("DecodeGrant = %+v, %q; want an empty grant", grant, reason)
	}
}

// TestDecodeGrant_FallsBackRatherThanGuesses verifies every body the decoder
// cannot read whole is answered with the reason it fell back and no grant:
// a body that is not JSON or not a fine-grained token, scopes that are absent
// or null, an access level it does not know, a scope naming both a project and
// a group or a selected membership naming neither, and more scopes than the
// bound.
func TestDecodeGrant_FallsBackRatherThanGuesses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want FallbackReason
	}{
		{name: "not JSON", body: `{`, want: FallbackGrantShape},
		{name: "not a fine-grained token", body: `{"granular":false,"granular_scopes":[]}`, want: FallbackGrantShape},
		{name: "absent scopes", body: `{"granular":true}`, want: FallbackGrantUnreadable},
		{name: "null scopes", body: `{"granular":true,"granular_scopes":null}`, want: FallbackGrantUnreadable},
		{name: "unknown access", body: `{"granular_scopes":[{"access":"everything","permissions":[]}]}`, want: FallbackGrantShape},
		{name: "both ids", body: `{"granular_scopes":[{"access":"selected_memberships","project_id":1,"group_id":2}]}`, want: FallbackGrantShape},
		{name: "selected with no id", body: `{"granular_scopes":[{"access":"selected_memberships","permissions":["read_issue"]}]}`, want: FallbackGrantShape},
		{name: "too many scopes", body: `{"granular_scopes":[{"access":"user"},{"access":"instance"},{"access":"all_memberships"}]}`, want: FallbackGrantTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			grant, reason := DecodeGrant([]byte(tc.body), 2)
			if reason != tc.want || grant.Scopes != nil {
				t.Errorf("DecodeGrant = %+v, %q; want no grant and %q", grant, reason, tc.want)
			}
		})
	}
}

// TestDecodeGrant_AtTheScopeBoundIsRead verifies a grant holding exactly as
// many scopes as the bound is read, so the bound refuses only what exceeds it.
func TestDecodeGrant_AtTheScopeBoundIsRead(t *testing.T) {
	_, reason := DecodeGrant([]byte(`{"granular_scopes":[{"access":"user"},{"access":"instance"}]}`), 2)
	if reason != FallbackNone {
		t.Errorf("reason = %q, want a grant at exactly the bound read", reason)
	}
}

// FuzzDecodeGrant holds the decoder to what its callers rely on whatever the
// instance sends: it never panics, never returns more scopes than the bound,
// returns a grant only with no fallback reason, and never returns a scope
// with an access level or a namespace it had to guess.
func FuzzDecodeGrant(f *testing.F) {
	f.Add([]byte(`{"granular":true,"granular_scopes":[{"access":"selected_memberships","permissions":["read_issue"],"group_id":9}]}`))
	f.Add([]byte(`{"granular_scopes":[{"access":"personal_projects"},{"access":"user"},{"access":"user"}]}`))
	f.Add([]byte(`{"granular_scopes":null}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, body []byte) {
		grant, reason := DecodeGrant(body, 3)
		if reason != FallbackNone {
			if grant.Scopes != nil {
				t.Errorf("a fallback came with scopes: %+v", grant)
			}
			return
		}
		if grant.Scopes == nil || len(grant.Scopes) > 3 {
			t.Errorf("a grant of %d scopes", len(grant.Scopes))
		}
		for _, scope := range grant.Scopes {
			if scope.Access < AccessPersonalProjects || scope.Access > AccessInstance {
				t.Errorf("access level %d", scope.Access)
			}
			if scope.Access == AccessSelectedMemberships && scope.Namespace == NamespaceNone {
				t.Errorf("a selected memberships scope with no namespace was guessed at")
			}
		}
	})
}
