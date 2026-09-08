package apiexposes

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// entityRoot is the namespace every entity lives under. A class declared
// anywhere else, a helper or a value object, is not an entity and is skipped.
var entityRoot = []string{"API", "Entities"}

// enterprisePrefix is the module Enterprise prepends are declared under: the
// target of EE::API::Entities::Project is API::Entities::Project.
const enterprisePrefix = "EE"

// Statement shapes, matched against a line with its comment stripped and its
// space trimmed.
var (
	moduleLine    = regexp.MustCompile(`^module\s+([A-Z][\w:]*)\s*$`)
	classLine     = regexp.MustCompile(`^class\s+([A-Z][\w:]*)(?:\s*<\s*((?:::)?[A-Z][\w:]*))?\s*$`)
	singletonLine = regexp.MustCompile(`^class\s*<<`)
	defLine       = regexp.MustCompile(`^def\s`)
	// endlessDef is `def name(args) = expr`, which opens nothing.
	endlessDef = regexp.MustCompile(`^def\s+[\w?!.]+(?:\([^)]*\))?\s*=`)
	// keywordOpener is a statement Ruby closes with `end`: a keyword at the
	// start of the line, or one used as an expression after an assignment, a
	// parenthesis or a comma (`@diffs ||= if ... end`). A modifier `if` after
	// a complete expression (`return x if y`) follows neither and is not one.
	// doOpener is a `do` at the end of the line, with or without block
	// parameters.
	keywordOpener = regexp.MustCompile(`(?:^|[=(,]\s*)(if|unless|case|begin|while|until|for)\b`)
	doOpener      = regexp.MustCompile(`(^|\s)do(\s*\|[^|]*\|)?\s*$`)
	// doValueBlock and braceValueBlock are blocks with parameters, which
	// compute the field rather than nesting others under it. The `do |obj|`
	// form runs to an `end` on a later line and opens a frame; the
	// `{ |obj, opts| ... }` form is closed inside the statement itself, since
	// the statement was joined until its braces balanced, and opens nothing.
	doValueBlock    = regexp.MustCompile(`\bdo\s*\|[^|]*\|\s*$`)
	braceValueBlock = regexp.MustCompile(`\{\s*\|`)
	// lambdaHead is a lambda whose body follows as a `do ... end` block:
	// `->(obj, opts)` or `lambda` at the end of what precedes the `do`.
	lambdaHead = regexp.MustCompile(`(->\s*\([^)]*\)|\blambda)\s*$`)
	optionPair = regexp.MustCompile(`^([a-z_]+):\s*(.*)$`)
)

// Parse reads every entity declared in files, keyed by repository-relative
// path, and returns them keyed by OpenAPI name with every condition's licensed
// features resolved against the feature table. exposes counts the fields
// read, nested ones included, which is the number a regeneration prints so a
// reader can see whether the source still looks like GitLab's.
func Parse(files map[string][]byte, features map[string]string) (entities map[string]Entity, exposes int, err error) {
	p := &parser{entities: map[string]*decl{}, prepends: map[string]*prepend{}}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if fileErr := p.file(path, files[path]); fileErr != nil {
			return nil, 0, fileErr
		}
	}
	return p.finish(features)
}

// decl is an entity while its references are still the literals the source
// wrote.
type decl struct {
	rubyPath  string
	namespace []string
	parentRef string
	Entity
}

// prepend is the Enterprise module prepended into one entity.
type prepend struct {
	file   string
	fields []Field
}

// parser gathers the declarations of every file before resolving anything,
// because a parent or a used entity is as often declared in another file as
// in the same one.
type parser struct {
	entities map[string]*decl
	prepends map[string]*prepend
	files    int
}

// frameKind says what a block on the stack is to an expose inside it.
type frameKind int

const (
	// frameModule is a module: a namespace segment and nothing else.
	frameModule frameKind = iota
	// frameClass is an entity, whose top-level exposes are its fields.
	frameClass
	// framePrepended is `prepended do` in an Enterprise module: its exposes
	// are the target entity's fields.
	framePrepended
	// frameNesting is `expose :x do ... end` without block parameters: its
	// exposes are x's nested fields.
	frameNesting
	// frameScope is `with_options` or a statement-level `if`, carrying a
	// condition every expose inside inherits.
	frameScope
	// frameSkip is a body no expose can be declared in: a method, a value
	// block, or any other block.
	frameSkip
)

type frame struct {
	kind frameKind
	name string
	// fields is where an expose directly inside the frame lands.
	fields *[]Field
	// condIf and condUnless are a scope's conditions.
	condIf, condUnless string
	// reopened marks a module frame opened by a `class X` with no
	// superclass: a namespace reopened to declare something inside it, whose
	// own exposes would belong to an entity declared elsewhere.
	reopened bool
}

// describe names a frame for an error.
func (f frame) describe() string {
	switch f.kind {
	case frameModule:
		return "module " + f.name
	case frameClass:
		return "class " + f.name
	case framePrepended:
		return "prepended block"
	case frameNesting:
		return "nesting expose block"
	case frameScope:
		return "with_options or if scope"
	default:
		return "method or value block"
	}
}

// scanner walks one file.
type scanner struct {
	p       *parser
	path    string
	edition string
	lines   []string
	pos     int
	frames  []frame
}

// file walks one file's statements.
func (p *parser) file(path string, src []byte) error {
	p.files++
	s := &scanner{p: p, path: path, lines: strings.Split(string(src), "\n")}
	if strings.HasPrefix(path, "ee/") {
		s.edition = "ee"
	}
	for s.pos < len(s.lines) {
		lineNo := s.pos + 1
		line := strings.TrimSpace(stripComment(s.lines[s.pos]))
		s.pos++
		if line == "" {
			continue
		}
		if err := s.statement(line, lineNo); err != nil {
			return fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
	}
	if len(s.frames) != 0 {
		return fmt.Errorf("%s: %d block(s) left open at the end of the file, the last a %s", path, len(s.frames), s.frames[len(s.frames)-1].describe())
	}
	return nil
}

// statement classifies one line and updates the stack.
func (s *scanner) statement(line string, lineNo int) error {
	switch {
	case isEnd(line):
		if len(s.frames) == 0 {
			return errors.New("end with nothing open")
		}
		s.frames = s.frames[:len(s.frames)-1]
		return nil
	case strings.HasPrefix(line, "expose ") || strings.HasPrefix(line, "expose("):
		return s.expose(line, lineNo)
	case strings.HasPrefix(line, "with_options"):
		opts := parseOptions(argsOf(strings.TrimPrefix(s.join(line), "with_options")))
		s.push(frame{kind: frameScope, condIf: opts["if"], condUnless: opts["unless"]})
		return nil
	case line == "prepended do":
		s.prepended()
		return nil
	}
	if handled, err := s.declaration(line, lineNo); handled || err != nil {
		return err
	}
	s.block(line)
	return nil
}

// isEnd reports whether a line closes a block: `end` alone, or `end` with a
// method call or a closing parenthesis after it.
func isEnd(line string) bool {
	return line == "end" || strings.HasPrefix(line, "end.") || strings.HasPrefix(line, "end)")
}

// declaration handles a module, class or method declaration, reporting
// whether the line was one.
func (s *scanner) declaration(line string, lineNo int) (bool, error) {
	switch {
	case singletonLine.MatchString(line):
		s.push(frame{kind: frameSkip})
	case strings.HasPrefix(line, "module "):
		match := moduleLine.FindStringSubmatch(line)
		if match == nil {
			return true, fmt.Errorf("module declaration this reader does not understand: %q", line)
		}
		s.push(frame{kind: frameModule, name: match[1]})
	case strings.HasPrefix(line, "class "):
		// A class this reader cannot name would leave its `end` unmatched
		// and every frame after it off by one, so it is an error rather than
		// a line to pass over.
		match := classLine.FindStringSubmatch(line)
		if match == nil {
			return true, fmt.Errorf("class declaration this reader does not understand: %q", line)
		}
		if err := s.class(match, lineNo); err != nil {
			return true, err
		}
	case defLine.MatchString(line):
		if !endlessDef.MatchString(line) && !strings.HasSuffix(line, " end") {
			s.push(frame{kind: frameSkip})
		}
	default:
		return false, nil
	}
	return true, nil
}

// block handles a line that opens a block Ruby closes with `end`: a
// statement-level `if` or `unless` directly in an entity is a scope whose
// condition its exposes inherit; every other block is skipped whole.
func (s *scanner) block(line string) {
	switch {
	case keywordOpener.MatchString(line):
		keyword, condition, _ := strings.Cut(line, " ")
		switch {
		case s.inSkip() || keyword != "if" && keyword != "unless":
			s.push(frame{kind: frameSkip})
		case keyword == "if":
			s.push(frame{kind: frameScope, condIf: condition})
		default:
			s.push(frame{kind: frameScope, condUnless: condition})
		}
	case doOpener.MatchString(line):
		s.push(frame{kind: frameSkip})
	}
}

// prepended opens the block an Enterprise module adds to a Community entity,
// or a skipped one when the module is not an entity's.
func (s *scanner) prepended() {
	target := s.prependTarget()
	if target == "" {
		s.push(frame{kind: frameSkip})
		return
	}
	bucket := s.p.prepends[target]
	if bucket == nil {
		bucket = &prepend{file: s.path}
		s.p.prepends[target] = bucket
	}
	s.push(frame{kind: framePrepended, fields: &bucket.fields})
}

// class opens an entity when the class sits under API::Entities and names a
// superclass, and a skipped body outside API::Entities. Ruby lets a class be
// opened again, and GitLab's entity files do it two ways. With no
// superclass (`class Result`, in ci/lint/result/include.rb) it is a
// namespace reopened to declare something inside it: a Grape entity always
// names Grape::Entity or another entity as its parent, and reading that line
// as an entity replaced the one result.rb declares with an empty one. With
// the same superclass (feature_flag/basic_user_list.rb) it is the same
// entity opened again: an entity declared inside it is named under it, and
// an expose there joins the entity's fields after those of the file read
// before it. A second declaration naming another parent is refused, since it
// is not the same class and one of the two would silently replace the other.
func (s *scanner) class(match []string, lineNo int) error {
	name, parent := match[1], match[2]
	namespace := s.namespace()
	if !isEntityNamespace(namespace) {
		s.push(frame{kind: frameSkip})
		return nil
	}
	if parent == "" {
		s.push(frame{kind: frameModule, name: name, reopened: true})
		return nil
	}
	if parent == "Grape::Entity" || parent == "::Grape::Entity" {
		parent = ""
	}
	rubyPath := strings.Join(append(append([]string{}, namespace...), name), "::")
	if existing := s.p.entities[rubyPath]; existing != nil {
		if existing.parentRef != parent {
			return fmt.Errorf("class %s declared twice with another parent, first at %s:%d", rubyPath, existing.File, existing.Line)
		}
		s.push(frame{kind: frameClass, name: name, fields: &existing.Fields})
		return nil
	}
	d := &decl{rubyPath: rubyPath, namespace: namespace, parentRef: parent}
	d.File = s.path
	d.Line = lineNo
	d.Edition = s.edition
	// An entity exposing nothing of its own (one that only inherits) is
	// written as an empty list rather than null, so a reader iterating fields
	// needs no guard.
	d.Fields = []Field{}
	s.p.entities[rubyPath] = d
	s.push(frame{kind: frameClass, name: name, fields: &d.Fields})
	return nil
}

// blockKind says what follows an expose's arguments.
type blockKind int

const (
	// blockNone is no block, or one closed inside the statement.
	blockNone blockKind = iota
	// blockValue is `do |obj| ... end`, computing the field.
	blockValue
	// blockNesting is `do ... end` without parameters, nesting fields.
	blockNesting
	// blockLambda is `if: ->(obj) do ... end`, the condition's own body.
	blockLambda
)

// expose reads one expose statement, however many lines it spans, and files
// its fields where the stack says they belong.
func (s *scanner) expose(line string, lineNo int) error {
	stmt := s.join(line)
	fields := s.currentFields()
	if fields == nil && s.inReopenedClass() {
		// The fields belong to an entity declared in another file, and
		// attaching them there would need Ruby's load order, which this
		// reader does not have.
		return errors.New("expose under a class with no superclass: an entity reopened to add fields is not read")
	}
	rest := strings.TrimPrefix(strings.TrimPrefix(stmt, "expose"), "(")
	if strings.HasPrefix(stmt, "expose(") {
		// Parenthesized arguments: the block, if any, follows the paren.
		closing := matchingClose(rest)
		if closing < 0 {
			return errors.New("expose with an unclosed parenthesis")
		}
		rest = rest[:closing] + " " + rest[closing+1:]
	}
	rest, block := blockShape(rest)
	lambdaBody := ""
	if block == blockLambda {
		lambdaBody = s.blockBody()
	}
	names, opts := parseArguments(rest)
	if len(names) == 0 {
		return fmt.Errorf("expose without a field name: %q", stmt)
	}
	if lambdaBody != "" {
		for _, key := range []string{"if", "unless"} {
			if lambdaHead.MatchString(opts[key]) {
				opts[key] += " do " + lambdaBody + " end"
			}
		}
	}

	var declared []Field
	for _, name := range names {
		field := Field{Name: name, Line: lineNo, If: opts["if"], Unless: opts["unless"], Using: firstOf(opts, "using", "with")}
		field.Splat = strings.HasPrefix(name, "*")
		if alias := opts["as"]; alias != "" && len(names) == 1 {
			field.Name = symbolName(alias)
		}
		field.Merge = opts["merge"] == "true"
		field.If = JoinIf(s.scopeCondition("if"), field.If)
		field.Unless = JoinUnless(s.scopeCondition("unless"), field.Unless)
		declared = append(declared, field)
	}

	switch {
	case block == blockValue || block == blockNesting && fields == nil:
		s.push(frame{kind: frameSkip})
	case block == blockNesting:
		*fields = append(*fields, declared...)
		last := &(*fields)[len(*fields)-1]
		s.push(frame{kind: frameNesting, fields: &last.Nested})
		return nil
	}
	if fields != nil {
		*fields = append(*fields, declared...)
	}
	return nil
}

// blockShape says what block, if any, ends an expose's arguments, and
// returns the arguments without it.
func blockShape(rest string) (string, blockKind) {
	if at := doValueBlock.FindStringIndex(rest); at != nil {
		return rest[:at[0]], blockValue
	}
	if at := braceValueBlock.FindStringIndex(rest); at != nil {
		return rest[:at[0]], blockNone
	}
	at := doOpener.FindStringIndex(rest)
	if at == nil {
		return rest, blockNone
	}
	if lambdaHead.MatchString(rest[:at[0]]) {
		return rest[:at[0]], blockLambda
	}
	return rest[:at[0]], blockNesting
}

// join reads the rest of a statement that continues on the following lines:
// while a bracket is open or the line ends with a comma or a backslash, the
// next line is part of it.
func (s *scanner) join(line string) string {
	stmt := line
	for (!balanced(stmt) || strings.HasSuffix(stmt, ",") || strings.HasSuffix(stmt, `\`)) && s.pos < len(s.lines) {
		stmt = strings.TrimSuffix(stmt, `\`)
		next := strings.TrimSpace(stripComment(s.lines[s.pos]))
		s.pos++
		if next == "" {
			continue
		}
		stmt += " " + next
	}
	return stmt
}

// blockBody reads the lines of a block opened by the statement just read, up
// to the `end` that closes it, and returns them joined as one line. Every
// opener inside is tracked so a nested `if` or `do` does not end the block
// early.
func (s *scanner) blockBody() string {
	var body []string
	depth := 1
	for s.pos < len(s.lines) {
		line := strings.TrimSpace(stripComment(s.lines[s.pos]))
		s.pos++
		if line == "" {
			continue
		}
		switch {
		case isEnd(line):
			depth--
			if depth == 0 {
				return strings.Join(body, "; ")
			}
		case keywordOpener.MatchString(line) || doOpener.MatchString(line) || defLine.MatchString(line):
			depth++
		}
		body = append(body, line)
	}
	return strings.Join(body, "; ")
}

// push opens a frame.
func (s *scanner) push(f frame) { s.frames = append(s.frames, f) }

// inSkip reports whether the statement sits inside a body no expose can be
// declared in.
func (s *scanner) inSkip() bool {
	return slices.ContainsFunc(s.frames, func(f frame) bool { return f.kind == frameSkip })
}

// inReopenedClass reports whether the statement sits directly in a class
// declared without a superclass, scopes aside.
func (s *scanner) inReopenedClass() bool {
	for _, f := range slices.Backward(s.frames) {
		switch f.kind {
		case frameModule:
			return f.reopened
		case frameScope:
			continue
		default:
			return false
		}
	}
	return false
}

// currentFields is where an expose at this point lands, or nil when it is
// inside a skipped body or outside any entity.
func (s *scanner) currentFields() *[]Field {
	for _, f := range slices.Backward(s.frames) {
		switch f.kind {
		case frameSkip:
			return nil
		case frameClass, framePrepended, frameNesting:
			return f.fields
		}
	}
	return nil
}

// scopeCondition joins the conditions of every enclosing scope, outermost
// first, for the key asked for: `if:` scopes all have to hold, `unless:`
// scopes omit the field when any holds.
func (s *scanner) scopeCondition(key string) string {
	joined := ""
	for _, f := range s.frames {
		if f.kind != frameScope {
			continue
		}
		if key == "unless" {
			joined = JoinUnless(joined, f.condUnless)
			continue
		}
		joined = JoinIf(joined, f.condIf)
	}
	return joined
}

// namespace is the constant path the stack spells: every module and class
// open at this point.
func (s *scanner) namespace() []string {
	var names []string
	for _, f := range s.frames {
		if f.kind == frameModule || f.kind == frameClass {
			names = append(names, strings.Split(f.name, "::")...)
		}
	}
	return names
}

// prependTarget is the entity an Enterprise module's `prepended do` adds to:
// the module path with its leading EE removed, or "" when the module is not
// an entity's.
func (s *scanner) prependTarget() string {
	namespace := s.namespace()
	if len(namespace) == 0 || namespace[0] != enterprisePrefix {
		return ""
	}
	target := namespace[1:]
	if !isEntityNamespace(target) {
		return ""
	}
	return strings.Join(target, "::")
}

// isEntityNamespace reports whether a path sits under API::Entities.
func isEntityNamespace(namespace []string) bool {
	if len(namespace) < len(entityRoot) {
		return false
	}
	for i, segment := range entityRoot {
		if namespace[i] != segment {
			return false
		}
	}
	return true
}

// finish resolves every reference against the declarations of every file,
// applies the Enterprise prepends, reads the licensed features out of each
// condition, and keys the result by OpenAPI name.
func (p *parser) finish(features map[string]string) (entities map[string]Entity, exposes int, err error) {
	for target, bucket := range p.prepends {
		d := p.entities[target]
		if d == nil {
			d = &decl{rubyPath: target, namespace: strings.Split(target, "::")[:strings.Count(target, "::")]}
			d.File = bucket.file
			d.Edition = "ee"
			p.entities[target] = d
		}
		for _, field := range bucket.fields {
			field.File = bucket.file
			field.Edition = "ee"
			d.Fields = append(d.Fields, field)
		}
	}

	entities = make(map[string]Entity, len(p.entities))
	for rubyPath, d := range p.entities {
		if d.parentRef != "" {
			d.Parent = OpenAPIName(p.resolve(d.parentRef, d.namespace))
		}
		scope := append(append([]string{}, d.namespace...), strings.Split(rubyPath, "::")[len(d.namespace):]...)
		exposes += p.resolveFields(d.Fields, scope, features)
		entities[OpenAPIName(rubyPath)] = d.Entity
	}
	if len(entities) == 0 {
		return nil, 0, fmt.Errorf("no entity declared under API::Entities in %d file(s)", p.files)
	}
	return entities, exposes, nil
}

// resolveFields resolves the used entities and reads the licensed features of
// every field, nested ones included, and returns how many there were.
func (p *parser) resolveFields(fields []Field, scope []string, features map[string]string) int {
	count := 0
	for i := range fields {
		field := &fields[i]
		count++
		if field.Using != "" {
			field.Using = OpenAPIName(p.resolve(field.Using, scope))
		}
		field.Features = licensedFeatures(field.If + " " + field.Unless)
		field.Tier = tierOf(field.Features, features)
		count += p.resolveFields(field.Nested, scope, features)
	}
	return count
}

// resolve turns a constant reference into the path of a declared entity, the
// way Ruby's lexical lookup would: an absolute reference as written, and a
// relative one tried from the innermost enclosing namespace outward, with
// Entities::X read as API::Entities::X. A reference nothing declares is
// returned as written, without its leading colons, so the record says what
// the source said rather than nothing.
func (p *parser) resolve(ref string, scope []string) string {
	if absolute, ok := strings.CutPrefix(ref, "::"); ok {
		return absolute
	}
	for i := len(scope); i >= 0; i-- {
		candidate := strings.Join(append(append([]string{}, scope[:i]...), ref), "::")
		if _, ok := p.entities[candidate]; ok {
			return candidate
		}
	}
	if strings.HasPrefix(ref, "Entities::") {
		return "API::" + ref
	}
	return ref
}

// argsOf returns the arguments of a call written with or without parentheses,
// up to a trailing `do`.
func argsOf(rest string) string {
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, "(") {
		if closing := matchingClose(rest[1:]); closing >= 0 {
			return rest[1 : closing+1]
		}
	}
	if at := doOpener.FindStringIndex(rest); at != nil {
		rest = rest[:at[0]]
	}
	return rest
}

// parseArguments reads an expose's arguments: the positional symbols are the
// field names, the `key: value` pairs its options.
func parseArguments(args string) (names []string, opts map[string]string) {
	opts = map[string]string{}
	for _, token := range splitTopLevel(args) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if match := optionPair.FindStringSubmatch(token); match != nil {
			opts[match[1]] = strings.TrimSpace(match[2])
			continue
		}
		names = append(names, symbolName(token))
	}
	return names, opts
}

// parseOptions reads only the options of a call's arguments.
func parseOptions(args string) map[string]string {
	_, opts := parseArguments(args)
	return opts
}

// symbolName reads a field name out of a symbol or a string literal.
func symbolName(token string) string {
	token = strings.TrimPrefix(strings.TrimSpace(token), ":")
	return strings.Trim(token, `'"`)
}

// firstOf returns the first option present among the keys.
func firstOf(opts map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := opts[key]; value != "" {
			return value
		}
	}
	return ""
}

// outsideQuotes calls visit for every byte of text that is not inside a
// string literal, with its index, stopping when visit returns false. A quote
// is opened by ' or " and closed by the same character, and a backslash
// escapes the byte after it.
func outsideQuotes(text string, visit func(i int, c byte) bool) {
	quote := byte(0)
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote = c
			continue
		}
		if !visit(i, c) {
			return
		}
	}
}

// depthChange is what a bracket does to the nesting depth.
func depthChange(c byte) int {
	switch c {
	case '(', '[', '{':
		return 1
	case ')', ']', '}':
		return -1
	default:
		return 0
	}
}

// splitTopLevel splits on the commas outside brackets and quotes.
func splitTopLevel(text string) []string {
	var parts []string
	depth, start := 0, 0
	outsideQuotes(text, func(i int, c byte) bool {
		depth += depthChange(c)
		if c == ',' && depth == 0 {
			parts = append(parts, text[start:i])
			start = i + 1
		}
		return true
	})
	return append(parts, text[start:])
}

// balanced reports whether every bracket opened in text is closed, outside
// quotes.
func balanced(text string) bool {
	depth := 0
	outsideQuotes(text, func(_ int, c byte) bool {
		depth += depthChange(c)
		return true
	})
	return depth <= 0
}

// matchingClose returns the index in text of the parenthesis closing one
// opened just before it, or -1.
func matchingClose(text string) int {
	depth, found := 1, -1
	outsideQuotes(text, func(i int, c byte) bool {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				found = i
				return false
			}
		}
		return true
	})
	return found
}

// stripComment removes a trailing Ruby comment, leaving a # inside a string.
func stripComment(line string) string {
	cut := len(line)
	outsideQuotes(line, func(i int, c byte) bool {
		if c == '#' {
			cut = i
			return false
		}
		return true
	})
	return line[:cut]
}
