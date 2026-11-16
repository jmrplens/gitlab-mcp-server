// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package finegrained

import (
	"encoding/json"
	"reflect"
	"testing"
)

// answerScope is one granular scope as GET /personal_access_tokens/:id
// spells it, which the tests decode the way client-go does.
type answerScope struct {
	Access      string   `json:"access"`
	Permissions []string `json:"permissions"`
	ProjectID   *int64   `json:"project_id"`
	GroupID     *int64   `json:"group_id"`
}

// tokenAnswer is the part of GET /personal_access_tokens/:id the grant is
// read from.
type tokenAnswer struct {
	Granular       *bool         `json:"granular"`
	GranularScopes []answerScope `json:"granular_scopes"`
}

// decodeAnswer decodes body the way the caller of [GrantFromScopes] reads it,
// keeping an absent or null list nil and an empty one empty.
func decodeAnswer(body []byte) (*bool, []GrantScope, error) {
	var answer tokenAnswer
	if err := json.Unmarshal(body, &answer); err != nil {
		return nil, nil, err
	}
	if answer.GranularScopes == nil {
		return answer.Granular, nil, nil
	}
	scopes := make([]GrantScope, 0, len(answer.GranularScopes))
	for _, scope := range answer.GranularScopes {
		scopes = append(scopes, GrantScope(scope))
	}
	return answer.Granular, scopes, nil
}

// grantFromBody evaluates the grant body carries, failing the test when body
// does not decode.
func grantFromBody(t *testing.T, body string, maxScopes int) (Grant, FallbackReason) {
	t.Helper()
	granular, scopes, err := decodeAnswer([]byte(body))
	if err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return GrantFromScopes(granular, scopes, maxScopes)
}

// TestGrantFromScopes_ReadsEveryNamespaceKind verifies each access level
// GitLab writes is read with the namespace it names: a personal project scope
// at the user, a selected membership at the project or group whose id it
// carries, and the others at no namespace.
func TestGrantFromScopes_ReadsEveryNamespaceKind(t *testing.T) {
	body := `{"granular":true,"granular_scopes":[
		{"access":"personal_projects","permissions":["read_project"]},
		{"access":"selected_memberships","permissions":["read_issue"],"project_id":7},
		{"access":"selected_memberships","permissions":["read_issue"],"group_id":9},
		{"access":"all_memberships","permissions":["read_merge_request"]},
		{"access":"user","permissions":["read_user"]},
		{"access":"instance","permissions":["read_metadata"]}
	]}`
	grant, reason := grantFromBody(t, body, 10)
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

// TestGrantFromScopes_UnionsARepeatedNamespaceAndScopesWithNone verifies two
// scopes naming one namespace, or one access level with no namespace, are
// read as one scope holding the permissions of both, each once and in the
// order they first appeared, while the same access on another project stays a
// scope of its own.
func TestGrantFromScopes_UnionsARepeatedNamespaceAndScopesWithNone(t *testing.T) {
	body := `{"granular_scopes":[
		{"access":"selected_memberships","permissions":["read_issue","update_issue"],"project_id":7},
		{"access":"selected_memberships","permissions":["update_issue","create_note"],"project_id":7},
		{"access":"selected_memberships","permissions":["read_issue"],"project_id":8},
		{"access":"user","permissions":["read_user"]},
		{"access":"instance","permissions":["read_metadata"]},
		{"access":"user","permissions":["read_personal_access_token"]}
	]}`
	grant, reason := grantFromBody(t, body, 10)
	if reason != FallbackNone {
		t.Fatalf("reason = %q", reason)
	}
	want := []Scope{
		{Access: AccessSelectedMemberships, Namespace: NamespaceProject, NamespaceID: 7, Permissions: []string{"read_issue", "update_issue", "create_note"}},
		{Access: AccessSelectedMemberships, Namespace: NamespaceProject, NamespaceID: 8, Permissions: []string{"read_issue"}},
		{Access: AccessUser, Permissions: []string{"read_user", "read_personal_access_token"}},
		{Access: AccessInstance, Permissions: []string{"read_metadata"}},
	}
	if !reflect.DeepEqual(grant.Scopes, want) {
		t.Errorf("scopes =\n%+v\nwant\n%+v", grant.Scopes, want)
	}
}

// TestGrantFromScopes_AnEmptyListIsAGrantOfNothing verifies an empty list of
// scopes is read as a grant that holds nothing, not as a grant that could not
// be read.
func TestGrantFromScopes_AnEmptyListIsAGrantOfNothing(t *testing.T) {
	grant, reason := grantFromBody(t, `{"granular":true,"granular_scopes":[]}`, 10)
	if reason != FallbackNone || grant.Scopes == nil || len(grant.Scopes) != 0 {
		t.Errorf("GrantFromScopes = %+v, %q; want an empty grant", grant, reason)
	}
}

// TestGrantFromScopes_FallsBackRatherThanGuesses verifies every answer the
// evaluation cannot read whole is answered with the reason it fell back and
// no grant: a token that says it is not fine-grained, scopes that are absent
// or null, an access level it does not know (a null entry among them), a scope
// naming both a project and a group or a selected membership naming neither,
// and more scopes than the bound. An answer that is not JSON never reaches
// it: the read falls back on its own (TestReadGrant in internal/gitlab).
func TestGrantFromScopes_FallsBackRatherThanGuesses(t *testing.T) {
	cases := []struct {
		name string
		body string
		want FallbackReason
	}{
		{name: "a null entry", body: `{"granular_scopes":[null]}`, want: FallbackGrantShape},
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
			grant, reason := grantFromBody(t, tc.body, 2)
			if reason != tc.want || grant.Scopes != nil {
				t.Errorf("GrantFromScopes = %+v, %q; want no grant and %q", grant, reason, tc.want)
			}
		})
	}
}

// TestGrantFromScopes_AtTheScopeBoundIsRead verifies a grant holding exactly
// as many scopes as the bound is read, so the bound refuses only what exceeds
// it.
func TestGrantFromScopes_AtTheScopeBoundIsRead(t *testing.T) {
	_, reason := grantFromBody(t, `{"granular_scopes":[{"access":"user"},{"access":"instance"}]}`, 2)
	if reason != FallbackNone {
		t.Errorf("reason = %q, want a grant at exactly the bound read", reason)
	}
}

// FuzzGrantFromScopes holds the evaluation to what its callers rely on
// whatever the instance sends, decoded the way client-go decodes it: it never
// panics, never returns more scopes than the bound, returns a grant only with
// no fallback reason, and never returns a scope with an access level or a
// namespace it had to guess. A body that does not decode is skipped, since
// the read falls back on it before any evaluation.
func FuzzGrantFromScopes(f *testing.F) {
	f.Add([]byte(`{"granular":true,"granular_scopes":[{"access":"selected_memberships","permissions":["read_issue"],"group_id":9}]}`))
	f.Add([]byte(`{"granular_scopes":[{"access":"personal_projects"},{"access":"user"},{"access":"user"}]}`))
	f.Add([]byte(`{"granular_scopes":null}`))
	f.Add([]byte(`[]`))
	f.Fuzz(func(t *testing.T, body []byte) {
		granular, scopes, err := decodeAnswer(body)
		if err != nil {
			return
		}
		grant, reason := GrantFromScopes(granular, scopes, 3)
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
