package prompts

import (
	"context"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/edition"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/actioncatalog"
)

// collectingRegistrar records what registerAll offers without a server.
type collectingRegistrar struct {
	names []string
}

func (r *collectingRegistrar) AddPrompt(prompt *mcp.Prompt, _ mcp.PromptHandler) {
	r.names = append(r.names, prompt.Name)
}

// registeredPromptNames returns every prompt registerAll offers, with the
// options applied.
func registeredPromptNames(client *gitlabclient.Client, opts ...RegisterOptions) []string {
	collector := &collectingRegistrar{}
	registerAll(registrarFor(collector, opts), client)
	return collector.names
}

// TestRegister_ExcludingAnActionRemovesThePromptServingTheSameData verifies
// that an operator's --exclude-tools reaches the prompt surface, which was the
// third request path to the same GitLab data with the same credential and the
// last one that took no options at all.
//
// Excluding gitlab_mr_changes_get used to leave review_mr returning the
// identical diffs, so the narrowing an operator configured was two thirds of a
// contract. Each subtest asserts both directions: the prompt whose data went is
// gone, and every prompt that shares no action with the exclusion is still
// there, because a filter that removes too much is its own defect.
func TestRegister_ExcludingAnActionRemovesThePromptServingTheSameData(t *testing.T) {
	tests := []struct {
		name    string
		opts    []RegisterOptions
		gone    []string
		present []string
	}{
		{
			name:    "no options registers everything",
			opts:    nil,
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
		{
			name:    "empty exclusion list registers everything",
			opts:    []RegisterOptions{{}},
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
		{
			name:    "excluding an action removes every prompt reading it",
			opts:    []RegisterOptions{{ExcludedActions: []string{"mr_review.changes_get"}}},
			gone:    []string{"review_mr", "summarize_mr_changes", "mr_risk_assessment", "mr_description_quality"},
			present: []string{"team_overview", "release_cadence", "label_distribution"},
		},
		{
			name:    "one of several backing actions is enough",
			opts:    []RegisterOptions{{ExcludedActions: []string{"branch.list"}}},
			gone:    []string{"project_health_check"},
			present: []string{"compare_branches", "review_mr"},
		},
		{
			name:    "excluding a group action removes the group prompts",
			opts:    []RegisterOptions{{ExcludedActions: []string{"merge_request.list_group"}}},
			gone:    []string{"team_overview", "group_mr_dashboard", "reviewer_workload", "weekly_team_recap"},
			present: []string{"review_mr", "label_distribution"},
		},
		{
			name: "several exclusions compose",
			opts: []RegisterOptions{{ExcludedActions: []string{"release.list", "repository.contributors"}}},
			gone: []string{"release_cadence", "project_contributors"},
			present: []string{
				"review_mr",
				"label_distribution",
			},
		},
		{
			name:    "blank and unknown entries change nothing",
			opts:    []RegisterOptions{{ExcludedActions: []string{"", "   ", "not.an_action"}}},
			present: []string{"review_mr", "team_overview", "release_cadence"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registered := registeredPromptNames(nil, tt.opts...)
			for _, name := range tt.gone {
				if slices.Contains(registered, name) {
					t.Errorf("prompt %q is still registered after excluding %v", name, tt.opts)
				}
			}
			for _, name := range tt.present {
				if !slices.Contains(registered, name) {
					t.Errorf("prompt %q was removed by excluding %v, which does not name it", name, tt.opts)
				}
			}
		})
	}
}

// TestRegister_ExcludedPromptIsNotListedByTheServer verifies the same thing on
// the wire: a client's prompts/list must not offer what the operator excluded,
// since a listed prompt is one a model will try to get, and prompts/get must
// refuse it rather than run the handler anyway.
//
// Filtering only the listing would be the worse half of the fix. A model that
// remembers a prompt name, or a client working from a cached list, calls
// prompts/get directly, and the handler holds the same credential whether or
// not the prompt was advertised.
func TestRegister_ExcludedPromptIsNotListedByTheServer(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	Register(server, nil, RegisterOptions{ExcludedActions: []string{"mr_review.changes_get"}})

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "probe", Version: "0.0.1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	list, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	names := make([]string, 0, len(list.Prompts))
	for _, prompt := range list.Prompts {
		names = append(names, prompt.Name)
	}

	t.Run("excluded prompt is not listed", func(t *testing.T) {
		if slices.Contains(names, "review_mr") {
			t.Error("prompts/list still offers review_mr after its diffs action was excluded")
		}
	})
	t.Run("unrelated prompt is still listed", func(t *testing.T) {
		if !slices.Contains(names, "team_overview") {
			t.Error("prompts/list no longer offers team_overview, which the exclusion does not name")
		}
	})
	t.Run("excluded prompt cannot be fetched", func(t *testing.T) {
		_, getErr := session.GetPrompt(ctx, &mcp.GetPromptParams{
			Name:      "review_mr",
			Arguments: map[string]string{argProjectID: "1", argMRIID: "1"},
		})
		if getErr == nil {
			t.Error("prompts/get served review_mr after it was excluded")
		}
	})
}

// TestPromptBackingActions_StaysAlignedWithBothSurfaces guards the two ways
// the hand-kept overlap table can rot.
//
// The table is the only thing relating a prompt to the tools serving the same
// data, so it fails in two directions: a prompt added to registerAll without an
// entry would silently escape every exclusion, and an action ID that no longer
// exists in the catalog would make an entry a dead string nobody notices.
// Neither is visible from either package alone.
func TestPromptBackingActions_StaysAlignedWithBothSurfaces(t *testing.T) {
	catalog, err := gitlabtools.BuildActionCatalog(nil, gitlabtools.ActionCatalogOptions{Tier: edition.Ultimate, IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}

	t.Run("every registered prompt is classified", func(t *testing.T) {
		for _, name := range registeredPromptNames(nil) {
			if _, ok := promptBackingActions[name]; !ok {
				t.Errorf("prompt %q has no entry in promptBackingActions, so --exclude-tools cannot reach it", name)
			}
		}
	})

	t.Run("no entry describes a prompt that is gone", func(t *testing.T) {
		registered := registeredPromptNames(nil)
		for name := range promptBackingActions {
			if !slices.Contains(registered, name) {
				t.Errorf("promptBackingActions names %q, which registerAll no longer registers", name)
			}
		}
	})

	t.Run("every named action exists in the catalog", func(t *testing.T) {
		for name, backing := range promptBackingActions {
			if len(backing) == 0 {
				t.Errorf("prompt %q lists no backing action; give it one or state why it has none", name)
				continue
			}
			for _, id := range backing {
				if _, ok := catalog.Action(actioncatalog.ActionID(id)); !ok {
					t.Errorf("prompt %q names action %q, which is not in the action catalog", name, id)
				}
			}
		}
	})
}
