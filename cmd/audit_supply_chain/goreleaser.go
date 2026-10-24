package main

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// goreleaserAction is the action whose configuration rule 2 reads, since the
// commands GoReleaser runs run in the job that runs it.
const goreleaserAction = "goreleaser/goreleaser-action"

// readsConfig opens a finding about the configuration a GoReleaser run reads,
// which the configuration's name follows.
const readsConfig = "reads "

// goreleaserConfigNames are the files GoReleaser reads its configuration from
// when no --config names one, in the order it tries them (cmd/config.go of the
// release the release job pins).
var goreleaserConfigNames = []string{
	".config/goreleaser.yml", ".config/goreleaser.yaml", ".goreleaser.yml", ".goreleaser.yaml", "goreleaser.yml", "goreleaser.yaml",
}

// goreleaserConfigFlag finds the configuration a GoReleaser command line names
// after --config or -f, as a word or as a workflow expression. The action's
// args input may be an expression choosing between two command lines, and the
// flag is found inside either.
var goreleaserConfigFlag = regexp.MustCompile(`(?:^|[\s'"])(?:--config|-f)(?:=|[ \t]+)(\$\{\{.*?\}\}|[^\s'"]+)`)

// goreleaserSigners are the GoReleaser sections whose entries run a command
// with arguments, each with the command an entry that names none runs and
// whether it runs in the dist directory rather than where GoReleaser runs.
var goreleaserSigners = []struct {
	section string
	command string
	inDist  bool
}{
	{section: "signs", command: "gpg"},
	{section: "binary_signs", command: "gpg"},
	{section: "docker_signs", command: "cosign"},
	{section: "sboms", command: "syft", inDist: true},
}

// goreleaserBuilders are the builders GoReleaser builds with by the go command
// itself: its default, named or left out.
var goreleaserBuilders = map[string]bool{"": true, "go": true}

// goreleaserCommand is one command a GoReleaser configuration runs: where the
// configuration writes it, the repository-relative directory GoReleaser runs
// it in, its text, and the env list it runs with.
type goreleaserCommand struct {
	env       any
	subject   string
	directory string
	text      string
}

// followGoReleaser judges what a step of goreleaser/goreleaser-action makes
// GoReleaser run: the configuration --config or -f names in the action's
// args, or the first of GoReleaser's own names, read from the directory the
// workdir input names; what decides how its builds run; and every command it
// runs, each held to the run-time code rules and the Go-command rules and
// followed into what it runs, in the directory GoReleaser runs it in, with the
// GOFLAGS its env list sets. A configuration this audit cannot find, read or
// parse is a finding, and one the job has read already is not read again.
func (a *supplyChainAudit) followGoReleaser(at reach, with map[string]any, seen map[string]bool) []string {
	label := at.origin + ": goreleaser-action "
	workdir := "."
	if written, ok := with["workdir"].(string); ok {
		workdir = written
	}
	if strings.Contains(workdir, "$") {
		return []string{label + "runs in " + workdir + ", a value only the run knows, so what it runs cannot be judged"}
	}
	config, problem := a.goreleaserConfig(workdir, inputText(with["args"]))
	if problem != "" {
		return []string{label + problem + ", so what it runs cannot be judged"}
	}
	if seen[config] {
		return nil
	}
	seen[config] = true
	text, readable := a.readRepositoryFile(config)
	if !readable {
		return []string{label + readsConfig + config + ", which is not a readable file in this repository, so what it runs cannot be judged"}
	}
	var document map[string]any
	if yaml.Unmarshal([]byte(text), &document) != nil {
		return []string{label + readsConfig + config + ", which does not parse as a GoReleaser configuration, so what it runs cannot be judged"}
	}
	problems := goreleaserBuildProblems(at.job+": "+config, document)
	for _, command := range goreleaserCommands(config, document, workdir) {
		inside := at.inside(command.subject, command.directory, place{})
		problems = append(problems, a.matchFile(at, command.subject, command.text)...)
		problems = append(problems, a.judgeGoCommands(inside, command.text)...)
		problems = append(problems, a.followReferences(inside, command.text, seen)...)
		problems = append(problems, goFlagsInList(at.job+": "+command.subject+".env", command.env)...)
	}
	return problems
}

// goreleaserBuildProblems judges what decides how a GoReleaser
// configuration's builds run, label naming the configuration: the GOFLAGS its
// env list sets for everything it runs, and of every build a builder other
// than go's, a tool other than the go command, an option on its flags the
// go command is not run with here, and the GOFLAGS its own env list sets. What
// the builds compile is the release itself, which no step runs.
func goreleaserBuildProblems(label string, document map[string]any) []string {
	problems := goFlagsInList(label+":env", document["env"])
	builds, _ := document["builds"].([]any)
	for index, raw := range builds {
		entry, _ := raw.(map[string]any)
		build := fmt.Sprintf("%s:builds[%d]", label, index)
		if builder := inputText(entry["builder"]); !goreleaserBuilders[builder] {
			problems = append(problems, build+".builder names "+builder+", a builder that runs a toolchain this audit does not read"+cannotJudgeBuild)
		}
		for _, field := range []string{"tool", "gobinary"} {
			problems = append(problems, goBinaryProblems(build+"."+field, entry[field])...)
		}
		var flags []string
		listed, _ := entry["flags"].([]any)
		for _, flag := range listed {
			flags = append(flags, inputText(flag))
		}
		problems = append(problems, goOptionProblems(build+".flags", flags)...)
		problems = append(problems, goFlagsInList(build+".env", entry["env"])...)
	}
	gomod, _ := document["gomod"].(map[string]any)
	return append(problems, goBinaryProblems(label+":gomod.gobinary", gomod["gobinary"])...)
}

// goBinaryProblems judges the program a GoReleaser setting names to build
// with, label naming the setting: the go command, by its name or a path to
// it, or none, which is the go command too.
func goBinaryProblems(label string, value any) []string {
	binary := inputText(value)
	if binary == "" || path.Base(binary) == "go" {
		return nil
	}
	return []string{label + " names " + binary + " in place of the go command" + cannotJudgeBuild}
}

// goFlagsInList holds the GOFLAGS an env list of a GoReleaser configuration
// sets, each entry a KEY=VALUE string, label naming the list, to what
// goFlagsProblems holds it to.
func goFlagsInList(label string, value any) []string {
	entries, _ := value.([]any)
	var problems []string
	for _, raw := range entries {
		if flags, set := strings.CutPrefix(inputText(raw), "GOFLAGS="); set {
			problems = append(problems, goFlagsProblems(label, flags)...)
		}
	}
	return problems
}

// goreleaserConfig is the configuration a GoReleaser run reads: the file its
// command line names, relative to workdir, or the first of GoReleaser's names
// workdir holds. problem says why there is none this audit can name.
func (a *supplyChainAudit) goreleaserConfig(workdir, args string) (config, problem string) {
	if match := goreleaserConfigFlag.FindStringSubmatch(args); match != nil {
		if strings.Contains(match[1], "$") {
			return "", readsConfig + match[1] + ", a value only the run knows"
		}
		return path.Join(workdir, match[1]), ""
	}
	for _, name := range goreleaserConfigNames {
		candidate := path.Join(workdir, name)
		if _, readable := a.readRepositoryFile(candidate); readable {
			return candidate, ""
		}
	}
	return "", "runs in " + workdir + ", which holds none of " + strings.Join(goreleaserConfigNames, ", ")
}

// readRepositoryFile reads a repository-relative file, and reports false for
// one that lies outside the repository or is not a regular file it can read.
func (a *supplyChainAudit) readRepositoryFile(relative string) (string, bool) {
	if !filepath.IsLocal(filepath.FromSlash(relative)) {
		return "", false
	}
	return readRegularFile(filepath.Join(a.root, filepath.FromSlash(relative)))
}

// goreleaserCommands returns every command a GoReleaser configuration runs,
// section by section: the before hooks, the hooks of every build and
// universal binary, the binary every upx entry runs, the command and arguments
// of every sign, binary sign, docker sign and sbom, the hooks of every docker
// image, every publisher, and the after hooks. A hook and a publisher run in
// their dir when they name one, relative to workdir, an sbom in the dist
// directory, and everything else in workdir, where GoReleaser itself runs: a
// build's dir is where it compiles, not where its hooks run.
func goreleaserCommands(config string, document map[string]any, workdir string) []goreleaserCommand {
	dist := path.Join(workdir, "dist")
	if written, ok := document["dist"].(string); ok {
		dist = path.Join(workdir, written)
	}
	before, _ := document["before"].(map[string]any)
	commands := goreleaserHooks(nil, config+":before.hooks", before["hooks"], workdir)
	commands = goreleaserSectionHooks(commands, config, document, "builds", workdir)
	commands = goreleaserSectionHooks(commands, config, document, "universal_binaries", workdir)
	upx, _ := document["upx"].([]any)
	for index, raw := range upx {
		entry, _ := raw.(map[string]any)
		binary := "upx"
		if written, ok := entry["binary"].(string); ok {
			binary = written
		}
		commands = append(commands, goreleaserCommand{subject: fmt.Sprintf("%s:upx[%d]", config, index), directory: workdir, text: binary})
	}
	for _, signer := range goreleaserSigners {
		directory := workdir
		if signer.inDist {
			directory = dist
		}
		entries, _ := document[signer.section].([]any)
		for index, raw := range entries {
			entry, _ := raw.(map[string]any)
			words := []string{signer.command}
			if command, ok := entry["cmd"].(string); ok {
				words[0] = command
			}
			arguments, _ := entry["args"].([]any)
			for _, argument := range arguments {
				words = append(words, shellWord(inputText(argument)))
			}
			subject := fmt.Sprintf("%s:%s[%d]", config, signer.section, index)
			commands = append(commands, goreleaserCommand{env: entry["env"], subject: subject, directory: directory, text: strings.Join(words, " ")})
		}
	}
	commands = goreleaserSectionHooks(commands, config, document, "dockers_v2", workdir)
	publishers, _ := document["publishers"].([]any)
	for index, raw := range publishers {
		entry, _ := raw.(map[string]any)
		subject := fmt.Sprintf("%s:publishers[%d]", config, index)
		commands = append(commands, goreleaserCommand{env: entry["env"], subject: subject, directory: hookDirectory(entry, workdir), text: inputText(entry["cmd"])})
	}
	after, _ := document["after"].(map[string]any)
	return goreleaserHooks(commands, config+":after.hooks", after["hooks"], workdir)
}

// goreleaserSectionHooks appends the commands the pre and post hooks of every
// entry of a section run.
func goreleaserSectionHooks(commands []goreleaserCommand, config string, document map[string]any, section, workdir string) []goreleaserCommand {
	entries, _ := document[section].([]any)
	for index, raw := range entries {
		entry, _ := raw.(map[string]any)
		hooks, _ := entry["hooks"].(map[string]any)
		for _, phase := range []string{"pre", "post"} {
			subject := fmt.Sprintf("%s:%s[%d].hooks.%s", config, section, index, phase)
			commands = goreleaserHooks(commands, subject, hooks[phase], workdir)
		}
	}
	return commands
}

// goreleaserHooks appends the commands a hooks value runs: one command
// written as a string, or a list of them, each a string or a mapping whose
// cmd runs in its dir with its env.
func goreleaserHooks(commands []goreleaserCommand, subject string, value any, workdir string) []goreleaserCommand {
	if text, ok := value.(string); ok {
		return append(commands, goreleaserCommand{subject: subject, directory: workdir, text: text})
	}
	hooks, _ := value.([]any)
	for index, raw := range hooks {
		key := fmt.Sprintf("%s[%d]", subject, index)
		switch hook := raw.(type) {
		case string:
			commands = append(commands, goreleaserCommand{subject: key, directory: workdir, text: hook})
		case map[string]any:
			commands = append(commands, goreleaserCommand{env: hook["env"], subject: key, directory: hookDirectory(hook, workdir), text: inputText(hook["cmd"])})
		}
	}
	return commands
}

// hookDirectory is the directory a hook or a publisher runs in: its dir,
// relative to workdir, when it names one, and workdir otherwise.
func hookDirectory(entry map[string]any, workdir string) string {
	if dir, ok := entry["dir"].(string); ok {
		return path.Join(workdir, dir)
	}
	return workdir
}

// shellWord writes one argument GoReleaser hands a command as the shell would
// read it back: quoted when it holds a space, so it stays one word.
func shellWord(argument string) string {
	if strings.ContainsAny(argument, " \t") {
		return "'" + argument + "'"
	}
	return argument
}
