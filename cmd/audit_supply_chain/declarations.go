// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// declarationsFile is where a declaration table finding sends its reader.
const declarationsFile = "cmd/audit_supply_chain/declarations.go"

// categoryHeldOffline is the one reason a match may be excused: the tool the
// pattern names is always run with a flag that forbids it to reach a
// registry, so what it runs is what the job built or installed from files it
// verified, and the reason names the flag and where it is passed.
const categoryHeldOffline = "held-offline"

// categories are the reasons a declaration may give, each with what it means.
// A declaration naming anything else is reported rather than trusted, since a
// category nobody defined is an excuse nobody reviewed.
var categories = map[string]string{
	categoryHeldOffline: "the matched tool is always given a flag that forbids network resolution, so it runs what the job built or verified; the reason names the flag and where it is passed",
}

// declaration is one excused match: why a file a credentialed job reaches may
// carry a pattern the run-time-code rules refuse, in a category and in words.
type declaration struct {
	category string
	reason   string
}

// declaredRunTimeCode are the matches this repository accepts in a file a
// credentialed job reaches, keyed by the file's repository-relative path (or
// Makefile:target for a recipe), one space, and the rule's name.
//
// An entry excuses that rule in that file wherever a credentialed job reaches
// it, and nothing else: the file's other rules, the same rule in another file,
// and a run block, which is fixed where it is written, are all still judged. A
// reference that resolves to no file is never declared either, since there is
// nothing behind it to excuse. An entry that excuses nothing is reported, on
// the terms every declaration table in this repository is held to.
var declaredRunTimeCode = map[string]declaration{
	"scripts/validate-npm.mjs npx": {
		category: categoryHeldOffline,
		reason:   "stopUnderNpx starts the launcher the way a client configured with npx does, to prove a SIGTERM sent to npx alone ends the server, and passes --offline in the arguments it spawns npx with, so npx resolves the package from the project the job installed from the tarballs it just packed and cannot fetch a published one; serverUnder then holds the process to that project's binary. The other matches are comments and the child process variable named after the tool",
	},
}

// categoryGeneratedAtRunTime is the one reason a uses: of a directory that
// holds no action may be accepted: the action that names it writes that
// directory at run time, from a file of its own the record holds and the audit
// judges, and the reason says what the written action runs and why that is
// accepted.
const categoryGeneratedAtRunTime = "generated-at-run-time"

// actionCategories are the reasons a declaration of an unjudged action may
// give, kept apart from [categories] because each table's reasons answer a
// different question.
var actionCategories = map[string]string{
	categoryGeneratedAtRunTime: "the action that names the directory writes it at run time from a file the record holds and the audit judges; the reason says what the written action runs and why that is accepted",
}

// declaredUnjudgedActions are the uses: of a directory holding no action that
// this repository accepts, keyed by what writes the reference (the action
// without its commit, a local action as its uses: spells it, or the workflow's
// path), one space, and the reference as written.
//
// The key leaves the commit out on purpose: a bump of the action keeps the
// declaration, and the record shows the reviewer of that bump what the
// generating file does now. An entry that excuses nothing is reported, on the
// terms every declaration table in this repository is held to.
//
// The table is empty. Its one entry was pypa/gh-action-pypi-publish, whose
// generated container action runs an image pypa names by a registry tag, which
// a push to pypa's ghcr.io namespace can move and this audit cannot see; the
// pypi job uploads with scripts/publish-pypi.sh instead (issue 1243).
var declaredUnjudgedActions = map[string]declaration{}

// categoryInputGuarded is the one reason a match in the run block of a pinned
// composite action's step may be accepted: the match sits in a branch the
// script takes only when one of the action's inputs holds one value, and the
// audit holds every step of a credentialed job that names the action to
// setting that input to a literal other than that value, or to leaving it to a
// default other than it.
const categoryInputGuarded = "input-guarded"

// guardCategories are the reasons a declaration of a guarded match may give,
// kept apart from [categories] because each table's reasons answer a
// different question.
var guardCategories = map[string]string{
	categoryInputGuarded: "the match runs only when one input of the action holds one value, which the audit holds every step naming the action to not setting; the reason quotes the branch and says what the other values run",
}

// guardedMatch is one excused match in a pinned composite action's run block:
// the input whose value takes the branch the match sits in, that value, and
// why the branch is accepted, in a category and in words.
type guardedMatch struct {
	category string
	input    string
	value    string
	reason   string
}

// declaredGuardedMatches are the matches in a pinned composite action's run
// blocks this repository accepts, keyed by the action without its commit, the
// word step, the step's index in the action's steps, and the rule's name,
// separated by single spaces.
//
// A run block of a workflow, or of an action in this repository, is fixed
// where it is written and is never declared; one in somebody else's action
// cannot be edited here, and a branch of it that the inputs every step passes
// never take is not code the job runs. The key leaves the commit out, as
// [declaredUnjudgedActions] does, so a bump keeps the declaration and the
// record shows the reviewer of the bump what the step does now; a bump that
// moves the match to another step leaves this entry excusing nothing, which is
// reported, on the terms every declaration table in this repository is held
// to.
var declaredGuardedMatches = map[string]guardedMatch{
	"sigstore/cosign-installer step 0 go-install": {
		category: categoryInputGuarded,
		input:    "cosign-release",
		value:    "main",
		reason:   "the step's script runs go install github.com/sigstore/cosign/v3/cmd/cosign@main only inside if [[ \"${input_cosign_release}\" == \"main\" ]], which builds cosign from its main branch and exits; any other release is a bootstrap binary held to a SHA-256 the action commits, which then verifies the requested release against a public key whose SHA-256 the action commits too. Every step here names an exact cosign-release",
	},
}

// categoryWriteFreeJob is the one reason an image that a pinned container
// action names by a tag may be accepted: every job that runs it is held by the
// audit to a token, declared by the job or its workflow, that writes nothing
// but its OIDC identity, to cache-mode
// none and to reading no secret, so whatever a push to the tag puts there runs
// with no write of this repository, no entry of the Actions cache to save or
// restore, and no secret. The cache is held apart because permissions: does
// not reach it: the runner hands every container action the job's runtime
// token, which on a push or a schedule saves to the cache that later jobs on
// main and under a release tag restore. The identity the job can still mint is
// one the audit cannot follow to what accepts it, so the reason says why the
// image is not named by its digest and which of the verifiers this repository
// relies on accept that identity.
const categoryWriteFreeJob = "write-free-job"

// imageCategories are the reasons a declaration of a tag-named image may give,
// kept apart from [categories] because each table's reasons answer a
// different question.
var imageCategories = map[string]string{
	categoryWriteFreeJob: "every job that runs the image holds a token, declared by the job or its workflow, that writes nothing but its OIDC identity, runs with cache-mode none and reads no secret, which the audit checks at each of them; the reason says why the image is not named by its digest and which verifiers accept the identity the job can mint",
}

// declaredTaggedImages are the images that pinned container actions run by a
// tag and that this repository accepts, keyed by the action without its
// commit, one space, and the docker:// image without its tag or digest.
//
// The key leaves the commit and the tag out on purpose: a bump of the action
// that moves its tag keeps the declaration, and the record shows the reviewer
// of that bump the tag the new commit names, while a bump that moves the image
// to another repository leaves the entry excusing nothing. An action of this
// repository is never declared, since its image can be pinned where it is
// written. An entry that excuses nothing is reported, on the terms every
// declaration table in this repository is held to.
var declaredTaggedImages = map[string]declaration{
	"ossf/scorecard-action docker://ghcr.io/ossf/scorecard-action": {
		category: categoryWriteFreeJob,
		reason:   "scorecard.yml publishes its results to the OpenSSF results API, which takes a run's results only when every step of the Scorecard job is one it approves, and a step naming the image by its digest is not: the API's verify_workflow.go reads a step's name as everything before its @, so docker://ghcr.io/ossf/scorecard-action:v2.5.0@sha256:... is an unapproved step and leaves the API finding no Scorecard job at all, and it takes the image directly only by a tag, which pins nothing the action's commit does not. This action pinned by its commit is therefore the one form that publishes; it is also the form the action's README documents and the one Dependabot proposes a bump of, after the github-actions cooldown and with the tag the new commit names in the record's diff, while it does not update a docker:// step. The analysis job holds contents: read and id-token: write, runs with cache-mode: none, reads no secret and runs on pushes to main and on the weekly schedule only. The identity it can mint names scorecard.yml at refs/heads/main: npm, PyPI and nuget.org refuse it, since each trusted publisher names release.yml and the release environment, and so does every signature or attestation check this repository runs or documents, each of which names release.yml, every one it runs at a release tag; the MCP Registry accepts it, since its GitHub OIDC login grants io.github.jmrplens/* to a token from any workflow of any repository of the owner, which nothing on this side can narrow. The SARIF it writes reaches code scanning through a job of its own, which runs no code of the image and refuses a SARIF whose runs are not Scorecard's supply-chain category before upload-sarif reads it",
	},
}

// The reasons a script in another language than Go may be declared for. Each
// but the last is a reason it has to stay; the last is a reason it is still
// here, which leaves the table with the item of issue 1114 that ports it.
const (
	scriptRunsBeforeAnyBinary    = "runs-before-any-binary"
	scriptDistributionEntryPoint = "distribution-entry-point"
	scriptEvaluatedInGitLab      = "evaluated-in-gitlab"
	scriptJobGlue                = "job-glue"
	scriptFixtureContent         = "fixture-content"
	scriptPortPending            = "port-pending"
)

// scriptCategories are the reasons a declaration of a script may give, each
// with what it means.
var scriptCategories = map[string]string{
	scriptRunsBeforeAnyBinary:    "it runs on a machine before any binary of this repository is there, as the curl | sh and irm | iex installers do",
	scriptDistributionEntryPoint: "it is the entry point a distribution hands its own runtime, which no binary of ours can be",
	scriptEvaluatedInGitLab:      "it is evaluated inside the GitLab container by gitlab-rails runner, embedded by the Go command that runs it",
	scriptJobGlue:                "it is thin glue around another program's command line, run by a job where setting up Go would cost more than it saves or by a make target; the reason names the jobs and targets that run it",
	scriptFixtureContent:         "it is content pushed to an instance as a fixture, not tooling",
	scriptPortPending:            "an item of issue 1114 ports it into Go, and it leaves the table with that item",
}

// scriptDeclaration is why one script in another language than Go may be in
// the tree: a category, the reason in words, and for a port still pending the
// item of issue 1114 that ports it, whose line in [pendingPorts] is then the
// reason.
type scriptDeclaration struct {
	category string
	reason   string
	item     int
}

// pendingPorts are the items of issue 1114 that move a script of this tree
// into Go and have not landed, each with what the scripts become. An item no
// pending script names any longer has landed, and its line is reported until
// it goes.
var pendingPorts = map[int]string{
	4:  "one coverage command over cmd/internal/golist that stages the module copy and still drives gremlins and gobco",
	5:  "a command that tells a runtime crash from a failing test in a go test -json stream",
	6:  "a Go orchestrator for the Docker e2e runs under test/e2e/, the gitlab-rails snippets embedded",
	7:  "the Orbit fixture setup and indexer wait beside cmd/gen_orbit_record",
	8:  "the SonarCloud scan beside the coverage command",
	11: "the em dash and skip-command gate over the diff and the pull request description",
	12: "the documentation link check, sharing a walker with cmd/audit_doc_tool_names",
	13: "one command that builds and validates the npm, PyPI and NuGet packages and verifies what each registry published",
	14: "the release stamp beside cmd/gen_lhm_manifest, written byte for byte as jq writes it today",
	15: "Go tests that drive what stays through os/exec and httptest, or node:test inside site/ for the site's own steps",
}

// declaredScripts are the scripts in another language than Go this
// repository holds, keyed by their repository-relative path, or by a
// directory written with a trailing slash for every script below it. A
// script no entry declares is reported, which is how a new one fails the gate
// the day it is added, and so is an entry that names no script in the tree,
// on the terms every declaration table in this repository is held to.
var declaredScripts = map[string]scriptDeclaration{
	".github/scripts/cosign-sign-retry.sh": {
		category: scriptJobGlue,
		reason:   "retries cosign's keyless signing in release.yml's sign-attest job when the ambient OIDC endpoint answers with an error body, around the cosign the job installed",
	},
	".github/scripts/go-test-crash-aware.sh": {category: scriptPortPending, item: 5},
	".github/scripts/needs-verdict.sh": {
		category: scriptJobGlue,
		reason:   "reads the needs context in ci.yml's checks job and in its ci job, the required CI verdict, two sparse fan-ins that set up no toolchain",
	},
	".github/scripts/referrers-fallback-repair.sh": {
		category: scriptJobGlue,
		reason:   "rewrites ghcr.io's referrers fallback tags with crane and jq in release.yml's sign-attest job",
	},
	"cmd/gen_api_live/introspect.rb": {
		category: scriptEvaluatedInGitLab,
		reason:   "cmd/gen_api_live embeds it and hands it to gitlab-rails runner inside the booted GitLab, where Ruby is what runs",
	},
	"mcpb/linux/launch.sh": {
		category: scriptDistributionEntryPoint,
		reason:   "the Linux entry point of the .mcpb bundle: there is no fat Linux binary, so a native launcher could not pick its own architecture",
	},
	"npm/gitlab-mcp-server/cli.js": {
		category: scriptDistributionEntryPoint,
		reason:   "the npm package's bin, which npx runs with Node to choose the platform package's binary",
	},
	"scripts/build-npm.mjs":                         {category: scriptPortPending, item: 13},
	"scripts/build_nuget.py":                        {category: scriptPortPending, item: 13},
	"scripts/build_pypi.py":                         {category: scriptPortPending, item: 13},
	"scripts/check-doc-links.mjs":                   {category: scriptPortPending, item: 12},
	"scripts/check-em-dash.sh":                      {category: scriptPortPending, item: 11},
	"scripts/check-spec-conditions.sh":              {category: scriptPortPending, item: 4},
	"scripts/check_doc_links_test.py":               {category: scriptPortPending, item: 12},
	"scripts/check_em_dash_sh_test.py":              {category: scriptPortPending, item: 11},
	"scripts/check_spec_conditions_sh_test.py":      {category: scriptPortPending, item: 4},
	"scripts/coverage-conditions.sh":                {category: scriptPortPending, item: 4},
	"scripts/coverage-mutants.sh":                   {category: scriptPortPending, item: 4},
	"scripts/coverage_conditions_sh_test.py":        {category: scriptPortPending, item: 4},
	"scripts/coverage_mutants_sh_test.py":           {category: scriptPortPending, item: 4},
	"scripts/e2e_docker_images_test.py":             {category: scriptPortPending, item: 6},
	"scripts/e2e_fixture_addresses_test.py":         {category: scriptPortPending, item: 6},
	"scripts/e2e_gitlab_exceptions_test.py":         {category: scriptPortPending, item: 6},
	"scripts/e2e_release_gate_test.py":              {category: scriptPortPending, item: 6},
	"scripts/e2e_skip_gate_test.py":                 {category: scriptPortPending, item: 6},
	"scripts/ensure_playwright_chromium_sh_test.py": {category: scriptPortPending, item: 15},
	"scripts/fetch_release_assets_sh_test.py":       {category: scriptPortPending, item: 15},
	"scripts/install_sh_test.py":                    {category: scriptPortPending, item: 15},
	"scripts/mcpb_launch_sh_test.py":                {category: scriptPortPending, item: 15},
	"scripts/mcpb_manifest_test.py":                 {category: scriptPortPending, item: 15},
	"scripts/npm_cli_js_test.py":                    {category: scriptPortPending, item: 15},
	"scripts/packaging_checksums_test.py":           {category: scriptPortPending, item: 13},
	"scripts/publish_npm_sh_test.py":                {category: scriptPortPending, item: 15},
	"scripts/publish_pypi_sh_test.py":               {category: scriptPortPending, item: 15},
	"scripts/setup-orbit-fixtures.sh":               {category: scriptPortPending, item: 7},
	"scripts/setup_orbit_fixtures_sh_test.py":       {category: scriptPortPending, item: 7},
	"scripts/site_check_i18n_code_test.py":          {category: scriptPortPending, item: 15},
	"scripts/site_normalize_html_test.py":           {category: scriptPortPending, item: 15},
	"scripts/site_version_token_test.py":            {category: scriptPortPending, item: 15},
	"scripts/smoke_test_image_sh_test.py":           {category: scriptPortPending, item: 15},
	"scripts/sonar-scan.sh":                         {category: scriptPortPending, item: 8},
	"scripts/update-homebrew-tap.sh":                {category: scriptPortPending, item: 14},
	"scripts/update-server-json-sha.sh":             {category: scriptPortPending, item: 14},
	"scripts/update_homebrew_tap_sh_test.py":        {category: scriptPortPending, item: 14},
	"scripts/update_server_json_sha_sh_test.py":     {category: scriptPortPending, item: 14},
	"scripts/validate-npm.mjs":                      {category: scriptPortPending, item: 13},
	"scripts/validate_npm_test.py":                  {category: scriptPortPending, item: 13},
	"scripts/validate_nuget.py":                     {category: scriptPortPending, item: 13},
	"scripts/validate_nuget_test.py":                {category: scriptPortPending, item: 13},
	"scripts/validate_pypi.py":                      {category: scriptPortPending, item: 13},
	"scripts/validate_pypi_test.py":                 {category: scriptPortPending, item: 13},
	"scripts/verify_published_packages.py":          {category: scriptPortPending, item: 13},
	"scripts/verify_published_packages_test.py":     {category: scriptPortPending, item: 13},
	"scripts/ensure-playwright-chromium.sh": {
		category: scriptJobGlue,
		reason:   "installs the Chromium the site's build drives, in ci.yml's site jobs and pages.yml, which set up Node and pnpm and no Go",
	},
	"scripts/fetch-release-assets.sh": {
		category: scriptJobGlue,
		reason:   "downloads and verifies the release assets in each publisher job of release.yml, the step that checks what its job then runs",
	},
	"scripts/install.ps1": {
		category: scriptRunsBeforeAnyBinary,
		reason:   "the irm | iex installer, which fetches and verifies the release binary on a machine that holds none",
	},
	"scripts/install.sh": {
		category: scriptRunsBeforeAnyBinary,
		reason:   "the curl | sh installer, which fetches and verifies the release binary on a machine that holds none",
	},
	"scripts/publish-npm.sh": {
		category: scriptJobGlue,
		reason:   "wraps npm publish in release.yml's npm job and make publish-npm; item 13 rewires it to the package command",
	},
	"scripts/publish-nuget.sh": {
		category: scriptJobGlue,
		reason:   "wraps dotnet nuget push in release.yml's nuget job and make publish-nuget; item 13 rewires it to the package command",
	},
	"scripts/publish-pypi.sh": {
		category: scriptJobGlue,
		reason:   "wraps twine and pypi-attestations, installed from the hash-pinned .github/requirements/pypi-publish.txt, in release.yml's pypi job, which mints the upload token from the job's identity, and in make publish-pypi; item 13 rewires it to the package command",
	},
	"scripts/smoke-test-image.sh": {
		category: scriptJobGlue,
		reason:   "drives docker against the built images in ci.yml's and release.yml's docker jobs",
	},
	"test/e2e/scripts/audit-command.sh":              {category: scriptPortPending, item: 6},
	"test/e2e/scripts/docker-images.sh":              {category: scriptPortPending, item: 6},
	"test/e2e/scripts/enterprise-activation-code.sh": {category: scriptPortPending, item: 6},
	"test/e2e/scripts/fixture-addresses.sh":          {category: scriptPortPending, item: 6},
	"test/e2e/scripts/gitlab-exceptions.sh":          {category: scriptPortPending, item: 6},
	"test/e2e/scripts/register-runner.sh":            {category: scriptPortPending, item: 6},
	"test/e2e/scripts/release-gate.sh":               {category: scriptPortPending, item: 6},
	"test/e2e/scripts/run-docker-e2e.sh":             {category: scriptPortPending, item: 6},
	"test/e2e/scripts/setup-bitbucket.sh":            {category: scriptPortPending, item: 6},
	"test/e2e/scripts/setup-gitlab.sh":               {category: scriptPortPending, item: 6},
	"test/e2e/scripts/skip-gate.sh":                  {category: scriptPortPending, item: 6},
	"test/e2e/scripts/wait-for-gitlab.sh": {
		category: scriptJobGlue,
		reason:   "polls a GitLab container with curl until it answers, in ee-schema.yml's job and in run-docker-e2e.sh; issue 1114 keeps it as glue beside the e2e orchestration item 6 ports",
	},
	"test/fixtures/orbit/": {
		category: scriptFixtureContent,
		reason:   "Python the Orbit live tests push to GitLab.com for its Knowledge Graph to index, which is what those tests query",
	},
}

// guardKey is a declaration of a guarded match as its key spells it: the
// pinned action without its commit, the index of the step in its steps, and
// the rule's name.
type guardKey struct {
	action string
	rule   string
	step   int
}

// String is the key the declaration table is written in.
func (k guardKey) String() string {
	return fmt.Sprintf("%s step %d %s", k.action, k.step, k.rule)
}

// guardedAction is the spelling of an action a key of [declaredGuardedMatches]
// or [declaredTaggedImages] names: an owner and a repository, and a directory
// inside it when the action is not at its root, with no commit and nothing of
// this repository's own.
var guardedAction = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_./-]+)?$`)

// parseGuardKey reads a key of [declaredGuardedMatches], and reports false for
// one that is not a pinned action without its commit, the word step, a step's
// index written as Go prints it, and a rule's name, separated by single
// spaces.
func parseGuardKey(key string) (guardKey, bool) {
	fields := strings.Split(key, " ")
	if len(fields) != 4 || fields[1] != "step" || !guardedAction.MatchString(fields[0]) || !slices.Contains(ruleNames(), fields[3]) {
		return guardKey{}, false
	}
	step, err := strconv.Atoi(fields[2])
	if err != nil || step < 0 || strconv.Itoa(step) != fields[2] {
		return guardKey{}, false
	}
	return guardKey{action: fields[0], step: step, rule: fields[3]}, true
}

// sortedGuardKeys returns the keys of a guarded match table in a stable
// order.
func sortedGuardKeys(guarded map[string]guardedMatch) []string {
	keys := make([]string, 0, len(guarded))
	for key := range guarded {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// tableProblems holds every table to what this audit read, once every
// workflow has been read: the secret table first, then the declarations of
// run-time code, then those of unjudged actions, then those of guarded
// matches, then those of tag-named images.
func (a *supplyChainAudit) tableProblems() []string {
	problems := append(a.secretProblems(), a.declarationProblems()...)
	problems = append(problems, a.unjudgedDeclarationProblems()...)
	problems = append(problems, a.guardedDeclarationProblems()...)
	return append(problems, a.taggedImageDeclarationProblems()...)
}

// taggedImageDeclarationProblems names every declaration of a tag-named image
// that cannot be read as written, gives a category this command does not
// define for one or no reason, or excused nothing, in a stable order.
func (a *supplyChainAudit) taggedImageDeclarationProblems() []string {
	keys := make([]string, 0, len(a.taggedImages))
	for key := range a.taggedImages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		action, image, separated := strings.Cut(key, " ")
		if !separated || !guardedAction.MatchString(action) || !isUntaggedImage(image) {
			problems = append(problems, fmt.Sprintf(
				"%s: %q is not an action without its commit and a docker:// image without its tag or digest, separated by one space",
				declarationsFile, key,
			))
			continue
		}
		entry := a.taggedImages[key]
		if imageCategories[entry.category] == "" {
			problems = append(problems, fmt.Sprintf("%s: %q: the category %q is not one this command defines for a tag-named image",
				declarationsFile, key, entry.category))
		}
		if strings.TrimSpace(entry.reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
		}
		if !a.taggedImagesUsed[key] {
			problems = append(problems, fmt.Sprintf(
				"%s: %q excuses nothing: no step runs that action naming that image by a tag", declarationsFile, key,
			))
		}
	}
	return problems
}

// isUntaggedImage reports whether image is spelled as a declaration of a
// tag-named image names one: a docker:// reference with a name and no tag, no
// digest and no white space.
func isUntaggedImage(image string) bool {
	return untaggedImage(image) == image && strings.HasPrefix(image, dockerScheme) &&
		len(image) > len(dockerScheme) && !strings.ContainsFunc(image, unicode.IsSpace)
}

// guardedDeclarationProblems names every declaration of a guarded match that
// cannot be read as written, gives a category this command does not define
// for one, names no input or no value, gives no reason, or excused nothing, in
// a stable order.
func (a *supplyChainAudit) guardedDeclarationProblems() []string {
	var problems []string
	for _, key := range sortedGuardKeys(a.guarded) {
		if _, ok := parseGuardKey(key); !ok {
			problems = append(problems, fmt.Sprintf(
				`%s: %q is not an action without its commit, "step", a step's index and a rule name (%s) separated by single spaces`,
				declarationsFile, key, strings.Join(ruleNames(), ", "),
			))
			continue
		}
		entry := a.guarded[key]
		if guardCategories[entry.category] == "" {
			problems = append(problems, fmt.Sprintf("%s: %q: the category %q is not one this command defines for a guarded match",
				declarationsFile, key, entry.category))
		}
		if strings.TrimSpace(entry.input) == "" || strings.TrimSpace(entry.value) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q does not name the input and the value that run the branch it accepts",
				declarationsFile, key))
		}
		if strings.TrimSpace(entry.reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
		}
		if !a.guardedUsed[key] {
			problems = append(problems, fmt.Sprintf(
				"%s: %q excuses nothing: no credentialed job reaches that step of that action matching that rule", declarationsFile, key,
			))
		}
	}
	return problems
}

// unjudgedDeclarationProblems names every declaration of an unjudged action
// that cannot be read as written, gives a category this command does not
// define for one or no reason, or excused nothing, in a stable order.
func (a *supplyChainAudit) unjudgedDeclarationProblems() []string {
	keys := make([]string, 0, len(a.unjudged))
	for key := range a.unjudged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		writer, reference, separated := strings.Cut(key, " ")
		if !separated || writer == "" || !strings.HasPrefix(reference, "./") {
			problems = append(problems, fmt.Sprintf("%s: %q is not an action and a ./ reference separated by one space",
				declarationsFile, key))
			continue
		}
		entry := a.unjudged[key]
		if actionCategories[entry.category] == "" {
			problems = append(problems, fmt.Sprintf("%s: %q: the category %q is not one this command defines for an unjudged action",
				declarationsFile, key, entry.category))
		}
		if strings.TrimSpace(entry.reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
		}
		if !a.unjudgedUsed[key] {
			problems = append(problems, fmt.Sprintf(
				"%s: %q excuses nothing: no step reaches that reference from that action", declarationsFile, key,
			))
		}
	}
	return problems
}

// declarationProblems names every declaration that cannot be read as written,
// gives a category this command does not define or no reason, or excused
// nothing, in a stable order.
func (a *supplyChainAudit) declarationProblems() []string {
	keys := make([]string, 0, len(a.declarations))
	for key := range a.declarations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		subject, rule, separated := strings.Cut(key, " ")
		if !separated || subject == "" || !slices.Contains(ruleNames(), rule) {
			problems = append(problems, fmt.Sprintf("%s: %q is not a path and a rule name (%s) separated by one space",
				declarationsFile, key, strings.Join(ruleNames(), ", ")))
			continue
		}
		entry := a.declarations[key]
		if categories[entry.category] == "" {
			problems = append(problems, fmt.Sprintf("%s: %q: the category %q is not one this command defines",
				declarationsFile, key, entry.category))
		}
		if strings.TrimSpace(entry.reason) == "" {
			problems = append(problems, fmt.Sprintf("%s: %q gives no reason", declarationsFile, key))
		}
		if !a.excused[key] {
			problems = append(problems, fmt.Sprintf(
				"%s: %q excuses nothing: no credentialed job reaches that file matching that rule", declarationsFile, key,
			))
		}
	}
	return problems
}

// ruleNames are the names a declaration may give a rule, in the order the
// rules are applied.
func ruleNames() []string {
	names := make([]string, 0, len(unlockedCode))
	for _, rule := range unlockedCode {
		names = append(names, rule.name)
	}
	return names
}
