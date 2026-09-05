package dependencyfirewall

import (
	"context"
	"fmt"
	"net/http"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionEvaluate is the canonical action ID. The evaluate endpoint is
// projected under the gitlab_project meta-tool, so its domain prefix is
// "project" (see actioncatalog DomainFromToolName). One evaluate call is not a
// domain, and a catalog group of one would cost every Premium instance a
// meta-tool holding a single action.
const ActionEvaluate = "project.dependency_firewall_evaluate"

// individualToolName is the individual-surface tool name for the evaluate
// action, declared rather than derived (IndividualTool.Name is the only source
// of truth for it).
const individualToolName = "gitlab_project_dependency_firewall_evaluate"

// ActionSpecs returns the canonical spec for the Dependency Firewall package
// evaluation action.
//
// The action is gated at premium because the API page states "Tier: Premium,
// Ultimate", and it is not marked GitLab.com only because the same page states
// "Offering: GitLab.com, GitLab Self-Managed, GitLab Dedicated".
//
// It is read-only despite being a POST: evaluating a coordinate against the
// project's policies returns a verdict and changes nothing, so --read-only and
// a read_api token both keep it.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("dependency_firewall_evaluate", evaluateRoute(client), toolutil.ActionSpecOptions{
			Aliases: []string{
				individualToolName,
				"evaluate a package against the dependency firewall",
				"check whether a package is blocked",
				"dependency firewall verdict for a package",
				"is this package allowed by policy",
			},
			Tags:  []string{"project", "security", "dependency_firewall", "supply_chain", "policy"},
			Usage: "Evaluate one package coordinate against a project's Dependency Firewall policies before depending on it. Use when the prompt asks whether a package is allowed, warned or blocked. Pass project_id, ecosystem, name and version. The call is an evaluation and installs nothing.",
			RelatedActions: []string{
				"project.get",
				"dependency.list",
				"project.security_settings_get",
			},
			InputSchemaOverrides: []toolutil.InputSchemaOverride{
				toolutil.SchemaPropertyOverride("ecosystem", map[string]any{
					"enum": ecosystemEnum(),
				}),
			},
			ParameterGuidance: map[string]toolutil.ParameterGuidance{
				"project_id": {
					SemanticRole:   "scope_project",
					ValueSource:    "Project ID or path whose Dependency Firewall policies decide the verdict.",
					ExampleBinding: `params.project_id:"group/project"`,
				},
				"ecosystem": {
					SemanticRole:   "package_ecosystem",
					ValueSource:    "The registry the package comes from, one of the enum values on this parameter.",
					ExampleBinding: `params.ecosystem:"npm"`,
				},
				"name": {
					SemanticRole:     "package_name",
					ValueSource:      "Package name as the registry spells it. Maven takes the groupId:artifactId form.",
					CommonConfusions: []string{"the project name", "the repository path"},
					ExampleBinding:   `params.name:"lodash"`,
				},
				"version": {
					SemanticRole:   "package_version",
					ValueSource:    "Exact version string to evaluate. Ranges are not evaluated.",
					ExampleBinding: `params.version:"4.17.15"`,
				},
			},
			OpenWorld:    true,
			Edition:      "premium",
			OwnerPackage: "dependencyfirewall",
			IndividualTool: toolutil.IndividualToolSpec{
				Name:        individualToolName,
				Title:       toolutil.TitleFromName(individualToolName),
				Description: "Evaluate one package coordinate against a project's Dependency Firewall policies (Premium, experimental). Returns: the outcome (allowed, warned or blocked) and the policy that produced a warned or blocked verdict. See also: gitlab_project_get, gitlab_list_project_dependencies.",
			},
		}),
	}
}

// ecosystemEnum returns the ecosystem values as the []any a JSON Schema enum
// needs, from the one Ecosystems list the handler also validates against.
func ecosystemEnum() []any {
	enum := make([]any, 0, len(Ecosystems))
	for _, e := range Ecosystems {
		enum = append(enum, e)
	}
	return enum
}

// evaluateRoute wraps the evaluate handler so an HTTP 404 becomes an
// informational result naming the feature flag instead of a bare error.
//
// The flag matters more here than a missing project does: while
// dependency_firewall_phase1 is off, every project on the instance answers 404,
// so a caller told only "not found" concludes the project is wrong and retries
// with another one forever.
func evaluateRoute(client *gitlabclient.Client) toolutil.ActionRoute {
	return toolutil.RouteAction(client, EvaluatePackage).WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if err != nil && toolutil.IsHTTPStatus(err, http.StatusNotFound) {
				return notFoundOutput{ProjectID: projectIdentifier(input)}, nil
			}
			return result, err
		}
	})
}

// projectIdentifier renders the project_id the caller passed, for the
// not-found message. An absent or unreadable value degrades to "the requested
// project" rather than an empty pair of asterisks.
func projectIdentifier(input map[string]any) string {
	value, ok := input["project_id"]
	if !ok || value == nil {
		return "the requested project"
	}
	if text := fmt.Sprintf("%v", value); text != "" {
		return "project " + text
	}
	return "the requested project"
}
