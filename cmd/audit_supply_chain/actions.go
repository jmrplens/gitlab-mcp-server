package main

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// actionReference is a pinned uses: value as the runner reads it: the
// repository it fetches, the directory of that repository the action's
// metadata sits in, empty for its root, and the commit.
type actionReference struct {
	repository string
	directory  string
	sha        string
}

// parseActionReference reads a uses: value as a pinned remote action, and
// reports false for anything else: a reference rule 1 refuses, a local
// directory, a container, and a pin that names no owner and repository.
func parseActionReference(uses string) (actionReference, bool) {
	if !pinnedUses.MatchString(uses) {
		return actionReference{}, false
	}
	name, sha, _ := strings.Cut(uses, "@")
	parts := strings.SplitN(name, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return actionReference{}, false
	}
	ref := actionReference{repository: parts[0] + "/" + parts[1], sha: sha}
	if len(parts) == 3 {
		ref.directory = parts[2]
	}
	return ref, true
}

// name is the action without its commit: the repository and the directory.
func (r actionReference) name() string {
	if r.directory == "" {
		return r.repository
	}
	return r.repository + "/" + r.directory
}

// key is the pinned reference in the spelling a uses: line carries, which is
// how the record is keyed.
func (r actionReference) key() string {
	return r.name() + "@" + r.sha
}

// openedAction is an action the audit reads: what a finding calls it and its
// steps, what its metadata file is called, what it says, and the directory a
// step's action path names.
type openedAction struct {
	root   place
	name   string
	label  string
	text   string
	writer string
}

// actionWalk is one job's walk through the actions its steps name: the job,
// whether it holds a credential, the workflow it is in, and what it has read
// already, shared with the job's own rule-2 reading so a script both reach is
// judged once.
type actionWalk struct {
	doc          map[string]any
	seen         map[string]bool
	job          string
	credentialed bool
}

// checkJobActions judges what the actions a job's steps name run, as though
// their steps were written in the job: every action's uses: lines by rule 1,
// the actions those name in turn, and, when the job holds a credential, their
// run blocks, the files those reach and their actions' inputs by rule 2.
//
// An audit built without a record judges nothing here, which is how the unit
// tests of the job rules build it; the audit of a tree always has one.
func (a *supplyChainAudit) checkJobActions(pathLabel, jobID string, job, doc map[string]any, credentialed bool, seen map[string]bool) []string {
	if a.store == nil {
		return nil
	}
	walk := actionWalk{doc: doc, seen: seen, job: pathLabel + ": job " + jobID, credentialed: credentialed}
	steps, _ := job["steps"].([]any)
	var problems []string
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		uses, _ := step["uses"].(string)
		with, _ := step["with"].(map[string]any)
		where := fmt.Sprintf("%s: step %d", walk.job, index)
		problems = append(problems, a.judgeUses(walk, where, uses, with, pathLabel)...)
	}
	return problems
}

// judgeUses opens the action a uses: value names, written in writer (the
// workflow, or the action whose step it is) with the inputs with sets: a
// pinned remote action from the record, a directory of the workspace from
// this repository. A container is not opened, and a reference rule 1 refuses
// is left to rule 1.
func (a *supplyChainAudit) judgeUses(walk actionWalk, where, uses string, with map[string]any, writer string) []string {
	if uses == "" || strings.HasPrefix(uses, "docker://") {
		return nil
	}
	if strings.HasPrefix(uses, "./") {
		return a.judgeLocalAction(walk, where, uses, writer)
	}
	if !pinnedUses.MatchString(uses) {
		return nil
	}
	ref, ok := parseActionReference(uses)
	if !ok {
		return a.once("repository "+uses, fmt.Sprintf("%s: uses: %s names no repository, so what it runs cannot be judged", where, uses))
	}
	return a.judgeRemoteAction(walk, where, ref, with)
}

// judgeRemoteAction opens a pinned action from the record. An action the
// record holds nothing for, or whose commit has no metadata file, is reported
// once however many steps reach it. The inputs a step sets are held to the
// guards declared for the action at every step of a credentialed job, before
// the action is known to have been read, since two steps of one job can set
// them differently and the action is read once.
func (a *supplyChainAudit) judgeRemoteAction(walk actionWalk, where string, ref actionReference, with map[string]any) []string {
	key := ref.key()
	problems := a.guardProblems(walk, where, ref, with)
	if walk.seen["action "+key] {
		return problems
	}
	walk.seen["action "+key] = true
	recorded, ok := a.store.action(ref)
	if !ok {
		return append(problems, a.once("record "+key, fmt.Sprintf("%s: uses: %s has no entry in %s, so what it runs cannot be judged; run %s",
			where, key, actionRecordPath, recordCommand))...)
	}
	if recorded.Metadata == "" {
		return append(problems, a.once("metadata "+key, fmt.Sprintf(
			"%s: uses: %s: that commit has no action.yml or action.yaml, so what it runs cannot be judged", where, key,
		))...)
	}
	return append(problems, a.judgeAction(walk, where, openedAction{
		name: key, label: key + "/" + recorded.Metadata, text: recorded.text(),
		writer: ref.name(), root: place{action: key},
	})...)
}

// guardProblems holds the inputs one step of a credentialed job sets for a
// pinned action to every match declared guarded in that action's run blocks:
// the step must set the guard's input to a literal other than the value that
// runs the branch the match sits in, or leave it to a default other than that
// value. A value written as an expression is one only the run knows, so it
// cannot rule the branch out. A step of a job holding no credential is not
// held, since rule 2 does not judge what that job runs, and neither is a step
// held to a declaration the table reports as unreadable or as naming no value.
func (a *supplyChainAudit) guardProblems(walk actionWalk, where string, ref actionReference, with map[string]any) []string {
	if !walk.credentialed {
		return nil
	}
	var problems []string
	for _, key := range sortedGuardKeys(a.guarded) {
		guarded, ok := parseGuardKey(key)
		guard := a.guarded[key]
		if !ok || guarded.action != ref.name() || strings.TrimSpace(guard.value) == "" {
			// The table names a declaration it cannot read, or one with no
			// value, on its own; holding a step to it would only repeat that.
			continue
		}
		raw, set := with[guard.input]
		how := "sets " + guard.input + " to "
		if !set {
			raw = a.inputDefault(ref, guard.input)
			how = "leaves " + guard.input + " to its default "
		}
		value := inputText(raw)
		accepted := fmt.Sprintf("the %s match %s accepts in its step %d", guarded.rule, declarationsFile, guarded.step)
		if strings.Contains(value, "${{") {
			problems = append(problems, fmt.Sprintf("%s: uses: %s %s%s, which only the run knows, so %s cannot be ruled out",
				where, ref.key(), how, value, accepted))
		} else if value == guard.value {
			problems = append(problems, fmt.Sprintf("%s: uses: %s %s%q, the value that runs %s",
				where, ref.key(), how, value, accepted))
		}
	}
	return problems
}

// inputDefault is the default the pinned action's metadata gives an input,
// and nil when the record holds no metadata for the action, the metadata does
// not parse or it gives the input none; the finding about the record or the
// metadata is made where the action is read.
func (a *supplyChainAudit) inputDefault(ref actionReference, input string) any {
	recorded, _ := a.store.action(ref)
	var metadata map[string]any
	if yaml.Unmarshal([]byte(recorded.text()), &metadata) != nil {
		return nil
	}
	inputs, _ := metadata["inputs"].(map[string]any)
	declared, _ := inputs[input].(map[string]any)
	return declared["default"]
}

// inputText is the text an input's value carries into the action: a string
// as written, nothing for an absent value, and any other scalar as YAML
// prints it, which is what the runner passes.
func inputText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

// localActionKey prefixes what a local action is remembered by, both in the
// walk's seen set and among the findings already reported, so it never
// collides with a remote action of the same spelling.
const localActionKey = "local "

// judgeLocalAction opens an action from a directory of the workspace, which
// is this repository as it was checked out. A directory that holds no action
// is one written at run time or none at all, and either way what it runs
// cannot be read; it is a finding unless the repository declares why it
// accepts it.
func (a *supplyChainAudit) judgeLocalAction(walk actionWalk, where, uses, writer string) []string {
	directory := path.Clean(strings.TrimPrefix(uses, "./"))
	if walk.seen[localActionKey+directory] {
		return nil
	}
	walk.seen[localActionKey+directory] = true
	if filepath.IsLocal(filepath.FromSlash(directory)) {
		for _, name := range metadataNames {
			metadata := path.Join(directory, name)
			if text, readable := readRegularFile(filepath.Join(a.root, filepath.FromSlash(metadata))); readable {
				return a.judgeAction(walk, where, openedAction{
					name: uses, label: metadata, text: text, writer: uses, root: place{path: directory},
				})
			}
		}
	}
	key := writer + " " + uses
	if _, declared := a.unjudged[key]; declared {
		a.unjudgedUsed[key] = true
		return nil
	}
	return a.once(localActionKey+key, fmt.Sprintf(
		"%s: uses: %s names no action.yml or action.yaml in this repository, so what it runs cannot be judged", where, uses,
	))
}

// judgeAction reads an opened action's metadata and judges what it runs. Only
// a composite action has steps to read: a JavaScript action runs the bundle
// committed at its pin, and a container action an image or a Dockerfile this
// audit does not open, which doc.go states as a limit. Metadata that does not
// parse, and a runs.using this audit does not know, are reported once.
func (a *supplyChainAudit) judgeAction(walk actionWalk, where string, action openedAction) []string {
	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(action.text), &metadata); err != nil {
		return a.once("parse "+action.label, fmt.Sprintf("%s does not parse, so what it runs cannot be judged: %v", action.label, err))
	}
	runs, _ := metadata["runs"].(map[string]any)
	declared, _ := runs["using"].(string)
	using := strings.ToLower(strings.TrimSpace(declared))
	if strings.HasPrefix(using, "node") || using == "docker" {
		return nil
	}
	if using != "composite" {
		return a.once("using "+action.label, fmt.Sprintf(
			"%s: runs.using %q is not one this audit reads, so what it runs cannot be judged", action.label, using,
		))
	}
	var problems []string
	if !a.reported["pins "+action.label] {
		a.reported["pins "+action.label] = true
		problems = checkPinnedUses(action.label, action.text)
	}
	steps, _ := runs["steps"].([]any)
	for index, rawStep := range steps {
		step, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		stepWhere := fmt.Sprintf("%s: %s step %d", where, action.name, index)
		problems = append(problems, a.judgeActionStep(walk, stepWhere, action, index, step)...)
	}
	return problems
}

// judgeActionStep judges step index of a composite action: the action it
// names, opened in turn, and, in a job holding a credential, the rules rule 2
// holds a workflow's own step to, with the action's directory as the action
// path its run block and everything that reaches can name. A match in the run
// block of a pinned action that a guarded declaration names is excused, since
// every step naming the action is held to the guard where it names it.
func (a *supplyChainAudit) judgeActionStep(walk actionWalk, where string, action openedAction, index int, step map[string]any) []string {
	rawUses, _ := step["uses"].(string)
	with, _ := step["with"].(map[string]any)
	problems := a.judgeUses(walk, where, rawUses, with, action.writer)
	if !walk.credentialed {
		return problems
	}
	uses, _, _ := strings.Cut(rawUses, "@")
	problems = append(problems, checkStepAction(where, uses, with, walk.doc)...)
	runSource, _ := step["run"].(string)
	runText := stripComments(runSource)
	for _, rule := range unlockedCode {
		if rule.match(runText) && !a.excusedByGuard(action, index, rule.name) {
			problems = append(problems, fmt.Sprintf("%s: run block matches %s: %s", where, rule.display, rule.why))
		}
	}
	directory, _ := step["working-directory"].(string)
	root := action.root
	reached := reach{actionDir: &root, job: walk.job, origin: where, directory: directory}
	return append(problems, a.followReferences(reached, runText, walk.seen)...)
}

// excusedByGuard reports whether a declared guarded match excuses rule in
// step index of an opened action, and records that it did. Only a pinned
// action's step can be excused: an action of this repository is fixed where
// it is written, like a workflow's own run block.
func (a *supplyChainAudit) excusedByGuard(action openedAction, index int, rule string) bool {
	if action.root.action == "" {
		return false
	}
	key := guardKey{action: action.writer, step: index, rule: rule}.String()
	if _, declared := a.guarded[key]; !declared {
		return false
	}
	a.guardedUsed[key] = true
	return true
}

// once returns problem the first time key is reported in this audit and
// nothing after, so an action every job names is named once.
func (a *supplyChainAudit) once(key, problem string) []string {
	if a.reported[key] {
		return nil
	}
	a.reported[key] = true
	return []string{problem}
}
