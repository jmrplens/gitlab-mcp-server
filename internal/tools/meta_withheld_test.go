// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestMetaActionKeys_IndexesEveryActionANarrowingCanRemove verifies the index a
// meta call naming a withheld action is matched by: each meta tool's route
// names and historical spellings, mapped to canonical IDs, for the actions of
// the unfiltered catalog and the ones the tier removed before it was built,
// and at Ultimate, where the tier removes nothing, the catalog alone.
func TestMetaActionKeys_IndexesEveryActionANarrowingCanRemove(t *testing.T) {
	free, err := MetaActionKeys(false, edition.Free)
	if err != nil {
		t.Fatalf("MetaActionKeys(Free) error = %v", err)
	}
	cases := []struct {
		tool, name, want string
	}{
		{tool: "gitlab_issue", name: "create", want: "issue.create"},
		{tool: "gitlab_project", name: "approval_config_get", want: "project.approval_config_get"},
	}
	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.name, func(t *testing.T) {
			if got := free[tc.tool][tc.name]; got != tc.want {
				t.Errorf("MetaActionKeys(Free)[%s][%s] = %q, want %q", tc.tool, tc.name, got, tc.want)
			}
		})
	}

	t.Run("at Ultimate the tier removes nothing", func(t *testing.T) {
		ultimate, ultimateErr := MetaActionKeys(false, edition.Ultimate)
		if ultimateErr != nil {
			t.Fatalf("MetaActionKeys(Ultimate) error = %v", ultimateErr)
		}
		if got := ultimate["gitlab_project"]["approval_config_get"]; got != "project.approval_config_get" {
			t.Errorf("MetaActionKeys(Ultimate) lost a served action: %q", got)
		}
		removed, removedErr := TierWithheld(false, &config.ServerConfig{Tier: edition.Ultimate})
		if removedErr != nil || removed != nil {
			t.Errorf("TierWithheld(Ultimate) = %v, %v, want nothing removed", removed, removedErr)
		}
	})
}

// TestMetaActionKeys_ARouteNameWinsOverAnotherActionsSpelling verifies the
// rule the dispatcher reads a name by: a name the tool routes is that action,
// whichever other action claims it as a historical spelling, and a spelling no
// route is named resolves to the action that claims it, wherever in the
// catalogs the two are.
func TestMetaActionKeys_ARouteNameWinsOverAnotherActionsSpelling(t *testing.T) {
	catalog := func(name string, aliases ...string) *actioncatalog.Catalog {
		t.Helper()
		route := toolutil.Route(func(context.Context, map[string]any) (any, error) { return "ran", nil })
		route.CompatibilityAliases = aliases
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_demo", BaseDomain: "demo", SurfaceKind: actioncatalog.SurfaceKindMetaGroup})
		group.SetAction(actioncatalog.Action{Name: name, Route: route})
		built := actioncatalog.NewCatalog()
		if err := built.AddGroup(group); err != nil {
			t.Fatalf("AddGroup() error = %v", err)
		}
		return built
	}

	keys := metaActionKeys([]*actioncatalog.Catalog{catalog("old", "list", "legacy"), catalog("list")})
	want := map[string]string{"old": "demo.old", "list": "demo.list", "legacy": "demo.old"}
	for name, id := range want {
		t.Run(name, func(t *testing.T) {
			if got := keys["gitlab_demo"][name]; got != id {
				t.Errorf("metaActionKeys()[gitlab_demo][%s] = %q, want %q", name, got, id)
			}
		})
	}
}

// TestMetaActionKeys_AHistoricalSpellingResolvesToItsAction verifies the
// index against the catalog the meta surface starts from: every historical
// spelling no route of its tool is named resolves to the action that claims
// it, which is how the dispatcher reads it too.
func TestMetaActionKeys_AHistoricalSpellingResolvesToItsAction(t *testing.T) {
	free, err := MetaActionKeys(false, edition.Free)
	if err != nil {
		t.Fatalf("MetaActionKeys(Free) error = %v", err)
	}
	catalog, err := SharedBaseCatalog(false, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("SharedBaseCatalog() error = %v", err)
	}
	type spelling struct{ tool, alias, id string }
	var spellings []spelling
	for _, group := range catalog.Groups() {
		for _, action := range group.Actions {
			for _, alias := range action.Route.CompatibilityAliases {
				if _, routed := group.Actions[alias]; !routed {
					spellings = append(spellings, spelling{tool: group.ToolName, alias: alias, id: string(action.ID)})
				}
			}
		}
	}
	if len(spellings) == 0 {
		t.Fatal("no action of the Free catalog carries a historical spelling to check")
	}
	for _, s := range spellings {
		t.Run(s.tool+"/"+s.alias, func(t *testing.T) {
			if got := free[s.tool][s.alias]; got != s.id {
				t.Errorf("MetaActionKeys(Free)[%s][%s] = %q, want %q", s.tool, s.alias, got, s.id)
			}
		})
	}
}

// TestMetaActionKeys_ReportsWhichCatalogItCouldNotRead verifies the index
// fails with the step that failed, through the seams that stand in for the
// failures no compiled-in spec can cause.
func TestMetaActionKeys_ReportsWhichCatalogItCouldNotRead(t *testing.T) {
	forced := errors.New("forced catalog failure")
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		want    string
	}{
		{
			name: "the base catalog",
			arrange: func(t *testing.T) {
				t.Helper()
				original := sharedBaseCatalog
				t.Cleanup(func() { sharedBaseCatalog = original })
				sharedBaseCatalog = func(bool, ActionCatalogOptions) (*actioncatalog.Catalog, error) { return nil, forced }
			},
			want: "index the meta action names: ",
		},
		{
			name: "what the tier removed",
			arrange: func(t *testing.T) {
				t.Helper()
				original := sharedTierRemoved
				t.Cleanup(func() { sharedTierRemoved = original })
				sharedTierRemoved = func(bool, edition.Tier) (*actioncatalog.Catalog, error) { return nil, forced }
			},
			want: "index the meta action names the tier removed: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			keys, err := MetaActionKeys(false, edition.Premium)
			if keys != nil || !errors.Is(err, forced) || !strings.HasPrefix(err.Error(), tc.want) {
				t.Errorf("MetaActionKeys() = %v, %v, want the forced failure under %q", keys, err, tc.want)
			}
		})
	}
}

// TestMetaActionKeys_AtUltimate_ReadsNothingTheTierRemoved verifies the index
// at the top tier is built from the base catalog alone: Ultimate removes
// nothing, so what the tier removed is not read at all, as [TierWithheld]
// does not read it either, rather than read and found empty. Every licensed
// deployment builds its index at Ultimate, and the read is a whole catalog.
func TestMetaActionKeys_AtUltimate_ReadsNothingTheTierRemoved(t *testing.T) {
	original := sharedTierRemoved
	reads := 0
	sharedTierRemoved = func(bool, edition.Tier) (*actioncatalog.Catalog, error) {
		reads++
		return actioncatalog.NewCatalog(), nil
	}
	t.Cleanup(func() { sharedTierRemoved = original })

	for _, tc := range []struct {
		name   string
		dotcom bool
	}{
		{name: "self-managed", dotcom: false},
		{name: "GitLab.com", dotcom: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys, err := MetaActionKeys(tc.dotcom, edition.Ultimate)
			if err != nil || len(keys) == 0 {
				t.Errorf("MetaActionKeys(Ultimate) = %d tools, %v; want the base catalog's index", len(keys), err)
			}
		})
	}
	if reads != 0 {
		t.Errorf("what the tier removed was read %d times at Ultimate, want none", reads)
	}
}

// TestSharedMetaCatalog_RecordsWhatTheTierWithheld verifies the meta catalog
// carries what the tier removed beside what its filter withheld, as the
// dynamic catalog does (issue 956), so a meta call naming one is answered
// with the tier it needs (issue 1282); and that an action the operator
// excluded by name is not among them (AUT-004).
func TestSharedMetaCatalog_RecordsWhatTheTierWithheld(t *testing.T) {
	client := UnboundClient(false)
	_, withheld, err := SharedMetaCatalog(client, &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"project.approval_rule_list"}})
	if err != nil {
		t.Fatalf("SharedMetaCatalog() error = %v", err)
	}
	recorded := map[string]string{}
	for _, entry := range withheld.ByTier {
		recorded[entry.ID] = entry.Edition
	}
	if got := recorded["project.approval_config_get"]; got != "premium" {
		t.Errorf("ByTier[project.approval_config_get] = %q, want premium", got)
	}
	if _, named := recorded["project.approval_rule_list"]; named {
		t.Error("ByTier names an action the operator excluded by name")
	}
}
