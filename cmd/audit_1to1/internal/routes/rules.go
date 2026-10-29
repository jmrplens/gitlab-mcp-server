// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package routes

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// The categories of a skip reason. The first two exclude nothing: a user's
// token can call the route, so it is classified as any other route. The rest
// exclude the routes that carry it, unless a reached route carries it too.
const (
	// skipPublic: GitLab serves the route with or without a token.
	skipPublic = "public"
	// skipUserToken: a user's token can call the route; GitLab only exempts
	// it from the fine-grained check.
	skipUserToken = "user-token"
	// skipOtherCaller: the route authenticates a credential that is not a
	// user's token, which a component, a runner, a job or another service
	// holds.
	skipOtherCaller = "other-credential"
	// skipInboundWebhook: another service delivers a request it signs.
	skipInboundWebhook = "inbound-webhook"
	// skipWorkhorse: Workhorse calls the route on the way to the one it
	// proxies.
	skipWorkhorse = "workhorse"
	// skipRemoved: GitLab removed the feature and answers the route with an
	// empty list or the removal notice.
	skipRemoved = "removed"
	// skipClientProtocol: the protocol of a package manager's client, which
	// GitLab forwards to the public registry.
	skipClientProtocol = "client-protocol"
	// skipInternalTesting: an endpoint GitLab's own evaluations call.
	skipInternalTesting = "internal-testing"
	// skipCatchAll: the route that answers a path no other route mounts.
	skipCatchAll = "catch-all"
)

// skipCategories are the categories a skip reason takes, each with whether it
// excludes the routes that carry it.
var skipCategories = map[string]bool{
	skipPublic:          false,
	skipUserToken:       false,
	skipOtherCaller:     true,
	skipInboundWebhook:  true,
	skipWorkhorse:       true,
	skipRemoved:         true,
	skipClientProtocol:  true,
	skipInternalTesting: true,
	skipCatchAll:        true,
}

// skipReason is what the table says of one reason a route skips the
// fine-grained check for.
type skipReason struct {
	Category string
	Reason   string
}

// excludes reports whether the reason takes the routes that carry it out of
// the classification.
func (s skipReason) excludes() bool { return skipCategories[s.Category] }

// otherCredential builds a reason a credential that is not a user's token
// calls the route with.
func otherCredential(reason string) skipReason {
	return skipReason{Category: skipOtherCaller, Reason: reason}
}

// skipReasons is closed over the whole record: a reason a route of the record
// carries that it does not hold is a problem of the table, whatever the
// route's class, and on a route no action reaches under /api/:version a
// finding too; a row whose reason no route carries is stale. Each reason is
// GitLab's own (route_setting :authorization,
// skip_granular_token_authorization: :<reason>), and what each says is read
// from the code that sets it.
var skipReasons = map[string]skipReason{
	"ai_workflows_oauth_auth": {
		Category: skipUserToken,
		Reason:   "a running workflow's ai_workflows token calls it, and so does a user's token with the api scope: each class adds the ai_workflows scope to the api scope API::API allows (lib/api/api.rb, lib/api/api_guard.rb) and only authenticates the user (ee/lib/api/ai/duo_workflows/workflows_internal.rb, whose comment that the Duo Workflow Service and Executor consume it restricts nothing); the routes that check the flow's composite identity are skipExceptions",
	},
	"ai_workflows_token_auth": {
		Category: skipUserToken,
		Reason:   "the Duo Workflow Service posts a workflow's audit events with its ai_workflows token, and a user's token with the api scope may too: the class adds that scope to api and only authenticates the user, whose own workflow it must be (ee/lib/api/ai/duo_workflows/audit_events.rb)",
	},
	"catch_all": {Category: skipCatchAll, Reason: "Grape's route for any path no other route mounts, which answers 404 (and 410 under the removed API v3, lib/api/api.rb)"},
	"cd_rollout_workflow_event": otherCredential(
		"a rollout's workflow posts its events with the per-rollout callback token, not a user's token (ee/lib/api/cd/rollouts.rb)",
	),
	"compliance_external_auth":            otherCredential("an external compliance service sets a control's status with the HMAC signature of the control's secret (ee/lib/api/compliance_external_controls.rb)"),
	"container_registry_event_auth":       otherCredential("the container registry notifies GitLab of an operation with its notification secret"),
	"deprecated_endpoint":                 {Category: skipRemoved, Reason: "GitLab removed the License-Check feature and its managed licenses API: the list answers an empty list and the rest the removal notice (ee/lib/api/managed_licenses.rb), license approval policies replacing it"},
	"error_tracking_token_auth":           otherCredential("the error tracking service checks a project's key with its shared token"),
	"external_registry_redirect":          {Category: skipClientProtocol, Reason: "an npm client's metadata and audit requests, which GitLab forwards to npmjs.org when the package is not its own (lib/api/concerns/packages/npm_endpoints.rb)"},
	"geo_jwt_auth":                        otherCredential("a Geo secondary calls the primary with its Geo JWT"),
	"geo_proxy_auth":                      otherCredential("Workhorse on a Geo site asks whether it should proxy, with the Geo JWT (lib/api/geo.rb)"),
	"github_oauth_webhook_signature_auth": {Category: skipInboundWebhook, Reason: "GitHub delivers a webhook of an OAuth-connected repository, signed with its webhook secret"},
	"gitlab_shared_secret_auth":           otherCredential("GitLab Shell posts a git audit event with the secret it shares with GitLab"),
	"gitlab_shell_token_auth":             otherCredential("GitLab Shell calls it with the secret token it shares with GitLab"),
	"iam_oauth_token_auth":                otherCredential("only an IAM-issued OAuth JWT identifies the caller; a personal access token is refused (lib/api/authn/userinfo.rb)"),
	"integration_token_auth":              otherCredential("Mattermost or Slack posts a slash command with the integration's token"),
	"internal_testing":                    {Category: skipInternalTesting, Reason: "GitLab's own evaluation of Duo Code Review calls it (ee/lib/api/duo_code_review.rb)"},
	"jira_forge_app_auth":                 otherCredential("the GitLab for Jira (Forge) app calls it with the token Atlassian issues the app"),
	"job_token_auth":                      otherCredential("a CI job calls it with its job token (CI_JOB_TOKEN), which a user's token is not"),
	"kas_jwt_auth":                        otherCredential("the agent server for Kubernetes (KAS) calls it with its JWT"),
	"mailroom_token_auth":                 otherCredential("mail_room posts an incoming email with the secret it shares with GitLab"),
	"modular_service_token_exchange":      {Category: skipUserToken, Reason: "every personal access token, classic or fine-grained, may call it and GitLab only keeps it out of the fine-grained permissions (ee/lib/api/authn/token_exchange.rb)"},
	"openbao_token_auth":                  otherCredential("OpenBao posts its audit log with its token"),
	"orbit_internal_auth":                 otherCredential("the Orbit indexer calls it with its internal token"),
	"pages_token_auth":                    otherCredential("the GitLab Pages daemon reads a domain's configuration with the secret it shares with GitLab"),
	"public_endpoint":                     {Category: skipPublic, Reason: "GitLab serves the route with or without a token, so a user's token reaches it as any other"},
	"runner_token_auth":                   otherCredential("a runner calls it with its authentication token, which an action of the runner group takes from the caller"),
	"scim_token_auth":                     otherCredential("an identity provider provisions users with the group's or the instance's SCIM token"),
	"slack_signature_auth":                {Category: skipInboundWebhook, Reason: "Slack delivers an event signed with the app's signing secret"},
	"subscription_portal_jwt_auth":        otherCredential("the Customers Portal calls it with its JWT"),
	"trigger_token_auth":                  otherCredential("a pipeline trigger token starts the pipeline, which pipeline.trigger_run takes from the caller"),
	"unleash_token_auth":                  otherCredential("an Unleash client reads a project's feature flags with its instance ID (lib/api/unleash.rb)"),
	"usage_data_auth":                     {Category: skipUserToken, Reason: "a user's token downloads the metric definitions, which GitLab keeps out of the fine-grained permissions (lib/api/usage_data.rb)"},
	"workhorse_pre_authorization":         {Category: skipWorkhorse, Reason: "Workhorse asks it whether an upload may go ahead before it streams the file to the route beside it, which is the one a caller sends"},
	"workhorse_verification_auth":         {Category: skipWorkhorse, Reason: "Workhorse calls it with the JWT it signs, on the request it proxies"},
}

// compositeIdentity builds the reason of a route whose handler refuses
// anything but a Duo flow's composite identity, read in the file named.
func compositeIdentity(file string) skipReason {
	return otherCredential("only a Duo flow's service account acting for a user calls it: verify_flow_composite_identity! (ee/lib/api/helpers/duo_workflow_helpers.rb) answers 403 to a user's own token (" + file + ")")
}

// skipExceptions are the routes whose skip reason excludes nothing that
// GitLab's code limits to another credential all the same, keyed by the
// route's name, each with the category and the reason that code gives. An
// exception answers a route its reason does not exclude on its own, so one
// whose route is reached, carries no reason or a reason that excludes it
// already answers nothing and is stale.
var skipExceptions = map[string]skipReason{
	"POST /projects/:id/merge_requests/:merge_request_iid/suggested_reviewers": compositeIdentity("ee/lib/api/ai_suggested_reviewers.rb"),
	"POST /ai/duo_workflows/code_review/add_comments":                          compositeIdentity("ee/lib/api/ai/duo_workflows/code_review.rb"),
	"POST /ai/duo_workflows/tools/risk_classification/results":                 compositeIdentity("ee/lib/api/ai/duo_workflows/tools/risk_classification/results.rb"),
}

// skipped decides a route that skips the fine-grained check: a reason the
// table does not hold is a finding, a reason it holds takes the route out
// unless it excludes nothing or a reached route carries it, and an exception
// takes out a route its reason leaves in. It reports whether the route is
// decided.
func (j *judge) skipped(route *apilive.Route, verdict *Route) bool {
	reason := skipOf(route)
	if reason == "" {
		return false
	}
	verdict.SkipReason = reason
	row, known := j.in.skips[reason]
	if !known {
		verdict.Class = ClassUnknownSkipReason
		return true
	}
	if !row.excludes() || j.reachedReasons[reason] {
		exception, held := j.in.exceptions[verdict.Route]
		if !held {
			return false
		}
		j.excepted[verdict.Route] = true
		row = exception
	}
	verdict.Class, verdict.Category, verdict.Reason = ClassSkip, row.Category, row.Reason
	return true
}

// skipProblems lists what is wrong with the skip table: a row with a category
// it does not define or no reason, a row whose reason no route carries, and a
// reason a route carries that no row holds.
func skipProblems(skips map[string]skipReason, carried map[string]bool) []string {
	var out []string
	for reason, row := range skips {
		label := "skip reason " + reason
		if _, defined := skipCategories[row.Category]; !defined {
			out = append(out, label+": category "+strconv.Quote(row.Category)+" is not one the table defines")
		}
		if row.Reason == "" {
			out = append(out, label+" gives no reason")
		}
		if !carried[reason] {
			out = append(out, label+" is carried by no route of the record")
		}
	}
	for reason := range carried {
		if _, held := skips[reason]; !held {
			out = append(out, "skip reason "+reason+" is carried by a route of the record and the table does not hold it")
		}
	}
	return out
}

// exceptionProblems lists what is wrong with the skip exceptions: one whose
// category the table does not define or excludes nothing, one with no
// reason, and one that answers no route.
func exceptionProblems(exceptions map[string]skipReason, excepted map[string]bool) []string {
	var out []string
	for name, exception := range exceptions {
		label := "skip exception " + name
		if !skipCategories[exception.Category] {
			out = append(out, label+": category "+strconv.Quote(exception.Category)+" excludes nothing or is not one the table defines")
		}
		if exception.Reason == "" {
			out = append(out, label+" gives no reason")
		}
		if !excepted[name] {
			out = append(out, label+" answers no route: the record does not mount it, an action reaches it, or its skip reason excludes it on its own")
		}
	}
	return out
}

// slugRule declares that an action reaches every route of a pattern whose
// varying segment the caller names: an integration's slug, which one handler
// dispatches on.
type slugRule struct {
	Action  string
	Method  string
	Pattern string
	Reason  string
}

// label names the rule in a report.
func (r slugRule) label() string { return r.Method + " " + r.Pattern + " (" + r.Action + ")" }

// integrationReason is why each integration route is reached by the one
// action of its scope.
const integrationReason = "the action names the integration by its slug, so it sends the PUT of whichever integration the caller asks for, and the request record places it on the first"

// slugRules are the actions whose request takes a segment of its route from
// the input.
var slugRules = []slugRule{
	{Action: "project.integration_set", Method: http.MethodPut, Pattern: "/projects/*/integrations/*", Reason: integrationReason},
	{Action: "project.integration_set_group", Method: http.MethodPut, Pattern: "/groups/*/integrations/*", Reason: integrationReason},
}

// slugAnswers decides a route a slug rule answers: one of the rule's verb and
// pattern that a spelling the rule's action derives meets. It reports whether
// the route is decided.
func (j *judge) slugAnswers(method, path string, verdict *Route) bool {
	for index, rule := range j.in.slugs {
		if rule.Method != method || !patternMatches(rule.Pattern, path) || !j.derives(rule.Action, method, path) {
			continue
		}
		j.slugRoutes[index] = append(j.slugRoutes[index], verdict.Route)
		verdict.Class, verdict.Family, verdict.Reason = ClassSlugFromInput, rule.label(), rule.Reason
		verdict.Actions = []string{rule.Action}
		return true
	}
	return false
}

// derives reports whether a spelling an action's requests derive meets a
// route: the same verb, and segment by segment the same, a placeholder of the
// derived spelling standing for any segment.
func (j *judge) derives(action, method, path string) bool {
	for _, derived := range j.derived[action] {
		verb, spelled, _ := strings.Cut(derived, " ")
		if verb != method {
			continue
		}
		want := apilive.PathSegments(spelled)
		for _, variant := range apilive.ExpandOptional(path) {
			if derivedMeets(want, apilive.PathSegments(variant)) {
				return true
			}
		}
	}
	return false
}

// derivedMeets reports whether a derived spelling meets a record spelling:
// segment by segment the same, a placeholder of the derived one standing for
// any segment.
func derivedMeets(derived, record []string) bool {
	if len(derived) != len(record) {
		return false
	}
	for index := range derived {
		if derived[index] != ":" && derived[index] != record[index] {
			return false
		}
	}
	return true
}

// slugProblems lists what is wrong with the slug rules: a rule that names an
// action the catalog does not hold, gives no reason or answers no route.
func slugProblems(rules []slugRule, answered [][]string, catalog map[string]bool) []string {
	var out []string
	for index, rule := range rules {
		label := "slug rule " + rule.label()
		if !catalog[rule.Action] {
			out = append(out, label+" names an action the catalog does not hold")
		}
		if rule.Reason == "" {
			out = append(out, label+" gives no reason")
		}
		if len(answered[index]) == 0 {
			out = append(out, label+" answers no route: no request of the action derives a spelling that meets one")
		}
	}
	return out
}

// The segment a deprecated route spells an integration under, and the one
// its twin does.
const (
	servicesSegment     = "services"
	integrationsSegment = "integrations"
)

// twinOf names the route of the same verb whose path spells the first
// segment from as to, reporting whether the path holds that segment.
func twinOf(method, path, from, to string) (string, bool) {
	segments := strings.Split(path, "/")
	index := slices.Index(segments, from)
	if index < 0 {
		return "", false
	}
	segments[index] = to
	return method + " " + strings.Join(segments, "/"), true
}

// aliasAnswers decides a /services/ route whose /integrations/ twin of the
// same verb is reached or answered: GitLab mounts both, the first as the
// deprecated spelling of the second. It reports whether the route is
// decided.
func (j *judge) aliasAnswers(method, path string, verdict *Route) bool {
	twin, spelled := twinOf(method, path, servicesSegment, integrationsSegment)
	if !spelled {
		return false
	}
	if _, held := j.routes[twin]; !held {
		return false
	}
	if findingClasses[j.classify(twin).Class] {
		return false
	}
	verdict.Class, verdict.Twin = ClassDeprecatedAlias, twin
	return true
}

// reachedAlias names the deprecated /services/ twin an action reaches in
// place of an /integrations/ route nothing answers, empty when there is none:
// the capability is offered, through the spelling GitLab deprecates, which
// the triage of the finding reads.
func (j *judge) reachedAlias(method, path string) string {
	twin, spelled := twinOf(method, path, integrationsSegment, servicesSegment)
	if !spelled || len(j.reached[twin]) == 0 {
		return ""
	}
	return twin
}
