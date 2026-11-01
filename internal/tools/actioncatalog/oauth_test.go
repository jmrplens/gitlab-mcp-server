// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actioncatalog

import (
	"context"
	"errors"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestAction_OAuthRefused_ReadsTheGeneratedTable verifies the actions GitLab
// refuses to an OAuth access token on every way, from the routes the
// generated table marks (issue 1238): the calling token's own revocation and
// rotations and the model registry's package file download are, the personal
// token read is not, since a call naming the token by id reaches a route
// GitLab serves an OAuth token, and an action with no row is refused nothing.
func TestAction_OAuthRefused_ReadsTheGeneratedTable(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{id: "access.token_personal_revoke_self", want: true},
		{id: "access.token_personal_rotate_self", want: true},
		{id: "access.token_group_rotate_self", want: true},
		{id: "access.token_project_rotate_self", want: true},
		{id: "model_registry.download", want: true},
		{id: "access.token_personal_get", want: false},
		{id: "issue.list", want: false},
		{id: "no.such_action", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			action := Action{ID: ActionID(tc.id), FineGrained: actiongrants.Requirement(tc.id)}
			if got := action.OAuthRefused(); got != tc.want {
				t.Errorf("OAuthRefused() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestCatalog_FilterServedToOAuth_DropsWhatNoWayRunsAndEmptyGroups verifies
// the narrowing an OAuth session's catalog takes: an action refused on every
// way leaves, the rest stay in their order, a group left empty is dropped,
// and a kept group is read-only once only reads are left in it.
func TestCatalog_FilterServedToOAuth_DropsWhatNoWayRunsAndEmptyGroups(t *testing.T) {
	route := toolutil.Route(func(context.Context, map[string]any) (any, error) { return "ran", nil })
	access := NewGroup(GroupOptions{ToolName: "gitlab_access", BaseDomain: "access", SurfaceKind: SurfaceKindMetaGroup})
	access.SetAction(Action{Name: "token_personal_get", Route: route, ReadOnly: true, FineGrained: actiongrants.Requirement("access.token_personal_get")})
	access.SetAction(Action{Name: "token_personal_rotate_self", Route: route, FineGrained: actiongrants.Requirement("access.token_personal_rotate_self")})
	registry := NewGroup(GroupOptions{ToolName: "gitlab_model_registry", BaseDomain: "model_registry", SurfaceKind: SurfaceKindMetaGroup})
	registry.SetAction(Action{Name: "download", Route: route, ReadOnly: true, FineGrained: actiongrants.Requirement("model_registry.download")})
	writes := NewGroup(GroupOptions{ToolName: "gitlab_demo", BaseDomain: "demo", SurfaceKind: SurfaceKindMetaGroup})
	writes.SetAction(Action{Name: "read", Route: route, ReadOnly: true})
	writes.SetAction(Action{Name: "write", Route: route})
	catalog := NewCatalog()
	err := errors.Join(catalog.AddGroup(access), catalog.AddGroup(registry), catalog.AddGroup(writes))
	if err != nil {
		t.Fatalf("AddGroup() error = %v", err)
	}

	served := catalog.FilterServedToOAuth()
	if _, kept := served.Group("gitlab_model_registry"); kept {
		t.Error("a group whose every action is refused to an OAuth token was kept")
	}
	group, kept := served.Group("gitlab_access")
	if !kept || len(group.Actions) != 1 || group.Actions["token_personal_get"].Name == "" {
		t.Fatalf("gitlab_access = %+v, %t; want only token_personal_get kept", group.Actions, kept)
	}
	if !group.ReadOnly {
		t.Error("a group left with reads only is not read-only")
	}
	if mixed, mixedKept := served.Group("gitlab_demo"); !mixedKept || mixed.ReadOnly || len(mixed.Actions) != 2 {
		t.Errorf("gitlab_demo = %+v, %t; want both actions kept and the group not read-only", mixed, mixedKept)
	}
	var none *Catalog
	if none.FilterServedToOAuth() != nil {
		t.Error("FilterServedToOAuth() on no catalog returned one")
	}
}
