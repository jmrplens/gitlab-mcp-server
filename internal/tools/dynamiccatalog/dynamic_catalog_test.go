// dynamic_catalog_test.go pins the two properties the server and the e2e suite
// both rely on: the filter runs before the standalone actions are added, and
// what read-only mode removed is reported with its cause.
package dynamiccatalog

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestBuild_ReadOnlyWithholdsWritesAndSaysWhose verifies an operator's
// read-only mode removes the writes, keeps the reads, and records the removal
// under the operator rather than the credential, which is what the dynamic
// surface needs to answer "exists but is not available" with the right remedy.
func TestBuild_ReadOnlyWithholdsWritesAndSaysWhose(t *testing.T) {
	t.Parallel()

	catalog, withheld, err := Build(nil, &config.ServerConfig{Tier: edition.Free, ReadOnly: true})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, ok := catalog.Action("issue.create"); ok {
		t.Error("issue.create survived read-only mode")
	}
	if _, ok := catalog.Action("issue.list"); !ok {
		t.Error("issue.list did not survive read-only mode")
	}
	if !slices.Contains(withheld.ByOperator, "issue.create") {
		t.Errorf("withheld.ByOperator = %v, want it to name issue.create", withheld.ByOperator)
	}
	if len(withheld.ByTokenScope) != 0 {
		t.Errorf("withheld.ByTokenScope = %v, want nothing: no credential narrowed this catalog", withheld.ByTokenScope)
	}
}

// TestBuild_SafeModePreviewsCoverTheStandaloneActions verifies that in safe
// mode every write in the built catalog answers with a preview, the standalone
// actions included.
//
// Every non-read-only handler is called with no parameters and no session,
// which a real handler could not survive and a preview handler ignores: a
// write that reached its real handler would fail on the missing session or
// try to reach GitLab, and either way return something other than a preview.
// That is what happened to the standalone writes before the previews were
// applied over the complete catalog.
func TestBuild_SafeModePreviewsCoverTheStandaloneActions(t *testing.T) {
	t.Parallel()

	built, _, err := Build(nil, &config.ServerConfig{Tier: edition.Free, SafeMode: true})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	writes := 0
	for _, action := range built.Actions() {
		if action.ReadOnly {
			continue
		}
		writes++
		if action.Route.Handler == nil {
			t.Errorf("%s has no handler", action.ID)
			continue
		}
		result, callErr := action.Route.Handler(t.Context(), map[string]any{})
		if callErr != nil {
			t.Errorf("%s in safe mode returned an error instead of a preview: %v", action.ID, callErr)
			continue
		}
		if _, ok := result.(toolutil.SafeModePreview); !ok {
			t.Errorf("%s in safe mode returned %T instead of a preview", action.ID, result)
		}
	}
	if writes == 0 {
		t.Fatal("the built catalog has no write to preview, so this test checked nothing")
	}
}

// TestBuild_AddsTheStandaloneActionsAfterFiltering verifies the standalone
// actions are on top of the filtered catalog rather than under the filter,
// so a narrowing cannot leave a hidden catalog action behind and the
// standalone ones are still reachable.
func TestBuild_AddsTheStandaloneActionsAfterFiltering(t *testing.T) {
	t.Parallel()

	cfg := &config.ServerConfig{Tier: edition.Free}
	built, withheld, err := Build(nil, cfg)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(withheld.ByOperator) != 0 || len(withheld.ByTokenScope) != 0 {
		t.Errorf("withheld = %+v, want nothing for an unnarrowed catalog", withheld)
	}

	plain, buildErr := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if buildErr != nil {
		t.Fatalf("BuildActionCatalog() error = %v", buildErr)
	}
	if built.CountActions() <= plain.CountActions() {
		t.Errorf("Build() has %d actions, the plain catalog %d; the standalone actions were not added", built.CountActions(), plain.CountActions())
	}
}
