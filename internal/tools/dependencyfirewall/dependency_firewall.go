package dependencyfirewall

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	gitlab "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FeatureFlag is the GitLab feature flag the Dependency Firewall API is served
// behind. It is named in the not-found hint so a caller on an instance without
// it is told why the endpoint is missing rather than left with a bare 404.
const FeatureFlag = "dependency_firewall_phase1"

// maxCoordinateLength is the documented maximum length of the name and version
// attributes ("Package name, maximum 255 characters"). Checking it here turns a
// server-side 400 into a message that names the offending field.
const maxCoordinateLength = 255

// evaluateOperation is the operation label carried by wrapped errors.
const evaluateOperation = "evaluate package against dependency firewall"

// Ecosystems lists every package ecosystem the Dependency Firewall evaluates,
// in the order the API documentation lists them. It is the source for both the
// input schema enum and the runtime validation, so the two cannot drift.
//
// The values are read off client-go's DependencyFirewallEcosystemValue
// constants, which v2.61.0 widened from the original four (maven, npm, pypi,
// gem) with composer, conan, golang, nuget, cargo, swift and pub.
var Ecosystems = []string{
	string(gitlab.DependencyFirewallEcosystemCargo),
	string(gitlab.DependencyFirewallEcosystemComposer),
	string(gitlab.DependencyFirewallEcosystemConan),
	string(gitlab.DependencyFirewallEcosystemGem),
	string(gitlab.DependencyFirewallEcosystemGolang),
	string(gitlab.DependencyFirewallEcosystemMaven),
	string(gitlab.DependencyFirewallEcosystemNPM),
	string(gitlab.DependencyFirewallEcosystemNuGet),
	string(gitlab.DependencyFirewallEcosystemPub),
	string(gitlab.DependencyFirewallEcosystemPyPI),
	string(gitlab.DependencyFirewallEcosystemSwift),
}

// EvaluatePackageInput holds the parameters for evaluating one package
// coordinate against a project's Dependency Firewall policies.
//
// The three coordinate fields map one-to-one onto client-go's
// EvaluatePackageOptions, which carries exactly Ecosystem, Name and Version.
// The documented optional `operation` attribute (download or upload) has no
// field on that options struct, so it cannot be sent through the wrapper; the
// gap is recorded in docs/development/upstream-bugs.md. GitLab defaults the
// attribute to download.
type EvaluatePackageInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Ecosystem string               `json:"ecosystem"  jsonschema:"Package ecosystem: cargo, composer, conan, gem, golang, maven, npm, nuget, pub, pypi, or swift,required"`
	Name      string               `json:"name"       jsonschema:"Package name, maximum 255 characters. For maven use the groupId:artifactId form (com.example:trivial-lib). For pypi, names are normalized per PEP 503 before evaluation,required"`
	Version   string               `json:"version"    jsonschema:"Package version, maximum 255 characters,required"`
}

// EvaluatePackageOutput is the outcome of a Dependency Firewall evaluation.
//
// It carries both fields of client-go's PackageEvaluation. Reason stays a
// pointer, as it is upstream: GitLab answers null when the outcome is allowed,
// and a pointer keeps "no reason was given" distinguishable from "the reason
// was the empty string".
type EvaluatePackageOutput struct {
	toolutil.HintableOutput
	// Outcome is allowed, warned or blocked. An allowed outcome means no
	// policy rule matched the package, which is not an assertion that
	// GitLab holds vulnerability or license data for it: a package absent
	// from the package metadata database is also allowed.
	Outcome string `json:"outcome"`
	// Reason names the policy that produced a warned or blocked outcome,
	// and is null for an allowed one.
	Reason *string `json:"reason"`
}

// toEvaluatePackageOutput converts the client-go evaluation into the MCP output.
func toEvaluatePackageOutput(e *gitlab.PackageEvaluation) EvaluatePackageOutput {
	return EvaluatePackageOutput{
		Outcome: string(e.Outcome),
		Reason:  e.Reason,
	}
}

// EvaluatePackage evaluates a single package coordinate against a project's
// Dependency Firewall policies.
//
// Endpoint: POST /api/v4/projects/:id/dependency_firewall/evaluate. The call
// changes nothing on the instance, so the action is read-only even though the
// verb is POST.
//
// A 404 is left for the action route to turn into an informational result
// naming the feature flag: on an instance where dependency_firewall_phase1 is
// off every project answers 404, and the bare status alone would read as "this
// project does not exist".
func EvaluatePackage(ctx context.Context, client *gitlabclient.Client, input EvaluatePackageInput) (EvaluatePackageOutput, error) {
	if input.ProjectID == "" {
		return EvaluatePackageOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	ecosystem := strings.ToLower(strings.TrimSpace(input.Ecosystem))
	if ecosystem == "" {
		return EvaluatePackageOutput{}, toolutil.ErrFieldRequired("ecosystem")
	}
	if !slices.Contains(Ecosystems, ecosystem) {
		return EvaluatePackageOutput{}, toolutil.ErrInvalidEnum("ecosystem", input.Ecosystem, Ecosystems)
	}
	name, err := coordinate("name", input.Name)
	if err != nil {
		return EvaluatePackageOutput{}, err
	}
	version, err := coordinate("version", input.Version)
	if err != nil {
		return EvaluatePackageOutput{}, err
	}
	if err = ctx.Err(); err != nil {
		return EvaluatePackageOutput{}, toolutil.WrapErr(evaluateOperation, err)
	}

	opts := &gitlab.EvaluatePackageOptions{
		Ecosystem: gitlab.DependencyFirewallEcosystemValue(ecosystem),
		Name:      name,
		Version:   version,
	}

	evaluation, _, err := client.GL().SecurityDependencyFirewall.EvaluatePackage(input.ProjectID.String(), opts, gitlab.WithContext(ctx))
	if err != nil {
		return EvaluatePackageOutput{}, wrapEvaluateErr(err)
	}
	return toEvaluatePackageOutput(evaluation), nil
}

// Hints for the refusals the evaluate endpoint documents beside the 404 the
// action route turns into a card.
const (
	// notEnforcedHint answers the 422 (dependency_firewall_not_enforced) GitLab
	// sends once the flag is on for a project the firewall is not turned on
	// for. On a self-managed instance that switch is an instance setting only
	// the Admin area writes, so the hint says where it is rather than naming an
	// action no surface offers.
	notEnforcedHint = "the Dependency Firewall is not turned on for this project, and GitLab evaluates a package only where it is: on a self-managed instance an administrator enables it under Admin > Settings > Security and compliance > Dependency firewall, which no API sets, and on GitLab.com an Owner enables it in the top-level group's security settings. Until then treat the package as not evaluated rather than as allowed"
	// evaluationFailedHint answers the 503 (dependency_firewall_evaluation_failed)
	// GitLab sends when it could not look the package up.
	evaluationFailedHint = "GitLab could not complete the evaluation. Retry after a short backoff, and until one answers treat the package as not evaluated rather than as allowed"
	// forbiddenHint answers a 403, whose likeliest cause here is the license.
	forbiddenHint = "the Dependency Firewall API requires GitLab Premium or Ultimate and a token allowed to read the project"
)

// wrapEvaluateErr adds the hint for the refusal the evaluate endpoint
// answered with. GitLab 19.4 answers 422 for a project the firewall is not
// turned on for, which the generic classification reads as invalid input and
// so sends a caller to change a coordinate that was never the problem.
func wrapEvaluateErr(err error) error {
	switch {
	case toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity):
		return toolutil.WrapErrWithHint(evaluateOperation, err, notEnforcedHint)
	case toolutil.IsHTTPStatus(err, http.StatusServiceUnavailable):
		return toolutil.WrapErrWithHint(evaluateOperation, err, evaluationFailedHint)
	default:
		return toolutil.WrapErrWithStatusHint(evaluateOperation, err, http.StatusForbidden, forbiddenHint)
	}
}

// coordinate validates one required package coordinate field and returns it
// trimmed. GitLab caps both name and version at 255 characters and answers 400
// beyond that, which says nothing about which field was too long.
func coordinate(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", toolutil.ErrFieldRequired(field)
	}
	if len(trimmed) > maxCoordinateLength {
		return "", fmt.Errorf("%s must be at most %d characters, got %d", field, maxCoordinateLength, len(trimmed))
	}
	return trimmed, nil
}
