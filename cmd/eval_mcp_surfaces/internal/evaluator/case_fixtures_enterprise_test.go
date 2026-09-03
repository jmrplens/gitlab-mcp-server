// case_fixtures_enterprise_test.go drives the Enterprise-only case fixtures
// against a fake GitLab: the push-rule projects and the group service accounts
// that Premium and Ultimate evaluation cases depend on.

package evaluator

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestEnterpriseCaseFixtures_ProvisionEveryDeclaredOutput verifies each
// Enterprise fixture variant provisions its resources against a fake GitLab,
// returns every declared output, passes its Validate hook, and is tagged as
// requiring the Enterprise runtime so it is never selected on CE.
func TestEnterpriseCaseFixtures_ProvisionEveryDeclaredOutput(t *testing.T) {
	client := newDestructiveFixtureClient(t)
	env := FixtureContext{Client: client, RuntimeEdition: EvalCaseEdition(editionEnterprise), ModelName: "model", RunIndex: 1, RunSuffix: "suffix", CaseID: "MT-ENT"}
	fixtures := []CaseFixtureSpec{
		EnterprisePushRuleProjectFixture(false),
		EnterprisePushRuleProjectFixture(true),
		EnterpriseGroupServiceAccountFixture(false),
		EnterpriseGroupServiceAccountFixture(true),
		ProjectServiceAccountFixture,
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			if string(fixture.RequiredRuntime) != editionEnterprise {
				t.Fatalf("%s RequiredRuntime = %q, want enterprise", fixture.Name, fixture.RequiredRuntime)
			}
			output, err := fixture.Ensure(t.Context(), env)
			if err != nil {
				t.Fatalf("%s Ensure() error = %v", fixture.Name, err)
			}
			if validateErr := validateFixtureOutput(fixture, output); validateErr != nil {
				t.Fatalf("%s outputs = %v, error = %v", fixture.Name, output, validateErr)
			}
			if validateErr := fixture.Validate(t.Context(), env, output); validateErr != nil {
				t.Fatalf("%s Validate() error = %v", fixture.Name, validateErr)
			}
		})
	}
}

// TestEnterprisePushRuleProjectFixture_SeedsOrClearsThePushRule verifies the
// seeded variant adds a commit-message rule to the disposable project while
// the empty variant deletes any rule the project came with, so the case can
// exercise create and update independently.
func TestEnterprisePushRuleProjectFixture_SeedsOrClearsThePushRule(t *testing.T) {
	cases := []struct {
		name       string
		seedRule   bool
		wantMethod string
	}{
		{name: "seeded adds a rule", seedRule: true, wantMethod: "POST"},
		{name: "empty deletes any rule", seedRule: false, wantMethod: "DELETE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pushRuleCalls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := r.URL.EscapedPath()
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(path, "/push_rule") {
					pushRuleCalls = append(pushRuleCalls, r.Method)
				}
				fmt.Fprint(w, gitLabFixtureObject)
			}))
			defer server.Close()
			env := FixtureContext{Client: newFixtureTestClient(t, server.URL), ModelName: "model", RunIndex: 1, RunSuffix: "suffix"}

			output, err := ensureEnterprisePushRuleProjectFixture(t.Context(), env, tc.seedRule)
			if err != nil {
				t.Fatalf("ensureEnterprisePushRuleProjectFixture() error = %v", err)
			}
			if output["default_branch"] != "main" || output["project_id"] == "" {
				t.Fatalf("output = %v, want the disposable project identity", output)
			}
			if strings.Join(pushRuleCalls, ",") != tc.wantMethod {
				t.Fatalf("push rule calls = %v, want a single %s", pushRuleCalls, tc.wantMethod)
			}
		})
	}
}

// TestEnterpriseFixtures_WithoutClient_ReturnError verifies both Enterprise
// fixture families refuse to provision without a GitLab client.
func TestEnterpriseFixtures_WithoutClient_ReturnError(t *testing.T) {
	cases := []struct {
		name   string
		ensure func() error
		want   string
	}{
		{name: "push rule project", ensure: func() error {
			_, err := ensureEnterprisePushRuleProjectFixture(t.Context(), FixtureContext{}, false)
			return err
		}, want: "enterprise push rule fixture requires GitLab client"},
		{name: "group service account", ensure: func() error {
			_, err := ensureEnterpriseGroupServiceAccountFixture(t.Context(), FixtureContext{}, false)
			return err
		}, want: "enterprise group service account fixture requires GitLab client"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.ensure(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ensure error = %v, want %q", err, tc.want)
			}
		})
	}
}

// TestEnterpriseGroupServiceAccountFixture_PATVariantAddsTokenID verifies the
// PAT variant provisions a personal access token alongside the service account
// and the base variant leaves the token identifier out of its output.
func TestEnterpriseGroupServiceAccountFixture_PATVariantAddsTokenID(t *testing.T) {
	cases := []struct {
		name        string
		withPAT     bool
		wantTokenID bool
	}{
		{name: "with pat", withPAT: true, wantTokenID: true},
		{name: "without pat", withPAT: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := FixtureContext{Client: newDestructiveFixtureClient(t), CaseID: "MT-ENT-SA"}
			output, err := ensureEnterpriseGroupServiceAccountFixture(t.Context(), env, tc.withPAT)
			if err != nil {
				t.Fatalf("ensureEnterpriseGroupServiceAccountFixture() error = %v", err)
			}
			if output["group_path"] != liveFixtureGroupPath || output["service_account_id"] == "" {
				t.Fatalf("output = %v, want the group service account identity", output)
			}
			if (output["token_id"] != "") != tc.wantTokenID {
				t.Fatalf("output token_id = %q, want present = %t", output["token_id"], tc.wantTokenID)
			}
		})
	}
}

// TestCreateLiveGroupServiceAccount_WithoutClient_ReturnsError verifies the
// group service-account helper refuses to run without a GitLab client.
func TestCreateLiveGroupServiceAccount_WithoutClient_ReturnsError(t *testing.T) {
	if _, _, err := createLiveGroupServiceAccount(t.Context(), nil, "MT-1"); err == nil || !strings.Contains(err.Error(), "group service account fixture requires GitLab client") {
		t.Fatalf("createLiveGroupServiceAccount() error = %v, want missing client error", err)
	}
}

// TestCreateLiveGroupServiceAccountPAT_AccountFailure_AbortsBeforeToken
// verifies a failed service-account creation is reported with the task ID and
// no token is requested.
func TestCreateLiveGroupServiceAccountPAT_AccountFailure_AbortsBeforeToken(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"message":"403 Forbidden"}`)
	}))
	defer server.Close()
	_, _, err := createLiveGroupServiceAccountPAT(t.Context(), newFixtureTestClient(t, server.URL), "MT-1")
	if err == nil || !strings.Contains(err.Error(), "prepare MT-1 fixture group service account") {
		t.Fatalf("createLiveGroupServiceAccountPAT() error = %v, want account failure", err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %v, want only the service account call", paths)
	}
}

// TestValidateEnterpriseCaseFixtureOutput_RequiresAnIdentifier verifies the
// Enterprise validator accepts any of the project, group or service-account
// identifiers and rejects an output carrying none.
func TestValidateEnterpriseCaseFixtureOutput_RequiresAnIdentifier(t *testing.T) {
	cases := []struct {
		name    string
		output  FixtureOutput
		wantErr bool
	}{
		{name: "project", output: FixtureOutput{"project_id": "1"}},
		{name: "group", output: FixtureOutput{"group_id": "my-org"}},
		{name: "service account", output: FixtureOutput{"service_account_id": "9"}},
		{name: "empty", output: FixtureOutput{}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEnterpriseCaseFixtureOutput(t.Context(), FixtureContext{}, tc.output)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateEnterpriseCaseFixtureOutput(%v) error = %v, want error = %t", tc.output, err, tc.wantErr)
			}
		})
	}
}
