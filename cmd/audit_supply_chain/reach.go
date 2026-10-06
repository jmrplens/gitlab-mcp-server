package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// scriptPath finds the script paths a text names: any path whose last
// directory is scripts/ and whose file is a shell, Node, Python or PowerShell
// script, so .github/scripts/, test/e2e/scripts/ and the root's scripts/ are
// all read under the name they are run by.
//
// The first group is what roots the path, when something does. A variable in
// front of it ($ROOT, ${repo_root}, $(CURDIR)) and the workspace expression
// GitHub checks the repository out into are read as the repository root, which
// is what every such spelling in this tree means. A leading slash is an
// absolute path or the tail of a URL, neither of which names a file in this
// repository. The second group is the path under that root.
var scriptPath = regexp.MustCompile(`(\$\{\{\s*github\.workspace\s*\}\}/|\$[({]?[A-Za-z_][A-Za-z0-9_]*[)}]?/|/)?((?:[A-Za-z0-9_.-]+/)*scripts/[A-Za-z0-9_.-]+\.(?:sh|mjs|py|ps1))\b`)

// ownPath is how a shell script names its own path: $0, or BASH_SOURCE when it
// may be sourced rather than run.
const ownPath = `\$\{?(?:0|BASH_SOURCE(?:\[0\])?)\}?`

// variableScript finds a script path rooted in a variable, a workflow
// expression or the location of the script that names it, whose path does not
// run through a scripts/ directory, since scriptPath reads those:
// "$HERE/helper.sh", "${SCRIPT_DIR}/lib/x.sh", "$(dirname "$0")/x.sh" and
// ${{ runner.temp }}/x.sh.
//
// Group 1 is the root as written, group 2 the inside of a workflow
// expression, group 3 a variable's name, and group 4 the path under the root.
var variableScript = regexp.MustCompile(`(\$\{\{\s*([^}]*?)\s*\}\}|\$[({]?([A-Za-z_][A-Za-z0-9_]*|[0-9])[)}]?|\$\([ \t]*dirname[ \t]+"?` +
	ownPath + `"?[ \t]*\))"?/((?:[A-Za-z0-9_.-]+/)*[A-Za-z0-9_.-]+\.(?:sh|mjs|py|ps1))\b`)

// selfDirectory is a shell assignment of a script's own directory, or of one
// above it: NAME="$(cd "$(dirname "$0")/.." && pwd)", with BASH_SOURCE for $0,
// without the cd, and behind export, local, readonly or declare. Group 1 is the
// name and group 2 the steps up.
var selfDirectory = regexp.MustCompile(`(?m)^[ \t]*(?:(?:export|local|readonly|declare(?:[ \t]+-[A-Za-z]+)*)[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)=.*?dirname[ \t]+"?` +
	ownPath + `"?[ \t]*\)((?:/\.\.)*)(?:["' \t)]|$)`)

// reach is where a text sits, for the findings about it and the references it
// makes: the job reading it, the place that wrote the text (a step, or the
// file or target it is the body of), the directory it runs in, the file or
// target that reached it, empty for a run block, and the script it is the
// body of, empty for a run block or a recipe.
type reach struct {
	job       string
	origin    string
	directory string
	via       string
	file      string
}

// label is how a finding names a file or target: by itself when a run block
// reached it, and with the file or target that reached it otherwise.
func (r reach) label(subject string) string {
	if r.via == "" {
		return subject
	}
	return subject + " (via " + r.via + ")"
}

// inside is the reach of the body of subject, which runs in directory and is
// the script file named, or no file.
func (r reach) inside(subject, directory, file string) reach {
	return reach{job: r.job, origin: r.job + ": " + r.label(subject), directory: directory, via: subject, file: file}
}

// rootOf is the directory a variableScript match is read from, and false when
// this audit cannot know it.
//
// The workspace GitHub checks the repository out into is the root, in either
// spelling. A script's own location, through dirname of its own path, a
// variable it assigns from that, or $PSScriptRoot in PowerShell, is the
// directory of the file being read, and names nothing in a run block or a
// recipe. Every other variable holds a value only the run knows.
func (r reach) rootOf(match []string, text string) (string, bool) {
	expression, name := match[2], match[3]
	if strings.HasPrefix(match[1], "${{") {
		return "", expression == "github.workspace"
	}
	if name == "GITHUB_WORKSPACE" {
		return "", true
	}
	if r.file == "" {
		return "", false
	}
	if name == "" {
		return path.Dir(r.file), true
	}
	levels, own := selfDirectories(text, r.file)[name]
	return path.Join(path.Dir(r.file), strings.Repeat("../", levels)), own
}

// selfDirectories names the variables a script holds its own directory in,
// with how many directories above it each one points: $PSScriptRoot in a
// PowerShell script, and every assignment selfDirectory reads.
func selfDirectories(text, file string) map[string]int {
	directories := map[string]int{}
	if strings.HasSuffix(file, ".ps1") {
		directories["PSScriptRoot"] = 0
	}
	for _, match := range selfDirectory.FindAllStringSubmatch(text, -1) {
		directories[match[1]] = strings.Count(match[2], "/..")
	}
	return directories
}

// followReferences reads every script and make target a text runs, applies the
// run-time-code rules to each, and follows what each of those runs in turn.
// seen holds what this job has read already, so a file reached twice, or a
// cycle back to one, is read once.
//
// What it cannot follow it reports: a script rooted in a value only the run
// knows, a make behind a wrapper written in a way it does not read, and a make
// it cannot read the arguments of or that names no target.
func (a *supplyChainAudit) followReferences(at reach, text string, seen map[string]bool) []string {
	var problems []string
	for _, match := range scriptPath.FindAllStringSubmatch(text, -1) {
		resolved, local := resolveReference(match[1], match[2], at.directory)
		problems = append(problems, a.readScript(at, match[0], resolved, local, seen)...)
	}
	for _, match := range variableScript.FindAllStringSubmatch(text, -1) {
		if strings.HasPrefix(match[4], "scripts/") || strings.Contains(match[4], "/scripts/") {
			continue
		}
		root, known := at.rootOf(match, text)
		if !known {
			problems = append(problems, fmt.Sprintf(
				"%s: %s is rooted in %s, which this audit cannot resolve, so what it runs cannot be judged",
				at.origin, match[0], match[1],
			))
			continue
		}
		resolved := path.Join(root, match[4])
		problems = append(problems, a.readScript(at, match[0], resolved, filepath.IsLocal(filepath.FromSlash(resolved)), seen)...)
	}
	invocations, unread := scanMake(text)
	for _, wrapper := range unread {
		problems = append(problems, fmt.Sprintf(
			"%s: make behind %s is written in a way this audit does not read, so the targets it runs cannot be judged",
			at.origin, wrapper,
		))
	}
	for _, invocation := range invocations {
		if invocation.refusal != "" {
			problems = append(problems, at.origin+": "+invocation.refusal+", so the targets it runs cannot be judged")
			continue
		}
		if len(invocation.targets) == 0 {
			problems = append(problems, at.origin+": make names no target, so the default goal it runs cannot be judged")
		}
		run := invocation.run(at.directory)
		for _, target := range invocation.targets {
			problems = append(problems, a.followTarget(at, run, target, true, seen)...)
		}
	}
	return problems
}

// resolveReference turns a script path as written into the repository-relative
// path it names, and reports false for one that names nothing in the
// repository: an absolute path, a URL, or a relative path that climbs out.
func resolveReference(prefix, written, directory string) (string, bool) {
	if prefix == "/" {
		return prefix + written, false
	}
	if prefix != "" {
		directory = ""
	}
	resolved := path.Join(directory, written)
	return resolved, filepath.IsLocal(filepath.FromSlash(resolved))
}

// readScript judges one script a job reaches and follows what it runs, or
// reports it when it is not a file this audit can read. A script this job has
// read already is not read again.
func (a *supplyChainAudit) readScript(at reach, written, resolved string, local bool, seen map[string]bool) []string {
	if seen[resolved] {
		return nil
	}
	seen[resolved] = true
	body, readable := "", false
	if local {
		body, readable = readRegularFile(filepath.Join(a.root, filepath.FromSlash(resolved)))
	}
	if !readable {
		return []string{fmt.Sprintf(
			"%s: %s resolves to %s, which is not a readable file in this repository, so what it runs cannot be judged",
			at.origin, written, resolved,
		)}
	}
	body = stripComments(body)
	problems := a.matchFile(at, resolved, body)
	return append(problems, a.followReferences(at.inside(resolved, at.directory, resolved), body, seen)...)
}

// followTarget judges one make target a job reaches: its recipe, the targets
// it depends on and what its recipe runs, in the directory and from the
// makefile its command named. A target the job names and the makefile has no
// rule for is reported; a prerequisite with no rule is a file make checks
// rather than a recipe it runs, and is passed over.
func (a *supplyChainAudit) followTarget(at reach, run makeRun, target string, invoked bool, seen map[string]bool) []string {
	rule := a.makefileRules(run.makefile)[target]
	if rule == nil && !invoked {
		return nil
	}
	subject := run.makefile + ":" + target
	if seen[subject] {
		return nil
	}
	seen[subject] = true
	if rule == nil {
		return []string{fmt.Sprintf("%s: make %s has no rule in %s, so its recipe cannot be judged", at.origin, target, run.makefile)}
	}
	recipe := stripComments(strings.Join(rule.recipe, "\n"))
	problems := a.matchFile(at, subject, recipe)
	inside := at.inside(subject, run.directory, "")
	for _, prerequisite := range rule.prerequisites {
		problems = append(problems, a.followTarget(inside, run, prerequisite, false, seen)...)
	}
	return append(problems, a.followReferences(inside, recipe, seen)...)
}

// matchFile applies the run-time-code rules to the body of a file or target a
// job reaches, passing over a match the declaration table excuses and
// recording that it did.
func (a *supplyChainAudit) matchFile(at reach, subject, body string) []string {
	var problems []string
	for _, rule := range unlockedCode {
		if !rule.match(body) {
			continue
		}
		key := subject + " " + rule.name
		if _, declared := a.declarations[key]; declared {
			a.excused[key] = true
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: %s matches %s: %s", at.job, at.label(subject), rule.display, rule.why))
	}
	return problems
}

// makefileRules returns the rules of a makefile, parsing it the first time it
// is asked for. A makefile that is not there, or that lies outside the
// repository, has no rules, so every target a job asks of it is reported.
func (a *supplyChainAudit) makefileRules(makefile string) map[string]*makeRule {
	rules, parsed := a.makefiles[makefile]
	if !parsed {
		text := ""
		if filepath.IsLocal(filepath.FromSlash(makefile)) {
			text, _ = readRegularFile(filepath.Join(a.root, filepath.FromSlash(makefile)))
		}
		rules = parseMakefile(text)
		a.makefiles[makefile] = rules
	}
	return rules
}

// readRegularFile reads a file this repository's workflows reach, and reports
// false for anything that is not a regular file it can read.
//
// The shape is decided ahead of the read rather than left to it: a directory
// would fail the read, but a named pipe would block on it and hang the audit
// instead of failing it.
func readRegularFile(full string) (string, bool) {
	info, err := os.Stat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(full) //#nosec G304 -- an audit tool reading a file this repository's own workflows run.
	if err != nil {
		return "", false
	}
	return string(data), true
}
