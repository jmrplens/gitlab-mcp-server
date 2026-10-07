package main

import (
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_action_grants/internal/join"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// The categories of a classic declaration: the three ways a REST route this
// server sends departs from GitLab's general rule, read_api for a GET or a
// HEAD and api for any other method (lib/api/api.rb:60-61 at v19.4.1-ee).
const (
	// categoryReadAPIEveryMethod is a route whose class grants read_api for
	// every method, its own comment calling it a read-only operation although
	// it is sent as a POST.
	categoryReadAPIEveryMethod = "read-api-every-method"
	// categoryEveryScope is a route whose class grants every scope a personal
	// access token can carry.
	categoryEveryScope = "every-scope"
	// categoryOursNotRead is a route GitLab authenticates by another
	// credential the caller passes as a parameter, a runner's token or a
	// trigger's, and on which nothing asks for the user, which is what reads
	// and validates the token this server holds (lib/api/helpers.rb:96 at
	// v19.4.1-ee).
	categoryOursNotRead = "credential-not-read"
)

// classicRoutes are the REST routes this server sends whose classic scope
// GitLab does not decide by their method, from a sweep of GitLab 19.4.1's
// source for every scope grant on a route that is not a GET and every route
// that skips the fine-grained check for another credential. The join holds a
// credential-not-read entry to the route's skip reason in the live record and
// reports a route with such a skip that no entry declares; the record carries
// no route's scopes, so the read_api grants are this table's alone, and a new
// grant of read_api on a route of ours is what nothing committed would catch.
// An entry no action sends, or one that agrees with the general rule, is a
// finding.
var classicRoutes = []join.ClassicDeclaration{
	{
		Route: "POST /orbit/query", Category: categoryReadAPIEveryMethod, Scope: finegrained.ClassicReadAPI,
		Reason: "ee/lib/api/orbit/data.rb:24-25 at v19.4.1-ee grants read_api to every route of API::Orbit::Data, its comment calling the POST a read-only operation. Measured on a self-managed 19.4.1-ee, where Orbit is not served: read_api passed the scope check and was answered 404 like api, and read_user was refused insufficient_scope. GitLab.com, the one instance that serves Orbit, carries the same line and was not measured with a read_api token; listing the action leaves GitLab's own 403 to answer if it refuses",
	},
	{
		Route: "POST /markdown", Category: categoryReadAPIEveryMethod, Scope: finegrained.ClassicReadAPI,
		Reason: "lib/api/markdown.rb:7-8 at v19.4.1-ee grants read_api to the route, its comment calling the POST a read-only operation. Measured on 19.4.1 CE and EE: a read_api token is answered 201",
	},
	{
		Route: "DELETE /personal_access_tokens/self", Category: categoryEveryScope, Scope: finegrained.ClassicReadAPI,
		Reason: "lib/api/personal_access_tokens/self_information.rb:16 at v19.4.1-ee grants every available scope, so a token may revoke itself whatever it carries, and :19-23 answers 400 to a token that is not a personal access token",
	},
	{
		Route: "POST /runners", Category: categoryOursNotRead, Scope: finegrained.ClassicOtherCredential,
		Reason: "lib/api/ci/runner.rb:54-64 at v19.4.1-ee skips the fine-grained check as runner_token_auth and registers the runner with Ci::Runners::RegisterRunnerService.new(params[:token], ...), the registration token the caller passes; nothing on the route asks for the user",
	},
	{
		Route: "DELETE /runners", Category: categoryOursNotRead, Scope: finegrained.ClassicOtherCredential,
		Reason: "lib/api/ci/runner.rb:95-100 at v19.4.1-ee skips the fine-grained check as runner_token_auth and authenticates with authenticate_runner!, which finds the runner by params[:token] (lib/api/ci/helpers/runner.rb:16-18 and 56-63); nothing on the route asks for the user",
	},
	{
		Route: "POST /runners/verify", Category: categoryOursNotRead, Scope: finegrained.ClassicOtherCredential,
		Reason: "lib/api/ci/runner.rb:140-142 at v19.4.1-ee skips the fine-grained check as runner_token_auth and authenticates with authenticate_runner! alone. Measured on 19.4.1: a read_api token, an api token and no token at all are answered the same",
	},
	{
		Route: "POST /projects/:id/(ref/:ref/)trigger/pipeline", Category: categoryOursNotRead, Scope: finegrained.ClassicOtherCredential,
		Reason: "lib/api/ci/triggers.rb:41-57 at v19.4.1-ee skips the fine-grained check as trigger_token_auth, finds the project with find_project rather than find_project!, and runs Ci::PipelineTriggerService.new(project, nil, params), which authenticates the trigger token the caller passes; nothing on the route asks for the user",
	},
}

// classicDeclaration answers, for one action, a gate the classic scope is
// held to: why the action departs from what the gate expects.
type classicDeclaration struct {
	Action   string
	Category string
	Reason   string
}

// The categories of an action-level classic declaration.
const (
	// categoryRequiresAPIForARead is an action the catalog classifies as a
	// read whose request GitLab refuses a read_api token.
	categoryRequiresAPIForARead = "gitlab-requires-api-for-a-read"
	// categoryWritesOnlyLocally is an action that reads GitLab and writes a
	// file on the machine the server runs on.
	categoryWritesOnlyLocally = "writes-only-locally"
	// categoryRevokesTheCallingToken is an action that revokes the very token
	// it is called with, which GitLab lets every scope do.
	categoryRevokesTheCallingToken = "revokes-the-calling-token"
	// categoryAnotherCredential is an action that writes with a credential
	// the caller passes, which GitLab authenticates instead of the token.
	categoryAnotherCredential = "another-credential-authenticates"
)

// classicVariations are the actions whose ways of running need different
// classic scopes, or that send an optional request needing more than the
// action, each with why that is right (gate 4). A variation leaves a read_api
// token served an action one of whose inputs GitLab refuses it, so none is
// declared without a reason a reviewer can check; there is none today.
var classicVariations []classicDeclaration

// annotationDisagreements are the actions whose catalog classification, read
// or write, disagrees with whether a read_api token reaches them (gate 5). The
// classification decides read-only mode, safe mode and the confirmation a
// destructive action asks for, and the reach decides what a read_api token is
// served, so each departure is declared here with its reason.
var annotationDisagreements = []classicDeclaration{
	{
		Action: "template.lint", Category: categoryRequiresAPIForARead,
		Reason: "it posts to /projects/:id/ci/lint, which lib/api/lint.rb at v19.4.1-ee grants ai_workflows on a POST and never read_api. Measured on 19.4.1 CE and EE: a read_api token is refused 403 insufficient_scope",
	},
	{
		Action: "project.dependency_firewall_evaluate", Category: categoryRequiresAPIForARead,
		Reason: "it posts to /projects/:id/dependency_firewall/evaluate, whose class grants no scope (ee/lib/api/security/dependency_firewall/evaluations.rb at v19.4.1-ee, its comment at :80 calling it a write path). Measured on 19.4.1-ee in each state of its flag and setting: a read_api token is refused 403",
	},
	{
		Action: "package.download", Category: categoryWritesOnlyLocally,
		Reason: "it sends GitLab GETs only and writes the package file to output_path on the machine the server runs on",
	},
	{
		Action: "access.token_personal_revoke_self", Category: categoryRevokesTheCallingToken,
		Reason: "DELETE /personal_access_tokens/self accepts every scope, so any personal access token may revoke itself; the action stays destructive and asks for confirmation",
	},
	{
		Action: "pipeline.trigger_run", Category: categoryAnotherCredential,
		Reason: "it creates a pipeline with the trigger token the caller passes, which GitLab authenticates instead of the token this server holds",
	},
	{
		Action: "runner.register", Category: categoryAnotherCredential,
		Reason: "it registers a runner with the registration token the caller passes, which GitLab authenticates instead of the token this server holds",
	},
	{
		Action: "runner.delete_by_token", Category: categoryAnotherCredential,
		Reason: "it deletes the runner whose authentication token the caller passes, which GitLab authenticates instead of the token this server holds",
	},
}
