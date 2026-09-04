// dynamiccatalog_test.go pins the two properties the server and the e2e suite
// both rely on: the filter runs before the standalone actions are added, and
// what read-only mode removed is reported with its cause.
package dynamiccatalog

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
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
