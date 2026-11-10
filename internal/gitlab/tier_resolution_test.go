// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// statusError is the error client-go returns for an answer with status code.
func statusError(code int) error {
	return &gl.ErrorResponse{StatusCode: code}
}

// TestTierReadUnresolved_TellsAnUnansweredReadFromAnAnswer holds the rule
// every step of the tier probe reads a failure by (issue 1782): GitLab
// answered when it sent a status about the question, and did not when nothing
// came back or the status is its own moment. The status boundaries are each
// side of every range the rule names, so a range that moved by one is caught.
func TestTierReadUnresolved_TellsAnUnansweredReadFromAnAnswer(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		unresolved bool
	}{
		{name: "299", err: statusError(299), unresolved: false},
		{name: "300", err: statusError(300), unresolved: true},
		{name: "399", err: statusError(399), unresolved: true},
		{name: "400", err: statusError(400), unresolved: false},
		{name: "401", err: statusError(http.StatusUnauthorized), unresolved: false},
		{name: "403", err: statusError(http.StatusForbidden), unresolved: false},
		{name: "407", err: statusError(407), unresolved: false},
		{name: "408", err: statusError(http.StatusRequestTimeout), unresolved: true},
		{name: "409", err: statusError(409), unresolved: false},
		{name: "428", err: statusError(428), unresolved: false},
		{name: "429", err: statusError(http.StatusTooManyRequests), unresolved: true},
		{name: "430", err: statusError(430), unresolved: false},
		{name: "499", err: statusError(499), unresolved: false},
		{name: "500", err: statusError(500), unresolved: true},
		{name: "501", err: statusError(501), unresolved: true},
		{name: "502", err: statusError(http.StatusBadGateway), unresolved: true},
		{name: "504", err: statusError(http.StatusGatewayTimeout), unresolved: true},
		{name: "a 404, which client-go answers with its sentinel", err: gl.ErrNotFound, unresolved: false},
		{name: "a GraphQL 404, wrapped", err: fmt.Errorf("failed to execute GraphQL query: %w", gl.ErrNotFound), unresolved: false},
		{name: "a GraphQL 502 with a JSON body", err: &gl.GraphQLResponseError{Err: statusError(http.StatusBadGateway)}, unresolved: true},
		{name: "a GraphQL 403 with a JSON body", err: &gl.GraphQLResponseError{Err: statusError(http.StatusForbidden)}, unresolved: false},
		{name: "a REST 403, wrapped", err: fmt.Errorf("listing: %w", statusError(http.StatusForbidden)), unresolved: false},
		{name: "a deadline", err: fmt.Errorf("failed to execute GraphQL query: %w", context.DeadlineExceeded), unresolved: true},
		{name: "a cancellation", err: context.Canceled, unresolved: true},
		{name: "a network failure", err: &url.Error{Op: "Post", URL: "https://gitlab.example", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}, unresolved: true},
		{name: "a body that does not decode", err: errors.New("invalid character '<' looking for beginning of value"), unresolved: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tierReadUnresolved(tc.err); got != tc.unresolved {
				t.Errorf("tierReadUnresolved(%v) = %t, want %t", tc.err, got, tc.unresolved)
			}
		})
	}
}

// TestGitLabStatus_OpensWhatClientGoWraps checks that the status is read from
// a GraphQL error client-go wraps without an Unwrap, and that no status is
// made up for an error that carries none.
func TestGitLabStatus_OpensWhatClientGoWraps(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		status   int
		answered bool
	}{
		{name: "REST", err: statusError(http.StatusBadGateway), status: http.StatusBadGateway, answered: true},
		{name: "GraphQL", err: &gl.GraphQLResponseError{Err: statusError(http.StatusServiceUnavailable)}, status: http.StatusServiceUnavailable, answered: true},
		{name: "GraphQL without a status", err: &gl.GraphQLResponseError{Err: context.DeadlineExceeded}},
		{name: "no status at all", err: context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, answered := gitLabStatus(tc.err)
			if status != tc.status || answered != tc.answered {
				t.Errorf("gitLabStatus(%v) = (%d, %t), want (%d, %t)", tc.err, status, answered, tc.status, tc.answered)
			}
		})
	}
}

// TestMembershipReadUnresolved_ReadsADocumentRefusalByItsMessages checks the
// rule for a membership page that came back with no groups: GitLab's refusal
// of the caller is its answer, and any other message beside it may be its own
// moment (a field that ran out of time, an internal error), which is asked
// again.
func TestMembershipReadUnresolved_ReadsADocumentRefusalByItsMessages(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		unresolved bool
	}{
		{name: "the permission refusal", err: &documentRefusalError{messages: []string{refusedMessage}}, unresolved: false},
		{name: "the permission refusal, twice", err: &documentRefusalError{messages: []string{refusedMessage, refusedMessage}}, unresolved: false},
		{name: "a timeout beside the permission refusal", err: &documentRefusalError{messages: []string{refusedMessage, "Timeout on Group.licensedFeatureAvailability"}}, unresolved: true},
		{name: "an internal error", err: &documentRefusalError{messages: []string{"Internal server error"}}, unresolved: true},
		{name: "no groups connection", err: errNoGroupsConnection, unresolved: true},
		{name: "a 403", err: &gl.GraphQLResponseError{Err: statusError(http.StatusForbidden)}, unresolved: false},
		{name: "a 502", err: &gl.GraphQLResponseError{Err: statusError(http.StatusBadGateway)}, unresolved: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := membershipReadUnresolved(tc.err); got != tc.unresolved {
				t.Errorf("membershipReadUnresolved(%v) = %t, want %t", tc.err, got, tc.unresolved)
			}
		})
	}
}

// TestDocumentRefusalError_Error_QuotesGitLabsMessages pins the text the INFO
// line of a membership read quotes, which is the one a production log carries.
func TestDocumentRefusalError_Error_QuotesGitLabsMessages(t *testing.T) {
	refusal := &documentRefusalError{messages: []string{"one", "two"}}
	if got, want := refusal.Error(), "GitLab answered the document with errors: one; two"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if permissionRefusalMessage != refusedMessage {
		t.Errorf("permissionRefusalMessage = %q, want GitLab's sentence %q", permissionRefusalMessage, refusedMessage)
	}
}

// ultimateGroup is a page whose one group carries the Ultimate feature.
var ultimateGroup = []fakeMembershipPage{{groups: []*fakeGroup{{path: "acme", premium: true, ultimate: true}}}}

// TestDetectTier_GitLabDotCom_UnansweredMembership_IsProvisionalUntilAskedAgain
// is the production case issue 1782 records: a credential whose top-level
// group is on an Ultimate plan, administering free namespaces, whose
// membership page GitLab.com did not answer in time. Before, the build read
// that as Free and nothing ever asked again. Now the first detection is Free
// and provisional, naming the membership step, and the next detection on the
// same client, with GitLab answering, is Ultimate and definitive, with no
// restart and no pinned tier.
func TestDetectTier_GitLabDotCom_UnansweredMembership_IsProvisionalUntilAskedAgain(t *testing.T) {
	cases := []struct {
		name string
		page fakeMembershipPage
	}{
		{name: "the page hangs past the deadline", page: fakeMembershipPage{hang: true}},
		{name: "a 502", page: fakeMembershipPage{status: http.StatusBadGateway}},
		{name: "a 504", page: fakeMembershipPage{status: http.StatusGatewayTimeout}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{tc.page}}
			client := server.client(t, false)

			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			first := client.DetectTier(ctx)
			if first.Tier != edition.Free || !first.Provisional || !first.Answered || !slices.Equal(first.Unresolved, []string{tierStepMembership}) {
				t.Errorf("first DetectTier() = %+v, want Free answered by the namespaces, provisional on the membership", first)
			}
			if client.Tier() != edition.Free {
				t.Errorf("client tier after the first detection = %v, want Free, the best known so far", client.Tier())
			}

			server.set(func(g *fakeGitLabDotCom) { g.pages = ultimateGroup })
			second := client.DetectTier(t.Context())
			if second.Tier != edition.Ultimate || second.Provisional || len(second.Unresolved) != 0 {
				t.Errorf("second DetectTier() = %+v, want Ultimate, definitive", second)
			}
			if client.Tier() != edition.Ultimate {
				t.Errorf("client tier after the second detection = %v, want Ultimate", client.Tier())
			}
		})
	}
}

// TestDetectTier_GitLabDotCom_AnswersThatSettleTheTier_AreNotProvisional is the
// contrast to the production case: answers GitLab gives about the caller, which
// asking again does not change, resolve exactly as before issue 1782 and are
// definitive, so no pool entry asks them again. Each is detected twice on one
// client, and both detections agree.
func TestDetectTier_GitLabDotCom_AnswersThatSettleTheTier_AreNotProvisional(t *testing.T) {
	premiumNamespace := []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}}
	fineGrained := fakeMembershipPage{groups: []*fakeGroup{nil, nil}, errors: []string{refusedMessage}}
	cases := []struct {
		name       string
		namespaces []map[string]any
		page       fakeMembershipPage
		want       edition.Tier
	}{
		{name: "a fine-grained token's null nodes after free namespaces", namespaces: freeNamespaces, page: fineGrained, want: edition.Free},
		{name: "a fine-grained token's null nodes after a premium namespace", namespaces: premiumNamespace, page: fineGrained, want: edition.Premium},
		{name: "a fine-grained token's null nodes with the namespaces refused", page: fineGrained, want: edition.Free},
		{name: "a user in no group", namespaces: freeNamespaces, page: fakeMembershipPage{}, want: edition.Free},
		{name: "a document refused for the caller's permission", namespaces: freeNamespaces, page: fakeMembershipPage{refused: true}, want: edition.Free},
		{name: "a 403", namespaces: freeNamespaces, page: fakeMembershipPage{status: http.StatusForbidden}, want: edition.Free},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: tc.namespaces, pages: []fakeMembershipPage{tc.page}}
			client := server.client(t, false)
			for attempt := range 2 {
				got := client.DetectTier(t.Context())
				if got.Tier != tc.want || got.Provisional || len(got.Unresolved) != 0 {
					t.Errorf("DetectTier() #%d = %+v, want %v, definitive", attempt+1, got, tc.want)
				}
			}
		})
	}
}

// TestDetectTier_GitLabDotCom_DocumentRefusedForAnotherReason_IsProvisional
// checks the other half of the rule for a document GitLab answered with data
// null: a message other than its permission refusal may be its own moment,
// which it answers with the same shape, so the step is asked again.
func TestDetectTier_GitLabDotCom_DocumentRefusedForAnotherReason_IsProvisional(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{{refused: true, message: "Internal server error"}}}
	if got, _ := server.detect(t, false); got.Tier != edition.Free || !got.Provisional {
		t.Errorf("DetectTier() = %+v, want Free, provisional", got)
	}
}

// TestDetectTier_SelfManagedLicense_Unanswered_IsProvisionalUntilAskedAgain is
// the self-managed half of issue 1782: an administrator's token whose license
// read GitLab did not answer resolved Free with the warning to pin the tier,
// and kept it. The license is the only step that answers there, so the result
// is provisional, naming it, and the next detection reads the plan.
func TestDetectTier_SelfManagedLicense_Unanswered_IsProvisionalUntilAskedAgain(t *testing.T) {
	selfManagedNamespaces := []map[string]any{{"full_path": "root", "kind": "user", "plan": "default"}}
	cases := []struct {
		name       string
		server     *fakeGitLabDotCom
		unresolved []string
	}{
		{
			name:       "a 502",
			server:     &fakeGitLabDotCom{namespaces: selfManagedNamespaces, license: http.StatusBadGateway},
			unresolved: []string{tierStepLicense},
		},
		{
			// The deadline that ends the hang is the one the namespace step
			// runs under, so it goes unread as well.
			name:       "a hang past the deadline",
			server:     &fakeGitLabDotCom{namespaces: selfManagedNamespaces, licenseHang: true},
			unresolved: []string{tierStepLicense, tierStepNamespaces},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := tc.server
			client := server.client(t, true)

			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			defer cancel()
			first := client.DetectTier(ctx)
			if first.Tier != edition.Free || !first.Provisional || first.Answered || !slices.Equal(first.Unresolved, tc.unresolved) {
				t.Errorf("first DetectTier() = %+v, want Free, provisional on %v", first, tc.unresolved)
			}

			server.set(func(g *fakeGitLabDotCom) { g.license, g.licensePlan, g.licenseHang = http.StatusOK, "ultimate", false })
			if second := client.DetectTier(t.Context()); second.Tier != edition.Ultimate || second.Provisional || !second.Answered {
				t.Errorf("second DetectTier() = %+v, want Ultimate from the license, definitive", second)
			}
		})
	}
}

// TestDetectTier_LicenseRefusedOrAbsent_IsAnAnswer checks that a license
// GitLab refuses the caller (403, a non-administrator) or does not have (404,
// a CE instance) is its answer: the probe goes on to the namespaces, and the
// Free it reaches is definitive.
func TestDetectTier_LicenseRefusedOrAbsent_IsAnAnswer(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: []map[string]any{{"full_path": "root", "kind": "user", "plan": "default"}}, license: status}
			if got, _ := server.detect(t, true); got.Tier != edition.Free || got.Provisional || len(got.Unresolved) != 0 {
				t.Errorf("DetectTier() = %+v, want Free, definitive", got)
			}
		})
	}
}

// TestResolveTier_Ultimate_IsNeverProvisional checks that a step not read
// cannot make an Ultimate provisional, since nothing a later read answers can
// raise it: the step is still named, for the log, and the answer is final.
func TestResolveTier_Ultimate_IsNeverProvisional(t *testing.T) {
	server := &fakeGitLabDotCom{
		license:    http.StatusBadGateway,
		namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "ultimate"}},
	}
	client := server.client(t, false)
	got := client.ResolveTier(t.Context())
	if got.Tier != edition.Ultimate || got.Provisional || !slices.Equal(got.Unresolved, []string{tierStepLicense}) {
		t.Errorf("ResolveTier() = %+v, want Ultimate, definitive, with the license named unread", got)
	}
}

// TestResolveTier_EveryStepUnanswered_NamesThemInOrder checks that every step
// not read is named, in the order the probe asks them, which is what the
// lines reporting a provisional tier print.
func TestResolveTier_EveryStepUnanswered_NamesThemInOrder(t *testing.T) {
	server := &fakeGitLabDotCom{
		license:          http.StatusBadGateway,
		namespacesStatus: http.StatusServiceUnavailable,
		pages:            []fakeMembershipPage{{status: http.StatusGatewayTimeout}},
	}
	client := server.client(t, false)
	got := client.ResolveTier(t.Context())
	want := []string{tierStepLicense, tierStepNamespaces, tierStepMembership}
	if got.Tier != edition.Free || got.Answered || !got.Provisional || !slices.Equal(got.Unresolved, want) {
		t.Errorf("ResolveTier() = %+v, want Free, nothing answered, provisional on %v", got, want)
	}
}

// TestResolveTier_TouchesNothingOnTheClient checks the property the pool
// relies on to ask a tier again under a registered catalog: the client keeps
// the tier it had whatever the probe answers, and nothing is warned about or
// asked beyond the probe's own steps, the edition included.
func TestResolveTier_TouchesNothingOnTheClient(t *testing.T) {
	cases := []struct {
		name   string
		server *fakeGitLabDotCom
		want   edition.Tier
	}{
		{name: "a higher answer", server: &fakeGitLabDotCom{namespaces: freeNamespaces, pages: ultimateGroup}, want: edition.Ultimate},
		{name: "no answer at all", server: &fakeGitLabDotCom{pages: []fakeMembershipPage{{}}}, want: edition.Free},
		{name: "a provisional answer", server: &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{{status: http.StatusBadGateway}}}, want: edition.Free},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := tc.server.client(t, false)
			client.SetTier(edition.Premium)
			logged := captureClientLog(t)

			if got := client.ResolveTier(t.Context()); got.Tier != tc.want {
				t.Errorf("ResolveTier() = %+v, want %v", got, tc.want)
			}
			if client.Tier() != edition.Premium {
				t.Errorf("client tier = %v, want the Premium it had", client.Tier())
			}
			if strings.Contains(logged.String(), `"level":"WARN"`) {
				t.Errorf("ResolveTier warned:\n%s", logged)
			}
			if asked := tc.server.versionRequests(); asked != 0 {
				t.Errorf("/version asked %d times, want none", asked)
			}
		})
	}
}
