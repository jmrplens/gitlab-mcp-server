// action_specs_test.go contains canonical route tests for project statistics.
package projectstatistics

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// statisticsSpec returns the package's one action spec, built against a
// client that forbids every request: everything asserted through it is
// metadata rather than a call.
func statisticsSpec(t *testing.T) toolutil.ActionSpec {
	t.Helper()
	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))
	if len(specs) != 1 {
		t.Fatalf("len(ActionSpecs) = %d, want 1", len(specs))
	}
	return specs[0]
}

// TestActionSpecs_ReadClassificationSurvivesTheProtectiveModes pins what the
// catalog projects onto every surface from this spec's constructor.
//
// Why that matters: read-only mode removes an action classified as mutating
// and safe mode answers it with a preview instead of running it, so one
// identifier changed at the call site would withdraw a plain read from the
// deployments most likely to be serving a reporter.
func TestActionSpecs_ReadClassificationSurvivesTheProtectiveModes(t *testing.T) {
	spec := statisticsSpec(t)
	if !spec.ReadOnly || spec.Destructive || !spec.Idempotent {
		t.Errorf("classification = {ReadOnly:%t Destructive:%t Idempotent:%t}, want {true false true}",
			spec.ReadOnly, spec.Destructive, spec.Idempotent)
	}
	if !spec.OpenWorld {
		t.Error("OpenWorld is false, though the action reads a remote GitLab instance")
	}
}

// TestActionSpecs_RelatedActionsNameCanonicalActions holds every entry of
// RelatedActions to the shape a caller can execute, and pins the one action
// this metadata exists to reach.
//
// Why that matters: nothing in the repository validates these strings, so a
// misspelled one passes every gate and answers a model "unknown action" the
// moment it follows the hint. The trap specific to this package is the
// domain: its action is aggregated into the gitlab_project group, so an id
// built from the owner package name (projectstatistics.get) would look right
// in this file and exist nowhere. project.get is named because a caller after
// repository size or storage wants that tool and not this one, which counts
// fetches.
func TestActionSpecs_RelatedActionsNameCanonicalActions(t *testing.T) {
	spec := statisticsSpec(t)
	if len(spec.RelatedActions) == 0 {
		t.Fatal("RelatedActions is empty, so nothing routes a model to the storage statistics this action does not return")
	}
	canonical := regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	for _, related := range spec.RelatedActions {
		t.Run(related, func(t *testing.T) {
			if !canonical.MatchString(related) {
				t.Errorf("related action %q is not a canonical domain.action id", related)
			}
			if strings.HasPrefix(related, spec.OwnerPackage+".") {
				t.Errorf("related action %q is built from the owner package; the catalog domain is project", related)
			}
		})
	}
	if !slices.Contains(spec.RelatedActions, "project.get") {
		t.Errorf("RelatedActions = %v, want project.get among them", spec.RelatedActions)
	}
}

// TestActionSpecs_DiscoveryMetadataIsFilled checks the fields a model reads to
// find this tool and fill its one argument. Each is optional to the
// constructor and invisible to every gate, so any of them can be dropped
// without a build or a test noticing, and the action then lists with no
// description, no tags to match on, and nothing warning that the fetch counts
// it returns are not the project's size.
func TestActionSpecs_DiscoveryMetadataIsFilled(t *testing.T) {
	spec := statisticsSpec(t)
	if spec.IndividualTool.Title == "" || spec.IndividualTool.Description == "" {
		t.Errorf("individual tool %q lists with title %q and description %q",
			spec.IndividualTool.Name, spec.IndividualTool.Title, spec.IndividualTool.Description)
	}
	if len(spec.Tags) == 0 {
		t.Error("Tags is empty, so tag search reaches this action through nothing")
	}
	guidance := spec.ParameterGuidance["project_id"]
	if guidance.ValueSource == "" || guidance.ExampleBinding == "" || len(guidance.CommonConfusions) == 0 {
		t.Errorf("project_id guidance = %+v, want a value source, an example binding and the size/fetch confusion", guidance)
	}
}

// TestActionSpecs_CallRouteError covers the project statistics route error path.
func TestActionSpecs_CallRouteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	spec := ActionSpecs(client)[0]
	if _, err := spec.Route.Handler(t.Context(), map[string]any{"project_id": "42"}); err == nil {
		t.Fatal("expected route error")
	}
}
