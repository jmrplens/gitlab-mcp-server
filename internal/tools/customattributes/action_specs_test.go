// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go verifies what a route answers when the path or the
// username in resource_id names nothing, through the wrapper the catalog routes
// every custom attribute action with.
package customattributes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// routes builds the route of each of the four actions the way the catalog does.
func routes(client *gitlabclient.Client) map[string]toolutil.ActionRoute {
	return map[string]toolutil.ActionRoute{
		"list":   AnswerUnresolved(toolutil.RouteAction(client, List)),
		"get":    AnswerUnresolved(toolutil.RouteAction(client, Get)),
		"set":    AnswerUnresolved(toolutil.RouteAction(client, Set)),
		"delete": AnswerUnresolved(toolutil.DestructiveAction(client, DeleteOutput)),
	}
}

// routeArgs is the arguments of a call of each action, on the named resource.
func routeArgs(action, resourceType, resourceID string) map[string]any {
	args := map[string]any{"resource_type": resourceType, "resource_id": resourceID}
	if action != "list" {
		args["key"] = testKeyDept
	}
	if action == "set" {
		args["value"] = "eng"
	}
	return args
}

// lookupOf is the path of the read each resource type's name goes through.
var lookupOf = map[string]string{
	testTypeUser:    "/api/v4/users",
	testTypeGroup:   "/api/v4/groups/no%2Fsuch",
	testTypeProject: "/api/v4/projects/no%2Fsuch",
}

// TestAnswerUnresolved_ANameThatNamesNothing_IsANotFoundResultWithAHint verifies
// that a path or a username GitLab finds nothing for answers each of the four
// actions with a not-found result naming it, sends only the read that looked for
// it, and reaches the formatter the result is rendered with. It is the difference
// between a model told the name is wrong and told where to read it, and one told
// the action failed.
func TestAnswerUnresolved_ANameThatNamesNothing_IsANotFoundResultWithAHint(t *testing.T) {
	for action := range routes(nil) {
		for typ := range lookupOf {
			t.Run(action+"/"+typ, func(t *testing.T) {
				assertNameNotFound(t, action, typ)
			})
		}
	}
}

// assertNameNotFound runs one action on a name of one resource type that names
// nothing, and holds the route to the not-found result.
func assertNameNotFound(t *testing.T, action, typ string) {
	t.Helper()
	var requests requestLog
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r)
		if typ == testTypeUser {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))

	got, err := routes(client)[action].Handler(t.Context(), routeArgs(action, typ, "no/such"))
	if err != nil {
		t.Fatalf("Handler() error = %v, want the not-found output", err)
	}
	if want := (unresolvedOutput{ResourceType: typ, Identifier: "no/such"}); got != want {
		t.Errorf("Handler() = %#v, want %#v", got, want)
	}
	if sent := requests.paths(); len(sent) != 1 || sent[0] != lookupOf[typ] {
		t.Errorf("requests = %q, want only the read of %q", sent, lookupOf[typ])
	}

	hint := typ + ".list"
	result := toolutil.MarkdownForResult(got)
	text := resultText(t, result)
	if !result.IsError || !strings.Contains(text, "no/such") || !strings.Contains(text, hint) {
		t.Errorf("result = %q (error %v), want an error naming no/such and the hint %s", text, result.IsError, hint)
	}
}

// resultText reads the one text block of a result.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want text", result.Content[0])
	}
	return text.Text
}

// TestFormatUnresolved_EachResourceType_NamesItAndWhereItsNameIsRead verifies the
// result of each resource type, label and hints, rather than only that one of
// them is there: a user's has no path to look up, and a hint spelled for another
// resource sends a model to a list that cannot answer it.
func TestFormatUnresolved_EachResourceType_NamesItAndWhereItsNameIsRead(t *testing.T) {
	cases := []struct {
		resourceType string
		want         []string
	}{
		{testTypeUser, []string{"User Not Found", "**grace**", "user.list", "username", "numeric ID as resource_id"}},
		{testTypeGroup, []string{"Group Not Found", "**grace**", "group.list", "full path", "numeric ID as resource_id"}},
		{testTypeProject, []string{"Project Not Found", "**grace**", "project.list", "full path", "numeric ID as resource_id"}},
	}
	for _, tc := range cases {
		t.Run(tc.resourceType, func(t *testing.T) {
			text := resultText(t, formatUnresolved(unresolvedOutput{ResourceType: tc.resourceType, Identifier: "grace"}))
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("result = %q, want it to contain %q", text, want)
				}
			}
		})
	}
}

// TestAnswerUnresolved_AnyOtherError_PassesThrough verifies the wrapper answers
// the name alone: a lookup refused for another reason, and a 404 from the
// attribute route once the name was found, stay errors, since the first is a
// permission and the second a resource that is there.
func TestAnswerUnresolved_AnyOtherError_PassesThrough(t *testing.T) {
	cases := []struct {
		name    string
		respond http.HandlerFunc
		want    string
	}{
		{
			name: "the lookup is refused",
			respond: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			},
			want: "403",
		},
		{
			name: "the attribute route answers 404",
			respond: func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "custom_attributes") {
					testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Custom Attribute Not Found"}`)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, `{"id":7}`)
			},
			want: hintMarker,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tc.respond)
			got, err := routes(client)["get"].Handler(t.Context(), routeArgs("get", testTypeProject, "a/b"))
			if err == nil {
				t.Fatalf("Handler() = %#v, want an error", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if errors.As(err, new(*unresolvedError)) {
				t.Errorf("error = %#v, want no unresolved name", err)
			}
		})
	}
}

// TestUnresolvedError_NamesWhatWasLookedUpAndKeepsGitLabsAnswer verifies the error
// a handler returns when no wrapper answers it: it says what was looked for, and
// the 404 behind it stays readable.
func TestUnresolvedError_NamesWhatWasLookedUpAndKeepsGitLabsAnswer(t *testing.T) {
	cause := errors.New("404")
	err := &unresolvedError{resourceType: testTypeProject, identifier: "a/b", cause: cause}
	if want := `no project named "a/b" was found`; err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
	if !errors.Is(err, cause) {
		t.Errorf("errors.Is(%v, cause) = false", err)
	}
}

// TestAnswerUnresolved_ARebinding_KeepsTheAnswer verifies the shared catalog
// builds the wrapped handler for another client too, since HTTP mode binds each
// request's credential onto the route rather than using the client it was built
// with.
func TestAnswerUnresolved_ARebinding_KeepsTheAnswer(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
	}))
	route := routes(testutil.NewTestClient(t, testutil.ForbiddenHandler(t)))["list"]
	got, err := route.Bind(client)(context.Background(), routeArgs("list", testTypeGroup, "a/b"))
	if err != nil {
		t.Fatalf("Bind(client)() error = %v, want the not-found output", err)
	}
	if want := (unresolvedOutput{ResourceType: testTypeGroup, Identifier: "a/b"}); got != want {
		t.Errorf("Bind(client)() = %#v, want %#v", got, want)
	}
}
