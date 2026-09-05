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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
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
		{"project with trailing slash", "gitlab://project/42/"},
		{"group with trailing slash", "gitlab://group/9/"},
		{"snippet with trailing slash", "gitlab://snippet/5/"},
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
	seen := make(map[string]Kind, len(kindMeta))
	for k := range kindMeta {
		name := k.String()
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

// TestKindTemplate_CoversEverySubscribableKind verifies no subscribable
// kind is missing from the template map, which a watcher would only
// discover at poll time as an unreadable resource.
func TestKindTemplate_CoversEverySubscribableKind(t *testing.T) {
	for kind := range kindMeta {
		if kind.Template() == "" {
			t.Errorf("%v is a named kind with no URI template", kind)
		}
	}
	if got := KindUnknown.Template(); got != "" {
		t.Errorf("KindUnknown.Template() = %q, want empty", got)
	}
}

// TestKindCount_MatchesEveryDocumentThatCitesIt derives the subscribable
// kind count from the whitelist and asserts every document that states the
// number still states this one.
//
// The count appears in prose no generator owns — the capability reference,
// the front-door README, and the site page in both languages. When the
// whitelist grows, this is the test that turns a silently stale "26" into a
// failing build with the list of files to touch. ADRs are deliberately not
// checked: they record the number as it was when the decision was made.
func TestKindCount_MatchesEveryDocumentThatCitesIt(t *testing.T) {
	count := len(Templates())

	root := filepath.Join("..", "..")
	tests := []struct {
		file   string
		phrase string
	}{
		{"docs/reference/capabilities/subscriptions.md", fmt.Sprintf("%d kinds of resource", count)},
		{"README.md", fmt.Sprintf("%d resource kinds, single objects plus three single-parent lists", count)},
		{"site/src/content/docs/capabilities/subscriptions.mdx", fmt.Sprintf("%d resource kinds, single objects plus three single-parent lists", count)},
		{"site/src/content/docs/es/capabilities/subscriptions.mdx", fmt.Sprintf("%d tipos de recurso, objetos únicos más tres listas de un solo padre", count)},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tt.file))) //#nosec G304 -- fixed in-repo path
			if err != nil {
				t.Fatalf("read %s: %v", tt.file, err)
			}
			if !strings.Contains(string(data), tt.phrase) {
				t.Errorf("%s does not contain %q; the whitelist now has %d kinds — update the document",
					tt.file, tt.phrase, count)
			}
		})
	}
}

// TestTemplates_MatchesWhitelistExactly verifies the exported template list
// — the one gitlab://tools advertises — is the whitelist, whole and sorted,
// with nothing added and nothing missing.
func TestTemplates_MatchesWhitelistExactly(t *testing.T) {
	got := Templates()
	if len(got) != len(kindMeta) {
		t.Fatalf("Templates() returned %d entries, whitelist has %d", len(got), len(kindMeta))
	}
	if !slices.IsSorted(got) {
		t.Error("Templates() is not sorted; the advertisement should be stable across runs")
	}
	want := make(map[string]bool, len(kindMeta))
	for _, meta := range kindMeta {
		want[meta.template] = true
	}
	for _, template := range got {
		if !want[template] {
			t.Errorf("Templates() advertises %q, which the whitelist does not contain", template)
		}
	}
}
