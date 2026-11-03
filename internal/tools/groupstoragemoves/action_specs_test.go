// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains integration tests for the group storage move tool
// closures in ActionSpecs routes with a mock GitLab API.
package groupstoragemoves

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const (
	registerStorageMoveJSON = `[{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}]`
	registerSingleMoveJSON  = `{"id":1,"state":"finished","group":{"id":42,"web_url":"https://gitlab.example.com/groups/test","full_path":"test"},"source_storage_name":"default","destination_storage_name":"storage2"}`
)

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs) = %d, want 6", len(specs))
	}
	for _, spec := range specs {
		if spec.OwnerPackage != "groupstoragemoves" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch r.Method {
		case http.MethodGet:
			if strings.Contains(path, "repository_storage_moves") && !strings.HasSuffix(path, "/1") {
				testutil.RespondJSON(w, http.StatusOK, registerStorageMoveJSON)
			} else {
				testutil.RespondJSON(w, http.StatusOK, registerSingleMoveJSON)
			}
		case http.MethodPost:
			if strings.Contains(path, "all") {
				testutil.RespondJSON(w, http.StatusAccepted, `{"message":"202 Accepted"}`)
			} else {
				testutil.RespondJSON(w, http.StatusCreated, registerSingleMoveJSON)
			}
		default:
			http.NotFound(w, r)
		}
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
		{"gitlab_retrieve_all_group_storage_moves", map[string]any{}},
		{"gitlab_retrieve_group_storage_moves", map[string]any{"group_id": 42}},
		{"gitlab_get_group_storage_move", map[string]any{"id": 1}},
		{"gitlab_get_group_storage_move_for_group", map[string]any{"group_id": 42, "id": 1}},
		{"gitlab_schedule_group_storage_move", map[string]any{"group_id": 42, "destination_storage_name": "storage2"}},
		{"gitlab_schedule_all_group_storage_moves", map[string]any{"source_storage_name": "default", "destination_storage_name": "storage2"}},
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
		{"gitlab_retrieve_group_storage_moves", map[string]any{"group_id": "no/such"}},
		{"gitlab_get_group_storage_move_for_group", map[string]any{"group_id": "no/such", "id": 1}},
		{"gitlab_schedule_group_storage_move", map[string]any{"group_id": "no/such"}},
	} {
		t.Run(tool.name, func(t *testing.T) {
			var requests requestLog
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.add(r)
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			}))
			got, err := routeOf(t, client, tool.name).Handler(t.Context(), tool.args)
			if err != nil {
				t.Fatalf("Handler() error = %v, want the not-found output", err)
			}
			if want := (unresolvedOutput{Path: "no/such"}); got != want {
				t.Errorf("Handler() = %#v, want %#v", got, want)
			}
			if sent := requests.requests(); !slices.Equal(sent, []string{"GET /api/v4/groups/no%2Fsuch"}) {
				t.Errorf("requests = %q, want only the read of the path", sent)
			}
			assertNotFoundResult(t, got)
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
func assertNotFoundResult(t *testing.T, got any) {
	t.Helper()
	result := toolutil.MarkdownForResult(got)
	text := result.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{"Group Not Found", "**no/such**", "group.list", "numeric ID as group_id"} {
		if !strings.Contains(text, want) {
			t.Errorf("result = %q, want it to contain %q", text, want)
		}
	}
	if !result.IsError {
		t.Errorf("result.IsError = false, want a not-found result to be an error")
	}
}

// TestAnswerUnresolved_AnyOtherError_PassesThrough verifies the wrapper answers the
// path alone: a read refused for another reason, and a 404 from the storage move
// route once the path was found, stay errors, since the first is a permission and
// the second a group that is there.
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
			want: "verify group_id exists",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			route := answerUnresolved(toolutil.RouteAction(testutil.NewTestClient(t, tc.respond), RetrieveForGroup))
			got, err := route.Handler(t.Context(), map[string]any{"group_id": "a/b"})
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
	route := answerUnresolved(toolutil.RouteAction(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)), RetrieveForGroup))
	got, err := route.Bind(client)(t.Context(), map[string]any{"group_id": "a/b"})
	if err != nil {
		t.Fatalf("Bind(client)() error = %v, want the not-found output", err)
	}
	if want := (unresolvedOutput{Path: "a/b"}); got != want {
		t.Errorf("Bind(client)() = %#v, want %#v", got, want)
	}
}
