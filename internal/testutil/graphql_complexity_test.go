package testutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/ast"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// measuredEpicNotesDocument is the epic notes list document as issue 968
// first widened it, which GitLab.com refused for costing 274 at first=100.
// It is a fixture because GitLab measured it, at several page sizes, and the
// figures pin the per-item multiplier, the Gitaly surcharge and the
// truncation together.
const measuredEpicNotesDocument = `
query($fullPath: ID!, $iid: String!, $first: Int, $after: String) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      widgets {
        ... on WorkItemWidgetNotes {
          discussions(first: $first, after: $after) {
            pageInfo { hasNextPage endCursor }
            nodes {
              notes {
                nodes {
                  id
                  body
                  author { id name username webUrl avatarUrl }
                  system
                  internal
                  imported
                  externalAuthor
                  authorIsContributor
                  maxAccessLevelOfAuthor
                  createdAt
                  updatedAt
                  lastEditedAt
                  lastEditedBy { id name username webUrl avatarUrl }
                  noteableId
                  noteableType
                  resolvable
                  resolved
                  resolvedAt
                  resolvedBy { id name username webUrl avatarUrl }
                  url
                }
              }
            }
          }
        }
      }
    }
  }
}`

// measuredEpicDiscussionsDocument is the epic discussions list document as
// issue 968 first widened it, which GitLab.com measured at exactly 250 at
// first=100: the most an authenticated caller may send, with nothing to spare.
const measuredEpicDiscussionsDocument = `
query($fullPath: ID!, $iid: String!, $first: Int, $after: String) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
      widgets {
        ... on WorkItemWidgetNotes {
          discussions(first: $first, after: $after) {
            pageInfo { hasNextPage endCursor }
            nodes {
              id
              replyId
              createdAt
              resolvable
              resolved
              resolvedAt
              resolvedBy { username }
              notes {
                nodes {
                  id
                  body
                  author { username }
                  system
                  internal
                  imported
                  externalAuthor
                  authorIsContributor
                  maxAccessLevelOfAuthor
                  createdAt
                  updatedAt
                  lastEditedAt
                  lastEditedBy { username }
                  noteableId
                  noteableType
                  resolvable
                  resolved
                  resolvedAt
                  resolvedBy { username }
                  url
                }
              }
            }
          }
        }
      }
    }
  }
}`

// epicVariables returns the variables every epic fixture binds, with first
// set to size when size is not nil.
func epicVariables(size any) map[string]any {
	variables := map[string]any{"fullPath": "gitlab-org", "iid": "23720"}
	if size != nil {
		variables["first"] = size
	}
	return variables
}

// TestGitLabQueryComplexity_MatchesWhatGitLabMeasured verifies the estimate
// against the figures GitLab.com reported for the same documents on
// 2026-09-26, each read either from the refusal ("Query has complexity of
// 274") or from a queryComplexity { score } selection, whose own cost of 2
// is taken off. The cases are chosen so each rule of the computation decides
// at least one of them: the per-item multiplier and its truncation (251 is
// 247.5 before it), the default page size when first is absent, a literal
// first, a fragment spread, a field declared free and inherited from its
// interface, the largest sum over the object types an interface's fragments
// reach, and a fragment on an interface whose possible types include another
// interface.
func TestGitLabQueryComplexity_MatchesWhatGitLabMeasured(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		document  string
		variables map[string]any
		want      int
	}{
		{name: "notes at first 100", document: measuredEpicNotesDocument, variables: epicVariables(100), want: 274},
		{name: "notes at first 90, truncated", document: measuredEpicNotesDocument, variables: epicVariables(int64(90)), want: 251},
		{name: "notes at first 70, decoded from JSON", document: measuredEpicNotesDocument, variables: epicVariables(float64(70)), want: 206},
		{name: "notes with no first, at the default page size", document: measuredEpicNotesDocument, variables: epicVariables(nil), want: 274},
		{name: "discussions at first 100", document: measuredEpicDiscussionsDocument, variables: epicVariables(100), want: 250},
		// The tier probe's membership document (issue 1224), measured on
		// 2026-10-08 with a queryComplexity selection whose own cost (3 for
		// score and limit, 2 for score alone) is taken off. Query.groups
		// costs two, its default sort counting one, and its resolver charges
		// a hundredth of its cost per item.
		{name: "top-level groups at first 20", document: gitlabclient.MembershipTierQuery, variables: map[string]any{"first": 20}, want: 13},
		{name: "top-level groups at first 50", document: gitlabclient.MembershipTierQuery, variables: map[string]any{"first": 50}, want: 16},
		{name: "top-level groups at first 100", document: gitlabclient.MembershipTierQuery, variables: map[string]any{"first": 100}, want: 22},
		{
			name: "a spread with a literal first",
			document: `query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) { workItem(iid: $iid) { widgets { ...Notes } } }
}
fragment Notes on WorkItemWidgetNotes { discussions(first: 10) { nodes { id } } }`,
			variables: epicVariables(nil),
			want:      10,
		},
		{
			name: "the largest sum over the widget types",
			document: `query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) { workItem(iid: $iid) { widgets {
    type
    ... on WorkItemWidgetNotes { discussionLocked }
    ... on WorkItemWidgetLabels { allowsScopedLabels }
  } } }
}`,
			variables: epicVariables(nil),
			want:      4,
		},
		{
			name: "a free field inherited from its interface",
			document: `query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) { workItem(iid: $iid) { widgets {
    ... on WorkItemWidgetNotes { type discussionLocked }
  } } }
}`,
			variables: epicVariables(nil),
			want:      4,
		},
		{
			name:     "a fragment on an interface that another interface implements",
			document: `query { currentUser { id ... on Todoable { webUrl } } }`,
			want:     3,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := GitLabQueryComplexity(testCase.document, testCase.variables)
			if err != nil {
				t.Fatalf("GitLabQueryComplexity() error = %v", err)
			}
			if got != testCase.want {
				t.Errorf("GitLabQueryComplexity() = %d, want the %d GitLab.com measured", got, testCase.want)
			}
		})
	}
}

// TestGitLabQueryComplexity_MembershipTierQueryAtTheRegistersPage_IsTheMeasuredFigure
// holds the tier probe's membership document (internal/gitlab, issue 1224) at the page
// size register row AUT-003 sets to the figure GitLab.com measured and under
// the limit GitLab refuses a query above, so that a change to either the
// selection or the page size sends whoever made it to measure again. It lives
// here because internal/gitlab cannot import this package, which imports it.
// When it fails because the selection changed, send the document to GitLab.com
// with that page size and a queryComplexity { score } selection, and record the
// figure less the selection's own 2.
func TestGitLabQueryComplexity_MembershipTierQueryAtTheRegistersPage_IsTheMeasuredFigure(t *testing.T) {
	const measured = 22 // 2026-10-08, at first=100
	got, err := GitLabQueryComplexity(gitlabclient.MembershipTierQuery, map[string]any{"first": tenancy.TierMembershipPageSize})
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got != measured {
		t.Errorf("the membership document costs %d at first=%d by the estimate, and GitLab measured %d at first=100: measure it again and record the figure",
			got, tenancy.TierMembershipPageSize, measured)
	}
	if got > GitLabAuthenticatedMaxComplexity {
		t.Errorf("the membership document costs %d, over the %d GitLab refuses a query above", got, GitLabAuthenticatedMaxComplexity)
	}
}

// TestGitLabQueryComplexity_UnrecordedFieldIgnoresItsPageSize verifies that a
// connection the cost table does not record is charged one plus its children
// and never reads its page size: GitLab multiplies only for a resolver that
// declares a multiplier, so a page size the estimate cannot read must not
// fail a field that would not have used it.
func TestGitLabQueryComplexity_UnrecordedFieldIgnoresItsPageSize(t *testing.T) {
	got, err := GitLabQueryComplexity(`query($first: Int) { currentUser { groups(first: $first) { nodes { id } } } }`,
		map[string]any{"first": "ten"})
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got != 4 {
		t.Errorf("GitLabQueryComplexity() = %d, want 4: currentUser, groups, nodes and id at one each", got)
	}
}

// TestGitLabQueryComplexity_AnAliasIsAFieldOfItsOwn verifies a field selected
// twice under two response keys is charged twice, as graphql-ruby keys a
// field by its alias when it has one, rather than refused as a repetition.
func TestGitLabQueryComplexity_AnAliasIsAFieldOfItsOwn(t *testing.T) {
	got, err := GitLabQueryComplexity(`query { currentUser { id again: id } }`, nil)
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got != 3 {
		t.Errorf("GitLabQueryComplexity() = %d, want 3: currentUser and the id under each key", got)
	}
}

// TestGitLabQueryComplexity_RefusesWhatItDoesNotModel verifies every refusal,
// including the ones raised below a fragment or a field, which only count if
// they travel back up to the caller.
func TestGitLabQueryComplexity_RefusesWhatItDoesNotModel(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		document  string
		variables map[string]any
		want      string
	}{
		{name: "a document GitLab refuses", document: `query { nope }`, want: `Cannot query field "nope"`},
		{
			name:     "two operations",
			document: `query A { currentUser { id } } query B { currentUser { id } }`,
			want:     "defines 2 operations",
		},
		{
			name:     "a directive on a field",
			document: `query { currentUser @include(if: true) { id } }`,
			want:     `field "currentUser" carries a directive`,
		},
		{
			name:     "a directive on an inline fragment",
			document: `query { currentUser { ... on CurrentUser @include(if: true) { id } } }`,
			want:     `the fragment on "CurrentUser" carries a directive`,
		},
		{
			name:     "a directive on a spread",
			document: `query { currentUser { ...Me @include(if: true) } } fragment Me on CurrentUser { id }`,
			want:     `the spread of "Me" carries a directive`,
		},
		{
			name:     "a directive inside an inline fragment",
			document: `query { currentUser { ... on CurrentUser { id @include(if: true) } } }`,
			want:     `field "id" carries a directive`,
		},
		{
			name:     "a directive inside a spread",
			document: `query { currentUser { ...Me } } fragment Me on CurrentUser { id @include(if: true) }`,
			want:     `field "id" carries a directive`,
		},
		{
			name:     "a key selected twice",
			document: `query { currentUser { id id } }`,
			want:     `CurrentUser receives "id" twice`,
		},
		{
			name:      "a page size that is not a number",
			document:  measuredEpicNotesDocument,
			variables: epicVariables("ten"),
			want:      "WorkItemWidgetNotes.discussions: first is string",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := GitLabQueryComplexity(testCase.document, testCase.variables)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("GitLabQueryComplexity() = %d, %v, want an error containing %q", got, err, testCase.want)
			}
			if got != 0 {
				t.Errorf("GitLabQueryComplexity() = %d alongside its error, want 0", got)
			}
		})
	}
}

// TestGitLabQueryComplexity_SchemaUnavailable_ReportsTheLoadFailure verifies
// the load failure is returned rather than read as a document costing
// nothing. The embedded schema cannot fail to load, so the loader is swapped.
func TestGitLabQueryComplexity_SchemaUnavailable_ReportsTheLoadFailure(t *testing.T) {
	original := complexitySchema
	loadErr := errors.New("the pin is corrupt")
	complexitySchema = func() (*ast.Schema, error) { return nil, loadErr }
	t.Cleanup(func() { complexitySchema = original })

	if got, err := GitLabQueryComplexity(`query { currentUser { id } }`, nil); !errors.Is(err, loadErr) || got != 0 {
		t.Errorf("GitLabQueryComplexity() = %d, %v, want 0 and %v", got, err, loadErr)
	}
}

// TestPageLimit_TakesTheSmallestOfFirstLastAndTheDefault verifies each page
// size GitLab charges for, one argument at a time, since a connection may be
// asked for its last items as readily as its first.
func TestPageLimit_TakesTheSmallestOfFirstLastAndTheDefault(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		arguments map[string]any
		want      int64
	}{
		{name: "neither", arguments: map[string]any{}, want: gitLabDefaultMaxPageSize},
		{name: "a first above the default", arguments: map[string]any{"first": 500}, want: gitLabDefaultMaxPageSize},
		{name: "a first below it", arguments: map[string]any{"first": 20}, want: 20},
		{name: "a last below it", arguments: map[string]any{"last": int64(30)}, want: 30},
		{name: "a last below the first", arguments: map[string]any{"first": 40, "last": 30}, want: 30},
		{name: "a first below the last", arguments: map[string]any{"first": float64(10), "last": 30}, want: 10},
		{name: "a null first", arguments: map[string]any{"first": nil, "last": 50}, want: 50},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := pageLimit(testCase.arguments)
			if err != nil || got != testCase.want {
				t.Errorf("pageLimit(%v) = %d, %v, want %d", testCase.arguments, got, err, testCase.want)
			}
		})
	}
}

// TestPageLimit_RefusesALastThatIsNotANumber verifies the refusal names the
// argument it read, since first and last are read by the same loop.
func TestPageLimit_RefusesALastThatIsNotANumber(t *testing.T) {
	got, err := pageLimit(map[string]any{"last": true})
	if !errors.Is(err, errPageSizeNotNumber) || !strings.Contains(err.Error(), "last is bool") || got != 0 {
		t.Errorf("pageLimit() = %d, %v, want 0 and an error naming last", got, err)
	}
}
