// action_specs_test.go contains canonical-route tests for project member actions.
package members

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Every endpoint of the member surface answers with a member of its own, so
// that the route a spec is built with is readable off the result. One fixture
// behind all of them is what made the routes interchangeable: exchanging Get
// and GetInherited in ActionSpecs, or Add and Edit, left the whole suite green
// while a model asking for a direct member was answered by the inherited
// endpoint, which reports the access a parent group grants and the project
// does not.
const (
	actionSpecMemberJSON          = `{"id":10,"username":"alice","name":"Alice","state":"active","access_level":30,"web_url":"https://gitlab.example.com/alice"}`
	actionSpecInheritedMemberJSON = `{"id":11,"username":"bruno","name":"Bruno","state":"active","access_level":40,"web_url":"https://gitlab.example.com/bruno"}`
	actionSpecListedMemberJSON    = `{"id":12,"username":"carla","name":"Carla","state":"active","access_level":20,"web_url":"https://gitlab.example.com/carla"}`
	actionSpecAddedMemberJSON     = `{"id":13,"username":"dana","name":"Dana","state":"active","access_level":10,"web_url":"https://gitlab.example.com/dana"}`
	actionSpecEditedMemberJSON    = `{"id":14,"username":"erik","name":"Erik","state":"active","access_level":50,"web_url":"https://gitlab.example.com/erik"}`
)

// actionSpecMember is the [Output] one of those fixtures must arrive as. The
// whole object is compared, since a route that reached the wrong endpoint
// differs in every field of it and a single-field check would rest on whichever
// field happened to be read.
func actionSpecMember(id int64, username, name string, accessLevel int) Output {
	return Output{
		ID:          id,
		Username:    username,
		Name:        name,
		State:       "active",
		AccessLevel: accessLevel,
		WebURL:      "https://gitlab.example.com/" + username,
	}
}

// assertRoutedMember holds one route's result to the member the endpoint it
// must reach answered with.
func assertRoutedMember(t *testing.T, result any, want Output) {
	t.Helper()
	got, ok := result.(Output)
	if !ok {
		t.Fatalf("route result = %T, want members.Output", result)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("route result = %#v, want %#v", got, want)
	}
}

// TestActionSpecs_CallAllRoutes exercises every project member tool through its
// canonical route, and holds each one to the endpoint it must have reached.
func TestActionSpecs_CallAllRoutes(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberActionHandler())))

	tests := []struct {
		name   string
		tool   string
		args   map[string]any
		verify func(t *testing.T, result any)
	}{
		{
			name: "list", tool: "gitlab_project_members_list", args: map[string]any{"project_id": "42"},
			verify: func(t *testing.T, result any) {
				t.Helper()
				out, ok := result.(ListOutput)
				if !ok {
					t.Fatalf("route result = %T, want members.ListOutput", result)
				}
				want := []Output{actionSpecMember(12, "carla", "Carla", 20)}
				if !reflect.DeepEqual(out.Members, want) {
					t.Errorf("listed members = %#v, want %#v", out.Members, want)
				}
			},
		},
		{
			name: "get", tool: "gitlab_project_member_get", args: map[string]any{"project_id": "42", "user_id": 10},
			verify: func(t *testing.T, result any) {
				t.Helper()
				assertRoutedMember(t, result, actionSpecMember(10, "alice", "Alice", 30))
			},
		},
		{
			name: "get_inherited", tool: "gitlab_project_member_get_inherited", args: map[string]any{"project_id": "42", "user_id": 10},
			verify: func(t *testing.T, result any) {
				t.Helper()
				assertRoutedMember(t, result, actionSpecMember(11, "bruno", "Bruno", 40))
			},
		},
		{
			name: "add", tool: "gitlab_project_member_add", args: map[string]any{"project_id": "42", "user_id": 10, "access_level": 30},
			verify: func(t *testing.T, result any) {
				t.Helper()
				assertRoutedMember(t, result, actionSpecMember(13, "dana", "Dana", 10))
			},
		},
		{
			name: "edit", tool: "gitlab_project_member_edit", args: map[string]any{"project_id": "42", "user_id": 10, "access_level": 30},
			verify: func(t *testing.T, result any) {
				t.Helper()
				assertRoutedMember(t, result, actionSpecMember(14, "erik", "Erik", 50))
			},
		},
		{
			name: "delete", tool: "gitlab_project_member_delete", args: map[string]any{"project_id": "42", "user_id": 10},
			verify: func(t *testing.T, result any) {
				t.Helper()
				if _, ok := result.(toolutil.DeleteOutput); !ok {
					t.Fatalf("route result = %T, want toolutil.DeleteOutput", result)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.tool)
			}
			tt.verify(t, result)
		})
	}
}

// TestActionSpecs_DeleteOutput verifies the delete route preserves its success message.
func TestActionSpecs_DeleteOutput(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, memberActionHandler())))

	result, err := byTool["gitlab_project_member_delete"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "user_id": 10})
	if err != nil {
		t.Fatalf("Route.Handler(gitlab_project_member_delete) error: %v", err)
	}
	out, ok := result.(toolutil.DeleteOutput)
	if !ok {
		t.Fatalf("Route.Handler(gitlab_project_member_delete) returned %T, want toolutil.DeleteOutput", result)
	}
	if out.Message != "Successfully deleted project member." {
		t.Fatalf("delete message = %q", out.Message)
	}
}

// TestActionSpecs_MutationErrors verifies mutating and lookup route failures propagate.
func TestActionSpecs_MutationErrors(t *testing.T) {
	byTool := memberSpecsByTool(t, ActionSpecs(testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
	}))))

	tests := []struct {
		tool string
		args map[string]any
	}{
		{"gitlab_project_member_add", map[string]any{"project_id": "42", "user_id": 1, "access_level": 30}},
		{"gitlab_project_member_edit", map[string]any{"project_id": "42", "user_id": 1, "access_level": 40}},
		{"gitlab_project_member_delete", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get", map[string]any{"project_id": "42", "user_id": 1}},
		{"gitlab_project_member_get_inherited", map[string]any{"project_id": "42", "user_id": 1}},
	}
	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			_, err := byTool[tt.tool].Route.Handler(t.Context(), tt.args)
			if err == nil {
				t.Fatal("expected route error")
			}
		})
	}
}

// TestCatalogSurface_DeleteConfirmDeclined covers destructive confirmation when the user declines.
func TestCatalogSurface_DeleteConfirmDeclined(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	byTool := memberSpecsByTool(t, ActionSpecs(client))

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	toolutil.RegisterSurfaceToolFromSpec(server, byTool["gitlab_project_member_delete"], toolutil.SurfaceToolRegisterOptions{
		Description: "Test project member destructive confirmation.",
		Icons:       toolutil.IconUser,
	})

	st, ct := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0.0.1"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, _ *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		},
	})
	session, err := mcpClient.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		session.Close()
		_ = serverSession.Wait()
	})

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "gitlab_project_member_delete",
		Arguments: map[string]any{"project_id": "42", "user_id": 1},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for declined confirmation")
	}
}

// memberDiscovery is what one member tool must be discoverable by and pointed
// at: the phrases a model's own words are matched against, the actions it leads
// to, and the opening of the usage line and the description that say which of
// the six it is.
type memberDiscovery struct {
	aliases     []string
	related     []string
	usage       string
	description string
}

// memberDiscoveryWant spells that out per tool, as literals. Nothing here used
// to read a branch's own words, only their shape, so any two branches of
// memberOptions could trade their usage, aliases and description whole and pass:
// gitlab_project_member_get would then be described as the one that includes
// parent-group inheritance, which is the difference a model consults that
// description to settle. The usage and description are pinned by their opening
// sentence rather than in full, because the rest of each names sibling actions
// through the same constants the surface publishes.
var memberDiscoveryWant = map[string]memberDiscovery{
	"gitlab_project_members_list": {
		aliases:     []string{"list project members", "who has access to project", "show project team", "project collaborators"},
		related:     []string{"project.member_get", "project.member_add", "project.get", "user.get"},
		usage:       "List the members of a project, including members inherited from parent groups.",
		description: "List a project's members (direct plus inherited from parent groups).",
	},
	"gitlab_project_member_get": {
		aliases:     []string{"get project member", "show member access level", "is user a member"},
		related:     []string{"project.members", "project.member_inherited", "project.member_edit", "project.member_delete"},
		usage:       "Get one direct project member by project_id plus user_id.",
		description: "Get a single direct project member by user ID.",
	},
	"gitlab_project_member_get_inherited": {
		aliases:     []string{"get inherited member", "member including inherited", "effective project access"},
		related:     []string{"project.member_get", "project.members", "project.get"},
		usage:       "Get a project member including membership inherited from any parent group.",
		description: "Get a project member including membership inherited from parent groups.",
	},
	"gitlab_project_member_add": {
		aliases:     []string{"add project member", "grant project access", "add existing user to project", "give user access"},
		related:     []string{"project.members", "project.member_get", "project.member_edit", "user.get"},
		usage:       "Add a user to a project team by user_id or username with an access_level.",
		description: "Add a user to a project team by user_id or username with an access level.",
	},
	"gitlab_project_member_edit": {
		aliases:     []string{"edit project member", "change member access level", "promote member", "update membership"},
		related:     []string{"project.members", "project.member_get", "project.member_add", "project.member_delete"},
		usage:       "Edit an existing project member's access_level, expires_at, or member_role_id.",
		description: "Edit a project member's access level, expiry, or custom member role.",
	},
	"gitlab_project_member_delete": {
		aliases:     []string{"remove project member", "revoke project access", "delete membership", "kick user from project"},
		related:     []string{"project.members", "project.member_get", "project.member_edit"},
		usage:       "Remove a member from a project by user_id.",
		description: "Remove a member from a project (destructive, requires confirmation).",
	},
}

// TestMemberOptions_EveryRegisteredTool_CarriesMetadataOfItsOwn verifies that
// each of the six tool names ActionSpecs registers is answered by a branch of
// memberOptions with its own usage, its own aliases, its own related actions
// and an individual-tool description, rather than falling through to the
// package defaults or carrying a sibling's words.
//
// Why it matters: memberOptions decides that per tool name, and a name with no
// branch is not an error: it yields a spec whose usage is the placeholder and
// whose individual description is empty. The surface is still registered and
// still routes, so a tool added to ActionSpecs and forgotten here ships with no
// description at all, which is the one piece of metadata a model reads to
// decide whether the tool is the one it wants.
func TestMemberOptions_EveryRegisteredTool_CarriesMetadataOfItsOwn(t *testing.T) {
	specs := ActionSpecs(testutil.NewTestClient(t, memberActionHandler()))
	if len(specs) != 6 {
		t.Fatalf("len(ActionSpecs()) = %d, want 6", len(specs))
	}

	defaults := memberOptions("gitlab_project_member_not_a_tool")
	for _, spec := range specs {
		t.Run(spec.IndividualTool.Name, func(t *testing.T) {
			name := spec.IndividualTool.Name
			want, ok := memberDiscoveryWant[name]
			if !ok {
				t.Fatalf("no expected discovery metadata is written for %s", name)
			}
			opts := memberOptions(name)
			if opts.Usage == defaults.Usage {
				t.Errorf("usage is still the package placeholder %q", opts.Usage)
			}
			if !strings.HasPrefix(opts.Usage, want.usage) {
				t.Errorf("Usage = %q, want it to open with %q", opts.Usage, want.usage)
			}
			if !strings.HasPrefix(opts.IndividualTool.Description, want.description) {
				t.Errorf("IndividualTool.Description = %q, want it to open with %q", opts.IndividualTool.Description, want.description)
			}
			// The tool's own name leads the list, so the expectation names the
			// natural-language phrases alone and this compares the whole of it.
			wantAliases := append([]string{name}, want.aliases...)
			if !slices.Equal(opts.Aliases, wantAliases) {
				t.Errorf("Aliases = %v, want %v", opts.Aliases, wantAliases)
			}
			if !slices.Equal(opts.RelatedActions, want.related) {
				t.Errorf("RelatedActions = %v, want %v", opts.RelatedActions, want.related)
			}
			assertMemberGuidanceRoles(t, opts.ParameterGuidance)
		})
	}
}

// assertMemberGuidanceRoles holds the two shared guidance blocks to the
// parameter each is keyed by. Both are package-level values reached by name, so
// exchanging their bodies moves every call site at once and no comparison
// against the value itself can see it.
func assertMemberGuidanceRoles(t *testing.T, guidance map[string]toolutil.ParameterGuidance) {
	t.Helper()
	roles := map[string]string{
		"project_id":   "scope_project",
		"user_id":      "user_id",
		"access_level": "access_level",
	}
	for param, wantRole := range roles {
		got, ok := guidance[param]
		if !ok {
			continue
		}
		if got.SemanticRole != wantRole {
			t.Errorf("ParameterGuidance[%q].SemanticRole = %q, want %q", param, got.SemanticRole, wantRole)
		}
	}
}

// TestMemberOptions_AnUnknownToolName_KeepsThePackageDefaults verifies what a
// name no branch matches is answered with: the shared tags, owner package and
// related actions, its own name as the only alias, and no individual-tool
// description.
//
// Why it matters: this is the fall-through the test above is measured against,
// and it is deliberately harmless rather than a panic or a half-filled spec —
// the defaults are a complete, if generic, spec. Stating it here is what makes
// "the description is empty" a fact about the branch that is missing rather
// than about whichever branch happened to run last.
func TestMemberOptions_AnUnknownToolName_KeepsThePackageDefaults(t *testing.T) {
	const name = "gitlab_project_member_not_a_tool"
	opts := memberOptions(name)

	if opts.Usage != "Use to execute members domain action." {
		t.Errorf("Usage = %q, want the package default", opts.Usage)
	}
	if opts.IndividualTool.Description != "" {
		t.Errorf("IndividualTool.Description = %q, want empty for a name no branch matches", opts.IndividualTool.Description)
	}
	if opts.IndividualTool.Name != name {
		t.Errorf("IndividualTool.Name = %q, want %q", opts.IndividualTool.Name, name)
	}
	if len(opts.Aliases) != 1 || opts.Aliases[0] != name {
		t.Errorf("Aliases = %v, want only %q", opts.Aliases, name)
	}
	if opts.ParameterGuidance != nil {
		t.Errorf("ParameterGuidance = %v, want none", opts.ParameterGuidance)
	}
	if opts.OwnerPackage != "members" {
		t.Errorf("OwnerPackage = %q, want %q", opts.OwnerPackage, "members")
	}
}

func memberActionHandler() http.Handler {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /api/v4/projects/42/members/all/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecInheritedMemberJSON)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/all", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[`+actionSpecListedMemberJSON+`]`)
	})
	handler.HandleFunc("GET /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecMemberJSON)
	})
	handler.HandleFunc("POST /api/v4/projects/42/members", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, actionSpecAddedMemberJSON)
	})
	handler.HandleFunc("PUT /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, actionSpecEditedMemberJSON)
	})
	handler.HandleFunc("DELETE /api/v4/projects/42/members/10", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return handler
}

func memberSpecsByTool(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		toolName := spec.IndividualTool.Name
		if toolName == "" {
			t.Fatalf("spec %s missing IndividualTool.Name", spec.Name)
		}
		if _, exists := byTool[toolName]; exists {
			t.Fatalf("duplicate individual tool %q", toolName)
		}
		byTool[toolName] = spec
	}
	return byTool
}
