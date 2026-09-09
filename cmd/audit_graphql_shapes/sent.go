package main

import (
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"golang.org/x/tools/go/packages"
)

// The grain a sent finding is read at, and the claim it makes. REST asks the
// same question per package and per output type; here the walk starts at a
// document, but a package sends several and a field one document leaves out is
// routinely selected by a sibling, so a claim made per document would be false
// of the package a reader would act on. The claim is therefore package-wide:
// no document this package sends selects the field at that object. The
// document a finding names is the witness the field was offered at, not the
// extent of the claim.
const sentGrain = "package and schema type"

// What acting on a sent finding costs, which is the difference between a
// struct field, a struct, and a catalog action.
const (
	// sentLeaf is a scalar or an enum: one more field on the decoder and one
	// more on the output type.
	sentLeaf = "leaf"
	// sentObject is an object: a struct to decode it into, and a decision
	// about how much of it to publish.
	sentObject = "object"
	// sentCollection is a list or a connection: paging it is a new action
	// rather than a new field, so it is triaged apart.
	sentCollection = "collection"
)

// Whether the schema promises the field with every answer.
const (
	// sentAlways is a non-null field: GitLab sends it whenever it sends the
	// object, which is what an unconditional REST expose says over there.
	sentAlways = "always"
	// sentNullable is a nullable field: the schema allows null, and what
	// decides it is a resolver this audit cannot read.
	sentNullable = "nullable"
)

// pageInfoType is the connection cursor object. Nothing in it is payload, and
// a decoder that drops it is a pagination defect the "decoded and never
// selected" leg and toolutil.PaginationFromResponse already own.
const pageInfoType = "PageInfo"

// mutationIDField is the echo of a value this server never supplies, so it is
// structurally incapable of carrying information back.
const mutationIDField = "clientMutationId"

// errorsField is the user-facing error list every GitLab mutation payload
// carries. It is the one field of this whole dimension that gates.
const errorsField = "errors"

// connectionPlumbing are the field names that move a connection rather than
// carry it. Asking about them would report the transport on every paged
// object, and the answer is always the same.
var connectionPlumbing = map[string]bool{ //nolint:gochecknoglobals // the exclusion table this walk judges by
	"edges":    true,
	"nodes":    true,
	"node":     true,
	"cursor":   true,
	"pageInfo": true,
	"count":    true,
}

// sentField is one field the pinned schema offers at an object this server
// decodes, that the document never selects.
//
// It is paths.UnsurfacedField read against GraphQL, with the two
// annotations that make that list readable replaced by what GraphQL actually
// states. There is no tier and no deprecation here, and they are absent by
// fact rather than by omission: see [sentOracles].
type sentField struct {
	// Grain is always "package and schema type": the claim is that no
	// document this package sends selects the field at that object.
	Grain string `json:"grain"`
	// Package is the import path of the package the struct decoding the
	// object is declared in, which is the package that would have to publish
	// the field. It is not always the package the send is in: a note mutation
	// is sent through a wrapper in toolutil and decoded into the domain's own
	// struct, and filing the finding against the wrapper would name a package
	// that publishes nothing.
	Package string `json:"package"`
	// Document is the constant the document was declared as at the position
	// this field was witnessed offered and unselected. A sibling document of
	// the same package may name the same object and does not select this
	// field either, or the finding would not be here.
	Document string `json:"document"`
	// Position is where a reader opens that send, and HandedOverAt where the
	// document was named when the call received it through a parameter.
	Position     string `json:"position"`
	HandedOverAt string `json:"handed_over_at,omitempty"`
	// Operation names the operation the witness position was reached through,
	// since a document may define several.
	Operation string `json:"operation"`
	// Path is the response path from data down, for locating the position
	// only: the join key is the package, the schema type and the field.
	Path string `json:"path"`
	// SchemaType is the object the field is offered on, which is what the
	// REST entity names over there.
	SchemaType string `json:"schema_type"`
	// Type is the Go type decoding that object.
	Type string `json:"type"`
	// Field is the schema's field name, and FieldType its type as the SDL
	// spells it.
	Field     string `json:"field"`
	FieldType string `json:"field_type"`
	// Class is leaf, object or collection.
	Class string `json:"class"`
	// Sent is always or nullable.
	Sent string `json:"sent"`
	// SameNameInPackage names a field some output type of the package
	// publishes under the spelling this one would take, when one does.
	//
	// It is a lead and never an answer, and the name says so on purpose. The
	// match is over every output type of the package flattened together, not
	// over the type that models THIS object, because nothing here pairs a
	// GraphQL type with an output type. So a hit may be the same value under
	// another spelling, or a different object's field that happens to share a
	// name: vulnerabilities publishes web_url on its item and nothing ever
	// fills it, precisely because no document selects webUrl, and the older
	// spelling of this key called that "published" and invited a triager to
	// skip the row. A wrong reassurance is worse than none, since a triage
	// list is read by skipping what is annotated.
	SameNameInPackage string `json:"same_name_in_package,omitempty"`
	// Occurrences counts the positions this same field was offered at and
	// nothing selected it, since a finding is reported once per package,
	// schema type and field.
	Occurrences int `json:"occurrences,omitempty"`
	// Category and Reason are the declaration in sent_declarations.go
	// accounting for the finding, when one does. Empty for the half a reader
	// is asked to act on, exactly as REST leaves it.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`

	// position and origin are where the pairing was, kept unrendered so the
	// paths above can be written relative to the audited root.
	position token.Position
	origin   token.Position
}

// declared reports whether a declaration accounts for the finding.
func (f sentField) declared() bool { return f.Category != "" }

// sentCoverage counts what became of every object position the walk entered,
// so the report says how much of the surface the question was asked of rather
// than only how often it was answered.
//
// The distinction is the point: Asked counts one position per schema type per
// pairing and is therefore smaller than the surface a reader would assume from
// it, and every skip beside it is a place where the schema offers fields
// nothing here will ever report. Published as counters rather than as prose
// because each of them moves with the tree.
//
// Reached is Asked plus Traversed plus RepeatedType and nothing else. The
// three counters under those are positions the walk left before a struct was
// ever matched against the object, so they are outside Reached rather than
// part of it, and summing all seven would count nothing twice but would mean
// nothing either.
type sentCoverage struct {
	// Reached counts the object positions the walk entered with a struct to
	// judge them by, excluding the operation roots and the cursor object,
	// which are never part of a response.
	Reached int `json:"positions_reached"`
	// Asked counts the positions the question was put at: one per schema type
	// per pairing, since a type reached twice in one pairing is asked about
	// once. A later position of the same type is skipped even when it selects
	// strictly less, which is what RepeatedType counts.
	Asked int `json:"positions_asked"`
	// Traversed counts the positions skipped for decoding no scalar or enum:
	// a struct that reads only the next hop is an envelope, and its object is
	// not something this server publishes.
	Traversed int `json:"positions_skipped_traversed"`
	// RepeatedType counts the positions skipped because the pairing had
	// already been asked about that schema type.
	RepeatedType int `json:"positions_skipped_repeated_type"`
	// Undecoded counts the object selections no Go field reads, where the walk
	// stops with a note and never reaches whatever the schema offers under
	// them.
	Undecoded int `json:"positions_skipped_undecoded"`
	// Map counts the objects decoded into a map, which has no fields to judge
	// and no package to file a finding against, so neither the question nor
	// the mutation-errors gate is put there.
	Map int `json:"positions_skipped_map"`
	// SelfDecoding counts the objects decoded by a type that unmarshals
	// itself, which is trusted to know what it reads and so is walked no
	// further.
	SelfDecoding int `json:"positions_skipped_self_decoding"`
}

// add folds one pairing's counters into the run's.
func (c *sentCoverage) add(other sentCoverage) {
	c.Reached += other.Reached
	c.Asked += other.Asked
	c.Traversed += other.Traversed
	c.RepeatedType += other.RepeatedType
	c.Undecoded += other.Undecoded
	c.Map += other.Map
	c.SelfDecoding += other.SelfDecoding
}

// key is the join key: the grain a reader triages at, what the declaration
// table is keyed by, and what the evidence a document did select is recorded
// under, so a sibling document's selection cancels the finding. Deliberately
// not the path, which is brittle and multi-valued for one type, since WorkItem
// alone is reached at seven positions.
func (f sentField) key() string { return sentKey(f.Package, f.SchemaType, f.Field) }

// sentKey spells the join key, so a finding and the evidence against it are
// built the same way.
func sentKey(pkg, schemaType, field string) string { return pkg + "." + schemaType + "." + field }

// objectPosition is one place in the walk where a GraphQL object meets the Go
// struct that decodes it: the schema type, that struct's fields, the selection
// set the document wrote there, what it selected and what of that it decoded,
// whether the struct reads a leaf of its own rather than only the next hop, the
// dotted path a finding is named by, and the Go type itself.
//
// The walk carries them as one value because they describe one position and
// are never passed apart; spelled out they are eight arguments in a row, of
// which two are maps of the same type and one is a bare bool.
type objectPosition struct {
	gqlType    *ast.Type
	fields     []goField
	selections ast.SelectionSet
	selected   map[string]bool
	decoded    map[string]bool
	readsLeaf  bool
	path       string
	goType     types.Type
}

// askSchema reports the fields the schema offers at one object this server
// decodes and the document does not select, records what the document did
// select as the evidence that cancels a sibling document's finding, and gates
// the one sub-class that can be gated.
//
// Three conditions decide whether the question is asked here at all, and each
// of them is what keeps this dimension a backlog rather than a schema dump.
// The position must be one a Go struct decodes, which bounds the walk by our
// own decoders instead of by the schema graph. It must not be the operation
// root, whose fields are other requests rather than this response: measured
// against the real documents, the root alone offers seventeen thousand of the
// twenty-two thousand fields a naive walk would report, and what it offers is
// the catalog question the action catalog already answers. And the object must
// be read rather than merely traversed, which a struct decoding at least one
// scalar or enum shows: a struct that decodes only the next hop is an
// envelope, and the objects that rule removes are Project, Group, Namespace,
// Pipeline and User, every one of them a domain R-PATH already asks the sent
// question of against GitLab's own OpenAPI record, with a tier-aware oracle
// this one does not have.
func (j *judge) askSchema(at objectPosition) {
	definition := j.schema.Types[at.gqlType.NamedType]
	if definition == nil || j.isRoot(definition) || definition.Name == pageInfoType {
		return
	}
	pkg := decoderPackage(at.goType, j.pairing.Package)
	j.recordSelected(pkg, definition, at.decoded)
	j.gateMutationErrors(definition, at.fields, at.path)

	j.coverage.Reached++
	switch {
	case !at.readsLeaf:
		j.coverage.Traversed++
		return
	case j.asked[definition.Name]:
		j.coverage.RepeatedType++
		return
	}
	j.asked[definition.Name] = true
	j.coverage.Asked++

	reported := map[string]bool{}
	for _, offering := range j.offering(definition, at.selections) {
		for _, field := range offering.Fields {
			if at.selected[field.Name] || reported[field.Name] || skipSentField(field) {
				continue
			}
			reported[field.Name] = true
			j.sent = append(j.sent, sentField{
				Grain:      sentGrain,
				Package:    pkg,
				Document:   j.pairing.Label(),
				Operation:  j.operation,
				Path:       at.path,
				SchemaType: definition.Name,
				Type:       typeString(at.goType),
				Field:      field.Name,
				FieldType:  field.Type.String(),
				Class:      j.classOf(field.Type),
				Sent:       sentOf(field.Type),
				position:   j.pairing.Position,
				origin:     j.pairing.Origin,
			})
		}
	}
}

// recordSelected records what this position selected AND decoded, under the
// key a finding about the same object is reported at.
//
// It is what makes the package-wide claim true. A package sends several
// documents at one object and they deliberately differ: branchrules sends a CE
// document that omits codeOwnerApprovalRequired precisely because the field is
// Premium and an EE document that selects it, and epicissues sends a query
// that reads a work item and a mutation whose payload can echo nothing but the
// id. Judging each document alone reports the sibling's selections as gaps,
// which is how a tier expression came out as a missing field.
//
// Decoded and not merely selected, because the claim being cancelled is that
// the package never surfaces the value. A document that asks GitLab for a
// field and drops it on the floor surfaces nothing, so counting it here would
// let one dead selection silence the package's real gap — the same
// union-standing-for-an-intersection error the package grain was fixed for,
// one level down and harder to see. The sibling leg already reports such a
// selection as one nothing reads.
func (j *judge) recordSelected(pkg string, definition *ast.Definition, decoded map[string]bool) {
	for name := range decoded {
		j.selected[sentKey(pkg, definition.Name, name)] = true
	}
}

// countSelfDecoding, countMap and countUndecoded record the three positions
// the walk leaves without asking the schema anything, so a coverage figure
// says where the question was not put rather than implying it was put
// everywhere. Each is the counterpart of a stop written for a reason the shape
// leg needs, and none of them can be closed here: a type that unmarshals
// itself is trusted with its own decoding, a map has neither fields to judge
// nor a package to file a finding against, and an object no Go field reads has
// no decoder to compare the schema with.
func (j *judge) countSelfDecoding(gqlType *ast.Type) {
	if j.isAskablePosition(gqlType) {
		j.coverage.SelfDecoding++
	}
}

func (j *judge) countMap(gqlType *ast.Type) {
	if j.isAskablePosition(gqlType) {
		j.coverage.Map++
	}
}

func (j *judge) countUndecoded(field *ast.Field) {
	if len(field.SelectionSet) > 0 && j.isAskablePosition(fieldType(field)) {
		j.coverage.Undecoded++
	}
}

// isAskablePosition reports whether a position is one the question could have
// been put at: an object of the response, rather than a leaf, an operation
// root or the cursor object.
func (j *judge) isAskablePosition(gqlType *ast.Type) bool {
	for gqlType.Elem != nil {
		gqlType = gqlType.Elem
	}
	definition := j.schema.Types[gqlType.NamedType]
	if definition == nil || j.isRoot(definition) || definition.Name == pageInfoType {
		return false
	}
	switch definition.Kind {
	case ast.Object, ast.Interface, ast.Union:
		return true
	default:
		return false
	}
}

// gateMutationErrors fails a mutation payload whose user-facing errors the
// decoder has no field for.
//
// It is the only sub-class of this dimension that gates, and it gates because
// it is not a candidate for the surface: a payload whose errors we cannot
// decode drops GitLab's own account of why a mutation did nothing, and the
// tool reports success. What the document asks for is not the condition, and
// used to be: a payload that selects errors and decodes none fails in exactly
// the same way as one that selects nothing, since the values arrive and are
// thrown away. The other half needs no gate here, because a decoder field the
// document never selects is already a hard failure of the leg above.
func (j *judge) gateMutationErrors(definition *ast.Definition, fields []goField, path string) {
	if !isMutationPayload(definition) || lookupField(fields, errorsField) != nil {
		return
	}
	j.fail(path+"."+errorsField, definition.Name+" is a mutation payload whose errors no field of the decoder reads, so a mutation GitLab refused reads as one that worked")
}

// decoderPackage names the package the struct decoding an object is declared
// in, falling back to the package the send is in for a type no package owns.
//
// It is the package that would have to publish a field the document leaves
// out, which the package the call sits in is not: every epic note and
// discussion mutation is sent by a shared wrapper in toolutil and decoded into
// the domain's own struct, so attributing a finding, or the answer to whether
// the value is published already, to the send names a package that publishes
// nothing.
func decoderPackage(goType types.Type, fallback string) string {
	if named, ok := goType.(*types.Named); ok && named.Obj() != nil && named.Obj().Pkg() != nil {
		return named.Obj().Pkg().Path()
	}
	return fallback
}

// offering lists the definitions whose fields are asked about at one position.
//
// A union definition carries no fields of its own, so asking it alone would
// silently report nothing and lose the whole location family of a security
// finding. Asking about every member instead would report every location
// variant on every finding, which is the schema-graph explosion in miniature.
// So a union is asked once per member the document names in a fragment, and
// never about a member it does not name; an interface is asked about its own
// fields and each named implementation on top.
func (j *judge) offering(definition *ast.Definition, selections ast.SelectionSet) []*ast.Definition {
	var offering []*ast.Definition
	if definition.Kind != ast.Union {
		offering = append(offering, definition)
	}
	if definition.Kind == ast.Object {
		return offering
	}
	for _, name := range typeConditions(selections) {
		if name == definition.Name {
			continue
		}
		for _, possible := range j.schema.PossibleTypes[definition.Name] {
			if possible.Name == name {
				offering = append(offering, possible)
				break
			}
		}
	}
	return offering
}

// isRoot reports whether a definition is an operation root, whose fields are
// other requests rather than part of any response.
func (j *judge) isRoot(definition *ast.Definition) bool {
	for _, root := range []*ast.Definition{j.schema.Query, j.schema.Mutation, j.schema.Subscription} {
		if root != nil && root.Name == definition.Name {
			return true
		}
	}
	return false
}

// classOf says what acting on a field would cost.
func (j *judge) classOf(fieldType *ast.Type) string {
	if fieldType.Elem != nil {
		return sentCollection
	}
	definition := j.schema.Types[fieldType.NamedType]
	if definition == nil {
		return sentObject
	}
	switch definition.Kind {
	case ast.Scalar, ast.Enum:
		return sentLeaf
	case ast.Object, ast.Interface, ast.Union:
		if isConnection(definition) {
			return sentCollection
		}
		return sentObject
	default:
		return sentObject
	}
}

// isLeafType reports whether a type bottoms out in a scalar or an enum, which
// is what distinguishes an object that is read from one that is traversed.
func (j *judge) isLeafType(fieldType *ast.Type) bool {
	for fieldType.Elem != nil {
		fieldType = fieldType.Elem
	}
	definition := j.schema.Types[fieldType.NamedType]
	return definition != nil && (definition.Kind == ast.Scalar || definition.Kind == ast.Enum)
}

// sentOf says whether the schema promises the field with every answer.
func sentOf(fieldType *ast.Type) string {
	if fieldType.NonNull {
		return sentAlways
	}
	return sentNullable
}

// isConnection reports whether an object is a Relay connection, which is what
// makes a field on it a collection rather than an object.
func isConnection(definition *ast.Definition) bool {
	return definition.Fields.ForName("nodes") != nil || definition.Fields.ForName("edges") != nil
}

// isMutationPayload reports whether an object is a GitLab mutation payload,
// which every one of them says by carrying the client mutation id beside the
// errors.
func isMutationPayload(definition *ast.Definition) bool {
	return definition.Fields.ForName(mutationIDField) != nil && definition.Fields.ForName(errorsField) != nil
}

// skipSentField reports whether a field is excluded from the question, each
// exclusion costing what its comment says against the real documents.
func skipSentField(field *ast.FieldDefinition) bool {
	if connectionPlumbing[field.Name] || field.Name == mutationIDField || strings.HasPrefix(field.Name, "__") {
		return true
	}
	// A field you must supply an identifier to fetch is a second request
	// rather than something GitLab sends with this response. One field of the
	// pinned schema is lost this way, UserCore.savedReply, and it is plainly
	// a lookup.
	for _, argument := range field.Arguments {
		if argument.Type.NonNull && argument.DefaultValue == nil {
			return true
		}
	}
	return false
}

// typeConditions lists the type names a selection set names in its fragments,
// which is what decides the members of a union or interface this walk asks
// about.
func typeConditions(selections ast.SelectionSet) []string {
	var names []string
	for _, selection := range selections {
		switch s := selection.(type) {
		case *ast.InlineFragment:
			if s.TypeCondition != "" {
				names = append(names, s.TypeCondition)
			}
			names = append(names, typeConditions(s.SelectionSet)...)
		case *ast.FragmentSpread:
			if s.Definition != nil {
				names = append(names, s.Definition.TypeCondition)
				names = append(names, typeConditions(s.Definition.SelectionSet)...)
			}
		}
	}
	return names
}

// publishedIndex is what each package publishes, by package path then by the
// normalized spelling of a field name.
type publishedIndex map[string]map[string]string

// outputTypeSuffixes name the exported types a package publishes to a model.
var outputTypeSuffixes = []string{"Output", "Item"} //nolint:gochecknoglobals // the naming convention this repository publishes under

// collectPublished reads what every package publishes, so a field the
// document never selects can say whether the value reaches a caller anyway.
//
// The comparison is against the package's own output types rather than the
// struct the document decodes into: the existing leg already fails a Go field
// no selection reads, so a decoder can never carry a field the document
// leaves out, and comparing against it would answer nothing. Names are
// normalized because GitLab spells a field camelCase and this repository
// publishes it snake_case.
func collectPublished(pkgs []*packages.Package) publishedIndex {
	index := publishedIndex{}
	for _, pkg := range pkgs {
		if pkg.Types == nil {
			continue
		}
		names := map[string]string{}
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			named, ok := scope.Lookup(name).(*types.TypeName)
			if !ok || !named.Exported() || !publishesOutput(name) {
				continue
			}
			if body, isStruct := named.Type().Underlying().(*types.Struct); isStruct {
				collectPublishedFields(body, names, map[string]bool{})
			}
		}
		if len(names) > 0 {
			index[pkg.PkgPath] = names
		}
	}
	return index
}

// publishesOutput reports whether a type name is one this repository publishes
// a response under.
func publishesOutput(name string) bool {
	for _, suffix := range outputTypeSuffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// collectPublishedFields records every json name a type publishes, walking
// into the structs it nests, since a value GitLab offers at one level may be
// published a level down.
func collectPublishedFields(body *types.Struct, names map[string]string, seen map[string]bool) {
	for _, field := range jsonFields(body) {
		if _, ok := names[normalizeFieldName(field.name)]; !ok {
			names[normalizeFieldName(field.name)] = field.name
		}
		nested, ok := structOf(field.typ)
		if !ok {
			continue
		}
		key := types.TypeString(field.typ, nil)
		if seen[key] {
			continue
		}
		seen[key] = true
		collectPublishedFields(nested, names, seen)
	}
}

// structOf reads through pointers, slices and arrays to the struct a field
// holds, when it holds one.
func structOf(goType types.Type) (*types.Struct, bool) {
	for {
		switch t := goType.Underlying().(type) {
		case *types.Pointer:
			goType = t.Elem()
		case *types.Slice:
			goType = t.Elem()
		case *types.Array:
			goType = t.Elem()
		case *types.Struct:
			return t, true
		default:
			return nil, false
		}
	}
}

// normalizeFieldName reduces a field name to what two spellings of one value
// have in common, so startLine and start_line compare equal.
func normalizeFieldName(name string) string {
	var normalized strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z':
			normalized.WriteRune(r + ('a' - 'A'))
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			normalized.WriteRune(r)
		}
	}
	return normalized.String()
}

// dedupeSent reduces the raw occurrences to one finding per package, schema
// type and field, dropping every field some document of that package did
// select and counting the rest.
//
// The subtraction is what makes the reported list an intersection rather than
// a union of per-document answers. A raw occurrence is one document's silence
// at one position; a package's other documents are the rest of the evidence,
// and a field any of them selects at that object is published by the package
// and is not a gap in it. Without it the largest packages read as
// nineteen percent false, and the falsehood lands exactly where a reader looks
// first: a tier expressed as a CE and an EE document reported the EE half as
// missing, and a mutation payload that can echo nothing but an id reported the
// query's whole work item.
//
// The grain is deliberately not the schema type alone: epicdiscussions and
// epicnotes both failing to surface an author flag is two gaps in two tools,
// and a reader closing one does not close the other.
func dedupeSent(found []sentField, published publishedIndex, selected map[string]bool, root string) []sentField {
	byKey := map[string]int{}
	var deduped []sentField
	for _, field := range found {
		if selected[field.key()] {
			continue
		}
		if at, ok := byKey[field.key()]; ok {
			deduped[at].Occurrences++
			continue
		}
		field.Occurrences = 1
		field.Position = relative(field.position, root)
		if field.origin.IsValid() {
			field.HandedOverAt = relative(field.origin, root)
		}
		if name, ok := published[field.Package][normalizeFieldName(field.Field)]; ok {
			field.SameNameInPackage = name
		}
		byKey[field.key()] = len(deduped)
		deduped = append(deduped, field)
	}
	sortSent(deduped)
	return deduped
}

// sortSent orders the findings the way a reader reads them: by package, then
// by the object they were read on, then by field.
func sortSent(found []sentField) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].Package != found[j].Package {
			return found[i].Package < found[j].Package
		}
		if found[i].SchemaType != found[j].SchemaType {
			return found[i].SchemaType < found[j].SchemaType
		}
		return found[i].Field < found[j].Field
	})
}
