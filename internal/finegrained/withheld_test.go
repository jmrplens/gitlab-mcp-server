package finegrained

import (
	"strings"
	"testing"
)

// deniedTable is a table holding one denied row whose denial the case sets.
func deniedTable(denial Denial) *Table {
	table := testTable()
	table.Actions = []Requirement{{ID: "x.y", Denied: &denial}}
	return table
}

// TestAuthority_WithheldText_PhaseANamesTheCauseAndWhatGitLabDoes verifies the
// words a model is told for an action no fine-grained token can run name the
// cause, the element that decides it and what GitLab does to the request,
// cause by cause.
func TestAuthority_WithheldText_PhaseANamesTheCauseAndWhatGitLabDoes(t *testing.T) {
	cases := []struct {
		name   string
		denial Denial
		want   string
	}{
		{
			name:   "an undeclared type read with null",
			denial: Denial{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull},
			want: `action "x.y" exists but is not available to a fine-grained personal access token: GitLab 19.4.1 declares ` +
				`no fine-grained permission on the GraphQL type Namespace this action reads, and answers such a read with null. ` +
				`Use a classic personal access token with read_api for reads or api for writes (an existing one, on an instance ` +
				`that no longer lets you create them), where the group does not refuse classic tokens. Do not report the ` +
				`capability as missing.`,
		},
		{
			name:   "an undeclared type whose items are removed",
			denial: Denial{Cause: CauseTypeUndeclared, Element: "BranchRule", Effect: EffectRemoved},
			want:   "on the GraphQL type BranchRule this action reads, and removes the items from such a list.",
		},
		{
			name:   "an undeclared type that nulls its list",
			denial: Denial{Cause: CauseTypeUndeclared, Element: "SecurityAttribute", Effect: EffectListNull},
			want:   "answers such a list with null.",
		},
		{
			name:   "an undeclared mutation",
			denial: Denial{Cause: CauseMutationUndeclared, Element: "securityScanProfileAttach", Effect: EffectRefused},
			want:   "on the GraphQL mutation securityScanProfileAttach this action sends, and refuses such a write.",
		},
		{
			name:   "an undeclared payload",
			denial: Denial{Cause: CausePayloadUndeclared, Element: "CustomEmoji", Effect: EffectCommittedThenNull},
			want:   "on the GraphQL type CustomEmoji this action's write answers with, and commits such a write and answers null.",
		},
		{
			name:   "a boundary that does not resolve, read",
			denial: Denial{Cause: CauseBoundaryUnresolvable, Element: "WorkItem", Effect: EffectNullOrEmpty},
			want: "declares the GraphQL type WorkItem at a boundary the object this action reaches never resolves to, " +
				"and answers such a read with null or an empty list.",
		},
		{
			name:   "a boundary that does not resolve, in a write's answer",
			denial: Denial{Cause: CauseBoundaryUnresolvable, Element: "WorkItem", Effect: EffectCommittedThenNull},
			want: "declares the GraphQL type WorkItem at a boundary the object this action reaches never resolves to, " +
				"and commits such a write and answers null.",
		},
		{
			name:   "a mutation whose own boundary does not resolve",
			denial: Denial{Cause: CauseBoundaryUnresolvable, Element: "workItemUpdate", Effect: EffectRefused},
			want: "declares the GraphQL mutation workItemUpdate at a boundary the object this action reaches never resolves to, " +
				"and refuses such a write.",
		},
		{
			name:   "a deferred route",
			denial: Denial{Cause: CauseRESTTodo, Element: "POST /policies", Effect: EffectRefused},
			want:   "has deferred the fine-grained permission of the route POST /policies this action calls, and refuses such a request.",
		},
		{
			name:   "an undeclared route",
			denial: Denial{Cause: CauseRESTUndeclared, Element: "GET /x", Effect: EffectRefused},
			want:   "declares no fine-grained permission on the route GET /x this action calls, and refuses such a request.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			authority := Unevaluated(deniedTable(tc.denial), FallbackNone, "")
			got := authority.WithheldText("x.y", authority.Decide("x.y"))
			if !strings.Contains(got, tc.want) {
				t.Errorf("WithheldText =\n%s\nwant it to contain\n%s", got, tc.want)
			}
			if strings.Contains(got, "gitlab_") {
				t.Errorf("WithheldText names a tool: %s", got)
			}
		})
	}
}

// TestAuthority_WithheldText_CarriesTheRegisterPrefixRightAfterTheAction
// verifies each phase's words carry their stable text right after the action
// they name, which is what a client matches the refusal on and what the tenant
// register declares as its prefix.
func TestAuthority_WithheldText_CarriesTheRegisterPrefixRightAfterTheAction(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	cases := []struct {
		name     string
		decision Decision
		prefix   string
	}{
		{name: "phase A", decision: authority.Decide("epic.list"), prefix: "exists but is not available to a fine-grained personal access token"},
		{
			name:     "phase B",
			decision: Decision{Cause: CauseNotGranted, Missing: []uint32{0}, Known: true},
			prefix:   "exists but this fine-grained personal access token was not granted what it needs",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := authority.WithheldText("epic.list", tc.decision)
			if want := `action "epic.list" ` + tc.prefix + ": "; !strings.HasPrefix(got, want) {
				t.Errorf("WithheldText = %q, want it to begin %q", got, want)
			}
		})
	}
}

// TestAuthority_WithheldText_ACallThatPassesHasNoWords verifies a call the
// decision lets through, with a row or without one, is told nothing.
func TestAuthority_WithheldText_ACallThatPassesHasNoWords(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	for _, id := range []string{"merge_request.approve", "issue.list"} {
		t.Run(id, func(t *testing.T) {
			if got := authority.WithheldText(id, authority.Decide(id)); got != "" {
				t.Errorf("WithheldText(%q) = %q, want none", id, got)
			}
		})
	}
}

// TestAuthority_WithheldText_ARowWithNoDenialStillSaysWhy verifies a withheld
// decision for an action the table holds no row for, or whose row carries no
// denial, is still told why, in the general words, rather than in none.
func TestAuthority_WithheldText_ARowWithNoDenialStillSaysWhy(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	for _, id := range []string{"issue.list", "merge_request.approve"} {
		t.Run(id, func(t *testing.T) {
			got := authority.WithheldText(id, Decision{Cause: CauseTypeUndeclared})
			if !strings.Contains(got, "GitLab 19.4.1 declares no fine-grained permission this action can be granted.") {
				t.Errorf("WithheldText = %s", got)
			}
		})
	}
}

// TestAuthority_WithheldText_AppendsWhyTheGrantWasNotEvaluated verifies each
// reason a grant was not evaluated is told after the refusal, in words that
// say what the token or the instance would need for it to be, and that with
// no such reason nothing is appended.
func TestAuthority_WithheldText_AppendsWhyTheGrantWasNotEvaluated(t *testing.T) {
	denial := Denial{Cause: CauseTypeUndeclared, Element: "Namespace", Effect: EffectNull}
	cases := []struct {
		reason   FallbackReason
		reported string
		want     string
	}{
		{FallbackNone, "", "classic tokens. Do not report"},
		{FallbackGrantUnreadable, "", " The token cannot read its own grant (a token created with Personal Access Token: Read can), so the grant was not evaluated. Do not"},
		{FallbackGrantTooLarge, "", " The token's grant is larger than this server reads, so the grant was not evaluated."},
		{FallbackGrantShape, "", " The token's grant holds a scope this server cannot read without guessing, so the grant was not evaluated."},
		{FallbackVersionUnreadable, "", " The instance did not report a version this server can read (a token granted Metadata: Read lets it), so the grant was not evaluated."},
		{FallbackVersionOutside, "19.5.0-pre", " The instance reports GitLab 19.5.0-pre and the permissions are recorded for 19.4 only, so the grant was not evaluated."},
		{FallbackUnknownPermission, "", " The token's grant names a permission GitLab 19.4.1 does not define, so the grant was not evaluated."},
	}
	for _, tc := range cases {
		t.Run("reason "+string(tc.reason), func(t *testing.T) {
			authority := Unevaluated(deniedTable(denial), tc.reason, tc.reported)
			got := authority.WithheldText("x.y", authority.Decide("x.y"))
			if !strings.Contains(got, tc.want) {
				t.Errorf("WithheldText =\n%s\nwant it to contain\n%s", got, tc.want)
			}
		})
	}
}

// TestAuthority_WithheldText_PhaseBNamesTheMissingPermissionsByTheirGrantableWords
// verifies a call refused for what the grant lacks names each missing
// permission by the words GitLab's token creation page grants it by, with the
// boundary it is held at.
func TestAuthority_WithheldText_PhaseBNamesTheMissingPermissionsByTheirGrantableWords(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	cases := []struct {
		name    string
		missing []uint32
		want    string
	}{
		{
			name:    "one permission",
			missing: []uint32{0},
			want: `action "merge_request.approve" exists but this fine-grained personal access token was not granted what it ` +
				`needs: the project permission [Merge Request: Approve], as GitLab 19.4.1 declares it. Create a fine-grained ` +
				`token that grants it, or use a classic token with the api scope (an existing one, on an instance that no ` +
				`longer lets you create them), where the group does not refuse classic tokens. Do not report the capability ` +
				`as missing.`,
		},
		{
			name:    "two groups, one without a grantable name",
			missing: []uint32{0, 1},
			want: "the project permission [Merge Request: Approve], and the project or group permissions [Issue: Read, " +
				"read_role_only], as GitLab 19.4.1 declares it. Create a fine-grained token that grants them,",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := Decision{Callable: false, Cause: CauseNotGranted, Missing: tc.missing, Known: true}
			got := authority.WithheldText("merge_request.approve", decision)
			if !strings.Contains(got, tc.want) {
				t.Errorf("WithheldText =\n%s\nwant it to contain\n%s", got, tc.want)
			}
		})
	}
}
