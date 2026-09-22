//go:build e2e

// dependencyfirewall_test.go covers the one action of the Dependency Firewall
// domain, which is served behind an instance feature flag and was reached by
// nothing.
//
// The flag is the whole reason this file is shaped the way it is. While
// dependency_firewall_phase1 is off, the endpoint answers 404 for every
// project on the instance, so the action has two answers worth asserting and
// a test that saw only one of them would be covering half the handler: with
// the flag off it is an informational card that names the flag, written for
// exactly this case because a bare not-found reads as "the project is wrong"
// and sends a model round the same retry forever; with the flag on it is a
// verdict.
//
// Both halves run in one test, in that order, because the second changes the
// instance and the first is the only chance to see the default state. The
// write is instance-global, so the test declares the lock and an
// administrator, and the fixture puts the flag back where it found it.

package ee

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dependencyfirewall"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The coordinate every call here evaluates: a real npm package at a real
// version, since GitLab looks the pair up in its package metadata and a
// coordinate nothing knows about tells a reader nothing about which of the
// two answers it got.
const (
	dependencyFirewallEcosystem = "npm"
	dependencyFirewallPackage   = "lodash"
	dependencyFirewallVersion   = "4.17.15"
)

// dependencyFirewallOutcomes is what GitLab documents the verdict can be.
var dependencyFirewallOutcomes = []string{"allowed", "warned", "blocked"}

// TestDependencyFirewall_Evaluate_RefusesWithTheFlagOffAndAnswersWithItOn
// drives the evaluate action on both sides of the feature flag.
//
// One surface rather than three: the action is one POST reached through the
// same catalog on every surface, the surfaces differ in how the call is named,
// and the flag flip is an instance-wide write this test would then be making
// three times.
func TestDependencyFirewall_Evaluate_RefusesWithTheFlagOffAndAnswersWithItOn(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal))
	s := e.On(harness.SurfaceDynamic)
	project := fixture.NewProject(e, fixture.WithNamePrefix("depfw"))

	params := map[string]any{
		"project_id": project.IDParam(),
		"ecosystem":  dependencyFirewallEcosystem,
		"name":       dependencyFirewallPackage,
		"version":    dependencyFirewallVersion,
	}

	before := fixture.ReadFeature(e, dependencyfirewall.FeatureFlag)
	if before.On() {
		e.T.Logf("the instance already holds %s on, so the refusal half has nothing to observe",
			dependencyfirewall.FeatureFlag)
	} else {
		t.Run("with the flag off the refusal names the flag", func(t *testing.T) {
			text := harness.Refused(s, actionDependencyFirewallEvaluate, params, harness.FailureNotFound)
			assertMentions(e, "the Dependency Firewall refusal", text,
				dependencyfirewall.FeatureFlag, "Premium", "project.get")
		})
	}

	if !fixture.FeatureDefined(e, dependencyfirewall.FeatureFlag) {
		// A flag this instance defines nowhere belongs to a release it
		// predates, and no value set for it would make the endpoint appear.
		// The refusal above is then the whole of what this GitLab can be
		// asked, and saying so beats turning the flag on and asserting the
		// same 404 twice.
		e.T.Logf("this GitLab does not define %s, so the endpoint is not in this release and only the refusal ran",
			dependencyfirewall.FeatureFlag)
		return
	}

	fixture.PinFeature(e, dependencyfirewall.FeatureFlag, true)

	t.Run("with the flag on the evaluation answers a verdict", func(t *testing.T) {
		out, err := harness.Try[dependencyfirewall.EvaluatePackageOutput](s, actionDependencyFirewallEvaluate, params)
		if err != nil {
			// One refusal is a fact about the instance rather than about the
			// action, and only one: the endpoint is an experiment, so a
			// release that defines the flag and still answers not-found for a
			// project with no firewall configured says nothing about this
			// handler. Everything else is a finding, and accepting it here is
			// how a transport failure, a credential refused or a decoder that
			// stopped matching would pass as a verdict nobody read.
			if !mentionsAny(err.Error(), dependencyfirewall.FeatureFlag, "not found") {
				e.T.Fatalf("the evaluation failed for a reason that is not the documented not-found: %v", err)
			}
			e.T.Logf("the evaluation answered the not-found card with %s on, so this release serves no firewall for a "+
				"project without one: %v", dependencyfirewall.FeatureFlag, err)
			return
		}
		if !containsOutcome(out.Outcome) {
			e.T.Errorf("the verdict is %q, want one of %s", out.Outcome, strings.Join(dependencyFirewallOutcomes, ", "))
		}
		// The reason is GitLab's account of a policy that matched, so it is
		// null exactly when nothing did. A blocked package with no reason
		// leaves a caller with a refusal it cannot act on.
		switch {
		case out.Outcome == "allowed" && out.Reason != nil && *out.Reason != "":
			e.T.Errorf("an allowed verdict carries the reason %q, and no policy matched it", *out.Reason)
		case out.Outcome != "allowed" && (out.Reason == nil || *out.Reason == ""):
			e.T.Errorf("the %s verdict names no policy, so the caller cannot tell what matched", out.Outcome)
		}
	})
}

// containsOutcome reports whether a verdict is one of the documented three.
func containsOutcome(outcome string) bool {
	for _, want := range dependencyFirewallOutcomes {
		if strings.EqualFold(outcome, want) {
			return true
		}
	}
	return false
}
