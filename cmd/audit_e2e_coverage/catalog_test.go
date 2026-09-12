package main

import (
	"sort"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestBuildServedCatalog_RealCatalog_KnownFacts verifies the served catalog
// against facts the projection rests on and that the real catalog is known
// to hold: the diagnostics group is in, the standalone flows are in and
// named by their own tool on meta, a shadowed individual name is unservable
// with its owner named, an action declaring no individual tool is
// unservable there, and a tier prunes what it should.
func TestBuildServedCatalog_RealCatalog_KnownFacts(t *testing.T) {
	ultimate := builtCatalog(t, edition.Ultimate)

	t.Run("the diagnostics group is included", func(t *testing.T) {
		if _, known := ultimate.actions["server.status"]; !known {
			t.Error("server.status is not in the catalog: IncludeMCP was not applied")
		}
	})
	t.Run("server.health_check has no individual tool", func(t *testing.T) {
		action := ultimate.actions["server.health_check"]
		if action.individualTool != "" || action.metaTool != "gitlab_server" {
			t.Errorf("server.health_check = individual %q, meta %q; want none and gitlab_server", action.individualTool, action.metaTool)
		}
	})
	t.Run("repository.file_history is shadowed on individual", func(t *testing.T) {
		action := ultimate.actions["repository.file_history"]
		if action.individualTool != "" || action.individualOwner == "" {
			t.Errorf("repository.file_history = individual %q, owner %q; want unservable with its owner named", action.individualTool, action.individualOwner)
		}
	})
	t.Run("the elicitation flow is a standalone action named by its own tool on meta", func(t *testing.T) {
		action := ultimate.actions["interactive.issue_create"]
		if !action.standalone || action.metaTool != "gitlab_interactive_issue_create" || action.individualTool != "gitlab_interactive_issue_create" {
			t.Errorf("interactive.issue_create = standalone %t, meta %q, individual %q", action.standalone, action.metaTool, action.individualTool)
		}
	})
}

// builtCatalog builds the real catalog at a tier, failing the test if it
// cannot.
func builtCatalog(t *testing.T, tier edition.Tier) *servedCatalog {
	t.Helper()
	catalog, err := buildServedCatalog(tier)
	if err != nil {
		t.Fatalf("buildServedCatalog(%s) error = %v", tier, err)
	}
	return catalog
}

// TestBuildServedCatalog_RealCatalog_TierAndOrder verifies that a licensed
// action carries its tier and is pruned at Free, and that the id list is
// sorted and complete.
func TestBuildServedCatalog_RealCatalog_TierAndOrder(t *testing.T) {
	ultimate := builtCatalog(t, edition.Ultimate)
	free := builtCatalog(t, edition.Free)

	action, known := ultimate.actions["merge_train.get"]
	if !known || action.tier != edition.Premium {
		t.Errorf("merge_train.get = known %t, tier %s; want premium", known, action.tier)
	}
	if _, served := free.actions["merge_train.get"]; served {
		t.Error("merge_train.get is served at free")
	}
	if len(ultimate.ids) != len(ultimate.actions) || len(ultimate.ids) < 1000 {
		t.Errorf("ids = %d for %d actions", len(ultimate.ids), len(ultimate.actions))
	}
	if !sort.StringsAreSorted(ultimate.ids) {
		t.Error("ids are not sorted")
	}
}

// TestCatalogAction_ToolOn_Surfaces verifies the tool each surface calls an
// action by, and which surfaces cannot reach it.
func TestCatalogAction_ToolOn_Surfaces(t *testing.T) {
	action := catalogAction{id: "issue.list", metaTool: "gitlab_issue", individualTool: "gitlab_issue_list"}
	bare := catalogAction{id: "server.health_check", metaTool: "gitlab_server"}
	cases := []struct {
		name      string
		action    catalogAction
		surface   string
		wantTool  string
		reachable bool
	}{
		{name: "dynamic is the execute tool", action: action, surface: config.ToolSurfaceDynamic, wantTool: "gitlab_execute_action", reachable: true},
		{name: "meta is the group", action: action, surface: config.ToolSurfaceMeta, wantTool: "gitlab_issue", reachable: true},
		{name: "individual is the declared name", action: action, surface: config.ToolSurfaceIndividual, wantTool: "gitlab_issue_list", reachable: true},
		{name: "no individual tool", action: bare, surface: config.ToolSurfaceIndividual, reachable: false},
		{name: "unknown surface", action: action, surface: "other", reachable: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool, reachable := tc.action.toolOn(tc.surface)
			if tool != tc.wantTool || reachable != tc.reachable {
				t.Errorf("toolOn(%s) = (%q, %t), want (%q, %t)", tc.surface, tool, reachable, tc.wantTool, tc.reachable)
			}
		})
	}
}

// TestCatalogAction_UnservableReason_Rules verifies the reasons a surface in
// a mode cannot serve an action, in the order they are judged.
func TestCatalogAction_UnservableReason_Rules(t *testing.T) {
	mutation := catalogAction{id: "issue.create", metaTool: "gitlab_issue", individualTool: "gitlab_issue_create"}
	read := catalogAction{id: "issue.list", readOnly: true, metaTool: "gitlab_issue", individualTool: "gitlab_issue_list"}
	shadowed := catalogAction{id: "repository.file_history", readOnly: true, metaTool: "gitlab_repository", individualOwner: "repository.file"}
	served := map[string]bool{"gitlab_issue": true, "gitlab_issue_list": true, "gitlab_execute_action": true}
	cases := []struct {
		name    string
		action  catalogAction
		surface string
		mode    string
		want    string
	}{
		{name: "read-only withholds a mutation before anything else", action: mutation, surface: config.ToolSurfaceMeta, mode: modeReadOnly, want: "withheld by read-only mode"},
		{name: "safe mode withholds nothing", action: mutation, surface: config.ToolSurfaceMeta, mode: modeSafe, want: ""},
		{name: "a read in read-only is served", action: read, surface: config.ToolSurfaceMeta, mode: modeReadOnly, want: ""},
		{name: "shadowed names its owner", action: shadowed, surface: config.ToolSurfaceIndividual, mode: modeDefault, want: "individual tool name is registered for repository.file"},
		{name: "a tool the session did not list", action: mutation, surface: config.ToolSurfaceIndividual, mode: modeDefault, want: "tool gitlab_issue_create not served"},
		{name: "served", action: read, surface: config.ToolSurfaceIndividual, mode: modeDefault, want: ""},
		{name: "an unknown surface", action: read, surface: "other", mode: modeDefault, want: "no other tool"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.action.unservableReason(tc.surface, tc.mode, served); got != tc.want {
				t.Errorf("unservableReason(%s, %s) = %q, want %q", tc.surface, tc.mode, got, tc.want)
			}
		})
	}
}
