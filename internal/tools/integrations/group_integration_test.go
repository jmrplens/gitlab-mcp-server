// group_integration_test.go holds the tests of how an integration's path is
// built: integrationPath, the one place the project and group integration
// handlers build the path of a single integration, and the four handlers that
// build one, each held to the path it asks client-go for. The handlers' traffic
// with GitLab is driven through httptest in set_integration_test.go.
package integrations

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestIntegrationPath_AwkwardSlug_StaysOneSegment asserts that the scope id and
// the slug are each escaped into exactly one segment, for both scopes.
//
// The slug is the reason this exists. It is free text a caller types, so one
// carrying a slash names another endpoint of the same scope unless it is
// escaped, and no request a real integration makes can show whether it was:
// every slug GitLab has is lowercase letters and hyphens, which escaping leaves
// as they are. So the property is held here, on the function the four raw
// requests share, and on each handler by
// TestIntegrationRequests_RefusedBeforeSending_CarryEachValueInOneSegment,
// rather than through a request whose slug no integration has, which the
// request inventory would record as an endpoint GitLab does not have. The dots
// are written "%2E" because gl.PathEscape writes them that way; the transport
// restores them on the wire, which the handler tests assert.
func TestIntegrationPath_AwkwardSlug_StaysOneSegment(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		id     toolutil.StringOrInt
		slug   string
		want   string
	}{
		{
			name:   "a group path and a slug that climbs out of its endpoint",
			prefix: groupsPathPrefix,
			id:     "100% my group/sub.group",
			slug:   "../users/1 %",
			want:   "groups/100%25%20my%20group%2Fsub%2Egroup/integrations/%2E%2E%2Fusers%2F1%20%25",
		},
		{
			name:   "a project path and a slug naming a sibling integration",
			prefix: projectsPathPrefix,
			id:     "my group/my.project",
			slug:   "jira/../slack 100%",
			want:   "projects/my%20group%2Fmy%2Eproject/integrations/jira%2F%2E%2E%2Fslack%20100%25",
		},
		{
			name:   "a slug GitLab has, which escaping leaves as it is",
			prefix: groupsPathPrefix,
			id:     "7",
			slug:   "custom-issue-tracker",
			want:   "groups/7/integrations/custom-issue-tracker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := integrationPath(tt.prefix, tt.id, tt.slug)
			if got != tt.want {
				t.Errorf("integrationPath(%q, %q, %q) = %q, want %q", tt.prefix, tt.id, tt.slug, got, tt.want)
			}
			if segments := strings.Split(got, "/"); len(segments) != 4 {
				t.Errorf("integrationPath(%q, %q, %q) = %q, %d segments, want 4 whatever the values carry",
					tt.prefix, tt.id, tt.slug, got, len(segments))
			}
		})
	}
}

// TestIntegrationRequests_RefusedBeforeSending_CarryEachValueInOneSegment
// asserts, for each of the four handlers that build an integration path
// themselves, that the request it asks client-go for carries the scope id and
// the slug in one segment each, and that a request client-go refuses to build
// is returned as the handler's error.
//
// The request is refused by a client-wide request option, which client-go runs
// inside NewRequest, after the URL is built and before anything is sent. That
// is what lets an awkward slug be asserted per handler: the option reads the
// escaped path and refuses, so no request reaches the mock (ForbiddenHandler
// fails the test if one does) and nothing is written into the request
// inventory, whose endpoint check would read such a slug as an endpoint GitLab
// does not have. A handler that went back to building its path without
// escaping the slug fails here, not only integrationPath. The expected suffixes
// are written out rather than computed with integrationPath, so the handler is
// held to the escaping and not merely to calling the helper.
//
// The refusal is also the one input that reaches the error branch after each
// NewRequest call for a request with no body: a path that gl.PathEscape built
// always decodes, so that branch is entered only when a request option fails.
func TestIntegrationRequests_RefusedBeforeSending_CarryEachValueInOneSegment(t *testing.T) {
	const (
		awkwardGroup   = "100% my group/sub.group"
		awkwardProject = "my group/my.project"
		awkwardSlug    = "../users/1 %"
		groupSuffix    = "/groups/100%25%20my%20group%2Fsub%2Egroup/integrations/%2E%2E%2Fusers%2F1%20%25"
		projectSuffix  = "/projects/my%20group%2Fmy%2Eproject/integrations/%2E%2E%2Fusers%2F1%20%25"
	)
	config := map[string]any{"webhook": testWebhook}

	tests := []struct {
		name       string
		call       func(ctx context.Context, client *gitlabclient.Client) error
		wantMethod string
		wantSuffix string
	}{
		{
			name: "group get",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := GetGroupIntegration(ctx, client, GetGroupIntegrationInput{GroupID: awkwardGroup, Slug: awkwardSlug})
				return err
			},
			wantMethod: http.MethodGet,
			wantSuffix: groupSuffix,
		},
		{
			name: "group set",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := SetGroupIntegration(ctx, client, SetGroupIntegrationInput{GroupID: awkwardGroup, Slug: awkwardSlug, Config: config})
				return err
			},
			wantMethod: http.MethodPut,
			wantSuffix: groupSuffix,
		},
		{
			name: "group delete",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				return DeleteGroupIntegration(ctx, client, DeleteGroupIntegrationInput{GroupID: awkwardGroup, Slug: awkwardSlug})
			},
			wantMethod: http.MethodDelete,
			wantSuffix: groupSuffix,
		},
		{
			name: "project set",
			call: func(ctx context.Context, client *gitlabclient.Client) error {
				_, err := SetIntegration(ctx, client, SetIntegrationInput{ProjectID: awkwardProject, Slug: awkwardSlug, Config: config})
				return err
			},
			wantMethod: http.MethodPut,
			wantSuffix: projectSuffix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			errRefused := errors.New("request refused before sending")
			var method, escapedPath string
			refuse := func(req *retryablehttp.Request) error {
				method, escapedPath = req.Method, req.URL.EscapedPath()
				return errRefused
			}
			if err := gl.WithRequestOptions(refuse)(client.GL()); err != nil {
				t.Fatalf("installing the refusing request option: %v", err)
			}

			err := tt.call(t.Context(), client)
			if !errors.Is(err, errRefused) {
				t.Fatalf("error = %v, want it to wrap %v", err, errRefused)
			}
			if method != tt.wantMethod {
				t.Errorf("request method = %q, want %q", method, tt.wantMethod)
			}
			if !strings.HasSuffix(escapedPath, tt.wantSuffix) {
				t.Errorf("request path = %q, want it to end in %q", escapedPath, tt.wantSuffix)
			}
		})
	}
}
