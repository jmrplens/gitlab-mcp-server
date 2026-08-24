// Package subscriptions implements MCP resource subscriptions
// (resources/subscribe) over GitLab resources.
//
// Delivery is polling-based, in both transports: a watcher re-reads a
// subscribed URI on an interval and emits notifications/resources/updated
// only when the content actually changed. There is no webhook path — this
// server does not run an inbound receiver of any kind, a decision recorded
// in ADR-0016 (docs/development/adr/adr-0016-no-webhook-ingestion.md), not
// a gap left for later.
package subscriptions

import (
	"slices"
	"strings"
)

// Kind classifies a subscribable resource. It determines how a watcher
// detects change, how fast it polls, and when it retires itself — a
// pipeline that reached "success" will never change again, whereas a wiki
// page can change at any time.
type Kind uint8

// The subscribable resource kinds, one per registered resource template
// whose content can change over time.
//
// Two categories are deliberately absent. Project- and group-wide
// collections (issues, branches, labels, members, milestones, releases,
// tags, projects) are excluded because any change anywhere in the namespace
// invalidates them, which turns one subscription into a notification
// firehose and burns the polling budget for no added signal. Commits are
// excluded because they are immutable: every field a commit resource
// returns is a property of the commit object itself, which git guarantees
// is fixed for a given SHA, so a watcher would poll forever and never
// notify.
const (
	// KindUnknown is the zero value, returned for any URI that is not
	// subscribable. Callers must treat it as "reject", never as a default.
	KindUnknown Kind = iota

	// Project-scoped.
	KindProject                 // gitlab://project/{ref}
	KindPipeline                // gitlab://project/{ref}/pipeline/{id}
	KindPipelineJobs            // gitlab://project/{ref}/pipeline/{id}/jobs
	KindPipelineLatest          // gitlab://project/{ref}/pipelines/latest
	KindJob                     // gitlab://project/{ref}/job/{id}
	KindMergeRequest            // gitlab://project/{ref}/mr/{iid}
	KindMergeRequestDiscussions // gitlab://project/{ref}/mr/{iid}/discussions
	KindMergeRequestNotes       // gitlab://project/{ref}/mr/{iid}/notes
	KindIssue                   // gitlab://project/{ref}/issue/{iid}
	KindDeployment              // gitlab://project/{ref}/deployment/{id}
	KindEnvironment             // gitlab://project/{ref}/environment/{id}
	KindFeatureFlag             // gitlab://project/{ref}/feature_flag/{name}
	KindRelease                 // gitlab://project/{ref}/release/{tag}
	KindTag                     // gitlab://project/{ref}/tag/{tag}
	KindBranch                  // gitlab://project/{ref}/branch/{branch}
	KindMilestone               // gitlab://project/{ref}/milestone/{iid}
	KindLabel                   // gitlab://project/{ref}/label/{id}
	KindBoard                   // gitlab://project/{ref}/board/{id}
	KindDeployKey               // gitlab://project/{ref}/deploy_key/{id}
	KindProjectSnippet          // gitlab://project/{ref}/snippet/{id}
	KindWiki                    // gitlab://project/{ref}/wiki/{slug}
	KindFile                    // gitlab://project/{ref}/file/{gitref}/{path}

	// Group-scoped.
	KindGroup          // gitlab://group/{ref}
	KindGroupLabel     // gitlab://group/{ref}/label/{id}
	KindGroupMilestone // gitlab://group/{ref}/milestone/{iid}

	// Instance-scoped.
	KindSnippet // gitlab://snippet/{id}
)

// kindNames maps each kind to the name used in logs and rejection messages.
var kindNames = map[Kind]string{
	KindProject:                 "project",
	KindPipeline:                "pipeline",
	KindPipelineJobs:            "pipeline_jobs",
	KindPipelineLatest:          "pipeline_latest",
	KindJob:                     "job",
	KindMergeRequest:            "merge_request",
	KindMergeRequestDiscussions: "merge_request_discussions",
	KindMergeRequestNotes:       "merge_request_notes",
	KindIssue:                   "issue",
	KindDeployment:              "deployment",
	KindEnvironment:             "environment",
	KindFeatureFlag:             "feature_flag",
	KindRelease:                 "release",
	KindTag:                     "tag",
	KindBranch:                  "branch",
	KindMilestone:               "milestone",
	KindLabel:                   "label",
	KindBoard:                   "board",
	KindDeployKey:               "deploy_key",
	KindProjectSnippet:          "project_snippet",
	KindWiki:                    "wiki",
	KindFile:                    "file",
	KindGroup:                   "group",
	KindGroupLabel:              "group_label",
	KindGroupMilestone:          "group_milestone",
	KindSnippet:                 "snippet",
}

// String returns the kind's name, for logs and error messages. Unmapped
// values render "unknown" rather than an empty string so a log line never
// silently loses the field.
func (k Kind) String() string {
	if name, ok := kindNames[k]; ok {
		return name
	}
	return "unknown"
}

// kindTemplates maps each subscribable kind to the URI template
// internal/resources registered it under.
//
// This is the bridge a watcher needs: the SDK keeps its URI-to-handler
// lookup unexported, so re-reading a subscribed URI means resolving it to
// the template whose handler the router would have dispatched to.
// TestClassify_MatchesRegisteredResourceTemplates checks these strings
// against the live registry, so a renamed template cannot leave a stale
// entry here.
var kindTemplates = map[Kind]string{
	KindProject:                 "gitlab://project/{project_id}",
	KindPipeline:                "gitlab://project/{project_id}/pipeline/{pipeline_id}",
	KindPipelineJobs:            "gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs",
	KindPipelineLatest:          "gitlab://project/{project_id}/pipelines/latest",
	KindJob:                     "gitlab://project/{project_id}/job/{job_id}",
	KindMergeRequest:            "gitlab://project/{project_id}/mr/{merge_request_iid}",
	KindMergeRequestDiscussions: "gitlab://project/{project_id}/mr/{merge_request_iid}/discussions",
	KindMergeRequestNotes:       "gitlab://project/{project_id}/mr/{merge_request_iid}/notes",
	KindIssue:                   "gitlab://project/{project_id}/issue/{issue_iid}",
	KindDeployment:              "gitlab://project/{project_id}/deployment/{deployment_id}",
	KindEnvironment:             "gitlab://project/{project_id}/environment/{environment_id}",
	KindFeatureFlag:             "gitlab://project/{project_id}/feature_flag/{name}",
	KindRelease:                 "gitlab://project/{project_id}/release/{tag_name}",
	KindTag:                     "gitlab://project/{project_id}/tag/{tag_name}",
	KindBranch:                  "gitlab://project/{project_id}/branch/{branch}",
	KindMilestone:               "gitlab://project/{project_id}/milestone/{milestone_iid}",
	KindLabel:                   "gitlab://project/{project_id}/label/{label_id}",
	KindBoard:                   "gitlab://project/{project_id}/board/{board_id}",
	KindDeployKey:               "gitlab://project/{project_id}/deploy_key/{deploy_key_id}",
	KindProjectSnippet:          "gitlab://project/{project_id}/snippet/{snippet_id}",
	KindWiki:                    "gitlab://project/{project_id}/wiki/{slug}",
	KindFile:                    "gitlab://project/{project_id}/file/{ref}/{+path}",
	KindGroup:                   "gitlab://group/{group_id}",
	KindGroupLabel:              "gitlab://group/{group_id}/label/{label_id}",
	KindGroupMilestone:          "gitlab://group/{group_id}/milestone/{milestone_iid}",
	KindSnippet:                 "gitlab://snippet/{snippet_id}",
}

// Template returns the URI template this kind's resource is registered
// under, or "" for [KindUnknown].
func (k Kind) Template() string {
	return kindTemplates[k]
}

// The root prefixes every resource URI starts with. These mirror
// internal/resources' own constants.
const (
	projectPrefix = "gitlab://project/"
	groupPrefix   = "gitlab://group/"
	snippetPrefix = "gitlab://snippet/"
)

// Pattern segments used by the tail table.
//
// Every registered URI template expands its variables as RFC 6570 simple
// strings, which do not match "/" — verified by round-tripping the real
// resource router, where "gitlab://project/group/proj/pipeline/99" and
// "gitlab://project/42/wiki/parent/child" both fail to resolve while their
// percent-encoded forms resolve fine. So a namespace reference or
// identifier containing a slash must arrive encoded ("group%2Fproj"), and
// every segment below is exactly one path segment. The single exception is
// the file resource's "{+path}", a reserved expansion that does match
// slashes.
const (
	patNumeric = "#"  // one segment, a positive base-10 integer
	patName    = "*"  // one segment, any non-empty value
	patPath    = "**" // one or more segments, greedy to the end of the URI
)

// tail describes one URI layout after the root prefix and namespace
// reference have been consumed, as a sequence of pattern segments.
//
// A tail whose kind is KindUnknown is registered on purpose: it names a
// real resource that is deliberately not subscribable, and having it in the
// table is what stops that URI being mistaken for something else.
type tail struct {
	prefix string
	segs   []string
	kind   Kind
}

// tails lists every registered URI layout. Patterns are mutually exclusive
// by segment count and literal segments, so order carries no meaning —
// TestClassify_TailPatternsAreMutuallyExclusive enforces that.
var tails = []tail{
	// Project, single objects addressed by a numeric identifier.
	{projectPrefix, []string{"pipeline", patNumeric}, KindPipeline},
	{projectPrefix, []string{"pipeline", patNumeric, "jobs"}, KindPipelineJobs},
	{projectPrefix, []string{"pipelines", "latest"}, KindPipelineLatest},
	{projectPrefix, []string{"job", patNumeric}, KindJob},
	{projectPrefix, []string{"mr", patNumeric}, KindMergeRequest},
	{projectPrefix, []string{"mr", patNumeric, "discussions"}, KindMergeRequestDiscussions},
	{projectPrefix, []string{"mr", patNumeric, "notes"}, KindMergeRequestNotes},
	{projectPrefix, []string{"issue", patNumeric}, KindIssue},
	{projectPrefix, []string{"deployment", patNumeric}, KindDeployment},
	{projectPrefix, []string{"environment", patNumeric}, KindEnvironment},
	{projectPrefix, []string{"milestone", patNumeric}, KindMilestone},
	{projectPrefix, []string{"label", patName}, KindLabel},
	{projectPrefix, []string{"board", patNumeric}, KindBoard},
	{projectPrefix, []string{"deploy_key", patNumeric}, KindDeployKey},
	{projectPrefix, []string{"snippet", patNumeric}, KindProjectSnippet},

	// Project, single objects addressed by a name.
	{projectPrefix, []string{"feature_flag", patName}, KindFeatureFlag},
	{projectPrefix, []string{"release", patName}, KindRelease},
	{projectPrefix, []string{"tag", patName}, KindTag},
	{projectPrefix, []string{"branch", patName}, KindBranch},
	{projectPrefix, []string{"wiki", patName}, KindWiki},

	// Project, the one resource with a multi-segment tail.
	{projectPrefix, []string{"file", patName, patPath}, KindFile},

	// Project, immutable: a commit's fields are properties of the commit
	// object, fixed for a given SHA, so a watcher could never notify.
	{projectPrefix, []string{"commit", patName}, KindUnknown},

	// Project collections.
	{projectPrefix, []string{"branches"}, KindUnknown},
	{projectPrefix, []string{"issues"}, KindUnknown},
	{projectPrefix, []string{"labels"}, KindUnknown},
	{projectPrefix, []string{"members"}, KindUnknown},
	{projectPrefix, []string{"milestones"}, KindUnknown},
	{projectPrefix, []string{"releases"}, KindUnknown},
	{projectPrefix, []string{"tags"}, KindUnknown},

	// Group.
	{groupPrefix, []string{"label", patName}, KindGroupLabel},
	{groupPrefix, []string{"milestone", patNumeric}, KindGroupMilestone},
	{groupPrefix, []string{"members"}, KindUnknown},
	{groupPrefix, []string{"projects"}, KindUnknown},
}

// bareKinds maps each root prefix to the kind of a URI carrying only a
// namespace reference and no tail.
var bareKinds = map[string]Kind{
	projectPrefix: KindProject,
	groupPrefix:   KindGroup,
	snippetPrefix: KindSnippet,
}

// Classify reports which subscribable resource a concrete URI names, and
// whether it is subscribable at all.
//
// This is the whitelist the MCP SubscribeHandler enforces. The SDK does not
// check that a subscribed URI corresponds to any registered resource, and at
// least one shipping client (Cursor) sends resources/subscribe even against
// a server that advertises subscribe: false — so rejecting here is what
// stops clients from holding silent subscriptions to URIs that will never
// produce a notification.
//
// The whitelist deliberately mirrors what the resource router can actually
// resolve, rather than being merely a superset of it. Accepting a URI the
// router would answer with "resource not found" is the worst outcome
// available: the subscription is acknowledged, then every poll fails, so
// the client waits for a notification that can never arrive.
func Classify(uri string) (Kind, bool) {
	prefix, rest, ok := splitRoot(uri)
	if !ok {
		return KindUnknown, false
	}

	// The namespace reference is one segment. A path-style reference
	// reaches us percent-encoded, so it has no slash to split on here.
	ref, tailPath, _ := strings.Cut(rest, "/")
	if ref == "" {
		return KindUnknown, false
	}
	// Snippets are addressed by a numeric ID; projects and groups accept
	// either a numeric ID or an encoded path.
	if prefix == snippetPrefix && !isPositiveInt(ref) {
		return KindUnknown, false
	}

	if tailPath == "" {
		kind := bareKinds[prefix]
		return kind, kind != KindUnknown
	}

	segs := strings.Split(tailPath, "/")
	for _, t := range tails {
		if t.prefix != prefix {
			continue
		}
		if matchSegments(segs, t.segs) {
			return t.kind, t.kind != KindUnknown
		}
	}
	return KindUnknown, false
}

// splitRoot identifies which root prefix a URI carries and returns the
// remainder after it.
func splitRoot(uri string) (prefix, rest string, ok bool) {
	for _, p := range []string{projectPrefix, groupPrefix, snippetPrefix} {
		if r, cut := strings.CutPrefix(uri, p); cut && r != "" {
			return p, r, true
		}
	}
	return "", "", false
}

// matchSegments reports whether the concrete segments satisfy the pattern.
//
// patPath is greedy and may only appear last, consuming every remaining
// segment; every other pattern element matches exactly one segment.
func matchSegments(segs, pattern []string) bool {
	for i, p := range pattern {
		if p == patPath {
			// Must be the final pattern element, and needs at least one
			// segment to consume.
			if i != len(pattern)-1 || i >= len(segs) {
				return false
			}
			// An empty segment means a doubled or trailing slash, which
			// names nothing.
			return !slices.Contains(segs[i:], "")
		}
		if i >= len(segs) {
			return false
		}
		if !matchSegment(segs[i], p) {
			return false
		}
	}
	return len(segs) == len(pattern)
}

// matchSegment reports whether one concrete segment satisfies one pattern
// element, which is either a wildcard or a literal.
func matchSegment(seg, p string) bool {
	switch p {
	case patNumeric:
		return isPositiveInt(seg)
	case patName:
		return seg != ""
	default:
		return seg == p
	}
}

// isPositiveInt reports whether s is a base-10 integer greater than zero.
//
// Leading zeros are accepted deliberately: GitLab resolves a zero-padded
// identifier to the same object as its bare form (verified against a live
// instance — GET /projects/02317 and GET /projects/2317 both return the same
// project), and this server's own resource read path parses identifiers with
// strconv.ParseInt, which accepts them too. Rejecting them here would create
// a URI a client can read but not subscribe to.
//
// Zero and negatives are rejected: GitLab identifiers and IIDs are
// auto-increment integers starting at 1, so neither can ever name a real
// object. A sign character is rejected for the same reason — GitLab returns
// 404 for "+2317", so accepting it would only create a subscription
// guaranteed to be dropped on its first read.
func isPositiveInt(s string) bool {
	nonZero := false
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		if s[i] != '0' {
			nonZero = true
		}
	}
	return nonZero
}
