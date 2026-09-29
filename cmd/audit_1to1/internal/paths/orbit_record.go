package paths

import (
	"cmp"
	"maps"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// OrbitCheck holds the Orbit output types to what GitLab.com answered the
// requests their handlers build.
//
// No other oracle here says anything about them. The live API record is taken
// from a booted self-managed GitLab, which serves no Orbit route with a
// response entity: ee/lib/api/orbit/data.rb presents hashes that
// ee/lib/analytics/knowledge_graph/grpc_client.rb builds by hand from gRPC
// messages, and GitLab's generated OpenAPI document gives query, schema,
// status and tools no response schema either. So the type grain skipped every
// one of them, some for want of a pairing and the rest for want of a schema,
// and a key GitLab.com added or dropped reached no audit.
//
// The oracle is docs/development/orbit-responses.json, a recording
// cmd/gen_orbit_record makes through the handlers themselves. Each recorded
// call names the output type its handler returned, so the join needs no
// inference: the calls of one output type are unioned, and the type is walked
// field by field to every depth and held against the keys those answers
// carried. Two directions are asked, as the type grain asks them: a field the
// type publishes that no recorded answer carries, and a key an answer carries
// that no field publishes.
//
// The record keeps the JSON kind of every value, which no other record here
// does, so a third question is asked of every key, at the key and at every
// depth of elements below it, in two halves that ask it of two types (see
// [typeSource.kinds]). A handler never decodes GitLab's answer into its output
// type: it decodes it into the client-go struct a converter then reads, so a
// number that begins to arrive as a string fails that struct at run time, and
// the decoder half holds each struct a converter pairs with the output type to
// the kinds recorded at the keys it declares. The published half holds the
// output type to the same kinds, asking whether it carries each one as it came
// to the caller, a null as a null or as the key left out. Without either, the
// only trace of a changed kind would be a diff of a re-recording, which is
// then committed. The converter between the two is read by neither: a value
// it drops, reformats or defaults is not seen, and a null element of a list,
// which the Orbit converters skip, is reported against the published type all
// the same.
//
// Three rules keep it honest. A field whose Go type is not a struct of the
// package (a scalar, or a value decoded as any and passed on as it came) is a
// leaf, and nothing an answer carries below it is judged, since the server
// publishes whatever is there. A subtree the record kept verbatim, because its
// keys are data, shields what a type publishes below it; the whole body kept
// verbatim shields nothing, since the output type is then this server's own
// wrapper around it and has no key of GitLab's to publish, which a declaration
// says. And a finding is reported at the top of what is missing, not once per
// field below it. An answer that is a bare array is compared with the one
// field of a type that wraps it, which is this server's packaging and not a
// key GitLab could send, the way the type grain treats an envelope.
//
// It reports and never gates, for the reason [TypedShapeCheck] does: the
// recording is one instance on one day, and a finding is a candidate for the
// surface rather than a defect in it. What reaches the gate is a declaration
// that answers nothing and a recorded output type the tree no longer declares,
// both through the report's stale list.
type OrbitCheck struct {
	// Ran is false when the record could not be read, which is the one way
	// this check is skipped.
	Ran bool `json:"ran"`
	// Record names the artifact that answered.
	Record string `json:"record,omitempty"`
	// Source is what the record says it was taken from and when.
	Source orbitrecord.Source `json:"source,omitzero"`
	// Calls counts the recorded calls read.
	Calls int `json:"calls"`
	// Compared names the output types held against their calls, as
	// "package.Type", sorted.
	Compared []string `json:"types_compared,omitempty"`
	// Nested names the types reached below them and compared under the path
	// they sit at, sorted.
	Nested []string `json:"nested_types_compared,omitempty"`
	// Wrappers names the output types that wrap a bare array answer, whose
	// one field was compared with the array rather than reported.
	Wrappers []string `json:"bare_array_wrappers,omitempty"`
	// Unpublished are the fields a type publishes that no recorded answer of
	// its calls carries.
	Unpublished []OrbitField `json:"unpublished,omitempty"`
	// Unsurfaced are the keys a recorded answer carries that no field of the
	// type publishes.
	Unsurfaced []OrbitField `json:"unsurfaced,omitempty"`
	// KindsCompared counts the places the published half of the third
	// question was put: each field both sides have, at each depth of elements
	// a kind was recorded at, whose Go type could be judged. It is what tells
	// an empty Mismatched from a question nobody asked.
	KindsCompared int `json:"kinds_compared"`
	// Mismatched are the fields whose Go type does not carry a kind the
	// answers carried there, one per depth of elements the kind was recorded
	// at, the path spelled with that many element markers.
	Mismatched []OrbitField `json:"mismatched_kinds,omitempty"`
	// KindsUnjudged are the fields the answers carried whose Go type this
	// reader cannot map to a JSON kind: one that decodes or writes itself, one
	// from a package it does not model, one it cannot read. They are listed so
	// the third question's blind spot is counted rather than silent.
	KindsUnjudged []OrbitField `json:"kinds_unjudged,omitempty"`
	// DecodersRead is false when the converter pairing could not be had, which
	// is the one way the decoder half is skipped while the rest runs.
	DecodersRead bool `json:"decoders_read"`
	// Decoders names each output type compared and the client-go struct a
	// converter fills it from, as "package.Type from gl.Struct", sorted: what
	// the decoder half was put to.
	Decoders []string `json:"decoders,omitempty"`
	// Undecoded names the output types no converter pairs with a client-go
	// struct, which the decoder half has nothing to ask of.
	Undecoded []string `json:"outputs_without_decoder,omitempty"`
	// DecoderKindsCompared counts the places the decoder half was put, as
	// KindsCompared does for the published half.
	DecoderKindsCompared int `json:"decoder_kinds_compared"`
	// DecoderMismatched are the fields of a client-go struct that do not
	// decode a kind the answers carried there, which is a handler failing at
	// run time, spelled as Mismatched is and naming the struct as gl.Struct.
	DecoderMismatched []OrbitField `json:"decoder_mismatched_kinds,omitempty"`
	// DecoderKindsUnjudged are the fields of a client-go struct this reader
	// cannot map to a JSON kind, and the structs it cannot judge at all, one
	// that decodes its whole body itself or one the parse did not find, named
	// at the body's path.
	DecoderKindsUnjudged []OrbitField `json:"decoder_kinds_unjudged,omitempty"`
	// Missing names the output types the record names and the tree no longer
	// declares: the recording is of a handler that returns something else now.
	Missing []string `json:"types_not_found,omitempty"`
	// UnusedDeclarations names the declarations in orbit_declarations.go that
	// matched no finding, sorted.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// OrbitField is one disagreement between an output type and its recorded
// answers.
type OrbitField struct {
	// Package owns the output type, repository relative.
	Package string `json:"package"`
	// Output is the type the recorded calls returned.
	Output string `json:"output"`
	// Path is where the field sits under the output, element markers elided,
	// the way a Go field path reads (system.components.name).
	Path string `json:"path"`
	// Type is the Go type publishing the field, or the one a key the answers
	// carry would belong in.
	Type string `json:"type"`
	// Field is the json tag, or the key an answer carried.
	Field string `json:"field"`
	// GoType is the field's Go type as the source writes it, for a kind
	// finding.
	GoType string `json:"go_type,omitempty"`
	// Kinds are the JSON kinds the answers carried the key with, for a key
	// no field publishes, the kinds the field's Go type does not take, for
	// a kind finding, and every kind recorded there, for a field whose type
	// could not be judged.
	Kinds []string `json:"kinds,omitempty"`
	// Calls are the recorded calls searched.
	Calls []string `json:"calls"`
	// Category and Reason are the declaration accounting for the finding,
	// empty for one nothing answers.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for this finding.
func (f OrbitField) declared() bool { return f.Category != "" }

// undeclared counts the findings of the three questions, both halves of the
// third, no declaration accounts for, which is what a reader is asked to act
// on.
func (c OrbitCheck) undeclared() int {
	count := 0
	for _, findings := range [][]OrbitField{c.Unpublished, c.Unsurfaced, c.Mismatched, c.DecoderMismatched} {
		for _, finding := range findings {
			if !finding.declared() {
				count++
			}
		}
	}
	return count
}

// judged names every type this check compared, output or nested, as
// "package.Type" in the short spelling the type grain's skip lists use.
func (c OrbitCheck) judged() map[string]bool {
	names := map[string]bool{}
	for _, list := range [][]string{c.Compared, c.Nested} {
		for _, name := range list {
			names[name] = true
		}
	}
	return names
}

// staleDeclarations renders this run's unused declarations and missing types
// as findings for the report's stale list. A run that did not read the record
// has nothing to say about either.
func (c OrbitCheck) staleDeclarations() []string {
	if !c.Ran {
		return nil
	}
	stale := make([]string, 0, len(c.UnusedDeclarations)+len(c.Missing))
	for _, key := range c.UnusedDeclarations {
		stale = append(stale, key+" is declared against the Orbit response record and no finding matched it: the record now carries the key, or the type no longer publishes it")
	}
	for _, name := range c.Missing {
		stale = append(stale, name+" is named by the Orbit response record and the tree declares no such type: re-record with make gen-orbit-record")
	}
	return stale
}

// readOrbitRecord is the seam over the committed record, so a test hands the
// check a record without writing one.
var readOrbitRecord = orbitrecord.Read

// orbitCheck compares every output type the Orbit record names with its
// recorded answers.
func orbitCheck(root string) OrbitCheck {
	doc, err := readOrbitRecord(filepath.Join(root, orbitrecord.DefaultDir))
	if err != nil {
		return OrbitCheck{}
	}
	check := OrbitCheck{Ran: true, Record: orbitrecord.FileName, Source: doc.Source, Calls: len(doc.Calls)}
	decoders := readDecoders(root)
	check.DecodersRead = decoders != nil
	sources := map[string]*typeSource{}
	nested := map[string]bool{}
	for _, output := range outputsOf(doc.Calls) {
		pkg, name := splitOutput(output.name)
		source := sources[pkg]
		if source == nil {
			source = readTypeSource(root, pkg)
			sources[pkg] = source
		}
		if _, declared := source.structs[name]; !declared {
			check.Missing = append(check.Missing, output.name)
			continue
		}
		published := source.walk(name)
		recorded, wrapper := recordedKeys(output.calls, published.topLevel)
		if wrapper != "" {
			check.Wrappers = append(check.Wrappers, shortPackage(pkg)+"."+name)
		}
		judge := orbitJudgement{pkg: pkg, output: name, calls: output.labels(), published: published, recorded: recorded, types: source}
		check.Unpublished = append(check.Unpublished, judge.unpublished()...)
		check.Unsurfaced = append(check.Unsurfaced, judge.unsurfaced()...)
		kinds := judge.mismatched()
		check.KindsCompared += kinds.compared
		check.Mismatched = append(check.Mismatched, kinds.found...)
		check.KindsUnjudged = append(check.KindsUnjudged, kinds.unjudged...)
		check.Compared = append(check.Compared, shortPackage(pkg)+"."+name)
		for _, typeName := range published.structsBelow {
			nested[qualifiedType(pkg, typeName)] = true
		}
		if decoders != nil {
			decoders.judge(&check, output, pkg, name)
		}
	}
	check.Nested = slices.Sorted(maps.Keys(nested))
	for _, findings := range [][]OrbitField{
		check.Unpublished, check.Unsurfaced, check.Mismatched, check.KindsUnjudged, check.DecoderMismatched, check.DecoderKindsUnjudged,
	} {
		sortOrbitFindings(findings)
	}
	return check.classified()
}

// qualifiedType names a type the walk reached the way the type grain's lists
// do: a type of the package under the package's short name, and a shape
// internal/toolutil shares under the name it is already qualified by.
func qualifiedType(pkg, typeName string) string {
	if strings.Contains(typeName, ".") {
		return typeName
	}
	return shortPackage(pkg) + "." + typeName
}

// recordedOutput is one output type and the calls that returned it.
type recordedOutput struct {
	name  string
	calls []orbitrecord.Call
}

// labels names the calls the way a finding lists them.
func (o recordedOutput) labels() []string {
	labels := make([]string, 0, len(o.calls))
	for _, call := range o.calls {
		labels = append(labels, call.ID().String())
	}
	return labels
}

// outputsOf groups the calls by the output type they returned, sorted by type
// and, within one, in the record's own order.
func outputsOf(calls []orbitrecord.Call) []recordedOutput {
	byName := map[string]*recordedOutput{}
	var names []string
	for _, call := range calls {
		output := byName[call.Output]
		if output == nil {
			output = &recordedOutput{name: call.Output}
			byName[call.Output] = output
			names = append(names, call.Output)
		}
		output.calls = append(output.calls, call)
	}
	sort.Strings(names)
	out := make([]recordedOutput, 0, len(names))
	for _, name := range names {
		out = append(out, *byName[name])
	}
	return out
}

// splitOutput cuts internal/tools/orbit.StatusOutput into its package and its
// type name. A name with no package cuts to an empty one, which no tree
// declares a type in, so it is reported missing rather than read from the
// repository root.
func splitOutput(output string) (pkg, name string) {
	cut := strings.LastIndex(output, ".")
	if cut < 0 {
		return "", output
	}
	return output[:cut], output[cut+1:]
}

// typeSource is one package's structs, resolved the way the published-type
// walk resolves them: the package's own and the shapes internal/toolutil
// shares, the hints type aside. Beside them it keeps what the package says
// about its other types: what each is declared as, and which decode or write
// themselves. The decoder half reads client-go's package into one too, with
// no shared shapes.
//
// Only the package's own types are known that way. A type internal/toolutil
// declares as something other than a struct is one this reader cannot judge,
// and a shared struct is judged by its fields, which is right for every shared
// struct today, since none of them decodes or writes itself.
type typeSource struct {
	structs  map[string]declaredStruct
	scalars  map[string]bool
	named    map[string]goShape
	decoders map[string]bool
	encoders map[string]bool
}

// readTypeSource parses one package under the repository root.
func readTypeSource(root, pkg string) *typeSource {
	shared, _ := sharedShapes(filepath.Join(root, sharedDir), sharedPrefix)
	parsed := parsePackage(filepath.Join(root, filepath.FromSlash(pkg)))
	byName := shared
	for _, candidate := range parsed.structs {
		byName[candidate.Name] = candidate
	}
	return &typeSource{structs: byName, scalars: parsed.scalars, named: parsed.named, decoders: parsed.decoders, encoders: parsed.encoders}
}

// publishedTree is everything one output type publishes, at every depth.
type publishedTree struct {
	// fields maps each dotted path to what publishes it.
	fields map[string]publishedField
	// topLevel are the output's own fields, sorted.
	topLevel []string
	// structsBelow names the struct types reached below the output, sorted.
	structsBelow []string
}

// publishedField is one field of the walk.
type publishedField struct {
	// owner is the struct declaring the field.
	owner string
	// field is its json tag.
	field string
	// child is the struct the field carries, empty for a leaf.
	child string
	// shape is the field's Go type, which the kinds recorded at it are held
	// to.
	shape goShape
}

// walk publishes one output type's fields to every depth. A struct already on
// the path is not entered again, so a type that reaches itself ends there.
func (s *typeSource) walk(name string) publishedTree {
	tree := publishedTree{fields: map[string]publishedField{}}
	below := map[string]bool{}
	var visit func(structName, prefix string, walking map[string]bool)
	visit = func(structName, prefix string, walking map[string]bool) {
		walking[structName] = true
		defer delete(walking, structName)
		whole := flatten(s.structs[structName], s.structs, s.scalars, map[string]bool{})
		for _, field := range whole.Fields {
			path := joinPath(prefix, field)
			entry := publishedField{owner: structName, field: field, shape: whole.FieldShapes[field]}
			if child := whole.FieldTypes[field]; s.isStruct(child) && !walking[child] {
				entry.child = child
				below[child] = true
				visit(child, path, walking)
			}
			tree.fields[path] = entry
		}
	}
	visit(name, "", map[string]bool{})
	tree.topLevel = flatten(s.structs[name], s.structs, s.scalars, map[string]bool{}).Fields
	tree.structsBelow = slices.Sorted(maps.Keys(below))
	return tree
}

// isStruct reports whether a field's type is a struct this source resolves.
func (s *typeSource) isStruct(typeName string) bool {
	_, found := s.structs[typeName]
	return found
}

// recordedKey is what the answers of one output type carried at one path.
type recordedKey struct {
	// kinds are the JSON kinds carried, by the depth of elements they were
	// carried at: 0 for the key's own value, 1 for the elements of the list
	// it holds. The elided path spells both the same way, and a kind means
	// something only at its own depth.
	kinds    map[int]map[string]bool
	verbatim bool
}

// recordedKeys unions the key trees of one output type's calls, keyed by the
// elided path the Go walk spells. When every answer is a bare array and the
// type publishes exactly one field, that field is the array: the answers'
// paths are moved under it and its name is returned.
func recordedKeys(calls []orbitrecord.Call, topLevel []string) (keys map[string]recordedKey, wrapper string) {
	if bareArray(calls) && len(topLevel) == 1 {
		wrapper = topLevel[0]
	}
	keys = map[string]recordedKey{}
	for _, call := range calls {
		for _, key := range call.Response.Keys {
			path := joinPath(wrapper, orbitrecord.Elided(key.Path))
			depth := trailingElements(key.Path)
			entry := keys[path]
			if entry.kinds == nil {
				entry.kinds = map[int]map[string]bool{}
			}
			if entry.kinds[depth] == nil {
				entry.kinds[depth] = map[string]bool{}
			}
			for _, kind := range key.Kinds {
				entry.kinds[depth][kind] = true
			}
			entry.verbatim = entry.verbatim || key.Verbatim
			keys[path] = entry
		}
	}
	return keys, wrapper
}

// bareArray reports whether every call answered with an array body.
func bareArray(calls []orbitrecord.Call) bool {
	for _, call := range calls {
		for _, key := range call.Response.Keys {
			if key.Path == orbitrecord.Root && !slices.Equal(key.Kinds, []string{orbitrecord.KindArray}) {
				return false
			}
		}
	}
	return len(calls) > 0
}

// orbitJudgement is one output type held against its recorded answers, or,
// for the decoder half of the kinds question, the client-go struct its handler
// decodes them into.
type orbitJudgement struct {
	pkg       string
	output    string
	calls     []string
	published publishedTree
	recorded  map[string]recordedKey
	// types resolves a field's Go type for the kinds question.
	types *typeSource
	// decoding puts the kinds question under the decoder's reading rather
	// than the published type's (see [typeSource.kinds]).
	decoding bool
	// qualifier is written before the name of the struct a kind finding
	// names: empty for this repository's types, gl. for client-go's.
	qualifier string
}

// unpublished reports every published field no answer carries, at the top of
// what is missing, and nothing below a subtree the record kept verbatim.
func (j orbitJudgement) unpublished() []OrbitField {
	var found []OrbitField
	for _, path := range slices.Sorted(maps.Keys(j.published.fields)) {
		if _, carried := j.recorded[path]; carried {
			continue
		}
		parent := parentPath(path)
		if _, parentCarried := j.recorded[parent]; parent != "" && !parentCarried {
			continue
		}
		if j.shieldedByVerbatim(path) {
			continue
		}
		entry := j.published.fields[path]
		found = append(found, OrbitField{
			Package: j.pkg, Output: j.output, Path: path, Type: entry.owner, Field: entry.field, Calls: j.calls,
		})
	}
	return found
}

// shieldedByVerbatim reports whether a path sits below a subtree the record
// kept verbatim, the body itself aside.
func (j orbitJudgement) shieldedByVerbatim(path string) bool {
	segments := strings.Split(path, ".")
	for depth := range len(segments) - 1 {
		if j.recorded[strings.Join(segments[:depth+1], ".")].verbatim {
			return true
		}
	}
	return false
}

// unsurfaced reports every key an answer carries that no field publishes, at
// the top of what is missing, and nothing below a field published as a leaf,
// whose value is passed on whole.
func (j orbitJudgement) unsurfaced() []OrbitField {
	var found []OrbitField
	for _, path := range slices.Sorted(maps.Keys(j.recorded)) {
		if path == "" {
			continue
		}
		if _, published := j.published.fields[path]; published {
			continue
		}
		parent := parentPath(path)
		owner, parentPublished := j.published.fields[parent]
		if parent == "" {
			owner = publishedField{child: j.output}
		} else if !parentPublished || owner.child == "" {
			// Below a key already reported, or below a field that publishes
			// the value whole.
			continue
		}
		found = append(found, OrbitField{
			Package: j.pkg, Output: j.output, Path: path, Type: owner.child, Field: lastSegment(path),
			Kinds: slices.Sorted(maps.Keys(j.recorded[path].kinds[0])), Calls: j.calls,
		})
	}
	return found
}

// joinPath joins a dotted prefix and a segment, either of which may be empty.
func joinPath(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	if segment == "" {
		return prefix
	}
	return prefix + "." + segment
}

// parentPath is the dotted path one level up, "" at the top.
func parentPath(path string) string {
	return path[:max(strings.LastIndex(path, "."), 0)]
}

// lastSegment is the last key of a dotted path.
func lastSegment(path string) string {
	return path[strings.LastIndex(path, ".")+1:]
}

// sortOrbitFindings orders findings by package, output and path.
func sortOrbitFindings(found []OrbitField) {
	slices.SortFunc(found, func(a, b OrbitField) int {
		return cmp.Or(strings.Compare(a.Package, b.Package), strings.Compare(a.Output, b.Output), strings.Compare(a.Path, b.Path))
	})
}
