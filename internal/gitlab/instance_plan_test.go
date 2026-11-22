// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// instance_plan_test.go holds the first step of the tier probe from GitLab
// 19.6 on, the instance plan the version answer carries (issue 1491): which
// answers [parseInstancePlan] keeps, that [Client.versionDirect] stores the
// one it read, and what [Client.DetectTier] asks after each.
package gitlab

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// TestParseInstancePlan_KeepsOnlyAPlanGitLabCouldSend holds the reading of the
// raw value: an absent key, null, the four plans
// ee/app/models/ee/app_config/instance_metadata.rb sends, and anything else,
// which is no answer whatever it looks like, since under
// --allow-any-gitlab-url the instance is one a caller named.
func TestParseInstancePlan_KeepsOnlyAPlanGitLabCouldSend(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		kind instancePlanKind
		tier edition.Tier
	}{
		{name: "absent", raw: "", kind: instancePlanAbsent},
		{name: "null", raw: "null", kind: instancePlanPerNamespace},
		{name: "free", raw: `"free"`, kind: instancePlanRecognized, tier: edition.Free},
		{name: "starter", raw: `"starter"`, kind: instancePlanRecognized, tier: edition.Premium},
		{name: "premium", raw: `"premium"`, kind: instancePlanRecognized, tier: edition.Premium},
		{name: "ultimate", raw: `"ultimate"`, kind: instancePlanRecognized, tier: edition.Ultimate},
		{name: "a legacy license plan", raw: `"gold"`, kind: instancePlanUnrecognized},
		{name: "another spelling", raw: `"Ultimate"`, kind: instancePlanUnrecognized},
		{name: "a trial suffix", raw: `"ultimate_trial"`, kind: instancePlanUnrecognized},
		{name: "empty string", raw: `""`, kind: instancePlanUnrecognized},
		{name: "a number", raw: `42`, kind: instancePlanUnrecognized},
		{name: "an object", raw: `{"name":"ultimate"}`, kind: instancePlanUnrecognized},
		{name: "oversized", raw: `"` + strings.Repeat("u", instancePlanMaxBytes) + `"`, kind: instancePlanUnrecognized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseInstancePlan(json.RawMessage(tc.raw))
			if got.kind != tc.kind || got.tier != tc.tier {
				t.Errorf("parseInstancePlan(%s) = %+v, want kind %d tier %v", tc.raw, got, tc.kind, tc.tier)
			}
		})
	}
}

// planInstance answers the tier probe's questions with a version answer the
// test chooses, and counts what was asked. license is what /license answers
// (403 when zero, the plan licensePlan with 200), namespaces the list
// /namespaces answers (403 when nil), and a request to the GraphQL endpoint
// answers no group. refuseVersion answers /version with GitLab's refusal of
// the fine-grained Metadata: Read permission, and versionStatus, when set,
// answers it with that status and versionStatusBody. topGroups and allGroups
// are the page the membership query answers with topLevelOnly and without it,
// no group when empty; a request without it is also counted under
// allGroupsDocument.
type planInstance struct {
	versionBody       string
	refuseVersion     bool
	versionStatus     int
	versionStatusBody string
	license           int
	licensePlan       string
	namespaces        []map[string]any
	topGroups         string
	allGroups         string

	mu    sync.Mutex
	asked map[string]int
}

// serve answers one request. It runs on the httptest server's goroutine, so it
// reports with Errorf and returns rather than aborting the test.
func (p *planInstance) serve(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.asked[r.URL.Path]++
		p.mu.Unlock()
		switch r.URL.Path {
		case versionAPIPath:
			if p.refuseVersion {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(metadataRefusalBody))
				return
			}
			if p.versionStatus != 0 {
				w.WriteHeader(p.versionStatus)
				_, _ = w.Write([]byte(p.versionStatusBody))
				return
			}
			respondJSON(w, p.versionBody)
		case "/api/v4/license":
			if p.license != http.StatusOK {
				w.WriteHeader(max(p.license, http.StatusForbidden))
				return
			}
			respondJSON(w, `{"plan":"`+p.licensePlan+`"}`)
		case "/api/v4/namespaces":
			if p.namespaces == nil {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			body, err := json.Marshal(p.namespaces)
			if err != nil {
				t.Errorf("encoding the namespaces fixture: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			respondJSON(w, string(body))
		case "/api/graphql":
			var request struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decoding the membership request: %v", err)
			}
			if err := graphqlschema.Validate(request.Query, request.Variables); err != nil {
				t.Errorf("GitLab would refuse the membership request: %v", err)
			}
			body := p.allGroups
			if strings.Contains(request.Query, "topLevelOnly: true") {
				body = p.topGroups
			} else {
				p.mu.Lock()
				p.asked[allGroupsDocument]++
				p.mu.Unlock()
			}
			respondJSON(w, cmp.Or(body, noGroupsPage))
		default:
			t.Errorf("the tier probe asked %s %s, which it has no reason to", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}
}

// noGroupsPage is a membership page with no group on it, and allGroupsDocument
// the key a membership request without topLevelOnly is counted under.
const (
	noGroupsPage      = `{"data":{"groups":{"nodes":[],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`
	allGroupsDocument = "membership without topLevelOnly"
)

// groupPage is a membership page holding one group with the two features'
// answers.
func groupPage(path string, premium, ultimate bool) string {
	return fmt.Sprintf(`{"data":{"groups":{"nodes":[{"fullPath":%q,"premium":{"available":%t},"ultimate":{"available":%t}}],`+
		`"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`, path, premium, ultimate)
}

// answeredMembership answers r with a membership page holding no group when it
// is the membership query's POST, and reports whether it did, for a stand-in
// whose every other endpoint is a GET: an enterprise build from 18.11 is asked
// the membership step from issue 1491 on.
func answeredMembership(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost || r.URL.Path != "/api/graphql" {
		return false
	}
	respondJSON(w, noGroupsPage)
	return true
}

// client starts p and returns a client of it that has read nothing yet.
func (p *planInstance) client(t *testing.T) *Client {
	t.Helper()
	p.asked = map[string]int{}
	srv := httptest.NewServer(p.serve(t))
	t.Cleanup(srv.Close)
	return newReleaseClient(t, srv.URL)
}

// count is how many times path was asked.
func (p *planInstance) count(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asked[path]
}

// TestVersionDirect_KeepsThePlanItRead checks that the version read keeps the
// plan beside the version, in each of the three states detection tells apart,
// and that a later read replaces what an earlier one kept.
func TestVersionDirect_KeepsThePlanItRead(t *testing.T) {
	cases := []struct {
		name string
		body string
		kind instancePlanKind
	}{
		{name: "a plan", body: `{"version":"19.6.1-ee","enterprise":true,"plan":"premium"}`, kind: instancePlanRecognized},
		{name: "null", body: `{"version":"19.6.0-pre","enterprise":true,"plan":null}`, kind: instancePlanPerNamespace},
		{name: "no key", body: `{"version":"19.5.5-ee","enterprise":true}`, kind: instancePlanAbsent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instance := newReleaseInstance(t, http.StatusOK, `{"version":"19.6.1-ee","plan":"ultimate"}`)
			client := newReleaseClient(t, instance.url)
			if _, err := client.Initialize(t.Context()); err != nil {
				t.Fatalf("Initialize() error = %v", err)
			}
			instance.answer(http.StatusOK, tc.body)
			if _, err := client.versionDirect(t.Context()); err != nil {
				t.Fatalf("versionDirect() error = %v", err)
			}
			got := client.plan.Load()
			if got == nil || got.kind != tc.kind {
				t.Errorf("kept plan = %+v, want kind %d", got, tc.kind)
			}
		})
	}
}

// TestDetectTier_InstancePlan_SettlesTheTierWithoutAnotherStep is the case
// issue 1491 is for: on 19.6 or later a recognized plan is the instance's own
// answer, for any token that may read the metadata, so neither the license,
// the namespaces nor the membership step is asked. free is an answer too: an
// unlicensed enterprise build resolves Free without the warning it used to
// give at every start.
func TestDetectTier_InstancePlan_SettlesTheTierWithoutAnotherStep(t *testing.T) {
	standInForGitLabDotCom(t)
	cases := []struct {
		plan string
		want edition.Tier
	}{
		{plan: "free", want: edition.Free},
		{plan: "starter", want: edition.Premium},
		{plan: "premium", want: edition.Premium},
		{plan: "ultimate", want: edition.Ultimate},
	}
	for _, tc := range cases {
		t.Run(tc.plan, func(t *testing.T) {
			instance := &planInstance{
				versionBody: `{"version":"19.6.1-ee","enterprise":true,"plan":"` + tc.plan + `"}`,
				license:     http.StatusOK, licensePlan: "ultimate",
				namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "ultimate"}},
			}
			client := instance.client(t)
			logged := captureClientLog(t)

			got := client.DetectTier(t.Context(), t.Context())
			if got.Tier != tc.want || !got.Answered || got.Provisional {
				t.Errorf("DetectTier() = %+v, want %v answered and not provisional", got, tc.want)
			}
			if client.Tier() != tc.want {
				t.Errorf("client tier = %v, want %v", client.Tier(), tc.want)
			}
			for _, path := range []string{"/api/v4/license", "/api/v4/namespaces", "/api/graphql"} {
				t.Run(path, func(t *testing.T) {
					if n := instance.count(path); n != 0 {
						t.Errorf("%s asked %d times after the instance plan answered, want none", path, n)
					}
				})
			}
			if n := instance.count(versionAPIPath); n != 1 {
				t.Errorf("/version asked %d times, want once", n)
			}
			if strings.Contains(logged.String(), `"level":"WARN"`) {
				t.Errorf("a tier the instance plan answered warned:\n%s", logged)
			}
			if !strings.Contains(logged.String(), "detected GitLab tier from the instance plan") {
				t.Errorf("the answer was not logged:\n%s", logged)
			}
		})
	}
}

// TestDetectTier_InstancePlanNull_SkipsTheLicense holds what a null plan says:
// subscriptions are held per namespace, as on GitLab.com, where /license
// answers nobody and describes no caller's entitlement. The license is not
// asked, and the namespace plans and the membership step answer as they did.
func TestDetectTier_InstancePlanNull_SkipsTheLicense(t *testing.T) {
	t.Run("the namespace plans answer", func(t *testing.T) {
		instance := &planInstance{
			versionBody: `{"version":"19.6.1-ee","enterprise":true,"plan":null}`,
			license:     http.StatusOK, licensePlan: "ultimate",
			namespaces: []map[string]any{{"full_path": "acme", "kind": "group", "plan": "premium"}},
		}
		client := instance.client(t)
		if got := client.DetectTier(t.Context(), t.Context()); got.Tier != edition.Premium || !got.Answered {
			t.Errorf("DetectTier() = %+v, want Premium from the namespace plan", got)
		}
		if n := instance.count("/api/v4/license"); n != 0 {
			t.Errorf("/license asked %d times under a null plan, want none", n)
		}
	})

	t.Run("a GitLab.com member", func(t *testing.T) {
		server := &fakeGitLabDotCom{
			versionBody: `{"version":"19.6.0-pre","enterprise":true,"plan":null}`,
			license:     http.StatusOK, licensePlan: "ultimate",
			namespaces: freeNamespaces,
			pages:      []fakeMembershipPage{{groups: []*fakeGroup{{path: "gitlab-community", premium: true, ultimate: true}}}},
		}
		got, client := server.detect(t, false)
		if got.Tier != edition.Ultimate || client.Tier() != edition.Ultimate {
			t.Errorf("DetectTier() = %+v, client %v; want Ultimate from the top-level group", got, client.Tier())
		}
		if n := server.licenseRequests(); n != 0 {
			t.Errorf("/license asked %d times under a null plan, want none", n)
		}
	})
}

// TestDetectTier_NoInstancePlanAnswer_RunsTheCascade holds every case where
// the version answer settles nothing: GitLab.com's prerelease before the
// field was deployed, a release before 19.6 (where the key cannot appear, and
// is not read if it does), a value outside the four plans, which is logged at
// debug and never read as Free, and a version the token was refused. Each
// asks the license first, as before, which answers here.
func TestDetectTier_NoInstancePlanAnswer_RunsTheCascade(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		refuseVersion bool
		debug         string
	}{
		{name: "19.6.0-pre without the key", body: `{"version":"19.6.0-pre","enterprise":true}`},
		{name: "a release before 19.6", body: `{"version":"19.5.5-ee","enterprise":true,"plan":"free"}`},
		{name: "an unrecognized plan", body: `{"version":"19.6.1-ee","enterprise":true,"plan":"gold"}`, debug: "the instance reports a plan this server does not recognize"},
		{name: "a refused version", refuseVersion: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &planInstance{versionBody: tc.body, refuseVersion: tc.refuseVersion, license: http.StatusOK, licensePlan: "premium"}
			client := instance.client(t)
			logged := captureClientLog(t)

			if got := client.DetectTier(t.Context(), t.Context()); got.Tier != edition.Premium || !got.Answered {
				t.Errorf("DetectTier() = %+v, want Premium from the license", got)
			}
			if n := instance.count("/api/v4/license"); n != 1 {
				t.Errorf("/license asked %d times, want once", n)
			}
			if tc.debug != "" && !strings.Contains(logged.String(), tc.debug) {
				t.Errorf("the unrecognized plan was not logged at debug:\n%s", logged)
			}
		})
	}
}

// TestDetectTier_UnansweredVersionRead_LeavesTheTierProvisional holds the plan
// step to the rule every other step keeps (issue 1782): a version read GitLab
// did not answer could have carried a plan that raises the tier, so a
// non-administrator refused the license and shown "default" namespace plans is
// served Free provisionally, naming the step, and asked again rather than
// settled on Free. A version read GitLab answered with a status that says
// something, a 404 or GitLab's refusal of a scope here, is no plan and leaves
// the cascade's answer final, as a release before 19.6 does.
func TestDetectTier_UnansweredVersionRead_LeavesTheTierProvisional(t *testing.T) {
	defaultPlans := []map[string]any{{"full_path": "acme", "kind": "group", "plan": "default"}}
	cases := []struct {
		name            string
		versionStatus   int
		statusBody      string
		versionBody     string
		wantProvisional bool
	}{
		{name: "a 502", versionStatus: http.StatusBadGateway, wantProvisional: true},
		{name: "a 429", versionStatus: http.StatusTooManyRequests, wantProvisional: true},
		{name: "a body that does not decode", versionBody: `{"version":`, wantProvisional: true},
		{name: "a 404", versionStatus: http.StatusNotFound},
		{name: "a scope refusal", versionStatus: http.StatusForbidden, statusBody: insufficientScopeBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instance := &planInstance{
				versionStatus: tc.versionStatus, versionStatusBody: tc.statusBody, versionBody: tc.versionBody,
				license: http.StatusForbidden, namespaces: defaultPlans,
			}
			client := instance.client(t)

			got := client.DetectTier(t.Context(), t.Context())
			if got.Tier != edition.Free || got.Provisional != tc.wantProvisional {
				t.Errorf("DetectTier() = %+v, want Free with Provisional %v", got, tc.wantProvisional)
			}
			wantUnresolved := []string(nil)
			if tc.wantProvisional {
				wantUnresolved = []string{tierStepPlan}
			}
			if !slices.Equal(got.Unresolved, wantUnresolved) {
				t.Errorf("Unresolved = %q, want %q", got.Unresolved, wantUnresolved)
			}
			// A provisional answer asks no edition, so the plan step's read is
			// the only one; the Free fallback's warning asks the edition again.
			if n := instance.count(versionAPIPath); tc.wantProvisional && n != 1 {
				t.Errorf("/version asked %d times, want once", n)
			}
		})
	}
}

// TestTierFromInstancePlan_AVersionWithoutAKeptPlan_AnswersNothing covers a
// client that holds a version and no plan, which only a read that stored the
// version without the plan could leave: it is no answer, not a panic.
func TestTierFromInstancePlan_AVersionWithoutAKeptPlan_AnswersNothing(t *testing.T) {
	client := newReleaseClient(t, "http://127.0.0.1:1")
	version := "19.6.1-ee"
	client.version.Store(&version)
	if got := client.tierFromInstancePlan(t.Context()); got.kind != instancePlanAbsent {
		t.Errorf("tierFromInstancePlan() = %+v, want no answer", got)
	}
}

// TestDetectTier_AfterInitialize_ReadsThePlanWithoutAskingAgain is a stdio
// start's cost: Initialize read the version, and the plan with it, so the tier
// is detected without a second request to /version.
func TestDetectTier_AfterInitialize_ReadsThePlanWithoutAskingAgain(t *testing.T) {
	instance := &planInstance{versionBody: `{"version":"19.6.1-ee","enterprise":true,"plan":"ultimate"}`}
	client := instance.client(t)
	if _, err := client.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if got := client.DetectTier(t.Context(), t.Context()); got.Tier != edition.Ultimate {
		t.Errorf("DetectTier() = %+v, want Ultimate", got)
	}
	if n := instance.count(versionAPIPath); n != 1 {
		t.Errorf("/version asked %d times across Initialize and DetectTier, want once", n)
	}
}
