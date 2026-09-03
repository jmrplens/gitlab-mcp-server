// Package dependencyfirewall implements the MCP tool for GitLab's Dependency
// Firewall package evaluation endpoint.
//
// One handler wraps the single public endpoint:
//
//   - EvaluatePackage — POST /api/v4/projects/:id/dependency_firewall/evaluate
//
// The endpoint evaluates one package coordinate (ecosystem, name, version)
// against the project's Dependency Firewall policies and answers with an
// outcome of allowed, warned or blocked, plus the policy that produced a
// warned or blocked outcome.
//
// Tier and availability, both taken from the API page rather than inferred:
// "Tier: Premium, Ultimate" and "Offering: GitLab.com, GitLab Self-Managed,
// GitLab Dedicated", so the action is gated at premium and is not GitLab.com
// only. The API itself was "Introduced in GitLab 19.4 with a feature flag
// named `dependency_firewall_phase1`. Disabled by default", and it is an
// experiment. An instance that does not have the flag enabled answers 404 for
// every project, which is indistinguishable from a project the token cannot
// see, so [EvaluatePackage] turns a 404 into an informational result naming
// the flag instead of a bare error. That is the shape internal/tools/orbit
// already uses for the equally experimental Knowledge Graph API.
//
// Reference: https://docs.gitlab.com/api/dependency_firewall/
package dependencyfirewall
