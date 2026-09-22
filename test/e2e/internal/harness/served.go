//go:build e2e

// served.go asks, once per session, whether the server serves what the
// server's own assemblers say it should.
//
// It is not a second implementation of the catalog. The expectation comes from
// calling tools.SharedMetaCatalog, tools.SharedIndividualCatalog and
// dynamiccatalog.Build with the config.ServerConfig the binary built for
// itself: the probed tier, the credential's scopes after NarrowToTokenScope,
// the protective mode and the operator's exclusions. What is reimplemented
// here is only the last step each surface takes after the catalog is built,
// which is registration, and each of those is a handful of lines.
//
// Why it exists: the thing a test cannot see is a tool that was never
// registered. A suite whose sessions were assembled in-process was served five
// catalog groups its own binary withholds for want of a scope, and every test
// of those groups passed. Checking the set once, when a session starts, turns
// that class into one failure that names the difference.

package harness

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamiccatalog"
)

// servedSets is what one session listed when it started.
type servedSets struct {
	// tools are the registered tool names.
	tools []string
	// resources are the static resource URIs.
	resources []string
	// templates are the resource URI templates.
	templates []string
	// prompts are the prompt names.
	prompts []string
	// promptSpecs pairs each served prompt with the arguments it declares, so
	// a sweep can bind the required ones from the World and skip a prompt it
	// cannot satisfy rather than watch it error.
	promptSpecs []PromptSpec
	// actions are the catalog actions this session can reach, which is not a
	// listing: it comes from the catalog the assemblers built.
	actions map[ActionID]struct{}
}

// PromptSpec is one served prompt and the arguments it declares, split into
// the ones a call must carry and the ones it may.
type PromptSpec struct {
	// Name is the prompt name a prompts/get call names.
	Name string
	// Required are the argument names the prompt refuses to render without.
	Required []string
	// Optional are the argument names it reads when given and does without
	// otherwise.
	Optional []string
}

// listServed reads the listings a session publishes, and only the ones the
// server declared at initialize.
//
// A client that calls a method the server did not declare is asking for
// "method not found", which is what the minimal capability surface answers to
// prompts/list: it registers no prompt, so the server declares no prompts
// capability. Reading the declaration rather than calling unconditionally is
// what a client is supposed to do, and it is what lets a scenario drive the
// minimal surface at all.
//
// The iterators are used rather than one call each, because a listing may be
// paginated and a first page read as the whole set would make the served-set
// check report every tool after it as missing.
func listServed(ctx context.Context, session *mcp.ClientSession) (servedSets, error) {
	var served servedSets
	capabilities := declaredCapabilities(session.InitializeResult())

	if capabilities.tools {
		for tool, err := range session.Tools(ctx, nil) {
			if err != nil {
				return served, fmt.Errorf("tools/list: %w", err)
			}
			served.tools = append(served.tools, tool.Name)
		}
	}
	if capabilities.resources {
		for resource, err := range session.Resources(ctx, nil) {
			if err != nil {
				return served, fmt.Errorf("resources/list: %w", err)
			}
			served.resources = append(served.resources, resource.URI)
		}
		for template, err := range session.ResourceTemplates(ctx, nil) {
			if err != nil {
				return served, fmt.Errorf("resources/templates/list: %w", err)
			}
			served.templates = append(served.templates, template.URITemplate)
		}
	}
	if capabilities.prompts {
		for prompt, err := range session.Prompts(ctx, nil) {
			if err != nil {
				return served, fmt.Errorf("prompts/list: %w", err)
			}
			served.prompts = append(served.prompts, prompt.Name)
			served.promptSpecs = append(served.promptSpecs, promptSpecOf(prompt))
		}
	}

	slices.Sort(served.tools)
	slices.Sort(served.resources)
	slices.Sort(served.templates)
	slices.Sort(served.prompts)
	slices.SortFunc(served.promptSpecs, func(a, b PromptSpec) int { return strings.Compare(a.Name, b.Name) })
	return served, nil
}

// listable says which listings the server declared at initialize.
type listable struct {
	tools     bool
	resources bool
	prompts   bool
}

// declaredCapabilities reads what the server said it serves. A session whose
// initialize result the SDK did not record is treated as serving everything,
// which is the behavior this had before it read the declaration at all: a
// listing that then fails is reported rather than silently skipped.
func declaredCapabilities(result *mcp.InitializeResult) listable {
	if result == nil || result.Capabilities == nil {
		return listable{tools: true, resources: true, prompts: true}
	}
	return listable{
		tools:     result.Capabilities.Tools != nil,
		resources: result.Capabilities.Resources != nil,
		prompts:   result.Capabilities.Prompts != nil,
	}
}

// completionReferences lists every reference and argument a completion can
// address on a session: each argument a served prompt declares, required and
// optional alike, and each variable of each served resource template, spelled
// the way a completion call names its target.
//
// It is the denominator the coverage command divides the completion cells by,
// written on the session line because nothing else says what a session could
// have completed. Without it every completion cell was one a test had asked
// for, counted against a list of what the tests had asked for.
func completionReferences(served servedSets) []string {
	var references []string
	for _, spec := range served.promptSpecs {
		for _, argument := range slices.Concat(spec.Required, spec.Optional) {
			references = append(references, completionCallTarget(spec.Name, argument))
		}
	}
	for _, template := range served.templates {
		for _, variable := range TemplateVariables(template) {
			references = append(references, completionCallTarget(template, variable))
		}
	}
	slices.Sort(references)
	return slices.Compact(references)
}

// TemplateVariables returns the RFC 6570 variable names of a resource
// template in the order they appear, each stripped of the reserved "+" a
// path variable carries.
//
// It is exported because the completion sweep asks for a completion of each
// of them and the harness spells a session's completion denominator from the
// same list: two rules for what a variable is would let a sweep complete one
// name while the denominator counted another.
func TemplateVariables(template string) []string {
	var names []string
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			return names
		}
		end := strings.IndexByte(rest[open:], '}')
		if end < 0 {
			return names
		}
		names = append(names, strings.TrimPrefix(rest[open+1:open+end], "+"))
		rest = rest[open+end+1:]
	}
}

// promptSpecOf splits a listed prompt's arguments into required and optional.
func promptSpecOf(prompt *mcp.Prompt) PromptSpec {
	spec := PromptSpec{Name: prompt.Name}
	for _, arg := range prompt.Arguments {
		if arg == nil {
			continue
		}
		if arg.Required {
			spec.Required = append(spec.Required, arg.Name)
			continue
		}
		spec.Optional = append(spec.Optional, arg.Name)
	}
	slices.Sort(spec.Required)
	slices.Sort(spec.Optional)
	return spec
}

// surfaceExpectation is what one configuration should serve: the catalog-backed
// tool names, and the actions those tools can reach.
type surfaceExpectation struct {
	// tools are the registered names the catalog accounts for, sorted.
	tools []string
	// actions are the catalog actions the surface can reach, the standalone
	// utilities among them wherever the surface registers them.
	actions map[ActionID]struct{}
	// standalone are the tool names registered outside the catalog, which the
	// listing comparison ignores: they are pinned by behavior tests instead.
	// The actions behind them are in actions all the same.
	standalone []string
}

// The three assemblers the expectation is read from. They are variables so a
// test can make one fail, which nothing a session can be configured with does,
// and a failure there has to stop the session rather than leave it expecting
// nothing.
var (
	assembleDynamicCatalog    = dynamiccatalog.Build
	assembleMetaCatalog       = gitlabtools.SharedMetaCatalog
	assembleIndividualCatalog = gitlabtools.SharedIndividualCatalog
)

// expectedSurface asks the server's own assemblers what a configuration
// serves, given the config.ServerConfig the binary built for itself.
//
// The client it builds the catalogs with is the harness's own. Binding rebuilds
// the handlers for that client and touches nothing this reads, so the names and
// the action IDs are the ones the binary's own catalog carries.
func expectedSurface(inst *instance, surface Surface, serverCfg *config.ServerConfig) (surfaceExpectation, error) {
	client := inst.client
	standalone := standaloneToolNames(client)

	switch surface {
	case SurfaceDynamic:
		catalog, _, err := assembleDynamicCatalog(client, serverCfg)
		if err != nil {
			return surfaceExpectation{}, fmt.Errorf("assemble the dynamic catalog: %w", err)
		}
		// The dynamic surface registers two tools whatever the catalog holds;
		// the catalog decides which actions they reach, which is what the
		// action set carries.
		return surfaceExpectation{
			tools:      []string{dynamictools.ExecuteActionToolName, dynamictools.FindActionToolName},
			actions:    catalogActionIDs(catalog),
			standalone: standalone,
		}, nil
	case SurfaceMeta:
		catalog, _, err := assembleMetaCatalog(client, serverCfg)
		if err != nil {
			return surfaceExpectation{}, fmt.Errorf("assemble the meta catalog: %w", err)
		}
		names := make([]string, 0, catalog.CountGroups())
		for _, group := range catalog.Groups() {
			names = append(names, group.ToolName)
		}
		slices.Sort(names)
		actions := catalogActionIDs(catalog)
		if standaloneErr := addStandaloneActions(actions, inst, serverCfg); standaloneErr != nil {
			return surfaceExpectation{}, standaloneErr
		}
		return surfaceExpectation{tools: names, actions: actions, standalone: standalone}, nil
	case SurfaceIndividual:
		catalog, _, err := assembleIndividualCatalog(client, serverCfg)
		if err != nil {
			return surfaceExpectation{}, fmt.Errorf("assemble the individual catalog: %w", err)
		}
		names, actions := individualRegistrations(catalog, serverCfg.ReadOnly)
		if standaloneErr := addStandaloneActions(actions, inst, serverCfg); standaloneErr != nil {
			return surfaceExpectation{}, standaloneErr
		}
		return surfaceExpectation{tools: names, actions: actions, standalone: standalone}, nil
	default:
		return surfaceExpectation{}, fmt.Errorf("unknown tool surface %q", surface)
	}
}

// addStandaloneActions adds the standalone utilities a meta or individual
// server registers to the actions the surface's catalog accounts for.
//
// Those two surfaces register them outside the catalog, so no catalog of theirs
// names them, and until issue 903 a session on either refused to send a call to
// one: the projection could spell it, and the served set said it was not there.
// They are read from the projection, which is built from the assembly that
// does carry them, at the tier the server serves.
func addStandaloneActions(actions map[ActionID]struct{}, inst *instance, serverCfg *config.ServerConfig) error {
	projected, err := newProjection(serverCfg.Tier, inst.client.IsGitLabDotCom())
	if err != nil {
		return fmt.Errorf("project the standalone utilities: %w", err)
	}
	maps.Copy(actions, standaloneActions(projected, serverCfg))
	return nil
}

// standaloneActions replays what the binary does with the standalone utilities
// on the meta and individual surfaces, which is register every one of them
// and then run the visibility pass over the tools it registered.
//
// That pass removes a tool whose registered name an operator's exclusion names
// exactly, and in read-only mode every tool that does not read; safe mode
// wraps the rest in a preview and removes nothing, so it keeps them all here.
// The exact name is the rule because it is the binary's rule on these two
// surfaces, even though the dynamic surface also honors the group name: a
// harness that applied the broader rule would expect a tool gone that the
// server still serves (issue 911 tracks the difference). The read-only mode is
// the configuration's own, which already carries the narrowing a credential
// that cannot write imposes.
func standaloneActions(projected *projection, serverCfg *config.ServerConfig) map[ActionID]struct{} {
	actions := make(map[ActionID]struct{})
	for id, action := range projected.actions {
		if !action.standalone || slices.Contains(serverCfg.ExcludeTools, action.metaTool) {
			continue
		}
		if serverCfg.ReadOnly && !action.readOnly {
			continue
		}
		actions[id] = struct{}{}
	}
	return actions
}

// credentialFacts is what the binary learns from the credential it starts
// with, and builds its catalog from: the token's scopes, and the tier the
// token could read off the license.
//
// The tier is the credential's and not the instance's. The license endpoint
// answers administrators only, so a token belonging to anyone else is served
// the Free catalog on a licensed instance whatever the run's own probe found,
// and an expectation built from the run's tier would name every licensed
// group the server never registered for it.
type credentialFacts struct {
	scopes []string
	tier   edition.Tier
}

// credential returns what the binary learns from the run's own token, which
// the probe already resolved.
func (inst *instance) credential() credentialFacts {
	return credentialFacts{scopes: inst.facts.Scopes, tier: inst.facts.Tier}
}

// serverConfigFor builds the configuration the binary builds for itself from
// the same inputs: what the environment said, and what the credential the
// session runs with can do and can see.
//
// The configuration it is given is a normalized one, which is what lets every
// field be copied across as written: the empty values a caller may leave have
// already become the defaults the child will apply.
func serverConfigFor(inst *instance, cfg ServerConfig, cred credentialFacts) *config.ServerConfig {
	serverCfg := &config.ServerConfig{
		GitLabURL:         inst.facts.URL,
		ToolSurface:       string(cfg.Surface),
		CapabilitySurface: string(cfg.Capabilities),
		Tier:              cfg.resolvedTier(cred.tier),
		ReadOnly:          cfg.Mode == ModeReadOnly,
		SafeMode:          cfg.Mode == ModeSafe,
		ExcludeTools:      slices.Clone(cfg.ExcludeTools),
		TokenScopes:       cred.scopes,
		// What the session asked for, which [ServerConfig.normalized] has
		// already resolved to the binary's own default when it asked for
		// nothing. It was the default outright until a model was the reader of
		// these schemas, and an expectation built from the default while the
		// child served compact would compare the right names against the wrong
		// surface.
		MetaParamSchema: string(cfg.MetaParamSchema),
	}
	// The same call the binary makes, in the same place: a credential that
	// cannot write is served a read-only surface whatever the deployment
	// asked for, and an expectation built without it would name write tools
	// the server never registered.
	gitlabclient.NarrowToTokenScope(serverCfg)
	return serverCfg
}

// standaloneToolNames lists the tools registered outside the action catalog:
// gitlab_discover_project and the gitlab_interactive_* flows.
//
// They are left out of the listing comparison rather than checked there
// because nothing in the catalog accounts for them, so a comparison would be
// this file restating the spec list to itself. Their presence is pinned by the
// behavior tests that call them, and the actions behind them are accounted for
// by [standaloneActions].
func standaloneToolNames(client *gitlabclient.Client) []string {
	specs := gitlabtools.StandaloneSurfaceToolSpecs(client)
	names := make([]string, 0, len(specs))
	for _, spec := range specs {
		names = append(names, spec.Name)
	}
	slices.Sort(names)
	return names
}

// catalogActionIDs returns every action a catalog holds.
func catalogActionIDs(catalog *actioncatalog.Catalog) map[ActionID]struct{} {
	actions := make(map[ActionID]struct{}, catalog.CountActions())
	for _, action := range catalog.Actions() {
		actions[ActionID(action.ID)] = struct{}{}
	}
	return actions
}

// individualRegistrations replays what the individual surface registers from a
// catalog: one tool per action that declares a name, the first declarer of a
// name keeping it, and, in read-only mode, only the ones whose annotation says
// they read.
//
// The read-only step is here rather than in the catalog because that is where
// the binary does it: SharedIndividualCatalog applies the exclusions and the
// credential's scopes, and read-only mode is applied afterwards, over the
// registered tools, by the visibility pass.
func individualRegistrations(catalog *actioncatalog.Catalog, readOnly bool) (names []string, actions map[ActionID]struct{}) {
	names = make([]string, 0, catalog.CountActions())
	actions = make(map[ActionID]struct{}, catalog.CountActions())
	taken := make(map[string]struct{}, catalog.CountActions())

	for _, group := range catalog.Groups() {
		if !individualGroupRegistered(group) {
			continue
		}
		for _, action := range group.ActionsInOrder() {
			name := strings.TrimSpace(action.IndividualTool.Name)
			if name == "" {
				continue
			}
			if _, already := taken[name]; already {
				continue
			}
			taken[name] = struct{}{}
			if readOnly && !individualActionReadOnly(action) {
				continue
			}
			names = append(names, name)
			actions[ActionID(action.ID)] = struct{}{}
		}
	}
	slices.Sort(names)
	return names, actions
}

// individualGroupRegistered mirrors the group gate the individual surface
// applies with standalone utilities included, which is how the binary calls it.
// Only the dynamic controller group has no individual projection.
func individualGroupRegistered(group actioncatalog.Group) bool {
	switch group.SurfaceKind {
	case actioncatalog.SurfaceKindGitLabAction, actioncatalog.SurfaceKindMetaGroup,
		actioncatalog.SurfaceKindRuntimeUtility, actioncatalog.SurfaceKindInteractiveUtility:
		return true
	default:
		return false
	}
}

// individualActionReadOnly is the read-only bit the individual surface serves
// as a tool's annotation: the action's own classification, narrowed by an
// override that can only ever narrow it.
func individualActionReadOnly(action actioncatalog.Action) bool {
	readOnly := action.ReadOnly
	overrides := action.IndividualTool.AnnotationOverrides.NarrowingOnly(action.ReadOnly, action.Idempotent)
	if overrides.ReadOnly != nil {
		readOnly = *overrides.ReadOnly
	}
	return readOnly
}

// missingDiffLimit is how many names each half of a difference report carries.
// A surface can differ by a thousand names when something is wrong at the
// catalog level, and a thousand-line failure hides the count that says so.
const missingDiffLimit = 20

// checkServedTools compares what a session listed with what the assemblers say
// it should serve, ignoring the tools registered outside the catalog.
func checkServedTools(surface Surface, served []string, expected surfaceExpectation) error {
	standalone := make(map[string]struct{}, len(expected.standalone))
	for _, name := range expected.standalone {
		standalone[name] = struct{}{}
	}

	// Dropped from both sides, not only from what was served: the standalone
	// tools come from RegisterMetaStandaloneTools on meta and individual alike,
	// not from either catalog, so the expectation built from a catalog does not
	// name them and a listing does. [standaloneActions] is what accounts for the
	// actions behind them; this comparison is about the catalog-backed names.
	catalogBacked := withoutNames(served, standalone)
	wanted := withoutNames(expected.tools, standalone)

	missing := difference(wanted, catalogBacked)
	unexpected := difference(catalogBacked, wanted)
	if len(missing) == 0 && len(unexpected) == 0 {
		return nil
	}

	var report strings.Builder
	fmt.Fprintf(&report, "the %s session does not serve what the catalog assemblers say it should:\n", surface)
	fmt.Fprintf(&report, "  served %d catalog-backed tools, expected %d\n", len(catalogBacked), len(wanted))
	writeNameList(&report, "expected and not served", missing)
	writeNameList(&report, "served and not expected", unexpected)
	report.WriteString("  standalone tools are excluded from this comparison: " + strings.Join(expected.standalone, ", "))
	return errors.New(report.String())
}

// writeNameList adds one half of a difference to the report, truncated.
func writeNameList(report *strings.Builder, heading string, names []string) {
	if len(names) == 0 {
		return
	}
	shown := names
	suffix := ""
	if len(shown) > missingDiffLimit {
		shown = shown[:missingDiffLimit]
		suffix = fmt.Sprintf(" (and %d more)", len(names)-missingDiffLimit)
	}
	fmt.Fprintf(report, "  %s (%d): %s%s\n", heading, len(names), strings.Join(shown, ", "), suffix)
}

// withoutNames returns the names that are not in the given set.
func withoutNames(names []string, excluded map[string]struct{}) []string {
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, found := excluded[name]; !found {
			kept = append(kept, name)
		}
	}
	return kept
}

// difference returns the sorted names in a that are not in b.
func difference(a, b []string) []string {
	present := make(map[string]struct{}, len(b))
	for _, name := range b {
		present[name] = struct{}{}
	}
	var only []string
	for _, name := range a {
		if _, found := present[name]; !found {
			only = append(only, name)
		}
	}
	slices.Sort(only)
	return slices.Compact(only)
}
