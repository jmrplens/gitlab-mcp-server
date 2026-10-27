// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// goCommandReason is why rule 2 holds a Go command to this module and the
// standard library: the half every finding about another module's code ends
// with.
const goCommandReason = "any code a credentialed job runs holds the job's credential, so a Go command run there is built from this module and the standard library alone"

// cannotJudgeBuild ends a finding about a go command this audit cannot read.
const cannotJudgeBuild = ", so what it builds cannot be judged"

// cannotJudgeRun ends a finding about a go command that builds nothing it
// runs, go vet, handed a program this audit cannot read.
const cannotJudgeRun = ", so what it runs cannot be judged"

// passesOption opens the refusal of an option a go command passes, which the
// option's spelling follows, and refusedBecause joins that option to why it
// is refused.
const (
	passesOption   = "passes "
	refusedBecause = ", which "
)

// goRunCommand is how a finding names the one subcommand that runs a single
// package, and the only one whose package is followed by the program's own
// arguments.
const goRunCommand = "run"

// goCommand finds the go command where the shell runs it, the way makeCommand
// finds make: in command position, behind any assignments and wrappers in
// front of it, and inside the string sh -c is handed, with the program named
// by itself or by a path ending in go. Group 2 is the go word and group 3 the
// subcommand rule 2 judges; a prose mention, inside an echo or a quoted
// message, is not in command position and does not match.
var goCommand = regexp.MustCompile(goCommandPattern())

// goCommandPattern is goCommand's expression, assembled in a function so the
// pieces it shares with makeCommand are joined where a test runs them.
func goCommandPattern() string {
	return `(?m)` + commandStart + commandKeywords + `(?:(?:` + assignmentWord + `|` + commandWrapper + `)[ \t]+)*` +
		`(?:[A-Za-z0-9_.~/-]*/)?(go)[ \t]+(run|build|install|test|vet|generate|tool)(?:[ \t;&|)\x60'"]|$)`
}

// goBehindWrapper finds, after a wrapper in command position, a go word and a
// subcommand rule 2 judges, group 1 being the go word, so that one behind a
// wrapper goCommand does not read is reported rather than passed over.
var goBehindWrapper = regexp.MustCompile(`(?:^|[^A-Za-z0-9_.$-])(?:[A-Za-z0-9_.~/-]*/)?(go)[ \t]+(?:run|build|install|test|vet|generate|tool)(?:[ \t;&|)\x60'"]|$)`)

// refusedGoFlags are the options of the go command that change what it
// builds, or run another program while it builds, in a way the source this
// audit reads cannot show, each with what it does. One of them on a command
// line, on GOFLAGS or on a GoReleaser build's flags is a finding.
var refusedGoFlags = map[string]string{
	"toolexec":   "runs every tool of the build through a program this audit does not read",
	"exec":       "runs the program it builds through a program this audit does not read",
	"overlay":    "builds files this audit does not read in place of the module's",
	"modfile":    "reads another go.mod in place of the module's",
	"pkgdir":     "installs and loads packages from a directory this audit does not read",
	"linkshared": "links against shared libraries this audit does not read",
}

// goVetTool is the option of go vet that runs another program as its
// analyzer, the one of its own a credentialed job may not pass.
const goVetTool = "vettool"

// otherModulePatterns are the meta-patterns of the go command that name
// another module's packages: every package of the build list, the tools a
// tool directive of go.mod declares, and every package of the workspace's
// modules. std and cmd, the toolchain's own, read as the standard library.
var otherModulePatterns = map[string]bool{"all": true, "tool": true, "work": true}

// goInvocation is one go command a text runs: its subcommand and the words
// after it, split as the shell splits them.
type goInvocation struct {
	subcommand string
	words      []string
}

// scanGo returns every go command a text runs, and the wrappers that run one
// this audit cannot read, once per wrapper: a command holds one go before its
// next separator, whatever stands in front of it. A line continued with a
// backslash is joined first, as the shell joins it.
func scanGo(text string) (invocations []goInvocation, unread []string) {
	text = strings.ReplaceAll(text, "\\\n", " ")
	read := map[int]bool{}
	for _, location := range goCommand.FindAllStringSubmatchIndex(text, -1) {
		read[location[5]] = true
		rest := text[location[7]:]
		if location[2] != -1 {
			rest = cutAtQuote(rest, text[location[2]])
		}
		invocations = append(invocations, goInvocation{subcommand: text[location[6]:location[7]], words: commandWords(rest)})
	}
	for _, location := range makeBehindWrapper.FindAllStringSubmatchIndex(text, -1) {
		from, to := location[5], len(text)
		if end := strings.IndexAny(text[from:], "\n;&|)\x60"); end != -1 {
			to = from + end
		}
		if found := goBehindWrapper.FindStringSubmatchIndex(text[from:to]); len(found) == 4 && !read[from+found[3]] {
			unread = append(unread, text[location[4]:location[5]])
		}
	}
	return invocations, unread
}

// goFlag says what an option of the go command takes.
type goFlag int

const (
	// goFlagUnknown is an option goFlags does not hold.
	goFlagUnknown goFlag = iota
	// goFlagBool is an option that takes nothing, or a value after =.
	goFlagBool
	// goFlagValue takes the next word as its value when none follows an =.
	goFlagValue
	// goFlagArgs is go test's -args, after which every word is the test
	// binary's.
	goFlagArgs
)

// goFlags are the options of go run, go build, go install and go test, the
// build flags and the test flags, by name without their dashes. One this table
// does not hold is reported rather than guessed at, since an option read as a
// flag that takes no value would turn its value into a package.
var goFlags = map[string]goFlag{
	"a": goFlagBool, "n": goFlagBool, "x": goFlagBool, "v": goFlagBool, "race": goFlagBool, "msan": goFlagBool,
	"asan": goFlagBool, "cover": goFlagBool, "work": goFlagBool, "trimpath": goFlagBool, "linkshared": goFlagBool,
	"modcacherw": goFlagBool, "buildvcs": goFlagBool, "json": goFlagBool, "i": goFlagBool, "c": goFlagBool,
	"benchmem": goFlagBool, "failfast": goFlagBool, "fullpath": goFlagBool, "short": goFlagBool, "artifacts": goFlagBool,

	"C": goFlagValue, "o": goFlagValue, "p": goFlagValue, "asmflags": goFlagValue, "buildmode": goFlagValue,
	"compiler": goFlagValue, "gccgoflags": goFlagValue, "gcflags": goFlagValue, "installsuffix": goFlagValue,
	"ldflags": goFlagValue, "mod": goFlagValue, "modfile": goFlagValue, "overlay": goFlagValue, "pgo": goFlagValue,
	"pkgdir": goFlagValue, "tags": goFlagValue, "toolexec": goFlagValue, "covermode": goFlagValue,
	"coverpkg": goFlagValue, "exec": goFlagValue, "bench": goFlagValue, "benchtime": goFlagValue,
	"blockprofile": goFlagValue, "blockprofilerate": goFlagValue, "count": goFlagValue, "coverprofile": goFlagValue,
	"cpu": goFlagValue, "cpuprofile": goFlagValue, "fuzz": goFlagValue, "fuzzminimizetime": goFlagValue,
	"fuzztime": goFlagValue, "list": goFlagValue, "memprofile": goFlagValue, "memprofilerate": goFlagValue,
	"mutexprofile": goFlagValue, "mutexprofilefraction": goFlagValue, "outputdir": goFlagValue,
	"parallel": goFlagValue, "run": goFlagValue, "shuffle": goFlagValue, "skip": goFlagValue,
	"timeout": goFlagValue, "trace": goFlagValue, "vet": goFlagValue,

	"args": goFlagArgs,
}

// goToolchainTools are the tools go tool runs from the toolchain itself rather
// than from a tool directive of go.mod.
var goToolchainTools = map[string]bool{
	"addr2line": true, "asm": true, "buildid": true, "cgo": true, "compile": true, "covdata": true, "cover": true,
	"dist": true, "distpack": true, "doc": true, "fix": true, "link": true, "nm": true, "objdump": true, "pack": true,
	"pprof": true, "preprofile": true, "test2json": true, "trace": true, "vet": true,
}

// goArguments are the words of a go command read as the go command reads
// them: why they cannot be read when they cannot, the directory -C changes
// into, the packages it builds, and whether those are .go files it was handed
// in place of a package.
type goArguments struct {
	refusal   string
	directory string
	packages  []string
	files     bool
}

// parseGoArguments reads the words after a subcommand. go run takes its
// options before the package, or the .go files, and hands every word after it
// to the program; go build, go install and go test take options anywhere and
// build every other word, the directory itself when they name none, and go
// test stops at -args. Their words name .go files in place of a package when
// the first of them does, as go reads them. The first option refusedGoFlags
// names is the refusal, with what it does, and the words after it are still
// read, so the package a pinning rule judges is still found.
func parseGoArguments(subcommand string, words []string) goArguments {
	var parsed goArguments
	for index := 0; index < len(words); index++ {
		word := words[index]
		if !strings.HasPrefix(word, "-") {
			if subcommand == goRunCommand {
				parsed.packages, parsed.files = runTarget(words[index:])
				return parsed
			}
			parsed.packages = append(parsed.packages, word)
			continue
		}
		spelled, value, attached := strings.Cut(word, "=")
		name := strings.TrimPrefix(strings.TrimPrefix(spelled, "-"), "-")
		kind := goFlags[name]
		if kind == goFlagUnknown {
			parsed.refusal = passesOption + spelled + ", an option this audit does not read"
			return parsed
		}
		if why := refusedGoFlags[name]; why != "" && parsed.refusal == "" {
			parsed.refusal = passesOption + spelled + refusedBecause + why
		}
		if kind == goFlagArgs {
			break
		}
		if kind == goFlagValue && !attached {
			if index+1 == len(words) {
				parsed.refusal = passesOption + spelled + " without the value it takes"
				return parsed
			}
			index++
			value = words[index]
		}
		if name == "C" {
			parsed.directory = value
		}
	}
	if subcommand != goRunCommand {
		if len(parsed.packages) == 0 {
			parsed.packages = []string{"."}
		}
		parsed.files = strings.HasSuffix(parsed.packages[0], ".go")
	}
	return parsed
}

// runTarget is what go run builds out of the words from its first non-option:
// the .go files they open with, or the one package they open with.
func runTarget(words []string) (target []string, files bool) {
	for _, word := range words {
		if !strings.HasSuffix(word, ".go") {
			break
		}
		target = append(target, word)
	}
	if len(target) > 0 {
		return target, true
	}
	return words[:1], false
}

// goRunAtVersion reports whether text runs go run of a package at a version,
// which builds another module's content whatever its version is; @latest is
// left to the latest rule, so one command is one finding.
func goRunAtVersion(text string) bool {
	invocations, _ := scanGo(text)
	for _, invocation := range invocations {
		if invocation.subcommand != goRunCommand {
			continue
		}
		parsed := parseGoArguments(goRunCommand, invocation.words)
		if len(parsed.packages) == 0 {
			continue
		}
		if _, version, versioned := strings.Cut(parsed.packages[0], "@"); versioned && version != "latest" {
			return true
		}
	}
	return false
}

// judgeGoCommands holds every go command a text a credentialed job runs to
// this module and the standard library, with what it builds resolved from the
// directory the text runs in, and every GOFLAGS the text sets to the options
// a go command written out is held to.
func (a *supplyChainAudit) judgeGoCommands(at reach, text string) []string {
	invocations, unread := scanGo(text)
	var problems []string
	for _, wrapper := range unread {
		problems = append(problems, at.origin+": go behind "+wrapper+" is written in a way this audit does not read"+cannotJudgeBuild)
	}
	for _, setting := range goFlagsSetting.FindAllStringSubmatch(text, -1) {
		problems = append(problems, goFlagsProblems(at.origin, unquote(setting[1]))...)
	}
	for _, invocation := range invocations {
		problems = append(problems, a.judgeGoInvocation(at, invocation)...)
	}
	return problems
}

// judgeGoInvocation judges one go command: go generate and a go tool the
// toolchain does not ship outright, the options go vet runs another program
// with, and every package go run, go build, go install or go test builds
// through its imports.
func (a *supplyChainAudit) judgeGoInvocation(at reach, invocation goInvocation) []string {
	switch invocation.subcommand {
	case "generate":
		return []string{at.origin + ": go generate runs whatever the //go:generate lines of its packages name, " +
			"which this audit does not read, so what it runs cannot be judged"}
	case "tool":
		return goToolProblems(at, invocation.words)
	case "vet":
		return goVetProblems(at, invocation.words)
	}
	command := "go " + invocation.subcommand
	parsed := parseGoArguments(invocation.subcommand, invocation.words)
	if parsed.refusal != "" {
		return []string{at.origin + ": " + command + " " + parsed.refusal + cannotJudgeBuild}
	}
	if len(parsed.packages) == 0 {
		return []string{at.origin + ": go run names no package, so what it runs cannot be judged"}
	}
	directory := joinPath(at.directory, parsed.directory)
	if parsed.files {
		return a.judgeGoFiles(at.origin+": "+command+" "+strings.Join(parsed.packages, " "), directory, parsed.packages, command == "go test")
	}
	var problems []string
	for _, written := range parsed.packages {
		problems = append(problems, a.judgeGoPackage(at.origin+": "+command+" "+written, command, directory, written)...)
	}
	return problems
}

// goVetProblems judges go vet, which type-checks its packages and runs no code
// of theirs, by the options that would have it run another program: its own
// -vettool, and an option refusedGoFlags names.
func goVetProblems(at reach, words []string) []string {
	for _, word := range words {
		if !strings.HasPrefix(word, "-") {
			continue
		}
		spelled, _, _ := strings.Cut(word, "=")
		name := strings.TrimLeft(spelled, "-")
		if name == goVetTool {
			return []string{at.origin + ": go vet passes " + spelled + ", which runs a program this audit does not read as the analyzer" + cannotJudgeRun}
		}
		if why := refusedGoFlags[name]; why != "" {
			return []string{at.origin + ": go vet passes " + spelled + refusedBecause + why + cannotJudgeRun}
		}
	}
	return nil
}

// goFlagsSetting finds GOFLAGS assigned wherever a text writes it as a word:
// in front of a command, exported, written to the environment file of later
// steps, or handed to go env -w. Group 1 is the value as written, quotes
// included.
var goFlagsSetting = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])GOFLAGS=("[^"\n]*"|'[^'\n]*'|[^\s;&|)\x60]*)`)

// goFlagsItself is GOFLAGS's own value carried into a new one, which adds no
// option to it.
var goFlagsItself = regexp.MustCompile(`^\$(?:GOFLAGS|\{GOFLAGS(?::?-)?\})$`)

// unquote strips the one pair of quotes a shell word is written in.
func unquote(word string) string {
	if len(word) > 1 && (word[0] == '"' || word[0] == '\'') && word[len(word)-1] == word[0] {
		return word[1 : len(word)-1]
	}
	return word
}

// goFlagsProblems holds a value GOFLAGS is set to, label naming where, to the
// options a go command written out is held to: GOFLAGS is a list of options
// every later go command reads before its own. Its own value carried along
// adds nothing, an entry that is not an option is one go refuses to run with,
// and a value only the run knows, a shell expansion or a GoReleaser template,
// cannot be judged.
func goFlagsProblems(label, value string) []string {
	var problems []string
	for word := range strings.FieldsSeq(value) {
		word = strings.Trim(word, `"'`)
		if goFlagsItself.MatchString(word) {
			continue
		}
		if strings.ContainsAny(word, "$`") || strings.Contains(word, goreleaserTemplate) {
			return []string{label + ": GOFLAGS is set to " + value + ", a value only the run knows" + cannotJudgeBuild}
		}
		problems = append(problems, goOptionProblems(label+": GOFLAGS", []string{word})...)
	}
	return problems
}

// goOptionProblems holds standalone options of the go command, label naming
// where they are set, to the ones this audit reads and does not refuse. A word
// that is not an option names nothing to judge, unless it holds a GoReleaser
// template, which the run can fill in with one. A template inside an option's
// value is not refused: GoReleaser hands each flag to go as one argument.
func goOptionProblems(label string, words []string) []string {
	var problems []string
	for _, word := range words {
		if !strings.HasPrefix(word, "-") {
			if strings.Contains(word, goreleaserTemplate) {
				problems = append(problems, label+" holds "+word+", a value only the run knows"+cannotJudgeBuild)
			}
			continue
		}
		spelled, _, _ := strings.Cut(word, "=")
		name := strings.TrimLeft(spelled, "-")
		if why := refusedGoFlags[name]; why != "" {
			problems = append(problems, label+" passes "+spelled+refusedBecause+why+cannotJudgeBuild)
		} else if goFlags[name] == goFlagUnknown {
			problems = append(problems, label+" passes "+spelled+", an option this audit does not read"+cannotJudgeBuild)
		}
	}
	return problems
}

// goFlagsInEnv holds the GOFLAGS an env mapping of a workflow, a job or a step
// sets, label naming the mapping, to what goFlagsProblems holds it to.
func goFlagsInEnv(label string, env any) []string {
	mapping, _ := env.(map[string]any)
	raw, set := mapping["GOFLAGS"]
	if !set {
		return nil
	}
	return goFlagsProblems(label+": env", inputText(raw))
}

// goToolProblems judges go tool: a tool the toolchain ships runs the
// toolchain's own code, and any other name runs a tool a go.mod tool directive
// declares, which is another module's.
func goToolProblems(at reach, words []string) []string {
	takesValue := false
	for _, word := range words {
		if takesValue {
			takesValue = false
			continue
		}
		if word == "-modfile" {
			takesValue = true
			continue
		}
		if strings.HasPrefix(word, "-") {
			continue
		}
		if goToolchainTools[word] {
			return nil
		}
		return []string{at.origin + ": go tool " + word + " is not a tool the Go toolchain ships, so it runs one go.mod " +
			"declares, which is another module's code: " + goCommandReason}
	}
	return nil
}

// unresolvedPackage is the finding a package written in a way this audit
// cannot resolve to a directory carries.
func unresolvedPackage(label string) []string {
	return []string{label + " names a package this audit cannot resolve (a value only the run knows, " +
		"or a path outside this repository)" + cannotJudgeBuild}
}

// otherModules is the finding a package that is not this checkout's carries.
func otherModules(label string) []string {
	return []string{label + " builds packages of other modules, not this checkout's: " + goCommandReason}
}

// unknownWords are the characters only a value the run knows can hold in a
// package: a variable, a substitution, an expression, a glob or a home
// directory.
const unknownWords = "$`{}*?[]~"

// judgeGoPackage judges one package argument of a build-family command, label
// naming the command and the package as written. A package at a version is
// another module's content; one go run or a go install the pinning rules
// refuse is theirs to report. A relative package is resolved from directory, a
// pattern ending in /... to every package below it, and a path of this module
// to its directory; a standard-library package holds nothing to judge, and a
// meta-pattern naming other modules' packages is refused.
func (a *supplyChainAudit) judgeGoPackage(label, command, directory, written string) []string {
	if _, version, versioned := strings.Cut(written, "@"); versioned {
		if pinnedElsewhere(command, version) {
			return nil
		}
		return otherModules(label)
	}
	if strings.ContainsAny(written, unknownWords) || strings.HasPrefix(written, "/") {
		return unresolvedPackage(label)
	}
	modulePath, ok := a.goModulePath()
	if !ok {
		return noModule(label)
	}
	base, ours := packageDirectory(written, directory, modulePath)
	if !ours {
		if !otherModulePatterns[written] && isStandardImport(written) {
			return nil
		}
		return otherModules(label)
	}
	pattern := base == "..." || strings.HasSuffix(base, "/...")
	if pattern {
		base = path.Clean(strings.TrimSuffix(base, "..."))
	}
	if !filepath.IsLocal(filepath.FromSlash(base)) {
		return unresolvedPackage(label)
	}
	tests := command == "go test"
	starts := a.packageStarts(base, pattern, tests)
	if len(starts) == 0 {
		return []string{label + " names no directory holding Go files in this repository" + cannotJudgeBuild}
	}
	return a.goGraphProblems(label, modulePath, starts, tests)
}

// pinnedElsewhere reports whether a package at a version is another rule's
// to judge: go run's is the go-run-module rule's, and a go install at latest
// or at anything but an exact version is the latest or the go-install rule's.
func pinnedElsewhere(command, version string) bool {
	return command == "go "+goRunCommand || (command == "go install" && (version == "latest" || !exactModuleVersion.MatchString(version)))
}

// packageDirectory is the repository-relative directory a package argument of
// this module names, a pattern's /... kept: a relative path read from
// directory, or a path of the module read from the root. ours is false for
// any other import path.
func packageDirectory(written, directory, modulePath string) (base string, ours bool) {
	if written == "." || written == ".." || strings.HasPrefix(written, "./") || strings.HasPrefix(written, "../") {
		return path.Join(directory, written), true
	}
	if written == modulePath || strings.HasPrefix(written, modulePath+"/") {
		return path.Join(".", strings.TrimPrefix(written, modulePath)), true
	}
	return "", false
}

// packageStarts are the packages a package argument builds: every package
// below base for a pattern, base itself otherwise, and none when base holds no
// Go file.
func (a *supplyChainAudit) packageStarts(base string, pattern, tests bool) []goStart {
	if pattern {
		dirs := a.goPatternDirs(base, tests)
		starts := make([]goStart, 0, len(dirs))
		for _, dir := range dirs {
			starts = append(starts, goStart{dir: dir})
		}
		return starts
	}
	if _, found, _ := a.goImports(base, nil, tests); found {
		return []goStart{{dir: base}}
	}
	return nil
}

// judgeGoFiles judges the .go files a go command was handed in place of a
// package: those files alone, read from directory, a _test.go file among them
// only for go test, which tests says.
func (a *supplyChainAudit) judgeGoFiles(label, directory string, files []string, tests bool) []string {
	modulePath, ok := a.goModulePath()
	if !ok {
		return noModule(label)
	}
	byDirectory := map[string][]string{}
	var order []string
	for _, file := range files {
		full := path.Join(directory, file)
		if strings.ContainsAny(file, unknownWords) || !filepath.IsLocal(filepath.FromSlash(full)) {
			return unresolvedPackage(label)
		}
		dir := path.Dir(full)
		if _, seen := byDirectory[dir]; !seen {
			order = append(order, dir)
		}
		byDirectory[dir] = append(byDirectory[dir], path.Base(full))
	}
	starts := make([]goStart, 0, len(order))
	for _, dir := range order {
		starts = append(starts, goStart{dir: dir, files: byDirectory[dir]})
	}
	return a.goGraphProblems(label, modulePath, starts, tests)
}

// noModule is the finding a go command carries when the repository root names
// no module to tell this module's packages from another's.
func noModule(label string) []string {
	return []string{label + " is judged against the module the repository root's go.mod names, and it names none" + cannotJudgeBuild}
}

// goStart is a package a go command builds by name: its repository-relative
// directory, and the files of it go run was handed, none meaning all of them.
type goStart struct {
	dir   string
	files []string
}

// goGraphProblems walks the imports of the packages a go command builds, and
// of every package of this module they reach, and reports what lies outside
// the standard library and this module, each import with the first package,
// in directory order, that makes it. Test files count for the packages go test
// names and for nothing they import, as go builds them.
func (a *supplyChainAudit) goGraphProblems(label, modulePath string, starts []goStart, tests bool) []string {
	walk := importWalk{modulePath: modulePath, visited: map[string]bool{}, outside: map[string]string{}}
	for _, start := range starts {
		walk.visited[start.dir] = true
	}
	round := starts
	for len(round) != 0 {
		var next []goStart
		for _, current := range round {
			imports, _, problem := a.goImports(current.dir, current.files, tests)
			if problem != "" {
				return []string{label + problem}
			}
			reached, missing := a.followImports(&walk, current.dir, imports)
			if missing != "" {
				return []string{label + " imports " + missing + ", which names no directory holding Go files in this repository" + cannotJudgeBuild}
			}
			next = append(next, reached...)
		}
		round, tests = next, false
	}
	if len(walk.outside) == 0 {
		return nil
	}
	return []string{label + " imports " + renderOutside(walk.outside) + ", outside the standard library and this module: " + goCommandReason}
}

// importWalk is one walk through a Go command's imports: the module whose
// packages it follows, the directories it has reached, and each import
// outside the standard library and the module with the package that makes it.
type importWalk struct {
	visited    map[string]bool
	outside    map[string]string
	modulePath string
}

// followImports takes the imports of the package in importer: one outside
// the standard library and the module is recorded with the first package, in
// directory order, that makes it, and a package of the module the walk has
// not reached is returned to be read next. missing names an import of the
// module whose directory holds no Go file.
func (a *supplyChainAudit) followImports(walk *importWalk, importer string, imports []string) (reached []goStart, missing string) {
	for _, imported := range imports {
		if imported != walk.modulePath && !strings.HasPrefix(imported, walk.modulePath+"/") {
			if !isStandardImport(imported) {
				first := importer
				if previous, seen := walk.outside[imported]; seen {
					first = min(first, previous)
				}
				walk.outside[imported] = first
			}
			continue
		}
		dep := path.Join(".", strings.TrimPrefix(imported, walk.modulePath))
		if walk.visited[dep] {
			continue
		}
		walk.visited[dep] = true
		if _, found, _ := a.goImports(dep, nil, false); !found {
			return nil, imported
		}
		reached = append(reached, goStart{dir: dep})
	}
	return reached, ""
}

// renderOutside lists the imports outside the module with the package that
// makes each, in import path order, naming three and counting the rest.
func renderOutside(outside map[string]string) string {
	imports := make([]string, 0, len(outside))
	for imported := range outside {
		imports = append(imports, imported)
	}
	sort.Strings(imports)
	const named = 3
	rendered := make([]string, 0, named)
	for _, imported := range imports[:min(named, len(imports))] {
		rendered = append(rendered, imported+" (from "+outside[imported]+")")
	}
	list := strings.Join(rendered, ", ")
	if rest := len(imports) - named; rest > 0 {
		list += " and " + strconv.Itoa(rest) + " more"
	}
	return list
}

// isStandardImport reports whether an import path names a package of the
// standard library, which by Go's convention is one whose first element holds
// no dot.
func isStandardImport(imported string) bool {
	first, _, _ := strings.Cut(imported, "/")
	return !strings.Contains(first, ".")
}

// goImports returns the imports of the Go files of a repository-relative
// directory, as go reads them: every .go file whose name does not start with
// an underscore or a dot, its _test.go files only when tests says so, and only
// the files named when only names any. found is false when the directory holds
// none; problem names a file that does not parse.
//
// Every file is read whatever its build constraints say, which is the union of
// every platform's build: a job runs on one of them, and reading them all can
// only report an import no build makes, never miss one a build does.
func (a *supplyChainAudit) goImports(dir string, only []string, tests bool) (imports []string, found bool, problem string) {
	entries, _ := os.ReadDir(filepath.Join(a.root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !isGoSource(name, tests) || (only != nil && !slices.Contains(only, name)) {
			continue
		}
		found = true
		file := path.Join(dir, name)
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(a.root, filepath.FromSlash(file)), nil, parser.ImportsOnly)
		if err != nil {
			return nil, true, " reads " + file + ", which does not parse, so what it imports cannot be judged"
		}
		for _, spec := range parsed.Imports {
			imported, _ := strconv.Unquote(spec.Path.Value)
			imports = append(imports, imported)
		}
	}
	return imports, found, ""
}

// isGoSource reports whether go builds a file of that name: a .go file not
// starting with an underscore or a dot, and a _test.go file only for a test.
func isGoSource(name string, tests bool) bool {
	if !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return false
	}
	return tests || !strings.HasSuffix(name, "_test.go")
}

// goPatternDirs returns every directory at or below dir that holds a package,
// skipping what go's ./... skips: a directory whose name starts with a dot or
// an underscore, testdata, vendor, and the root of another module.
func (a *supplyChainAudit) goPatternDirs(dir string, tests bool) []string {
	var dirs []string
	if _, found, _ := a.goImports(dir, nil, tests); found {
		dirs = append(dirs, dir)
	}
	entries, _ := os.ReadDir(filepath.Join(a.root, filepath.FromSlash(dir)))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
			continue
		}
		child := path.Join(dir, name)
		if _, err := os.Stat(filepath.Join(a.root, filepath.FromSlash(child), "go.mod")); err == nil {
			continue
		}
		dirs = append(dirs, a.goPatternDirs(child, tests)...)
	}
	return dirs
}

// goModulePath is the module the repository root's go.mod names, and false
// when there is no go.mod there or it names none.
func (a *supplyChainAudit) goModulePath() (string, bool) {
	text, _ := readRegularFile(filepath.Join(a.root, "go.mod"))
	for _, line := range splitLines(text) {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), true
		}
	}
	return "", false
}
