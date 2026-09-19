// action_specs_test.go contains canonical route error tests for Sidekiq metrics
// and the assertions over the discovery metadata each action publishes.
package sidekiq

import (
	"maps"
	"net/http"
	"reflect"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestActionSpecs_EachAction_CarriesTheUsageWrittenForIt asserts the exact
// sentence each of the four actions publishes, keyed by the action's own name.
// The usage used to be resolved from that name by a chain of comparisons, and
// because nothing read which sentence came back, negating any one of them
// handed an action a sibling's description and every test still passed. A model
// choosing between four metrics reads nothing else to tell them apart.
func TestActionSpecs_EachAction_CarriesTheUsageWrittenForIt(t *testing.T) {
	want := map[string]string{
		"sidekiq_queue_metrics":    "Read Sidekiq queue metrics for backlog and latency monitoring.",
		"sidekiq_process_metrics":  "Read Sidekiq worker process metrics for concurrency and busy slots.",
		"sidekiq_job_stats":        "Read Sidekiq aggregate job stats such as processed, failed, and enqueued counts.",
		"sidekiq_compound_metrics": "Read combined Sidekiq metrics payload for queue/process/job monitoring.",
	}

	byName := sidekiqSpecsByName(newSidekiqMetadataSpecs(t))
	if len(byName) != len(want) {
		t.Fatalf("ActionSpecs names = %v, want %d actions", slices.Sorted(maps.Keys(byName)), len(want))
	}
	for name, usage := range want {
		t.Run(name, func(t *testing.T) {
			spec, ok := byName[name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", name)
			}
			if spec.Usage != usage {
				t.Errorf("Usage for %s = %q, want %q", name, spec.Usage, usage)
			}
		})
	}
}

// TestActionSpecs_RelatedActions_NameTheIDsTheCatalogHolds asserts the two
// canonical IDs every Sidekiq action points a model at. Nothing in the tree
// resolves a related action against the catalog, so a wrong spelling ships
// silently and answers "unknown action" the first time a model follows it:
// this list named `health.status`, which no catalog holds, because the action
// is published by the `gitlab_server` group and so is `server.status`.
func TestActionSpecs_RelatedActions_NameTheIDsTheCatalogHolds(t *testing.T) {
	want := []string{"admin.metadata_get", "server.status"}
	for name, spec := range sidekiqSpecsByName(newSidekiqMetadataSpecs(t)) {
		t.Run(name, func(t *testing.T) {
			if !reflect.DeepEqual(spec.RelatedActions, want) {
				t.Errorf("RelatedActions for %s = %#v, want %#v", name, spec.RelatedActions, want)
			}
		})
	}
}

// newSidekiqMetadataSpecs builds the specs against a backend that answers
// nothing: the metadata tests read what each spec publishes and never call it.
func newSidekiqMetadataSpecs(t *testing.T) []toolutil.ActionSpec {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	return ActionSpecs(client)
}

// sidekiqSpecsByName indexes specs by their canonical action name, which is the
// key the usage and related-action tables are written against.
func sidekiqSpecsByName(specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	return byName
}

// TestActionSpecs_CallRouteErrors validates Sidekiq route error paths.
func TestActionSpecs_CallRouteErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	})
	client := testutil.NewTestClient(t, mux)
	specByTool := sidekiqSpecsByTool(ActionSpecs(client))

	tools := []string{
		"gitlab_get_sidekiq_queue_metrics",
		"gitlab_get_sidekiq_process_metrics",
		"gitlab_get_sidekiq_job_stats",
		"gitlab_get_sidekiq_compound_metrics",
	}
	for _, name := range tools {
		t.Run(name, func(t *testing.T) {
			spec, ok := specByTool[name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", name)
			}
			if _, err := spec.Route.Handler(t.Context(), map[string]any{}); err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}
