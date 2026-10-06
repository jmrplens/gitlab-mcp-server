// catalog_filter_test.go covers the bookkeeping behind a narrowed catalog: what
// a filter removed and on whose decision, which is what lets the dynamic
// surface tell a model "this credential cannot" apart from "this server
// cannot".
package tools

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

// readAPIConfig is the configuration a credential carrying read_api and not
// api is served with, after NarrowToTokenScope.
func readAPIConfig() *config.ServerConfig {
	return &config.ServerConfig{ReadAPIOnly: true, TokenScopes: []string{"read_api"}}
}

// scopeWithheldIDs lists the keys of the token-scope bookkeeping.
func scopeWithheldIDs(withheld []actioncatalog.ScopeWithheld) []string {
	ids := make([]string, 0, len(withheld))
	for _, entry := range withheld {
		ids = append(ids, entry.ID)
	}
	return ids
}

// scopeMissing is what the token-scope bookkeeping says the credential lacks
// for one key, both halves together, and whether it names the key at all.
func scopeMissing(withheld []actioncatalog.ScopeWithheld, id string) ([]string, bool) {
	entry, ok := scopeEntry(withheld, id)
	return entry.Missing(), ok
}

// scopeEntry is the token-scope bookkeeping's entry for one key, and whether
// it names the key at all.
func scopeEntry(withheld []actioncatalog.ScopeWithheld, id string) (actioncatalog.ScopeWithheld, bool) {
	for _, entry := range withheld {
		if entry.ID == id {
			return entry, true
		}
	}
	return actioncatalog.ScopeWithheld{}, false
}

func assertWithheldByTokenScope(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	filtered, withheld := mustFilterCatalog(t, catalog, readAPIConfig())
	if _, ok := filtered.Action(withheldWriteAction); ok {
		t.Fatalf("FilterActionCatalog() kept %q for a read_api token", withheldWriteAction)
	}
	if missing, ok := scopeMissing(withheld.ByTokenScope, withheldWriteAction); !ok || !slices.Equal(missing, []string{"api"}) {
		t.Errorf("withheld.ByTokenScope gives %q missing %v (named %t); want api, or a narrowed credential would be reported as a missing capability",
			withheldWriteAction, missing, ok)
	}
	if slices.Contains(withheld.ByOperator, withheldWriteAction) {
		t.Errorf("withheld.ByOperator names %q, but the token is the cause here", withheldWriteAction)
	}
	if slices.Contains(scopeWithheldIDs(withheld.ByTokenScope), withheldReadAction) {
		t.Errorf("withheld.ByTokenScope names %q, which is still reachable", withheldReadAction)
	}
}

func assertWithheldByOperator(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	_, withheld := mustFilterCatalog(t, catalog, &config.ServerConfig{ReadOnly: true})
	if !slices.Contains(withheld.ByOperator, withheldWriteAction) {
		t.Errorf("withheld.ByOperator does not name %q", withheldWriteAction)
	}
	if slices.Contains(scopeWithheldIDs(withheld.ByTokenScope), withheldWriteAction) {
		t.Errorf("withheld.ByTokenScope names %q, but no credential narrowed this deployment", withheldWriteAction)
	}
}

func assertExcludedToolIsNotWithheld(t *testing.T, catalog *actioncatalog.Catalog) {
	t.Helper()
	cfg := readAPIConfig()
	cfg.ReadOnly = true
	cfg.ExcludeTools = []string{"gitlab_issue"}
	_, withheld := mustFilterCatalog(t, catalog, cfg)
	for _, keys := range [][]string{scopeWithheldIDs(withheld.ByTokenScope), withheld.ByOperator} {
		if slices.Contains(keys, withheldWriteAction) {
			t.Errorf("an excluded tool's action %q was reported as withheld; exclusion means it does not exist here", withheldWriteAction)
		}
	}
}

// TestFilterActionCatalog_ReadAPI_ServesWhatGitLabAcceptsFromIt pins the
// reach of a credential carrying read_api and not api, decided per action
// from what it sends rather than by its classification: a read GitLab
// refuses read_api (template.lint posts to the CI lint route) is withheld for
// want of api, a write it accepts (package.download sends GETs and writes a
// file on the server's machine; pipeline.trigger_run is authenticated by the
// trigger token the caller passes) is served, and a group keeping one such
// write is not annotated read-only.
func TestFilterActionCatalog_ReadAPI_ServesWhatGitLabAcceptsFromIt(t *testing.T) {
	t.Parallel()
	filtered, withheld := mustFilterCatalog(t, freeCatalogWithMCP(t), readAPIConfig())
	for _, id := range []actioncatalog.ActionID{"package.download", "pipeline.trigger_run", "repository.markdown_render", "issue.list"} {
		t.Run(string(id), func(t *testing.T) {
			t.Parallel()
			if _, ok := filtered.Action(id); !ok {
				t.Errorf("a read_api token is not served %s, which GitLab accepts from read_api", id)
			}
		})
	}
	if missing, ok := scopeMissing(withheld.ByTokenScope, "template.lint"); !ok || !slices.Equal(missing, []string{"api"}) {
		t.Errorf("template.lint is withheld from read_api with %v missing (named %t), want api", missing, ok)
	}
	if group, ok := filtered.Group("gitlab_package"); !ok || group.ReadOnly {
		t.Errorf("gitlab_package keeps package.download for read_api and is annotated read-only %t (present %t)", group.ReadOnly, ok)
	}
	if group, ok := filtered.Group("gitlab_issue"); !ok || !group.ReadOnly {
		t.Errorf("gitlab_issue keeps only reads for read_api and is annotated read-only %t (present %t)", group.ReadOnly, ok)
	}
}

// TestFilterActionCatalog_ReadAPIUnderReadOnly_FilesEveryWriteUnderTheOperator
// pins the two narrowings together: with the operator's read-only mode on as
// well as a read_api token, every write is filed under the operator, which
// runs first, the writes GitLab would accept from read_api included, and the
// read GitLab refuses read_api is still filed under the token.
func TestFilterActionCatalog_ReadAPIUnderReadOnly_FilesEveryWriteUnderTheOperator(t *testing.T) {
	t.Parallel()
	cfg := readAPIConfig()
	cfg.ReadOnly = true
	filtered, withheld := mustFilterCatalog(t, freeCatalogWithMCP(t), cfg)
	for _, id := range []string{"package.download", "pipeline.trigger_run", withheldWriteAction} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			if !slices.Contains(withheld.ByOperator, id) || slices.Contains(scopeWithheldIDs(withheld.ByTokenScope), id) {
				t.Errorf("%s is not withheld by the operator alone", id)
			}
		})
	}
	if _, ok := scopeMissing(withheld.ByTokenScope, "template.lint"); !ok {
		t.Error("template.lint, a read GitLab refuses read_api, is not withheld by the token's scope")
	}
	if _, ok := filtered.Action("issue.list"); !ok {
		t.Error("issue.list is not served")
	}
}

// readAPIWrites are the five actions the catalog classifies as writes that a
// credential carrying read_api and not api is served, because GitLab accepts
// each of them from read_api (ADR-0026).
var readAPIWrites = []actioncatalog.ActionID{
	"package.download",
	"access.token_personal_revoke_self",
	"pipeline.trigger_run",
	"runner.register",
	"runner.delete_by_token",
}

// TestFilterActionCatalog_ReadAPIUnderSafeMode_PreviewsTheWritesReadAPIReaches
// pins safe mode over the read_api narrowing on the catalog the meta surface
// registers, for both instance classes. Each of the five writes a read_api
// credential is served keeps its write classification, which is what safe
// mode keys on, answers with a preview, and carries no destructive flag any
// more, since the preview replaced the handler there was something to confirm
// for. They are real writes such a session reaches, the revocation and the
// runner deletion destructive among them, so a narrowing that ran after the
// previews, or one that rebuilt the actions it kept, would run them in a
// safe-mode session with nothing else failing.
func TestFilterActionCatalog_ReadAPIUnderSafeMode_PreviewsTheWritesReadAPIReaches(t *testing.T) {
	t.Parallel()
	for _, instance := range []struct {
		name   string
		dotcom bool
	}{{name: "self-managed"}, {name: "GitLab.com", dotcom: true}} {
		t.Run(instance.name, func(t *testing.T) {
			t.Parallel()
			base, err := SharedBaseCatalog(instance.dotcom, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
			if err != nil {
				t.Fatalf("SharedBaseCatalog() error = %v", err)
			}
			cfg := readAPIConfig()
			cfg.SafeMode = true
			filtered, _ := mustFilterCatalog(t, base, cfg)
			for _, id := range readAPIWrites {
				t.Run(string(id), func(t *testing.T) {
					assertPreviewedWrite(t, filtered, id)
				})
			}
		})
	}
}

// assertPreviewedWrite asserts a catalog serves the action as a write whose
// handler answers with a safe-mode preview and that asks for no confirmation.
func assertPreviewedWrite(t *testing.T, catalog *actioncatalog.Catalog, id actioncatalog.ActionID) {
	t.Helper()
	action, ok := catalog.Action(id)
	if !ok {
		t.Fatalf("%s is not served to a read_api credential", id)
	}
	if action.ReadOnly || action.Destructive || action.Route.Destructive {
		t.Errorf("%s is read-only %t and destructive %t (route %t); want a write whose preview asks for no confirmation",
			id, action.ReadOnly, action.Destructive, action.Route.Destructive)
	}
	result, err := action.Route.Handler(t.Context(), map[string]any{})
	if _, isPreview := result.(toolutil.SafeModePreview); err != nil || !isPreview {
		t.Errorf("%s in safe mode answered %T (error %v), want a preview", id, result, err)
	}
}

// freeCatalogWithMCP is the Free catalog with the MCP-only actions, the one
// the read_api narrowing tests filter.
func freeCatalogWithMCP(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	return catalog
}

// TestFilterActionCatalog_AdminMode_NamesEveryScopeTheTokenLacks pins what a
// credential without admin_mode is told it lacks for an action of an
// admin_mode group, and who asks for each: admin_mode, which this server
// demands before it serves the group, alone for a read; api beside it for a
// write when the token carries read_api, which GitLab requires for what the
// write sends; and admin_mode alone again for a write when the token carries
// api, which is what it used to be told to reauthorize with. admin_mode is
// never put on GitLab, which serves admin.metadata_get to any authenticated
// token.
func TestFilterActionCatalog_AdminMode_NamesEveryScopeTheTokenLacks(t *testing.T) {
	t.Parallel()
	catalog, err := BuildActionCatalog(nil, ActionCatalogOptions{Tier: edition.Free, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	admin := []string{"admin_mode"}
	cases := []struct {
		name       string
		cfg        *config.ServerConfig
		action     string
		wantGitLab []string
		wantServer []string
	}{
		{name: "a read and read_api", cfg: readAPIConfig(), action: "admin.metadata_get", wantServer: admin},
		{name: "a write and read_api", cfg: readAPIConfig(), action: "admin.settings_update", wantGitLab: []string{"api"}, wantServer: admin},
		{name: "a write and api", cfg: &config.ServerConfig{TokenScopes: []string{"api"}}, action: "admin.settings_update", wantServer: admin},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, withheld := mustFilterCatalog(t, catalog, tc.cfg)
			entry, ok := scopeEntry(withheld.ByTokenScope, tc.action)
			if !ok || !slices.Equal(entry.ByGitLab, tc.wantGitLab) || !slices.Equal(entry.ByServer, tc.wantServer) {
				t.Errorf("%s is withheld with %v by GitLab and %v by this server (named %t), want %v and %v",
					tc.action, entry.ByGitLab, entry.ByServer, ok, tc.wantGitLab, tc.wantServer)
			}
		})
	}
}

// TestFilterActionCatalog_AdminModeUnderReadOnly_FilesEveryWriteUnderTheOperator
// verifies a write of an admin_mode group in a read-only deployment is filed
// under the operator, whatever scope the credential lacks besides, for a
// read_api token and an api token alike: the group step removes it before
// read-only mode runs, and filed under the token it would send the caller to
// reauthorize for an action the deployment withholds anyway. A read of the
// group is still the token's, and names admin_mode alone.
func TestFilterActionCatalog_AdminModeUnderReadOnly_FilesEveryWriteUnderTheOperator(t *testing.T) {
	t.Parallel()
	catalog := freeCatalogWithMCP(t)
	cases := []struct {
		name string
		cfg  *config.ServerConfig
	}{
		{name: "read_api", cfg: &config.ServerConfig{ReadOnly: true, ReadAPIOnly: true, TokenScopes: []string{"read_api"}}},
		{name: "api", cfg: &config.ServerConfig{ReadOnly: true, TokenScopes: []string{"api"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, withheld := mustFilterCatalog(t, catalog, tc.cfg)
			if missing, byScope := scopeMissing(withheld.ByTokenScope, "admin.settings_update"); byScope || !slices.Contains(withheld.ByOperator, "admin.settings_update") {
				t.Errorf("admin.settings_update is withheld by the operator %t, by the token's scope %t with %v missing; want the operator alone",
					slices.Contains(withheld.ByOperator, "admin.settings_update"), byScope, missing)
			}
			if missing, ok := scopeMissing(withheld.ByTokenScope, "admin.metadata_get"); !ok || !slices.Equal(missing, []string{"admin_mode"}) {
				t.Errorf("admin.metadata_get is withheld with %v missing (named %t), want [admin_mode]", missing, ok)
			}
			if slices.Contains(withheld.ByOperator, "admin.metadata_get") {
				t.Error("admin.metadata_get, a read, is withheld by the operator")
			}
		})
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
		if action.ClassicNeed() != finegrained.ClassicAPI || len(action.Compatibility.ActionAliases) == 0 {
			continue
		}
		wantAlias = action.Compatibility.ActionAliases[0].Alias
		wantAction = string(action.ID)
		break
	}
	if wantAlias == "" {
		// A claim about this repository's own catalog, not about the
		// environment: compatibility aliases are declared today, and a catalog
		// that stopped declaring any on a mutating action would silently
		// retire the ADR-0018 refusal message this test covers.
		t.Fatal("no mutating action in the Free catalog declares a compatibility alias, so the withheld-alias report this test exists for is not exercised")
	}

	_, withheld := mustFilterCatalog(t, catalog, readAPIConfig())
	if !slices.Contains(scopeWithheldIDs(withheld.ByTokenScope), wantAlias) {
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
	// The populated cases are the ones that can go wrong: a missing "after"
	// carries no actions, so a diff that ran anyway would report every action
	// of "before" as withheld and tell the caller the whole catalog was taken
	// from them.
	populated := scopeFilterTestCatalog(t)
	if got := RemovedActionKeys(populated, nil); got != nil {
		t.Errorf("RemovedActionKeys(populated, nil) = %v, want nothing claimed rather than every action of the catalog", got)
	}
	if got := RemovedActionKeys(nil, populated); got != nil {
		t.Errorf("RemovedActionKeys(nil, populated) = %v, want nothing claimed", got)
	}
}

// TestExcludeFromCatalog_LogsExactlyWhatItRemoved covers the count in the one
// line an operator sees about their own --exclude-tools configuration.
//
// The count is the whole point of the line. Removal already worked when it was
// added; what did not was telling a working exclusion apart from one that
// matched nothing, because the only figure logged came from the registered-tool
// filter and the default surface registers two tools, neither of them an
// exclusion target. A count that is not the difference between the two catalogs
// puts that back, and a line logged when nothing was removed says an exclusion
// happened that did not.
func TestExcludeFromCatalog_LogsExactlyWhatItRemoved(t *testing.T) {
	catalog := excludeCountingTestCatalog(t)

	t.Run("a matching entry reports the actions it took away", func(t *testing.T) {
		output := captureSlogOutput(t)

		filtered := ExcludeFromCatalog(catalog, []string{"gitlab_zzz_exclude_first"})

		if got, want := filtered.CountActions(), catalog.CountActions()-1; got != want {
			t.Fatalf("filtered catalog has %d actions, want %d", got, want)
		}
		if !strings.Contains(output.String(), `"excluded":1`) {
			t.Errorf("startup log = %s, want the one removed action counted as the difference between %d and %d",
				output.String(), catalog.CountActions(), filtered.CountActions())
		}
	})

	t.Run("an entry that matches nothing reports no removal", func(t *testing.T) {
		output := captureSlogOutput(t)

		ExcludeFromCatalog(catalog, []string{"gitlab_zzz_exclude_absent"})

		if strings.Contains(output.String(), "excluded catalog actions by configuration") {
			t.Errorf("startup log = %s, want no removal line when the entry named nothing", output.String())
		}
	})
}

// TestExcludeFromCatalog_StandaloneEntries_AreNotReportedAsNamingNothing
// covers the warning an operator reads about their own --exclude-tools file.
//
// The standalone utilities are in none of the catalogs this function
// filters, so an entry naming one was always unmatched here, and the warning
// used to name it beside the dead entries with a note excusing them all as
// filtered elsewhere. That was false of the canonical ID on every surface and
// made every standalone entry, working or not, read the same. The warning now
// names only what neither the catalog nor a standalone utility answers, in
// the operator's order, and is absent when that is nothing.
//
// Sequential: the logger it captures is process-wide.
func TestExcludeFromCatalog_StandaloneEntries_AreNotReportedAsNamingNothing(t *testing.T) {
	catalog := excludeCountingTestCatalog(t)

	cases := []struct {
		name    string
		exclude []string
		// wantEntries is the entries value the one warning carries, or ""
		// when no warning is due.
		wantEntries string
	}{
		{
			name:        "every standalone spelling beside one dead entry",
			exclude:     []string{"interactive.issue_create", "gitlab_interactive", "gitlab_discover_project", "gitlab_zzz_absent"},
			wantEntries: "gitlab_zzz_absent",
		},
		{
			name:    "only standalone entries",
			exclude: []string{"interactive.issue_create", "gitlab_interactive", "discover_project.resolve"},
		},
		{
			name:    "a catalog entry and a standalone one",
			exclude: []string{"gitlab_zzz_exclude_first", "interactive.mr_create"},
		},
		{
			name:        "dead entries keep the operator's order around a live one",
			exclude:     []string{"gitlab_zzz_absent", "gitlab_zzz_exclude_first", "other.absent"},
			wantEntries: "gitlab_zzz_absent, other.absent",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output := captureSlogOutput(t)

			ExcludeFromCatalog(catalog, tc.exclude)

			logged := output.String()
			warnings := strings.Count(logged, `"level":"WARN"`)
			if tc.wantEntries == "" {
				if warnings != 0 {
					t.Errorf("log = %s, want no warning when every entry named something", logged)
				}
				return
			}
			if warnings != 1 || !strings.Contains(logged, `"entries":"`+tc.wantEntries+`"`) {
				t.Errorf("log = %s, want one warning naming exactly %q", logged, tc.wantEntries)
			}
			if strings.Contains(logged, `"note"`) {
				t.Errorf("log = %s, want no note excusing the standalone entries", logged)
			}
		})
	}
}

// excludeCountingTestCatalog returns a catalog of two one-action groups, so a
// count of what an exclusion removed differs from the count of what it kept and
// from their sum.
func excludeCountingTestCatalog(t *testing.T) *actioncatalog.Catalog {
	t.Helper()
	catalog := actioncatalog.NewCatalog()
	for _, toolName := range []string{"gitlab_zzz_exclude_first", "gitlab_zzz_exclude_second"} {
		action := actioncatalog.Action{
			Name:         "list",
			OwnerPackage: "tools",
			Route:        toolutil.ActionRoute{InputSchema: map[string]any{"type": "object"}},
		}
		options := actioncatalog.GroupOptions{
			ToolName:     toolName,
			OwnerPackage: "tools",
			SurfaceKind:  actioncatalog.SurfaceKindMetaGroup,
		}
		if err := catalog.AddAction(toolName, action, options); err != nil {
			t.Fatalf("AddAction(%s) error = %v", toolName, err)
		}
	}
	return catalog
}

// captureSlogOutput sends the default logger into a buffer for the rest of the
// test and restores it afterwards.
//
// The logger is process-wide, so a test that captures it must not run in
// parallel with one that logs. Every caller here is sequential, which the Go
// runner keeps apart from the parallel tests of this package: those are paused
// until the sequential ones have all run.
func captureSlogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buffer
}

// TestExcludeFromCatalog_NothingToExclude_ReturnsTheSameCatalog covers the
// early exit: with no patterns the catalog is handed back untouched rather
// than copied and counted.
func TestExcludeFromCatalog_NothingToExclude_ReturnsTheSameCatalog(t *testing.T) {
	t.Parallel()

	catalog := actioncatalog.NewCatalog()
	if ExcludeFromCatalog(catalog, nil) != catalog {
		t.Error("ExcludeFromCatalog with no patterns returned a different catalog")
	}
}

// TestFilterActionCatalog_ScopeFilterFails_ReturnsTheErrorAndNoCatalog covers
// the one path out of the filter that is not a narrowed catalog.
//
// A half-filtered catalog is worse than none: the caller would register a
// surface missing whichever domain the rebuild stopped at, with nothing said
// about it. So the error is returned and the catalog is not, and the withheld
// bookkeeping is empty rather than partial.
func TestFilterActionCatalog_ScopeFilterFails_ReturnsTheErrorAndNoCatalog(t *testing.T) {
	restore := failAddFilteredGroup(t)
	defer restore()

	catalog, withheld, err := FilterActionCatalog(scopeFilterTestCatalog(t), &config.ServerConfig{TokenScopes: []string{"api"}})

	if err == nil {
		t.Fatal("FilterActionCatalog() error = nil, want the rebuild failure")
	}
	if catalog != nil {
		t.Errorf("catalog = %+v, want none when the rebuild failed", catalog)
	}
	if len(withheld.ByTokenScope) != 0 || len(withheld.ByOperator) != 0 || len(withheld.ExcludedByName) != 0 {
		t.Errorf("withheld = %+v, want nothing recorded when the rebuild failed", withheld)
	}
}
