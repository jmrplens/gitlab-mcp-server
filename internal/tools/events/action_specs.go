package events

import (
	"slices"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The value sets below are the ones GitLab filters events on, which is not
// what the events API page lists. All three event routes (GET /events,
// /users/:id/events and /projects/:id/events) declare the filters through
// event_filter_params in lib/api/helpers/events_helpers.rb, as
// `values: Event.actions` and `values: Event.target_types`, and the two are
// not checked alike. Event.target_types is an array, so Grape refuses a
// target_type outside it with 400. Event.actions is the Rails enum hash, and
// Grape 2.4 reads a Hash given to `values:` as an options hash and takes its
// :value key as the value set, which that hash does not hold, so no action is
// checked at all; EventsFinder#by_action then returns the feed unfiltered for
// a name Event.actions does not hold. An unknown action is therefore answered
// with every event, as though it had filtered them, which is why
// [CheckActionFilter] refuses one before the request. Both sets are read off
// app/models/event.rb at GitLab 19.4.1, in the model's own order.

// eventActions is the keys of Event::ACTIONS, which `Event.actions` returns
// and EventsFinder#by_action looks the filter up in. transferred (13) is the
// one the page omits.
var eventActions = []string{
	"created", "updated", "closed", "reopened", "pushed", "commented", "merged",
	"joined", "left", "destroyed", "expired", "approved", "transferred",
}

// eventTargetTypes is the keys of Event::TARGET_TYPES, which
// `Event.target_types` returns. It names wiki and design, which the page
// omits, and has no epic, which the page lists since GitLab 17.3: an epic's
// events are recorded with target type Epic or WorkItem, and no key of
// Event::TARGET_TYPES maps to either, so GitLab refuses target_type=epic.
var eventTargetTypes = []string{
	"issue", "milestone", "merge_request", "note", "project", "snippet", "user",
	"wiki", "design",
}

// CheckActionFilter refuses an action filter outside eventActions, naming the
// values GitLab filters on, and accepts an empty one, which asks for no
// filter. GitLab never refuses such an action (see above): it answers with the
// whole feed, which a caller cannot tell from a filtered one, so every event
// listing calls this before its request.
func CheckActionFilter(action string) error {
	if action == "" || slices.Contains(eventActions, action) {
		return nil
	}
	return toolutil.ErrInvalidEnum("action", action, eventActions)
}

// FilterSchemaOverrides returns the input-schema overrides every event
// listing serves on its action and target_type filters: the value set GitLab
// filters each on as the enum, and a description naming the same values, so
// the schema a model reads and the prose beside it come from one list.
// user.contribution_events serves it too, since its route takes the same
// filters.
func FilterSchemaOverrides() []toolutil.InputSchemaOverride {
	return []toolutil.InputSchemaOverride{
		toolutil.SchemaPropertyOverride("action", map[string]any{
			"enum": schemaEnum(eventActions),
			"description": "Filter by event action, one of " + strings.Join(eventActions, ", ") +
				". Any other value is refused before the request, since GitLab would ignore it and answer every event.",
		}),
		toolutil.SchemaPropertyOverride("target_type", map[string]any{
			"enum": schemaEnum(eventTargetTypes),
			"description": "Filter by event target type, one of " + strings.Join(eventTargetTypes, ", ") +
				". GitLab refuses any other value, epic included. Responses spell the type in model form" +
				" (Issue, MergeRequest, WikiPage::Meta, DesignManagement::Design), which a filter does not accept.",
		}),
	}
}

// schemaEnum returns values as the []any a JSON Schema enum holds.
func schemaEnum(values []string) []any {
	enum := make([]any, len(values))
	for i, value := range values {
		enum[i] = value
	}
	return enum
}

// UserActionSpecs returns canonical specs for event actions exposed through gitlab_user.
func UserActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		userEventReadSpec("event_list_project", toolutil.RouteAction(client, ListProjectEvents), "gitlab_project_event_list"),
		userEventReadSpec("event_list_contributions", toolutil.RouteAction(client, ListCurrentUserContributionEvents), "gitlab_user_contribution_event_list"),
	}
}

func userEventReadSpec(name string, route toolutil.ActionRoute, individualTool string) toolutil.ActionSpec {
	usage := "List the authenticated user's contribution events (pushes, comments, issue and merge request activity) across visible resources, with action/target filters and offset or keyset pagination."
	aliases := []string{individualTool, "list my activity", "my contribution events", "what have I done recently", "my recent activity"}
	related := []string{"user.get", "project.get"}
	description := "List the current user's contribution events. Returns: each event with action_name, target type and IID, push_data, embedded note, author object, created timestamp, and pagination metadata. See also: gitlab_project_event_list, gitlab_get_user."
	guidance := map[string]toolutil.ParameterGuidance{}
	if name == "event_list_project" {
		usage = "List the visible activity events for one specific project (pushes, comments, member changes, issue and merge request actions), with action/target filters and offset or keyset pagination."
		aliases = []string{individualTool, "list project activity", "project event feed", "recent activity in a project", "what happened in this project"}
		related = []string{"project.get", "user.get"}
		description = "List a project's visible activity events. Returns: each event with action_name, target type and IID, push_data, embedded note with author, data (ref, commits, repository), author object, created timestamp, and pagination metadata. See also: gitlab_user_contribution_event_list, gitlab_project_get."
		guidance["project_id"] = toolutil.ParameterGuidance{
			SemanticRole:     "scope_project",
			ValueSource:      "Project ID or full namespace path whose activity events should be listed.",
			ExampleBinding:   `params.project_id:"group/project"`,
			CommonConfusions: []string{"Use the project that owns the events. Do not pass a group path or user ID here."},
		}
	}

	options := toolutil.ActionSpecOptions{
		Aliases:           aliases,
		Tags:              []string{"user", "event"},
		Usage:             usage,
		RelatedActions:    related,
		ParameterGuidance: guidance,
		OpenWorld:         true,
		OwnerPackage:      "events",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        individualTool,
			Title:       toolutil.TitleFromName(individualTool),
			Description: description,
		},
		InputSchemaOverrides: FilterSchemaOverrides(),
	}
	return toolutil.NewReadActionSpec(name, route, options)
}
