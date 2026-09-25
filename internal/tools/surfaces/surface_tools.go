package surfaces

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncompat"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/elicitationtools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/projectdiscovery"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	surfaceSafeModeGlobalWrapper = "global-safe-mode-wrapper"
	surfaceReadOnlyGlobalFilter  = "global-read-only-filter"
)

// CatalogOptions controls projection of standalone surface tools into an action
// catalog.
type CatalogOptions struct {
	ReadOnlyOnly     bool
	ExcludeToolNames []string
}

// StandaloneToolSpecs returns visible utility tools that remain outside
// ordinary GitLab API meta-tool dispatchers.
func StandaloneToolSpecs(client *gitlabclient.Client) []actioncatalog.SurfaceToolSpec {
	specs := make([]actioncatalog.SurfaceToolSpec, 0, 5)
	specs = append(specs, surfaceToolSpecsFromActions(surfaceToolGroupOptions{
		GroupToolName:         "gitlab_discover_project",
		BaseDomain:            "discover_project",
		SurfaceKind:           actioncatalog.SurfaceKindRuntimeUtility,
		Icons:                 toolutil.IconProject,
		FormatResult:          toolutil.MarkdownForResult,
		OwnerPackage:          "projectdiscovery",
		Description:           "Resolve a full git remote URL to a GitLab project and return its project_id and metadata. Read-only; use only for complete git remote URLs from .git/config or git remote -v.",
		CompatibilityToolName: "gitlab_discover_project",
	}, projectdiscovery.ActionSpecs(client))...)
	specs = append(specs, surfaceToolSpecsFromActions(surfaceToolGroupOptions{
		GroupToolName:          "gitlab_interactive",
		BaseDomain:             "interactive",
		SurfaceKind:            actioncatalog.SurfaceKindInteractiveUtility,
		Icons:                  toolutil.IconConfig,
		CapabilityRequirements: []string{"elicitation"},
		FormatResult:           elicitationtools.FormatResult,
		OwnerPackage:           "elicitationtools",
		Description:            "Guided interactive creation flows for issues, merge requests, projects, and releases. Mutating; use only when the task explicitly asks for a guided flow.",
		CompatibilityToolName:  "gitlab_interactive",
	}, elicitationtools.ActionSpecs(client))...)
	return specs
}

// AddToolCatalog projects surface specs into a catalog used by Dynamic
// discovery/execution.
func AddToolCatalog(catalog *actioncatalog.Catalog, specs []actioncatalog.SurfaceToolSpec, opts CatalogOptions) (*actioncatalog.Catalog, error) {
	if catalog == nil {
		catalog = actioncatalog.NewCatalog()
	}
	kept, err := filterToolSpecs(specs, opts)
	if err != nil {
		return nil, err
	}
	if addErr := addToolGroups(catalog, kept); addErr != nil {
		return nil, addErr
	}
	return catalog, nil
}

// ExcludedToolSpecs resolves an --exclude-tools list against surface specs and
// returns the tool names of the specs it removes, together with the entries
// that named none of them, in the order the operator wrote them.
//
// The rule is the action catalog's, asked rather than restated: the specs are
// assembled into a catalog of their own, unfiltered, and
// [actioncatalog.Catalog.FilterExcludedToolNames] judges it, so a group name,
// an individual tool name and a canonical action ID reach a standalone utility
// exactly as they reach a catalog action. Each surface used to keep a copy of
// its own for these tools, and the copies had drifted apart: the dynamic one
// matched the tool and the group name, the meta and individual ones the
// registered name alone, and none the canonical ID, which is the one spelling
// that means the same thing on every surface.
//
// The names are the ones the specs register under, which is what the pass
// over registered tools removes on the meta and individual surfaces and what
// [AddToolCatalog] leaves out on the dynamic one. An empty list builds
// nothing, since [AddToolCatalog] asks on every call and most calls exclude
// nothing.
func ExcludedToolSpecs(specs []actioncatalog.SurfaceToolSpec, excludeTools []string) (excluded map[string]struct{}, unmatched []string, err error) {
	if len(excludeTools) == 0 {
		return nil, nil, nil
	}
	assembled := actioncatalog.NewCatalog()
	if addErr := addToolGroups(assembled, specs); addErr != nil {
		return nil, nil, addErr
	}
	filtered, unmatched := assembled.FilterExcludedToolNames(excludeTools)
	kept := make(map[actioncatalog.ActionID]struct{}, filtered.CountActions())
	for _, action := range filtered.Actions() {
		kept[action.ID] = struct{}{}
	}
	excluded = make(map[string]struct{})
	for _, action := range assembled.Actions() {
		if _, ok := kept[action.ID]; !ok {
			excluded[action.IndividualTool.Name] = struct{}{}
		}
	}
	return excluded, unmatched, nil
}

// addToolGroups projects specs into catalog one group at a time, naming the
// group, or the first action of the one that clashed, when a group cannot be
// built or added.
func addToolGroups(catalog *actioncatalog.Catalog, specs []actioncatalog.SurfaceToolSpec) error {
	for _, groupSpec := range ToolGroupSpecs(specs) {
		group, err := actioncatalog.GroupFromSpecs(groupSpec.GroupOptions(), groupSpec.Actions)
		if err != nil {
			return fmt.Errorf("build surface tool group %s: %w", groupSpec.ToolName, err)
		}
		if addErr := catalog.AddGroup(group); addErr != nil {
			return fmt.Errorf("add surface tool group %s: %w", surfaceGroupActionLabel(group), addErr)
		}
	}
	return nil
}

func surfaceGroupActionLabel(group actioncatalog.Group) string {
	actions := group.ActionsInOrder()
	if len(actions) == 0 {
		return group.ToolName
	}
	return group.ToolName + "." + actions[0].Name
}

// ToolGroupSpecs groups surface specs into catalog group specs.
func ToolGroupSpecs(specs []actioncatalog.SurfaceToolSpec) []actioncatalog.CatalogGroupSpec {
	if len(specs) == 0 {
		return nil
	}
	type groupedSurface struct {
		options surfaceToolGroupOptions
		actions []toolutil.ActionSpec
	}
	groups := make(map[string]groupedSurface)
	for _, spec := range specs {
		spec = actioncatalog.CloneSurfaceToolSpec(spec)
		actionSpec, err := spec.ActionSpec()
		if err != nil {
			panic(fmt.Errorf("project surface tool %s: %w", spec.Name, err))
		}
		group := groups[spec.GroupToolName]
		if group.options.GroupToolName == "" {
			group.options = surfaceToolGroupOptions{
				GroupToolName:          spec.GroupToolName,
				BaseDomain:             spec.BaseDomain,
				SurfaceKind:            spec.SurfaceKind,
				Icons:                  spec.Icons,
				CapabilityRequirements: spec.CapabilityRequirements,
				FormatResult:           spec.FormatResult,
				OwnerPackage:           spec.OwnerPackage,
				Description:            spec.GroupDescription,
			}
		}
		group.actions = append(group.actions, actionSpec)
		groups[spec.GroupToolName] = group
	}
	toolNames := make([]string, 0, len(groups))
	for toolName := range groups {
		toolNames = append(toolNames, toolName)
	}
	sort.Strings(toolNames)
	out := make([]actioncatalog.CatalogGroupSpec, 0, len(toolNames))
	for _, toolName := range toolNames {
		group := groups[toolName]
		out = append(out, surfaceActionSpecGroup(group.options, group.actions))
	}
	return out
}

type surfaceToolGroupOptions struct {
	GroupToolName          string
	BaseDomain             string
	SurfaceKind            actioncatalog.SurfaceKind
	Icons                  []mcp.Icon
	CapabilityRequirements []string
	FormatResult           toolutil.FormatResultFunc
	OwnerPackage           string
	Description            string
	CompatibilityToolName  string
}

func surfaceToolSpecsFromActions(opts surfaceToolGroupOptions, specs []toolutil.ActionSpec) []actioncatalog.SurfaceToolSpec {
	specs = actioncompat.ApplyToActionSpecs(opts.CompatibilityToolName, opts.BaseDomain, specs)
	out := make([]actioncatalog.SurfaceToolSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, actioncatalog.SurfaceToolSpec{
			Name:  spec.IndividualTool.Name,
			Title: spec.IndividualTool.Title,
			// The action's own description, and separately the group's, which
			// every spec of the group carries because the group is assembled
			// from its actions and has no record of its own. Taking the
			// group's from the first action's, which is what happened until
			// GroupDescription existed, made a dispatcher introduce itself as
			// whichever action was registered first.
			Description:            spec.IndividualTool.Description,
			GroupDescription:       opts.Description,
			GroupToolName:          opts.GroupToolName,
			BaseDomain:             opts.BaseDomain,
			ActionName:             spec.Name,
			SurfaceKind:            opts.SurfaceKind,
			Route:                  spec.Route,
			Aliases:                spec.Aliases,
			Tags:                   spec.Tags,
			RelatedActions:         spec.RelatedActions,
			Compatibility:          spec.Compatibility,
			Icons:                  opts.Icons,
			CapabilityRequirements: opts.CapabilityRequirements,
			FormatResult:           opts.FormatResult,
			SafeModePolicy:         surfaceSafeModeGlobalWrapper,
			ReadOnlyPolicy:         surfaceReadOnlyGlobalFilter,
			OwnerPackage:           opts.OwnerPackage,
			ReadOnly:               spec.ReadOnly,
			Destructive:            spec.Destructive,
			Idempotent:             spec.Idempotent,
			OpenWorld:              spec.OpenWorld,
		})
	}
	return out
}

func surfaceActionSpecGroup(opts surfaceToolGroupOptions, specs []toolutil.ActionSpec) actioncatalog.CatalogGroupSpec {
	return actioncatalog.CatalogGroupSpec{
		ToolName:               opts.GroupToolName,
		Description:            opts.Description,
		ReadOnly:               readOnlyGroup(specs),
		Icons:                  opts.Icons,
		BaseDomain:             opts.BaseDomain,
		CapabilityRequirements: opts.CapabilityRequirements,
		FormatResult:           opts.FormatResult,
		Actions:                specs,
		OwnerPackage:           opts.OwnerPackage,
		SurfaceKind:            opts.SurfaceKind,
	}
}

// filterToolSpecs returns the specs a deployment serves: without the ones the
// operator excluded and, in read-only mode, without the ones that write.
//
// The exclusion is judged over every spec, before read-only mode removes any,
// which is the order the catalog filter applies the two in. The count it logs
// is the only line an operator gets about a standalone exclusion on the
// dynamic surface: the pass over registered tools sees two tools there and
// neither is ever a standalone utility, so without it a working exclusion read
// the same as one that named nothing.
func filterToolSpecs(specs []actioncatalog.SurfaceToolSpec, opts CatalogOptions) ([]actioncatalog.SurfaceToolSpec, error) {
	excluded, _, err := ExcludedToolSpecs(specs, opts.ExcludeToolNames)
	if err != nil {
		return nil, fmt.Errorf("resolve excluded surface tools: %w", err)
	}
	if len(excluded) > 0 {
		slog.Info("excluded standalone actions by configuration", "excluded", len(excluded), "patterns", opts.ExcludeToolNames)
	}
	out := make([]actioncatalog.SurfaceToolSpec, 0, len(specs))
	for _, spec := range specs {
		if opts.ReadOnlyOnly && !spec.ReadOnly {
			continue
		}
		// Trimmed as registration trims it, since the names resolved above
		// are the ones the specs register under.
		if _, ok := excluded[strings.TrimSpace(spec.Name)]; ok {
			continue
		}
		out = append(out, spec)
	}
	return out, nil
}

func readOnlyGroup(specs []toolutil.ActionSpec) bool {
	if len(specs) == 0 {
		return false
	}
	for _, spec := range specs {
		if !spec.ReadOnly {
			return false
		}
	}
	return true
}
