// dependency_firewall_test.go contains unit tests for the Dependency Firewall
// package evaluation handler. Tests use httptest to mock the GitLab API and
// cover the success, validation, and error paths.
package dependencyfirewall

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	gitlab "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
)

// evaluatePath is the endpoint the request is asserted against. The project is
// URL-encoded on the wire; net/http hands the handler the decoded path.
const evaluatePath = "/api/v4/projects/group/project/dependency_firewall/evaluate"

// TestEvaluatePackage_BlockedOutcome verifies that a blocked verdict is
// decoded with its reason, and that the request GitLab receives carries the
// method, path and JSON body the API documents.
func TestEvaluatePackage_BlockedOutcome(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		testutil.AssertRequestPath(t, r, evaluatePath)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
			return
		}
		var sent map[string]any
		if err = json.Unmarshal(body, &sent); err != nil {
			t.Errorf("body is not JSON: %v (%s)", err, body)
			testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
			return
		}
		// Checked field by field rather than in a loop: this runs on the
		// server's goroutine, where a subtest cannot be opened.
		if sent["ecosystem"] != "npm" {
			t.Errorf("body[ecosystem] = %v, want npm", sent["ecosystem"])
		}
		if sent["name"] != "lodash" {
			t.Errorf("body[name] = %v, want lodash", sent["name"])
		}
		if sent["version"] != "4.17.15" {
			t.Errorf("body[version] = %v, want 4.17.15", sent["version"])
		}
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"blocked","reason":"Package 'lodash' violates 'deny-mit' policy"}`)
	}))

	out, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
		ProjectID: "group/project",
		Ecosystem: "npm",
		Name:      "lodash",
		Version:   "4.17.15",
	})
	if err != nil {
		t.Fatalf("EvaluatePackage() error = %v", err)
	}
	if out.Outcome != outcomeBlocked {
		t.Errorf("Outcome = %q, want %q", out.Outcome, outcomeBlocked)
	}
	if out.Reason == nil {
		t.Fatal("Reason = nil, want the policy that blocked the package")
	}
	if !strings.Contains(*out.Reason, "deny-mit") {
		t.Errorf("Reason = %q, want the policy name", *out.Reason)
	}
}

// TestEvaluatePackage_AllowedKeepsNullReason verifies that an allowed outcome
// keeps Reason as a nil pointer. GitLab answers null there, and collapsing it
// to an empty string would erase the difference between "no reason given" and
// "the reason was empty".
func TestEvaluatePackage_AllowedKeepsNullReason(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	}))

	out, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
		ProjectID: "group/project",
		Ecosystem: "pypi",
		Name:      "flask-login",
		Version:   "0.6.3",
	})
	if err != nil {
		t.Fatalf("EvaluatePackage() error = %v", err)
	}
	if out.Outcome != outcomeAllowed {
		t.Errorf("Outcome = %q, want %q", out.Outcome, outcomeAllowed)
	}
	if out.Reason != nil {
		t.Errorf("Reason = %q, want nil", *out.Reason)
	}
}

// TestEvaluatePackage_NormalizesEcosystemCase verifies that an ecosystem given
// in the wrong case is accepted and sent lowercase, which is the only spelling
// GitLab matches.
func TestEvaluatePackage_NormalizesEcosystemCase(t *testing.T) {
	var sentEcosystem string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sent map[string]any
		if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if value, ok := sent["ecosystem"].(string); ok {
			sentEcosystem = value
		}
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	}))

	if _, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
		ProjectID: "group/project",
		Ecosystem: "  NuGet  ",
		Name:      "Newtonsoft.Json",
		Version:   "13.0.3",
	}); err != nil {
		t.Fatalf("EvaluatePackage() error = %v", err)
	}
	if sentEcosystem != "nuget" {
		t.Errorf("ecosystem sent = %q, want %q", sentEcosystem, "nuget")
	}
}

// TestEvaluatePackage_EveryEcosystemIsAccepted verifies that all eleven
// documented ecosystems pass validation. client-go v2.61.0 widened the enum
// from four values, and a stale copy of the list here would silently refuse
// the seven that were added.
func TestEvaluatePackage_EveryEcosystemIsAccepted(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	}))

	for _, ecosystem := range Ecosystems {
		t.Run(ecosystem, func(t *testing.T) {
			if _, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
				ProjectID: "group/project",
				Ecosystem: ecosystem,
				Name:      "example",
				Version:   "1.0.0",
			}); err != nil {
				t.Errorf("EvaluatePackage() error = %v", err)
			}
		})
	}
}

// TestEcosystems_MatchClientGoConstants verifies the exposed enum is exactly
// the set client-go declares, so a future widening upstream cannot leave this
// server offering a subset.
func TestEcosystems_MatchClientGoConstants(t *testing.T) {
	want := map[string]bool{
		string(gitlab.DependencyFirewallEcosystemCargo):    true,
		string(gitlab.DependencyFirewallEcosystemComposer): true,
		string(gitlab.DependencyFirewallEcosystemConan):    true,
		string(gitlab.DependencyFirewallEcosystemGem):      true,
		string(gitlab.DependencyFirewallEcosystemGolang):   true,
		string(gitlab.DependencyFirewallEcosystemMaven):    true,
		string(gitlab.DependencyFirewallEcosystemNPM):      true,
		string(gitlab.DependencyFirewallEcosystemNuGet):    true,
		string(gitlab.DependencyFirewallEcosystemPub):      true,
		string(gitlab.DependencyFirewallEcosystemPyPI):     true,
		string(gitlab.DependencyFirewallEcosystemSwift):    true,
	}
	if len(Ecosystems) != len(want) {
		t.Fatalf("len(Ecosystems) = %d, want %d", len(Ecosystems), len(want))
	}
	for _, ecosystem := range Ecosystems {
		if !want[ecosystem] {
			t.Errorf("Ecosystems has %q, which client-go does not declare", ecosystem)
		}
	}
}

// TestEvaluatePackage_ValidationErrors verifies that every required field and
// documented bound is checked before the request leaves, and that the message
// names the offending field.
func TestEvaluatePackage_ValidationErrors(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler called, want validation to fail before the request")
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	}))

	tests := []struct {
		name    string
		input   EvaluatePackageInput
		wantSub string
	}{
		{
			name:    "missing project_id",
			input:   EvaluatePackageInput{Ecosystem: "npm", Name: "lodash", Version: "1"},
			wantSub: "project_id is required",
		},
		{
			name:    "missing ecosystem",
			input:   EvaluatePackageInput{ProjectID: "1", Name: "lodash", Version: "1"},
			wantSub: "ecosystem is required",
		},
		{
			name:    "unknown ecosystem",
			input:   EvaluatePackageInput{ProjectID: "1", Ecosystem: "deb", Name: "lodash", Version: "1"},
			wantSub: "invalid ecosystem",
		},
		{
			name:    "missing name",
			input:   EvaluatePackageInput{ProjectID: "1", Ecosystem: "npm", Version: "1"},
			wantSub: "name is required",
		},
		{
			name:    "missing version",
			input:   EvaluatePackageInput{ProjectID: "1", Ecosystem: "npm", Name: "lodash"},
			wantSub: "version is required",
		},
		{
			name: "name too long",
			input: EvaluatePackageInput{
				ProjectID: "1", Ecosystem: "npm",
				Name:    strings.Repeat("a", maxCoordinateLength+1),
				Version: "1",
			},
			wantSub: "name must be at most 255 characters",
		},
		{
			name: "version too long",
			input: EvaluatePackageInput{
				ProjectID: "1", Ecosystem: "npm", Name: "lodash",
				Version: strings.Repeat("9", maxCoordinateLength+1),
			},
			wantSub: "version must be at most 255 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EvaluatePackage(t.Context(), client, tt.input)
			if err == nil {
				t.Fatalf("EvaluatePackage() error = nil, want %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("EvaluatePackage() error = %q, want it to contain %q", err, tt.wantSub)
			}
		})
	}
}

// TestEvaluatePackage_ForbiddenCarriesTierHint verifies that a 403 explains the
// licensing requirement, which is the likeliest cause of one here.
func TestEvaluatePackage_ForbiddenCarriesTierHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
		ProjectID: "group/project", Ecosystem: "npm", Name: "lodash", Version: "1",
	})
	if err == nil {
		t.Fatal("EvaluatePackage() error = nil, want a forbidden error")
	}
	if !strings.Contains(err.Error(), "Premium or Ultimate") {
		t.Errorf("EvaluatePackage() error = %q, want the tier hint", err)
	}
}

// TestEvaluatePackage_NotFoundIsReturnedAsError verifies the handler itself
// still fails on a 404. Turning that into guidance is the action route's job,
// and doing it here as well would leave the typed handler unable to report a
// missing project at all.
func TestEvaluatePackage_NotFoundIsReturnedAsError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Project Not Found"}`)
	}))

	_, err := EvaluatePackage(t.Context(), client, EvaluatePackageInput{
		ProjectID: "group/project", Ecosystem: "npm", Name: "lodash", Version: "1",
	})
	if err == nil {
		t.Fatal("EvaluatePackage() error = nil, want a not-found error")
	}
}

// TestEvaluatePackage_CanceledContext verifies the handler gives up before
// issuing a request when the caller has already gone away.
func TestEvaluatePackage_CanceledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("handler called, want the canceled context to short-circuit")
		testutil.RespondJSON(w, http.StatusOK, `{"outcome":"allowed","reason":null}`)
	}))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := EvaluatePackage(ctx, client, EvaluatePackageInput{
		ProjectID: "group/project", Ecosystem: "npm", Name: "lodash", Version: "1",
	}); err == nil {
		t.Fatal("EvaluatePackage() error = nil, want the canceled context")
	}
}
