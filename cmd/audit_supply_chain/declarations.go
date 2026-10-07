package main

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
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
		reason: "stopUnderNpx starts the launcher the way a client configured with npx does, to prove a SIGTERM " +
			"sent to npx alone ends the server, and passes --offline in the arguments it spawns npx with, so npx " +
			"resolves the package from the project the job installed from the tarballs it just packed and cannot " +
			"fetch a published one; serverUnder then holds the process to that project's binary. The other " +
			"matches are comments and the child process variable named after the tool",
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
var declaredUnjudgedActions = map[string]declaration{
	"pypa/gh-action-pypi-publish ./.github/.tmp/.generated-actions/run-pypi-publish-in-docker-container": {
		category: categoryGeneratedAtRunTime,
		reason: "create-docker-action.py, which the record holds for the pinned commit, writes this directory as a " +
			"container action whose image is docker://ghcr.io/pypa/gh-action-pypi-publish:<the commit the workflow " +
			"pins>, the image pypa's own release workflow publishes for that commit. The image is named by a " +
			"registry tag, which a push to pypa's ghcr.io namespace can move and this audit cannot see, so the " +
			"pypi job trusts that namespace as far as it trusts the action's repository; replacing the action " +
			"with an upload of our own is the fix, which is issue 1243",
	},
}

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
		reason: "the step's script runs go install github.com/sigstore/cosign/v3/cmd/cosign@main only inside " +
			"if [[ \"${input_cosign_release}\" == \"main\" ]], which builds cosign from its main branch and exits; " +
			"any other release is a bootstrap binary held to a SHA-256 the action commits, which then verifies " +
			"the requested release against a public key whose SHA-256 the action commits too. Every step here " +
			"names an exact cosign-release",
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

// guardedAction is the spelling of an action a guarded key names: an owner
// and a repository, and a directory inside it when the action is not at its
// root, with no commit and nothing of this repository's own.
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
// matches.
func (a *supplyChainAudit) tableProblems() []string {
	problems := append(a.secretProblems(), a.declarationProblems()...)
	problems = append(problems, a.unjudgedDeclarationProblems()...)
	return append(problems, a.guardedDeclarationProblems()...)
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
