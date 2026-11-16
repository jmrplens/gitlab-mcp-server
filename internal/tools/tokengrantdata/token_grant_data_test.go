// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tokengrantdata

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestOptions_AGrant_IsSpelledAsClientGoOptions holds the conversion both
// token create handlers send: no grant is nil, which client-go leaves out of
// the request, and each scope becomes one options entry carrying its access,
// permissions and the projects and groups it names.
func TestOptions_AGrant_IsSpelledAsClientGoOptions(t *testing.T) {
	if got := Options(nil); got != nil {
		t.Errorf("Options(nil) = %v, want nil", got)
	}
	got := Options([]GranularScopeInput{
		{Access: "user", Permissions: []string{"read_user"}},
		{Access: "selected_memberships", Permissions: []string{"read_project"}, ProjectIDs: []int64{7}, GroupIDs: []int64{9}},
	})
	want := []*gl.GranularScopeOptions{
		{Access: new(gl.GranularScopeAccessUser), Permissions: []string{"read_user"}},
		{Access: new(gl.GranularScopeAccessSelectedMemberships), Permissions: []string{"read_project"}, ProjectIDs: []int64{7}, GroupIDs: []int64{9}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Options() = %#v, want %#v", got, want)
	}
}

// TestCheckScopesChoice_ExactlyOneKindOfGrant_IsAccepted holds the
// rule both token create routes share: classic scopes or a grant, one of
// them and never both, with an empty list counted as absent, since the
// handlers leave an empty list out of the request.
func TestCheckScopesChoice_ExactlyOneKindOfGrant_IsAccepted(t *testing.T) {
	grant := []GranularScopeInput{{Access: "user", Permissions: []string{"read_user"}}}
	cases := []struct {
		name     string
		scopes   []string
		granular []GranularScopeInput
		wantErr  bool
	}{
		{name: "classic scopes alone", scopes: []string{"api"}},
		{name: "a grant alone", granular: grant},
		{name: "neither", wantErr: true},
		{name: "empty lists of both", scopes: []string{}, granular: []GranularScopeInput{}, wantErr: true},
		{name: "both", scopes: []string{"api"}, granular: grant, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckScopesChoice(tc.scopes, tc.granular)
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckScopesChoice() error = %v, want error %t", err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "exactly one of scopes") {
				t.Errorf("error = %q, want it to name the two alternatives", err)
			}
		})
	}
}

// TestCheckGrant_RefusesTheChoiceFirstAndThenTheScope holds the order
// of the two refusals: a call naming both kinds of grant is told so even
// when its grant is malformed too, a well-chosen call with a malformed grant
// is told what is wrong with the scope, and a sound call passes.
func TestCheckGrant_RefusesTheChoiceFirstAndThenTheScope(t *testing.T) {
	malformed := []GranularScopeInput{{Access: "user"}}
	cases := []struct {
		name     string
		scopes   []string
		granular []GranularScopeInput
		want     string
	}{
		{name: "both kinds, the grant malformed", scopes: []string{"api"}, granular: malformed, want: "exactly one of scopes"},
		{name: "a malformed grant alone", granular: malformed, want: "granular_scopes[0].permissions is required"},
		{name: "a sound grant", granular: []GranularScopeInput{{Access: "user", Permissions: []string{"read_user"}}}},
		{name: "classic scopes", scopes: []string{"api"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckGrant(tc.scopes, tc.granular)
			if tc.want == "" {
				if err != nil {
					t.Errorf("CheckGrant() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("CheckGrant() error = %v, want one carrying %q", err, tc.want)
			}
		})
	}
}

// TestValidateScopes_AGrantGitLabTakes_IsAccepted holds every
// access value with what it may carry, and an absent grant, to no error.
func TestValidateScopes_AGrantGitLabTakes_IsAccepted(t *testing.T) {
	cases := []struct {
		name   string
		scopes []GranularScopeInput
	}{
		{name: "no grant"},
		{name: "every access", scopes: []GranularScopeInput{
			{Access: "selected_memberships", Permissions: []string{"read_project"}, ProjectIDs: []int64{7}, GroupIDs: []int64{9}},
			{Access: "selected_memberships", Permissions: []string{"read_group"}, GroupIDs: []int64{9}},
			{Access: "personal_projects", Permissions: []string{"read_project"}},
			{Access: "all_memberships", Permissions: []string{"read_project"}},
			{Access: "user", Permissions: []string{"read_user"}},
			{Access: "instance", Permissions: []string{"read_metadata"}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateScopes(tc.scopes); err != nil {
				t.Errorf("ValidateScopes() error = %v, want nil", err)
			}
		})
	}
}

// TestValidateScopes_AScopeGitLabWouldReadOtherwise_IsRefused covers
// each refusal, naming the offending entry by its index: GitLab refuses an
// unknown access and a scope without permissions, drops the IDs beside any
// access but selected_memberships, which widens the grant to all of that
// access, and builds no scope at all from selected_memberships without IDs.
func TestValidateScopes_AScopeGitLabWouldReadOtherwise_IsRefused(t *testing.T) {
	valid := GranularScopeInput{Access: "user", Permissions: []string{"read_user"}}
	cases := []struct {
		name  string
		scope GranularScopeInput
		want  string
	}{
		{name: "an unknown access", scope: GranularScopeInput{Access: "everything", Permissions: []string{"read_user"}}, want: "granular_scopes[1].access must be one of personal_projects, all_memberships, selected_memberships, user, instance"},
		{name: "no access", scope: GranularScopeInput{Permissions: []string{"read_user"}}, want: "granular_scopes[1].access must be one of"},
		{name: "no permissions", scope: GranularScopeInput{Access: "user"}, want: "granular_scopes[1].permissions is required"},
		{name: "projects beside all_memberships", scope: GranularScopeInput{Access: "all_memberships", Permissions: []string{"read_project"}, ProjectIDs: []int64{7}}, want: "granular_scopes[1]: project_ids and group_ids go with access selected_memberships"},
		{name: "groups beside personal_projects", scope: GranularScopeInput{Access: "personal_projects", Permissions: []string{"read_project"}, GroupIDs: []int64{9}}, want: "granular_scopes[1]: project_ids and group_ids go with access selected_memberships"},
		{name: "selected_memberships naming nothing", scope: GranularScopeInput{Access: "selected_memberships", Permissions: []string{"read_project"}}, want: "which needs at least one of them"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScopes([]GranularScopeInput{valid, tc.scope})
			if err == nil {
				t.Fatal("ValidateScopes() = nil, want an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to carry %q", err, tc.want)
			}
		})
	}
}

// TestWrapCreateErr_TheGranular404_CarriesTheFeatureFlagHint holds the
// hint to the one case it explains: a 404 answered to a granular call. A
// classic call's 404 and a granular call's other refusals keep the action's
// own status hint, or GitLab's message when the status is not the hinted one.
func TestWrapCreateErr_TheGranular404_CarriesTheFeatureFlagHint(t *testing.T) {
	refusal := func(status int) error {
		return &gl.ErrorResponse{Response: &http.Response{StatusCode: status, Request: &http.Request{Method: http.MethodPost}}, Message: "refused"}
	}
	const ownHint = "the action's own hint"
	cases := []struct {
		name     string
		err      error
		granular bool
		want     string
		notWant  string
	}{
		{name: "a granular 404", err: refusal(http.StatusNotFound), granular: true, want: NotFoundHint},
		{name: "a classic 404", err: refusal(http.StatusNotFound), notWant: NotFoundHint},
		{name: "a granular refusal on the hinted status", err: refusal(http.StatusForbidden), granular: true, want: ownHint, notWant: NotFoundHint},
		{name: "a classic refusal on the hinted status", err: refusal(http.StatusForbidden), want: ownHint},
		{name: "a refusal on another status", err: refusal(http.StatusBadRequest), want: "refused", notWant: ownHint},
		{name: "an error that is no HTTP refusal", err: errors.New("connection reset"), granular: true, want: "connection reset", notWant: NotFoundHint},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := WrapCreateErr("create_token", tc.err, tc.granular, OnStatus(http.StatusForbidden), ownHint).Error()
			if !strings.HasPrefix(got, "create_token") {
				t.Errorf("error = %q, want it to name the operation first", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("error = %q, want it to carry %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("error = %q, want it without %q", got, tc.notWant)
			}
		})
	}
}

// TestSchemaOverrides_PublishTheChoiceAndTheAccessValues pins the
// two overrides: the root oneOf over the two kinds of grant and the access
// enum, which is client-go's five values in its order.
func TestSchemaOverrides_PublishTheChoiceAndTheAccessValues(t *testing.T) {
	want := []toolutil.InputSchemaOverride{
		toolutil.SchemaOneOfRequired("scopes", "granular_scopes"),
		toolutil.SchemaEnumOverride("granular_scopes.access", "personal_projects", "all_memberships", "selected_memberships", "user", "instance"),
	}
	if got := SchemaOverrides(); !reflect.DeepEqual(got, want) {
		t.Errorf("SchemaOverrides() = %#v, want %#v", got, want)
	}
}
