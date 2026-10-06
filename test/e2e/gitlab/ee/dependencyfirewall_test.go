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
// verdict for a project the firewall is turned on for, and a refusal saying
// where it is turned on for any other, which on a Docker instance is every
// project, since no API turns it on there.
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

	// The two halves run in order on this test rather than as subtests: every
	// harness helper fails through the Env's own T, and a subtest failing the
	// test above it panics the run instead of reporting, which is how the flag
	// on half's first refusal on GitLab 19.4 surfaced.
	before := fixture.ReadFeature(e, dependencyfirewall.FeatureFlag)
	if before.On() {
		e.T.Logf("the instance already holds %s on, so the refusal half has nothing to observe",
			dependencyfirewall.FeatureFlag)
	} else {
		text := harness.Refused(s, actionDependencyFirewallEvaluate, params, harness.FailureNotFound)
		assertMentions(e, "the Dependency Firewall refusal", text,
			dependencyfirewall.FeatureFlag, "Premium", "project.get")
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

	out, err := harness.Try[dependencyfirewall.EvaluatePackageOutput](s, actionDependencyFirewallEvaluate, params)
	if err != nil {
		assertFirewallOffRefusal(e, err)
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
}

// assertFirewallOffRefusal holds an evaluation refused with the flag on to
// the one answer that is a fact about the instance rather than about the
// action: the firewall is not turned on for the project.
//
// With the flag on, GitLab 19.4 evaluates a package only for a project the
// firewall is turned on for, and on a self-managed instance that is an
// instance setting only the Admin area writes, so a Docker instance answers
// 422 dependency_firewall_not_enforced to every project and a verdict is out
// of this suite's reach. What is held is that the refusal says where the
// firewall is turned on, since the generic reading of a 422 is invalid input
// and sends a caller to change a package coordinate that was never wrong. A
// release that defines the flag and still answers not-found for such a
// project is accepted too, being the same fact in the shape older releases
// gave it. Everything else is a finding, and accepting it here is how a
// transport failure, a credential refused or a decoder that stopped matching
// would pass as a refusal nobody read.
func assertFirewallOffRefusal(e *harness.Env, err error) {
	e.T.Helper()
	text := err.Error()
	switch {
	case mentionsAny(text, "422"):
		assertMentions(e, "the refusal of a project the firewall is off for", text,
			"not turned on for this project", "Admin > Settings > Security and compliance", "not evaluated")
		e.T.Logf("the evaluation answered that the firewall is off for the project, which no API turns on: %s", firstLine(text))
	case mentionsAny(text, dependencyfirewall.FeatureFlag, "not found"):
		e.T.Logf("the evaluation answered the not-found card with %s on, so this release serves no firewall for a "+
			"project without one: %v", dependencyfirewall.FeatureFlag, err)
	default:
		e.T.Fatalf("the evaluation failed for a reason that is not the firewall being off for the project: %v", err)
	}
}

// TestDependencyFirewall_Evaluate_IsWithheldFromAReadAPIToken holds the
// evaluation to what GitLab answers a token carrying read_api and not api: it
// is a read the catalog classifies as such, sent as a POST that GitLab grants
// no scope to but api, so a read_api session is not served it and the
// dynamic surface says api is the scope it lacks (ADR-0026). The session pins
// the Premium tier, since a non-administrator's token cannot read the license
// and would otherwise be served the Free catalog, where the action is absent
// for another reason.
func TestDependencyFirewall_Evaluate_IsWithheldFromAReadAPIToken(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin))

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Token {
		return fixture.NewToken(e, fixture.NewUser(e, "depfw-read"), "read_api")
	}, func(e *harness.Env, surface harness.Surface, token fixture.Token) {
		s := e.Session(harness.ServerConfig{Surface: surface, Token: token.Value, Tier: harness.TierPremium})
		if s.Serves(actionDependencyFirewallEvaluate) {
			e.T.Fatalf("a %s session on a read_api token serves %s, which GitLab answers only from api", surface, actionDependencyFirewallEvaluate)
		}
		// The arguments never reach GitLab: the refusal comes before any
		// handler runs, so the project is a placeholder.
		declined := harness.Withheld(s, actionDependencyFirewallEvaluate, map[string]any{
			"project_id": "1", "ecosystem": dependencyFirewallEcosystem, "name": dependencyFirewallPackage, "version": dependencyFirewallVersion,
		})
		if surface == harness.SurfaceDynamic && !strings.Contains(declined, "GitLab requires the api scope") {
			e.T.Errorf("%s was declined without naming the api scope the credential lacks: %q", actionDependencyFirewallEvaluate, declined)
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
