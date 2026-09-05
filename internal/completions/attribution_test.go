// attribution_test.go covers the credential every completion runs under.
//
// The handler is built once per configuration shape and stores the
// credential-less client, so each search resolves the caller's own from the
// request context. Seventeen searches repeat that by hand, and one that forgot
// would look exactly like an editor whose suggestions are empty: the contract
// here is to answer with an empty list rather than an error, which is what makes
// a silent failure indistinguishable from a quiet GitLab.
package completions

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// completableArguments is every argument name [Handler.Complete] answers for,
// with the sibling arguments each one needs resolved before it will search.
var completableArguments = []struct {
	name     string
	ref      string
	resolved map[string]string
}{
	{name: "project_id", ref: refPrompt},
	{name: "group_id", ref: refPrompt},
	{name: "username", ref: refPrompt},
	{name: "merge_request_iid", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "issue_iid", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "from", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "to", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "ref", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "tag", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "pipeline_id", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "sha", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "branch", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "source_branch", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "target_branch", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "label", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "milestone_id", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "milestone", ref: refPrompt, resolved: map[string]string{"project_id": "42"}},
	{name: "milestone", ref: refPrompt, resolved: map[string]string{"group_id": "42"}},
	{name: "job_id", ref: refPrompt, resolved: map[string]string{"project_id": "42", "pipeline_id": "7"}},
	{name: "project_id", ref: refResource},
	{name: "group_id", ref: refResource},
	{name: "merge_request_iid", ref: refResource, resolved: map[string]string{"project_id": "42"}},
	{name: "issue_iid", ref: refResource, resolved: map[string]string{"project_id": "42"}},
}

// completeRequest builds a completion request for one argument, with the
// sibling arguments the dispatcher needs already resolved.
func completeRequest(ref, name string, resolved map[string]string) *mcp.CompleteRequest {
	req := &mcp.CompleteRequest{}
	req.Params = &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: ref, Name: "review_mr"},
		Argument: mcp.CompleteParamsArgument{Name: name, Value: "a"},
	}
	if len(resolved) > 0 {
		req.Params.Context = &mcp.CompleteContext{Arguments: resolved}
	}
	return req
}

// TestComplete_NeverUsesTheClientTheHandlerWasBuiltWith walks every argument the
// handler answers for and requires each search to go through the request's own
// credential.
//
// A search that used the stored client would reach nobody's GitLab on a shared
// server and answer with an empty list, which is also what a correct search of
// an empty instance answers. Counting requests on the two clients is what tells
// them apart.
func TestComplete_NeverUsesTheClientTheHandlerWasBuiltWith(t *testing.T) {
	var captured atomic.Int64
	stored := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		captured.Add(1)
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))
	handler := NewHandler(stored)

	for _, tt := range completableArguments {
		t.Run(tt.ref+"/"+tt.name, func(t *testing.T) {
			var hits atomic.Int64
			bound := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			}))
			ctx := gitlabclient.WithClient(t.Context(), bound)

			if _, err := handler.Complete(ctx, completeRequest(tt.ref, tt.name, tt.resolved)); err != nil {
				t.Fatalf("Complete: %v", err)
			}

			if hits.Load() == 0 {
				t.Errorf("completing %s searched no GitLab at all, so it cannot show which client it used", tt.name)
			}
			if captured.Load() != 0 {
				t.Errorf("completing %s searched through the client the handler was built with; on a shared server "+
					"that client carries no credential", tt.name)
			}
		})
	}
}

// TestComplete_AnUnattributedRequest_IsAnsweredEmptyAndSaidOutLoud covers the
// one failure this surface cannot report to the caller.
//
// The contract is that autocomplete is never blocked, so the answer stays an
// empty list. But every other cause of an empty completion is a GitLab hiccup
// that fixes itself, while this one is a wiring defect that never will, and it
// used to leave nothing on the wire and nothing in the log at the default level:
// an editor with no suggestions, forever, and no way to find out why.
func TestComplete_AnUnattributedRequest_IsAnsweredEmptyAndSaidOutLoud(t *testing.T) {
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	handler := NewHandler(gitlabclient.NewUnboundClient("https://gitlab.invalid"))

	result, err := handler.Complete(context.Background(), completeRequest(refPrompt, "project_id", nil))

	if err != nil {
		t.Fatalf("Complete returned %v; an unanswerable completion must not block the client", err)
	}
	if result == nil || len(result.Completion.Values) != 0 {
		t.Fatalf("Complete returned %v, want an empty completion", result)
	}
	if !strings.Contains(logged.String(), toolutil.UnattributedRequestMessage) {
		t.Errorf("nothing was logged at warn level about a completion that could not be attributed:\n%s", logged.String())
	}
}
