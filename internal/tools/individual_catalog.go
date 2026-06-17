package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// IndividualCatalogRegisterOptions controls how catalog actions are
// projected into the individual-tool MCP surface.
//
// ApplyEditionFilters enables CE/Enterprise/GitLab.com gating; Enterprise
// and GitLabDotCom flag the deployment type. ReadOnlyOnly restricts
// registration to read-only actions. SafeMode routes mutating actions
// through the safe-mode preview handler. IncludeHiddenActions keeps
// individual tools whose [toolutil.IndividualToolSpec.Hidden] flag is
// true. IncludeStandaloneUtilities allows runtime/interactive/server
// maintenance utility groups. AllowedToolNames and ExcludeToolNames are
// post-filter allow/exclude lists. DescriptionForTool computes a
// per-action description when the spec does not provide one.
type IndividualCatalogRegisterOptions struct {
	ApplyEditionFilters        bool
	Enterprise                 bool
	GitLabDotCom               bool
	ReadOnlyOnly               bool
	SafeMode                   bool
	IncludeHiddenActions       bool
	IncludeStandaloneUtilities bool
	AllowedToolNames           []string
	ExcludeToolNames           []string
	DescriptionForTool         func(actioncatalog.Action) string
}

// individualCatalogRegisterState carries the gating sets and registered
// tool names for a single [RegisterIndividualCatalogTools] invocation.
// Tracking registrations in a map ensures duplicate individual tool names
// are registered at most once even when the catalog contains them.
type individualCatalogRegisterState struct {
	opts       IndividualCatalogRegisterOptions
	allowed    map[string]struct{}
	excluded   map[string]struct{}
	registered map[string]struct{}
}

// RegisterIndividualCatalogTools registers individual MCP tools by
// projecting eligible actions from the canonical action catalog onto the
// supplied server. Nil server or catalog inputs are accepted as no-ops
// so startup paths that pre-flight the catalog remain safe. The function
// applies the gating rules documented on
// [IndividualCatalogRegisterOptions] in order: edition → surface kind →
// allow/exclude → read-only filter.
func RegisterIndividualCatalogTools(server *mcp.Server, catalog *actioncatalog.Catalog, opts IndividualCatalogRegisterOptions) {
	if server == nil || catalog == nil {
		return
	}
	state := individualCatalogRegisterState{opts: opts, allowed: stringSet(opts.AllowedToolNames), excluded: stringSet(opts.ExcludeToolNames), registered: make(map[string]struct{})}
	for _, group := range catalog.Groups() {
		registerIndividualCatalogGroup(server, group, state)
	}
}

// registerIndividualCatalogGroup projects every action in a single
// catalog group onto the server. Groups failing the surface or edition
// gate are silently skipped. The group's [actioncatalog.Group.FormatResult]
// is preferred; otherwise the dispatcher defaults to [markdownForResult].
func registerIndividualCatalogGroup(server *mcp.Server, group actioncatalog.Group, state individualCatalogRegisterState) {
	if !individualCatalogGroupEligible(group, state.opts) {
		return
	}
	formatResult := group.FormatResult
	if formatResult == nil {
		formatResult = markdownForResult
	}
	for _, action := range group.ActionsInOrder() {
		registerIndividualCatalogAction(server, group, action, formatResult, state)
	}
}

// registerIndividualCatalogAction projects one catalog action into an
// [mcp.Tool], validates eligibility, applies the read-only-only filter,
// and registers a handler that performs safe-mode and destructive
// confirmation before invoking the catalog route.
func registerIndividualCatalogAction(server *mcp.Server, group actioncatalog.Group, action actioncatalog.Action, formatResult toolutil.FormatResultFunc, state individualCatalogRegisterState) {
	toolName := strings.TrimSpace(action.IndividualTool.Name)
	if !individualCatalogToolEligible(toolName, action, state) {
		return
	}
	tool := mustIndividualToolFromCatalogAction(action, group.Icons, state.opts)
	if state.opts.ReadOnlyOnly && (tool.Annotations == nil || !tool.Annotations.ReadOnlyHint) {
		return
	}
	state.registered[toolName] = struct{}{}
	mcp.AddTool[map[string]any, any](server, tool, individualCatalogHandler(toolName, action, formatResult, state.opts))
}

// individualCatalogToolEligible reports whether the projected individual
// tool name should be registered. Applies the allow and exclude filters,
// then returns false if the name was already registered to keep
// duplicate individual tool names de-duplicated.
func individualCatalogToolEligible(toolName string, action actioncatalog.Action, state individualCatalogRegisterState) bool {
	if toolName == "" || !individualCatalogActionEligible(action, state.opts) {
		return false
	}
	if len(state.allowed) > 0 {
		if _, ok := state.allowed[toolName]; !ok {
			return false
		}
	}
	if _, ok := state.excluded[toolName]; ok {
		return false
	}
	_, exists := state.registered[toolName]
	return !exists
}

// individualCatalogActionEligible reports whether the action passes the
// edition filters. Returns true when filters are disabled so the same
// code path is reused for both gated and un-gated deployment shapes.
func individualCatalogActionEligible(action actioncatalog.Action, opts IndividualCatalogRegisterOptions) bool {
	if !opts.ApplyEditionFilters {
		return true
	}
	if action.GitLabDotComOnly && !opts.GitLabDotCom {
		return false
	}
	if action.Edition != "" && !opts.Enterprise {
		return false
	}
	return true
}

// individualCatalogGroupEligible reports whether the catalog group
// should contribute actions to the individual surface. The base meta
// and sampling groups are always eligible; runtime, interactive, and
// server maintenance groups require
// [IndividualCatalogRegisterOptions.IncludeStandaloneUtilities] to be
// opted in. Edition and GitLab.com filters apply when the gating flag
// is set.
func individualCatalogGroupEligible(group actioncatalog.Group, opts IndividualCatalogRegisterOptions) bool {
	switch group.SurfaceKind {
	case actioncatalog.SurfaceKindMetaGroup, actioncatalog.SurfaceKindGitLabAction, actioncatalog.SurfaceKindSamplingUtility:
		// Ordinary GitLab actions and sampling-backed actions are part of the
		// current individual surface.
	case actioncatalog.SurfaceKindRuntimeUtility, actioncatalog.SurfaceKindInteractiveUtility, actioncatalog.SurfaceKindServerMaintenance:
		if !opts.IncludeStandaloneUtilities {
			return false
		}
	default:
		return false
	}
	if opts.ApplyEditionFilters {
		if group.EnterpriseOnly && !opts.Enterprise {
			return false
		}
		if group.GitLabDotComOnly && !opts.GitLabDotCom {
			return false
		}
	}
	return true
}

// mustIndividualToolFromCatalogAction projects a catalog action into the
// [mcp.Tool] consumed by the individual surface. The description follows
// a fallback order: explicit spec description, curated snapshot
// description, custom DescriptionForTool callback, usage text, and
// finally a generic title. Panics on projection error so registration
// fails fast instead of exposing a malformed tool.
func mustIndividualToolFromCatalogAction(action actioncatalog.Action, icons []mcp.Icon, opts IndividualCatalogRegisterOptions) *mcp.Tool {
	spec := actionSpecFromCatalogAction(action)
	description := strings.TrimSpace(spec.IndividualTool.Description)
	if description == "" {
		description = strings.TrimSpace(catalogIndividualToolDescriptions[action.IndividualTool.Name])
	}
	if description == "" && opts.DescriptionForTool != nil {
		description = strings.TrimSpace(opts.DescriptionForTool(action))
	}
	if description == "" {
		description = strings.TrimSpace(action.Usage)
	}
	if description == "" {
		description = toolutil.TitleFromName(spec.IndividualTool.Name) + "."
	}
	tool, err := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{
		Description: description,
		Icons:       icons,
	})
	if err != nil {
		panic(fmt.Sprintf("project catalog action %s to individual tool: %v", action.ID, err))
	}
	return tool
}

// actionSpecFromCatalogAction converts a canonical catalog action back
// into an [toolutil.ActionSpec] for re-projection. The reverse mapping
// is used when callers want to feed the projection pipeline with a
// catalog action that has already been validated against the canonical
// schema.
func actionSpecFromCatalogAction(action actioncatalog.Action) toolutil.ActionSpec {
	return toolutil.NewActionSpec(action.Name, action.Route, toolutil.ActionSpecOptions{
		Aliases:                action.Aliases,
		Tags:                   action.Tags,
		Usage:                  action.Usage,
		RelatedActions:         action.RelatedActions,
		Compatibility:          action.Compatibility,
		ReadOnly:               action.ReadOnly,
		Destructive:            action.Destructive,
		Idempotent:             action.Idempotent,
		OpenWorld:              action.OpenWorld,
		Edition:                action.Edition,
		GitLabDotComOnly:       action.GitLabDotComOnly,
		OwnerPackage:           action.OwnerPackage,
		IndividualTool:         action.IndividualTool,
		ContentKind:            action.ContentKind,
		NotFoundPolicy:         action.NotFoundPolicy,
		EmbeddedResourcePolicy: action.EmbeddedResourcePolicy,
		RichResultPolicy:       action.RichResultPolicy,
		SchemaValidationNotes:  action.SchemaValidationNotes,
		RuntimeValidationNotes: action.RuntimeValidationNotes,
	})
}

// individualCatalogHandler returns the handler bound to one individual
// tool. The handler enforces safe-mode previews for mutating actions,
// destructive-action confirmation via [toolutil.ConfirmDestructiveAction],
// and rich-result hint injection via [toolutil.WithHints]. The catalog
// route's handler is invoked with the request context enhanced by
// [toolutil.ContextWithRequest] so logging and request metadata
// propagate.
func individualCatalogHandler(toolName string, action actioncatalog.Action, formatResult toolutil.FormatResultFunc, opts IndividualCatalogRegisterOptions) mcp.ToolHandlerFor[map[string]any, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, input map[string]any) (*mcp.CallToolResult, any, error) {
		if opts.SafeMode && !individualCatalogActionReadOnly(action) {
			result, err := safeModeHandler(toolName)(ctx, req)
			return result, nil, err
		}
		if action.Route.Destructive {
			message := fmt.Sprintf("Confirm %s? This action may be irreversible.", toolName)
			if result := toolutil.ConfirmDestructiveAction(ctx, req, input, message); result != nil {
				return result, nil, nil
			}
		}
		actionCtx := toolutil.ContextWithRequest(ctx, req)
		start := time.Now()
		result, err := action.Route.Handler(actionCtx, input)
		toolutil.LogToolCallAll(ctx, req, toolName, start, err)
		if err != nil {
			return nil, nil, err
		}
		callResult := formatResult(result)
		if callResult != nil && callResult.IsError {
			return callResult, nil, nil
		}
		return toolutil.WithHints(callResult, result, nil)
	}
}

// individualCatalogActionReadOnly reports whether the action is
// read-only for individual tool registration. Honors the explicit
// [toolutil.IndividualToolSpec.AnnotationOverrides.ReadOnly] override
// when set so compatibility aliases can mark otherwise mutating actions
// as read-only.
func individualCatalogActionReadOnly(action actioncatalog.Action) bool {
	readOnly := action.ReadOnly
	if action.IndividualTool.AnnotationOverrides.ReadOnly != nil {
		readOnly = *action.IndividualTool.AnnotationOverrides.ReadOnly
	}
	return readOnly
}

// stringSet returns a set keyed by the trimmed, non-empty values in
// input. Returns nil for an empty or nil input so callers can do a
// single nil check to decide whether a filter is active.
func stringSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}
