// action_specs_test.go contains unit tests for the events ActionSpec
// metadata, verifying every action carries a description, that both listings
// serve the filter values GitLab filters on, and that an action GitLab would
// ignore is refused before the request.
package events

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// gitLabEventActions and gitLabEventTargetTypes pin, independently of the
// lists the package serves, the keys of Event::ACTIONS and
// Event::TARGET_TYPES in app/models/event.rb at GitLab 19.4.1, which the
// events routes filter on (lib/api/helpers/events_helpers.rb,
// event_filter_params, and EventsFinder): Grape refuses a target_type outside
// the second, and nothing refuses an action outside the first, which
// EventsFinder answers with the unfiltered feed. A change to either list here
// is a change of GitLab release and cites it.
var (
	gitLabEventActions = []any{
		"created", "updated", "closed", "reopened", "pushed", "commented", "merged",
		"joined", "left", "destroyed", "expired", "approved", "transferred",
	}
	gitLabEventTargetTypes = []any{
		"issue", "milestone", "merge_request", "note", "project", "snippet", "user",
		"wiki", "design",
	}
)

// TestUserActionSpecs_FilterEnums_AreTheValuesGitLabFiltersOn holds the
// served input schema of both event listings to the pinned value sets: the
// enum is exactly GitLab's set in GitLab's order, so epic (which GitLab
// refuses) is not offered while wiki, design and transferred (which it
// filters on) are, and the description names the same values in the same
// order, so the prose beside the enum cannot drift from it.
func TestUserActionSpecs_FilterEnums_AreTheValuesGitLabFiltersOn(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	filters := []struct {
		property string
		want     []any
	}{
		{"action", gitLabEventActions},
		{"target_type", gitLabEventTargetTypes},
	}
	for _, spec := range UserActionSpecs(client) {
		properties, ok := spec.Route.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s input schema has no properties: %#v", spec.IndividualTool.Name, spec.Route.InputSchema)
		}
		for _, filter := range filters {
			t.Run(spec.IndividualTool.Name+"/"+filter.property, func(t *testing.T) {
				property, isMap := properties[filter.property].(map[string]any)
				if !isMap {
					t.Fatalf("property %q = %#v, want a schema object", filter.property, properties[filter.property])
				}
				if !reflect.DeepEqual(property["enum"], filter.want) {
					t.Errorf("enum = %v, want %v", property["enum"], filter.want)
				}
				names := make([]string, len(filter.want))
				for i, value := range filter.want {
					names[i] = value.(string)
				}
				description, _ := property["description"].(string)
				if want := "one of " + strings.Join(names, ", ") + "."; !strings.Contains(description, want) {
					t.Errorf("description = %q, want it to contain %q", description, want)
				}
			})
		}
	}
}

// TestFilterSchemaOverrides_EachCallReturnsItsOwnValues verifies that a caller
// changing the overrides it was handed cannot reach the next caller's: the
// users package serves the same overrides, and a shared backing array would
// let one spec's schema edit another's.
func TestFilterSchemaOverrides_EachCallReturnsItsOwnValues(t *testing.T) {
	first := FilterSchemaOverrides()
	first[0].Values["enum"].([]any)[0] = "mutated"
	first[1].Values["description"] = "mutated"

	second := FilterSchemaOverrides()
	if got := second[0].Values["enum"].([]any)[0]; got != "created" {
		t.Errorf("second call's first action = %v, want created", got)
	}
	if got := second[1].Values["description"]; got == "mutated" {
		t.Error("second call's target_type description was changed through the first call's")
	}
	if eventActions[0] != "created" {
		t.Errorf("eventActions[0] = %q, want created", eventActions[0])
	}
}

// TestCheckActionFilter_RefusesOnlyAnActionGitLabDoesNotFilterOn verifies
// that every pinned action and the empty value, which asks for no filter, are
// accepted, and that anything else is refused with an error naming the value
// and every action GitLab filters on: a near miss a model might write, a
// capitalized key (GitLab looks the filter up by exact key), and a target
// type in the action's place. GitLab answers each of those with the whole
// feed, so the refusal is the only way a caller learns of the mistake.
func TestCheckActionFilter_RefusesOnlyAnActionGitLabDoesNotFilterOn(t *testing.T) {
	accepted := []string{""}
	for _, action := range gitLabEventActions {
		accepted = append(accepted, action.(string))
	}
	for _, action := range accepted {
		t.Run("accepts "+action, func(t *testing.T) {
			if err := CheckActionFilter(action); err != nil {
				t.Errorf("CheckActionFilter(%q) = %v, want nil", action, err)
			}
		})
	}

	names := make([]string, len(gitLabEventActions))
	for i, action := range gitLabEventActions {
		names[i] = action.(string)
	}
	for _, action := range []string{"approve", "Created", "issue", " pushed"} {
		t.Run("refuses "+action, func(t *testing.T) {
			err := CheckActionFilter(action)
			if err == nil {
				t.Fatalf("CheckActionFilter(%q) = nil, want a refusal", action)
			}
			want := `invalid action "` + action + `", must be one of: ` + strings.Join(names, ", ")
			if err.Error() != want {
				t.Errorf("CheckActionFilter(%q) = %q, want %q", action, err, want)
			}
		})
	}
}

// TestEventListings_UnknownAction_RefusedBeforeTheRequest verifies that both
// listings of this package refuse an action outside the set before sending
// anything: the mock fails the test if a request reaches it, and each error
// names the operation and the refused value.
func TestEventListings_UnknownAction_RefusedBeforeTheRequest(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		call      func(ctx context.Context, client *gitlabclient.Client) error
	}{
		{"project events", "project_event_list", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := ListProjectEvents(ctx, client, ListProjectEventsInput{ProjectID: "42", Action: "approve"})
			return err
		}},
		{"contribution events", "user_contribution_event_list", func(ctx context.Context, client *gitlabclient.Client) error {
			_, err := ListCurrentUserContributionEvents(ctx, client, ListContributionEventsInput{Action: "approve"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(t.Context(), testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
			if err == nil {
				t.Fatal("error = nil, want the refusal of action approve")
			}
			for _, want := range []string{tt.operation + ":", `invalid action "approve"`, "transferred"} {
				t.Run(want, func(t *testing.T) {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q, want it to contain %q", err, want)
					}
				})
			}
		})
	}
}

// TestUserActionSpecs_Descriptions verifies the R-META individual-tool
// descriptions follow the "Returns: … See also: …" form.
func TestUserActionSpecs_Descriptions(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	for _, spec := range UserActionSpecs(client) {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			desc := spec.IndividualTool.Description
			if desc == "" {
				t.Fatal("empty description")
			}
			if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
				t.Errorf("description missing Returns/See also: %q", desc)
			}
		})
	}
}
