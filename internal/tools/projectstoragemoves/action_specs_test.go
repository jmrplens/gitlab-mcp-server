// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains integration tests for the project storage move
// tool closures in ActionSpecs routes with a mock GitLab API.
package projectstoragemoves

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const registerStorageMoveJSON = `{
	"id": 1,
	"created_at": "2026-01-15T10:30:00Z",
	"state": "finished",
	"source_storage_name": "default",
	"destination_storage_name": "storage2",
	"project": {"id": 42, "name": "my-project", "path_with_namespace": "group/my-project"}
}`

// TestActionSpecs_Metadata verifies project storage move action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "projectstoragemoves" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_ActionIDs_NameTheActionsThisPackageDeclares holds every
// canonical ID this package quotes to the actions it really registers under the
// storage_move catalog domain, in both directions.
//
// Nothing else checks these: the discovery audit only counts an empty
// RelatedActions, so a wrong spelling passes every gate and answers a model
// "unknown action" the moment it follows the hint. This package shipped all six
// spelled from its own package name, and the markdown hints kept a second,
// correct copy beside them. That is why the one block is now shared and why
// this asserts the set rather than a sample.
func TestActionSpecs_ActionIDs_NameTheActionsThisPackageDeclares(t *testing.T) {
	// The domain is named here rather than read from the source, so the
	// comparison has an oracle of its own: storage_move is the catalog group
	// all three storage-move packages register under.
	const wantDomain = "storage_move"

	specs := ActionSpecs(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))

	declared := make(map[string]bool, len(specs))
	for _, spec := range specs {
		declared[wantDomain+"."+spec.Name] = true
	}
	quoted := map[string]bool{
		actionRetrieveAll: true, actionRetrieveOne: true, actionGet: true,
		actionGetForProject: true, actionSchedule: true, actionScheduleAll: true,
	}
	if !maps.Equal(quoted, declared) {
		t.Errorf("quoted action IDs %v, declared %v", slices.Sorted(maps.Keys(quoted)), slices.Sorted(maps.Keys(declared)))
	}

	for _, spec := range specs {
		t.Run(spec.Name, func(t *testing.T) {
			for _, related := range spec.RelatedActions {
				if !quoted[related] {
					t.Errorf("RelatedActions names %q, which is not one of this package's action IDs", related)
				}
			}
		})
	}
}

// TestActionSpecs_CallRoutes verifies all registered project storage move routes execute successfully.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	// List all storage moves
	mux.HandleFunc("GET /api/v4/project_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	// List moves for a project
	mux.HandleFunc("GET /api/v4/projects/{pid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+registerStorageMoveJSON+`]`)
	})
	// Get single move by global ID
	mux.HandleFunc("GET /api/v4/project_repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	// Get single move for a project
	mux.HandleFunc("GET /api/v4/projects/{pid}/repository_storage_moves/{id}", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
	})
	// Schedule move for a project
	mux.HandleFunc("POST /api/v4/projects/{pid}/repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, registerStorageMoveJSON)
	})
	// Schedule all moves
	mux.HandleFunc("POST /api/v4/project_repository_storage_moves", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"message":"202 Accepted"}`)
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_retrieve_all_project_storage_moves", map[string]any{}},
		{"gitlab_retrieve_project_storage_moves", map[string]any{"project_id": 42}},
		{"gitlab_get_project_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_project_storage_move_for_project", map[string]any{"project_id": 42, "id": 1}},
		{"gitlab_schedule_project_storage_move", map[string]any{"project_id": 42, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_project_storage_moves", map[string]any{"source_storage_name": "default"}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}

// TestAnswerUnresolved_APathThatNamesNothing_IsANotFoundResultWithAHint verifies
// that a path GitLab finds nothing for answers each of the three actions with a
// not-found result naming it and saying where a path is read, through the route
// the catalog holds, and sends only the read that looked for it.
func TestAnswerUnresolved_APathThatNamesNothing_IsANotFoundResultWithAHint(t *testing.T) {
	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"gitlab_retrieve_project_storage_moves", map[string]any{"project_id": "no/such"}},
		{"gitlab_get_project_storage_move_for_project", map[string]any{"project_id": "no/such", "id": 1}},
		{"gitlab_schedule_project_storage_move", map[string]any{"project_id": "no/such"}},
	} {
		t.Run(tool.name, func(t *testing.T) {
			var requests requestLog
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.add(r)
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
			}))
			got, err := routeOf(t, client, tool.name).Handler(t.Context(), tool.args)
			if err != nil {
				t.Fatalf("Handler() error = %v, want the not-found output", err)
			}
			if want := (unresolvedOutput{Path: "no/such"}); got != want {
				t.Errorf("Handler() = %#v, want %#v", got, want)
			}
			if sent := requests.requests(); !slices.Equal(sent, []string{"GET /api/v4/projects/no%2Fsuch"}) {
				t.Errorf("requests = %q, want only the read of the path", sent)
			}
			assertNotFoundResult(t, got)
		})
	}
}

// TestActionSpecs_AnEncodedPath_IsLookedUpEscapedOnce verifies a project path
// sent URL-encoded, the form the schema names, reaches GitLab escaped once
// through the route the catalog holds for each of the three actions that read a
// path into its ID: the route decodes project_id before the handler runs, as the
// dispatchers decode every path identifier by name, and client-go escapes the
// plain path itself, so the lookup is top%2Fapp and never top%252Fapp.
func TestActionSpecs_AnEncodedPath_IsLookedUpEscapedOnce(t *testing.T) {
	for _, tc := range []struct {
		tool, route string
		args        map[string]any
	}{
		{"gitlab_retrieve_project_storage_moves", "GET /api/v4/projects/42/repository_storage_moves", map[string]any{"project_id": "top%2Fapp"}},
		{"gitlab_get_project_storage_move_for_project", "GET /api/v4/projects/42/repository_storage_moves/1", map[string]any{"project_id": "top%2Fapp", "id": 1}},
		{"gitlab_schedule_project_storage_move", "POST /api/v4/projects/42/repository_storage_moves", map[string]any{"project_id": "top%2Fapp"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			var requests requestLog
			client := testutil.NewTestClient(t, projectAnswer(&requests))
			if _, err := routeOf(t, client, tc.tool).Handler(t.Context(), tc.args); err != nil {
				t.Fatalf("Handler() error = %v", err)
			}
			want := []string{"GET /api/v4/projects/top%2Fapp", tc.route}
			if got := requests.requests(); !slices.Equal(got, want) {
				t.Errorf("requests = %q, want %q", got, want)
			}
		})
	}
}

// routeOf finds the route the catalog holds for one individual tool.
func routeOf(t *testing.T, client *gitlabclient.Client, tool string) toolutil.ActionRoute {
	t.Helper()
	for _, spec := range ActionSpecs(client) {
		if spec.IndividualTool.Name == tool {
			return spec.Route
		}
	}
	t.Fatalf("no spec for %s", tool)
	return toolutil.ActionRoute{}
}

// assertNotFoundResult holds an output to the not-found result it renders as.
// A project hidden or pending deletion is found by its numeric ID alone, and
// the result sends the reader to the list that holds one, the project list
// with the two filters an administrator sets, never to the storage move list,
// which holds only projects that were moved before.
func assertNotFoundResult(t *testing.T, got any) {
	t.Helper()
	result := toolutil.MarkdownForResult(got)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"Project Not Found", "**no/such**", "project.list", "numeric ID", "include_pending_delete=true", "include_hidden=true"} {
		if !strings.Contains(text, want) {
			t.Errorf("result = %q, want it to contain %q", text, want)
		}
	}
	if strings.Contains(text, actionRetrieveAll) {
		t.Errorf("result = %q, want it not to send the reader to %s, which lists no project that was never moved", text, actionRetrieveAll)
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want a not-found result to be an error")
	}
}

// TestAnswerUnresolved_AnyOtherError_PassesThrough verifies the wrapper answers the
// path alone: a read refused for another reason, and a 404 from the storage move
// route once the path was found, stay errors, since the first is a permission and
// the second a project that is there.
func TestAnswerUnresolved_AnyOtherError_PassesThrough(t *testing.T) {
	cases := []struct {
		name    string
		respond http.HandlerFunc
		want    string
	}{
		{
			name: "the read is refused",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			want: "403",
		},
		{
			name: "the storage move route answers 404",
			respond: func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "repository_storage_moves") {
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, `{"id":42}`)
			},
			want: "verify project_id exists",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := answerUnresolved(toolutil.RouteAction(testutil.NewTestClient(t, tc.respond), RetrieveForProject))
			got, err := route.Handler(t.Context(), map[string]any{"project_id": "a/b"})
			if err == nil {
				t.Fatalf("Handler() = %#v, want an error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestAnswerUnresolved_ARebinding_KeepsTheAnswer verifies the shared catalog builds
// the wrapped handler for another client too, since HTTP mode binds each request's
// credential onto the route rather than using the client it was built with.
func TestAnswerUnresolved_ARebinding_KeepsTheAnswer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	route := answerUnresolved(toolutil.RouteAction(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), RetrieveForProject))
	got, err := route.Bind(client)(t.Context(), map[string]any{"project_id": "a/b"})
	if err != nil {
		t.Fatalf("Bind(client)() error = %v, want the not-found output", err)
	}
	if want := (unresolvedOutput{Path: "a/b"}); got != want {
		t.Errorf("Bind(client)() = %#v, want %#v", got, want)
	}
}

// TestGuidanceProjectID_SaysWhatTheFieldTakes verifies the guidance a model reads
// offers the path and says what it costs, and the one case it does not reach.
func TestGuidanceProjectID_SaysWhatTheFieldTakes(t *testing.T) {
	guidance := guidanceProjectID()
	if !strings.Contains(guidance.ValueSource, "full path") || !strings.Contains(guidance.ExampleBinding, "group/project") {
		t.Errorf("guidance = %+v, want it to offer the path", guidance)
	}
	if len(guidance.CommonConfusions) != 1 || !strings.Contains(guidance.CommonConfusions[0], "numeric ID alone") {
		t.Errorf("CommonConfusions = %q, want the project a path cannot find", guidance.CommonConfusions)
	}
}
