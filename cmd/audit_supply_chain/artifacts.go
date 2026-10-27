// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// The actions an artifact is handed between the jobs of a run with, and the
// name upload-artifact gives one that names none.
const (
	downloadArtifactAction = "actions/download-artifact"
	uploadArtifactAction   = "actions/upload-artifact"
	checkoutAction         = "actions/checkout"
	defaultArtifactName    = "artifact"
)

// otherRunInputs are the inputs of actions/download-artifact that take an
// artifact from a run other than the one the job is in, in the order a finding
// names them.
var otherRunInputs = []string{"github-token", "repository", "run-id"}

// stepOutput is the value of a job output a step computes, the step's id in
// group 1 and the output's key in group 2.
var stepOutput = regexp.MustCompile(`^\$\{\{\s*steps\.([A-Za-z0-9_-]+)\.outputs\.([A-Za-z0-9_-]+)\s*\}\}$`)

// thisRunsCommit is the one ref a checkout may name and still build the
// commit the run is for.
var thisRunsCommit = regexp.MustCompile(`^\$\{\{\s*github\.sha\s*\}\}$`)

// sha256Tool is a command that computes or checks a SHA-256 digest.
var sha256Tool = regexp.MustCompile(`\bsha256sum\b|\bshasum[ \t]+-a[ \t]*256\b`)

// checkOption is the option that makes sha256sum or shasum check digests
// rather than compute them.
var checkOption = regexp.MustCompile(`(?:^|[ \t])(?:-c|--check)(?:[ \t]|$)`)

// toolArtifactProblems holds a credentialed job to how it takes a tool another
// job of the workflow built with go: every download of an artifact such a job
// uploads is from this run, by a job that needs the builder, and is followed
// at once by a step that checks it with sha256sum against an output the
// builder computed with sha256sum; the builder checks out this run's commit.
//
// A tool is recognized by its builder: a job whose own run blocks hold a go
// build or a go install and whose steps upload the artifact the download
// takes, by name, by pattern, or with neither, every artifact of the run.
func (a *supplyChainAudit) toolArtifactProblems(pathLabel, jobID string, job, jobs map[string]any, order []string) []string {
	steps, _ := job["steps"].([]any)
	var problems []string
	for index, raw := range steps {
		step, _ := raw.(map[string]any)
		uses, _ := step["uses"].(string)
		if name, _, _ := strings.Cut(uses, "@"); name != downloadArtifactAction {
			continue
		}
		with, _ := step["with"].(map[string]any)
		for _, builder := range toolBuilders(jobs, order, jobID, with) {
			builderJob, _ := jobs[builder].(map[string]any)
			where := fmt.Sprintf("%s: job %s: step %d downloads what job %s builds with go", pathLabel, jobID, index, builder)
			var other []string
			for _, input := range otherRunInputs {
				if _, set := with[input]; set {
					other = append(other, input)
				}
			}
			if len(other) > 0 {
				problems = append(problems, where+" from another run ("+strings.Join(other, ", ")+"), so it is not the build of this run's commit")
			}
			if !slices.Contains(jobNeeds(job), builder) {
				problems = append(problems, where+" without needing that job, so the SHA-256 that job computed cannot be read from its outputs")
			}
			problems = append(problems, a.builderProblems(pathLabel, builder, builderJob)...)
			if digests := digestOutputs(builderJob); len(digests) > 0 && !checksNext(steps, index, job, builder, digests) {
				problems = append(problems, where+", and the step after it does not check it with sha256sum against that job's "+
					strings.Join(digests, " or ")+" output, so whatever the artifact holds is what runs")
			}
		}
	}
	return problems
}

// builderProblems reports, once however many jobs take its tool, a builder
// that checks out anything but this run's commit or hands its tool over with
// no output a step of it computed with sha256sum.
func (a *supplyChainAudit) builderProblems(pathLabel, builder string, job map[string]any) []string {
	var problems []string
	steps, _ := job["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		uses, _ := step["uses"].(string)
		if name, _, _ := strings.Cut(uses, "@"); name != checkoutAction {
			continue
		}
		with, _ := step["with"].(map[string]any)
		for _, input := range []string{"ref", "repository"} {
			value := inputText(with[input])
			if value == "" || (input == "ref" && thisRunsCommit.MatchString(value)) {
				continue
			}
			message := fmt.Sprintf("%s: job %s builds with go a tool it hands over from a checkout of %s %s, not of this run's commit",
				pathLabel, builder, input, value)
			problems = append(problems, a.once("checkout "+pathLabel+" "+builder+" "+input+" "+value, message)...)
		}
	}
	if len(digestOutputs(job)) == 0 {
		message := fmt.Sprintf("%s: job %s builds with go a tool it hands over with no output a step of it computed with sha256sum, "+
			"so the job running it has nothing to check it against", pathLabel, builder)
		problems = append(problems, a.once("digest "+pathLabel+" "+builder, message)...)
	}
	return problems
}

// toolBuilders returns, in document order, every job of the workflow other
// than consumer that builds with go in its own run blocks and uploads an
// artifact a download with the inputs with takes.
func toolBuilders(jobs map[string]any, order []string, consumer string, with map[string]any) []string {
	var builders []string
	for _, id := range orderedKeys(jobs, order) {
		job, _ := jobs[id].(map[string]any)
		if id == consumer || !buildsWithGo(job) {
			continue
		}
		if slices.ContainsFunc(uploadedNames(job), func(name string) bool { return takesArtifact(with, name) }) {
			builders = append(builders, id)
		}
	}
	return builders
}

// buildsWithGo reports whether a job's own run blocks hold a go build or a go
// install.
func buildsWithGo(job map[string]any) bool {
	steps, _ := job["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		run, _ := step["run"].(string)
		invocations, _ := scanGo(stripComments(run))
		for _, invocation := range invocations {
			if invocation.subcommand == "build" || invocation.subcommand == "install" {
				return true
			}
		}
	}
	return false
}

// uploadedNames returns the names of the artifacts a job's steps upload.
func uploadedNames(job map[string]any) []string {
	var names []string
	steps, _ := job["steps"].([]any)
	for _, raw := range steps {
		step, _ := raw.(map[string]any)
		uses, _ := step["uses"].(string)
		if name, _, _ := strings.Cut(uses, "@"); name != uploadArtifactAction {
			continue
		}
		with, _ := step["with"].(map[string]any)
		name := inputText(with["name"])
		if name == "" {
			name = defaultArtifactName
		}
		names = append(names, name)
	}
	return names
}

// takesArtifact reports whether a download with the inputs with takes the
// artifact uploaded under name: by its name, by a pattern matching it, or with
// neither, every artifact of the run. A name or pattern only the run knows may
// take any of them.
func takesArtifact(with map[string]any, uploaded string) bool {
	name, pattern := inputText(with["name"]), inputText(with["pattern"])
	if strings.Contains(name+pattern, "${{") {
		return true
	}
	if name != "" {
		return name == uploaded
	}
	if pattern != "" {
		matched, _ := path.Match(pattern, uploaded)
		return matched
	}
	return true
}

// jobNeeds returns the jobs a job needs, written as one name or as a list.
func jobNeeds(job map[string]any) []string {
	if single, ok := job["needs"].(string); ok {
		return []string{single}
	}
	listed, _ := job["needs"].([]any)
	needs := make([]string, 0, len(listed))
	for _, raw := range listed {
		needs = append(needs, inputText(raw))
	}
	return needs
}

// digestOutputs returns, sorted, the outputs of a job whose value is an
// output of one of its steps that computes a digest with sha256sum and writes
// that output's key.
func digestOutputs(job map[string]any) []string {
	outputs, _ := job["outputs"].(map[string]any)
	steps, _ := job["steps"].([]any)
	var digests []string
	for name, raw := range outputs {
		match := stepOutput.FindStringSubmatch(strings.TrimSpace(inputText(raw)))
		if match == nil {
			continue
		}
		for _, rawStep := range steps {
			step, _ := rawStep.(map[string]any)
			run := stripComments(inputText(step["run"]))
			if step["id"] == match[1] && sha256Tool.MatchString(run) && strings.Contains(run, match[2]+"=") {
				digests = append(digests, name)
			}
		}
	}
	sort.Strings(digests)
	return digests
}

// checksNext reports whether the step after step index of a job checks a
// digest with sha256sum or shasum on a line that reads one of the builder's
// digest outputs, directly or through an environment variable the step or the
// job sets from it.
func checksNext(steps []any, index int, job map[string]any, builder string, digests []string) bool {
	if index+1 == len(steps) {
		return false
	}
	step, _ := steps[index+1].(map[string]any)
	var references []string
	for _, digest := range digests {
		expression := "needs." + builder + ".outputs." + digest
		references = append(references, expression)
		for _, holder := range []any{step["env"], job["env"]} {
			environment, _ := holder.(map[string]any)
			for name, value := range environment {
				if strings.Contains(inputText(value), expression) {
					references = append(references, "$"+name, "${"+name+"}")
				}
			}
		}
	}
	for _, line := range splitLines(stripComments(inputText(step["run"]))) {
		if sha256Tool.MatchString(line) && checkOption.MatchString(line) &&
			slices.ContainsFunc(references, func(reference string) bool { return strings.Contains(line, reference) }) {
			return true
		}
	}
	return false
}
