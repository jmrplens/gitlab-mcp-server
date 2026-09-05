// dynamic_catalog_test.go pins the two properties the server and the e2e suite
// both rely on: the filter runs before the standalone actions are added, and
// what read-only mode removed is reported with its cause.
package dynamiccatalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/dynamic"
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

// healthyGitLab answers every request well enough for the health action to
// report the URL it reached.
func healthyGitLab() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"17.0.0","revision":"abc123","id":1,"username":"tester"}`))
	})
}

// TestBuild_SharesOneCatalogAcrossClients verifies two clients with one
// configuration get catalogs bound from one shared origin, that the
// standalone actions travel with it and carry a binder, and that each copy
// runs its actions under its own client.
func TestBuild_SharesOneCatalogAcrossClients(t *testing.T) {
	clientA := testutil.NewTestClient(t, healthyGitLab())
	clientB := testutil.NewTestClient(t, healthyGitLab())
	cfg := &config.ServerConfig{Tier: edition.Free}

	catalogA, _, err := Build(clientA, cfg)
	if err != nil {
		t.Fatalf("Build(A) error = %v", err)
	}
	catalogB, _, err := Build(clientB, cfg)
	if err != nil {
		t.Fatalf("Build(B) error = %v", err)
	}
	if catalogA == catalogB || catalogA.SharedOrigin() == nil || catalogA.SharedOrigin() != catalogB.SharedOrigin() {
		t.Fatal("the two clients did not get distinct catalogs bound from one shared origin")
	}
	standalone, ok := catalogA.Action("discover_project.resolve")
	if !ok || standalone.Route.Bind == nil {
		t.Fatalf("discover_project.resolve = %+v, %t; want a standalone action with a binder", standalone.Route, ok)
	}
	urls := make(map[string]struct{})
	for name, catalog := range map[string]*actioncatalog.Catalog{"A": catalogA, "B": catalogB} {
		t.Run("client "+name, func(t *testing.T) {
			action, found := catalog.Action("server.status")
			if !found {
				t.Fatal("server.status missing from the catalog")
			}
			result, runErr := action.Route.Handler(context.Background(), map[string]any{})
			if runErr != nil {
				t.Fatalf("server.status error = %v", runErr)
			}
			encoded, _ := json.Marshal(result)
			var output struct {
				GitLabURL string `json:"gitlab_url"`
			}
			_ = json.Unmarshal(encoded, &output)
			urls[output.GitLabURL] = struct{}{}
		})
	}
	if len(urls) != 2 {
		t.Fatalf("server.status reached %v, want each client's own instance", urls)
	}
}

// TestBuild_FailuresAreReportedWithTheirStep verifies each of the three
// assembly steps names itself when it fails, through the seams that stand in
// for failures no real input can cause, under keys no other test has built.
func TestBuild_FailuresAreReportedWithTheirStep(t *testing.T) {
	forced := errors.New("forced catalog failure")
	fresh := func(suffix string) *config.ServerConfig {
		return &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"fail-" + t.Name() + "-" + suffix}}
	}
	cases := []struct {
		name    string
		arrange func(t *testing.T)
		want    string
	}{
		{
			name: "base catalog",
			arrange: func(t *testing.T) {
				t.Helper()
				original := sharedBaseCatalog
				t.Cleanup(func() { sharedBaseCatalog = original })
				sharedBaseCatalog = func(bool, gitlabtools.ActionCatalogOptions) (*actioncatalog.Catalog, error) { return nil, forced }
			},
			want: "build action catalog",
		},
		{
			name: "filter",
			arrange: func(t *testing.T) {
				t.Helper()
				original := filterActionCatalog
				t.Cleanup(func() { filterActionCatalog = original })
				filterActionCatalog = func(*actioncatalog.Catalog, *config.ServerConfig) (*actioncatalog.Catalog, gitlabtools.WithheldActions, error) {
					return nil, gitlabtools.WithheldActions{}, forced
				}
			},
			want: "filter dynamic action catalog",
		},
		{
			name: "standalone actions",
			arrange: func(t *testing.T) {
				t.Helper()
				original := addStandaloneCatalog
				t.Cleanup(func() { addStandaloneCatalog = original })
				addStandaloneCatalog = func(*actioncatalog.Catalog, *gitlabclient.Client, dynamictools.StandaloneOptions) (*actioncatalog.Catalog, error) {
					return nil, forced
				}
			},
			want: "add standalone dynamic actions",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			_, _, err := Build(nil, fresh(tc.name))
			if !errors.Is(err, forced) || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Build() error = %v, want the forced failure carrying %q", err, tc.want)
			}
		})
	}
}
