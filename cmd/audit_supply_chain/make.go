package main

import (
	"path"
	"regexp"
	"strings"
)

// The pieces the two make patterns are built from. makeCommand and
// makeBehindWrapper share the first three, so a place one of them reads as the
// start of a command the other reads the same way.
const (
	// commandStart is where a shell command can start: the start of a line, a
	// separator or the opening of a substitution, or the quote that opens the
	// string a shell is handed with -c. Group 1 captures that quote, so the
	// command can be cut where the string closes.
	commandStart = `(?:^|[;&|(\x60]|\b(?:ba|da|k|z)?sh(?:[ \t]+[-+]o[ \t]+[A-Za-z]+|[ \t]+-[A-Za-z]+)*[ \t]+-[A-Za-z]*c[A-Za-z]*[ \t]+(["']))`

	// commandKeywords are the reserved words, and the brace of a group, that
	// may stand in front of a command, and the @, - and + a recipe line may
	// start with.
	commandKeywords = `[ \t]*(?:(?:!|if|then|do|else|elif|while|until|\{)[ \t]+)*[@+-]*`

	// assignmentWord is a variable assignment in front of a command. Its value
	// is any run of quoted strings, simple substitutions, escaped characters
	// and characters that cannot end a word.
	assignmentWord = `[A-Za-z_][A-Za-z0-9_]*=(?:"(?:[^"\\\n]|\\.)*"|'[^'\n]*'|\$\([^()\n]*\)|\\.|[^ \t\n;&|()\x60"'\\])*`

	// commandWrapper is a program that runs the command written after it,
	// with the options this audit reads it with: env with -i, -0 and -u, exec
	// with -c and -l, command with -p, nohup, time with -p, nice with an
	// adjustment, timeout with its signal options and a duration, and sudo
	// with its flags and a user or group. Any other option leaves the make
	// behind it to makeBehindWrapper.
	commandWrapper = `(?:env(?:[ \t]+(?:-[i0]+|--ignore-environment|--null|-u[ \t]*[A-Za-z_][A-Za-z0-9_]*|--unset=[A-Za-z_][A-Za-z0-9_]*|-))*` +
		`|exec(?:[ \t]+-[cl]+)*|command(?:[ \t]+-p)?|nohup|time(?:[ \t]+-p)?` +
		`|nice(?:[ \t]+(?:-n[ \t]*-?[0-9]+|--adjustment=-?[0-9]+|-[0-9]+))?` +
		`|timeout(?:[ \t]+(?:--preserve-status|--foreground|-v|--verbose|-[sk][ \t]*[A-Z0-9.]+[smhd]?|--(?:signal|kill-after)=[A-Z0-9.]+[smhd]?))*[ \t]+[0-9.]+[smhd]?` +
		`|sudo(?:[ \t]+(?:-[EHnPS]+|-[ug][ \t]*[A-Za-z0-9_-]+))*)`
)

// makeCommand finds make where the shell runs it: in command position, behind
// any variable assignments and wrappers in front of it, and inside the string
// a shell is handed with -c. $(MAKE) and ${MAKE} match wherever they are
// written, since that spelling only ever runs make, including through a shell
// function a recipe wraps it in. A make inside an echo or a quoted suggestion
// is prose and does not match.
var makeCommand = regexp.MustCompile(`(?m)` + commandStart + commandKeywords +
	`(?:(?:` + assignmentWord + `|` + commandWrapper + `)[ \t]+)*make|\$\(MAKE\)|\$\{MAKE\}`)

// makeBehindWrapper finds a wrapper in command position, group 2 naming it, so
// that a make it runs in a spelling makeCommand does not read (env -C, xargs,
// eval, a timeout signal it does not know) is reported rather than passed over.
var makeBehindWrapper = regexp.MustCompile(`(?m)` + commandStart + commandKeywords +
	`(?:` + assignmentWord + `[ \t]+)*(env|exec|command|nohup|time|nice|timeout|sudo|xargs|eval|(?:ba|da|k|z)?sh)(?:[ \t]|$)`)

// makeRuleLine is a rule line of a Makefile: one or more targets, one or two
// colons, and the prerequisites after them, with a recipe after a semicolon.
// A colon followed by an equals sign or a third colon is an assignment (:=,
// ::=, :::=) and does not match.
var makeRuleLine = regexp.MustCompile(`^([A-Za-z0-9_./%-]+(?:[ \t]+[A-Za-z0-9_./%-]+)*)[ \t]*::?(?:([^=:].*))?$`)

// makeConditional is a conditional directive. GNU make reads one inside a
// recipe as part of the rule, so it does not end the recipe it sits in.
var makeConditional = regexp.MustCompile(`^(?:ifeq|ifneq|ifdef|ifndef|else|endif)\b`)

// makeRule is one Makefile target: the targets it depends on and the lines of
// its recipe, merged across every rule that names it.
type makeRule struct {
	prerequisites []string
	recipe        []string
}

// makeInvocation is one make command: why it cannot be followed when it
// cannot, the directories its -C options change into, the makefiles its -f
// options name, and the targets it builds.
type makeInvocation struct {
	refusal     string
	directories []string
	makefiles   []string
	targets     []string
}

// makeRun is where a make command runs its recipes and the makefile it reads
// them from, both repository-relative.
type makeRun struct {
	directory string
	makefile  string
}

// run resolves an invocation against the directory its command runs in. Each
// -C changes into a directory below the one before, as make applies them, and
// the makefile a -f names, or Makefile without one, is read from the last.
func (invocation *makeInvocation) run(directory string) makeRun {
	for _, change := range invocation.directories {
		directory = joinPath(directory, change)
	}
	makefile := path.Join(directory, "Makefile")
	for _, named := range invocation.makefiles {
		makefile = joinPath(directory, named)
	}
	return makeRun{directory: directory, makefile: makefile}
}

// joinPath resolves name against directory, unless name is absolute.
func joinPath(directory, name string) string {
	if path.IsAbs(name) {
		return path.Clean(name)
	}
	return path.Join(directory, name)
}

// makeArgument says what a make option does with an argument.
type makeArgument int

const (
	// takesNothing is a flag.
	takesNothing makeArgument = iota
	// takesWord needs an argument: the rest of its cluster, or the next word.
	takesWord
	// takesInteger takes the rest of its cluster, or the next word when that
	// is all digits, as make reads -j.
	takesInteger
	// takesNumber takes the rest of its cluster, or the next word when that
	// starts with a digit or a dot, as make reads -l.
	takesNumber
	// takesAttached takes only what is attached to it: the rest of its
	// cluster, or what follows an equals sign.
	takesAttached
)

// makeOption is one option of GNU make: the argument it takes and, for the
// three this audit acts on, its role: 'C' changes the directory, 'f' names a
// makefile and 'E' evaluates makefile text.
type makeOption struct {
	argument makeArgument
	role     byte
}

// makeShortOptions are GNU make's single-letter options.
var makeShortOptions = map[byte]makeOption{
	'C': {argument: takesWord, role: 'C'},
	'f': {argument: takesWord, role: 'f'},
	'E': {argument: takesWord, role: 'E'},
	'I': {argument: takesWord},
	'o': {argument: takesWord},
	'W': {argument: takesWord},
	'j': {argument: takesInteger},
	'l': {argument: takesNumber},
	'O': {argument: takesAttached},
	'b': {}, 'B': {}, 'd': {}, 'e': {}, 'h': {}, 'i': {}, 'k': {}, 'L': {}, 'm': {},
	'n': {}, 'p': {}, 'q': {}, 'r': {}, 'R': {}, 's': {}, 'S': {}, 't': {}, 'v': {}, 'w': {},
}

// makeLongOptions are GNU make's long options, spelled out in full: make
// accepts an unambiguous prefix of one too, which this audit does not, and
// reports instead.
var makeLongOptions = map[string]makeOption{
	"directory":       {argument: takesWord, role: 'C'},
	"file":            {argument: takesWord, role: 'f'},
	"makefile":        {argument: takesWord, role: 'f'},
	"eval":            {argument: takesWord, role: 'E'},
	"include-dir":     {argument: takesWord},
	"old-file":        {argument: takesWord},
	"assume-old":      {argument: takesWord},
	"what-if":         {argument: takesWord},
	"new-file":        {argument: takesWord},
	"assume-new":      {argument: takesWord},
	"jobserver-style": {argument: takesWord},
	"jobs":            {argument: takesInteger},
	"load-average":    {argument: takesNumber},
	"max-load":        {argument: takesNumber},
	"debug":           {argument: takesAttached},
	"output-sync":     {argument: takesAttached},
	"shuffle":         {argument: takesAttached},

	"always-make": {}, "environment-overrides": {}, "help": {}, "ignore-errors": {},
	"keep-going": {}, "check-symlink-times": {}, "just-print": {}, "dry-run": {},
	"recon": {}, "print-data-base": {}, "question": {}, "no-builtin-rules": {},
	"no-builtin-variables": {}, "silent": {}, "quiet": {}, "no-silent": {},
	"no-keep-going": {}, "stop": {}, "touch": {}, "trace": {}, "version": {},
	"print-directory": {}, "no-print-directory": {}, "warn-undefined-variables": {},
}

// take returns the word after index when an option that is not given its
// argument attached takes it, with the index of the last word used; a flag and
// an option that takes only an attached argument take none. It reports false
// when the option needs an argument and no word is left.
func (argument makeArgument) take(words []string, index int) (value string, last int, ok bool) {
	if index+1 >= len(words) {
		return "", index, argument != takesWord
	}
	following := words[index+1]
	integer := argument == takesInteger && strings.Trim(following, "0123456789") == ""
	number := argument == takesNumber && following != "" && strings.IndexByte("0123456789.", following[0]) >= 0
	if argument == takesWord || integer || number {
		return following, index + 1, true
	}
	return "", index, true
}

// scanMake returns every make command a text runs, and the wrappers that run a
// make this audit cannot read, once per make.
//
// A line continued with a backslash is joined first, as the shell joins it.
func scanMake(text string) (invocations []makeInvocation, unread []string) {
	text = strings.ReplaceAll(text, "\\\n", " ")
	read := map[int]bool{}
	for _, location := range makeCommand.FindAllStringSubmatchIndex(text, -1) {
		rest := text[location[1]:]
		if continuesWord(rest) {
			continue
		}
		read[location[1]] = true
		if location[2] != -1 {
			rest = cutAtQuote(rest, text[location[2]])
		}
		invocations = append(invocations, parseMakeArguments(commandWords(rest)))
	}
	return invocations, wrappersUnread(text, read)
}

// wrappersUnread names the wrapper in front of each make that makeCommand did
// not read, read holding where each make it read ends.
//
// command -v make asks whether make exists and runs nothing, so it is not one.
func wrappersUnread(text string, read map[int]bool) []string {
	var unread []string
	for _, location := range makeBehindWrapper.FindAllStringSubmatchIndex(text, -1) {
		wrapper := text[location[4]:location[5]]
		from, to := location[5], len(text)
		if end := strings.IndexAny(text[from:], "\n;&|)\x60"); end >= 0 {
			to = from + end
		}
		if fields := strings.Fields(text[from:to]); wrapper == "command" && len(fields) > 0 && strings.EqualFold(fields[0], "-v") {
			continue
		}
		for _, end := range makeWordEnds(text, from, to) {
			if !read[end] {
				read[end] = true
				unread = append(unread, wrapper)
			}
		}
	}
	return unread
}

// makeWordEnds returns where each make between from and to in a text ends: the
// word itself, not part of cmake, makefile or make-thing. from is past the
// wrapper in front of the command, so a character always precedes it.
func makeWordEnds(text string, from, to int) []int {
	var ends []int
	for offset := from; ; {
		index := strings.Index(text[offset:to], "make")
		if index < 0 {
			return ends
		}
		start := offset + index
		offset = start + len("make")
		if !partOfWord(text[start-1]) && !continuesWord(text[offset:to]) {
			ends = append(ends, offset)
		}
	}
}

// partOfWord reports whether a character next to make makes it part of a
// longer word.
func partOfWord(char byte) bool {
	return strings.IndexByte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./-$", char) >= 0
}

// continuesWord reports whether the text after a make continues it into a
// longer word: makefile, make-thing, make.log, make=1 and make: are not make.
func continuesWord(rest string) bool {
	return rest != "" && (partOfWord(rest[0]) || rest[0] == '=' || rest[0] == ':')
}

// cutAtQuote cuts a command at the quote that closes the string a shell was
// handed it in, when the string closes on the same line.
func cutAtQuote(rest string, quote byte) string {
	if end := closingQuote(rest, 0, quote); end >= 0 {
		return rest[:end]
	}
	return rest
}

// closingQuote returns where the quote that closes a string starting at from
// sits, or -1 when the line ends first. A backslash escapes the next character
// inside double quotes and nowhere inside single ones.
func closingQuote(text string, from int, quote byte) int {
	for index := from; index < len(text); index++ {
		switch text[index] {
		case quote:
			return index
		case '\n':
			return -1
		case '\\':
			if quote == '"' {
				index++
			}
		}
	}
	return -1
}

// substitutionEnd returns where the command substitution whose opening
// parenthesis is at open ends: just past its closing parenthesis, or at the end
// of its line when nothing closes it there.
func substitutionEnd(text string, open int) int {
	depth := 0
	for index := open; index < len(text); index++ {
		switch text[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index + 1
			}
		case '\n':
			return index
		}
	}
	return len(text)
}

// commandWords splits what follows make into the words of its command, the way
// the shell would: up to the first separator outside quotes, with the quotes
// removed and a command substitution kept whole.
//
// A quote that is not where a word or an assigned value starts, or that
// nothing closes before the line ends, closes the string the command was
// written inside, as in sh -c "cd site && make build", and ends the command.
// So does a comment.
func commandWords(rest string) []string {
	var words []string
	var word strings.Builder
	started := false
	finish := func() {
		if started {
			words = append(words, word.String())
		}
		word.Reset()
		started = false
	}
	for index := 0; index < len(rest); index++ {
		char := rest[index]
		if char == ' ' || char == '\t' {
			finish()
			continue
		}
		if strings.IndexByte("\n;&|)\x60", char) >= 0 || (char == '#' && !started) {
			finish()
			return words
		}
		if char == '"' || char == '\'' {
			closing := closingQuote(rest, index+1, char)
			if closing == -1 || (started && !strings.HasSuffix(word.String(), "=")) {
				finish()
				return words
			}
			word.WriteString(rest[index+1 : closing])
			index = closing
			started = true
			continue
		}
		started = true
		if strings.HasPrefix(rest[index:], "$(") {
			end := substitutionEnd(rest, index+1)
			word.WriteString(rest[index:end])
			index = end - 1
			continue
		}
		if char == '\\' && index+1 < len(rest) {
			index++
		}
		word.WriteByte(rest[index])
	}
	finish()
	return words
}

// parseMakeArguments reads the words of a make command as GNU make reads its
// arguments: the options wherever they sit until --, a word with an equals sign
// as a variable assignment, and every other word as a target.
//
// An option this audit does not read, one missing its argument, --eval, and
// more than one makefile each leave a refusal instead of something to follow.
func parseMakeArguments(words []string) makeInvocation {
	var invocation makeInvocation
	options := true
	for index := 0; index < len(words) && invocation.refusal == ""; index++ {
		word := words[index]
		if options && word == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(word, "--") {
			index = invocation.longOption(words, index)
			continue
		}
		if options && len(word) > 1 && word[0] == '-' {
			index = invocation.shortOptions(words, index)
			continue
		}
		if !strings.Contains(word, "=") {
			invocation.targets = append(invocation.targets, word)
		}
	}
	if invocation.refusal == "" && len(invocation.makefiles) > 1 {
		invocation.refusal = "make reads more than one makefile (" + strings.Join(invocation.makefiles, ", ") +
			"), which this audit does not merge"
	}
	return invocation
}

// longOption reads the long option at index and returns the index of the last
// word it used.
func (invocation *makeInvocation) longOption(words []string, index int) int {
	name, value, attached := strings.Cut(words[index][2:], "=")
	option, known := makeLongOptions[name]
	if !known {
		invocation.refusal = "make passes --" + name + ", an option this audit does not read"
		return index
	}
	return invocation.option(option, "--"+name, value, attached, words, index)
}

// shortOptions reads the cluster of single-letter options at index and returns
// the index of the last word it used. The first option in the cluster that
// takes an argument takes the rest of the cluster as it.
func (invocation *makeInvocation) shortOptions(words []string, index int) int {
	cluster := words[index]
	for position := 1; position < len(cluster); position++ {
		letter := cluster[position]
		option, known := makeShortOptions[letter]
		if !known {
			invocation.refusal = "make passes -" + string(letter) + ", an option this audit does not read"
			return index
		}
		if option.argument != takesNothing {
			value := cluster[position+1:]
			return invocation.option(option, "-"+string(letter), value, value != "", words, index)
		}
	}
	return index
}

// option applies one option given the argument attached to it, if any, taking
// the next word when the option takes one, and returns the index of the last
// word it used.
func (invocation *makeInvocation) option(option makeOption, spelled, value string, attached bool, words []string, index int) int {
	if !attached {
		var ok bool
		value, index, ok = option.argument.take(words, index)
		if !ok {
			invocation.refusal = "make passes " + spelled + " without the argument it takes"
			return index
		}
	}
	switch option.role {
	case 'C':
		invocation.directories = append(invocation.directories, value)
	case 'f':
		invocation.makefiles = append(invocation.makefiles, value)
	case 'E':
		invocation.refusal = "make passes " + spelled + ", which adds makefile text this audit does not read"
	}
	return index
}

// parseMakefile reads a Makefile into its rules.
//
// A rule line opens a rule; every line after it that starts with a tab, or
// continues a recipe line ending in a backslash, belongs to its recipe, and so
// does what follows a semicolon on the rule line. Any other line ending in a
// backslash is joined to the next first, as make joins it, so a rule line's
// prerequisites may run on. A blank line, a comment and a conditional
// directive leave the rule open, as they do for make. Any other line, an
// assignment or another directive, closes it.
func parseMakefile(text string) map[string]*makeRule {
	rules := map[string]*makeRule{}
	var current []*makeRule
	lines := splitLines(text)
	continued := false
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		if continued || strings.HasPrefix(line, "\t") {
			for _, rule := range current {
				rule.recipe = append(rule.recipe, line)
			}
			continued = strings.HasSuffix(line, "\\")
			continue
		}
		for strings.HasSuffix(line, "\\") && index+1 < len(lines) {
			index++
			line = strings.TrimSuffix(line, "\\") + " " + lines[index]
		}
		if !leavesRuleOpen(line) {
			current = openRules(rules, line)
		}
	}
	return rules
}

// leavesRuleOpen reports whether a line outside a recipe runs nothing and
// leaves the rule before it open: a blank line, a comment, or a conditional
// directive.
func leavesRuleOpen(line string) bool {
	return strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") || makeConditional.MatchString(line)
}

// openRules reads a line that is not part of a recipe: the rules it opens, or
// none when it is not a rule line.
func openRules(rules map[string]*makeRule, line string) []*makeRule {
	groups := makeRuleLine.FindStringSubmatch(line)
	if groups == nil {
		return nil
	}
	prerequisites, recipe, inline := strings.Cut(groups[2], ";")
	var opened []*makeRule
	for target := range strings.FieldsSeq(groups[1]) {
		rule := rules[target]
		if rule == nil {
			rule = &makeRule{}
			rules[target] = rule
		}
		rule.prerequisites = append(rule.prerequisites, strings.Fields(prerequisites)...)
		if inline {
			rule.recipe = append(rule.recipe, recipe)
		}
		opened = append(opened, rule)
	}
	return opened
}
