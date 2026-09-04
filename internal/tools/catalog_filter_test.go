// catalog_filter_test.go covers the bookkeeping behind a narrowed catalog: what
// a filter removed and on whose decision, which is what lets the dynamic
// surface tell a model "this credential cannot" apart from "this server
// cannot".
package tools

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// TestFilterActionCatalog_ReportsWhatItWithheldAndWhy pins that narrowing the
// catalog records what was removed, split by whose decision the caller can act
// on.
//
// The dynamic surface answers an action it cannot find with "unknown action"
// plus near misses. That is correct for a typo and a misdiagnosis for a
// narrowed credential: the near misses are all real read-only actions, so the
// answer reads as "this server cannot write" rather than "this token cannot".
// Splitting the two causes is what lets the surface say "reauthorize" only when
// reauthorizing would actually help.
//
// Tools removed by name through --exclude-tools stay out of both lists on
// purpose: the operator asked for them not to exist, so naming them in an error
// would leak the configuration and contradict it.
func TestFilterActionCatalog_ReportsWhatItWithheldAndWhy(t *testing.T) {
	t.Parallel()

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{
		Tier:       edition.Free,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("a read-only token gets the reauthorize half", func(t *testing.T) {
		t.Parallel()
		assertWithheldByTokenScope(t, catalog)
	})

	t.Run("an operator-imposed read-only mode gets the other half", func(t *testing.T) {
		t.Parallel()
		assertWithheldByOperator(t, catalog)
	})

	t.Run("an excluded tool is not withheld, it is absent", func(t *testing.T) {
		t.Parallel()
		assertExcludedToolIsNotWithheld(t, catalog)
	})

	t.Run("nothing is withheld when nothing is narrowed", func(t *testing.T) {
		t.Parallel()
		assertNothingWithheld(t, catalog)
	})
}

// withheldWriteAction and withheldReadAction are the two catalog actions the
// withheld-bookkeeping assertions below are written against: one that read-only
// mode removes and one it must keep.
const (
	withheldWriteAction = "issue.create"
	withheldReadAction  = "issue.list"
)

// mustFilterCatalog runs the catalog filter and fails on error, returning both
// the narrowed catalog and the bookkeeping under test.
func mustFilterCatalog(t *testing.T, catalog *actioncatalog.Catalog, cfg *config.ServerConfig) (*actioncatalog.Catalog, WithheldActions) {
	t.Helper()
	filtered, withheld, err := FilterActionCatalog(catalog, cfg)
	if err != nil {
		t.Fatalf("FilterActionCatalog() error = %v", err)
	}
	return filtered, withheld
}

func assertWithheldByTokenScope(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	filtered, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:               true,
		ReadOnlyFromTokenScope: true,
	})
	if _, ok := filtered.Action(withheldWriteAction); ok {
		t.Fatalf("FilterActionCatalog() kept %q in a read-only catalog", withheldWriteAction)
	}
	if !slices.Contains(withheld.ByTokenScope, withheldWriteAction) {
		t.Errorf("withheld.ByTokenScope does not name %q; a narrowed credential would be reported as a missing capability", withheldWriteAction)
	}
	if slices.Contains(withheld.ByOperator, withheldWriteAction) {
		t.Errorf("withheld.ByOperator names %q, but the token is the cause here", withheldWriteAction)
	}
	if slices.Contains(withheld.ByTokenScope, withheldReadAction) {
		t.Errorf("withheld.ByTokenScope names %q, which is still reachable", withheldReadAction)
	}
}

func assertWithheldByOperator(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{ReadOnly: true})
	if !slices.Contains(withheld.ByOperator, withheldWriteAction) {
		t.Errorf("withheld.ByOperator does not name %q", withheldWriteAction)
	}
	if slices.Contains(withheld.ByTokenScope, withheldWriteAction) {
		t.Errorf("withheld.ByTokenScope names %q, but no credential narrowed this deployment", withheldWriteAction)
	}
}

func assertExcludedToolIsNotWithheld(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:     true,
		ExcludeTools: []string{"gitlab_issue"},
	})
	for _, keys := range [][]string{withheld.ByTokenScope, withheld.ByOperator} {
		if slices.Contains(keys, withheldWriteAction) {
			t.Errorf("an excluded tool's action %q was reported as withheld; exclusion means it does not exist here", withheldWriteAction)
		}
	}
}

func assertNothingWithheld(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{})
	if len(withheld.ByTokenScope) != 0 || len(withheld.ByOperator) != 0 {
		t.Errorf("withheld = %+v, want empty for an unnarrowed catalog", withheld)
	}
}

// TestRemovedActionKeys_CoverCompatibilityAliases pins that an action's older
// names are withheld alongside its canonical one.
//
// The dynamic registry resolves compatibility aliases exactly as it resolves
// declared ones, so a caller working from a name the catalog used to carry
// would otherwise be told the action is unknown, which is precisely the
// misdiagnosis the withheld path exists to prevent, arriving through the door
// left open.
func TestRemovedActionKeys_CoverCompatibilityAliases(t *testing.T) {
	t.Parallel()

	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{
		Tier:       edition.Free,
		IncludeMCP: true,
	})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	var wantAlias string
	var wantAction string
	for _, action := range catalog.Actions() {
		if action.ReadOnly || len(action.Compatibility.ActionAliases) == 0 {
			continue
		}
		wantAlias = action.Compatibility.ActionAliases[0].Alias
		wantAction = string(action.ID)
		break
	}
	if wantAlias == "" {
		t.Skip("no mutating action in the Free catalog declares a compatibility alias")
	}

	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{
		ReadOnly:               true,
		ReadOnlyFromTokenScope: true,
	})
	if !slices.Contains(withheld.ByTokenScope, wantAlias) {
		t.Errorf("withheld.ByTokenScope omits %q, the compatibility alias of the withheld action %q; a caller using it would be told the action does not exist",
			wantAlias, wantAction)
	}
}

// TestRemovedActionKeys_ReportsWhatAFilterTookAway covers the diff behind the
// message a caller gets when an action was withheld.
//
// Naming the cause is the whole point: a model told only that an action is
// unknown, alongside suggestions that are all real read-only actions, concludes
// the server lacks the capability rather than that the credential is narrow.
// A missing catalog on either side yields nothing rather than claiming
// everything was removed.
func TestRemovedActionKeys_ReportsWhatAFilterTookAway(t *testing.T) {
	t.Parallel()

	if got := RemovedActionKeys(nil, actioncatalog.NewCatalog()); got != nil {
		t.Errorf("RemovedActionKeys(nil, empty) = %v, want nothing claimed", got)
	}
	if got := RemovedActionKeys(actioncatalog.NewCatalog(), nil); got != nil {
		t.Errorf("RemovedActionKeys(empty, nil) = %v, want nothing claimed", got)
	}
}

// TestExcludeFromCatalog_NothingToExclude_ReturnsTheSameCatalog covers the
// early exit: with no patterns the catalog is handed back untouched rather
// than copied and counted.
func TestExcludeFromCatalog_NothingToExclude_ReturnsTheSameCatalog(t *testing.T) {
	t.Parallel()

	catalog := actioncatalog.NewCatalog()
	if got := ExcludeFromCatalog(catalog, nil); got != catalog {
		t.Error("ExcludeFromCatalog with no patterns returned a different catalog")
	}
}
