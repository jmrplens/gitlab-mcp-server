package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/mcpsurface"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamiccatalog"
)

// tiers are the three licensing tiers in ascending order. Every build of the
// catalog is made once for each, and an action's tier is the first that
// serves it.
var tiers = []edition.Tier{edition.Free, edition.Premium, edition.Ultimate}

// tierNames are the tiers as GitLab names them, which is the same in every
// language this reference is written in.
var tierNames = [...]string{edition.Free: "Free", edition.Premium: "Premium", edition.Ultimate: "Ultimate"}

// build is one catalog as the server assembles it for an instance class and a
// tier.
type build struct {
	dotcom  bool
	tier    edition.Tier
	catalog *actioncatalog.Catalog
}

// buildCatalog is dynamiccatalog.Build behind a variable. It assembles the
// catalog compiled into this binary with a configuration that narrows nothing,
// so only a test can make it fail.
var buildCatalog = dynamiccatalog.Build

// defaultBuilds builds the catalog the default dynamic surface serves, for a
// self-managed instance and for GitLab.com at each tier, in ascending tier
// order: a later build of an action is the wider one, which is what its
// parameters are read from.
func defaultBuilds() ([]build, error) {
	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()
	clients := []*gitlabclient.Client{selfManaged, mcpsurface.NewGitLabComClient()}
	var builds []build
	for _, tier := range tiers {
		for _, client := range clients {
			catalog, _, err := buildCatalog(client, &config.ServerConfig{Tier: tier})
			if err != nil {
				return nil, fmt.Errorf("build the %s catalog (GitLab.com %t): %w", tier, client.IsGitLabDotCom(), err)
			}
			builds = append(builds, build{dotcom: client.IsGitLabDotCom(), tier: tier, catalog: catalog})
		}
	}
	return builds, nil
}

// surfaceNames is the set of tool names the meta and the individual surfaces
// register, across both instance classes, at Ultimate.
type surfaceNames struct {
	meta       map[string]bool
	individual map[string]bool
}

// defaultSurfaces lists the meta and individual surfaces as a client receives
// them, for a self-managed instance and for GitLab.com.
func defaultSurfaces() surfaceNames {
	selfManaged, cleanup := mcpsurface.NewStubClient()
	defer cleanup()
	names := surfaceNames{meta: map[string]bool{}, individual: map[string]bool{}}
	for _, client := range []*gitlabclient.Client{selfManaged, mcpsurface.NewGitLabComClient()} {
		addNames(names.meta, mcpsurface.MetaTools(client, edition.Ultimate))
		addNames(names.individual, mcpsurface.IndividualTools(client, edition.Ultimate))
	}
	return names
}

// addNames records the name of every listed tool.
func addNames(into map[string]bool, listed []*mcp.Tool) {
	for _, tool := range listed {
		into[tool.Name] = true
	}
}

// reference is the whole catalog as the pages describe it.
type reference struct {
	groups []*refGroup
	// served counts every action each build serves, indexed like
	// refGroup.served.
	served [2][3]int
	// version is the GitLab release the token requirements were recorded
	// from, as the fine-grained table names it.
	version string
}

// actionCount is how many distinct actions any build serves.
func (r reference) actionCount() int {
	count := 0
	for _, group := range r.groups {
		count += len(group.actions)
	}
	return count
}

// refGroup is one catalog group: one meta-tool on the meta surface, and one
// page of the reference.
type refGroup struct {
	tool string
	slug string
	// standalone is a group whose actions are tools of their own on the meta
	// and individual surfaces (project discovery, the guided flows), rather
	// than the actions of one meta-tool.
	standalone   bool
	scopes       []string
	capabilities []string
	// served is how many of the group's actions each build serves, indexed by
	// instance class (self-managed, GitLab.com) and tier.
	served  [2][3]int
	actions []*refAction
}

// refAction is one action as a page describes it.
type refAction struct {
	id         string
	name       string
	standalone bool
	// metaTool is the tool the meta surface reaches the action through: the
	// group's, or for a standalone group the action's own.
	metaTool    string
	individual  string
	tier        edition.Tier
	dotcomOnly  bool
	readOnly    bool
	destructive bool
	idempotent  bool
	description string
	params      []param
	// oneOf lists the alternative parameter sets of which the schema requires
	// at least one, beside the parameters it always requires, each holding
	// what it adds to them (actioncatalog.RequiredParamAlternatives).
	oneOf [][]string

	// domain is the action ID's prefix, which is also the heading the
	// fine-grained permissions page files the action under.
	domain string
	// classic is the scope a classic or OAuth token needs for the requests
	// the action sends, and scopes the ones its group demands beside it.
	classic finegrained.ClassicScope
	scopes  []string
	// oauthRefused are the routes the action sends that GitLab refuses to an
	// OAuth token, and oauthRefusedEveryWay is set when no input lets the
	// action run without one.
	oauthRefused         []string
	oauthRefusedEveryWay bool
	// fineGrained is what a fine-grained token needs for it.
	fineGrained *finegrained.Description

	// latest is the widest build's copy of the action, and paramTiers the
	// lowest tier at which each of its parameters is served. Both are read
	// once every build has been seen.
	latest     actioncatalog.Action
	paramTiers map[string]edition.Tier
}

// classIndex is the index of an instance class in the served counts.
func classIndex(dotcom bool) int {
	if dotcom {
		return 1
	}
	return 0
}

// assembly is the state assemble folds the builds into.
type assembly struct {
	ref     reference
	scopes  map[string][]string
	groups  map[string]*refGroup
	actions map[string]*refAction
}

// assemble folds the builds into the groups the pages describe, holds every
// tool name a page will print to the surface that registers it, and reads
// what each action needs of a token from grants, the table its catalog rows
// point into, with oauthRefused naming the routes of that table GitLab
// refuses to an OAuth token.
func assemble(builds []build, surfaces surfaceNames, scopes map[string][]string, grants *finegrained.Table, oauthRefused []oauthRefusedRoute) (reference, error) {
	refused, refusedErr := oauthRefusedSet(grants, oauthRefused)
	if refusedErr != nil {
		return reference{}, refusedErr
	}
	a := &assembly{ref: reference{version: grants.DisplayVersion()}, scopes: scopes, groups: map[string]*refGroup{}, actions: map[string]*refAction{}}
	for _, b := range builds {
		for _, group := range b.catalog.Groups() {
			a.fold(b, group)
		}
	}
	ref := a.ref
	slices.SortFunc(ref.groups, func(x, y *refGroup) int { return cmp.Compare(x.tool, y.tool) })
	index := seeAlsoIndex(a.actions)
	for _, rg := range ref.groups {
		slices.SortFunc(rg.actions, func(x, y *refAction) int { return cmp.Compare(x.id, y.id) })
		for _, ra := range rg.actions {
			if err := ra.finish(rg, surfaces, index, grants, refused); err != nil {
				return reference{}, err
			}
		}
	}
	return ref, nil
}

// fold records one group of one build: what the build serves of it, and each
// of its actions, which a later build sees wider.
func (a *assembly) fold(b build, group actioncatalog.Group) {
	rg := a.groups[group.ToolName]
	if rg == nil {
		rg = &refGroup{
			tool:         group.ToolName,
			slug:         strings.ReplaceAll(strings.TrimPrefix(group.ToolName, "gitlab_"), "_", "-"),
			standalone:   group.SurfaceKind != actioncatalog.SurfaceKindMetaGroup,
			scopes:       a.scopes[group.ToolName],
			capabilities: group.CapabilityRequirements,
		}
		a.groups[group.ToolName] = rg
		a.ref.groups = append(a.ref.groups, rg)
	}
	rg.served[classIndex(b.dotcom)][b.tier] += len(group.Actions)
	a.ref.served[classIndex(b.dotcom)][b.tier] += len(group.Actions)
	for _, action := range group.ActionsInOrder() {
		ra := a.actions[string(action.ID)]
		if ra == nil {
			ra = &refAction{id: string(action.ID), tier: b.tier, dotcomOnly: true, paramTiers: map[string]edition.Tier{}}
			a.actions[ra.id] = ra
			rg.actions = append(rg.actions, ra)
		}
		ra.dotcomOnly = ra.dotcomOnly && b.dotcom
		ra.latest = action
		for name := range properties(action.Route.InputSchema) {
			if _, seen := ra.paramTiers[name]; !seen {
				ra.paramTiers[name] = b.tier
			}
		}
	}
}

// finish fills in what an action's page entry needs from its widest build,
// and names the tools it is reached through, refusing a name the surface it
// belongs to does not register, and an action the fine-grained table holds no
// classic scope for, since its token lines would be a guess. refused are the
// routes of the table GitLab refuses to an OAuth token.
func (ra *refAction) finish(group *refGroup, surfaces surfaceNames, index map[string]string, grants *finegrained.Table, refused map[string]bool) error {
	action := ra.latest
	row := action.FineGrained
	if row == nil || row.Classic == finegrained.ClassicUnknown {
		return fmt.Errorf("%s: the action grants table holds no classic scope for it; run make gen-action-grants first", ra.id)
	}
	ra.domain, _, _ = strings.Cut(ra.id, ".")
	ra.classic = row.Classic
	ra.scopes = group.scopes
	ra.oauthRefused, ra.oauthRefusedEveryWay = oauthRefusal(grants, row, refused)
	ra.fineGrained = grants.Describe(row)
	ra.name = action.Name
	ra.readOnly = action.ReadOnly
	ra.destructive = action.Destructive
	ra.idempotent = action.Idempotent
	ra.individual = action.IndividualTool.Name
	ra.standalone = group.standalone
	ra.metaTool = group.tool
	if ra.standalone {
		ra.metaTool = ra.individual
	}
	if !surfaces.meta[ra.metaTool] {
		return fmt.Errorf("%s: the meta surface registers no tool %q", ra.id, ra.metaTool)
	}
	if ra.individual != "" && !surfaces.individual[ra.individual] {
		return fmt.Errorf("%s: the individual surface registers no tool %q", ra.id, ra.individual)
	}
	ra.description = servedDescription(action, index)
	ra.params, ra.oneOf = parameters(action.Route.InputSchema, ra.paramTiers)
	return nil
}
