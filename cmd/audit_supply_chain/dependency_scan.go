// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/sourcewalk"
)

// dependencyScanTarget is the make target that holds every lockfile of the
// tree to OSV.dev's advisories, malicious packages included, which is what
// SECURITY.md promises blocks a change (Dependencies).
const dependencyScanTarget = "check-dependency-vulns"

// osvImageVariable is the Makefile variable that names the OSV-Scanner image
// the target runs.
const osvImageVariable = "OSV_SCANNER_IMAGE"

// osvConfigName is the file OSV-Scanner reads beside each lockfile it scans
// for the advisories it may set aside there, and OpenSSF Scorecard's
// Vulnerabilities check reads it the same way.
const osvConfigName = "osv-scanner.toml"

// ciWorkflow is the workflow whose verdict job branch protection requires.
const ciWorkflow = workflowDir + "/ci.yml"

// requiredVerdictJob is the job of ci.yml that branch protection requires,
// "✅ CI": a check decides whether a change merges only when this job needs
// it, directly or through jobs it needs, and counts its result.
const requiredVerdictJob = "ci"

// maxReviewDays is the furthest ahead, in days from the run, an entry's
// ignoreUntil may lie: six months, after which the entry is reviewed again.
// The bound only ever relaxes as the days pass, so an entry that met it when
// it was written never fails it later; the day itself is OSV-Scanner's to
// enforce, which stops honoring the entry on it.
const maxReviewDays = 183

// verdictExemptions are the variables a verdict job hands
// .github/scripts/needs-verdict.sh to name the jobs whose failure, or whose
// skip, does not fail the verdict.
var verdictExemptions = []string{"INFORMATIONAL", "MAY_SKIP"}

// notAffectedJustifications are the reasons an advisory may be set aside for,
// the five OpenVEX gives a not_affected statement, each a claim a reviewer can
// check: the vulnerable component is not present where it was found, the
// vulnerable code is not, it is never executed, no adversary can control the
// input that reaches it, or a mitigation already stands in front of it. An
// entry's reason opens with one of them, a colon and a space.
var notAffectedJustifications = []string{
	"component_not_present",
	"inline_mitigations_already_exist",
	"vulnerable_code_cannot_be_controlled_by_adversary",
	"vulnerable_code_not_in_execute_path",
	"vulnerable_code_not_present",
}

// osvConfigKeys are the keys of an [[IgnoredVulns]] table this audit reads,
// and every one an entry has to give.
var osvConfigKeys = []string{"id", "ignoreUntil", "reason"}

// The shapes the audit reads an osv-scanner.toml in: the one table header it
// accepts, a key and its value on one line, a double-quoted string with no
// escape in it, a bare TOML local date, and an advisory id the way OSV
// spells one (GO-2026-5932, GHSA-xxxx-xxxx-xxxx, PYSEC-2024-1, MAL-2025-1).
var (
	osvConfigTable    = regexp.MustCompile(`^\[\[IgnoredVulns\]\]$`)
	osvConfigKeyValue = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*(.*)$`)
	osvConfigString   = regexp.MustCompile(`^"([^"\\]*)"$`)
	osvConfigDate     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	osvAdvisoryID     = regexp.MustCompile(`^[A-Z][A-Z0-9]*(-[0-9A-Za-z]+)+$`)
)

// osvImageAssignment finds the Makefile's assignment of the scanner image,
// in any of make's assignment operators.
var osvImageAssignment = regexp.MustCompile(`(?m)^` + osvImageVariable + `[ \t]*(?:::?|\?|\+)?=[ \t]*(.*?)[ \t]*$`)

// pinnedOSVImage is an image named by the sha256 digest of its manifest, a
// tag before the digest allowed, as rule 1 reads a docker:// image.
var pinnedOSVImage = regexp.MustCompile(`^[^@\s]+@sha256:[0-9a-f]{64}$`)

// unfilteredConfig is the one --config the recipe may pass: a file that sets
// nothing aside, for the pass that asks about malicious packages with no allow
// list in the way. OSV-Scanner groups an advisory with its aliases, and an
// entry naming any id of the group sets the whole group aside, so a MAL- entry
// is set aside by an entry naming its GHSA or CVE alias, and an advisory
// GitHub classifies as embedded malware (CWE-506) need have no MAL- alias at
// all. No rule over the allow lists can see either; a pass that honors none
// does.
const unfilteredConfig = "/dev/null"

// osvConfigFlag finds each --config the recipe passes and the word it names,
// spelled with a space or an equals sign.
var osvConfigFlag = regexp.MustCompile(`--config(?:=|[ \t]+)(\S*)`)

// readConfig is a seam over readTextFile for the osv-scanner.toml files, so a
// test can drive the branch a file the tests own never produces as root: a
// regular file that cannot be read.
var readConfig = readTextFile

// checkDependencyScan holds the dependency scan to what SECURITY.md promises
// of it: the Makefile runs OSV-Scanner pinned by digest over the allow list
// beside each lockfile, the required check of ci.yml cannot pass without that
// target having passed, and every allow list sets an advisory aside only as
// the policy allows.
func checkDependencyScan(root string) ([]string, error) {
	workflows, err := loadWorkflows(root)
	if err != nil {
		return nil, err
	}
	makefile, err := readTextFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil, err
	}
	problems := makefileScanProblems(makefile)
	problems = append(problems, workflowScanProblems(workflows)...)
	now := time.Now().UTC()
	for _, config := range osvConfigsInTree(root) {
		text, readErr := readConfig(filepath.Join(root, filepath.FromSlash(config)))
		if readErr != nil {
			return nil, readErr
		}
		problems = append(problems, osvConfigProblems(config, text, now)...)
	}
	return problems, nil
}

// makefileScanProblems holds the Makefile's dependency scan to running the
// scanner image it pins by digest, to reading the allow list beside each
// lockfile rather than one file a --config would put in all their places, and
// to a second pass under --config /dev/null, which no allow list narrows.
func makefileScanProblems(makefile string) []string {
	var problems []string
	assignments := osvImageAssignment.FindAllStringSubmatch(stripComments(makefile), -1)
	switch {
	case len(assignments) == 0:
		problems = append(problems, fmt.Sprintf("Makefile: %s is not assigned, so make %s names no scanner image this audit can hold to a digest",
			osvImageVariable, dependencyScanTarget))
	case len(assignments) > 1:
		problems = append(problems, fmt.Sprintf("Makefile: %s is assigned %d times, so which image make %s runs depends on the order make reads them in",
			osvImageVariable, len(assignments), dependencyScanTarget))
	case !pinnedOSVImage.MatchString(assignments[0][1]):
		problems = append(problems, fmt.Sprintf("Makefile: %s=%s is not pinned to a sha256 image digest, so the scanner that decides is whatever the tag names when the job runs",
			osvImageVariable, assignments[0][1]))
	}
	rule := parseMakefile(makefile)[dependencyScanTarget]
	if rule == nil {
		return append(problems, fmt.Sprintf("Makefile: no rule makes %s, so nothing holds the lockfiles to the dependency advisories", dependencyScanTarget))
	}
	recipe := strings.Join(rule.recipe, "\n")
	if !strings.Contains(recipe, "$("+osvImageVariable+")") {
		problems = append(problems, fmt.Sprintf("Makefile: the recipe of %s does not run $(%s), so the image pinned above is not the scanner that decides",
			dependencyScanTarget, osvImageVariable))
	}
	unfiltered := 0
	for _, flag := range osvConfigFlag.FindAllStringSubmatch(recipe, -1) {
		if flag[1] == unfilteredConfig {
			unfiltered++
		}
	}
	if strings.Count(recipe, "--config") > unfiltered {
		problems = append(problems, fmt.Sprintf("Makefile: the recipe of %s passes --config naming something other than %s, which puts one file in place of the %s beside each lockfile, the allow lists this audit judges",
			dependencyScanTarget, unfilteredConfig, osvConfigName))
	}
	if unfiltered == 0 {
		problems = append(problems, fmt.Sprintf("Makefile: the recipe of %s has no pass under --config %s, the one no allow list narrows, so a malicious package an entry sets aside by one of its aliases passes the scan",
			dependencyScanTarget, unfilteredConfig))
	}
	return problems
}

// scanCommand is the whole run block of the step that makes the scan, its
// comments aside: anything written beside it, a variable assignment, an
// option, a || or a pipe, can change which scanner runs or let its failure
// pass, and none of those is a make invocation this audit could refuse by its
// targets alone.
const scanCommand = "make " + dependencyScanTarget

// scanEnvironment are the variables a workflow, the scan's job or its step may
// not set, each read by make or by the step's shell before the recipe runs:
// make takes options and assignments from MAKEFLAGS and GNUMAKEFLAGS and
// extra makefiles from MAKEFILES, and bash sources BASH_ENV when it starts. A
// variable named after one of the target's own (scanVariablePrefix) is
// refused with them, since make -e would let it replace the Makefile's.
var scanEnvironment = []string{"BASH_ENV", "GNUMAKEFLAGS", "MAKEFILES", "MAKEFLAGS", osvImageVariable}

// scanVariablePrefix opens the name of every other Makefile variable the scan
// target reads.
const scanVariablePrefix = "DEPENDENCY_VULNS_"

// workflowScanProblems holds ci.yml to running the dependency scan in a job
// the required check counts: one that runs make check-dependency-vulns from
// the root Makefile as the whole of a step, under no condition, no shell of
// its own and no variable make or that shell reads, after no step but the
// checkout, and that the verdict job reaches through needs no verdict on the
// way excuses.
func workflowScanProblems(workflows []workflow) []string {
	index := slices.IndexFunc(workflows, func(file workflow) bool { return file.path == ciWorkflow && file.doc != nil })
	if index < 0 {
		return []string{fmt.Sprintf("%s: the workflow is missing or is not a mapping, so nothing shows the required check depends on make %s",
			ciWorkflow, dependencyScanTarget)}
	}
	file := workflows[index]
	jobs, _ := file.doc["jobs"].(map[string]any)
	var problems []string
	scanners := 0
	for _, id := range orderedKeys(jobs, file.jobOrder) {
		job, _ := jobs[id].(map[string]any)
		positions := scanSteps(file.doc, job)
		if len(positions) == 0 {
			continue
		}
		scanners++
		where := fmt.Sprintf("%s: job %s runs make %s", ciWorkflow, id, dependencyScanTarget)
		all, _ := job["steps"].([]any)
		steps := make([]map[string]any, 0, len(positions))
		for _, position := range positions {
			step, _ := all[position].(map[string]any)
			steps = append(steps, step)
			if run := strings.TrimSpace(stripComments(inputText(step["run"]))); run != scanCommand {
				problems = append(problems, fmt.Sprintf("%s in a run block that is not %q alone (%q), so what is written beside it can change which scanner runs or let its failure pass",
					where, scanCommand, run))
			}
		}
		if conditional := conditionalKeys(job, steps); len(conditional) > 0 {
			problems = append(problems, where+" under "+strings.Join(conditional, " and ")+
				", so the required check can pass on a run where the scan did not")
		}
		if shells := shellKeys(file.doc, job, steps); len(shells) > 0 {
			problems = append(problems, where+" under "+strings.Join(shells, " and ")+
				", a shell of its own that decides what the run block does, where the runner's default fails the step with make")
		}
		problems = append(problems, scanEnvironmentProblems(where, file.doc, job, steps)...)
		for position, raw := range all[:positions[0]] {
			if step, _ := raw.(map[string]any); !isCheckoutStep(step) {
				problems = append(problems, fmt.Sprintf("%s after step %d, which is not %s: a step before the scan can rewrite the tree it reads, $GITHUB_ENV or $GITHUB_PATH",
					where, position+1, checkoutAction))
			}
		}
		if excuse := verdictPath(jobs, requiredVerdictJob, id); excuse != "" {
			problems = append(problems, where+", and "+excuse)
		}
	}
	if scanners == 0 {
		problems = append(problems, fmt.Sprintf("%s: no job runs make %s, so no change is held to the dependency advisories before it merges",
			ciWorkflow, dependencyScanTarget))
	}
	return problems
}

// scanSteps returns the positions, among a job's steps, of the steps whose run
// block makes the dependency scan target from the root Makefile.
func scanSteps(doc, job map[string]any) []int {
	steps, _ := job["steps"].([]any)
	var found []int
	for position, raw := range steps {
		step, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		invocations, _ := scanMake(stripComments(inputText(step["run"])))
		for _, invocation := range invocations {
			if invocation.refusal == "" && slices.Contains(invocation.targets, dependencyScanTarget) &&
				invocation.run(runDirectory(doc, job, step)).makefile == "Makefile" {
				found = append(found, position)
				break
			}
		}
	}
	return found
}

// shellKeys names where a shell other than the runner's default is chosen for
// the scan's steps: a shell: on a step, or a defaults.run.shell on the job or
// the workflow. A shell is a command line the run block is handed to, so one
// that never runs it passes the step.
func shellKeys(doc, job map[string]any, steps []map[string]any) []string {
	var keys []string
	if slices.ContainsFunc(steps, func(step map[string]any) bool { _, set := step["shell"]; return set }) {
		keys = append(keys, "shell:")
	}
	for _, holder := range []scanSetting{{"the job's", job}, {"the workflow's", doc}} {
		defaults, _ := holder.fields["defaults"].(map[string]any)
		run, _ := defaults["run"].(map[string]any)
		if _, set := run["shell"]; set {
			keys = append(keys, holder.name+" defaults.run.shell")
		}
	}
	return keys
}

// scanSetting is one level of the workflow a setting of the scan's step can
// come from, named as a finding names it: the workflow, the job or the step.
type scanSetting struct {
	name   string
	fields map[string]any
}

// scanEnvironmentProblems reports every variable of scanEnvironment, or named
// with scanVariablePrefix, that the workflow, the job or one of the scan's
// steps sets in its env.
func scanEnvironmentProblems(where string, doc, job map[string]any, steps []map[string]any) []string {
	holders := []scanSetting{{"the workflow's", doc}, {"the job's", job}}
	for _, step := range steps {
		holders = append(holders, scanSetting{"the step's", step})
	}
	var problems []string
	for _, holder := range holders {
		env, _ := holder.fields["env"].(map[string]any)
		names := make([]string, 0, len(env))
		for name := range env {
			if slices.Contains(scanEnvironment, name) || strings.HasPrefix(name, scanVariablePrefix) {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			problems = append(problems, fmt.Sprintf("%s with %s set in %s env, which make or the step's shell reads, so the scan that runs need not be the one the Makefile pins",
				where, name, holder.name))
		}
	}
	return problems
}

// isCheckoutStep reports whether a step only checks out the tree the scan
// reads: it runs actions/checkout, at whatever pin rule 1 holds it to, and no
// run block. Any other step before the scan could rewrite that tree,
// $GITHUB_ENV or $GITHUB_PATH before make runs.
func isCheckoutStep(step map[string]any) bool {
	_, runs := step["run"]
	name, _, _ := strings.Cut(inputText(step["uses"]), "@")
	return !runs && name == checkoutAction
}

// conditionalKeys names the keys that let the scan's job or one of its steps
// be left out or fail without failing the job: an if on either, and
// continue-on-error on either.
func conditionalKeys(job map[string]any, steps []map[string]any) []string {
	var keys []string
	holders := append([]map[string]any{job}, steps...)
	for _, key := range []string{"if", "continue-on-error"} {
		for _, holder := range holders {
			if _, present := holder[key]; present {
				keys = append(keys, key+":")
				break
			}
		}
	}
	return keys
}

// verdictPath says why the verdict job does not count the target job, or
// nothing when it does: the verdict job is missing, or no chain of needs from
// it reaches the target without passing a job that names the next one among
// the jobs whose failure or skip its verdict excuses.
func verdictPath(jobs map[string]any, verdict, target string) string {
	if _, ok := jobs[verdict].(map[string]any); !ok {
		return fmt.Sprintf("there is no job %s, the required check, to count it", verdict)
	}
	seen := map[string]bool{verdict: true}
	queue := []string{verdict}
	excused := false
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		job, _ := jobs[current].(map[string]any)
		exempt := verdictExempt(job)
		for _, needed := range jobNeeds(job) {
			if exempt[needed] {
				excused = excused || needed == target
				continue
			}
			if needed == target {
				return ""
			}
			if !seen[needed] {
				seen[needed] = true
				queue = append(queue, needed)
			}
		}
	}
	if excused {
		return fmt.Sprintf("job %s, the required check, reaches it only through a verdict that excuses its failure or its skip (%s)",
			verdict, strings.Join(verdictExemptions, " or "))
	}
	return fmt.Sprintf("job %s, the required check, does not need it, directly or through the jobs it needs", verdict)
}

// verdictExempt returns the jobs a job's verdict excuses: every name the
// variables of verdictExemptions list, in the job's env or in any step's.
func verdictExempt(job map[string]any) map[string]bool {
	exempt := map[string]bool{}
	holders := []map[string]any{job}
	steps, _ := job["steps"].([]any)
	for _, raw := range steps {
		if step, ok := raw.(map[string]any); ok {
			holders = append(holders, step)
		}
	}
	for _, holder := range holders {
		env, _ := holder["env"].(map[string]any)
		for _, variable := range verdictExemptions {
			for name := range strings.FieldsSeq(inputText(env[variable])) {
				exempt[name] = true
			}
		}
	}
	return exempt
}

// configUnreadDirectories are the directories below the root the allow list
// walk does not read, by their repository-relative path: the untracked plan/
// working area, which .gitignore keeps out of the scan, and the worktrees
// Claude Code checks out under .claude, which the recipe's --x-exclude does.
var configUnreadDirectories = map[string]bool{"plan": true, ".claude/worktrees": true}

// configUnreadNames are the directories the allow list walk does not read
// wherever they sit: git's own, which the scan skips, and a package manager's
// install and a build's output, which .gitignore keeps out of it.
var configUnreadNames = map[string]bool{".git": true, "node_modules": true, "dist": true}

// osvConfigsInTree returns, sorted and repository-relative, every
// osv-scanner.toml the scan could read. The walk reads every directory but
// the few it names and another checkout, a dot-directory's included: the
// scan leaves a directory out only for .gitignore or an --x-exclude, never
// for a leading dot, so a .devcontainer/osv-scanner.toml sets aside what it
// names beside a lockfile there, and a walk that took the source rule's
// view of dot-directories would never read it.
func osvConfigsInTree(root string) []string {
	var configs []string
	osvConfigsBelow(root, "", &configs)
	sort.Strings(configs)
	return configs
}

// osvConfigsBelow appends every osv-scanner.toml in the repository-relative
// directory dir of root, and below it.
func osvConfigsBelow(root, dir string, configs *[]string) {
	entries, _ := os.ReadDir(filepath.Join(root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		relative := path.Join(dir, entry.Name())
		full := filepath.Join(root, filepath.FromSlash(relative))
		if entry.IsDir() {
			if !configUnreadDirectories[relative] && !configUnreadNames[entry.Name()] && !sourcewalk.IsNestedCheckout(full) {
				osvConfigsBelow(root, relative, configs)
			}
			continue
		}
		if entry.Type().IsRegular() && entry.Name() == osvConfigName {
			*configs = append(*configs, relative)
		}
	}
}

// osvIgnore is one [[IgnoredVulns]] table: the line its header sits on, and
// each key it gives with its value and the value's line.
type osvIgnore struct {
	values map[string]string
	lines  map[string]int
	line   int
}

// osvConfigProblems reads one osv-scanner.toml in the one shape this audit
// accepts and holds every entry to the policy: an advisory id that is not a
// malicious package's, a reason opening with a not_affected justification and
// saying why it holds, and an ignoreUntil at most maxReviewDays after now.
// Anything the file holds outside that shape is a finding rather than a guess
// at what OSV-Scanner would make of it, since what it makes of it is what is
// set aside.
func osvConfigProblems(config, text string, now time.Time) []string {
	entries, problems := parseOSVConfig(config, text)
	limit := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, maxReviewDays)
	first := map[string]int{}
	for _, entry := range entries {
		where := fmt.Sprintf("%s:%d", config, entry.line)
		var missing []string
		for _, key := range osvConfigKeys {
			if _, ok := entry.values[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("%s: the [[IgnoredVulns]] table gives no %s, and every entry names the advisory, the reason it is not exploitable and the day it is reviewed again",
				where, strings.Join(missing, " and no ")))
		}
		if id, ok := entry.values["id"]; ok {
			problems = append(problems, osvIDProblems(config, entry.lines["id"], id, first)...)
		}
		if reason, ok := entry.values["reason"]; ok {
			problems = append(problems, osvReasonProblems(config, entry.lines["reason"], reason)...)
		}
		if until, ok := entry.values["ignoreUntil"]; ok {
			problems = append(problems, osvUntilProblems(config, entry.lines["ignoreUntil"], until, limit)...)
		}
	}
	return problems
}

// parseOSVConfig reads the entries of one osv-scanner.toml, and a finding for
// every line outside the shape the audit reads: comments and blank lines,
// [[IgnoredVulns]] headers, and under each one key per line, id and reason as
// a double-quoted string without escapes and ignoreUntil as a bare date.
func parseOSVConfig(config, text string) (entries []*osvIgnore, problems []string) {
	for index, raw := range splitLines(text) {
		number := index + 1
		where := fmt.Sprintf("%s:%d", config, number)
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if osvConfigTable.MatchString(line) {
			entries = append(entries, &osvIgnore{line: number, values: map[string]string{}, lines: map[string]int{}})
			continue
		}
		groups := osvConfigKeyValue.FindStringSubmatch(line)
		if groups == nil {
			problems = append(problems, fmt.Sprintf("%s: %q is not a line this audit reads: an %s here holds comments and [[IgnoredVulns]] tables of id, ignoreUntil and reason, one key a line",
				where, line, osvConfigName))
			continue
		}
		key, value := groups[1], groups[2]
		if problem := osvValueProblem(key, value, entries); problem != "" {
			problems = append(problems, where+": "+problem)
			continue
		}
		entry := entries[len(entries)-1]
		if quoted := osvConfigString.FindStringSubmatch(value); quoted != nil {
			value = quoted[1]
		}
		entry.values[key] = value
		entry.lines[key] = number
	}
	return entries, problems
}

// osvValueProblem is what is wrong with a key and its value as written, or
// nothing: a key outside a table, one this audit does not read, one the table
// gives twice, or a value not in the one spelling the audit reads for it.
func osvValueProblem(key, value string, entries []*osvIgnore) string {
	if len(entries) == 0 {
		return key + " sits outside an [[IgnoredVulns]] table, which is the only kind of entry this audit reads"
	}
	if !slices.Contains(osvConfigKeys, key) {
		return key + " is not a key this audit reads: an entry gives id, ignoreUntil and reason and nothing else, so that what it sets aside is what a reviewer reads"
	}
	if _, given := entries[len(entries)-1].values[key]; given {
		return fmt.Sprintf("the [[IgnoredVulns]] table gives %s twice", key)
	}
	if key == "ignoreUntil" {
		if !osvConfigDate.MatchString(value) {
			return fmt.Sprintf("ignoreUntil = %s is not a bare date, YYYY-MM-DD", value)
		}
		return ""
	}
	if !osvConfigString.MatchString(value) {
		return fmt.Sprintf("%s = %s is not a double-quoted string on one line with no escape in it", key, value)
	}
	return ""
}

// osvIDProblems holds an entry's advisory id to the way OSV spells one, to
// not being a malicious package's, and to being set aside once per file;
// first records the line each id was first given on.
func osvIDProblems(config string, line int, id string, first map[string]int) []string {
	where := fmt.Sprintf("%s:%d", config, line)
	if !osvAdvisoryID.MatchString(id) {
		return []string{fmt.Sprintf("%s: %q is not an advisory id the way OSV spells one", where, id)}
	}
	if strings.HasPrefix(id, "MAL-") {
		return []string{fmt.Sprintf("%s: %s is a malicious-package advisory, which is never set aside: the package is removed and the lockfile regenerated", where, id)}
	}
	if earlier, seen := first[id]; seen {
		return []string{fmt.Sprintf("%s: %s is set aside again, first on line %d", where, id, earlier)}
	}
	first[id] = line
	return nil
}

// osvReasonProblems holds an entry's reason to opening with a not_affected
// justification, a colon and a space, followed by why it holds.
func osvReasonProblems(config string, line int, reason string) []string {
	justification, why, found := strings.Cut(reason, ": ")
	if !found || !slices.Contains(notAffectedJustifications, justification) {
		return []string{fmt.Sprintf("%s:%d: the reason does not open with one of the OpenVEX justifications for not_affected (%s) and a colon, so it does not say why the advisory is not exploitable here",
			config, line, strings.Join(notAffectedJustifications, ", "))}
	}
	if strings.TrimSpace(why) == "" {
		return []string{fmt.Sprintf("%s:%d: the reason names %s and says nothing after it, so a reviewer has no claim to check", config, line, justification)}
	}
	return nil
}

// osvUntilProblems holds an entry's ignoreUntil to being a day of the
// calendar no later than limit.
func osvUntilProblems(config string, line int, until string, limit time.Time) []string {
	where := fmt.Sprintf("%s:%d", config, line)
	day, err := time.Parse(time.DateOnly, until)
	if err != nil {
		return []string{fmt.Sprintf("%s: ignoreUntil = %s is not a day of the calendar", where, until)}
	}
	if day.After(limit) {
		return []string{fmt.Sprintf("%s: ignoreUntil = %s is more than %d days ahead, and an entry is reviewed again at least every six months (the latest day today is %s)",
			where, until, maxReviewDays, limit.Format(time.DateOnly))}
	}
	return nil
}
