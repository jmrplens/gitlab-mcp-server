package gitlab

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

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
// is how GitLab refuses a document; noConnection answers data with no groups
// field. errors are top-level errors answered beside the page's groups, which
// is how GitLab answers a page it resolved in part: a node one of whose
// non-null features it refused is null, and the reason is in errors.
type fakeMembershipPage struct {
	groups       []*fakeGroup
	hasNext      bool
	status       int
	refused      bool
	noConnection bool
	errors       []string
}

// refusedMessage is the message the refused fixture answers with, so a test
// can tell GitLab's refusal from an answer that carried no groups connection.
const refusedMessage = "The resource that you are attempting to access does not exist or you don't have permission to perform this action"

// fakeGitLabDotCom answers the questions the tier cascade asks the way
// GitLab.com answers a caller who administers no paid namespace: the license
// refused, the namespaces with the plans given (refused when nil), and the
// membership query a page at a time. Every GraphQL request is judged by the
// pinned GitLab schema, document and variables alike, as the transport
// testutil.NewTestClient serves judges them; this package cannot use that
// transport, since testutil imports it.
type fakeGitLabDotCom struct {
	namespaces []map[string]any
	pages      []fakeMembershipPage

	mu    sync.Mutex
	asked []map[string]any // the variables of every membership request
}

// serve answers one request. It runs on the httptest server's goroutine, so it
// reports with Errorf and returns rather than aborting the test.
func (g *fakeGitLabDotCom) serve(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case versionAPIPath:
			respondJSON(w, `{"version":"19.5.0-pre","enterprise":true}`)
		case "/api/v4/license":
			w.WriteHeader(http.StatusForbidden)
		case "/api/v4/namespaces":
			if g.namespaces == nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			body, err := json.Marshal(g.namespaces)
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
	g.mu.Unlock()

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
	if index >= len(g.pages) {
		// A walk past the fixture is answered as the end, so a walk that
		// never stops asking ends here and is caught by the count.
		respondJSON(w, `{"data":{"groups":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)
		return
	}
	page := g.pages[index]
	switch {
	case page.status != 0:
		w.WriteHeader(page.status)
		return
	case page.refused:
		respondJSON(w, `{"data":null,"errors":[{"message":"`+refusedMessage+`"}]}`)
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

// detect builds a client against g, treats it as a GitLab.com client unless
// selfManaged, and resolves its tier.
func (g *fakeGitLabDotCom) detect(t *testing.T, selfManaged bool) (edition.Tier, *Client) {
	t.Helper()
	if !selfManaged {
		previous := membershipTierApplies
		membershipTierApplies = func(*Client) bool { return true }
		t.Cleanup(func() { membershipTierApplies = previous })
	}
	srv := httptest.NewServer(g.serve(t))
	t.Cleanup(srv.Close)
	client, err := NewClient(&config.Config{GitLabURL: srv.URL, GitLabToken: testValidToken, DisableRetries: true})
	if err != nil {
		t.Fatalf(fmtNewClientErr, err)
	}
	return client.DetectTier(t.Context()), client
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
			if got != tc.want || client.Tier() != tc.want {
				t.Errorf("DetectTier() = %v, client tier %v; want %v", got, client.Tier(), tc.want)
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
	if got, _ := server.detect(t, false); got != edition.Ultimate {
		t.Errorf("DetectTier() = %v, want Ultimate", got)
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
	if got, _ := server.detect(t, true); got != edition.Free {
		t.Errorf("DetectTier() = %v, want Free", got)
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
			if got, _ := server.detect(t, false); got != tc.want {
				t.Errorf("DetectTier() = %v, want %v", got, tc.want)
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
	if got, _ := server.detect(t, false); got != edition.Premium {
		t.Errorf("DetectTier() = %v, want Premium: the paid group is on the second page", got)
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
	if got, _ := server.detect(t, false); got != edition.Ultimate {
		t.Errorf("DetectTier() = %v, want Ultimate", got)
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
	if got, _ := server.detect(t, false); got != edition.Free {
		t.Errorf("DetectTier() = %v, want Free: the paid group sits past the page bound", got)
	}
	if asked := server.requests(); len(asked) != membershipMaxPages {
		t.Errorf("membership pages read = %d, want the bound, %d", len(asked), membershipMaxPages)
	}
}

// TestDetectTier_Membership_FailedRead_KeepsTheEarlierAnswer checks what a
// membership read GitLab does not answer resolves to: the answer the namespace
// step already gave, never less. ADR-0018's asymmetry decides it, a tier
// resolved too low silently removing tools, and a failed read is no evidence
// that the caller holds less than it was already shown to hold.
func TestDetectTier_Membership_FailedRead_KeepsTheEarlierAnswer(t *testing.T) {
	premiumNamespace := []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}}
	cases := []struct {
		name       string
		namespaces []map[string]any
		page       fakeMembershipPage
		want       edition.Tier
	}{
		{name: "a server error after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{status: http.StatusInternalServerError}, want: edition.Premium},
		{name: "a refused document after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{refused: true}, want: edition.Premium},
		{name: "every group refused after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{groups: []*fakeGroup{nil}, errors: []string{refusedMessage}}, want: edition.Premium},
		{name: "no groups connection after a premium namespace", namespaces: premiumNamespace, page: fakeMembershipPage{noConnection: true}, want: edition.Premium},
		{name: "a server error after free namespaces", namespaces: freeNamespaces, page: fakeMembershipPage{status: http.StatusInternalServerError}, want: edition.Free},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: tc.namespaces, pages: []fakeMembershipPage{tc.page}}
			if got, _ := server.detect(t, false); got != tc.want {
				t.Errorf("DetectTier() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDetectTier_Membership_FailedLaterPage_KeepsWhatAnEarlierPageAnswered
// checks the failure path mid-walk: a page that fails after an earlier one
// answered keeps that answer.
func TestDetectTier_Membership_FailedLaterPage_KeepsWhatAnEarlierPageAnswered(t *testing.T) {
	server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{
		{groups: []*fakeGroup{{path: "acme", premium: true}}, hasNext: true},
		{status: http.StatusInternalServerError},
	}}
	if got, _ := server.detect(t, false); got != edition.Premium {
		t.Errorf("DetectTier() = %v, want Premium: the first page answered before the second failed", got)
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
		name  string
		pages []fakeMembershipPage
	}{
		{name: "free groups only", pages: []fakeMembershipPage{{groups: []*fakeGroup{{path: "hobby"}}}}},
		{name: "free groups, then a page that fails", pages: []fakeMembershipPage{
			{groups: []*fakeGroup{{path: "hobby"}}, hasNext: true},
			{status: http.StatusInternalServerError},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := &fakeGitLabDotCom{namespaces: premiumNamespace, pages: tc.pages}
			if got, _ := server.detect(t, false); got != edition.Premium {
				t.Errorf("DetectTier() = %v, want Premium: the namespace step answered Premium", got)
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
// warns nobody.
func TestDetectTier_GitLabDotCom_NoStepAnswers_Warns(t *testing.T) {
	cases := []struct {
		name  string
		page  fakeMembershipPage
		warns bool
	}{
		{name: "a member of no group", page: fakeMembershipPage{}, warns: true},
		{name: "a membership read that fails", page: fakeMembershipPage{status: http.StatusInternalServerError}, warns: true},
		{name: "a refused document", page: fakeMembershipPage{refused: true}, warns: true},
		{name: "every group refused", page: fakeMembershipPage{groups: []*fakeGroup{nil, nil}, errors: []string{refusedMessage}}, warns: true},
		{name: "free groups", page: fakeMembershipPage{groups: []*fakeGroup{{path: "hobby"}}}, warns: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureClientLog(t)
			server := &fakeGitLabDotCom{pages: []fakeMembershipPage{tc.page}}
			if got, _ := server.detect(t, false); got != edition.Free {
				t.Errorf("DetectTier() = %v, want Free", got)
			}
			warned := strings.Contains(logged.String(), `"level":"WARN"`) &&
				strings.Contains(logged.String(), "could not determine the licensing tier") &&
				strings.Contains(logged.String(), "GITLAB_MCP_TIER")
			if warned != tc.warns {
				t.Errorf("warned naming GITLAB_MCP_TIER = %t, want %t:\n%s", warned, tc.warns, logged)
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
	if got, _ := server.detect(t, false); got != edition.Premium {
		t.Errorf("DetectTier() = %v, want Premium: the page resolved a Premium group beside a refused one", got)
	}
	if asked := server.requests(); len(asked) != 1 {
		t.Errorf("membership pages read = %d, want 1: a page GitLab answered in part ends the walk", len(asked))
	}
}

// TestDetectTier_Membership_FailedRead_SaysSoAtInfo checks that a membership
// read on GitLab.com that did not come back whole is reported above debug,
// naming the setting that pins the tier. The namespace step may already have
// answered Free, so no warning fires, and without this line the tier a caller
// is served would say nothing about the step that could have raised it.
func TestDetectTier_Membership_FailedRead_SaysSoAtInfo(t *testing.T) {
	cases := []struct {
		name string
		page fakeMembershipPage
		says bool
	}{
		{name: "a server error", page: fakeMembershipPage{status: http.StatusInternalServerError}, says: true},
		{name: "a refused document", page: fakeMembershipPage{refused: true}, says: true},
		{name: "a page answered in part", page: fakeMembershipPage{groups: []*fakeGroup{nil}, errors: []string{refusedMessage}}, says: true},
		{name: "free groups", page: fakeMembershipPage{groups: []*fakeGroup{{path: "hobby"}}}, says: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logged := captureClientLog(t)
			server := &fakeGitLabDotCom{namespaces: freeNamespaces, pages: []fakeMembershipPage{tc.page}}
			if got, _ := server.detect(t, false); got != edition.Free {
				t.Errorf("DetectTier() = %v, want Free", got)
			}
			said := false
			for line := range strings.Lines(logged.String()) {
				if strings.Contains(line, `"level":"INFO"`) && strings.Contains(line, "top-level group") && strings.Contains(line, "GITLAB_MCP_TIER") {
					said = true
				}
			}
			if said != tc.says {
				t.Errorf("an INFO line naming the membership read and GITLAB_MCP_TIER = %t, want %t:\n%s", said, tc.says, logged)
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
