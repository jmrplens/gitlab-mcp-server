// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	// The guided flows join after the filter, and read-only mode is applied
	// to them there, so asking for one is answered with its cause too.
	if !slices.Contains(withheld.ByOperator, "interactive.issue_create") {
		t.Errorf("withheld.ByOperator = %v, want it to name the guided issue creation flow", withheld.ByOperator)
	}
	if _, ok := catalog.Action("discover_project.resolve"); !ok {
		t.Error("discover_project.resolve, which reads, did not survive read-only mode")
	}
	if len(withheld.ByTokenScope) != 0 {
		t.Errorf("withheld.ByTokenScope = %v, want nothing: no credential narrowed this catalog", withheld.ByTokenScope)
	}
}

// TestBuild_RecordsWhatTheTierWithheldAndNeverAnExcludedAction verifies the
// dynamic catalog carries the actions the licensing tier removed beside the
// ones the other filters withheld (issue 956), so the registry built from it
// can answer a call for one with the tier it needs, that an action the
// operator excluded by name is not among them (AUT-004), and that at Ultimate
// nothing is recorded. A Premium action is asked for through the registry the
// way cmd/server builds it, and the answer names the tier rather than calling
// the action unknown.
func TestBuild_RecordsWhatTheTierWithheldAndNeverAnExcludedAction(t *testing.T) {
	t.Parallel()
	byID := func(withheld []actioncatalog.TierWithheld) map[string]string {
		ids := make(map[string]string, len(withheld))
		for _, entry := range withheld {
			ids[entry.ID] = entry.Edition
		}
		return ids
	}

	free, withheld, err := Build(nil, &config.ServerConfig{Tier: edition.Free})
	if err != nil {
		t.Fatalf("Build(Free) error = %v", err)
	}
	if got := byID(withheld.ByTier)["merge_train.list_project"]; got != "premium" {
		t.Errorf("ByTier[merge_train.list_project] = %q, want premium: the merge train group is Premium", got)
	}
	if _, kept := byID(withheld.ByTier)["issue.list"]; kept {
		t.Error("ByTier names issue.list, which the Free tier serves")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "dynamiccatalog-test", Version: "0"}, nil)
	dynamictools.RegisterCatalogFindExecuteTools(server, free, dynamictools.WithTierWithheld(withheld.ByTier, edition.Free, true))
	text := executeError(t, server, "merge_train.list_project")
	if !strings.Contains(text, "it needs GitLab Premium or Ultimate") || strings.Contains(text, "unknown action") {
		t.Errorf("execute merge_train.list_project at Free = %q, want the tier it needs rather than an unknown action", text)
	}

	_, excluded, err := Build(nil, &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"gitlab_merge_train"}})
	if err != nil {
		t.Fatalf("Build(Free, excluding gitlab_merge_train) error = %v", err)
	}
	for id := range byID(excluded.ByTier) {
		if strings.HasPrefix(id, "merge_train.") {
			t.Errorf("ByTier names %s, which the operator excluded", id)
		}
	}
	if len(excluded.ByTier) == 0 {
		t.Error("ByTier is empty, want every other action the Free tier removed")
	}

	_, ultimate, err := Build(nil, &config.ServerConfig{Tier: edition.Ultimate})
	if err != nil {
		t.Fatalf("Build(Ultimate) error = %v", err)
	}
	if len(ultimate.ByTier) != 0 {
		t.Errorf("ByTier at Ultimate = %d entries, want none", len(ultimate.ByTier))
	}
}

// TestBuild_TheTierBookkeepingFollowsTheSession holds the arguments Build hands
// the tier's bookkeeping, which every test above passes for a self-managed Free
// session with nothing else narrowed. The instance class decides which actions
// exist at all: Orbit's group is contributed only for a GitLab.com client, so
// a GitLab.com session at Free has to record orbit.query as needing Premium,
// where a self-managed one has nothing to record for it. The rest of the
// configuration decides what else each entry says: a read-only deployment
// records that read-only mode would withhold a Premium write too, so the
// answer to it does not send the model to pin a tier that would only uncover
// the next refusal.
func TestBuild_TheTierBookkeepingFollowsTheSession(t *testing.T) {
	t.Parallel()
	entry := func(withheld []actioncatalog.TierWithheld, id string) (actioncatalog.TierWithheld, bool) {
		for _, candidate := range withheld {
			if candidate.ID == id {
				return candidate, true
			}
		}
		return actioncatalog.TierWithheld{}, false
	}

	_, dotcom, err := Build(gitlabtools.UnboundClient(true), &config.ServerConfig{Tier: edition.Free})
	if err != nil {
		t.Fatalf("Build(GitLab.com, Free) error = %v", err)
	}
	if orbit, ok := entry(dotcom.ByTier, "orbit.query"); !ok || orbit.Edition != "premium" {
		t.Errorf("ByTier[orbit.query] on GitLab.com at Free = %+v (recorded %t), want it recorded as needing premium", orbit, ok)
	}
	_, selfManaged, err := Build(nil, &config.ServerConfig{Tier: edition.Free})
	if err != nil {
		t.Fatalf("Build(self-managed, Free) error = %v", err)
	}
	if _, ok := entry(selfManaged.ByTier, "orbit.query"); ok {
		t.Error("ByTier names orbit.query on a self-managed instance, which carries no Orbit group at any tier")
	}

	_, readOnly, err := Build(nil, &config.ServerConfig{Tier: edition.Free, ReadOnly: true})
	if err != nil {
		t.Fatalf("Build(Free, read-only) error = %v", err)
	}
	if add, ok := entry(readOnly.ByTier, "merge_train.add"); !ok || !add.ReadOnlyMode {
		t.Errorf("ByTier[merge_train.add] on a read-only Free deployment = %+v (recorded %t), want read-only mode named", add, ok)
	}
	if list, ok := entry(readOnly.ByTier, "merge_train.list_project"); !ok || list.ReadOnlyMode {
		t.Errorf("ByTier[merge_train.list_project] on a read-only Free deployment = %+v (recorded %t), want a read recorded without read-only mode", list, ok)
	}
}

// executeError runs gitlab_execute_action for action through an in-memory
// client of server and returns the text of the tool error it answers with.
func executeError(t *testing.T, server *mcp.Server, action string) string {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "dynamiccatalog-client", Version: "0"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: dynamictools.ExecuteActionToolName, Arguments: map[string]any{"action": action, "params": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !result.IsError || len(result.Content) == 0 {
		t.Fatalf("CallTool() = %+v, want a tool error", result)
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		t.Fatalf("CallTool() content = %#v, want text", result.Content[0])
	}
	return text.Text
}

// scopeKeys lists the keys of the token-scope bookkeeping.
func scopeKeys(withheld []actioncatalog.ScopeWithheld) []string {
	keys := make([]string, 0, len(withheld))
	for _, entry := range withheld {
		keys = append(keys, entry.ID)
	}
	return keys
}

// TestBuild_ACacheHitStillCarriesTheScopeCause verifies the bookkeeping a
// second pool entry receives. A read_api credential is admitted and served
// what read_api reaches, and the dynamic surface answers a request for one of
// the removed actions with the cause instead of "unknown action, did you
// mean": the older message listed real read-only actions, from which a model
// concluded the server lacked the capability rather than that the credential
// was narrow. A guided flow, which joins after the filter, is withheld with
// its cause too, and a write read_api reaches is kept.
//
// The second Build is the point. It is a cache hit, and the withheld lists
// come back beside the cached catalog rather than from the build that filled
// it; a cache that returned the catalog and dropped the bookkeeping would pass
// every test that only ever built once.
func TestBuild_ACacheHitStillCarriesTheScopeCause(t *testing.T) {
	t.Parallel()

	narrowed := func() *config.ServerConfig {
		return &config.ServerConfig{
			Tier:         edition.Free,
			ReadAPIOnly:  true,
			TokenScopes:  []string{"read_api"},
			ExcludeTools: []string{"scope-" + t.Name()},
		}
	}
	first, firstWithheld, err := Build(nil, narrowed())
	if err != nil {
		t.Fatalf("Build(first) error = %v", err)
	}
	second, secondWithheld, err := Build(nil, narrowed())
	if err != nil {
		t.Fatalf("Build(second) error = %v", err)
	}
	if first.SharedOrigin() == nil || first.SharedOrigin() != second.SharedOrigin() {
		t.Fatal("the second Build did not hit the cache, so it proves nothing about a cached one")
	}
	for name, withheld := range map[string]gitlabtools.WithheldActions{"first build": firstWithheld, "cache hit": secondWithheld} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			keys := scopeKeys(withheld.ByTokenScope)
			if !slices.Contains(keys, "issue.create") || !slices.Contains(keys, "interactive.issue_create") {
				t.Errorf("withheld.ByTokenScope = %v, want it to name issue.create and interactive.issue_create as removed by the credential", keys)
			}
			if len(withheld.ByOperator) != 0 {
				t.Errorf("withheld.ByOperator = %v, want nothing: the credential narrowed this catalog, not the operator", withheld.ByOperator)
			}
			if slices.Contains(keys, "issue.list") || slices.Contains(keys, "package.download") {
				t.Errorf("withheld.ByTokenScope = %v, want what read_api reaches kept rather than withheld", keys)
			}
		})
	}
	for _, kept := range []actioncatalog.ActionID{"issue.list", "package.download"} {
		t.Run(string(kept), func(t *testing.T) {
			if _, ok := second.Action(kept); !ok {
				t.Errorf("%s is missing from the cached read_api catalog", kept)
			}
		})
	}
	if _, ok := second.Action("issue.create"); ok {
		t.Error("issue.create survived in the cached read_api catalog")
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

// TestBuild_SafeModeIsAppliedOverTheCompleteCatalogRatherThanInsideTheFilter
// verifies the ordering this package exists to establish: the filter is handed
// a configuration with safe mode switched off, so the catalog the standalone
// actions join still carries its real write handlers, and the previews are
// applied once afterwards over everything.
//
// It has to be stated at the two ends of that ordering, because the middle is
// invisible. WithSafeModePreviews replaces a route whatever was in it, so
// applying it twice produces exactly what applying it once produces: letting
// the filter preview as well changes no answer any other test asks, and
// deleting the line that stops it leaves the ordering resting on an
// idempotence that belongs to another package. That is worth a test rather
// than a comment because the day the rewrite stops being idempotent is the day
// the standalone writes go back to being previewed by a second pass over a
// catalog already wrapped by the first, which is the defect this package was
// split out to fix.
//
// A still-destructive action is the discriminator rather than a handler call:
// the preview rewrite clears Destructive on every write it replaces, and
// calling a real write handler with no parameters is the very thing
// TestBuild_SafeModePreviewsCoverTheStandaloneActions relies on failing.
func TestBuild_SafeModeIsAppliedOverTheCompleteCatalogRatherThanInsideTheFilter(t *testing.T) {
	original := addStandaloneCatalog
	t.Cleanup(func() { addStandaloneCatalog = original })
	var joined *actioncatalog.Catalog
	addStandaloneCatalog = func(catalog *actioncatalog.Catalog, client *gitlabclient.Client, opts dynamictools.StandaloneOptions) (*actioncatalog.Catalog, error) {
		joined = catalog
		return original(catalog, client, opts)
	}

	built, _, err := Build(nil, &config.ServerConfig{
		Tier:         edition.Free,
		SafeMode:     true,
		ExcludeTools: []string{"ordering-" + t.Name()},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if joined == nil {
		t.Fatal("the standalone step never ran, so this test saw no filtered catalog")
	}
	if len(destructiveActionIDs(joined)) == 0 {
		t.Error("the filtered catalog reached the standalone step already previewed; safe mode belongs over the complete catalog, not inside the filter")
	}
	if surviving := destructiveActionIDs(built); len(surviving) != 0 {
		t.Errorf("the built catalog still marks %v destructive, so the final safe-mode pass did not reach every write", surviving)
	}
}

// TestBuild_SafeModeOverReadAPI_PreviewsTheWritesReadAPIReaches verifies safe
// mode and the read_api narrowing together on the dynamic catalog, for both
// instance classes. The five actions the catalog classifies as writes that a
// read_api credential is served (ADR-0026) are kept, keep their write
// classification, answer with a preview, and carry no destructive flag, so
// execute asks no confirmation for a call that changes nothing. They are real
// writes such a session reaches, the revocation and the runner deletion
// destructive among them, so a narrowing applied after the previews, or one
// that rebuilt the actions it kept, would run them in safe mode with nothing
// else failing.
func TestBuild_SafeModeOverReadAPI_PreviewsTheWritesReadAPIReaches(t *testing.T) {
	t.Parallel()
	writes := []actioncatalog.ActionID{
		"package.download", "access.token_personal_revoke_self", "pipeline.trigger_run", "runner.register", "runner.delete_by_token",
	}
	for _, instance := range []struct {
		name   string
		dotcom bool
	}{{name: "self-managed"}, {name: "GitLab.com", dotcom: true}} {
		t.Run(instance.name, func(t *testing.T) {
			t.Parallel()
			built, _, err := Build(gitlabtools.UnboundClient(instance.dotcom), &config.ServerConfig{
				Tier: edition.Free, SafeMode: true, ReadAPIOnly: true, TokenScopes: []string{"read_api"},
			})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			for _, id := range writes {
				t.Run(string(id), func(t *testing.T) {
					action, ok := built.Action(id)
					if !ok {
						t.Fatalf("%s is not served to a read_api credential", id)
					}
					if action.ReadOnly || action.Destructive || action.Route.Destructive {
						t.Errorf("%s is read-only %t and destructive %t (route %t); want a write whose preview asks for no confirmation",
							id, action.ReadOnly, action.Destructive, action.Route.Destructive)
					}
					result, callErr := action.Route.Handler(t.Context(), map[string]any{})
					if _, isPreview := result.(toolutil.SafeModePreview); callErr != nil || !isPreview {
						t.Errorf("%s in safe mode answered %T (error %v), want a preview", id, result, callErr)
					}
				})
			}
		})
	}
}

// TestBuild_ReadAPICatalog_AnnotatesExecuteAsAWrite pins the annotation
// change ADR-0026 records as NEG-005 on the default surface: a read_api
// credential is served five actions the catalog classifies as writes, so the
// execute tool registered over its catalog is annotated neither read-only nor
// non-destructive, and a client that approves read-only tools without asking
// asks before every call of such a session. The same credential in a
// read-only deployment is served reads alone, and its execute tool is
// read-only again.
func TestBuild_ReadAPICatalog_AnnotatesExecuteAsAWrite(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		readOnly     bool
		wantReadOnly bool
	}{
		{name: "read_api", wantReadOnly: false},
		{name: "read_api in a read-only deployment", readOnly: true, wantReadOnly: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			built, withheld, err := Build(nil, &config.ServerConfig{
				Tier: edition.Free, ReadOnly: tc.readOnly, ReadAPIOnly: true, TokenScopes: []string{"read_api"},
			})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			server := mcp.NewServer(&mcp.Implementation{Name: "dynamiccatalog-test", Version: "0"}, nil)
			dynamictools.RegisterCatalogFindExecuteTools(server, built,
				dynamictools.WithWithheldActions(withheld.ByTokenScope, withheld.ByOperator))
			tools, err := toolutil.ListRegisteredTools(t.Context(), server, "dynamiccatalog-test")
			if err != nil {
				t.Fatalf("ListRegisteredTools() error = %v", err)
			}
			index := slices.IndexFunc(tools, func(tool *mcp.Tool) bool { return tool.Name == dynamictools.ExecuteActionToolName })
			if index < 0 {
				t.Fatalf("%s is not registered", dynamictools.ExecuteActionToolName)
			}
			annotations := tools[index].Annotations
			if annotations == nil || annotations.DestructiveHint == nil {
				t.Fatalf("%s annotations = %+v, want both hints set", dynamictools.ExecuteActionToolName, annotations)
			}
			if annotations.ReadOnlyHint != tc.wantReadOnly || *annotations.DestructiveHint == tc.wantReadOnly {
				t.Errorf("%s is read-only %t and destructive %t, want read-only %t and destructive %t",
					dynamictools.ExecuteActionToolName, annotations.ReadOnlyHint, *annotations.DestructiveHint, tc.wantReadOnly, !tc.wantReadOnly)
			}
		})
	}
}

// destructiveActionIDs names every action of catalog still marked destructive.
// The safe-mode rewrite clears the flag on each write it replaces, so an empty
// answer means every write in the catalog is a preview.
func destructiveActionIDs(catalog *actioncatalog.Catalog) []string {
	var ids []string
	for _, action := range catalog.Actions() {
		if action.Destructive {
			ids = append(ids, string(action.ID))
		}
	}
	return ids
}

// TestBuild_DoesNotShareTheMetaSurfacesCatalogEntry verifies the dynamic
// catalog is cached under a name of its own.
//
// Both surfaces name one process-wide cache with the same tier, instance class
// and narrowing, and differ only in the prefix each puts in front of it. A
// dynamic catalog stored under the meta surface's name would simply be handed
// to whichever surface asked second, and neither would refuse it: the two are
// assembled to be interchangeable everywhere except in the standalone actions
// only the dynamic one carries. So the prefix is checked by what depends on
// it, in both directions: the dynamic catalog routes discover_project.resolve
// and the meta one, which registers its standalone tools outside the catalog,
// does not.
func TestBuild_DoesNotShareTheMetaSurfacesCatalogEntry(t *testing.T) {
	t.Parallel()

	narrowing := func() *config.ServerConfig {
		return &config.ServerConfig{Tier: edition.Free, ExcludeTools: []string{"surface-" + t.Name()}}
	}
	dynamic, _, err := Build(nil, narrowing())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	meta, _, err := gitlabtools.SharedMetaCatalog(nil, narrowing())
	if err != nil {
		t.Fatalf("SharedMetaCatalog() error = %v", err)
	}
	if dynamic.SharedOrigin() == nil || meta.SharedOrigin() == nil {
		t.Fatal("one of the two catalogs was not shared, so this proves nothing about the cache")
	}
	if dynamic.SharedOrigin() == meta.SharedOrigin() {
		t.Fatal("the dynamic and the meta surface were served one cache entry for the same narrowing")
	}
	if _, ok := dynamic.Action("discover_project.resolve"); !ok {
		t.Error("discover_project.resolve is missing from the dynamic catalog")
	}
	if _, ok := meta.Action("discover_project.resolve"); ok {
		t.Error("discover_project.resolve reached the meta catalog, which registers its standalone tools outside it")
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

// TestBuild_RefusesACatalogAnAssemblyStepLeftUnbindable verifies the two
// validations the assembly runs on what it built, which are the reason the
// step after buildActionCatalog cannot quietly add an action nothing can bind.
//
// Both are driven through the seams, because no real input reaches them: the
// standalone specs are fixed and the safe-mode rewrite installs a binder of
// its own. A route whose handler is installed directly is what each seam
// returns, since that is the shape an action would have if a future step
// forgot the binder, and it would refuse every call for every credential of
// the configuration.
func TestBuild_RefusesACatalogAnAssemblyStepLeftUnbindable(t *testing.T) {
	unbindable := func(t *testing.T) *actioncatalog.Catalog {
		t.Helper()
		catalog := actioncatalog.NewCatalog()
		group := actioncatalog.NewGroup(actioncatalog.GroupOptions{ToolName: "gitlab_widget", BaseDomain: "widget"})
		group.SetAction(actioncatalog.Action{Name: "get", Route: toolutil.ActionRoute{
			Handler:     func(context.Context, map[string]any) (any, error) { return map[string]any{}, nil },
			InputSchema: map[string]any{"type": "object"},
		}})
		if err := catalog.AddGroup(group); err != nil {
			t.Fatalf("AddGroup() error = %v", err)
		}
		return catalog
	}
	cases := []struct {
		name     string
		safeMode bool
		arrange  func(t *testing.T)
		want     string
	}{
		{
			name: "after the standalone actions join",
			arrange: func(t *testing.T) {
				t.Helper()
				original := addStandaloneCatalog
				t.Cleanup(func() { addStandaloneCatalog = original })
				addStandaloneCatalog = func(*actioncatalog.Catalog, *gitlabclient.Client, dynamictools.StandaloneOptions) (*actioncatalog.Catalog, error) {
					return unbindable(t), nil
				}
			},
			want: "validate dynamic action catalog",
		},
		{
			name:     "after the safe-mode rewrite",
			safeMode: true,
			arrange: func(t *testing.T) {
				t.Helper()
				original := safeModePreviews
				t.Cleanup(func() { safeModePreviews = original })
				safeModePreviews = func(*actioncatalog.Catalog) *actioncatalog.Catalog { return unbindable(t) }
			},
			want: "validate safe-mode dynamic action catalog",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			cfg := &config.ServerConfig{
				Tier:         edition.Free,
				SafeMode:     tc.safeMode,
				ExcludeTools: []string{"unbindable-" + t.Name()},
			}
			_, _, err := Build(nil, cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "has no binder") {
				t.Errorf("Build() error = %v, want %q naming the action with no binder", err, tc.want)
			}
		})
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
		{
			name: "tier bookkeeping",
			arrange: func(t *testing.T) {
				t.Helper()
				original := tierWithheld
				t.Cleanup(func() { tierWithheld = original })
				tierWithheld = func(bool, *config.ServerConfig) ([]actioncatalog.TierWithheld, error) { return nil, forced }
			},
			want: "record what the tier withheld",
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
