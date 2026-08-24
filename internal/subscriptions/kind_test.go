// kind_test.go validates the subscribable-URI whitelist: which concrete
// resource URIs a client may subscribe to, and — just as load-bearing —
// which ones must be rejected. The rejection half is not defensive
// paranoia: the MCP SDK does not verify that a subscribed URI names a
// registered resource, and at least one shipping client sends
// resources/subscribe even against a server advertising subscribe: false,
// so anything Classify lets through becomes a silent, never-notified
// subscription.
//
// Two tests at the end are guards against drift rather than unit tests of
// their own: one round-trips the real resource registry so a template that
// is added, renamed or removed cannot slip past this table, and one reads
// every template through the real router so the whitelist can never accept
// a URI the router would answer with "resource not found".
package subscriptions

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/resources"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// TestClassify_EverySubscribableShape_ReturnsItsKind verifies each
// subscribable URI layout classifies as its own kind.
func TestClassify_EverySubscribableShape_ReturnsItsKind(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Kind
	}{
		{"project", "gitlab://project/42", KindProject},
		{"pipeline", "gitlab://project/42/pipeline/99", KindPipeline},
		{"pipeline jobs", "gitlab://project/42/pipeline/99/jobs", KindPipelineJobs},
		{"latest pipeline", "gitlab://project/42/pipelines/latest", KindPipelineLatest},
		{"job", "gitlab://project/42/job/7", KindJob},
		{"merge request", "gitlab://project/42/mr/12", KindMergeRequest},
		{"mr discussions", "gitlab://project/42/mr/12/discussions", KindMergeRequestDiscussions},
		{"mr notes", "gitlab://project/42/mr/12/notes", KindMergeRequestNotes},
		{"issue", "gitlab://project/42/issue/3", KindIssue},
		{"deployment", "gitlab://project/42/deployment/5", KindDeployment},
		{"environment", "gitlab://project/42/environment/2", KindEnvironment},
		{"feature flag", "gitlab://project/42/feature_flag/new_ui", KindFeatureFlag},
		{"release", "gitlab://project/42/release/v1.2.3", KindRelease},
		{"tag", "gitlab://project/42/tag/v1.2.3", KindTag},
		{"branch", "gitlab://project/42/branch/main", KindBranch},
		{"milestone", "gitlab://project/42/milestone/8", KindMilestone},
		{"label", "gitlab://project/42/label/4", KindLabel},
		{"board", "gitlab://project/42/board/1", KindBoard},
		{"deploy key", "gitlab://project/42/deploy_key/6", KindDeployKey},
		{"project snippet", "gitlab://project/42/snippet/11", KindProjectSnippet},
		{"wiki", "gitlab://project/42/wiki/home", KindWiki},
		{"file", "gitlab://project/42/file/main/README.md", KindFile},
		{"file in nested directory", "gitlab://project/42/file/main/src/pkg/main.go", KindFile},
		{"group", "gitlab://group/9", KindGroup},
		{"group label", "gitlab://group/9/label/4", KindGroupLabel},
		{"group milestone", "gitlab://group/9/milestone/8", KindGroupMilestone},
		{"snippet", "gitlab://snippet/5", KindSnippet},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Classify(tt.uri)
			if !ok {
				t.Fatalf("Classify(%q) reported not subscribable, want %v", tt.uri, tt.want)
			}
			if got != tt.want {
				t.Errorf("Classify(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

// TestClassify_EncodedNamespaceReference_IsAccepted verifies a path-style
// namespace reference classifies correctly when percent-encoded.
//
// Encoding is the only form that works: the registered templates expand
// their variables as RFC 6570 simple strings, which do not match "/", so a
// raw-slash reference cannot be routed at all (see
// TestClassify_AgreesWithResourceRouter). Encoding also removes the
// ambiguity a raw slash would otherwise create — "gitlab://project/42/tag/
// main" could be read as either the tag "main" of project 42 or a project
// at path "42/tag" — because an encoded reference contains no slash to
// split on.
func TestClassify_EncodedNamespaceReference_IsAccepted(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Kind
	}{
		{"project path", "gitlab://project/group%2Fproj", KindProject},
		{"project path with tail", "gitlab://project/group%2Fproj/pipeline/99", KindPipeline},
		{"nested subgroups", "gitlab://project/group%2Fsub%2Fproj/mr/12", KindMergeRequest},
		{"project path with suffixed tail", "gitlab://project/group%2Fproj/pipeline/99/jobs", KindPipelineJobs},
		{"project path with literal tail", "gitlab://project/group%2Fproj/pipelines/latest", KindPipelineLatest},
		{"group path", "gitlab://group/parent%2Fchild", KindGroup},
		{"group path with tail", "gitlab://group/parent%2Fchild/milestone/8", KindGroupMilestone},
		{"encoded branch name", "gitlab://project/42/branch/feature%2Fnew-ui", KindBranch},
		{"encoded wiki slug", "gitlab://project/42/wiki/parent%2Fchild", KindWiki},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Classify(tt.uri)
			if !ok {
				t.Fatalf("Classify(%q) reported not subscribable, want %v", tt.uri, tt.want)
			}
			if got != tt.want {
				t.Errorf("Classify(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

// TestClassify_RawSlashInSingleSegmentValue_IsRejected verifies a value
// carrying an unencoded slash is refused rather than reinterpreted.
//
// The router cannot resolve these, so accepting them would acknowledge a
// subscription whose every poll returns "resource not found" — the client
// then waits forever for a notification that can never arrive.
func TestClassify_RawSlashInSingleSegmentValue_IsRejected(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"raw slash in project reference", "gitlab://project/group/proj"},
		{"raw slash in group reference", "gitlab://group/parent/child"},
		{"raw slash in branch name", "gitlab://project/42/branch/feature/x"},
		{"raw slash in wiki slug", "gitlab://project/42/wiki/parent/child"},
		{"raw slash in tag name", "gitlab://project/42/tag/release/2026.1"},
		{"raw slash in feature flag name", "gitlab://project/42/feature_flag/group/name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := Classify(tt.uri); ok {
				t.Errorf("Classify(%q) = %v, true; want rejected — the router cannot resolve a raw slash here", tt.uri, got)
			}
		})
	}
}

// TestClassify_SuffixedTail_TakesPrecedenceOverBareTail verifies a
// sub-collection URI never degrades into its parent kind.
func TestClassify_SuffixedTail_TakesPrecedenceOverBareTail(t *testing.T) {
	tests := []struct {
		bare, suffixed         string
		bareKind, suffixedKind Kind
	}{
		{
			bare: "gitlab://project/42/pipeline/99", bareKind: KindPipeline,
			suffixed: "gitlab://project/42/pipeline/99/jobs", suffixedKind: KindPipelineJobs,
		},
		{
			bare: "gitlab://project/42/mr/12", bareKind: KindMergeRequest,
			suffixed: "gitlab://project/42/mr/12/discussions", suffixedKind: KindMergeRequestDiscussions,
		},
		{
			bare: "gitlab://project/42/mr/12", bareKind: KindMergeRequest,
			suffixed: "gitlab://project/42/mr/12/notes", suffixedKind: KindMergeRequestNotes,
		},
	}
	for _, tt := range tests {
		t.Run(tt.suffixedKind.String(), func(t *testing.T) {
			if got, _ := Classify(tt.bare); got != tt.bareKind {
				t.Errorf("Classify(%q) = %v, want %v", tt.bare, got, tt.bareKind)
			}
			if got, _ := Classify(tt.suffixed); got != tt.suffixedKind {
				t.Errorf("Classify(%q) = %v, want %v", tt.suffixed, got, tt.suffixedKind)
			}
		})
	}
}

// TestClassify_CollectionAndImmutableResources_AreRejected verifies the two
// deliberate exclusions are refused.
//
// Collections are excluded because any change anywhere in the namespace
// invalidates them, turning one subscription into a notification firehose.
// Commits are excluded for the opposite reason: their content is immutable,
// so a watcher would poll forever and never notify.
func TestClassify_CollectionAndImmutableResources_AreRejected(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"project branches", "gitlab://project/42/branches"},
		{"project issues", "gitlab://project/42/issues"},
		{"project labels", "gitlab://project/42/labels"},
		{"project members", "gitlab://project/42/members"},
		{"project milestones", "gitlab://project/42/milestones"},
		{"project releases", "gitlab://project/42/releases"},
		{"project tags", "gitlab://project/42/tags"},
		{"group members", "gitlab://group/9/members"},
		{"group projects", "gitlab://group/9/projects"},
		{"commit is immutable", "gitlab://project/42/commit/abc123"},

		// The same exclusions with an encoded namespace reference.
		{"encoded project issues", "gitlab://project/group%2Fproj/issues"},
		{"encoded group members", "gitlab://group/parent%2Fchild/members"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Classify(tt.uri)
			if ok {
				t.Errorf("Classify(%q) = %v, true; want rejected", tt.uri, got)
			}
			if got != KindUnknown {
				t.Errorf("Classify(%q) kind = %v, want KindUnknown on rejection", tt.uri, got)
			}
		})
	}
}

// TestClassify_MalformedURI_IsRejected verifies structurally invalid URIs
// are rejected rather than coerced into some nearby shape.
func TestClassify_MalformedURI_IsRejected(t *testing.T) {
	tests := []struct {
		name string
		uri  string
	}{
		{"empty", ""},
		{"project prefix only", "gitlab://project/"},
		{"group prefix only", "gitlab://group/"},
		{"snippet prefix only", "gitlab://snippet/"},
		{"wrong scheme", "https://gitlab.com/project/42/pipeline/99"},
		{"unknown root", "gitlab://user/7"},
		{"static resource", "gitlab://tools"},
		{"empty namespace reference", "gitlab://project//pipeline/99"},
		{"missing identifier", "gitlab://project/42/pipeline/"},
		{"unknown trailing segment", "gitlab://project/42/pipeline/99/bogus"},
		{"extra segment after suffix", "gitlab://project/42/pipeline/99/jobs/1"},
		{"empty id before suffix", "gitlab://project/42/pipeline//jobs"},
		{"unknown resource segment", "gitlab://project/42/widget/1"},
		{"file without a path", "gitlab://project/42/file/main"},
		{"file with empty path", "gitlab://project/42/file/main/"},
		{"non-numeric snippet", "gitlab://snippet/abc"},
		{"snippet with tail", "gitlab://snippet/5/notes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := Classify(tt.uri); ok {
				t.Errorf("Classify(%q) = %v, true; want rejected", tt.uri, got)
			}
		})
	}
}

// TestClassify_NumericIdentifier_MatchesGitLabSemantics verifies numeric
// identifier handling matches what GitLab itself accepts.
//
// Leading zeros are accepted on purpose: verified against a live instance,
// GET /projects/02317 and GET /projects/2317 return the same project, and
// this server's own read path parses identifiers with strconv.ParseInt,
// which accepts them too — so rejecting them here would create a URI a
// client can read but not subscribe to. Zero, negatives and signs are
// rejected because GitLab identifiers are auto-increment integers starting
// at 1, so none of those can ever name a real object; GitLab answers 404
// for "+2317" specifically, also verified live.
func TestClassify_NumericIdentifier_MatchesGitLabSemantics(t *testing.T) {
	accepted := []struct{ name, uri string }{
		{"bare", "gitlab://project/42/pipeline/2317"},
		{"zero padded", "gitlab://project/42/pipeline/02317"},
		{"heavily padded", "gitlab://project/42/pipeline/0000001"},
	}
	for _, tt := range accepted {
		t.Run("accepted/"+tt.name, func(t *testing.T) {
			if got, ok := Classify(tt.uri); !ok {
				t.Errorf("Classify(%q) = %v, false; want accepted (GitLab resolves it)", tt.uri, got)
			}
		})
	}

	rejected := []struct{ name, uri string }{
		{"zero", "gitlab://project/42/pipeline/0"},
		{"all zeros", "gitlab://project/42/pipeline/000"},
		{"negative", "gitlab://project/42/pipeline/-1"},
		{"explicit plus", "gitlab://project/42/pipeline/+2317"},
		{"decimal", "gitlab://project/42/pipeline/1.5"},
		{"thousands separator", "gitlab://project/42/issue/1_000"},
		{"trailing space", "gitlab://project/42/job/7 "},
		{"digits then letters", "gitlab://project/42/mr/12abc"},
		{"word", "gitlab://project/42/pipeline/latest"},
	}
	for _, tt := range rejected {
		t.Run("rejected/"+tt.name, func(t *testing.T) {
			if got, ok := Classify(tt.uri); ok {
				t.Errorf("Classify(%q) = %v, true; want rejected", tt.uri, got)
			}
		})
	}
}

// TestClassify_FreeFormIdentifier_AcceptsNonNumeric verifies tails whose
// identifier is a name rather than a number still accept one, so the
// numeric tightening does not leak onto them.
func TestClassify_FreeFormIdentifier_AcceptsNonNumeric(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want Kind
	}{
		{"semver tag", "gitlab://project/42/release/v1.2.3", KindRelease},
		{"numeric-looking tag", "gitlab://project/42/release/2026", KindRelease},
		{"zero-looking tag", "gitlab://project/42/tag/0", KindTag},
		{"wiki slug", "gitlab://project/42/wiki/getting-started", KindWiki},
		{"flag with underscores", "gitlab://project/42/feature_flag/new_checkout_flow", KindFeatureFlag},
		// Labels are addressable by name as well as by ID, and the
		// resource handler resolves both — so refusing the name form would
		// leave a URI a client can read but not subscribe to.
		{"label by name", "gitlab://project/42/label/bug", KindLabel},
		{"label by id", "gitlab://project/42/label/7", KindLabel},
		{"group label by name", "gitlab://group/7/label/needs-review", KindGroupLabel},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Classify(tt.uri)
			if !ok {
				t.Fatalf("Classify(%q) reported not subscribable, want %v", tt.uri, tt.want)
			}
			if got != tt.want {
				t.Errorf("Classify(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

// TestKindString_EveryKind_HasADistinctName verifies each kind renders a
// distinct, non-empty name, since these strings appear in logs and
// rejection messages where "unknown" for a real kind would mislead.
func TestKindString_EveryKind_HasADistinctName(t *testing.T) {
	seen := make(map[string]Kind, len(kindNames))
	for k, name := range kindNames {
		if name == "" || name == "unknown" {
			t.Errorf("Kind(%d).String() = %q, want a distinct name", k, name)
			continue
		}
		if prev, dup := seen[name]; dup {
			t.Errorf("Kind(%d) and Kind(%d) both render %q", prev, k, name)
		}
		seen[name] = k
	}
}

// TestKindString_UnknownAndOutOfRange_RenderUnknown verifies the zero value
// and any future unmapped value degrade to "unknown" rather than an empty
// string.
func TestKindString_UnknownAndOutOfRange_RenderUnknown(t *testing.T) {
	if got := KindUnknown.String(); got != "unknown" {
		t.Errorf("KindUnknown.String() = %q, want %q", got, "unknown")
	}
	if got := Kind(200).String(); got != "unknown" {
		t.Errorf("Kind(200).String() = %q, want %q", got, "unknown")
	}
}

// TestMatchSegments_PathWildcard_OnlyMatchesTrailingSegments verifies the
// greedy wildcard behaves as documented: last position only, at least one
// segment, and no empty segment inside the run.
func TestMatchSegments_PathWildcard_OnlyMatchesTrailingSegments(t *testing.T) {
	tests := []struct {
		name    string
		segs    []string
		pattern []string
		want    bool
	}{
		{"consumes one segment", []string{"file", "main", "a.txt"}, []string{"file", patName, patPath}, true},
		{"consumes many segments", []string{"file", "main", "a", "b", "c"}, []string{"file", patName, patPath}, true},
		{"needs at least one segment", []string{"file", "main"}, []string{"file", patName, patPath}, false},
		{"rejects empty trailing segment", []string{"file", "main", ""}, []string{"file", patName, patPath}, false},
		{"rejects empty inner segment", []string{"file", "main", "a", "", "c"}, []string{"file", patName, patPath}, false},
		{"not last is never matched", []string{"file", "main", "a"}, []string{"file", patPath, patName}, false},
		{"exact arity required without wildcard", []string{"pipeline", "1", "extra"}, []string{"pipeline", patNumeric}, false},
		{"too few segments", []string{"pipeline"}, []string{"pipeline", patNumeric}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchSegments(tt.segs, tt.pattern); got != tt.want {
				t.Errorf("matchSegments(%q, %q) = %v, want %v", tt.segs, tt.pattern, got, tt.want)
			}
		})
	}
}

// TestClassify_TailPatternsAreMutuallyExclusive verifies no concrete URI can
// satisfy two entries in the tail table.
//
// The table is written as if order does not matter, and this is what makes
// that true. Two overlapping patterns would make the classification depend
// on declaration order — the exact bug class that let ".../tag/release/x"
// classify as a release in an earlier revision of this file.
func TestClassify_TailPatternsAreMutuallyExclusive(t *testing.T) {
	for i, a := range tails {
		for j, b := range tails {
			if i >= j || a.prefix != b.prefix {
				continue
			}
			sample := sampleSegments(a.segs)
			if matchSegments(sample, b.segs) {
				t.Errorf("tail %v (%q) and tail %v (%q) both match %q — declaration order decides, which it must not",
					a.kind, a.segs, b.kind, b.segs, sample)
			}
		}
	}
}

// sampleSegments turns a pattern into a representative concrete segment
// list, substituting a value for each wildcard.
func sampleSegments(pattern []string) []string {
	out := make([]string, 0, len(pattern)+1)
	for _, p := range pattern {
		switch p {
		case patNumeric:
			out = append(out, "1")
		case patName:
			out = append(out, "name")
		case patPath:
			out = append(out, "dir", "file.txt")
		default:
			out = append(out, p)
		}
	}
	return out
}

// TestClassify_EveryTailIsReachable verifies no entry in the tail table is
// shadowed, so no resource silently becomes unsubscribable.
func TestClassify_EveryTailIsReachable(t *testing.T) {
	for _, tl := range tails {
		uri := tl.prefix + "42/" + strings.Join(sampleSegments(tl.segs), "/")
		got, ok := Classify(uri)
		if got != tl.kind {
			t.Errorf("tail %q: Classify(%q) = %v, want %v", tl.segs, uri, got, tl.kind)
		}
		if ok != (tl.kind != KindUnknown) {
			t.Errorf("tail %q: Classify(%q) subscribable = %v, want %v", tl.segs, uri, ok, tl.kind != KindUnknown)
		}
	}
}

// TestClassify_EveryBareNamespaceIsReachable verifies each root prefix
// resolves to its bare kind.
func TestClassify_EveryBareNamespaceIsReachable(t *testing.T) {
	for prefix, want := range bareKinds {
		uri := prefix + "42"
		got, ok := Classify(uri)
		if got != want || !ok {
			t.Errorf("Classify(%q) = %v, %v; want %v, true", uri, got, ok, want)
		}
	}
}

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
// it is subscribable and as which kind. KindUnknown means "registered,
// deliberately not subscribable" — see the tail table's own comments for
// why each exclusion is there.
var wantSubscribable = map[string]Kind{
	"gitlab://group/{group_id}":                                        KindGroup,
	"gitlab://group/{group_id}/label/{label_id}":                       KindGroupLabel,
	"gitlab://group/{group_id}/members":                                KindUnknown,
	"gitlab://group/{group_id}/milestone/{milestone_iid}":              KindGroupMilestone,
	"gitlab://group/{group_id}/projects":                               KindUnknown,
	"gitlab://project/{project_id}":                                    KindProject,
	"gitlab://project/{project_id}/board/{board_id}":                   KindBoard,
	"gitlab://project/{project_id}/branch/{branch}":                    KindBranch,
	"gitlab://project/{project_id}/branches":                           KindUnknown,
	"gitlab://project/{project_id}/commit/{sha}":                       KindUnknown,
	"gitlab://project/{project_id}/deploy_key/{deploy_key_id}":         KindDeployKey,
	"gitlab://project/{project_id}/deployment/{deployment_id}":         KindDeployment,
	"gitlab://project/{project_id}/environment/{environment_id}":       KindEnvironment,
	"gitlab://project/{project_id}/feature_flag/{name}":                KindFeatureFlag,
	"gitlab://project/{project_id}/file/{ref}/{+path}":                 KindFile,
	"gitlab://project/{project_id}/issue/{issue_iid}":                  KindIssue,
	"gitlab://project/{project_id}/issues":                             KindUnknown,
	"gitlab://project/{project_id}/job/{job_id}":                       KindJob,
	"gitlab://project/{project_id}/label/{label_id}":                   KindLabel,
	"gitlab://project/{project_id}/labels":                             KindUnknown,
	"gitlab://project/{project_id}/members":                            KindUnknown,
	"gitlab://project/{project_id}/milestone/{milestone_iid}":          KindMilestone,
	"gitlab://project/{project_id}/milestones":                         KindUnknown,
	"gitlab://project/{project_id}/mr/{merge_request_iid}":             KindMergeRequest,
	"gitlab://project/{project_id}/mr/{merge_request_iid}/discussions": KindMergeRequestDiscussions,
	"gitlab://project/{project_id}/mr/{merge_request_iid}/notes":       KindMergeRequestNotes,
	"gitlab://project/{project_id}/pipeline/{pipeline_id}":             KindPipeline,
	"gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs":        KindPipelineJobs,
	"gitlab://project/{project_id}/pipelines/latest":                   KindPipelineLatest,
	"gitlab://project/{project_id}/release/{tag_name}":                 KindRelease,
	"gitlab://project/{project_id}/releases":                           KindUnknown,
	"gitlab://project/{project_id}/snippet/{snippet_id}":               KindProjectSnippet,
	"gitlab://project/{project_id}/tag/{tag_name}":                     KindTag,
	"gitlab://project/{project_id}/tags":                               KindUnknown,
	"gitlab://project/{project_id}/wiki/{slug}":                        KindWiki,
	"gitlab://snippet/{snippet_id}":                                    KindSnippet,
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
		expected, decided := wantSubscribable[tmpl]
		if !decided {
			t.Errorf("resource template %q has no subscribability decision — add it to wantSubscribable "+
				"and, if subscribable, to the tail table in kind.go", tmpl)
			continue
		}
		uri := concreteURI(tmpl)
		got, ok := Classify(uri)
		if got != expected {
			t.Errorf("template %q -> Classify(%q) = %v, want %v", tmpl, uri, got, expected)
		}
		if ok != (expected != KindUnknown) {
			t.Errorf("template %q -> Classify(%q) subscribable = %v, want %v", tmpl, uri, ok, expected != KindUnknown)
		}
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
		if kind == KindUnknown {
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

// TestKindTemplate_CoversEverySubscribableKind verifies no subscribable
// kind is missing from the template map, which a watcher would only
// discover at poll time as an unreadable resource.
func TestKindTemplate_CoversEverySubscribableKind(t *testing.T) {
	for kind := range kindNames {
		if kind.Template() == "" {
			t.Errorf("%v is a named kind with no URI template", kind)
		}
	}
	if got := KindUnknown.Template(); got != "" {
		t.Errorf("KindUnknown.Template() = %q, want empty", got)
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
// with no "id" field (reflect.TypeOf(nil).Kind()), so feeding handlers
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
		kind, subscribable := Classify(uri)
		if !subscribable {
			continue
		}
		t.Run(kind.String(), func(t *testing.T) {
			reachedAPI.Store(false)
			_, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
			if !reachedAPI.Load() {
				t.Errorf("Classify(%q) = %v, subscribable — but reading it never reached GitLab, "+
					"so the router could not resolve the URI (read error: %v)", uri, kind, err)
			}
		})
	}
}
