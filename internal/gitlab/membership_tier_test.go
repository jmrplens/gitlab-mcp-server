// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// TestMembershipTierApplies_IsGitLabDotComAlone holds the membership step to
// the one instance that sells its plans per top-level group. Every other
// instance is licensed as a whole, which the license step reads, and asking a
// self-managed instance which groups a caller belongs to would cost a request
// that cannot change the answer the cascade gives there today.
func TestMembershipTierApplies_IsGitLabDotComAlone(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{url: "https://gitlab.com", want: true},
		{url: "https://GitLab.com/", want: true},
		{url: "https://gitlab.example.com", want: false},
		{url: "https://gitlab.com.example.org", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			client, err := NewClient(newTestConfig(tc.url, testValidToken))
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			if got := membershipTierApplies(client); got != tc.want {
				t.Errorf("membershipTierApplies(%s) = %t, want %t", tc.url, got, tc.want)
			}
		})
	}
}

// fakeGroup is one top-level group as the membership query returns it: its
// path and whether its plan carries the Premium and the Ultimate feature the
// query asks about. A nil *fakeGroup is a null node.
type fakeGroup struct {
	path              string
	premium, ultimate bool
}

// fakeMembershipPage is what one page of the membership query answers. A
// status other than zero answers the request with that status and no body;
// refused answers HTTP 200 with a top-level errors array and data null, which
// is how GitLab refuses a document, with refusedMessage unless message names
// another; noConnection answers data with no groups field; hang answers
// nothing until the request ends, which is a GitLab that does not answer in
// time. errors are top-level errors answered beside the page's groups, which
// is how GitLab answers a page it resolved in part: a node one of whose
// non-null features it refused is null, and the reason is in errors.
type fakeMembershipPage struct {
	groups       []*fakeGroup
	hasNext      bool
	status       int
	refused      bool
	message      string
	noConnection bool
	hang         bool
	errors       []string
}

// refusedMessage is the message the refused fixture answers with, so a test
// can tell GitLab's refusal from an answer that carried no groups connection.
// It is GitLab's permission refusal, written out rather than read from the
// package, so a change of the constant the code compares against is caught.
const refusedMessage = "The resource that you are attempting to access does not exist or you don't have permission to perform this action"

// fakeGitLabDotCom answers the questions the tier cascade asks the way
// GitLab.com answers a caller who administers no paid namespace: the license
// refused, the namespaces with the plans given (refused when nil), and the
// membership query a page at a time. Every GraphQL request is judged by the
// pinned GitLab schema, document and variables alike, as the transport
// testutil.NewTestClient serves judges them; this package cannot use that
// transport, since testutil imports it.
//
// Its answers can change between two detections on the same client (set),
// which is how a test shows that a step GitLab did not answer is answered
// when asked again.
type fakeGitLabDotCom struct {
	namespaces []map[string]any
	pages      []fakeMembershipPage
	// license is the status /license answers, 403 when zero; with 200 it
	// answers licensePlan. licenseHang answers nothing until the request
	// ends.
	license     int
	licensePlan string
	licenseHang bool
	// namespacesStatus, when not zero, is the status /namespaces answers in
	// place of the list; namespacesHang answers nothing until the request
	// ends.
	namespacesStatus int
	namespacesHang   bool
	// failFirst are the statuses the first membership requests are answered
	// with, one each and with no body, before any page is: how a test makes
	// GitLab fail once and then answer.
	failFirst []int

	mu       sync.Mutex
	asked    []map[string]any // the variables of every membership request
	versions int              // the /version requests
}

// set changes what g answers from the next request on.
func (g *fakeGitLabDotCom) set(change func(*fakeGitLabDotCom)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	change(g)
}

// answers is what g answers now, read under its lock, so a test that changes
// it between two detections races with no handler still running.
func (g *fakeGitLabDotCom) answers() fakeGitLabDotCom {
	g.mu.Lock()
	defer g.mu.Unlock()
	return fakeGitLabDotCom{
		namespaces: g.namespaces, pages: g.pages,
		license: g.license, licensePlan: g.licensePlan, licenseHang: g.licenseHang,
		namespacesStatus: g.namespacesStatus, namespacesHang: g.namespacesHang,
	}
}

// versionRequests is how many times /version was asked.
func (g *fakeGitLabDotCom) versionRequests() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.versions
}

// hangUntilTheRequestEnds answers nothing until the client gives up on r or
// the server closes, which is a GitLab that does not answer in time.
func hangUntilTheRequestEnds(r *http.Request) {
	<-r.Context().Done()
}

// serve answers one request. It runs on the httptest server's goroutine, so it
// reports with Errorf and returns rather than aborting the test.
func (g *fakeGitLabDotCom) serve(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		now := g.answers()
		switch r.URL.Path {
		case versionAPIPath:
			g.set(func(g *fakeGitLabDotCom) { g.versions++ })
			respondJSON(w, `{"version":"19.5.0-pre","enterprise":true}`)
		case "/api/v4/license":
			switch {
			case now.licenseHang:
				hangUntilTheRequestEnds(r)
			case now.license == http.StatusOK:
				respondJSON(w, `{"plan":"`+now.licensePlan+`"}`)
			case now.license != 0:
				w.WriteHeader(now.license)
			default:
				w.WriteHeader(http.StatusForbidden)
			}
		case "/api/v4/namespaces":
			if now.namespacesHang {
				hangUntilTheRequestEnds(r)
				return
			}
			if now.namespacesStatus != 0 {
				w.WriteHeader(now.namespacesStatus)
				return
			}
			if now.namespaces == nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			body, err := json.Marshal(now.namespaces)
			if err != nil {
				t.Errorf("encoding the namespaces fixture: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			respondJSON(w, string(body))
		case "/api/graphql":
			g.membership(t, w, r)
		default:
			t.Errorf("the tier cascade asked %s %s, which it has no reason to", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// respondJSON writes body as a JSON answer with status 200.
func respondJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// membership answers one page of the membership query, chosen by the cursor
// the request carries: none is the first page and "page-N" is page N+1.
func (g *fakeGitLabDotCom) membership(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	var request struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		t.Errorf("decoding the membership request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := graphqlschema.Validate(request.Query, request.Variables); err != nil {
		t.Errorf("GitLab would refuse the membership request: %v", err)
	}
	g.mu.Lock()
	g.asked = append(g.asked, request.Variables)
	pages := g.pages
	failure := 0
	if len(g.failFirst) > 0 {
		failure, g.failFirst = g.failFirst[0], g.failFirst[1:]
	}
	g.mu.Unlock()
	if failure != 0 {
		w.WriteHeader(failure)
		return
	}

	index := 0
	if after, ok := request.Variables["after"].(string); ok {
		number, handedOut := strings.CutPrefix(after, "page-")
		parsed, err := strconv.Atoi(number)
		if !handedOut || err != nil {
			t.Errorf("cursor %q is not one this fixture handed out", after)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		index = parsed
	}
	if index >= len(pages) {
		// A walk past the fixture is answered as the end, so a walk that
		// never stops asking ends here and is caught by the count.
		respondJSON(w, `{"data":{"groups":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)
		return
	}
	page := pages[index]
	switch {
	case page.hang:
		hangUntilTheRequestEnds(r)
		return
	case page.status != 0:
		w.WriteHeader(page.status)
		return
	case page.refused:
		message := cmp.Or(page.message, refusedMessage)
		body, err := json.Marshal(map[string]any{"data": nil, "errors": []map[string]any{{"message": message}}})
		if err != nil {
			t.Errorf("encoding the refusal: %v", err)
		}
		respondJSON(w, string(body))
		return
	case page.noConnection:
		respondJSON(w, `{"data":{}}`)
		return
	}
	nodes := make([]any, 0, len(page.groups))
	for _, member := range page.groups {
		if member == nil {
			nodes = append(nodes, nil)
			continue
		}
		nodes = append(nodes, map[string]any{
			"fullPath": member.path,
			"premium":  map[string]any{"available": member.premium},
			"ultimate": map[string]any{"available": member.ultimate},
		})
	}
	cursor := ""
	if page.hasNext {
		cursor = "page-" + strconv.Itoa(index+1)
	}
	answer := map[string]any{"data": map[string]any{"groups": map[string]any{
		"nodes":    nodes,
		"pageInfo": map[string]any{"hasNextPage": page.hasNext, "endCursor": cursor},
	}}}
	if len(page.errors) > 0 {
		refusals := make([]map[string]any, 0, len(page.errors))
		for _, message := range page.errors {
			refusals = append(refusals, map[string]any{"message": message})
		}
		answer["errors"] = refusals
	}
	body, err := json.Marshal(answer)
	if err != nil {
		t.Errorf("encoding the membership page: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	respondJSON(w, string(body))
}

// requests returns the variables of every membership request so far.
func (g *fakeGitLabDotCom) requests() []map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]map[string]any(nil), g.asked...)
}

// client builds a client against g with retries off, and treats it as a
// GitLab.com client unless selfManaged.
func (g *fakeGitLabDotCom) client(t *testing.T, selfManaged bool) *Client {
	t.Helper()
	if !selfManaged {
		standInForGitLabDotCom(t)
	}
	srv := httptest.NewServer(g.serve(t))
	t.Cleanup(srv.Close)
	client, err := NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: testValidToken, DisableRetries: true})
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	return client
}

// retryingClient builds a GitLab.com client against g with the production
// retry policy and a wait ceiling short enough for a test (testRetryCeiling),
// which is how the pool's clients are built.
func (g *fakeGitLabDotCom) retryingClient(t *testing.T) *Client {
	t.Helper()
	standInForGitLabDotCom(t)
	srv := httptest.NewServer(g.serve(t))
	t.Cleanup(srv.Close)
	client, err := newTokenClient(srv.URL, testValidToken, false, false, testRetryCeiling)
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	return client
}

// standInForGitLabDotCom makes every client of the test a GitLab.com client,
// since a test server cannot have GitLab.com's host.
func standInForGitLabDotCom(t *testing.T) {
	t.Helper()
	previous := membershipTierApplies
	membershipTierApplies = func(*Client) bool { return true }
	t.Cleanup(func() { membershipTierApplies = previous })
}

// detect builds a client against g, treats it as a GitLab.com client unless
// selfManaged, and resolves its tier.
func (g *fakeGitLabDotCom) detect(t *testing.T, selfManaged bool) (TierResolution, *Client) {
	t.Helper()
	client := g.client(t, selfManaged)
	return client.DetectTier(t.Context(), t.Context()), client
}

// freeNamespaces is what GitLab.com lists for the maintainer's account, a
// Developer of a subgroup of a licensed group who owns one free group: the
// personal namespace and the owned group each report "free", and the group
// the account is only a member of reports no plan at all. Measured on
// 2026-10-08.
var freeNamespaces = []map[string]any{
	{"full_path": "jmrp", "kind": "user", "plan": "free"},
	{"full_path": "plens1", "kind": "group", "plan": "free"},
	{"full_path": "gitlab-community/community-members", "kind": "group"},
}

// TestDetectTier_GitLabDotComMember_ResolvesTheTopLevelGroupsPlan is the case
// issue 1224 measured: a GitLab.com account administering no paid namespace is
// a member of a group whose top-level group is licensed, and is served that
// tier instead of Free. The first row is the maintainer's account as GitLab.com
// answered it on 2026-10-08.
func TestDetectTier_GitLabDotComMember_ResolvesTheTopLevelGroupsPlan(t *testing.T) {
	cases := []struct {
		name   string
		groups []*fakeGroup
		want   edition.Tier
	}{
		{
			name: "a developer of a subgroup of an ultimate group",
			groups: []*fakeGroup{
				{path: "gitlab-community", premium: true, ultimate: true},
				{path: "plens1"},
			},
			want: edition.Ultimate,
		},
		{
			name:   "a member of a premium group",
			groups: []*fakeGroup{{path: "acme", premium: true}},
			want:   edition.Premium,
		},
		{
			name:   "a member of free groups only",
			groups: []*fakeGroup{{path: "plens1"}, {path: "hobby"}},
			want:   edition.Free,
		},
		{
			name:   "a null node is skipped",
			groups: []*fakeGroup{nil, {path: "acme", premium: true}},
			want:   edition.Premium,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{{groups: tc.groups}}}
			got, client := server.detect(t, false)
			if got.Tier != tc.want || client.Tier() != tc.want || got.Provisional {
				t.Errorf("DetectTier() = %+v, client tier %v; want %v, not provisional", got, client.Tier(), tc.want)
			}
			asked := server.requests()
			if len(asked) != 1 {
				t.Fatalf("membership pages read = %d, want 1", len(asked))
			}
			if first, ok := asked[0]["first"].(float64); !ok || int(first) != membershipPageSize {
				t.Errorf("first = %v, want the register's page size %d", asked[0]["first"], membershipPageSize)
			}
			if _, ok := asked[0]["after"]; ok {
				t.Errorf("the first page carried after = %v, want no cursor", asked[0]["after"])
			}
		})
	}
}

// TestDetectTier_NamespaceAlreadyUltimate_AsksNoMembership checks that the
// membership step is skipped when the namespace step already found the
// highest tier there is, since no membership could raise it.
func TestDetectTier_NamespaceAlreadyUltimate_AsksNoMembership(t *testing.T) {
	server := &fakeGitLabDotCom{
		namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "ultimate"}},
		pages:      []fakeMembershipPage{{groups: []*fakeGroup{{path: "other", premium: true}}}},
	}
	if got, _ := server.detect(t, false); got.Tier != edition.Ultimate || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Ultimate, not provisional", got)
	}
	if asked := server.requests(); len(asked) != 0 {
		t.Errorf("membership pages read = %d, want 0: the namespace plan already answered Ultimate", len(asked))
	}
}

// TestDetectTier_SelfManaged_AsksNoMembership checks that the membership step
// is GitLab.com's alone. A self-managed instance sells its tier as an instance
// license, which the first step reads, and a self-managed client keeps the
// cascade it had.
func TestDetectTier_SelfManaged_AsksNoMembership(t *testing.T) {
	server := &fakeGitLabDotCom{
		namespaces: []map[string]any{{"full_path": "root", "kind": "user", "plan": "default"}},
		pages:      []fakeMembershipPage{{groups: []*fakeGroup{{path: "acme", premium: true, ultimate: true}}}},
	}
	if got, _ := server.detect(t, true); got.Tier != edition.Free || got.Answered || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Free with no step answering and none unread", got)
	}
	if asked := server.requests(); len(asked) != 0 {
		t.Errorf("membership pages read = %d, want 0 on a self-managed instance", len(asked))
	}
}

// TestDetectTier_Membership_AnswersWhereNoNamespaceDid covers a GitLab.com
// caller whose namespace listing was refused: the membership step answers on
// its own, and a membership listing nobody is in leaves the tier unresolved,
// which falls back to Free as before.
func TestDetectTier_Membership_AnswersWhereNoNamespaceDid(t *testing.T) {
	cases := []struct {
		name   string
		groups []*fakeGroup
		want   edition.Tier
	}{
		{name: "a premium membership", groups: []*fakeGroup{{path: "acme", premium: true}}, want: edition.Premium},
		{name: "no membership at all", groups: nil, want: edition.Free},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{pages: []fakeMembershipPage{{groups: tc.groups}}}
			if got, _ := server.detect(t, false); got.Tier != tc.want || got.Provisional {
				t.Errorf("DetectTier() = %+v, want %v, not provisional", got, tc.want)
			}
		})
	}
}

// TestDetectTier_Membership_ReadsPastTheFirstPage checks the walk follows the
// cursor GitLab hands back: the paid group is on the second page.
func TestDetectTier_Membership_ReadsPastTheFirstPage(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{
		{groups: []*fakeGroup{{path: "plens1"}}, hasNext: true},
		{groups: []*fakeGroup{{path: "acme", premium: true}}},
	}}
	if got, _ := server.detect(t, false); got.Tier != edition.Premium || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Premium, not provisional: the paid group is on the second page", got)
	}
	asked := server.requests()
	if len(asked) != 2 {
		t.Fatalf("membership pages read = %d, want 2", len(asked))
	}
	if asked[1]["after"] != "page-1" {
		t.Errorf("the second page asked after = %v, want the first page's end cursor", asked[1]["after"])
	}
}

// TestDetectTier_Membership_StopsAtUltimate checks the early exit: no later
// group can raise the answer past the highest tier there is.
func TestDetectTier_Membership_StopsAtUltimate(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{
		{groups: []*fakeGroup{{path: "top", premium: true, ultimate: true}}, hasNext: true},
		{groups: []*fakeGroup{{path: "never-read", premium: true}}},
	}}
	if got, _ := server.detect(t, false); got.Tier != edition.Ultimate || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Ultimate, not provisional", got)
	}
	if asked := server.requests(); len(asked) != 1 {
		t.Errorf("membership pages read = %d, want 1: Ultimate is the highest answer there is", len(asked))
	}
}

// TestDetectTier_Membership_StopsAtThePageBound checks the bound the register
// sets (AUT-003): an account reaching more top-level groups than the bound
// covers is read that far and no further, and resolves on what it was shown.
func TestDetectTier_Membership_StopsAtThePageBound(t *testing.T) {
	pages := make([]fakeMembershipPage, membershipMaxPages+1)
	for i := range pages {
		pages[i] = fakeMembershipPage{groups: []*fakeGroup{{path: "free-" + strconv.Itoa(i)}}, hasNext: true}
	}
	pages[membershipMaxPages].groups = []*fakeGroup{{path: "past-the-bound", premium: true}}
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: pages}
	// The bound is policy rather than a read that failed, so the answer is
	// definitive and is not asked again.
	if got, _ := server.detect(t, false); got.Tier != edition.Free || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Free, not provisional: the paid group sits past the page bound", got)
	}
	if asked := server.requests(); len(asked) != membershipMaxPages {
		t.Errorf("membership pages read = %d, want the bound, %d", len(asked), membershipMaxPages)
	}
}

// TestDetectTier_Membership_FailedRead_KeepsTheEarlierAnswer checks what a
// membership read that did not come back whole resolves to: the answer the
// namespace step already gave, never less. ADR-0018's asymmetry decides it, a
// tier resolved too low silently removing tools, and a failed read is no
// evidence that the caller holds less than it was already shown to hold. A
// read GitLab did not answer, a server error or an answer with no groups
// connection, also leaves the answer provisional, while GitLab's refusal of
// the caller is an answer and does not (issue 1782).
func TestDetectTier_Membership_FailedRead_KeepsTheEarlierAnswer(t *testing.T) {
	premiumNamespace := []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}}
	cases := []struct {
		name        string
		namespaces  []map[string]any
		page        fakeMembershipPage
		want        edition.Tier
		provisional bool
	}{
		{name: "a server error after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{status: http.StatusInternalServerError}, want: edition.Premium, provisional: true},
		{name: "a refused document after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{refused: true}, want: edition.Premium},
		{name: "every group refused after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{groups: []*fakeGroup{nil}, errors: []string{refusedMessage}}, want: edition.Premium},
		{name: "no groups connection after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{noConnection: true}, want: edition.Premium, provisional: true},
		{name: "a server error after free namespaces", namespaces: freeNamespaces, page: fakeMembershipPage{status: http.StatusInternalServerError}, want: edition.Free, provisional: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: tc.namespaces, pages: []fakeMembershipPage{tc.page}}
			got, _ := server.detect(t, false)
			if got.Tier != tc.want || got.Provisional != tc.provisional || !got.Answered {
				t.Errorf("DetectTier() = %+v, want %v answered, provisional %t", got, tc.want, tc.provisional)
			}
			if wantUnresolved := tc.provisional; slices.Equal(got.Unresolved, []string{"membership"}) != wantUnresolved {
				t.Errorf("Unresolved = %v, want the membership step named: %t", got.Unresolved, wantUnresolved)
			}
		})
	}
}

// TestDetectTier_Membership_FailedLaterPage_KeepsWhatAnEarlierPageAnswered
// checks the failure path mid-walk: a page that fails after an earlier one
// answered keeps that answer, provisional, since the page that failed may
// have carried a higher one.
func TestDetectTier_Membership_FailedLaterPage_KeepsWhatAnEarlierPageAnswered(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{
		{groups: []*fakeGroup{{path: "acme", premium: true}}, hasNext: true},
		{status: http.StatusInternalServerError},
	}}
	if got, _ := server.detect(t, false); got.Tier != edition.Premium || !got.Provisional {
		t.Errorf("DetectTier() = %+v, want Premium, provisional: the first page answered before the second failed", got)
	}
}

// TestDetectTier_Membership_LowerAnswer_NeverLowersTheNamespaces checks that
// the membership step can only raise the tier the namespace step found. A
// caller who owns a Premium group and is a member of free ones holds Premium,
// and so does one whose membership walk answered Free on its first page and
// then failed: the membership answer that did come back is lower, and taking
// it would remove every Premium action without a word.
func TestDetectTier_Membership_LowerAnswer_NeverLowersTheNamespaces(t *testing.T) {
	premiumNamespace := []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}}
	cases := []struct {
		name        string
		pages       []fakeMembershipPage
		provisional bool
	}{
		{name: "free groups only", pages: []fakeMembershipPage{{groups: []*fakeGroup{{path: "hobby"}}}}},
		{name: "free groups, then a page that fails", pages: []fakeMembershipPage{
			{groups: []*fakeGroup{{path: "hobby"}}, hasNext: true},
			{status: http.StatusInternalServerError},
		}, provisional: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: premiumNamespace, pages: tc.pages}
			if got, _ := server.detect(t, false); got.Tier != edition.Premium || got.Provisional != tc.provisional {
				t.Errorf("DetectTier() = %+v, want Premium, provisional %t: the namespace step answered Premium", got, tc.provisional)
			}
			if asked := server.requests(); len(asked) != len(tc.pages) {
				t.Errorf("membership pages read = %d, want %d", len(asked), len(tc.pages))
			}
		})
	}
}

// TestDetectTier_GitLabDotCom_NoStepAnswers_Warns checks that the warning
// naming GITLAB_MCP_TIER still fires on GitLab.com when no step answered. It
// is the one signal an operator gets there, and the path a fine-grained token
// takes: the namespace listing refused, and every group of the membership
// page refused as well, which GitLab answers as null nodes beside an errors
// array. A membership answer of Free with groups in it has answered, and
// warns nobody. A membership read GitLab did not answer leaves the tier
// provisional, which is warned about in its own words, naming the step, and
// in place of the enterprise warning: the step that went unanswered is why
// the tier may be wrong, whatever the build.
func TestDetectTier_GitLabDotCom_NoStepAnswers_Warns(t *testing.T) {
	cases := []struct {
		name        string
		page        fakeMembershipPage
		warns       bool
		provisional bool
	}{
		{name: "a member of no group", page: fakeMembershipPage{}, warns: true},
		{name: "a membership read that fails", page: fakeMembershipPage{status: http.StatusInternalServerError}, warns: false, provisional: true},
		{name: "a refused document", page: fakeMembershipPage{refused: true}, warns: true},
		{name: "every group refused", page: fakeMembershipPage{groups: []*fakeGroup{nil, nil}, errors: []string{refusedMessage}}, warns: true},
		{name: "free groups", page: fakeMembershipPage{groups: []*fakeGroup{{path: "hobby"}}}, warns: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureClientLog(t)
			server := &fakeGitLabDotCom{pages: []fakeMembershipPage{tc.page}}
			if got, _ := server.detect(t, false); got.Tier != edition.Free || got.Provisional != tc.provisional {
				t.Errorf("DetectTier() = %+v, want Free, provisional %t", got, tc.provisional)
			}
			warned, warnedProvisional := false, false
			for line := range strings.Lines(logged.String()) {
				if !strings.Contains(line, `"level":"WARN"`) || !strings.Contains(line, "GITLAB_MCP_TIER") {
					continue
				}
				warned = warned || strings.Contains(line, "could not determine the licensing tier")
				warnedProvisional = warnedProvisional || (strings.Contains(line, "could not finish detecting the licensing tier") &&
					strings.Contains(line, `"unresolved":["membership"]`))
			}
			if warned != tc.warns {
				t.Errorf("warned naming GITLAB_MCP_TIER = %t, want %t:\n%s", warned, tc.warns, logged)
			}
			if warnedProvisional != tc.provisional {
				t.Errorf("warned that the membership step went unanswered = %t, want %t:\n%s", warnedProvisional, tc.provisional, logged)
			}
		})
	}
}

// TestDetectTier_Membership_PartialPage_KeepsTheGroupsItResolved checks the
// rule for a page GitLab answered in part. LicensedFeatureAvailability and its
// available field are non-null, so a feature GitLab refuses nulls the whole
// node and every node that came back was resolved in full: a group answering
// Premium is evidence of Premium, and discarding it beside a refused sibling
// would resolve lower than the caller was shown to hold. The walk ends at that
// page, since a later one is refused the same way, which bounds what a
// fine-grained token's walk costs to one request.
func TestDetectTier_Membership_PartialPage_KeepsTheGroupsItResolved(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{
		{groups: []*fakeGroup{nil, {path: "acme", premium: true}}, errors: []string{refusedMessage}, hasNext: true},
		{groups: []*fakeGroup{{path: "never-read", premium: true, ultimate: true}}},
	}}
	// A page GitLab answered in part is its answer, so the walk it ends is
	// not provisional.
	if got, _ := server.detect(t, false); got.Tier != edition.Premium || got.Provisional {
		t.Errorf("DetectTier() = %+v, want Premium, not provisional: the page resolved a Premium group beside a refused one", got)
	}
	if asked := server.requests(); len(asked) != 1 {
		t.Errorf("membership pages read = %d, want 1: a page GitLab answered in part ends the walk", len(asked))
	}
}

// TestDetectTier_Membership_FailedRead_SaysSoAtInfo checks that a membership
// read on GitLab.com that did not come back whole is reported above debug,
// naming the setting that pins the tier. The namespace step may already have
// answered Free, so no warning fires, and without this line the tier a caller
// is served would say nothing about the step that could have raised it. The
// line says whether the read was GitLab's answer or went unanswered.
func TestDetectTier_Membership_FailedRead_SaysSoAtInfo(t *testing.T) {
	cases := []struct {
		name       string
		page       fakeMembershipPage
		says       bool
		unresolved bool
	}{
		{name: "a server error", page: fakeMembershipPage{status: http.StatusInternalServerError}, says: true, unresolved: true},
		{name: "a refused document", page: fakeMembershipPage{refused: true}, says: true},
		{name: "a page answered in part", page: fakeMembershipPage{groups: []*fakeGroup{nil}, errors: []string{refusedMessage}}, says: true},
		{name: "free groups", page: fakeMembershipPage{groups: []*fakeGroup{{path: "hobby"}}}, says: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureClientLog(t)
			server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{tc.page}}
			if got, _ := server.detect(t, false); got.Tier != edition.Free {
				t.Errorf("DetectTier() = %+v, want Free", got)
			}
			said, unresolved := false, false
			for line := range strings.Lines(logged.String()) {
				if strings.Contains(line, `"level":"INFO"`) && strings.Contains(line, "top-level group") && strings.Contains(line, "GITLAB_MCP_TIER") {
					said = true
					unresolved = strings.Contains(line, `"unresolved":true`)
				}
			}
			if said != tc.says || unresolved != tc.unresolved {
				t.Errorf("an INFO line naming the membership read and GITLAB_MCP_TIER = %t (unresolved %t), want %t (unresolved %t):\n%s",
					said, unresolved, tc.says, tc.unresolved, logged)
			}
		})
	}
}

// TestMembershipPage_TellsARefusalFromAPartialAnswer checks what one page
// returns for each shape GitLab answers with: a document it refused carries
// GitLab's own reason, an answer with no groups connection says so, and a page
// answered in part returns the groups it resolved together with the reason
// for the rest.
func TestMembershipPage_TellsARefusalFromAPartialAnswer(t *testing.T) {
	cases := []struct {
		name       string
		page       fakeMembershipPage
		wantGroups bool
		wantErr    string
	}{
		{name: "refused", page: fakeMembershipPage{refused: true}, wantErr: refusedMessage},
		{name: "no groups connection", page: fakeMembershipPage{noConnection: true}, wantErr: errNoGroupsConnection.Error()},
		{name: "answered in part", page: fakeMembershipPage{groups: []*fakeGroup{nil, {path: "acme"}}, errors: []string{"one", "two"}}, wantGroups: true, wantErr: "one; two"},
		{name: "answered whole", page: fakeMembershipPage{groups: []*fakeGroup{{path: "acme"}}}, wantGroups: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{pages: []fakeMembershipPage{tc.page}}
			srv := httptest.NewServer(server.serve(t))
			t.Cleanup(srv.Close)
			client, err := NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: testValidToken, DisableRetries: true})
			if err != nil {
				t.Fatalf(fmtNewClientErr, err)
			}
			groups, err := client.membershipPage(t.Context(), "")
			if (groups != nil) != tc.wantGroups {
				t.Errorf("groups = %v, want returned: %t", groups, tc.wantGroups)
			}
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("err = %v, want none", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("err = %v, want one naming %q", err, tc.wantErr)
			}
		})
	}
}

// TestClient_MembershipRetry_RetriesTheReadLikeAGet holds the retry check of
// the membership query to retryablehttp's default policy, which is what a GET
// is retried by for a 5xx: a 5xx other than 501, a 429 and a failure to reach
// GitLab are retried, and nothing is once the context has ended or on a
// client built with retries off (issue 1782).
func TestClient_MembershipRetry_RetriesTheReadLikeAGet(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	refused := &url.Error{Op: "Post", URL: "https://gitlab.example/api/graphql", Err: errors.New("connection refused")}
	cases := []struct {
		name     string
		disabled bool
		ctx      context.Context
		status   int
		err      error
		retry    bool
	}{
		{name: "a 502", ctx: t.Context(), status: http.StatusBadGateway, retry: true},
		{name: "a 500", ctx: t.Context(), status: http.StatusInternalServerError, retry: true},
		{name: "a 504", ctx: t.Context(), status: http.StatusGatewayTimeout, retry: true},
		{name: "a 501", ctx: t.Context(), status: http.StatusNotImplemented, retry: false},
		{name: "a 429", ctx: t.Context(), status: http.StatusTooManyRequests, retry: true},
		{name: "a 403", ctx: t.Context(), status: http.StatusForbidden, retry: false},
		{name: "a 200", ctx: t.Context(), status: http.StatusOK, retry: false},
		{name: "GitLab not reached", ctx: t.Context(), err: refused, retry: true},
		{name: "a 502 after the context ended", ctx: ended, status: http.StatusBadGateway, retry: false},
		{name: "a 502 on a client built with retries off", disabled: true, ctx: t.Context(), status: http.StatusBadGateway, retry: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &Client{retriesDisabled: tc.disabled}
			var resp *http.Response
			if tc.err == nil {
				resp = &http.Response{StatusCode: tc.status, Status: http.StatusText(tc.status), Body: http.NoBody}
			}
			retry, _ := client.membershipRetry(tc.ctx, resp, tc.err)
			if retry != tc.retry {
				t.Errorf("membershipRetry() = %t, want %t", retry, tc.retry)
			}
		})
	}
}

// TestDetectTier_Membership_IsRetriedOnTheClientsPolicy drives the retry
// through client-go with the production policy: a page GitLab answers 502 and
// then answers is read on the second request, a 501 or a 502 on every attempt
// leaves the step unread after as many requests as RTC-006 allows, and a page
// whose clock ran out is not asked again.
func TestDetectTier_Membership_IsRetriedOnTheClientsPolicy(t *testing.T) {
	cases := []struct {
		name        string
		failFirst   []int
		pages       []fakeMembershipPage
		base        time.Duration
		want        edition.Tier
		provisional bool
		requests    int
	}{
		{name: "a 502, then the page", failFirst: []int{http.StatusBadGateway}, pages: ultimateGroup, want: edition.Ultimate, requests: 2},
		{name: "a 429, then the page", failFirst: []int{http.StatusTooManyRequests}, pages: ultimateGroup, want: edition.Ultimate, requests: 2},
		{name: "a 501", failFirst: []int{http.StatusNotImplemented}, pages: ultimateGroup, want: edition.Free, provisional: true, requests: 1},
		{
			name:      "a 502 every time",
			failFirst: []int{http.StatusBadGateway, http.StatusBadGateway, http.StatusBadGateway},
			pages:     ultimateGroup, want: edition.Free, provisional: true, requests: maxRetries + 1,
		},
		{
			name:  "a page that hangs past its clock",
			pages: []fakeMembershipPage{{hang: true}}, base: 200 * time.Millisecond,
			want: edition.Free, provisional: true, requests: 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: tc.pages, failFirst: tc.failFirst}
			client := server.retryingClient(t)
			base := t.Context()
			if tc.base > 0 {
				var cancel context.CancelFunc
				base, cancel = context.WithTimeout(t.Context(), tc.base)
				defer cancel()
			}
			got := client.DetectTier(t.Context(), base)
			if got.Tier != tc.want || got.Provisional != tc.provisional {
				t.Errorf("DetectTier() = %+v, want %v, provisional %t", got, tc.want, tc.provisional)
			}
			if asked := server.requests(); len(asked) != tc.requests {
				t.Errorf("membership requests = %d, want %d", len(asked), tc.requests)
			}
		})
	}
}
