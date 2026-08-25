// registry_integration_test.go round-trips the subscription whitelist
// against the real resource registry. It lives in the external test package
// because internal/resources imports internal/subscriptions (the recorder
// appends the subscribable marker from the whitelist), so an internal test
// importing resources back would be an import cycle.
package subscriptions_test

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/subscriptions"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// mcpSession registers the real resource surface against a mock GitLab and
// returns a connected in-memory client session.
func mcpSession(t *testing.T, handler http.Handler) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()

	client := testutil.NewTestClient(t, handler)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0.0.1"}, nil)
	resources.Register(server, client)

	st, ct := mcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, st, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// registeredTemplates round-trips the real resource registry and returns
// every registered URI template.
func registeredTemplates(t *testing.T) []string {
	t.Helper()
	session := mcpSession(t, http.NotFoundHandler())

	var uris []string
	for tmpl, err := range session.ResourceTemplates(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list resource templates: %v", err)
		}
		uris = append(uris, tmpl.URITemplate)
	}
	if len(uris) == 0 {
		t.Fatal("no resource templates registered — the round-trip is not exercising the real registry")
	}
	return uris
}

// templateParam matches one "{param}" or "{+param}" placeholder.
var templateParam = regexp.MustCompile(`\{\+?([a-z_]+)\}`)

// concreteURI turns a URI template into a concrete URI by substituting a
// representative value for each placeholder. Identifiers GitLab models as
// integers get a number; everything else gets a name, and the reserved
// "{+path}" expansion gets a nested path so the file resource is exercised
// the way a real client would use it.
func concreteURI(template string) string {
	return templateParam.ReplaceAllStringFunc(template, func(match string) string {
		name := templateParam.FindStringSubmatch(match)[1]
		switch {
		case strings.HasPrefix(match, "{+"):
			return "dir/file.txt"
		case strings.HasSuffix(name, "_id"), strings.HasSuffix(name, "_iid"):
			return "1"
		default:
			return "name"
		}
	})
}

// wantSubscribable records, for every registered resource template, whether
// it is subscribable and as which kind. subscriptions.KindUnknown means "registered,
// deliberately not subscribable" — see the tail table's own comments for
// why each exclusion is there.
var wantSubscribable = map[string]subscriptions.Kind{
	"gitlab://group/{group_id}":                                        subscriptions.KindGroup,
	"gitlab://group/{group_id}/label/{label_id}":                       subscriptions.KindGroupLabel,
	"gitlab://group/{group_id}/members":                                subscriptions.KindUnknown,
	"gitlab://group/{group_id}/milestone/{milestone_iid}":              subscriptions.KindGroupMilestone,
	"gitlab://group/{group_id}/projects":                               subscriptions.KindUnknown,
	"gitlab://project/{project_id}":                                    subscriptions.KindProject,
	"gitlab://project/{project_id}/board/{board_id}":                   subscriptions.KindBoard,
	"gitlab://project/{project_id}/branch/{branch}":                    subscriptions.KindBranch,
	"gitlab://project/{project_id}/branches":                           subscriptions.KindUnknown,
	"gitlab://project/{project_id}/commit/{sha}":                       subscriptions.KindUnknown,
	"gitlab://project/{project_id}/deploy_key/{deploy_key_id}":         subscriptions.KindDeployKey,
	"gitlab://project/{project_id}/deployment/{deployment_id}":         subscriptions.KindDeployment,
	"gitlab://project/{project_id}/environment/{environment_id}":       subscriptions.KindEnvironment,
	"gitlab://project/{project_id}/feature_flag/{name}":                subscriptions.KindFeatureFlag,
	"gitlab://project/{project_id}/file/{ref}/{+path}":                 subscriptions.KindFile,
	"gitlab://project/{project_id}/issue/{issue_iid}":                  subscriptions.KindIssue,
	"gitlab://project/{project_id}/issues":                             subscriptions.KindUnknown,
	"gitlab://project/{project_id}/job/{job_id}":                       subscriptions.KindJob,
	"gitlab://project/{project_id}/label/{label_id}":                   subscriptions.KindLabel,
	"gitlab://project/{project_id}/labels":                             subscriptions.KindUnknown,
	"gitlab://project/{project_id}/members":                            subscriptions.KindUnknown,
	"gitlab://project/{project_id}/milestone/{milestone_iid}":          subscriptions.KindMilestone,
	"gitlab://project/{project_id}/milestones":                         subscriptions.KindUnknown,
	"gitlab://project/{project_id}/mr/{merge_request_iid}":             subscriptions.KindMergeRequest,
	"gitlab://project/{project_id}/mr/{merge_request_iid}/discussions": subscriptions.KindMergeRequestDiscussions,
	"gitlab://project/{project_id}/mr/{merge_request_iid}/notes":       subscriptions.KindMergeRequestNotes,
	"gitlab://project/{project_id}/pipeline/{pipeline_id}":             subscriptions.KindPipeline,
	"gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs":        subscriptions.KindPipelineJobs,
	"gitlab://project/{project_id}/pipelines/latest":                   subscriptions.KindPipelineLatest,
	"gitlab://project/{project_id}/release/{tag_name}":                 subscriptions.KindRelease,
	"gitlab://project/{project_id}/releases":                           subscriptions.KindUnknown,
	"gitlab://project/{project_id}/snippet/{snippet_id}":               subscriptions.KindProjectSnippet,
	"gitlab://project/{project_id}/tag/{tag_name}":                     subscriptions.KindTag,
	"gitlab://project/{project_id}/tags":                               subscriptions.KindUnknown,
	"gitlab://project/{project_id}/wiki/{slug}":                        subscriptions.KindWiki,
	"gitlab://snippet/{snippet_id}":                                    subscriptions.KindSnippet,
}

// TestClassify_MatchesRegisteredResourceTemplates is the drift guard: every
// resource this server registers must have an explicit subscribability
// decision here, and every decision must still match a template that exists.
//
// Without this, the failure mode is silent in both directions. A new
// resource would arrive with no decision at all. A renamed template would
// leave its whitelist entry unreachable, quietly making that resource
// unsubscribable. Both look correct in isolation; only a comparison against
// the live registry catches them.
func TestClassify_MatchesRegisteredResourceTemplates(t *testing.T) {
	registered := registeredTemplates(t)
	seen := make(map[string]bool, len(registered))

	for _, tmpl := range registered {
		seen[tmpl] = true
		t.Run(tmpl, func(t *testing.T) {
			expected, decided := wantSubscribable[tmpl]
			if !decided {
				t.Errorf("resource template %q has no subscribability decision — add it to wantSubscribable "+
					"and, if subscribable, to the tail table in kind.go", tmpl)
				return
			}
			uri := concreteURI(tmpl)
			got, ok := subscriptions.Classify(uri)
			if got != expected {
				t.Errorf("subscriptions.Classify(%q) = %v, want %v", uri, got, expected)
			}
			if ok != (expected != subscriptions.KindUnknown) {
				t.Errorf("subscriptions.Classify(%q) subscribable = %v, want %v", uri, ok, expected != subscriptions.KindUnknown)
			}
		})
	}

	for tmpl := range wantSubscribable {
		if !seen[tmpl] {
			t.Errorf("wantSubscribable decides %q, but no such resource template is registered — "+
				"it was renamed or removed, leaving its whitelist entry unreachable", tmpl)
		}
	}

	// Every subscribable kind must name a template that is actually
	// registered, since that string is how a watcher finds the handler to
	// re-read through. A stale entry here would make the resource
	// unreadable at poll time while still classifying as subscribable.
	for tmpl, kind := range wantSubscribable {
		if kind == subscriptions.KindUnknown {
			continue
		}
		got := kind.Template()
		if got == "" {
			t.Errorf("%v has no Template(), so a watcher cannot resolve its handler", kind)
			continue
		}
		if got != tmpl {
			t.Errorf("%v.Template() = %q, want %q", kind, got, tmpl)
		}
		if !seen[got] {
			t.Errorf("%v.Template() = %q, which is not a registered resource template", kind, got)
		}
	}
}

// TestClassify_AgreesWithResourceRouter verifies the whitelist never accepts
// a URI the resource router cannot resolve.
//
// This is the guard that matters most, and it caught a real bug: an earlier
// revision allowed unencoded slashes in namespace references, branch names
// and wiki slugs. Those URIs classified as subscribable but every
// resources/read for them returned "resource not found", so a subscription
// would have been acknowledged and then never fired — the worst available
// outcome, since the client has no signal that anything is wrong.
//
// The check is one-directional on purpose: a routable resource may be
// deliberately non-subscribable (collections, commits), but a subscribable
// URI must always be routable.
// Routing is detected by whether the resource handler reached the GitLab
// API rather than by inspecting the read error. That keeps the check
// independent of response decoding, which matters for two reasons: the
// mock cannot produce a body every one of the 36 handlers decodes, and
// client-go v2.58.2's Issue.UnmarshalJSON panics outright on a JSON object
// with no "id" field (reflect.TypeOf(nil).subscriptions.Kind()), so feeding handlers
// synthetic bodies is a trap. A 404 from the mock exercises the full
// routing path and stops cleanly before any decoding.
func TestClassify_AgreesWithResourceRouter(t *testing.T) {
	var reachedAPI atomic.Bool
	session := mcpSession(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reachedAPI.Store(true)
		http.Error(w, `{"message":"404 Not Found"}`, http.StatusNotFound)
	}))
	ctx := context.Background()

	for _, tmpl := range registeredTemplates(t) {
		uri := concreteURI(tmpl)
		kind, subscribable := subscriptions.Classify(uri)
		if !subscribable {
			continue
		}
		t.Run(kind.String(), func(t *testing.T) {
			reachedAPI.Store(false)
			_, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
			if !reachedAPI.Load() {
				t.Errorf("subscriptions.Classify(%q) = %v, subscribable — but reading it never reached GitLab, "+
					"so the router could not resolve the URI (read error: %v)", uri, kind, err)
			}
		})
	}
}
