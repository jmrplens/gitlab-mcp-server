package tools

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools/actioncatalog"
)

func TestBuildActionCatalog_IncludesBaseEnterpriseAndMCPActions(t *testing.T) {
	t.Run("base", func(t *testing.T) {
		base, err := BuildActionCatalog(nil, ActionCatalogOptions{})
		if err != nil {
			t.Fatalf("BuildActionCatalog(base) error = %v", err)
		}
		if base.CountGroups() == 0 || base.CountActions() == 0 {
			t.Fatalf("base catalog counts = groups %d actions %d, want non-zero", base.CountGroups(), base.CountActions())
		}
		if _, ok := base.Action("project.list"); !ok {
			t.Fatal("base catalog missing project.list")
		}
		if _, ok := base.Action("server.status"); ok {
			t.Fatal("base catalog contains server.status without IncludeMCP")
		}
	})

	t.Run("mcp", func(t *testing.T) {
		withMCP, err := BuildActionCatalog(nil, ActionCatalogOptions{IncludeMCP: true})
		if err != nil {
			t.Fatalf("BuildActionCatalog(with MCP) error = %v", err)
		}
		if _, ok := withMCP.Action("server.status"); !ok {
			t.Fatal("MCP catalog missing server.status")
		}
	})

	t.Run("enterprise", func(t *testing.T) {
		base, err := BuildActionCatalog(nil, ActionCatalogOptions{})
		if err != nil {
			t.Fatalf("BuildActionCatalog(base) error = %v", err)
		}
		enterprise, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
		if err != nil {
			t.Fatalf("BuildActionCatalog(enterprise) error = %v", err)
		}
		if enterprise.CountActions() <= base.CountActions() {
			t.Fatalf("enterprise action count = %d, want greater than base %d", enterprise.CountActions(), base.CountActions())
		}
	})
}

func TestBuildActionCatalog_CapturesInlineAndDelegatedGroups(t *testing.T) {
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	for _, actionID := range []string{"project.list", "search.code", "runner.list", "analyze.issue_summary"} {
		t.Run(actionID, func(t *testing.T) {
			if _, ok := catalog.Action(actioncatalog.ActionID(actionID)); !ok {
				t.Fatalf("catalog missing %s", actionID)
			}
		})
	}

	group, ok := catalog.Group("gitlab_analyze")
	if !ok {
		t.Fatal("catalog missing gitlab_analyze group")
	}
	if !group.ReadOnly {
		t.Fatal("gitlab_analyze group should be read-only")
	}
	if group.FormatResult == nil {
		t.Fatal("gitlab_analyze group should preserve its custom formatter")
	}
}

func TestBuildActionCatalog_DoesNotRegisterMCPTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Enterprise: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	if catalog.CountGroups() == 0 || catalog.CountActions() == 0 {
		t.Fatalf("catalog counts = groups %d actions %d, want non-zero", catalog.CountGroups(), catalog.CountActions())
	}

	if names := toolNamesFromServer(t, server); len(names) != 0 {
		t.Fatalf("BuildActionCatalog() registered MCP tools as a side effect: %v", names)
	}
}

func TestBuildMCPActionGroup_NilUpdaterOmitsUpdateActions(t *testing.T) {
	group := BuildMCPActionGroup(nil, nil)
	if _, ok := group.Actions["status"]; !ok {
		t.Fatal("BuildMCPActionGroup(nil updater) missing status action")
	}
	if _, ok := group.Actions["apply_update"]; ok {
		t.Fatal("BuildMCPActionGroup(nil updater) contains apply_update")
	}
	if group.ToolName != "gitlab_server" || group.Description == "" || len(group.Icons) == 0 {
		t.Fatalf("BuildMCPActionGroup metadata = %+v, want tool name, description, and icons", group)
	}
}
